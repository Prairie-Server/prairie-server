package imageutil

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

func TestGenerateVariants(t *testing.T) {
	src := makeTestJPEG(t, 800, 600)
	result, err := GenerateVariants(src, []int{500, 300})
	if err != nil {
		t.Fatalf("GenerateVariants: %v", err)
	}
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
			if cfg.Width != 800 || cfg.Height != 600 {
				t.Fatalf("original size = %dx%d, want 800x600", cfg.Width, cfg.Height)
			}
		case "w500":
			if cfg.Width != 500 {
				t.Fatalf("w500 width = %d, want 500", cfg.Width)
			}
		case "w300":
			if cfg.Width != 300 {
				t.Fatalf("w300 width = %d, want 300", cfg.Width)
			}
		}
	}
	for _, key := range []string{"original", "w500", "w300"} {
		if keys[key] == nil {
			t.Fatalf("missing variant %s", key)
		}
	}
	for _, v := range result.Variants {
		if len(v.AVIF) < 12 || string(v.AVIF[4:8]) != "ftyp" {
			t.Fatalf("variant %s missing AVIF payload", v.Key)
		}
		if len(v.PNG) < 8 || string(v.PNG[:4]) != "\x89PNG" {
			t.Fatalf("variant %s missing PNG payload", v.Key)
		}
	}
}

func TestGenerateVariantsCapsOriginal(t *testing.T) {
	// Just over the 1920 cap on the long edge; keep pixels modest for CI time.
	src := makeTestJPEG(t, 2000, 800)
	result, err := GenerateVariants(src, nil)
	if err != nil {
		t.Fatalf("GenerateVariants: %v", err)
	}
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
	src := makeTestJPEG(t, 800, 600)
	result, err := GenerateSquareVariants(src, []int{256})
	if err != nil {
		t.Fatalf("GenerateSquareVariants: %v", err)
	}
	found := false
	for _, v := range result.Variants {
		cfg, err := webp.DecodeConfig(bytes.NewReader(v.Data))
		if err != nil {
			t.Fatalf("decode %s: %v", v.Key, err)
		}
		if cfg.Width != cfg.Height {
			t.Fatalf("%s not square: %dx%d", v.Key, cfg.Width, cfg.Height)
		}
		if len(v.AVIF) < 12 || string(v.AVIF[4:8]) != "ftyp" {
			t.Fatalf("variant %s missing AVIF payload", v.Key)
		}
		if len(v.PNG) < 8 || string(v.PNG[:4]) != "\x89PNG" {
			t.Fatalf("variant %s missing PNG payload", v.Key)
		}
		if v.Key == "w256" {
			found = true
			if cfg.Width != 256 {
				t.Fatalf("w256 size = %d, want 256", cfg.Width)
			}
		}
	}
	if !found {
		t.Fatal("missing w256")
	}
}

func TestThumbhash(t *testing.T) {
	src := makeTestJPEG(t, 120, 80)
	hash, err := Thumbhash(src)
	if err != nil {
		t.Fatalf("Thumbhash: %v", err)
	}
	if hash == "" {
		t.Fatal("empty thumbhash")
	}

	// WebP-only sources need the normalize-png WASM path.
	webpSrc, err := GenerateVariants(src, nil)
	if err != nil {
		t.Fatalf("GenerateVariants for webp source: %v", err)
	}
	hash2, err := Thumbhash(webpSrc.Variants[0].Data)
	if err != nil {
		t.Fatalf("Thumbhash(webp): %v", err)
	}
	if hash2 == "" {
		t.Fatal("empty thumbhash from webp")
	}
}

func TestNormalizeRejectsGarbage(t *testing.T) {
	_, err := GenerateVariants([]byte("not-an-image"), []int{100})
	if err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestGenerateVariantsFromSVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="200"><rect width="400" height="200" fill="#234"/></svg>`)
	result, err := GenerateVariants(svg, []int{200})
	if err != nil {
		t.Fatalf("GenerateVariants(svg): %v", err)
	}
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
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func TestPNGRoundTripNormalize(t *testing.T) {
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
