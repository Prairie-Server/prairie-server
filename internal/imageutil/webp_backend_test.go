package imageutil

import (
	"bytes"
	"testing"

	"golang.org/x/image/webp"
)

func TestConfigureWebPEncoderPrefersFFmpeg(t *testing.T) {
	name, err := ConfigureWebPEncoder(WebPEncoderConfig{Backend: WebPBackendAuto, FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatalf("ConfigureWebPEncoder: %v", err)
	}
	if requireWebP("ffmpeg") == nil && name != WebPBackendFFmpeg {
		t.Fatalf("backend = %q, want ffmpeg when libwebp is present", name)
	}
}

func TestFFmpegWebPEncodeSmoke(t *testing.T) {
	if requireWebP("ffmpeg") != nil {
		t.Skip("ffmpeg/libwebp unavailable")
	}
	_, err := ConfigureWebPEncoder(WebPEncoderConfig{Backend: WebPBackendFFmpeg, FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ConfigureWebPEncoder(WebPEncoderConfig{Backend: WebPBackendAuto, FFmpegPath: "ffmpeg"})
	})

	src := makeTestJPEG(t, 400, 300)
	result, err := GenerateWebPVariants(src, []int{300, 200})
	if err != nil {
		t.Fatalf("GenerateWebPVariants: %v", err)
	}
	if result.Ext != ".webp" {
		t.Fatalf("ext = %q", result.Ext)
	}
	keys := map[string]bool{}
	for _, v := range result.Variants {
		keys[v.Key] = true
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
				t.Fatalf("original = %dx%d", cfg.Width, cfg.Height)
			}
		case "w300":
			if cfg.Width != 300 {
				t.Fatalf("w300 width = %d", cfg.Width)
			}
		case "w200":
			if cfg.Width != 200 {
				t.Fatalf("w200 width = %d", cfg.Width)
			}
		}
	}
	for _, key := range []string{"original", "w300", "w200"} {
		if !keys[key] {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestFFmpegWebPNoUpscale(t *testing.T) {
	if requireWebP("ffmpeg") != nil {
		t.Skip("ffmpeg/libwebp unavailable")
	}
	_, err := ConfigureWebPEncoder(WebPEncoderConfig{Backend: WebPBackendFFmpeg, FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ConfigureWebPEncoder(WebPEncoderConfig{Backend: WebPBackendAuto, FFmpegPath: "ffmpeg"})
	})

	src := makeTestJPEG(t, 120, 80)
	result, err := GenerateWebPVariants(src, []int{500})
	if err != nil {
		t.Fatalf("GenerateWebPVariants: %v", err)
	}
	for _, v := range result.Variants {
		if v.Key != "w500" {
			continue
		}
		cfg, err := webp.DecodeConfig(bytes.NewReader(v.Data))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if cfg.Width != 120 {
			t.Fatalf("upscaled to width %d, want 120", cfg.Width)
		}
	}
}

func TestWebPWASMFallback(t *testing.T) {
	forceWebPWASMForTest.Store(true)
	t.Cleanup(func() { forceWebPWASMForTest.Store(false) })
	src := makeTestJPEG(t, 80, 60)
	result, err := GenerateWebPVariants(src, []int{40})
	if err != nil {
		t.Fatalf("GenerateWebPVariants(wasm): %v", err)
	}
	if len(result.Variants) < 2 {
		t.Fatalf("variants = %d", len(result.Variants))
	}
}
