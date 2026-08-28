package livetv

import "strings"

// ClientCapabilities is the subset of playback capabilities a Live TV client
// reports when it starts a session. It mirrors the codec fields of the VOD
// start-playback request so both paths speak the same vocabulary.
type ClientCapabilities struct {
	CodecsVideo []string `json:"codecs_video"`
	CodecsAudio []string `json:"codecs_audio"`
	// AudioPassthroughCodecs are formats the client can hand to a receiver
	// untouched even when it cannot decode them itself.
	AudioPassthroughCodecs []string `json:"audio_passthrough_codecs"`
	// MaxAudioChannels caps the delivered channel count. 0 means the client did
	// not say, which the planner treats as stereo when it has to re-encode.
	MaxAudioChannels int `json:"max_audio_channels"`
	// MaxResolution ("1080p", "720p", …) optionally scales an encoded stream
	// down. Empty keeps the broadcast resolution.
	MaxResolution string `json:"max_resolution"`
}

// Declared reports whether the client actually described itself. Clients that
// predate capability reporting send nothing, and those are all native players
// that were already decoding the broadcast codecs directly.
func (c ClientCapabilities) Declared() bool {
	return len(c.CodecsVideo) > 0 || len(c.CodecsAudio) > 0
}

// SourceCodecs describes what a tuner is expected to put on the wire.
type SourceCodecs struct {
	Video    string
	Audio    string
	Channels int
}

// BroadcastSourceCodecs is what an ATSC 1.0 tuner emits: MPEG-2 video with
// AC-3 audio, up to 5.1. Prairie does not probe the tuner before tuning (that
// would burn a tuner and add seconds to every channel change), so this is the
// assumed source. Assuming broadcast codecs is the safe direction: a client
// that cannot decode them gets a transcode it can always play, while a client
// that can decode them keeps the zero-cost copy.
var BroadcastSourceCodecs = SourceCodecs{Video: "mpeg2video", Audio: "ac3", Channels: 6}

// StreamPlan is the per-stream copy-or-encode decision for one live session.
type StreamPlan struct {
	// VideoCodec is "copy" or a target codec such as "h264".
	VideoCodec string
	// AudioCodec is "copy" or a target codec such as "aac".
	AudioCodec string
	// AudioChannels caps the re-encoded channel count; 0 means stereo.
	AudioChannels int
	// MaxResolution optionally scales an encoded video stream down.
	MaxResolution string
}

// Transcodes reports whether the plan re-encodes either stream, which is what
// makes a live session expensive enough to gate on concurrency.
func (p StreamPlan) Transcodes() bool {
	return p.TranscodesVideo() || !strings.EqualFold(p.AudioCodec, "copy")
}

// TranscodesVideo reports whether the plan re-encodes video.
func (p StreamPlan) TranscodesVideo() bool {
	return !strings.EqualFold(p.VideoCodec, "copy")
}

// PlanLiveStream decides what the bridge must re-encode for one client.
//
// Browsers are the reason this exists: MSE cannot decode MPEG-2 video or AC-3
// audio, so a copy-only bridge hands Chrome a black screen with no sound. Every
// stream the client did not claim support for is re-encoded to something it can
// decode; everything else is copied so native TV players keep the cheap path.
func PlanLiveStream(caps ClientCapabilities, source SourceCodecs) StreamPlan {
	plan := StreamPlan{VideoCodec: "copy", AudioCodec: "copy"}
	if !caps.Declared() {
		return plan
	}

	if !supportsCodec(caps.CodecsVideo, source.Video) {
		plan.VideoCodec = "h264"
		plan.MaxResolution = liveDownscaleResolution(caps.MaxResolution)
	}

	audioSupported := supportsCodec(caps.CodecsAudio, source.Audio) ||
		supportsCodec(caps.AudioPassthroughCodecs, source.Audio)
	if !audioSupported {
		plan.AudioCodec = "aac"
		plan.AudioChannels = plannedAudioChannels(caps, source)
	}

	return plan
}

// plannedAudioChannels keeps surround only when the client asked for it and the
// broadcast carries it. Browsers report nothing here and get a stereo downmix,
// which is what a 5.1 AC-3 broadcast needs to be usable on laptop speakers.
func plannedAudioChannels(caps ClientCapabilities, source SourceCodecs) int {
	sourceChannels := source.Channels
	if sourceChannels <= 0 {
		sourceChannels = 2
	}
	wanted := caps.MaxAudioChannels
	if wanted <= 0 {
		return 2
	}
	if wanted > sourceChannels {
		wanted = sourceChannels
	}
	if wanted < 2 {
		return 2
	}
	return wanted
}

// liveDownscaleResolution keeps only caps that are actually below broadcast
// resolution. The scale filters pin an exact output height, so honoring a
// "1080p" cap on a 720p broadcast would upscale the picture — burning encoder
// time to make it blurrier.
func liveDownscaleResolution(maxResolution string) string {
	switch strings.ToLower(strings.TrimSpace(maxResolution)) {
	case "720p", "480p", "420p", "328p":
		return strings.ToLower(strings.TrimSpace(maxResolution))
	default:
		return ""
	}
}

func supportsCodec(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), wanted) {
			return true
		}
	}
	return false
}
