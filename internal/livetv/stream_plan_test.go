package livetv

import "testing"

// Browsers cannot decode the MPEG-2 / AC-3 an OTA tuner emits, so both streams
// must be re-encoded or Live TV is a black screen with no audio.
func TestPlanLiveStreamTranscodesForBrowsers(t *testing.T) {
	plan := PlanLiveStream(ClientCapabilities{
		CodecsVideo:   []string{"h264", "vp9", "av1"},
		CodecsAudio:   []string{"aac", "opus", "flac"},
		MaxResolution: "1080p",
	}, BroadcastSourceCodecs)

	if plan.VideoCodec != "h264" {
		t.Fatalf("video = %q, want h264", plan.VideoCodec)
	}
	if plan.AudioCodec != "aac" {
		t.Fatalf("audio = %q, want aac", plan.AudioCodec)
	}
	if plan.AudioChannels != 2 {
		t.Fatalf("channels = %d, want a stereo downmix", plan.AudioChannels)
	}
	// 1080p is at or above broadcast, so nothing should be rescaled.
	if plan.MaxResolution != "" {
		t.Fatalf("max resolution = %q, want no scaling", plan.MaxResolution)
	}
	if !plan.Transcodes() || !plan.TranscodesVideo() {
		t.Fatal("expected the plan to report transcoding")
	}
}

// Native TV players decode the broadcast codecs, so they keep the cheap copy.
func TestPlanLiveStreamCopiesForNativeDecoders(t *testing.T) {
	plan := PlanLiveStream(ClientCapabilities{
		CodecsVideo: []string{"mpeg2video", "h264", "hevc"},
		CodecsAudio: []string{"ac3", "eac3", "aac"},
	}, BroadcastSourceCodecs)

	if plan.VideoCodec != "copy" || plan.AudioCodec != "copy" {
		t.Fatalf("plan = %+v, want copy/copy", plan)
	}
	if plan.Transcodes() {
		t.Fatal("copy plan must not report transcoding")
	}
}

// Clients that predate capability reporting are native players that were
// already decoding the broadcast directly.
func TestPlanLiveStreamCopiesWhenClientSaysNothing(t *testing.T) {
	plan := PlanLiveStream(ClientCapabilities{}, BroadcastSourceCodecs)
	if plan.VideoCodec != "copy" || plan.AudioCodec != "copy" {
		t.Fatalf("plan = %+v, want copy/copy", plan)
	}
}

// A receiver that can pass AC-3 through keeps surround even though the video
// has to be re-encoded.
func TestPlanLiveStreamHonorsAudioPassthrough(t *testing.T) {
	plan := PlanLiveStream(ClientCapabilities{
		CodecsVideo:            []string{"h264"},
		CodecsAudio:            []string{"aac"},
		AudioPassthroughCodecs: []string{"ac3"},
	}, BroadcastSourceCodecs)

	if plan.VideoCodec != "h264" {
		t.Fatalf("video = %q, want h264", plan.VideoCodec)
	}
	if plan.AudioCodec != "copy" {
		t.Fatalf("audio = %q, want copy for a passthrough receiver", plan.AudioCodec)
	}
}

func TestPlanLiveStreamAudioChannels(t *testing.T) {
	surround := PlanLiveStream(ClientCapabilities{
		CodecsVideo:      []string{"mpeg2video"},
		CodecsAudio:      []string{"aac"},
		MaxAudioChannels: 6,
	}, BroadcastSourceCodecs)
	if surround.AudioChannels != 6 {
		t.Fatalf("channels = %d, want 6", surround.AudioChannels)
	}

	// A client asking for more than the broadcast carries gets what exists.
	clamped := PlanLiveStream(ClientCapabilities{
		CodecsVideo:      []string{"mpeg2video"},
		CodecsAudio:      []string{"aac"},
		MaxAudioChannels: 8,
	}, SourceCodecs{Video: "mpeg2video", Audio: "ac3", Channels: 2})
	if clamped.AudioChannels != 2 {
		t.Fatalf("channels = %d, want 2", clamped.AudioChannels)
	}

	mono := PlanLiveStream(ClientCapabilities{
		CodecsVideo:      []string{"mpeg2video"},
		CodecsAudio:      []string{"aac"},
		MaxAudioChannels: 1,
	}, BroadcastSourceCodecs)
	if mono.AudioChannels != 2 {
		t.Fatalf("channels = %d, want a stereo floor", mono.AudioChannels)
	}

	unknownSource := PlanLiveStream(ClientCapabilities{
		CodecsVideo:      []string{"mpeg2video"},
		CodecsAudio:      []string{"aac"},
		MaxAudioChannels: 6,
	}, SourceCodecs{Video: "mpeg2video", Audio: "ac3"})
	if unknownSource.AudioChannels != 2 {
		t.Fatalf("channels = %d, want 2 when the source layout is unknown", unknownSource.AudioChannels)
	}
}

// Scale filters pin an exact height, so a cap at or above broadcast resolution
// must not be honored — it would upscale the picture.
func TestLiveDownscaleResolution(t *testing.T) {
	for _, cap := range []string{"", "1080p", "2160p", "unknown"} {
		if got := liveDownscaleResolution(cap); got != "" {
			t.Fatalf("liveDownscaleResolution(%q) = %q, want empty", cap, got)
		}
	}
	for _, cap := range []string{"720p", "480p", "420p", "328p"} {
		if got := liveDownscaleResolution(cap); got != cap {
			t.Fatalf("liveDownscaleResolution(%q) = %q", cap, got)
		}
	}
	if got := liveDownscaleResolution(" 720P "); got != "720p" {
		t.Fatalf("normalized cap = %q, want 720p", got)
	}
}

func TestClientCapabilitiesDeclared(t *testing.T) {
	if (ClientCapabilities{}).Declared() {
		t.Fatal("empty capabilities must not count as declared")
	}
	if !(ClientCapabilities{CodecsVideo: []string{"h264"}}).Declared() {
		t.Fatal("video codecs should count as declared")
	}
	if !(ClientCapabilities{CodecsAudio: []string{"aac"}}).Declared() {
		t.Fatal("audio codecs should count as declared")
	}
}
