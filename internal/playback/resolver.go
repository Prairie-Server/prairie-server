// Package playback provides play method resolution, streaming, transcoding,
// and session management for Prairie.
package playback

import (
	"slices"
	"strings"

	"github.com/prairie-server/prairie-server/internal/access"
	"github.com/prairie-server/prairie-server/internal/models"
)

// PlayMethod represents how a media file will be streamed.
type PlayMethod string

const (
	PlayDirect    PlayMethod = "direct"
	PlayRemux     PlayMethod = "remux"
	PlayTranscode PlayMethod = "transcode"
)

// ClientCapabilities describes what the client can play natively.
//
// AudioPassthroughCodecs are codecs the connected audio sink can decode bit-
// exact (e.g. an HDMI AVR accepting EAC3/Atmos). They are treated as supported
// audio codecs for resolution purposes so we can stream-copy surround audio
// instead of downmixing+re-encoding to AAC. Distinct from CodecsAudio, which
// describes what the client itself can decode.
type ClientCapabilities struct {
	CodecsVideo            []string `json:"codecs_video"` // e.g., h264, hevc, av1
	CodecsAudio            []string `json:"codecs_audio"` // e.g., aac, opus, flac
	AudioPassthroughCodecs []string `json:"audio_passthrough_codecs,omitempty"`
	Containers             []string `json:"containers"`     // e.g., mp4, webm, mkv
	MaxResolution          string   `json:"max_resolution"` // e.g., 1080p, 2160p
	// MaxAudioChannels caps the layout a re-encode may target (6 for a 5.1
	// panel, 8 for 7.1). 0 means the client did not say, which keeps the stereo
	// downmix. See EffectiveAudioChannels.
	MaxAudioChannels int  `json:"max_audio_channels,omitempty"`
	HDR              bool `json:"hdr"`
}

// AdminSettings controls server-side playback constraints.
type AdminSettings struct {
	TranscodeEnabled bool
	Allow4KTranscode bool
}

// PlayDecision is the result of resolving how to play a file.
type PlayDecision struct {
	Method         PlayMethod
	File           *models.MediaFile
	Reason         string // human-readable explanation
	TranscodeAudio bool   // true when remuxing should transcode audio to AAC
}

type VersionSelectionFilter struct {
	EditionKey           string
	PresentationKind     string
	PresentationGroupKey string
}

// Resolve determines the play method for a given file and client capabilities.
// Returns direct if client supports codec+container, remux if codec matches
// but container doesn't, transcode otherwise.
func Resolve(file *models.MediaFile, caps ClientCapabilities, settings AdminSettings) *PlayDecision {
	// Check if client supports the video codec.
	videoOK := containsStr(caps.CodecsVideo, file.CodecVideo)
	// Audio is considered OK if the client can decode the codec itself OR its
	// sink can passthrough it. Passthrough lets us stream-copy surround audio
	// (EAC3/AC3/DTS/TrueHD) to HDMI AVRs instead of re-encoding to stereo AAC.
	audioOK := containsStr(caps.CodecsAudio, file.CodecAudio) ||
		containsStr(caps.AudioPassthroughCodecs, file.CodecAudio)
	// The same question for the remux route, but alias-tolerant. Probes and
	// client tables spell these codecs differently ("E-AC-3" vs "eac3" vs "ec3"),
	// and on the remux path a spelling mismatch is not harmless: it skips the
	// container-safety decision entirely and re-encodes audio that the client had
	// in fact claimed. audioOK keeps its raw comparison because it also gates
	// direct play, where a wider notion of "supported" would change more routes
	// than this fix intends.
	remuxAudioOK := audioCodecClaimed(caps.CodecsAudio, file.CodecAudio) ||
		audioCodecClaimed(caps.AudioPassthroughCodecs, file.CodecAudio)
	// Check if client supports the container.
	containerOK := containsStr(caps.Containers, file.Container)

	// Check resolution constraint.
	if !resolutionFits(file.Resolution, caps.MaxResolution) {
		if !settings.TranscodeEnabled {
			return &PlayDecision{
				Method: PlayDirect,
				File:   file,
				Reason: "file resolution exceeds client max but transcode disabled; attempting direct",
			}
		}
		// Need transcode to lower resolution.
		return &PlayDecision{
			Method: PlayTranscode,
			File:   file,
			Reason: "file resolution exceeds client max resolution",
		}
	}

	// Case 1: Client supports codec + container → direct play.
	if videoOK && audioOK && containerOK {
		return &PlayDecision{
			Method: PlayDirect,
			File:   file,
			Reason: "client supports all codecs and container",
		}
	}

	// A copy-unsafe source (H.264 with conflicting in-band PPS) cannot take a
	// video stream-copy route: the remux would desync strict decoders. Force it
	// past the remux cases into a full video transcode. Direct play of the
	// original file (Case 1) stays available — decoders that reparse in-band
	// parameter sets handle the original container fine.
	copyUnsafe := videoCopyUnsafeFile(file)

	// Case 2: Client supports codecs but not container → remux.
	//
	// The audio still has to survive the new container. Direct play (Case 1)
	// hands over the original file, where a flat codecs_audio claim is the right
	// question to ask; a remux rewrites into MP4, where it is not. See
	// remuxAudioCopySafe.
	if videoOK && remuxAudioOK && !containerOK && !copyUnsafe {
		if !remuxAudioCopySafe(file.CodecAudio, caps) {
			return &PlayDecision{
				Method:         PlayRemux,
				File:           file,
				TranscodeAudio: true,
				Reason:         "client supports codecs but not container; remuxing with audio transcode to AAC (source audio is not reliably carried in MP4)",
			}
		}
		return &PlayDecision{
			Method: PlayRemux,
			File:   file,
			Reason: "client supports codecs but not container; remuxing",
		}
	}

	// Case 3: Video OK but audio codec unsupported → remux with audio transcode.
	// This is much cheaper than a full video transcode.
	if videoOK && !audioOK && !copyUnsafe {
		return &PlayDecision{
			Method:         PlayRemux,
			File:           file,
			TranscodeAudio: true,
			Reason:         "client supports video codec but not audio; remuxing with audio transcode to AAC",
		}
	}

	// Case 4: Client can't play video codec → full transcode.
	if !settings.TranscodeEnabled {
		return &PlayDecision{
			Method: PlayDirect,
			File:   file,
			Reason: "transcode needed but disabled; attempting direct play",
		}
	}

	return &PlayDecision{
		Method: PlayTranscode,
		File:   file,
		Reason: "client cannot play video codec; transcoding",
	}
}

// SelectVersion chooses the best file version from a list of available files
// based on client capabilities and admin settings.
// Priority: direct-playable > remux > transcode, then highest quality, then smallest file.
func SelectVersion(files []*models.MediaFile, caps ClientCapabilities, settings AdminSettings) (*PlayDecision, error) {
	return SelectVersionFiltered(files, caps, settings, VersionSelectionFilter{})
}

// SelectVersionFiltered chooses the best interchangeable file version within
// one edition/presentation group.
func SelectVersionFiltered(
	files []*models.MediaFile,
	caps ClientCapabilities,
	settings AdminSettings,
	filter VersionSelectionFilter,
) (*PlayDecision, error) {
	if len(files) == 0 {
		return nil, ErrNoVersions
	}

	candidates := files
	if filter.EditionKey != "" || filter.PresentationKind != "" || filter.PresentationGroupKey != "" {
		filtered := make([]*models.MediaFile, 0, len(files))
		for _, f := range files {
			if filter.EditionKey != "" && f.EditionKey != filter.EditionKey {
				continue
			}
			if filter.PresentationKind != "" && f.PresentationKind != filter.PresentationKind {
				continue
			}
			if filter.PresentationGroupKey != "" && f.PresentationGroupKey != filter.PresentationGroupKey {
				continue
			}
			filtered = append(filtered, f)
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}

	var directFiles, remuxFiles, transcodeFiles []*PlayDecision

	for _, f := range candidates {
		// Filter by client max resolution.
		if !resolutionFits(f.Resolution, caps.MaxResolution) {
			// If 4K transcoding disabled and this is 4K, skip entirely.
			if is4K(f.Resolution) && !settings.Allow4KTranscode {
				continue
			}
		}

		decision := Resolve(f, caps, settings)

		switch decision.Method {
		case PlayDirect:
			directFiles = append(directFiles, decision)
		case PlayRemux:
			remuxFiles = append(remuxFiles, decision)
		case PlayTranscode:
			transcodeFiles = append(transcodeFiles, decision)
		}
	}

	// Prefer direct > remux > transcode.
	if len(directFiles) > 0 {
		return bestQuality(directFiles), nil
	}
	if len(remuxFiles) > 0 {
		return bestQuality(remuxFiles), nil
	}
	if len(transcodeFiles) > 0 {
		return bestQuality(transcodeFiles), nil
	}

	// Fallback: use first file with direct play.
	return &PlayDecision{
		Method: PlayDirect,
		File:   candidates[0],
		Reason: "no compatible version found; falling back to first file",
	}, nil
}

// bestQuality picks the highest quality file. Among ties, picks smallest file.
func bestQuality(decisions []*PlayDecision) *PlayDecision {
	best := decisions[0]
	for _, d := range decisions[1:] {
		if access.CompareQuality(d.File.Resolution, best.File.Resolution) > 0 {
			best = d
		} else if access.CompareQuality(d.File.Resolution, best.File.Resolution) == 0 && d.File.FileSize < best.File.FileSize {
			best = d
		}
	}
	return best
}

// resolutionOrder returns a numeric value for sorting resolutions.
func resolutionOrder(res string) int {
	switch {
	case access.CompareQuality(res, "4320p") == 0:
		return 5
	case access.CompareQuality(res, resolution2160p) == 0:
		return 4
	case access.CompareQuality(res, resolution1080p) == 0:
		return 3
	case access.CompareQuality(res, "720p") == 0:
		return 2
	case access.CompareQuality(res, "480p") == 0:
		return 1
	default:
		return 0
	}
}

// resolutionFits checks if the file resolution fits within the client's max.
func resolutionFits(fileRes, maxRes string) bool {
	if maxRes == "" {
		return true // no constraint
	}
	return resolutionOrder(fileRes) <= resolutionOrder(maxRes)
}

// is4K returns true if the resolution is 2160p or higher.
func is4K(res string) bool {
	return access.CompareQuality(res, resolution2160p) >= 0
}

// containsStr checks if a slice contains a string.
func containsStr(slice []string, s string) bool {
	return slices.Contains(slice, s)
}

// videoCopyUnsafeFile reports whether the file's video stream cannot be safely
// stream-copied into an avc1/fMP4 segment. It is set once by the multi-PPS
// bitstream scan (H.264 sources that redefine a pic_parameter_set_id in-band
// with conflicting content). Scan failures also disable copy for the current
// decision while remaining eligible for retry on a later request.
func videoCopyUnsafeFile(file *models.MediaFile) bool {
	if file == nil || len(file.VideoTracks) == 0 {
		return false
	}
	track := file.VideoTracks[0]
	return track.VideoCopyUnsafe || (track.MultiplePPS != nil && *track.MultiplePPS)
}

// remuxAudioCopySafe reports whether a source audio codec can be carried into
// the remux container (MP4) untouched, given what the client actually told us.
//
// A remux changes the container under the client, and a flat codecs_audio entry
// only claims "I can decode this codec", not "I can decode it in MP4". Those are
// different claims, and treating them as one is what produced a 4K AV1 stream
// that played video with no sound: the client listed ac3 (an unprobed static
// default), the source had an AC3 companion track, so audio was copied into MP4
// and the TV could not decode it -- reporting "not supported audio codec but
// video can be played".
//
// AAC is unconditionally safe: it is MP4's native audio codec and the format
// every client that accepts MP4 at all can decode.
//
// AC-3, E-AC-3, DTS and TrueHD are spec-legal in MP4 (and genuinely work on
// Apple platforms and through an HDMI sink) but unreliable elsewhere, so they
// need evidence beyond a flat codec list. AudioPassthroughCodecs is exactly that
// evidence: it means a sink decodes the codec bit-exact, which is a claim about
// delivery rather than about a decoder existing somewhere. Without it the audio
// is converted to AAC -- a cheap, single-threaded encode next to a video copy,
// and far better than silence.
//
// Anything else falls through to conversion. Being wrong in this direction costs
// a small amount of CPU; being wrong the other way costs the viewer their audio.
func remuxAudioCopySafe(codec string, caps ClientCapabilities) bool {
	switch normalizeAudioCodecForRemux(codec) {
	case "aac":
		return true
	case "ac3", "eac3":
		// Decodable from MP4 wherever the codec itself is decodable: Samsung
		// reports AC-3 on all models and E-AC-3 on 2018+, and Apple platforms
		// handle both. A plain claim is therefore evidence enough, and requiring
		// sink passthrough here only forced needless re-encodes.
		return audioCodecClaimed(caps.CodecsAudio, codec) ||
			audioCodecClaimed(caps.AudioPassthroughCodecs, codec)
	case "dts", "truehd":
		// Genuinely fragile in MP4: DTS-in-MP4 is rare and Tizen reports no DTS
		// decode at all, while TrueHD in MP4 is unsupported essentially
		// everywhere. Only a declared sink passthrough justifies a copy.
		return audioCodecClaimed(caps.AudioPassthroughCodecs, codec)
	default:
		return false
	}
}

// audioCodecClaimed reports whether a capability list names a codec, comparing
// both sides through normalizeAudioCodecForRemux.
//
// Both ends of this comparison are free-form. The source codec comes from a
// probe ("E-AC-3", "DTS-HD MA", "eac3" depending on the muxer), and the client
// list is hand-written or table-generated ("eac3", "ec3", "Dolby Digital Plus").
// Comparing them raw means a client that canonically claims ["eac3"] fails to
// match a source labeled "E-AC-3" and has its declared sink passthrough
// silently downmixed to AAC -- the exact promise this list exists to keep.
//
// Scoped to the remux decision on purpose. Direct play hands over the original
// file and its raw comparison decides a wider set of routes, so widening what
// counts as a match there is a separate change with its own blast radius.
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
