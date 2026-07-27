package imageutil

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	_ "golang.org/x/image/webp"
)

// svtMinEdge is SVT-AV1's minimum frame edge. Below this the encoder rejects
// the frame; we upscale (never downscale) so both edges meet the floor.
const svtMinEdge = 64

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
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-threads", "1",
		"-filter_threads", "1",
		"-i", inPath,
	}
	if vf := svtPrepareFilter(imageBytes); vf != "" {
		args = append(args, "-vf", vf)
	}
	args = append(args,
		"-frames:v", "1",
		"-c:v", "libsvtav1",
		"-preset", strconv.Itoa(preset),
		"-crf", strconv.Itoa(crf),
		"-svtav1-params", "lp=1",
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	)
	cmd := exec.CommandContext(ctx, e.ffmpeg, args...)
	cmd.Env = append(os.Environ(), "OMP_NUM_THREADS=1")
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

// svtPrepareFilter returns an ffmpeg -vf string that makes the frame safe for
// SVT-AV1 + yuv420p without forcing a fixed canvas:
//
//   - never shrink (matches WASM: narrower-than-target sources are not upscaled
//     for the width ladder; here we only grow when below SVT's 64px floor)
//   - pad at most 1px per edge to reach even dimensions for 4:2:0
//   - never pad to a target smaller than the input (the prior max(iw,64) pad
//     expression broke on some landscape/logo paths)
func svtPrepareFilter(imageBytes []byte) string {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		// Unknown dims: use a safe upscale-only filter that never pads down.
		return "scale='max(iw\\,64)':'max(ih\\,64)':force_original_aspect_ratio=increase," +
			"pad='ceil(iw/2)*2':'ceil(ih/2)*2':(ow-iw)/2:(oh-ih)/2"
	}
	w, h := cfg.Width, cfg.Height
	outW, outH := svtOutputSize(w, h)
	if outW == w && outH == h {
		return ""
	}
	// Upscale when below the SVT floor; otherwise only even-pad (≤1px).
	if outW > w+1 || outH > h+1 {
		return fmt.Sprintf("scale=%d:%d:flags=bilinear", outW, outH)
	}
	return fmt.Sprintf("pad=%d:%d:0:0:color=black", outW, outH)
}

// svtOutputSize returns the encode frame size: at least svtMinEdge on each
// side (aspect preserved when upscaling) and even width/height for yuv420p.
func svtOutputSize(w, h int) (int, int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	outW, outH := w, h
	if outW < svtMinEdge || outH < svtMinEdge {
		scaleW := float64(svtMinEdge) / float64(outW)
		scaleH := float64(svtMinEdge) / float64(outH)
		scale := scaleW
		if scaleH > scale {
			scale = scaleH
		}
		outW = int(float64(w)*scale + 0.999) // ceil
		outH = int(float64(h)*scale + 0.999)
	}
	if outW%2 != 0 {
		outW++
	}
	if outH%2 != 0 {
		outH++
	}
	if outW < svtMinEdge {
		outW = svtMinEdge
		if outW%2 != 0 {
			outW++
		}
	}
	if outH < svtMinEdge {
		outH = svtMinEdge
		if outH%2 != 0 {
			outH++
		}
	}
	return outW, outH
}
