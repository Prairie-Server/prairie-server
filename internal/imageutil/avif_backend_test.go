package imageutil

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Prefer native SVT when ffmpeg/libsvtav1 is present so CI exercises the
	// production default path. Falls back to WASM automatically.
	_, _ = ConfigureAVIFEncoder(EncoderConfig{Backend: BackendAuto})
	os.Exit(m.Run())
}

func TestConfigureAVIFEncoderPrefersSVT(t *testing.T) {
	name, err := ConfigureAVIFEncoder(EncoderConfig{Backend: BackendAuto, FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatalf("ConfigureAVIFEncoder: %v", err)
	}
	if requireSVT("ffmpeg") == nil && name != BackendSVT && name != BackendNVENC {
		t.Fatalf("backend = %q, want svt or nvenc when libsvtav1 is present", name)
	}
}

func TestSVTEncodeSmoke(t *testing.T) {
	if requireSVT("ffmpeg") != nil {
		t.Skip("libsvtav1/ffmpeg unavailable")
	}
	_, err := ConfigureAVIFEncoder(EncoderConfig{Backend: BackendSVT, FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ConfigureAVIFEncoder(EncoderConfig{Backend: BackendAuto, FFmpegPath: "ffmpeg"})
	})
	src := makeTestJPEG(t, 320, 480)
	result, err := GenerateAVIFSiblings(src, []int{300})
	if err != nil {
		t.Fatalf("GenerateAVIFSiblings: %v", err)
	}
	foundOriginal, foundW300 := false, false
	for _, v := range result.Variants {
		switch v.Key {
		case "original":
			foundOriginal = true
			if len(v.AVIF) != 0 {
				t.Fatalf("original must skip AVIF, got %d bytes", len(v.AVIF))
			}
		case "w300":
			foundW300 = true
			if len(v.AVIF) < 12 || string(v.AVIF[4:8]) != "ftyp" {
				t.Fatalf("w300 missing AVIF payload (%d bytes)", len(v.AVIF))
			}
		}
	}
	if !foundOriginal || !foundW300 {
		t.Fatalf("missing variants: original=%v w300=%v", foundOriginal, foundW300)
	}
}

func TestQualityToCRF(t *testing.T) {
	t.Parallel()
	if got := qualityToCRF(90); got < 20 || got > 40 {
		t.Fatalf("qualityToCRF(90) = %d, want mid-range", got)
	}
	if qualityToCRF(100) >= qualityToCRF(50) {
		t.Fatal("higher quality must map to lower CRF")
	}
}

func TestEncodeBudgetShared(t *testing.T) {
	prev := EncodeBudgetSize()
	t.Cleanup(func() { SetEncodeBudgetSize(prev) })
	SetEncodeBudgetSize(2)
	if got := EncodeBudgetSize(); got != 2 {
		t.Fatalf("EncodeBudgetSize = %d, want 2", got)
	}
	SetEncodeBudgetSize(0)
	if got := EncodeBudgetSize(); got < 1 {
		t.Fatalf("EncodeBudgetSize after reset = %d", got)
	}
}
