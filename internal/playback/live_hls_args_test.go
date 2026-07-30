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
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL: "http://tuner/auto/v4.1",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	if !hasFlagValue(args, "-c:v", "copy") || !hasFlagValue(args, "-c:a", "copy") {
		t.Fatalf("expected copy for both streams: %s", argsString(args))
	}
	if !hasFlagValue(args, "-map", "0:a:0?") {
		t.Fatalf("copy sessions keep optional audio mapping: %s", argsString(args))
	}
	if !hasFlagValue(args, "-flags", "low_delay") {
		t.Fatalf("expected the low-latency remux flags: %s", argsString(args))
	}
	if strings.Contains(argsString(args), "libx264") {
		t.Fatalf("copy session must not encode: %s", argsString(args))
	}
	if strings.Contains(argsString(args), "aresample=") || strings.Contains(argsString(args), "avoid_negative_ts") {
		t.Fatalf("copy remux must not add encode-only continuity flags: %s", argsString(args))
	}
}

// The browser case: MPEG-2 video and AC-3 audio both have to be re-encoded.
func TestBuildLiveHLSArgsEncodesForBrowsers(t *testing.T) {
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
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
	if !hasFlagValue(args, "-map", "0:a:0") {
		t.Fatalf("encoded audio must require an audio track: %s", joined)
	}
	if strings.Contains(joined, "0:a:0?") {
		t.Fatalf("encoded audio must not use optional audio mapping: %s", joined)
	}
	if !hasFlagValue(args, "-af", "aresample=async=1:first_pts=0") {
		t.Fatalf("expected audio continuity filter so segment 0 carries AAC: %s", joined)
	}
	if !hasFlagValue(args, "-avoid_negative_ts", "make_zero") {
		t.Fatalf("expected zero-based timestamps for encoded live HLS: %s", joined)
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
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
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
	args, pipeline := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		SourceVideoCodec: "mpeg2video",
		HWAccel:          hwAccelNVENC,
		HWDecode:         HWDecodeOn,
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
	if pipeline.Decoder != "mpeg2_cuvid" {
		t.Fatalf("decoder = %q, want mpeg2_cuvid", pipeline.Decoder)
	}
}

// Software MPEG-2 decode at 59.94fps is what pushes a live transcode below
// realtime, so the CUVID decoder has to be named before the input.
func TestBuildLiveHLSArgsPinsHardwareDecodeBeforeInput(t *testing.T) {
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		SourceVideoCodec: "mpeg2video",
		HWAccel:          hwAccelNVENC,
		HWDecode:         HWDecodeOn,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	decoderAt, inputAt, encoderAt := -1, -1, -1
	for i := 0; i < len(args)-1; i++ {
		switch {
		case args[i] == "-c:v" && args[i+1] == "mpeg2_cuvid":
			decoderAt = i
		case args[i] == "-i":
			inputAt = i
		case args[i] == "-c:v" && args[i+1] == "h264_nvenc":
			encoderAt = i
		}
	}
	if decoderAt < 0 || inputAt < 0 || encoderAt < 0 {
		t.Fatalf("missing decoder/input/encoder: %s", argsString(args))
	}
	if decoderAt >= inputAt || inputAt >= encoderAt {
		t.Fatalf("decoder must precede -i and the encoder must follow it: %s", argsString(args))
	}
}

func TestBuildLiveHLSArgsHardwareDecodeOff(t *testing.T) {
	args, pipeline := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		SourceVideoCodec: "mpeg2video",
		HWAccel:          hwAccelNVENC,
		HWDecode:         HWDecodeOff,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	if pipeline.Decoder != "" {
		t.Fatalf("decoder = %q, want software decode", pipeline.Decoder)
	}
	if strings.Contains(argsString(args), "cuvid") {
		t.Fatalf("expected no CUVID decoder: %s", argsString(args))
	}
}

// Live defaults to a low-latency preset: an encode that drifts below realtime
// never catches up with a broadcast.
func TestBuildLiveHLSArgsDefaultsToLowLatency(t *testing.T) {
	software, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:   "http://tuner/auto/v4.1",
		VideoCodec: "h264",
		HWAccel:    "none",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)
	if !hasFlagValue(software, "-preset", "ultrafast") || !hasFlagValue(software, "-tune", "zerolatency") {
		t.Fatalf("expected x264 low-latency flags: %s", argsString(software))
	}

	nvenc, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:   "http://tuner/auto/v4.1",
		VideoCodec: "h264",
		HWAccel:    hwAccelNVENC,
		HWDecode:   HWDecodeOff,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)
	if !hasFlagValue(nvenc, "-preset", "p2") || !hasFlagValue(nvenc, "-tune", "ll") {
		t.Fatalf("expected NVENC low-latency flags: %s", argsString(nvenc))
	}
}

func TestBuildLiveHLSArgsQualityPresetKeepsVODBehavior(t *testing.T) {
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:      "http://tuner/auto/v4.1",
		VideoCodec:    "h264",
		HWAccel:       hwAccelNVENC,
		HWDecode:      HWDecodeOff,
		EncoderPreset: EncoderPresetQuality,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	joined := argsString(args)
	if strings.Contains(joined, "-tune ll") || strings.Contains(joined, "-preset p2") {
		t.Fatalf("quality preset must not add latency tuning: %s", joined)
	}
	if !hasFlagValue(args, "-cq:v", "23") {
		t.Fatalf("expected the quality rate control: %s", joined)
	}
}

// Capping frame rate is a fallback for hardware that cannot sustain the
// broadcast rate, so it must stay off unless asked for.
func TestBuildLiveHLSArgsFrameRateCap(t *testing.T) {
	uncapped, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:   "http://tuner/auto/v4.1",
		VideoCodec: "h264",
		HWAccel:    "none",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)
	if strings.Contains(argsString(uncapped), " -r ") {
		t.Fatalf("expected no frame rate cap by default: %s", argsString(uncapped))
	}

	capped, pipeline := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:     "http://tuner/auto/v4.1",
		VideoCodec:   "h264",
		HWAccel:      "none",
		FrameRateCap: "30",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)
	// Broadcast rates are fractional; 30 must mean 29.97, not 30.
	if !hasFlagValue(capped, "-r", "30000/1001") {
		t.Fatalf("expected a fractional 29.97 cap: %s", argsString(capped))
	}
	if pipeline.FrameRate != "30000/1001" {
		t.Fatalf("pipeline frame rate = %q", pipeline.FrameRate)
	}

	sixty, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:     "http://tuner/auto/v4.1",
		VideoCodec:   "h264",
		HWAccel:      "none",
		FrameRateCap: "60",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)
	if !hasFlagValue(sixty, "-r", "60000/1001") {
		t.Fatalf("expected a fractional 59.94 cap: %s", argsString(sixty))
	}
}

// A copy session must stay a plain remux no matter what the live policy says.
func TestBuildLiveHLSArgsCopyIgnoresEncodeSettings(t *testing.T) {
	args, pipeline := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		SourceVideoCodec: "mpeg2video",
		HWAccel:          hwAccelNVENC,
		HWDecode:         HWDecodeOn,
		FrameRateCap:     "30",
		EncoderPreset:    EncoderPresetLowLatency,
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	joined := argsString(args)
	if !hasFlagValue(args, "-c:v", "copy") || !hasFlagValue(args, "-c:a", "copy") {
		t.Fatalf("expected a straight remux: %s", joined)
	}
	if strings.Contains(joined, "cuvid") || strings.Contains(joined, "-hwaccel") || strings.Contains(joined, " -r ") {
		t.Fatalf("copy session must not carry encode flags: %s", joined)
	}
	if pipeline.Decoder != "" {
		t.Fatalf("decoder = %q, want none for a copy session", pipeline.Decoder)
	}
}

func TestBuildLiveHLSArgsScalesDownWhenAsked(t *testing.T) {
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         "http://tuner/auto/v4.1",
		VideoCodec:       "h264",
		TargetResolution: "720p",
		HWAccel:          "none",
	}, "/out/index.m3u8", "/out/seg_%05d.ts", 1, 6)

	if !strings.Contains(argsString(args), "scale=-2:720") {
		t.Fatalf("expected a 720p downscale: %s", argsString(args))
	}
}
