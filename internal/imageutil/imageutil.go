// Package imageutil provides image resizing and thumbhash generation
// for collection poster and backdrop uploads.
//
// Resizing and WebP/PNG encoding run in-process via an embedded Rust WASI
// module (tools/imageutil-wasm) executed by wazero. That keeps the Go build
// CGO-free while sandboxing untrusted image decode.
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
// quality loss.
func GenerateVariants(data []byte, widths []int) (*VariantResult, error) {
	return generateVariants(data, widths, defaultFormats)
}

// GenerateWebPVariants is the fast phase of artwork caching: WebP only, no
// AVIF. Callers that need AVIF coverage schedule GenerateAVIFSiblings afterward.
func GenerateWebPVariants(data []byte, widths []int) (*VariantResult, error) {
	return generateVariants(data, widths, "webp")
}

// GenerateAVIFSiblings re-encodes the same width ladder as WebP+AVIF so callers
// can upload only the AVIF payloads after a WebP-first publish. Canonical WebP
// bytes are regenerated (discarded by callers that already uploaded them);
// AVIF dominates cost so the extra WebP pass is cheap by comparison.
func GenerateAVIFSiblings(data []byte, widths []int) (*VariantResult, error) {
	return generateVariants(data, widths, "webp,avif")
}

func generateVariants(data []byte, widths []int, formats string) (*VariantResult, error) {
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
	return p.run(context.Background(), "variants", data, args)
}

// GenerateSquareVariants center-crops the source image to a square and returns
// a square original plus resized square variants, encoded as WebP with AVIF
// siblings.
func GenerateSquareVariants(data []byte, sizes []int) (*VariantResult, error) {
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
