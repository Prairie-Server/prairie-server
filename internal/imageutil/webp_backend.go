package imageutil

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

// WebP backend names for metadata.webp_encoder.
const (
	WebPBackendAuto   = "auto"
	WebPBackendFFmpeg = "ffmpeg"
	WebPBackendWASM   = "wasm"
)

// WebPEncoder produces WebP variants (decode + resize + encode) from source bytes.
type WebPEncoder interface {
	Name() string
	GenerateVariants(ctx context.Context, data []byte, widths []int) (*VariantResult, error)
	GenerateSquareVariants(ctx context.Context, data []byte, sizes []int) (*VariantResult, error)
}

// WebPEncoderConfig selects and configures the WebP still-image backend.
type WebPEncoderConfig struct {
	// Backend is auto|ffmpeg|wasm. Empty means auto.
	Backend string
	// FFmpegPath is the ffmpeg binary used by the ffmpeg backend.
	// Empty means "ffmpeg" on PATH (same binary as AVIF SVT — needs libwebp).
	FFmpegPath string
	// Quality is 1..=100 (default 90).
	Quality int
}

var (
	webpEncoderMu   sync.RWMutex
	webpEncoder     WebPEncoder = wasmWebPEncoder{}
	webpEncoderName             = WebPBackendWASM
)

// ConfigureWebPEncoder probes the host and installs the selected backend.
// Safe to call at startup and on config hot-reload.
func ConfigureWebPEncoder(cfg WebPEncoderConfig) (string, error) {
	cfg = normalizeWebPEncoderConfig(cfg)
	chosen, enc, err := selectWebPEncoder(cfg)
	if err != nil {
		return "", err
	}
	webpEncoderMu.Lock()
	webpEncoder = enc
	webpEncoderName = chosen
	webpEncoderMu.Unlock()
	slog.Info("imageutil: WebP encoder configured",
		"component", "imageutil",
		"backend", chosen,
		"ffmpeg", cfg.FFmpegPath,
		"quality", cfg.Quality,
	)
	return chosen, nil
}

// ActiveWebPBackend returns the currently configured WebP backend name.
func ActiveWebPBackend() string {
	webpEncoderMu.RLock()
	defer webpEncoderMu.RUnlock()
	return webpEncoderName
}

func currentWebPEncoder() WebPEncoder {
	webpEncoderMu.RLock()
	defer webpEncoderMu.RUnlock()
	if webpEncoder == nil {
		return wasmWebPEncoder{}
	}
	return webpEncoder
}

func normalizeWebPEncoderConfig(cfg WebPEncoderConfig) WebPEncoderConfig {
	cfg.Backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	if cfg.Backend == "" {
		cfg.Backend = WebPBackendAuto
	}
	if strings.TrimSpace(cfg.FFmpegPath) == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.Quality <= 0 {
		cfg.Quality = webpQuality
	}
	if cfg.Quality > 100 {
		cfg.Quality = 100
	}
	return cfg
}

func selectWebPEncoder(cfg WebPEncoderConfig) (string, WebPEncoder, error) {
	switch cfg.Backend {
	case WebPBackendWASM:
		return WebPBackendWASM, wasmWebPEncoder{}, nil
	case WebPBackendFFmpeg:
		if err := requireWebP(cfg.FFmpegPath); err != nil {
			return "", nil, err
		}
		return WebPBackendFFmpeg, newFFmpegWebPEncoder(cfg), nil
	default: // auto
		if err := requireWebP(cfg.FFmpegPath); err == nil {
			return WebPBackendFFmpeg, newFFmpegWebPEncoder(cfg), nil
		}
		slog.Warn("imageutil: ffmpeg/libwebp unavailable; using legacy WASM WebP",
			"component", "imageutil", "ffmpeg", cfg.FFmpegPath)
		return WebPBackendWASM, wasmWebPEncoder{}, nil
	}
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

// atomic flag used by tests to force WASM WebP.
var forceWebPWASMForTest atomic.Bool
