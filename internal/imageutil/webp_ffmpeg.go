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

// ffmpegWebPEncoder decodes, resizes, and encodes WebP via ffmpeg/libwebp.
// One encode-budget slot covers the full variant ladder (matches WASM).
type ffmpegWebPEncoder struct {
	ffmpeg  string
	quality int
}

func newFFmpegWebPEncoder(cfg WebPEncoderConfig) *ffmpegWebPEncoder {
	return &ffmpegWebPEncoder{
		ffmpeg:  cfg.FFmpegPath,
		quality: cfg.Quality,
	}
}

func (e *ffmpegWebPEncoder) Name() string { return WebPBackendFFmpeg }

func (e *ffmpegWebPEncoder) GenerateVariants(ctx context.Context, data []byte, widths []int) (*VariantResult, error) {
	return e.generate(ctx, data, widths, false)
}

func (e *ffmpegWebPEncoder) GenerateSquareVariants(ctx context.Context, data []byte, sizes []int) (*VariantResult, error) {
	return e.generate(ctx, data, sizes, true)
}

func (e *ffmpegWebPEncoder) generate(ctx context.Context, data []byte, dims []int, square bool) (*VariantResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ffmpeg webp: empty input")
	}
	if err := encodeBudget.Acquire(ctx); err != nil {
		return nil, err
	}
	defer encodeBudget.Release()

	srcW, srcH, err := decodeImageSize(data)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg webp: probe: %w", err)
	}

	dir, err := os.MkdirTemp("", "prairie-webp-")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg webp: scratch: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	inPath := filepath.Join(dir, "in.bin")
	if err := os.WriteFile(inPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("ffmpeg webp: write input: %w", err)
	}

	out := &VariantResult{Ext: ".webp", Variants: make([]Variant, 0, len(dims)+1)}

	// original (capped for non-square; square original is the center crop)
	origW, origH := srcW, srcH
	if square {
		side := srcW
		if srcH < side {
			side = srcH
		}
		origW, origH = side, side
	} else if origW > maxCachedOriginalDimension || origH > maxCachedOriginalDimension {
		origW, origH = fitInside(srcW, srcH, maxCachedOriginalDimension)
	}
	origW, origH = evenSize(origW, origH)
	origBytes, err := e.encodeOne(ctx, inPath, filepath.Join(dir, "original.webp"), origW, origH, square)
	if err != nil {
		return nil, err
	}
	out.Variants = append(out.Variants, Variant{Key: "original", Data: origBytes})

	seen := map[int]bool{}
	for _, dim := range dims {
		if dim <= 0 || seen[dim] {
			continue
		}
		seen[dim] = true
		targetW, targetH := dim, 0
		if square {
			targetW, targetH = dim, dim
		} else {
			// Never upscale: match WASM / prior bimg contract.
			if srcW <= dim {
				targetW, targetH = srcW, srcH
			} else {
				targetW, targetH = fitWidth(srcW, srcH, dim)
			}
		}
		targetW, targetH = evenSize(targetW, targetH)
		key := "w" + strconv.Itoa(dim)
		payload, encErr := e.encodeOne(ctx, inPath, filepath.Join(dir, key+".webp"), targetW, targetH, square)
		if encErr != nil {
			return nil, encErr
		}
		out.Variants = append(out.Variants, Variant{Key: key, Data: payload})
	}
	return out, nil
}

func (e *ffmpegWebPEncoder) encodeOne(ctx context.Context, inPath, outPath string, w, h int, square bool) ([]byte, error) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	vf := fmt.Sprintf("scale=%d:%d:flags=bilinear", w, h)
	if square {
		// Center-crop to square then scale to target edge.
		vf = fmt.Sprintf(
			"crop='min(iw\\,ih)':'min(iw\\,ih)':'(iw-min(iw\\,ih))/2':'(ih-min(iw\\,ih))/2',scale=%d:%d:flags=bilinear",
			w, h,
		)
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-threads", "1",
		"-filter_threads", "1",
		"-i", inPath,
		"-vf", vf,
		"-frames:v", "1",
		"-c:v", "libwebp",
		"-quality", strconv.Itoa(e.quality),
		"-compression_level", "4",
		"-pix_fmt", "yuv420p",
		"-an",
		outPath,
	}
	cmd := exec.CommandContext(ctx, e.ffmpeg, args...)
	cmd.Env = append(os.Environ(), "OMP_NUM_THREADS=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runEncodeCommand(cmd); err != nil {
		return nil, fmt.Errorf("ffmpeg webp: %w (%s)", err, trim(stderr.String()))
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg webp: read output: %w", err)
	}
	if len(out) < 12 {
		return nil, fmt.Errorf("ffmpeg webp: output too small (%d bytes)", len(out))
	}
	return out, nil
}

func decodeImageSize(data []byte) (int, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return 0, 0, fmt.Errorf("invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	return cfg.Width, cfg.Height, nil
}

func fitInside(w, h, maxDim int) (int, int) {
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	if w >= h {
		return fitWidth(w, h, maxDim)
	}
	nw := int(float64(w) * float64(maxDim) / float64(h))
	if nw < 1 {
		nw = 1
	}
	return nw, maxDim
}

func fitWidth(w, h, targetW int) (int, int) {
	if w <= targetW {
		return w, h
	}
	nh := int(float64(h) * float64(targetW) / float64(w))
	if nh < 1 {
		nh = 1
	}
	return targetW, nh
}

// evenSize bumps odd edges by 1 so libwebp/yuv420p accepts the frame.
func evenSize(w, h int) (int, int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w%2 != 0 {
		w++
	}
	if h%2 != 0 {
		h++
	}
	return w, h
}
