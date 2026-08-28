package livetv

import "testing"

func browserPlan() StreamPlan {
	return PlanLiveStream(ClientCapabilities{
		CodecsVideo: []string{"h264"},
		CodecsAudio: []string{"aac"},
	}, BroadcastSourceCodecs)
}

func TestSettingsForceCopy(t *testing.T) {
	plan := TranscodeSettings{PlayMethod: PlayMethodCopy}.applyTo(browserPlan())

	if plan.VideoCodec != "copy" || plan.AudioCodec != "copy" {
		t.Fatalf("plan = %+v, want copy/copy", plan)
	}
	if plan.Transcodes() {
		t.Fatal("forced copy must not report transcoding")
	}
}

// Forcing a transcode covers a client that lies about what it can decode.
func TestSettingsForceTranscode(t *testing.T) {
	nativePlan := PlanLiveStream(ClientCapabilities{
		CodecsVideo: []string{"mpeg2video"},
		CodecsAudio: []string{"ac3"},
	}, BroadcastSourceCodecs)
	if nativePlan.Transcodes() {
		t.Fatal("precondition: a native client should plan a copy")
	}

	plan := TranscodeSettings{PlayMethod: PlayMethodTranscode}.applyTo(nativePlan)
	if plan.VideoCodec != "h264" || plan.AudioCodec != "aac" {
		t.Fatalf("plan = %+v, want h264/aac", plan)
	}
	if plan.AudioChannels != 2 {
		t.Fatalf("channels = %d, want a stereo downmix", plan.AudioChannels)
	}
}

func TestSettingsAutoLeavesPlanAlone(t *testing.T) {
	original := browserPlan()
	plan := TranscodeSettings{PlayMethod: PlayMethodAuto}.applyTo(original)

	if plan != original {
		t.Fatalf("plan = %+v, want %+v", plan, original)
	}
}

// An operator ceiling only ever lowers the client's request.
func TestSettingsResolutionCeiling(t *testing.T) {
	capped := TranscodeSettings{MaxResolution: "720p"}.applyTo(browserPlan())
	if capped.MaxResolution != "720p" {
		t.Fatalf("max resolution = %q, want 720p", capped.MaxResolution)
	}

	clientWantsLess := browserPlan()
	clientWantsLess.MaxResolution = "480p"
	stillLess := TranscodeSettings{MaxResolution: "720p"}.applyTo(clientWantsLess)
	if stillLess.MaxResolution != "480p" {
		t.Fatalf("max resolution = %q, want the client's smaller 480p", stillLess.MaxResolution)
	}

	// "source" and anything at or above broadcast must not rescale.
	for _, setting := range []string{"source", "", "1080p", "2160p"} {
		plan := TranscodeSettings{MaxResolution: setting}.applyTo(browserPlan())
		if plan.MaxResolution != "" {
			t.Fatalf("max resolution for %q = %q, want no scaling", setting, plan.MaxResolution)
		}
	}
}

// A copy session has no encoder to apply a resolution ceiling to.
func TestSettingsResolutionCeilingIgnoredForCopy(t *testing.T) {
	nativePlan := PlanLiveStream(ClientCapabilities{
		CodecsVideo: []string{"mpeg2video"},
		CodecsAudio: []string{"ac3"},
	}, BroadcastSourceCodecs)

	plan := TranscodeSettings{MaxResolution: "720p"}.applyTo(nativePlan)
	if plan.MaxResolution != "" {
		t.Fatalf("max resolution = %q, want none on a copy session", plan.MaxResolution)
	}
}

func TestResolutionRankOrdersSmallestFirst(t *testing.T) {
	ordered := []string{"328p", "420p", "480p", "720p", "1080p", "2160p"}
	for i := 1; i < len(ordered); i++ {
		if resolutionRank(ordered[i-1]) >= resolutionRank(ordered[i]) {
			t.Fatalf("%s should rank below %s", ordered[i-1], ordered[i])
		}
	}
	// Unknown labels must never win a comparison against a real ceiling.
	if resolutionRank("weird") <= resolutionRank("2160p") {
		t.Fatal("unknown resolution should rank above every known one")
	}
}

// A bridge with no settings provider still tunes; the zero policy means
// "follow client capabilities".
func TestBridgeWithoutSettingsProvider(t *testing.T) {
	var bridge *HLSBridge
	if got := bridge.currentSettings(t.Context()); got != (TranscodeSettings{}) {
		t.Fatalf("nil bridge settings = %+v, want zero", got)
	}
	empty := &HLSBridge{}
	if got := empty.currentSettings(t.Context()); got != (TranscodeSettings{}) {
		t.Fatalf("unset provider settings = %+v, want zero", got)
	}
}

// Capping frame rate is a fallback, so "source" (the default) must mean off.
func TestSettingsFrameRateCap(t *testing.T) {
	for setting, want := range map[string]string{
		"source": "",
		"":       "",
		"weird":  "",
		"30":     "30",
		"60":     "60",
	} {
		if got := (TranscodeSettings{FrameRateCap: setting}).frameRateCap(); got != want {
			t.Fatalf("frameRateCap(%q) = %q, want %q", setting, got, want)
		}
	}
}
