package playback

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

// The case that motivated this: a Blu-ray remux whose default track is TrueHD
// 7.1, with an AC3 5.1 companion in the same language. Copying the companion
// beats re-encoding the lossless track on every axis -- no CPU, no quality loss,
// and 5.1 survives instead of collapsing to stereo.
func TestSelectClientPlayableAudioTrackPrefersCompanionOverReencode(t *testing.T) {
	tracks := []models.AudioTrack{
		{Codec: "truehd", Channels: 8, Language: "en"},
		{Codec: "dts", Channels: 8, Language: "en"},
		{Codec: "ac3", Channels: 6, Language: "en"},
		{Codec: "ac3", Channels: 2, Language: "en"},
	}
	claimed := []string{"aac", "ac3", "eac3", "mp3"}

	idx, ok := SelectClientPlayableAudioTrack(tracks, 0, claimed, 6)
	if !ok || idx != 2 {
		t.Fatalf("SelectClientPlayableAudioTrack() = (%d, %v), want the AC3 5.1 track at 2", idx, ok)
	}
}

// Channel count is preferred, so track order must not decide the outcome.
func TestSelectClientPlayableAudioTrackPrefersMoreChannels(t *testing.T) {
	tracks := []models.AudioTrack{
		{Codec: "truehd", Channels: 8, Language: "en"},
		{Codec: "ac3", Channels: 2, Language: "en"},
		{Codec: "ac3", Channels: 6, Language: "en"},
	}
	idx, ok := SelectClientPlayableAudioTrack(tracks, 0, []string{"ac3"}, 8)
	if !ok || idx != 2 {
		t.Fatalf("got (%d, %v), want the 5.1 track at 2 even though stereo comes first", idx, ok)
	}
}

// A track above the client's ceiling is exactly what this exists to avoid.
func TestSelectClientPlayableAudioTrackHonorsChannelCeiling(t *testing.T) {
	tracks := []models.AudioTrack{
		{Codec: "truehd", Channels: 8, Language: "en"},
		{Codec: "eac3", Channels: 8, Language: "en"},
		{Codec: "ac3", Channels: 6, Language: "en"},
	}
	idx, ok := SelectClientPlayableAudioTrack(tracks, 0, []string{"ac3", "eac3"}, 6)
	if !ok || idx != 2 {
		t.Fatalf("got (%d, %v), want the 5.1 track at 2; the 7.1 E-AC-3 exceeds the ceiling", idx, ok)
	}
}

// Correct language in stereo beats the wrong language in surround.
func TestSelectClientPlayableAudioTrackPrefersLanguageOverChannels(t *testing.T) {
	tracks := []models.AudioTrack{
		{Codec: "truehd", Channels: 8, Language: "en"},
		{Codec: "ac3", Channels: 6, Language: "de"},
		{Codec: "ac3", Channels: 2, Language: "en"},
	}
	idx, ok := SelectClientPlayableAudioTrack(tracks, 0, []string{"ac3"}, 8)
	if !ok || idx != 2 {
		t.Fatalf("got (%d, %v), want the English stereo track at 2", idx, ok)
	}
}

// No decodable track means the caller must fall back to re-encoding.
func TestSelectClientPlayableAudioTrackReportsNoCandidate(t *testing.T) {
	tracks := []models.AudioTrack{
		{Codec: "truehd", Channels: 8, Language: "en"},
		{Codec: "dts", Channels: 8, Language: "en"},
	}
	if idx, ok := SelectClientPlayableAudioTrack(tracks, 0, []string{"aac", "ac3"}, 6); ok {
		t.Fatalf("got (%d, true), want no candidate", idx)
	}
	// An empty claim list cannot qualify anything.
	if _, ok := SelectClientPlayableAudioTrack(tracks, 0, nil, 6); ok {
		t.Fatal("a client claiming no audio codecs must yield no candidate")
	}
}

// Aliased spellings must match, since probes and client tables disagree.
func TestSelectClientPlayableAudioTrackMatchesAliases(t *testing.T) {
	tracks := []models.AudioTrack{
		{Codec: "truehd", Channels: 8, Language: "en"},
		{Codec: "E-AC-3", Channels: 6, Language: "en"},
	}
	idx, ok := SelectClientPlayableAudioTrack(tracks, 0, []string{"eac3"}, 6)
	if !ok || idx != 1 {
		t.Fatalf("got (%d, %v), want the E-AC-3 track at 1", idx, ok)
	}
}

// Now that AC-3/E-AC-3 copy on a plain claim, the selected companion is copied
// rather than re-encoded -- which is the whole point of selecting it.
func TestRemuxAudioCopySafeAllowsAC3FamilyOnPlainClaim(t *testing.T) {
	caps := ClientCapabilities{CodecsAudio: []string{"aac", "ac3", "eac3", "mp3"}}
	for _, codec := range []string{"ac3", "AC-3", "eac3", "E-AC-3"} {
		if !remuxAudioCopySafe(codec, caps) {
			t.Errorf("remuxAudioCopySafe(%q) = false; a plain claim should be enough for the AC-3 family", codec)
		}
	}
	// DTS and TrueHD still need sink evidence.
	for _, codec := range []string{"dts", "DTS-HD MA", "truehd", "TrueHD"} {
		if remuxAudioCopySafe(codec, ClientCapabilities{CodecsAudio: []string{"dts", "truehd"}}) {
			t.Errorf("remuxAudioCopySafe(%q) = true on a plain claim; these need declared passthrough", codec)
		}
	}
}
