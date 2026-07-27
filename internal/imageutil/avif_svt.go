package imageutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

type svtEncoder struct {
	ffmpeg  string
	quality int
	speed   int
}

func newSVTEncoder(cfg EncoderConfig) *svtEncoder {
	return &svtEncoder{
		ffmpeg:  cfg.FFmpegPath,
		quality: cfg.Quality,
		speed:   cfg.Speed,
	}
}

func (e *svtEncoder) Name() string { return BackendSVT }

func (e *svtEncoder) Encode(ctx context.Context, imageBytes []byte, _ int) ([]byte, error) {
	if err := encodeBudget.Acquire(ctx); err != nil {
		return nil, err
	}
	defer encodeBudget.Release()
	return e.encodeUnlocked(ctx, imageBytes)
}

func (e *svtEncoder) encodeUnlocked(ctx context.Context, imageBytes []byte) ([]byte, error) {
	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("svt avif: empty input")
	}
	dir, err := os.MkdirTemp("", "prairie-avif-svt-")
	if err != nil {
		return nil, fmt.Errorf("svt avif: scratch: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	inPath := filepath.Join(dir, "in.bin")
	outPath := filepath.Join(dir, "out.avif")
	if err := os.WriteFile(inPath, imageBytes, 0o644); err != nil {
		return nil, fmt.Errorf("svt avif: write input: %w", err)
	}

	crf := qualityToCRF(e.quality)
	preset := speedToSVTPreset(e.speed)
	// lp=1: one logical processor per encode so NumCPU concurrent encodes do
	// not oversubscribe (SVT otherwise grabs all cores per process).
	// SVT rejects frames shorter than 64px; pad/scale so tiny test/logo edges work.
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inPath,
		"-vf", "pad=max(iw\\,64):max(ih\\,64):(ow-iw)/2:(oh-ih)/2",
		"-frames:v", "1",
		"-c:v", "libsvtav1",
		"-preset", strconv.Itoa(preset),
		"-crf", strconv.Itoa(crf),
		"-svtav1-params", "lp=1",
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	}
	cmd := exec.CommandContext(ctx, e.ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("svt avif: ffmpeg: %w (%s)", err, trim(stderr.String()))
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("svt avif: read output: %w", err)
	}
	if len(out) < 12 {
		return nil, fmt.Errorf("svt avif: output too small (%d bytes)", len(out))
	}
	return out, nil
}
