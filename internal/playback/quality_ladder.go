package playback

import (
	"strings"

	"github.com/prairie-server/prairie-server/internal/models"
)

// SourceVideoHeight is the probed height of a file's primary video stream, or 0
// when the probe recorded no video track.
//
// 0 is a meaningful answer, not an error: QualityLadderFor treats it as "offer
// every rung", on the grounds that hiding a source's own rung is worse than
// showing one option too many.
func SourceVideoHeight(file *models.MediaFile) int {
	if file == nil || len(file.VideoTracks) == 0 {
		return 0
	}
	return file.VideoTracks[0].Height
}

// QualityRung is one selectable step on the transcode ladder.
//
// BitrateKbps is what the encoder is capped to when this rung is chosen, and is
// also the figure the advice engine compares measured delivery against. The
// values are deliberately a little generous: a rung is only recommended when
// delivery clears it with headroom, so over-stating cost makes the engine
// downshift sooner rather than let a client stall.
type QualityRung struct {
	Resolution  string `json:"resolution"`
	Height      int    `json:"height"`
	BitrateKbps int    `json:"bitrate_kbps"`
	Label       string `json:"label"`
}

// qualityLadder is ordered highest-first, which is the order clients render and
// the order the step helpers walk.
//
// Every Resolution here must round-trip through NormalizeQualityV3 unchanged.
// That function is the wire contract shared by the plan request, the replan
// request and the stored profile preference, so a rung it does not recognize
// normalizes to "auto" and silently stops being selectable. Adding a rung means
// teaching NormalizeQualityV3 first -- which is why there is no 1440p or 360p
// step, not because the ladder would not benefit from them.
var qualityLadder = []QualityRung{
	{Resolution: resolution2160p, Height: 2160, BitrateKbps: 20000, Label: "4K"},
	{Resolution: resolution1080p, Height: 1080, BitrateKbps: 6000, Label: "1080p"},
	{Resolution: "720p", Height: 720, BitrateKbps: 3000, Label: "720p"},
	{Resolution: "480p", Height: 480, BitrateKbps: 1500, Label: "480p"},
}

// QualityLadderFor returns the rungs offerable for a source of the given height,
// highest first.
//
// Rungs above the source are omitted rather than clamped: upscaling spends
// encode time and bitrate reproducing detail the file does not contain, and
// offering "4K" for a 1080p source misrepresents what the viewer would get. A
// source shorter than the lowest rung still yields that rung, so the ladder is
// never empty and Auto always has somewhere to go.
//
// sourceHeight <= 0 means the probe reported no dimensions; the full ladder is
// returned, because wrongly hiding the source's own rung is worse than showing
// one option too many.
func QualityLadderFor(sourceHeight int) []QualityRung {
	if sourceHeight <= 0 {
		return append([]QualityRung(nil), qualityLadder...)
	}

	out := make([]QualityRung, 0, len(qualityLadder))
	for _, rung := range qualityLadder {
		// A small tolerance keeps sources mastered slightly off a rung (1080p
		// content at 1072 lines, say) from losing their own native rung.
		if rung.Height <= sourceHeight+8 {
			out = append(out, rung)
		}
	}
	if len(out) == 0 {
		out = append(out, qualityLadder[len(qualityLadder)-1])
	}
	return out
}

// RungFor resolves a ladder token ("1080p", "4k", "uhd") to its rung.
//
// "auto" and "original" are deliberately not rungs: they are modes, and neither
// names a bitrate the advice engine could compare a measurement against.
func RungFor(resolution string) (QualityRung, bool) {
	normalized, changed := NormalizeQualityV3(resolution)
	if changed {
		return QualityRung{}, false
	}
	for _, rung := range qualityLadder {
		if strings.EqualFold(rung.Resolution, normalized) {
			return rung, true
		}
	}
	return QualityRung{}, false
}

// StepDownFrom returns the next rung below resolution that the source can offer.
//
// Reports false at the bottom of the ladder, which the advice engine reads as
// "nothing left to give". Recommending the rung already in use would spend a
// replan slot and a rebuffer to arrive exactly where playback already is.
func StepDownFrom(resolution string, sourceHeight int) (QualityRung, bool) {
	ladder := QualityLadderFor(sourceHeight)
	for i, rung := range ladder {
		if strings.EqualFold(rung.Resolution, resolution) && i+1 < len(ladder) {
			return ladder[i+1], true
		}
	}
	return QualityRung{}, false
}

// StepUpFrom returns the next rung above resolution, bounded by the source.
func StepUpFrom(resolution string, sourceHeight int) (QualityRung, bool) {
	ladder := QualityLadderFor(sourceHeight)
	for i, rung := range ladder {
		if strings.EqualFold(rung.Resolution, resolution) && i > 0 {
			return ladder[i-1], true
		}
	}
	return QualityRung{}, false
}

// HighestRungWithin returns the best rung whose bitrate fits budgetKbps.
//
// Lets a session be placed on the ladder directly from a measured throughput
// instead of stepping down one rung per rebuffer, so a client on a badly
// congested link reaches something playable in one move. Reports false when even
// the lowest rung exceeds the budget, leaving the caller to choose between the
// floor and giving up.
func HighestRungWithin(budgetKbps int, sourceHeight int) (QualityRung, bool) {
	for _, rung := range QualityLadderFor(sourceHeight) {
		if rung.BitrateKbps <= budgetKbps {
			return rung, true
		}
	}
	return QualityRung{}, false
}
