package playback

import (
	"strings"

	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/userstore"
)

// AudioTrackSignatureFromTrack converts a probed audio track into the stable
// signature persisted for series-level sticky audio preferences.
func AudioTrackSignatureFromTrack(track models.AudioTrack) *userstore.AudioTrackSignature {
	sig := &userstore.AudioTrackSignature{
		Language:      strings.TrimSpace(track.Language),
		Title:         strings.TrimSpace(track.Title),
		EmbeddedTitle: strings.TrimSpace(track.EmbeddedTitle),
		Codec:         strings.TrimSpace(track.Codec),
		Layout:        strings.TrimSpace(track.Layout),
		Channels:      track.Channels,
	}
	if sig.IsZero() {
		return nil
	}
	return sig
}

func findExactAudioTrack(tracks []models.AudioTrack, sig *userstore.AudioTrackSignature) int {
	if sig == nil || sig.IsZero() {
		return -1
	}
	for i, track := range tracks {
		if audioTrackMatchesSignature(track, sig) {
			return i
		}
	}
	return -1
}

func audioTrackMatchesSignature(track models.AudioTrack, sig *userstore.AudioTrackSignature) bool {
	if sig == nil || sig.IsZero() {
		return false
	}
	return langMatch(track.Language, sig.Language) &&
		trackStringEqual(track.Title, sig.Title) &&
		trackStringEqual(track.EmbeddedTitle, sig.EmbeddedTitle) &&
		trackStringEqual(track.Codec, sig.Codec) &&
		trackStringEqual(track.Layout, sig.Layout) &&
		track.Channels == sig.Channels
}

func trackStringEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
