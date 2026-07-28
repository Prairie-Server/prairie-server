package imageutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Trimmed but verbatim-shaped output from ffmpeg 5.1.9, including the flag
// legend and its separator. The separator width differs between the two
// listings, which is exactly what a literal "--" comparison got wrong.
const muxersFixture = `File formats:
 D. = Demuxing supported
 .E = Muxing supported
 --
  E 3g2             3GP2 (3GPP2 file format)
  E avif            AVIF
 DE matroska,webm   Matroska / WebM
  E webp            WebP
`

const encodersFixture = `Encoders:
 V..... = Video
 A..... = Audio
 .F.... = Frame-level multithreading
 .....D = Supports direct rendering method 1
 ------
 V....D libaom-av1           libaom AV1 (codec av1)
 V..... libsvtav1            SVT-AV1 encoder (codec av1)
 V....D libwebp_anim         libwebp WebP image (codec webp)
 V....D libwebp              libwebp WebP image (codec webp)
 A..... wmav1                Windows Media Audio 1
`

func TestParseFFmpegCapabilitiesReadsMuxers(t *testing.T) {
	got := parseFFmpegCapabilities([]byte(muxersFixture))
	for _, name := range []string{"3g2", "avif", "webp", "matroska", "webm"} {
		if !got[name] {
			t.Errorf("muxer %q not parsed", name)
		}
	}
	// Legend rows must not leak in as capability names.
	for _, name := range []string{"=", "--", "D.", ".E", "Demuxing", "Muxing"} {
		if got[name] {
			t.Errorf("legend token %q parsed as a muxer", name)
		}
	}
}

func TestParseFFmpegCapabilitiesReadsEncoders(t *testing.T) {
	got := parseFFmpegCapabilities([]byte(encodersFixture))
	for _, name := range []string{"libaom-av1", "libsvtav1", "libwebp", "libwebp_anim", "wmav1"} {
		if !got[name] {
			t.Errorf("encoder %q not parsed", name)
		}
	}
	if got["------"] || got["="] {
		t.Error("legend tokens parsed as encoders")
	}
	if got["Video"] || got["Audio"] {
		t.Error("legend descriptions parsed as encoders")
	}
}

func TestParseFFmpegCapabilitiesHandlesEmptyAndGarbage(t *testing.T) {
	if got := parseFFmpegCapabilities(nil); len(got) != 0 {
		t.Errorf("nil output = %v, want empty", got)
	}
	if got := parseFFmpegCapabilities([]byte("command not found")); len(got) != 0 {
		t.Errorf("garbage output = %v, want empty (no separator, so no body)", got)
	}
}

// fakeFFmpeg writes a script that answers -encoders and -muxers with the
// fixtures, so the cache can be exercised without a real ffmpeg.
func fakeFFmpeg(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script stub is POSIX-only")
	}
	path := filepath.Join(t.TempDir(), "fake-ffmpeg")
	script := "#!/bin/sh\nfor a in \"$@\"; do\n" +
		"  case \"$a\" in\n" +
		"    -encoders) cat <<'EOF'\n" + encodersFixture + "EOF\n    exit 0;;\n" +
		"    -muxers) cat <<'EOF'\n" + muxersFixture + "EOF\n    exit 0;;\n" +
		"  esac\ndone\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake ffmpeg: %v", err)
	}
	return path
}

func resetProbeCaches(t *testing.T) {
	t.Helper()
	ffmpegEncoderCacheMu.Lock()
	defer ffmpegEncoderCacheMu.Unlock()
	ffmpegEncoderCache = map[string]map[string]bool{}
	ffmpegMuxerCache = map[string]map[string]bool{}
}

// The regression: the first question asked about a binary used to decide the
// answer to every later question. AVIF configures before WebP in production, so
// asking for the "avif" muxer first must not make the "webp" muxer disappear.
func TestMuxerProbeIsNotPoisonedByAnEarlierQuestion(t *testing.T) {
	resetProbeCaches(t)
	t.Cleanup(func() { resetProbeCaches(t) })
	ffmpeg := fakeFFmpeg(t)

	if !ffmpegHasMuxer(ffmpeg, "avif") {
		t.Fatal("avif muxer not detected")
	}
	if !ffmpegHasMuxer(ffmpeg, "webp") {
		t.Fatal("webp muxer reported missing after avif was probed first — cache poisoned")
	}
	if ffmpegHasMuxer(ffmpeg, "definitely-not-a-muxer") {
		t.Error("unknown muxer reported present")
	}
}

func TestEncoderProbeIsNotPoisonedByAnEarlierQuestion(t *testing.T) {
	resetProbeCaches(t)
	t.Cleanup(func() { resetProbeCaches(t) })
	ffmpeg := fakeFFmpeg(t)

	if !ffmpegHasEncoder(ffmpeg, "libsvtav1") {
		t.Fatal("libsvtav1 not detected")
	}
	if !ffmpegHasEncoder(ffmpeg, "libwebp") {
		t.Fatal("libwebp reported missing after libsvtav1 was probed first — cache poisoned")
	}
	if ffmpegHasEncoder(ffmpeg, "definitely-not-an-encoder") {
		t.Error("unknown encoder reported present")
	}
}

// Both backends must be selectable from the same binary, in either order. This
// is the production symptom: WebP silently fell back to WASM.
func TestWebPAndAVIFBothSelectableFromOneFFmpeg(t *testing.T) {
	ffmpeg := fakeFFmpeg(t)

	for _, order := range []struct {
		name  string
		first func() error
		then  func() error
	}{
		{
			name:  "avif first",
			first: func() error { return requireSVT(ffmpeg) },
			then:  func() error { return requireWebP(ffmpeg) },
		},
		{
			name:  "webp first",
			first: func() error { return requireWebP(ffmpeg) },
			then:  func() error { return requireSVT(ffmpeg) },
		},
	} {
		t.Run(order.name, func(t *testing.T) {
			resetProbeCaches(t)
			t.Cleanup(func() { resetProbeCaches(t) })
			if err := order.first(); err != nil {
				t.Fatalf("first probe failed: %v", err)
			}
			if err := order.then(); err != nil {
				t.Fatalf("second probe failed (order-dependent capability detection): %v", err)
			}
		})
	}
}

// A missing binary must report no capabilities rather than caching a stale yes.
func TestProbesReportNothingForAMissingBinary(t *testing.T) {
	resetProbeCaches(t)
	t.Cleanup(func() { resetProbeCaches(t) })
	missing := filepath.Join(t.TempDir(), "no-such-ffmpeg")

	if ffmpegHasEncoder(missing, "libwebp") || ffmpegHasMuxer(missing, "webp") {
		t.Error("a missing ffmpeg reported capabilities")
	}
	if err := requireWebP(missing); err == nil {
		t.Error("requireWebP succeeded with no ffmpeg present")
	}
}
