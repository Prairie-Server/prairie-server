package playback

import "testing"

// The failure this exists to prevent: a panel that tops out at 5.1 being handed
// eight-channel AAC, which it reports as an unsupported audio codec.
func TestEffectiveAudioChannelsRespectsClientCeiling(t *testing.T) {
	for _, tc := range []struct {
		name           string
		sourceChannels int
		maxChannels    int
		want           int
	}{
		{name: "7.1 source on a 5.1 panel", sourceChannels: 8, maxChannels: 6, want: 6},
		{name: "7.1 source on a 7.1 panel", sourceChannels: 8, maxChannels: 8, want: 8},
		{name: "5.1 source on a 7.1 panel never upmixes", sourceChannels: 6, maxChannels: 8, want: 6},
		{name: "stereo source stays stereo", sourceChannels: 2, maxChannels: 8, want: 2},
		{name: "ceiling of 2 forces stereo", sourceChannels: 8, maxChannels: 2, want: 2},
		// Silence about a capability is not evidence of it.
		{name: "no declared ceiling keeps the stereo default", sourceChannels: 8, maxChannels: 0, want: 2},
		{name: "negative ceiling is treated as absent", sourceChannels: 8, maxChannels: -1, want: 2},
		// An overstated ceiling is clamped, not rejected: a failed session is
		// worse than the best layout we actually produce.
		{name: "ceiling above what we encode is clamped", sourceChannels: 8, maxChannels: 16, want: 8},
		// Odd ceilings snap down to a real layout.
		{name: "ceiling of 7 snaps to 5.1", sourceChannels: 8, maxChannels: 7, want: 6},
		{name: "ceiling of 5 snaps to stereo", sourceChannels: 8, maxChannels: 5, want: 2},
		// A mono ceiling is honored rather than rounded up: "never exceed the
		// client's ceiling" has to include the bottom of the ladder.
		{name: "ceiling of 1 yields mono", sourceChannels: 8, maxChannels: 1, want: 1},
		// Unknown source count must not collapse a surround mix to mono.
		{name: "unknown source count follows the ceiling", sourceChannels: 0, maxChannels: 6, want: 6},
	} {
		if got := EffectiveAudioChannels(tc.sourceChannels, tc.maxChannels, "eac3"); got != tc.want {
			t.Errorf("%s: EffectiveAudioChannels(%d, %d) = %d, want %d",
				tc.name, tc.sourceChannels, tc.maxChannels, got, tc.want)
		}
	}
}

// Multichannel TrueHD decode is a known segment-muxing stall source, and that
// risk does not change because a client says it can take 5.1 on the far side.
func TestEffectiveAudioChannelsAlwaysDownmixesFragileLossless(t *testing.T) {
	for _, codec := range []string{"truehd", "TrueHD", "mlp", "MLP FBA"} {
		for _, maxChannels := range []int{0, 2, 6, 8, 16} {
			if got := EffectiveAudioChannels(8, maxChannels, codec); got != 2 {
				t.Errorf("EffectiveAudioChannels(8, %d, %q) = %d, want 2", maxChannels, codec, got)
			}
		}
	}
}

// The bitrate has to scale with the layout, or 5.1/7.1 arrives starved.
func TestAudioBitrateForChannels(t *testing.T) {
	for _, tc := range []struct {
		channels int
		want     string
	}{
		{channels: 2, want: "192k"},
		{channels: 6, want: "384k"},
		{channels: 8, want: "512k"},
		{channels: 1, want: "192k"},
	} {
		if got := AudioBitrateForChannels(tc.channels); got != tc.want {
			t.Errorf("AudioBitrateForChannels(%d) = %q, want %q", tc.channels, got, tc.want)
		}
	}
}

// Every value the resolver can return must be a layout the ladder names, so no
// caller can be handed a channel count nothing is obliged to decode.
func TestEffectiveAudioChannelsAlwaysReturnsALadderRung(t *testing.T) {
	valid := map[int]bool{}
	for _, rung := range audioChannelLadder {
		valid[rung] = true
	}
	for source := 0; source <= 10; source++ {
		for maxChannels := -2; maxChannels <= 12; maxChannels++ {
			got := EffectiveAudioChannels(source, maxChannels, "aac")
			if !valid[got] {
				t.Fatalf("EffectiveAudioChannels(%d, %d) = %d, which is not a ladder rung", source, maxChannels, got)
			}
		}
	}
}

func TestAudioTrackChannelsAtBounds(t *testing.T) {
	if got := AudioTrackChannelsAt(nil, 0); got != 0 {
		t.Errorf("AudioTrackChannelsAt(nil, 0) = %d, want 0", got)
	}
}

// "Never exceed the source" has to include mono, or the rule is only a
// suggestion. Duplicating a mono track into stereo doubles the bitrate for no
// information gain.
func TestEffectiveAudioChannelsPreservesMono(t *testing.T) {
	if got := EffectiveAudioChannels(1, 6, "aac"); got != 1 {
		t.Errorf("EffectiveAudioChannels(1, 6) = %d, want 1 (mono source stays mono)", got)
	}
	if got := EffectiveAudioChannels(1, 8, "eac3"); got != 1 {
		t.Errorf("EffectiveAudioChannels(1, 8) = %d, want 1", got)
	}
	// A mono ceiling is honored even from a surround source.
	if got := EffectiveAudioChannels(8, 1, "aac"); got != 1 {
		t.Errorf("EffectiveAudioChannels(8, 1) = %d, want 1 (client ceiling)", got)
	}
}

// EffectiveAC3Channels is capped by the encoder, the source, and the client's
// declared ceiling -- tightest wins. The 5.1 cap is not a style choice: our own
// image's `ffmpeg -h encoder=ac3` lists 5.1 as the widest supported layout.
func TestEffectiveAC3Channels(t *testing.T) {
	cases := []struct {
		name         string
		source, ceil int
		want         int
	}{
		{"7.1 source capped at the encoder's 5.1", 8, 8, 6},
		{"no client ceiling still capped at 5.1", 8, 0, 6},
		{"5.1 source passes through", 6, 6, 6},
		{"stereo ceiling wins over a 5.1 source", 6, 2, 2},
		{"stereo source is not upmixed to a 5.1 ceiling", 2, 6, 2},
		{"mono source stays mono", 1, 6, 1},
		{"mono ceiling honored from a surround source", 8, 1, 1},
		{"odd ceiling snaps down to a real layout", 8, 5, 2},
		{"unknown source with a binding ceiling still caps", 0, 2, 2},
		{"over-claimed ceiling clamps to the encoder max", 8, 99, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveAC3Channels(tc.source, tc.ceil); got != tc.want {
				t.Errorf("EffectiveAC3Channels(%d, %d) = %d, want %d", tc.source, tc.ceil, got, tc.want)
			}
		})
	}
}

// 0 means "leave the layout to FFmpeg", which is only correct when we genuinely
// have nothing to go on. Guessing 5.1 there would upmix an unknown stereo source.
func TestEffectiveAC3ChannelsDefersWhenNothingIsKnown(t *testing.T) {
	if got := EffectiveAC3Channels(0, 0); got != 0 {
		t.Errorf("EffectiveAC3Channels(0, 0) = %d, want 0 (defer to FFmpeg)", got)
	}
	// A ceiling that does not bind tighter than the encoder tells us nothing the
	// encoder would not already enforce, so it also defers.
	if got := EffectiveAC3Channels(0, 8); got != 0 {
		t.Errorf("EffectiveAC3Channels(0, 8) = %d, want 0", got)
	}
}

// The AC-3 target exists to carry surround to a receiver. Inheriting the AAC
// path's TrueHD-to-stereo safety would defeat the only reason to pick it -- that
// stall is in the TrueHD-to-AAC path, not here.
func TestEffectiveAC3ChannelsIgnoresFragileLosslessDownmix(t *testing.T) {
	if got := EffectiveAC3Channels(8, 6); got != 6 {
		t.Errorf("EffectiveAC3Channels(8, 6) = %d, want 6 even for a TrueHD source", got)
	}
	// Contrast with the AAC path, which must downmix the same source.
	if got := EffectiveAudioChannels(8, 6, "truehd"); got != 2 {
		t.Errorf("EffectiveAudioChannels(8, 6, truehd) = %d, want 2", got)
	}
}

func TestAC3BitrateForChannels(t *testing.T) {
	cases := map[int]string{
		0: "448k", // deferred layout cannot exceed 5.1, so pair the surround rate
		6: "448k",
		2: "192k",
		1: "96k",
	}
	for channels, want := range cases {
		if got := AC3BitrateForChannels(channels); got != want {
			t.Errorf("AC3BitrateForChannels(%d) = %q, want %q", channels, got, want)
		}
	}
}
