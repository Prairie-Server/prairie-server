package playback

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeProbeFFmpeg(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

func TestHardwareVideoDecoder(t *testing.T) {
	cases := map[string]string{
		"mpeg2video": "mpeg2_cuvid",
		"MPEG-2":     "mpeg2_cuvid",
		"mp2v":       "mpeg2_cuvid",
		"h264":       "h264_cuvid",
		"H.264":      "h264_cuvid",
		"hevc":       "hevc_cuvid",
		"h265":       "hevc_cuvid",
		"vc1":        "vc1_cuvid",
		"av1":        "av1_cuvid",
		"theora":     "",
	}
	for codec, want := range cases {
		if got := hardwareVideoDecoder(hwAccelNVENC, codec); got != want {
			t.Fatalf("hardwareVideoDecoder(nvenc, %q) = %q, want %q", codec, got, want)
		}
	}

	// QSV and VAAPI engage their decoders through the hwaccel device setup;
	// naming a decoder there fights with it.
	for _, accel := range []string{"qsv", hwAccelVAAPI, "none", ""} {
		if got := hardwareVideoDecoder(accel, "mpeg2video"); got != "" {
			t.Fatalf("hardwareVideoDecoder(%q) = %q, want empty", accel, got)
		}
	}
}

func TestResolveLiveVideoDecoderModes(t *testing.T) {
	resetDecoderProbeCacheForTest()
	t.Cleanup(resetDecoderProbeCacheForTest)

	withDecoder := writeProbeFFmpeg(t, "#!/bin/sh\necho ' V..... mpeg2_cuvid  Nvidia CUVID MPEG2VIDEO decoder'\n")
	withoutDecoder := writeProbeFFmpeg(t, "#!/bin/sh\necho ' V..... mpeg2  MPEG-2 video'\n")

	if got := resolveLiveVideoDecoder(HWDecodeAuto, hwAccelNVENC, "mpeg2video", withDecoder); got != "mpeg2_cuvid" {
		t.Fatalf("auto with decoder = %q, want mpeg2_cuvid", got)
	}
	if got := resolveLiveVideoDecoder(HWDecodeAuto, hwAccelNVENC, "mpeg2video", withoutDecoder); got != "" {
		t.Fatalf("auto without decoder = %q, want software", got)
	}
	if got := resolveLiveVideoDecoder(HWDecodeOff, hwAccelNVENC, "mpeg2video", withDecoder); got != "" {
		t.Fatalf("off = %q, want software", got)
	}
	// "on" trusts the operator and skips the probe.
	if got := resolveLiveVideoDecoder(HWDecodeOn, hwAccelNVENC, "mpeg2video", withoutDecoder); got != "mpeg2_cuvid" {
		t.Fatalf("on = %q, want mpeg2_cuvid", got)
	}
	if got := resolveLiveVideoDecoder(HWDecodeAuto, "none", "mpeg2video", withDecoder); got != "" {
		t.Fatalf("software encode = %q, want no hardware decoder", got)
	}
}

// A GPU that cannot decode this codec should cost one restart, not the
// channel: decode drops to software first, then acceleration entirely.
func TestLiveFallbackLadder(t *testing.T) {
	encode := LiveHLSOpts{
		VideoCodec:       "h264",
		SourceVideoCodec: "mpeg2video",
		HWAccel:          hwAccelNVENC,
		HWDecode:         HWDecodeOn,
	}
	ladder := liveFallbackLadder(encode)
	if len(ladder) != 3 {
		t.Fatalf("ladder length = %d, want 3", len(ladder))
	}
	if ladder[0].HWDecode != HWDecodeOn || ladder[0].HWAccel != hwAccelNVENC {
		t.Fatalf("first rung = %+v, want the configured pipeline", ladder[0])
	}
	if ladder[1].HWDecode != HWDecodeOff || ladder[1].HWAccel != hwAccelNVENC {
		t.Fatalf("second rung = %+v, want software decode with hardware encode", ladder[1])
	}
	if ladder[2].HWAccel != "none" || ladder[2].HWDecode != HWDecodeOff {
		t.Fatalf("third rung = %+v, want full software", ladder[2])
	}

	// Software decode already configured: only the encoder can degrade.
	softwareDecode := encode
	softwareDecode.HWDecode = HWDecodeOff
	if got := len(liveFallbackLadder(softwareDecode)); got != 2 {
		t.Fatalf("software-decode ladder length = %d, want 2", got)
	}

	// Nothing to fall back from.
	softwareOnly := encode
	softwareOnly.HWAccel = "none"
	if got := len(liveFallbackLadder(softwareOnly)); got != 1 {
		t.Fatalf("software ladder length = %d, want 1", got)
	}
	copySession := LiveHLSOpts{HWAccel: hwAccelNVENC}
	if got := len(liveFallbackLadder(copySession)); got != 1 {
		t.Fatalf("copy ladder length = %d, want 1", got)
	}
}

// Only hardware failures are worth retrying; a dead tuner must fail once.
func TestLooksLikeHardwareFailure(t *testing.T) {
	hardware := []string{
		"live hls ffmpeg exited: exit status 255 (Cannot load libcuda.so.1)",
		"Hardware device setup failed for decoder: Operation not permitted",
		"[vost#0:0/h264_nvenc] Error initializing a simple filtergraph",
		"Failed to initialize VAAPI connection",
	}
	for _, message := range hardware {
		if !looksLikeHardwareFailure(errors.New(message)) {
			t.Fatalf("expected a hardware failure: %s", message)
		}
	}

	other := []string{
		"live hls ffmpeg exited: exit status 1 (Server returned 404 Not Found)",
		"live hls playlist not ready within timeout",
		"connection refused",
	}
	for _, message := range other {
		if looksLikeHardwareFailure(errors.New(message)) {
			t.Fatalf("must not retry on: %s", message)
		}
	}
	if looksLikeHardwareFailure(nil) {
		t.Fatal("nil error is not a hardware failure")
	}
}

func TestFFmpegHasDecoderCachesPerBinary(t *testing.T) {
	resetDecoderProbeCacheForTest()
	t.Cleanup(resetDecoderProbeCacheForTest)

	dir := t.TempDir()
	counter := filepath.Join(dir, "calls")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho x >> " + counter + "\necho ' V..... h264_cuvid  Nvidia CUVID H264 decoder'\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if !ffmpegHasDecoder(ffmpeg, "h264_cuvid") {
			t.Fatal("expected the decoder to be reported available")
		}
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("read probe counter: %v", err)
	}
	if len(data) != 2 {
		t.Fatalf("probe ran %d times, want exactly 1", len(data)/2)
	}

	if ffmpegHasDecoder(ffmpeg, "") {
		t.Fatal("empty decoder name must not be reported available")
	}
}
