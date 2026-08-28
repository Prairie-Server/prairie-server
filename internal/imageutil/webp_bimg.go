package imageutil

import (
	"fmt"
	"log/slog"
)

// WebP backend names for metadata.webp_encoder. Production WebP encode uses
// libvips via bimg (silo path); these constants remain for admin settings compatibility.
const (
	WebPBackendAuto   = "auto"
	WebPBackendFFmpeg = "ffmpeg"
	WebPBackendWASM   = "wasm"
)

// WebPEncoderConfig selects the WebP still-image backend. The silo-synced server
// always encodes WebP through libvips/bimg regardless of this setting.
type WebPEncoderConfig struct {
	Backend    string
	FFmpegPath string
	Quality    int
}

// ConfigureWebPEncoder is a no-op compatibility hook: WebP generation is handled
// by libvips/bimg in GenerateVariants/GenerateWebPVariants.
func ConfigureWebPEncoder(cfg WebPEncoderConfig) (string, error) {
	_ = cfg
	slog.Info("imageutil: WebP encoder configured",
		"component", "imageutil",
		"backend", "libvips",
	)
	return "libvips", nil
}

// ActiveWebPBackend reports the effective WebP backend name.
func ActiveWebPBackend() string {
	return "libvips"
}

func requireWebP(ffmpegPath string) error {
	if !ffmpegHasEncoder(ffmpegPath, "libwebp") {
		return fmt.Errorf("ffmpeg %q missing libwebp (install ffmpeg with libwebp)", ffmpegPath)
	}
	if !ffmpegHasMuxer(ffmpegPath, "webp") {
		return fmt.Errorf("ffmpeg %q missing webp muxer", ffmpegPath)
	}
	return nil
}
