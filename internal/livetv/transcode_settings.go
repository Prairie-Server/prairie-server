package livetv

import (
	"context"
	"strings"
)

// Live TV play-method policy.
const (
	// PlayMethodAuto lets client capabilities decide copy vs transcode.
	PlayMethodAuto = "auto"
	// PlayMethodCopy always hands the broadcast streams through untouched.
	PlayMethodCopy = "copy"
	// PlayMethodTranscode always re-encodes, whatever the client claims.
	PlayMethodTranscode = "transcode"
)

// DefaultLiveTranscodeLeadSegments is how many segments a re-encoding session
// produces before its playlist is advertised. A transcode's output rate wobbles
// with scene complexity, and a player that starts with one segment of buffer
// stalls on the first dip.
const DefaultLiveTranscodeLeadSegments = 3

// TranscodeSettings is the operator-facing policy for live sessions. Values are
// read per tune so an admin change applies to the next channel start without a
// server restart.
type TranscodeSettings struct {
	// HWAccel is "auto", "nvenc", "qsv", "vaapi", or "none".
	HWAccel string
	// HWDecode is "auto", "on", or "off" — hardware decode of the broadcast.
	HWDecode string
	// EncoderPreset is "low_latency", "balanced", or "quality".
	EncoderPreset string
	// FrameRateCap is "source", "60", or "30".
	FrameRateCap string
	// MaxResolution is "source", "1080p", or "720p".
	MaxResolution string
	// PlayMethod forces copy or transcode; "auto" follows client capabilities.
	PlayMethod string
	// MaxTranscodes bounds concurrent encoding sessions; 0 uses the default and
	// a negative value disables the limit.
	MaxTranscodes int
}

// SettingsProvider supplies the current live transcode policy.
type SettingsProvider func(ctx context.Context) TranscodeSettings

// frameRateCap normalizes the configured cap into what ffmpeg should receive.
// "source" (the default) keeps the broadcast frame rate — capping is a fallback
// for hardware that cannot keep up, not a default.
func (s TranscodeSettings) frameRateCap() string {
	switch strings.ToLower(strings.TrimSpace(s.FrameRateCap)) {
	case "30":
		return "30"
	case "60":
		return "60"
	default:
		return ""
	}
}

// resolutionCap normalizes the configured ceiling. "source" keeps broadcast
// resolution; anything at or above it would upscale, so only real reductions
// are honored.
func (s TranscodeSettings) resolutionCap() string {
	return liveDownscaleResolution(s.MaxResolution)
}

// applyTo folds operator policy into a capability-derived plan.
func (s TranscodeSettings) applyTo(plan StreamPlan) StreamPlan {
	switch strings.ToLower(strings.TrimSpace(s.PlayMethod)) {
	case PlayMethodCopy:
		plan.VideoCodec = "copy"
		plan.AudioCodec = "copy"
		plan.AudioChannels = 0
		plan.MaxResolution = ""
		return plan
	case PlayMethodTranscode:
		plan.VideoCodec = "h264"
		if plan.AudioCodec == "" || strings.EqualFold(plan.AudioCodec, "copy") {
			plan.AudioCodec = "aac"
			if plan.AudioChannels == 0 {
				plan.AudioChannels = 2
			}
		}
	}
	// An operator ceiling only ever lowers what the client asked for.
	if capped := s.resolutionCap(); capped != "" && plan.TranscodesVideo() {
		if plan.MaxResolution == "" || resolutionRank(capped) < resolutionRank(plan.MaxResolution) {
			plan.MaxResolution = capped
		}
	}
	return plan
}

// resolutionRank orders resolution labels so the smaller ceiling wins.
func resolutionRank(resolution string) int {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "328p":
		return 1
	case "420p":
		return 2
	case "480p":
		return 3
	case "720p":
		return 4
	case "1080p":
		return 5
	case "2160p":
		return 6
	default:
		return 99
	}
}
