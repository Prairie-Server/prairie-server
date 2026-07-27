package imageutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
)

type nvencEncoder struct {
	ffmpeg   string
	quality  int
	minEdge  int
	fallback AVIFEncoder
	sem      chan struct{}
	mu       sync.Mutex
}

func newNVENCEncoder(cfg EncoderConfig, fallback AVIFEncoder) *nvencEncoder {
	sessions := cfg.NVENCSessions
	if sessions < 1 {
		sessions = 3
	}
	return &nvencEncoder{
		ffmpeg:   cfg.FFmpegPath,
		quality:  cfg.Quality,
		minEdge:  cfg.NVENCMinEdge,
		fallback: fallback,
		sem:      make(chan struct{}, sessions),
	}
}

func (e *nvencEncoder) Name() string { return BackendNVENC }

func (e *nvencEncoder) Encode(ctx context.Context, imageBytes []byte, widthHint int) ([]byte, error) {
	if widthHint > 0 && widthHint < e.minEdge {
		return e.fallback.Encode(ctx, imageBytes, widthHint)
	}
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	out, err := e.encodeNVENC(ctx, imageBytes)
	if err == nil {
		return out, nil
	}
	// Opportunistic: any NVENC failure falls back to SVT/WASM for this frame.
	return e.fallback.Encode(ctx, imageBytes, widthHint)
}

func (e *nvencEncoder) encodeNVENC(ctx context.Context, imageBytes []byte) ([]byte, error) {
	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("nvenc avif: empty input")
	}
	dir, err := os.MkdirTemp("", "prairie-avif-nvenc-")
	if err != nil {
		return nil, fmt.Errorf("nvenc avif: scratch: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	inPath := filepath.Join(dir, "in.bin")
	outPath := filepath.Join(dir, "out.avif")
	if err := os.WriteFile(inPath, imageBytes, 0o644); err != nil {
		return nil, fmt.Errorf("nvenc avif: write input: %w", err)
	}

	// Still-image encode: single I-frame, constqp. ffmpeg muxes AV1 into AVIF.
	// CQ/QP mapped from quality (90 → qp ~28).
	qp := qualityToNVENCQP(e.quality)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inPath,
		"-frames:v", "1",
		"-c:v", "av1_nvenc",
		"-preset", "p4",
		"-tune", "hq",
		"-rc", "constqp",
		"-qp", strconv.Itoa(qp),
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	}
	cmd := exec.CommandContext(ctx, e.ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nvenc avif: ffmpeg: %w (%s)", err, trim(stderr.String()))
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("nvenc avif: read output: %w", err)
	}
	if len(out) < 12 {
		return nil, fmt.Errorf("nvenc avif: output too small (%d bytes)", len(out))
	}
	return out, nil
}

func qualityToNVENCQP(quality int) int {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	// quality 90 → qp 28; quality 100 → 18; quality 50 → 45.
	qp := 18 + (100-quality)*40/100
	if qp < 10 {
		qp = 10
	}
	if qp > 51 {
		qp = 51
	}
	return qp
}
