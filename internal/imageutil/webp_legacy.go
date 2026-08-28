package imageutil

import (
	"context"
	"strconv"
)

// wasmWebPEncoder keeps the legacy zenwebp-in-WASM path for hosts without
// ffmpeg/libwebp and for explicit metadata.webp_encoder=wasm.
type wasmWebPEncoder struct{}

func (wasmWebPEncoder) Name() string { return WebPBackendWASM }

func (wasmWebPEncoder) GenerateVariants(ctx context.Context, data []byte, widths []int) (*VariantResult, error) {
	_ = ctx
	return generateVariantsWASM(data, widths, "webp")
}

func (wasmWebPEncoder) GenerateSquareVariants(ctx context.Context, data []byte, sizes []int) (*VariantResult, error) {
	_ = ctx
	p, err := getProcessor()
	if err != nil {
		return nil, err
	}
	args := []string{
		"--quality", strconv.Itoa(webpQuality),
		"--avif-speed", strconv.Itoa(avifSpeed),
		"--formats", "webp",
	}
	if csv := joinUintCSV(sizes); csv != "" {
		args = append(args, "--sizes", csv)
	}
	return p.run(ctx, "square-variants", data, args)
}

// generateVariantsWASM is the legacy WASI decode/resize/encode path.
func generateVariantsWASM(data []byte, widths []int, formats string, noAVIFKeys ...string) (*VariantResult, error) {
	return generateVariants(data, widths, formats, noAVIFKeys...)
}
