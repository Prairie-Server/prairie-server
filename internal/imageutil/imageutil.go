// Package imageutil provides image resizing and thumbhash generation
// for collection poster and backdrop uploads.
//
// WebP and AVIF still-image work prefer native ffmpeg backends (libwebp /
// libsvtav1, optional AV1 NVENC) with the embedded Rust WASI module as
// fallback. Trading hermetic vendored-WASM encode for native libs in the
// Docker image is intentional: WASM cannot match native SIMD/threads on
// small nodes. Hostile-byte decode stays in ffmpeg/WASI — not CGO libvips.
package imageutil

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	"go.n16f.net/thumbhash"
)

const (
	webpQuality = 90
	// avifSpeed is the rav1e speed preset passed to imageutil-wasm (1..=10).
	// 10 = fastest; photographic posters/backdrops stay visually identical to
	// slower presets while cutting WASM encode time multi-fold.
	avifSpeed = 10
	// defaultFormats keeps WebP canonical + AVIF sibling. PNG is omitted:
	// clients fall through AVIF→WebP on missing PNG (ArtworkImage onError /
	// native caches), and PNG was the largest encode of the trio.
	defaultFormats             = "webp,avif"
	maxCachedOriginalDimension = 1920
	thumbhashSourceDimension   = 100
)

// Variant holds a named image variant (e.g. "original", "w500").
// Data is the canonical WebP payload; AVIF and PNG are optional sibling
// upgrades for clients that prefer them (AVIF) or cannot decode WebP (PNG).
type Variant struct {
	Key  string
	Data []byte
	AVIF []byte // optional; empty when AVIF encode was skipped
	PNG  []byte // optional; empty when PNG encode was skipped
}

// VariantResult contains generated variants and their canonical output format.
type VariantResult struct {
	Variants []Variant
	Ext      string // canonical extension including dot: ".webp"
}

// GenerateVariants produces WebP (+ AVIF) variants of the source image at the
// requested widths, plus an "original" re-encoded in those formats. WebP
// remains the canonical cache key; AVIF is a dual-written sibling for clients
// that prefer it. Images narrower than a target width are re-encoded without
// upscaling. All resizes operate on the original bytes to avoid compounding
// quality loss. AVIF uses the configured native/WASM backend.
func GenerateVariants(data []byte, widths []int) (*VariantResult, error) {
	if ActiveAVIFBackend() == BackendWASM || forceWASMForTest.Load() {
		return generateVariants(data, widths, defaultFormats)
	}
	return encodeAVIFLadder(context.Background(), data, widths, nil)
}

// GenerateWebPVariants is the fast phase of artwork caching: WebP only, no
// AVIF. Callers that need AVIF coverage schedule GenerateAVIFSiblings afterward.
// Uses the configured native ffmpeg/libwebp backend when available, else WASM.
func GenerateWebPVariants(data []byte, widths []int) (*VariantResult, error) {
	if forceWebPWASMForTest.Load() {
		return generateVariants(data, widths, "webp")
	}
	return currentWebPEncoder().GenerateVariants(context.Background(), data, widths)
}

// GenerateAVIFSiblings resizes the display-width ladder (WebP via the configured
// backend) then encodes AVIF siblings with the configured AVIF backend.
// The "original" key keeps WebP only: full-size AVIF dominates encode cost and
// clients already fall through to WebP for it. Callers discard regenerated
// WebP and upload only the AVIF payloads.
func GenerateAVIFSiblings(data []byte, widths []int) (*VariantResult, error) {
	return encodeAVIFLadder(context.Background(), data, widths, []string{"original"})
}

func generateVariants(data []byte, widths []int, formats string, noAVIFKeys ...string) (*VariantResult, error) {
	p, err := getProcessor()
	if err != nil {
		return nil, err
	}
	args := []string{
		"--quality", strconv.Itoa(webpQuality),
		"--avif-speed", strconv.Itoa(avifSpeed),
		"--max-original", strconv.Itoa(maxCachedOriginalDimension),
		"--formats", formats,
	}
	if csv := joinUintCSV(widths); csv != "" {
		args = append(args, "--widths", csv)
	}
	if len(noAVIFKeys) > 0 {
		args = append(args, "--no-avif-keys", strings.Join(noAVIFKeys, ","))
	}
	return p.run(context.Background(), "variants", data, args)
}

// GenerateSquareVariants center-crops the source image to a square and returns
// a square original plus resized square variants, encoded as WebP with AVIF
// siblings.
func GenerateSquareVariants(data []byte, sizes []int) (*VariantResult, error) {
	if ActiveAVIFBackend() == BackendWASM || forceWASMForTest.Load() {
		p, err := getProcessor()
		if err != nil {
			return nil, err
		}
		args := []string{
			"--quality", strconv.Itoa(webpQuality),
			"--avif-speed", strconv.Itoa(avifSpeed),
			"--formats", defaultFormats,
		}
		if csv := joinUintCSV(sizes); csv != "" {
			args = append(args, "--sizes", csv)
		}
		return p.run(context.Background(), "square-variants", data, args)
	}

	var webp *VariantResult
	var err error
	if forceWebPWASMForTest.Load() || ActiveWebPBackend() == WebPBackendWASM {
		p, perr := getProcessor()
		if perr != nil {
			return nil, perr
		}
		args := []string{
			"--quality", strconv.Itoa(webpQuality),
			"--avif-speed", strconv.Itoa(avifSpeed),
			"--formats", "webp",
		}
		if csv := joinUintCSV(sizes); csv != "" {
			args = append(args, "--sizes", csv)
		}
		webp, err = p.run(context.Background(), "square-variants", data, args)
	} else {
		webp, err = currentWebPEncoder().GenerateSquareVariants(context.Background(), data, sizes)
	}
	if err != nil {
		return nil, err
	}
	enc := currentAVIFEncoder()
	for i := range webp.Variants {
		avif, encErr := enc.Encode(context.Background(), webp.Variants[i].Data, widthFromKey(webp.Variants[i].Key))
		if encErr != nil {
			return nil, fmt.Errorf("imageutil: square AVIF %s: %w", webp.Variants[i].Key, encErr)
		}
		webp.Variants[i].AVIF = avif
	}
	return webp, nil
}

// Thumbhash computes a base64-encoded thumbhash from raw image bytes.
// The image is scaled to max 100x100 before hashing.
func Thumbhash(data []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		normalized, normalizeErr := normalizeThumbhashSource(data)
		if normalizeErr != nil {
			return "", fmt.Errorf("imageutil: decode for thumbhash: %w", err)
		}
		img, _, err = image.Decode(bytes.NewReader(normalized))
		if err != nil {
			return "", fmt.Errorf("imageutil: decode normalized thumbhash source: %w", err)
		}
	}

	scaled := scaleImage(img, 100)
	hashBytes := thumbhash.EncodeImage(scaled)
	return base64.StdEncoding.EncodeToString(hashBytes), nil
}

func normalizeThumbhashSource(data []byte) ([]byte, error) {
	p, err := getProcessor()
	if err != nil {
		return nil, err
	}
	result, err := p.run(context.Background(), "normalize-png", data, []string{
		"--max-dim", strconv.Itoa(thumbhashSourceDimension),
	})
	if err != nil {
		return nil, err
	}
	for _, v := range result.Variants {
		if v.Key == "normalized" {
			return v.Data, nil
		}
	}
	if len(result.Variants) > 0 {
		return result.Variants[0].Data, nil
	}
	return nil, fmt.Errorf("imageutil: normalize thumbhash source: empty output")
}

// scaleImage scales src so its longest dimension does not exceed maxDim,
// preserving aspect ratio. Uses nearest-neighbor interpolation.
func scaleImage(src image.Image, maxDim int) *image.NRGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scale := 1.0
	if srcW > maxDim || srcH > maxDim {
		scaleW := float64(maxDim) / float64(srcW)
		scaleH := float64(maxDim) / float64(srcH)
		if scaleW < scaleH {
			scale = scaleW
		} else {
			scale = scaleH
		}
	}

	dstW := max(int(float64(srcW)*scale), 1)
	dstH := max(int(float64(srcH)*scale), 1)
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	for y := range dstH {
		srcY := min(int(float64(y)/scale), srcH-1)
		for x := range dstW {
			srcX := min(int(float64(x)/scale), srcW-1)
			r, g, b, a := src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY).RGBA()
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r >> 8), G: uint8(g >> 8),
				B: uint8(b >> 8), A: uint8(a >> 8),
			})
		}
	}
	return dst
}
