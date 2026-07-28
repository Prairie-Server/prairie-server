package playback

import (
	"strings"
	"testing"
)

func argsString(args []string) string {
	return strings.Join(args, " ")
}

func hasFlagValue(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

// Native players decode the broadcast directly, so the default stays a remux.
func TestBuildLiveHLSArgsCopiesByDefault(t *testing.T) {
	args := buildLiveHLSArgs(LiveHLSOpts{
		InputURL: "http://tuner/auto/v4.1",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	if !hasFlagValue(args, "-c:v", "copy") || !hasFlagValue(args, "-c:a", "copy") {
		t.Fatalf("expected copy for both streams: %s", argsString(args))
	}
	if !hasFlagValue(args, "-flags", "low_delay") {
		t.Fatalf("expected the low-latency remux flags: %s", argsString(args))
	}
	if strings.Contains(argsString(args), "libx264") {
		t.Fatalf("copy session must not encode: %s", argsString(args))
	}
}

// The browser case: MPEG-2 video and AC-3 audio both have to be re-encoded.
func TestBuildLiveHLSArgsEncodesForBrowsers(t *testing.T) {
	args := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		AudioCodec:       "aac",
		SourceVideoCodec: "mpeg2video",
		SourceAudioCodec: "ac3",
		HWAccel:          "none",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	joined := argsString(args)
	if !hasFlagValue(args, "-c:v", "libx264") {
		t.Fatalf("expected a software h264 encode: %s", joined)
	}
	if !hasFlagValue(args, "-c:a", "aac") || !hasFlagValue(args, "-ac", "2") {
		t.Fatalf("expected an AAC stereo downmix: %s", joined)
	}
	if !hasFlagValue(args, "-pix_fmt", "yuv420p") {
		t.Fatalf("expected 8-bit output for MSE: %s", joined)
	}
	// Segments must start on keyframes or hls.js cannot append them.
	if !strings.Contains(joined, "-force_key_frames") {
		t.Fatalf("expected segment-aligned keyframes: %s", joined)
	}
	if !hasFlagValue(args, "-hls_segment_type", "mpegts") {
		t.Fatalf("live segments stay MPEG-TS: %s", joined)
	}
}

// Audio-only encodes (client decodes MPEG-2 but not AC-3) leave video alone.
func TestBuildLiveHLSArgsEncodesAudioOnly(t *testing.T) {
	args := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:      "http://tuner/auto/v4.1",
		AudioCodec:    "aac",
		AudioChannels: 6,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	joined := argsString(args)
	if !hasFlagValue(args, "-c:v", "copy") {
		t.Fatalf("expected video copy: %s", joined)
	}
	if !hasFlagValue(args, "-ac", "6") {
		t.Fatalf("expected 5.1 AAC when the client asked for it: %s", joined)
	}
	if strings.Contains(joined, "-force_key_frames") {
		t.Fatalf("copy-video sessions must not force keyframes: %s", joined)
	}
}

func TestBuildLiveHLSArgsUsesNVENCWhenSelected(t *testing.T) {
	args := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		SourceVideoCodec: "mpeg2video",
		HWAccel:          hwAccelNVENC,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	joined := argsString(args)
	if !hasFlagValue(args, "-c:v", "h264_nvenc") {
		t.Fatalf("expected the NVENC encoder: %s", joined)
	}
	if !hasFlagValue(args, "-hwaccel", "cuda") {
		t.Fatalf("expected CUDA decode for the NVENC pipeline: %s", joined)
	}
	if !strings.Contains(joined, "scale_cuda") {
		t.Fatalf("expected the CUDA format filter: %s", joined)
	}
}

func TestBuildLiveHLSArgsScalesDownWhenAsked(t *testing.T) {
	args := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		TargetResolution: "720p",
		HWAccel:          "none",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	if !strings.Contains(argsString(args), "scale=-2:720") {
		t.Fatalf("expected a 720p downscale: %s", argsString(args))
	}
}
