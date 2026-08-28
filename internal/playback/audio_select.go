package playback

import (
	"strings"

	"github.com/prairie-server/prairie-server/internal/lang"
	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/userstore"
)

// OriginalLanguageSentinel is the value stored in audio language preference
// columns to mean "use the media item's original language." It is resolved
// to a concrete language code in the playback handler before reaching
// SelectAudioTrack.
const OriginalLanguageSentinel = "original"

// AudioTrackPreference holds a per-series audio track preference.
type AudioTrackPreference struct {
	AudioTrackIndex int
	AudioLanguage   string
	TrackSignature  *userstore.AudioTrackSignature
}

func langMatch(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return lang.Canonical(a) == lang.Canonical(b)
}

// SelectAudioTrack determines which audio track to use based on preferences.
//
// Priority:
// 1. Series preference exact track signature
// 2. Series preference index (if track exists at that index with matching language)
// 3. Series preference language (first track matching that language)
// 4. Profile preferred language (first track matching)
// 5. File's default track (first track with Default: true)
// 6. First track (index 0)
func SelectAudioTrack(tracks []models.AudioTrack, preferredLang string, seriesPref *AudioTrackPreference) int {
	if len(tracks) == 0 {
		return 0
	}

	// 1. Series preference: try exact signature match first.
	if seriesPref != nil {
		if idx := findExactAudioTrack(tracks, seriesPref.TrackSignature); idx >= 0 {
			return idx
		}

		// 2. Series preference: try exact index+language match.
		if seriesPref.AudioTrackIndex >= 0 && seriesPref.AudioTrackIndex < len(tracks) {
			if langMatch(tracks[seriesPref.AudioTrackIndex].Language, seriesPref.AudioLanguage) {
				return seriesPref.AudioTrackIndex
			}
		}

		// 3. Series preference: fall back to language match.
		if seriesPref.AudioLanguage != "" {
			for i, t := range tracks {
				if langMatch(t.Language, seriesPref.AudioLanguage) {
					return i
				}
			}
		}
	}

	// 4. Profile language preference.
	if preferredLang != "" {
		for i, t := range tracks {
			if langMatch(t.Language, preferredLang) {
				return i
			}
		}
	}

	// 5. File's default track.
	for i, t := range tracks {
		if t.Default {
			return i
		}
	}

	// 6. First track.
	return 0
}

// MatchAudioTrackAcrossVersions maps a selection made against one file's
// audio inventory onto another version of the same content. Track ordering is
// not stable across encodes, so carrying the raw ordinal can select a different
// language. Prefer the stable signature, then the selected language, and
// finally the effective file's default track.
func MatchAudioTrackAcrossVersions(
	requestedTracks []models.AudioTrack,
	effectiveTracks []models.AudioTrack,
	requestedIndex int,
) int {
	if len(effectiveTracks) == 0 {
		return 0
	}
	if len(requestedTracks) == 0 {
		return SelectAudioTrack(effectiveTracks, "", nil)
	}
	if requestedIndex < 0 || requestedIndex >= len(requestedTracks) {
		requestedIndex = SelectAudioTrack(requestedTracks, "", nil)
	}

	selected := requestedTracks[requestedIndex]
	return SelectAudioTrack(effectiveTracks, "", &AudioTrackPreference{
		AudioTrackIndex: requestedIndex,
		AudioLanguage:   selected.Language,
		TrackSignature:  AudioTrackSignatureFromTrack(selected),
	})
}

// BrowserSupportsAudioCodec returns true if the given audio codec can be
// played natively by web browsers without transcoding.
func BrowserSupportsAudioCodec(codec string) bool {
	switch strings.ToLower(codec) {
	case "aac", "mp3", "opus", "vorbis", "flac":
		return true
	default:
		return false
	}
}

// IsFragileLosslessAudioCodec reports codecs whose software decode path is
// unreliable under HLS remux/transcode — notably TrueHD/MLP (quant_step_size
// larger than huff_lsbs warnings that stall AAC muxing).
func IsFragileLosslessAudioCodec(codec string) bool {
	c := strings.ToLower(strings.TrimSpace(codec))
	if c == "" {
		return false
	}
	return strings.Contains(c, "truehd") || strings.Contains(c, "mlp")
}

// PreferTranscodeFriendlyAudioTrack picks a more reliable source track when
// re-encoding fragile lossless codecs (TrueHD/MLP) to AAC. Prefers a
// same-language AAC/EAC3/AC3 companion when present; otherwise returns the
// originally selected index so the caller can still force a stereo downmix.
func PreferTranscodeFriendlyAudioTrack(tracks []models.AudioTrack, selected int) int {
	if len(tracks) == 0 {
		return selected
	}
	if selected < 0 || selected >= len(tracks) {
		// Preserve the caller's ordinal when the inventory is incomplete —
		// remapping an out-of-range selection to 0 would silently change the
		// stream the client asked for.
		return selected
	}
	if !IsFragileLosslessAudioCodec(tracks[selected].Codec) {
		return selected
	}

	lang := tracks[selected].Language
	preferOrder := []string{"aac", "eac3", "ac3", "mp3", "opus"}

	pick := func(requireLang bool) int {
		for _, want := range preferOrder {
			for i, t := range tracks {
				if i == selected {
					continue
				}
				if requireLang && lang != "" && t.Language != "" && !langMatch(t.Language, lang) {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(t.Codec), want) {
					return i
				}
			}
		}
		return -1
	}

	if idx := pick(true); idx >= 0 {
		return idx
	}
	if idx := pick(false); idx >= 0 {
		return idx
	}
	return selected
}

// AudioTrackCodecAt returns the codec string for tracks[index], or empty when
// the inventory is missing / out of range.
func AudioTrackCodecAt(tracks []models.AudioTrack, index int) string {
	if index < 0 || index >= len(tracks) {
		return ""
	}
	return tracks[index].Codec
}

// AudioTrackChannelsAt returns the channel count for tracks[index], or 0 when
// the inventory is missing / out of range.
//
// 0 is a meaningful answer: EffectiveAudioChannels treats it as "the source
// count is unknown" and falls back to the client's ceiling alone, rather than
// assuming mono and collapsing a surround mix.
func AudioTrackChannelsAt(tracks []models.AudioTrack, index int) int {
	if index < 0 || index >= len(tracks) {
		return 0
	}
	return tracks[index].Channels
}

// SelectClientPlayableAudioTrack finds the best track the client can decode
// itself, so an unplayable default can be answered by choosing a different
// track rather than by re-encoding.
//
// This is the difference between "the TV cannot play this audio" and "the TV
// cannot play this file". A Blu-ray remux typically ships a lossless default
// (TrueHD/DTS-HD) alongside a lossy companion in the same language, and the
// companion is usually both decodable and multichannel. Copying it beats
// re-encoding the lossless track on every axis: no CPU, no quality loss, and
// surround survives instead of being downmixed to stereo.
//
// Preference order, strongest first:
//
//  1. The client claims the codec. A track it cannot decode is no use however
//     good it is.
//  2. Same language as the originally selected track. Correct language in
//     stereo beats the wrong language in 5.1.
//  3. Most channels, bounded by maxChannels. A client that declared a 5.1
//     ceiling gains nothing from a 7.1 track and may fail on it outright.
//  4. Lowest index, so the choice is deterministic for equal candidates.
//
// Reports false when no track qualifies, leaving the caller to fall back to
// re-encoding. maxChannels <= 0 means the client declared no ceiling, in which
// case channel count is still preferred but nothing is excluded for it -- a
// decodable track is better than a re-encode either way.
func SelectClientPlayableAudioTrack(
	tracks []models.AudioTrack,
	selected int,
	claimedCodecs []string,
	maxChannels int,
) (int, bool) {
	if len(tracks) == 0 || len(claimedCodecs) == 0 {
		return selected, false
	}

	preferredLang := ""
	if selected >= 0 && selected < len(tracks) {
		preferredLang = tracks[selected].Language
	}

	best := -1
	bestLangMatch := false
	bestChannels := -1
	for i, track := range tracks {
		if !audioCodecClaimed(claimedCodecs, track.Codec) {
			continue
		}
		// A track above the client's ceiling is exactly what this exists to
		// avoid handing over.
		if maxChannels > 0 && track.Channels > maxChannels {
			continue
		}

		langMatch := preferredLang == "" || track.Language == "" ||
			langMatch(track.Language, preferredLang)
		if best < 0 ||
			(langMatch && !bestLangMatch) ||
			(langMatch == bestLangMatch && track.Channels > bestChannels) {
			best, bestLangMatch, bestChannels = i, langMatch, track.Channels
		}
	}
	if best < 0 {
		return selected, false
	}
	return best, true
}

// audioCodecClaimed reports whether a capability list names a codec, comparing
// both sides through normalizeAudioCodecForRemux.
func audioCodecClaimed(list []string, codec string) bool {
	normalized := normalizeAudioCodecForRemux(codec)
	if normalized == "" {
		return false
	}
	for _, candidate := range list {
		if normalizeAudioCodecForRemux(candidate) == normalized {
			return true
		}
	}
	return false
}

func normalizeAudioCodecForRemux(codec string) string {
	normalized := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(codec)))
	switch normalized {
	case "aac", "mp4a", "aaclc", "heaac":
		return "aac"
	case "ac3", "dolbydigital":
		return "ac3"
	case "eac3", "ec3", "dolbydigitalplus", "ddp":
		return "eac3"
	case "dts", "dtshd", "dtshdma", "dtsx":
		return "dts"
	case "truehd", "mlp":
		return "truehd"
	default:
		return normalized
	}
}
