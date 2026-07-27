package imageutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// wasmAVIFEncoder keeps the legacy rav1e-in-WASM path for hosts without
// ffmpeg/libsvtav1 and for explicit metadata.avif_encoder=wasm.
type wasmAVIFEncoder struct{}

func (wasmAVIFEncoder) Name() string { return BackendWASM }

func (wasmAVIFEncoder) Encode(ctx context.Context, imageBytes []byte, _ int) ([]byte, error) {
	p, err := getProcessor()
	if err != nil {
		return nil, err
	}
	// Encode a single frame at the source dimensions (caller already resized).
	// formats=avif still forces a WebP canonical sibling inside the WASM helper;
	// we only return the AVIF payload. p.run acquires the shared encode budget.
	result, err := p.run(ctx, "variants", imageBytes, []string{
		"--quality", strconv.Itoa(webpQuality),
		"--avif-speed", strconv.Itoa(avifSpeed),
		"--max-original", strconv.Itoa(maxCachedOriginalDimension),
		"--formats", "avif",
	})
	if err != nil {
		return nil, err
	}
	for _, v := range result.Variants {
		if v.Key == "original" && len(v.AVIF) > 0 {
			return v.AVIF, nil
		}
	}
	for _, v := range result.Variants {
		if len(v.AVIF) > 0 {
			return v.AVIF, nil
		}
	}
	return nil, fmt.Errorf("wasm avif: no AVIF payload in manifest")
}

// encodeAVIFLadder resizes via the configured WebP backend then encodes
// display-width AVIFs with the configured AVIF backend. original is skipped
// when listed in noAVIFKeys.
func encodeAVIFLadder(ctx context.Context, data []byte, widths []int, noAVIFKeys []string) (*VariantResult, error) {
	skip := map[string]bool{}
	for _, k := range noAVIFKeys {
		k = strings.TrimSpace(strings.ToLower(k))
		if k != "" {
			skip[k] = true
		}
	}

	webp, err := GenerateWebPVariants(data, widths)
	if err != nil {
		return nil, err
	}

	enc := currentAVIFEncoder()
	out := &VariantResult{Ext: webp.Ext, Variants: make([]Variant, 0, len(webp.Variants))}
	for _, v := range webp.Variants {
		item := Variant{Key: v.Key, Data: v.Data}
		if skip[strings.ToLower(v.Key)] {
			out.Variants = append(out.Variants, item)
			continue
		}
		// Prefer encoding from the resized WebP bytes so native ffmpeg sees the
		// exact display dimensions (no second scale). WASM backend also accepts WebP.
		src := v.Data
		if len(src) == 0 {
			src = data
		}
		widthHint := widthFromKey(v.Key)
		avif, encErr := enc.Encode(ctx, src, widthHint)
		if encErr != nil {
			return nil, fmt.Errorf("imageutil: AVIF encode %s (%s): %w", v.Key, enc.Name(), encErr)
		}
		item.AVIF = avif
		out.Variants = append(out.Variants, item)
	}
	return out, nil
}

func widthFromKey(key string) int {
	key = strings.TrimSpace(strings.ToLower(key))
	if !strings.HasPrefix(key, "w") {
		return 0
	}
	n, err := strconv.Atoi(key[1:])
	if err != nil {
		return 0
	}
	return n
}
