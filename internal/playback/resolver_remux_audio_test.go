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
