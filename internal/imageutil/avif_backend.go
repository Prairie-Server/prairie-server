package imageutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Backend names for metadata.avif_encoder.
const (
	BackendAuto  = "auto"
	BackendSVT   = "svt"
	BackendNVENC = "nvenc"
	BackendWASM  = "wasm"
)

// AVIFEncoder encodes a single still image (already sized) to AVIF bytes.
type AVIFEncoder interface {
	Name() string
	Encode(ctx context.Context, imageBytes []byte, widthHint int) ([]byte, error)
}

// EncoderConfig selects and configures the AVIF still-image backend.
type EncoderConfig struct {
	// Backend is auto|svt|nvenc|wasm. Empty means auto.
	Backend string
	// FFmpegPath is the ffmpeg binary used by svt/nvenc backends.
	// Empty means "ffmpeg" on PATH (not jellyfin-ffmpeg — needs libsvtav1).
	FFmpegPath string
	// Quality is 1..=100 (matches prior rav1e/WASM quality; default 90).
	Quality int
	// Speed is 1..=10 (matches prior rav1e speed; 10 = fastest; default 10).
	Speed int
	// NVENCSessions caps concurrent NVENC encodes (consumer GPUs ~3). 0 → 3.
	NVENCSessions int
	// NVENCMinEdge skips GPU for images whose max edge is below this (tiny
	// 300–500px posters often lose to CPU after session overhead). 0 → 640.
	NVENCMinEdge int
	// EnableNVENC allows auto to pick NVENC when capable. Default true when
	// backend is auto/nvenc.
	EnableNVENC bool
}

var (
	avifEncoderMu   sync.RWMutex
	avifEncoder     AVIFEncoder = wasmAVIFEncoder{}
	avifEncoderName             = BackendWASM
)

// ConfigureAVIFEncoder probes the host and installs the selected backend.
// Safe to call at startup and on config hot-reload.
func ConfigureAVIFEncoder(cfg EncoderConfig) (string, error) {
	cfg = normalizeEncoderConfig(cfg)
	chosen, enc, err := selectAVIFEncoder(cfg)
	if err != nil {
		return "", err
	}
	avifEncoderMu.Lock()
	avifEncoder = enc
	avifEncoderName = chosen
	avifEncoderMu.Unlock()
	slog.Info("imageutil: AVIF encoder configured",
		"component", "imageutil",
		"backend", chosen,
		"ffmpeg", cfg.FFmpegPath,
		"quality", cfg.Quality,
		"speed", cfg.Speed,
	)
	return chosen, nil
}

// ActiveAVIFBackend returns the currently configured backend name.
func ActiveAVIFBackend() string {
	avifEncoderMu.RLock()
	defer avifEncoderMu.RUnlock()
	return avifEncoderName
}

func currentAVIFEncoder() AVIFEncoder {
	avifEncoderMu.RLock()
	defer avifEncoderMu.RUnlock()
	if avifEncoder == nil {
		return wasmAVIFEncoder{}
	}
	return avifEncoder
}

func normalizeEncoderConfig(cfg EncoderConfig) EncoderConfig {
	cfg.Backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	if cfg.Backend == "" {
		cfg.Backend = BackendAuto
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
	if cfg.Speed <= 0 {
		cfg.Speed = avifSpeed
	}
	if cfg.Speed > 10 {
		cfg.Speed = 10
	}
	if cfg.NVENCSessions <= 0 {
		cfg.NVENCSessions = 3
	}
	if cfg.NVENCMinEdge <= 0 {
		cfg.NVENCMinEdge = 640
	}
	switch cfg.Backend {
	case BackendAuto, BackendNVENC:
		cfg.EnableNVENC = true
	case BackendSVT, BackendWASM:
		cfg.EnableNVENC = false
	}
	return cfg
}

func selectAVIFEncoder(cfg EncoderConfig) (string, AVIFEncoder, error) {
	switch cfg.Backend {
	case BackendWASM:
		return BackendWASM, wasmAVIFEncoder{}, nil
	case BackendSVT:
		if err := requireSVT(cfg.FFmpegPath); err != nil {
			return "", nil, err
		}
		return BackendSVT, newSVTEncoder(cfg), nil
	case BackendNVENC:
		svt := newSVTEncoder(cfg)
		if err := requireSVT(cfg.FFmpegPath); err != nil {
			// NVENC-only without CPU fallback is rejected — always need SVT/WASM.
			if probeAV1NVENC(cfg.FFmpegPath) {
				slog.Warn("imageutil: NVENC available but SVT/libsvtav1 missing; falling back to WASM",
					"component", "imageutil")
				return BackendWASM, wasmAVIFEncoder{}, nil
			}
			return "", nil, fmt.Errorf("avif nvenc: %w", err)
		}
		if !probeAV1NVENC(cfg.FFmpegPath) {
			slog.Warn("imageutil: av1_nvenc unavailable; using SVT-AV1",
				"component", "imageutil", "ffmpeg", cfg.FFmpegPath)
			return BackendSVT, svt, nil
		}
		return BackendNVENC, newNVENCEncoder(cfg, svt), nil
	default: // auto
		svtOK := requireSVT(cfg.FFmpegPath) == nil
		if cfg.EnableNVENC && svtOK && probeAV1NVENC(cfg.FFmpegPath) {
			return BackendNVENC, newNVENCEncoder(cfg, newSVTEncoder(cfg)), nil
		}
		if svtOK {
			return BackendSVT, newSVTEncoder(cfg), nil
		}
		slog.Warn("imageutil: libsvtav1/ffmpeg unavailable; using legacy WASM rav1e",
			"component", "imageutil", "ffmpeg", cfg.FFmpegPath)
		return BackendWASM, wasmAVIFEncoder{}, nil
	}
}

func requireSVT(ffmpegPath string) error {
	if !ffmpegHasEncoder(ffmpegPath, "libsvtav1") {
		return fmt.Errorf("ffmpeg %q missing libsvtav1 (install ffmpeg with SVT-AV1)", ffmpegPath)
	}
	if !ffmpegHasMuxer(ffmpegPath, "avif") {
		return fmt.Errorf("ffmpeg %q missing avif muxer", ffmpegPath)
	}
	return nil
}

var (
	ffmpegEncoderCacheMu sync.Mutex
	ffmpegEncoderCache   = map[string]map[string]bool{}
	ffmpegMuxerCache     = map[string]map[string]bool{}
	av1NVENCProbeCache   = map[string]bool{}
)

// The caches below hold the *complete* capability set for an ffmpeg binary, not
// the answer to one question about it.
//
// They used to be filled with only the name the first caller asked about, while
// still being keyed on the ffmpeg path alone. Every later question about the same
// binary then hit that cache and was answered "no" from a map that had never
// looked. In production the AVIF backend configures first and asks for the
// "avif" muxer, so the WebP backend's "webp" muxer check one millisecond later
// read {avif: true} and fell back to the WASM encoder — on a machine whose
// ffmpeg lists both libwebp and the webp muxer. Every WebP encode in the process
// took the slow path.

func ffmpegHasEncoder(ffmpegPath, name string) bool {
	ffmpegEncoderCacheMu.Lock()
	defer ffmpegEncoderCacheMu.Unlock()
	m, ok := ffmpegEncoderCache[ffmpegPath]
	if !ok {
		out, err := exec.Command(ffmpegPath, "-hide_banner", "-encoders").CombinedOutput()
		if err != nil {
			out = nil
		}
		m = parseFFmpegCapabilities(out)
		ffmpegEncoderCache[ffmpegPath] = m
	}
	return m[name]
}

func ffmpegHasMuxer(ffmpegPath, name string) bool {
	ffmpegEncoderCacheMu.Lock()
	defer ffmpegEncoderCacheMu.Unlock()
	m, ok := ffmpegMuxerCache[ffmpegPath]
	if !ok {
		out, err := exec.Command(ffmpegPath, "-hide_banner", "-muxers").CombinedOutput()
		if err != nil {
			out = nil
		}
		m = parseFFmpegCapabilities(out)
		ffmpegMuxerCache[ffmpegPath] = m
	}
	return m[name]
}

// parseFFmpegCapabilities reads every name from an ffmpeg "-encoders" or
// "-muxers" listing. Both share a layout of "<flags> <name> <description>",
// where a name may be a comma-separated group ("matroska,webm").
//
// Lines before the dashed separator are the flag legend (" .E = Muxing
// supported") and are skipped: their second field is "=", not a codec name. The
// separator's width differs per listing — "--" for muxers, "------" for encoders
// — so it is matched as "a lone run of dashes" rather than a literal.
func parseFFmpegCapabilities(out []byte) map[string]bool {
	names := map[string]bool{}
	if len(out) == 0 {
		return names
	}
	body := false
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if !body {
			if len(fields) == 1 && isDashRun(fields[0]) {
				body = true
			}
			continue
		}
		if len(fields) < 2 || fields[1] == "=" {
			continue
		}
		for _, name := range strings.Split(fields[1], ",") {
			if name != "" {
				names[name] = true
			}
		}
	}
	return names
}

// isDashRun reports whether s is one or more '-' and nothing else.
func isDashRun(s string) bool {
	if s == "" {
		return false
	}
	return strings.Trim(s, "-") == ""
}

// probeAV1NVENC returns true when ffmpeg lists av1_nvenc, an NVIDIA device is
// present, and a smoke help probe succeeds. Ada (SM ≥ 8.9) is required for AV1
// NVENC on consumer cards; Ampere (e.g. RTX 3050) is rejected via compute-cap.
func probeAV1NVENC(ffmpegPath string) bool {
	ffmpegEncoderCacheMu.Lock()
	if v, ok := av1NVENCProbeCache[ffmpegPath]; ok {
		ffmpegEncoderCacheMu.Unlock()
		return v
	}
	ffmpegEncoderCacheMu.Unlock()

	ok := false
	defer func() {
		ffmpegEncoderCacheMu.Lock()
		av1NVENCProbeCache[ffmpegPath] = ok
		ffmpegEncoderCacheMu.Unlock()
	}()

	if !hasNVIDIADevice() {
		return false
	}
	if !ffmpegHasEncoder(ffmpegPath, "av1_nvenc") {
		return false
	}
	// Help text lists the encoder options only when the driver/SDK can load it.
	cmd := exec.Command(ffmpegPath, "-hide_banner", "-h", "encoder=av1_nvenc")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	text := string(out)
	if strings.Contains(strings.ToLower(text), "unknown encoder") {
		return false
	}
	// Optional compute-capability gate via nvidia-smi.
	if cap, okCap := nvidiaComputeCapability(); okCap && cap < 8.9 {
		slog.Info("imageutil: GPU compute capability below Ada AV1 NVENC (need >= 8.9); skipping NVENC",
			"component", "imageutil", "compute_cap", cap)
		return false
	}
	ok = true
	return true
}

func hasNVIDIADevice() bool {
	if _, err := os.Stat("/dev/nvidiactl"); err == nil {
		return true
	}
	matches, err := filepath.Glob("/dev/nvidia[0-9]*")
	return err == nil && len(matches) > 0
}

func nvidiaComputeCapability() (float64, bool) {
	// nvidia-smi --query-gpu=compute_cap --format=csv,noheader
	cmd := exec.Command("nvidia-smi", "--query-gpu=compute_cap", "--format=csv,noheader")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, false
	}
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	var major, minor int
	if _, err := fmt.Sscanf(line, "%d.%d", &major, &minor); err != nil {
		return 0, false
	}
	return float64(major) + float64(minor)/10.0, true
}

// qualityToCRF maps 1..=100 still quality onto SVT/libaom CRF (lower = better).
func qualityToCRF(quality int) int {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	// quality 90 → CRF ~28; quality 100 → 18; quality 50 → ~45.
	crf := 18 + (100-quality)*37/100
	if crf < 18 {
		crf = 18
	}
	if crf > 55 {
		crf = 55
	}
	return crf
}

// speedToSVTPreset maps rav1e-style 1..=10 onto SVT preset (higher = faster).
func speedToSVTPreset(speed int) int {
	if speed < 1 {
		speed = 1
	}
	if speed > 10 {
		speed = 10
	}
	// Keep 10→10 (fastest). Slower rav1e speeds map toward SVT 4–8.
	return speed
}

// atomic flag used by tests to force WASM.
var forceWASMForTest atomic.Bool
