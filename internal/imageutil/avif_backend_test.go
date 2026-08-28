package imageutil

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Prefer native ffmpeg backends when present so CI exercises the production
	// default path. Falls back to WASM automatically.
	_, _ = ConfigureAVIFEncoder(EncoderConfig{Backend: BackendAuto})
	_, _ = ConfigureWebPEncoder(WebPEncoderConfig{Backend: WebPBackendAuto})
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

func TestSVTEncodeLandscapeLogoAndSmallSource(t *testing.T) {
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

	// Wide/short logo: after w500 resize height stays well under a square canvas.
	// The old pad=max(iw,64):max(ih,64) path failed with "Padded dimensions
	// cannot be smaller than input" on some landscape frames.
	logo := makeTestJPEG(t, 1200, 200)
	logoResult, err := GenerateAVIFSiblings(logo, []int{500})
	if err != nil {
		t.Fatalf("GenerateAVIFSiblings(logo): %v", err)
	}
	assertVariantAVIF(t, logoResult, "w500")

	// Small source narrower than the ladder width: WASM re-encodes without
	// upscaling (e.g. 80×40 stays 80×40 for w300). Must still produce AVIF.
	small := makeTestJPEG(t, 80, 40)
	smallResult, err := GenerateAVIFSiblings(small, []int{300, 80})
	if err != nil {
		t.Fatalf("GenerateAVIFSiblings(small): %v", err)
	}
	assertVariantAVIF(t, smallResult, "w300")
	assertVariantAVIF(t, smallResult, "w80")

	// Odd landscape dims exercise ≤1px even-pad only.
	odd := makeTestJPEG(t, 501, 81)
	oddResult, err := GenerateAVIFSiblings(odd, []int{500})
	if err != nil {
		t.Fatalf("GenerateAVIFSiblings(odd): %v", err)
	}
	assertVariantAVIF(t, oddResult, "w500")
}

func assertVariantAVIF(t *testing.T, result *VariantResult, key string) {
	t.Helper()
	for _, v := range result.Variants {
		if v.Key != key {
			continue
		}
		if len(v.AVIF) < 12 || string(v.AVIF[4:8]) != "ftyp" {
			t.Fatalf("variant %s missing AVIF payload (%d bytes)", key, len(v.AVIF))
		}
		return
	}
	t.Fatalf("missing variant %s", key)
}

func TestSVTOutputSize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		w, h, wantW, wantH int
	}{
		{500, 80, 500, 80}, // landscape already safe
		{500, 55, 582, 64}, // short edge upscaled to 64, aspect kept
		{40, 30, 86, 64},   // both below floor
		{501, 80, 502, 80}, // odd width → +1 pad
		{500, 81, 500, 82}, // odd height → +1 pad
		{64, 64, 64, 64},   // exact floor
		{63, 100, 64, 102}, // width floor + even height
	}
	for _, tc := range cases {
		gotW, gotH := svtOutputSize(tc.w, tc.h)
		if gotW != tc.wantW || gotH != tc.wantH {
			t.Fatalf("svtOutputSize(%d,%d) = %dx%d, want %dx%d", tc.w, tc.h, gotW, gotH, tc.wantW, tc.wantH)
		}
		if gotW < svtMinEdge || gotH < svtMinEdge {
			t.Fatalf("svtOutputSize(%d,%d) below floor: %dx%d", tc.w, tc.h, gotW, gotH)
		}
		if gotW%2 != 0 || gotH%2 != 0 {
			t.Fatalf("svtOutputSize(%d,%d) not even: %dx%d", tc.w, tc.h, gotW, gotH)
		}
		if gotW < tc.w || gotH < tc.h {
			t.Fatalf("svtOutputSize(%d,%d) shrank to %dx%d", tc.w, tc.h, gotW, gotH)
		}
	}
}

func TestSVTPrepareFilterNeverPadsDown(t *testing.T) {
	t.Parallel()
	// Landscape JPEG already ≥64 and even → no filter (identity).
	wide := makeTestJPEG(t, 500, 80)
	if vf := svtPrepareFilter(wide); vf != "" {
		t.Fatalf("wide safe frame filter = %q, want empty", vf)
	}
	// Odd width → pad only (+1), never a fixed canvas smaller than input.
	odd := makeTestJPEG(t, 501, 80)
	vf := svtPrepareFilter(odd)
	if vf != "pad=502:80:0:0:color=black" {
		t.Fatalf("odd filter = %q, want pad=502:80:0:0:color=black", vf)
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
