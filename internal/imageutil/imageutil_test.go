package imageutil

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strconv"
	"testing"

	"golang.org/x/image/webp"
)

// fastFormats skips AVIF encode. Most tests only need WebP; AVIF is covered
// by TestPublicVariantAPIs so CI stays cheap.
const fastFormats = "webp"

func TestGenerateVariants(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	result := mustGenerateVariants(t, src, []int{300, 200}, fastFormats)
	if result.Ext != ".webp" {
		t.Fatalf("ext = %q, want .webp", result.Ext)
	}
	keys := map[string][]byte{}
	for _, v := range result.Variants {
		keys[v.Key] = v.Data
		if len(v.Data) == 0 {
			t.Fatalf("variant %s empty", v.Key)
		}
		cfg, err := webp.DecodeConfig(bytes.NewReader(v.Data))
		if err != nil {
			t.Fatalf("decode %s: %v", v.Key, err)
		}
		switch v.Key {
		case "original":
			if cfg.Width != 400 || cfg.Height != 300 {
				t.Fatalf("original size = %dx%d, want 400x300", cfg.Width, cfg.Height)
			}
		case "w300":
			if cfg.Width != 300 {
				t.Fatalf("w300 width = %d, want 300", cfg.Width)
			}
		case "w200":
			if cfg.Width != 200 {
				t.Fatalf("w200 width = %d, want 200", cfg.Width)
			}
		}
	}
	for _, key := range []string{"original", "w300", "w200"} {
		if keys[key] == nil {
			t.Fatalf("missing variant %s", key)
		}
	}
	for _, v := range result.Variants {
		if len(v.AVIF) != 0 {
			t.Fatalf("variant %s unexpectedly included AVIF", v.Key)
		}
		if len(v.PNG) != 0 {
			t.Fatalf("variant %s unexpectedly included PNG", v.Key)
		}
	}
}

func TestPublicVariantAPIs(t *testing.T) {
	t.Parallel()
	// Exercises the public WebP and AVIF entry points on a small canvas so CI
	// time stays modest. WebP uses libvips/bimg; AVIF uses the configured backend.
	src := makeTestJPEG(t, 160, 120)

	result, err := GenerateWebPVariants(src, []int{80})
	if err != nil {
		t.Fatalf("GenerateWebPVariants: %v", err)
	}
	keys := map[string]bool{}
	for _, v := range result.Variants {
		keys[v.Key] = true
		if len(v.Data) == 0 {
			t.Fatalf("variant %s missing WebP payload", v.Key)
		}
		if len(v.AVIF) != 0 {
			t.Fatalf("variant %s unexpectedly included AVIF inline", v.Key)
		}
		if len(v.PNG) != 0 {
			t.Fatalf("variant %s unexpectedly included PNG", v.Key)
		}
	}
	for _, key := range []string{"original", "w80"} {
		if !keys[key] {
			t.Fatalf("missing variant %s", key)
		}
	}

	siblings, err := GenerateAVIFSiblings(src, []int{80})
	if err != nil {
		t.Fatalf("GenerateAVIFSiblings: %v", err)
	}
	for _, v := range siblings.Variants {
		switch v.Key {
		case "original":
			if len(v.AVIF) != 0 {
				t.Fatalf("GenerateAVIFSiblings must skip original AVIF, got %d bytes", len(v.AVIF))
			}
			if len(v.Data) == 0 {
				t.Fatal("GenerateAVIFSiblings original WebP missing")
			}
		case "w80":
			if len(v.AVIF) < 12 || string(v.AVIF[4:8]) != "ftyp" {
				t.Fatalf("display variant %s missing AVIF payload", v.Key)
			}
		}
	}

	square, err := GenerateSquareVariants(src, []int{64})
	if err != nil {
		t.Fatalf("GenerateSquareVariants: %v", err)
	}
	found := false
	for _, v := range square.Variants {
		cfg, err := webp.DecodeConfig(bytes.NewReader(v.Data))
		if err != nil {
			t.Fatalf("decode %s: %v", v.Key, err)
		}
		if cfg.Width != cfg.Height {
			t.Fatalf("%s not square: %dx%d", v.Key, cfg.Width, cfg.Height)
		}
		if len(v.AVIF) < 12 || string(v.AVIF[4:8]) != "ftyp" {
			t.Fatalf("square variant %s missing AVIF payload", v.Key)
		}
		if v.Key == "w64" {
			found = true
			if cfg.Width != 64 {
				t.Fatalf("w64 size = %d, want 64", cfg.Width)
			}
		}
	}
	if !found {
		t.Fatal("missing w64")
	}
}

func TestGenerateVariantsCapsOriginal(t *testing.T) {
	t.Parallel()
	// Just over the 1920 cap on the long edge; keep pixels modest for CI time.
	src := makeTestJPEG(t, 2000, 400)
	result := mustGenerateVariants(t, src, nil, fastFormats)
	original := result.Variants[0].Data
	cfg, err := webp.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	if cfg.Width > maxCachedOriginalDimension {
		t.Fatalf("original width = %d, want <= %d", cfg.Width, maxCachedOriginalDimension)
	}
}

func TestGenerateSquareVariants(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 400, 300)
	result := mustGenerateSquareVariants(t, src, []int{128}, fastFormats)
	found := false
	for _, v := range result.Variants {
		cfg, err := webp.DecodeConfig(bytes.NewReader(v.Data))
		if err != nil {
			t.Fatalf("decode %s: %v", v.Key, err)
		}
		if cfg.Width != cfg.Height {
			t.Fatalf("%s not square: %dx%d", v.Key, cfg.Width, cfg.Height)
		}
		if len(v.PNG) != 0 {
			t.Fatalf("variant %s unexpectedly included PNG", v.Key)
		}
		if v.Key == "w128" {
			found = true
			if cfg.Width != 128 {
				t.Fatalf("w128 size = %d, want 128", cfg.Width)
			}
		}
	}
	if !found {
		t.Fatal("missing w128")
	}
}

func TestThumbhash(t *testing.T) {
	t.Parallel()
	src := makeTestJPEG(t, 120, 80)
	hash, err := Thumbhash(src)
	if err != nil {
		t.Fatalf("Thumbhash: %v", err)
	}
	if hash == "" {
		t.Fatal("empty thumbhash")
	}

	// WebP-only sources need the normalize-png WASM path.
	webpSrc := mustGenerateVariants(t, src, nil, "webp")
	hash2, err := Thumbhash(webpSrc.Variants[0].Data)
	if err != nil {
		t.Fatalf("Thumbhash(webp): %v", err)
	}
	if hash2 == "" {
		t.Fatal("empty thumbhash from webp")
	}
}

func TestNormalizeRejectsGarbage(t *testing.T) {
	t.Parallel()
	_, err := GenerateVariants([]byte("not-an-image"), []int{100})
	if err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestGenerateVariantsFromSVG(t *testing.T) {
	t.Parallel()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="200"><rect width="400" height="200" fill="#234"/></svg>`)
	result := mustGenerateVariants(t, svg, []int{200}, fastFormats)
	cfg, err := webp.DecodeConfig(bytes.NewReader(result.Variants[0].Data))
	if err != nil {
		t.Fatalf("decode svg original: %v", err)
	}
	if cfg.Width != 400 || cfg.Height != 200 {
		t.Fatalf("svg original size = %dx%d, want 400x200", cfg.Width, cfg.Height)
	}
}

func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func TestPNGRoundTripNormalize(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 128})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	hash, err := Thumbhash(buf.Bytes())
	if err != nil {
		t.Fatalf("Thumbhash png: %v", err)
	}
	if hash == "" {
		t.Fatal("empty thumbhash")
	}
}

func mustRunVariants(t *testing.T, op, flagName string, data []byte, values []int, formats string, extraArgs ...string) *VariantResult {
	t.Helper()
	p, err := getProcessor()
	if err != nil {
		t.Fatalf("getProcessor: %v", err)
	}
	args := append([]string{
		"--quality", strconv.Itoa(webpQuality),
		"--avif-speed", strconv.Itoa(avifSpeed),
	}, extraArgs...)
	args = append(args, "--formats", formats)
	if csv := joinUintCSV(values); csv != "" {
		args = append(args, "--"+flagName, csv)
	}
	result, err := p.run(context.Background(), op, data, args)
	if err != nil {
		t.Fatalf("%s(%s): %v", op, formats, err)
	}
	return result
}

func mustGenerateVariants(t *testing.T, data []byte, widths []int, formats string) *VariantResult {
	t.Helper()
	return mustRunVariants(t, "variants", "widths", data, widths, formats,
		"--max-original", strconv.Itoa(maxCachedOriginalDimension))
}

func mustGenerateSquareVariants(t *testing.T, data []byte, sizes []int, formats string) *VariantResult {
	t.Helper()
	return mustRunVariants(t, "square-variants", "sizes", data, sizes, formats)
}

func TestGenerateVariantsWrapsErrInvalidImage(t *testing.T) {
	_, err := GenerateVariants([]byte("not an image"), []int{300})
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("err = %v, want ErrInvalidImage", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 99, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestPixelDataUndecodable(t *testing.T) {
	fullPNG := testPNG(t, 600, 900)
	fullJPEG := largeTestJPEG(t, 600, 900)
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, image.NewPaletted(image.Rect(0, 0, 600, 900), palette.Plan9), nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	fullGIF := gifBuf.Bytes()

	for name, tc := range map[string]struct {
		data []byte
		want bool
	}{
		"valid png":      {fullPNG, false},
		"valid jpeg":     {fullJPEG, false},
		"truncated png":  {fullPNG[:len(fullPNG)/2], true},
		"truncated jpeg": {fullJPEG[:len(fullJPEG)/2], true},
		"truncated gif":  {fullGIF[:len(fullGIF)/2], false},
		"unknown":        {[]byte("not an image"), false},
		"huge header":    {withPNGDimensions(t, fullPNG[:len(fullPNG)/2], 100_000, 100_000), false},
		"just over cap":  {withPNGDimensions(t, fullPNG[:len(fullPNG)/2], 2_001, 2_000), false},
		"one tall row":   {withPNGDimensions(t, fullPNG[:len(fullPNG)/2], 1, maxPixelCheckPixels+1), false},
	} {
		if got := PixelDataUndecodable(tc.data); got != tc.want {
			t.Errorf("%s: PixelDataUndecodable = %v, want %v", name, got, tc.want)
		}
	}
}

// withPNGDimensions rewrites the IHDR width and height of a PNG, keeping the
// chunk CRC valid, so a small file declares an arbitrarily large raster.
func withPNGDimensions(t *testing.T, data []byte, width, height uint32) []byte {
	t.Helper()
	out := bytes.Clone(data)
	// Signature (8) + length (4) + "IHDR" (4), then width, height.
	const ihdr = 8 + 4
	if string(out[ihdr:ihdr+4]) != "IHDR" {
		t.Fatal("IHDR is not the first chunk")
	}
	binary.BigEndian.PutUint32(out[ihdr+4:], width)
	binary.BigEndian.PutUint32(out[ihdr+8:], height)
	binary.BigEndian.PutUint32(out[ihdr+4+13:], crc32.ChecksumIEEE(out[ihdr:ihdr+4+13]))
	return out
}
