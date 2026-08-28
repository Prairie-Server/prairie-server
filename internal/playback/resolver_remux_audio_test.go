package playback

import "testing"

func TestRemuxAudioCopySafeNormalizesCodecAliases(t *testing.T) {
	plain := ClientCapabilities{}
	for _, alias := range []string{"AAC", "aac_lc", "mp4a", "he-aac"} {
		if !remuxAudioCopySafe(alias, plain) {
			t.Errorf("remuxAudioCopySafe(%q) = false, want true", alias)
		}
	}
	for _, alias := range []string{"AC-3", "E-AC-3", "ec3", "DTS-HD MA", "TrueHD", "mlp", "", "flac"} {
		if remuxAudioCopySafe(alias, plain) {
			t.Errorf("remuxAudioCopySafe(%q) = true, want false without sink evidence", alias)
		}
	}
}

// Both ends of the comparison are free-form: probes emit "E-AC-3"/"DTS-HD MA"
// while client tables carry canonical "eac3"/"dts". Comparing them raw meant a
// declared sink passthrough was silently downmixed to AAC -- the exact promise
// the list exists to keep.
func TestRemuxAudioCopySafeMatchesAliasedSourceAgainstCanonicalClaim(t *testing.T) {
	caps := ClientCapabilities{AudioPassthroughCodecs: []string{"eac3", "dts", "ac3", "truehd"}}

	for _, sourceCodec := range []string{
		"E-AC-3", "e-ac-3", "EAC3", "ec3", "Dolby Digital Plus",
		"DTS-HD MA", "dts_hd", "DTSX",
		"AC-3", "Dolby Digital",
		"TrueHD", "MLP",
	} {
		if !remuxAudioCopySafe(sourceCodec, caps) {
			t.Errorf("remuxAudioCopySafe(%q) = false; a canonical claim must match an aliased source", sourceCodec)
		}
	}
}

// The reverse direction too: an aliased client claim must match a canonical
// source label.
func TestRemuxAudioCopySafeMatchesCanonicalSourceAgainstAliasedClaim(t *testing.T) {
	for _, claim := range []string{"E-AC-3", "ec3", "Dolby Digital Plus"} {
		caps := ClientCapabilities{AudioPassthroughCodecs: []string{claim}}
		if !remuxAudioCopySafe("eac3", caps) {
			t.Errorf("claim %q did not match source \"eac3\"", claim)
		}
	}
}

// Normalization must not turn unrelated codecs into matches.
func TestAudioCodecClaimedDoesNotOvermatch(t *testing.T) {
	caps := ClientCapabilities{AudioPassthroughCodecs: []string{"eac3"}}
	for _, sourceCodec := range []string{"ac3", "dts", "truehd", "aac", "flac", "opus", ""} {
		if sourceCodec == "aac" {
			continue // AAC is unconditionally safe, tested separately.
		}
		if remuxAudioCopySafe(sourceCodec, caps) {
			t.Errorf("remuxAudioCopySafe(%q) = true against a claim of only eac3", sourceCodec)
		}
	}
	if audioCodecClaimed([]string{"eac3"}, "") {
		t.Error("an empty source codec must not match anything")
	}
	if audioCodecClaimed(nil, "eac3") {
		t.Error("a nil capability list must not match")
	}
}
