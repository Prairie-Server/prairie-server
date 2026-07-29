package playback

import (
	"strings"

	"github.com/prairie-server/prairie-server/internal/models"
)

// QualityRung is one selectable step on the transcode ladder.
//
// ID is the stable identifier clients key their selection on. Resolution and
// BitrateKbps are what a client sends as target_resolution and
// target_bitrate_kbps when starting a transcode, so a rung fully describes
// itself and a client never has to invent either value.
//
// Several rungs share a Resolution at different bitrates (the "High" variants),
// so Resolution alone does not identify a rung -- match on ID.
type QualityRung struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Resolution  string `json:"resolution"`
	Height      int    `json:"height"`
	BitrateKbps int    `json:"bitrate_kbps"`
}

// qualityLadder is ordered highest-first, which is the order clients render and
// the order the step helpers walk.
//
// Every Resolution here must be one resolutionToScale understands, or an encode
// at that rung silently produces no scale filter and runs at source resolution.
// A test pins that.
//
// Note this is a target_resolution contract, not a quality_preference one.
// NormalizeQualityV3 governs the stored profile preference and the plan/replan
// requests, and deliberately knows a smaller set of tokens. Conflating the two
// is why an earlier version of this ladder wrongly dropped the High variants and
// 420p, which the web player has offered all along.
var qualityLadder = []QualityRung{
	{ID: "2160p", Label: "4K", Resolution: resolution2160p, Height: 2160, BitrateKbps: 20000},
	{ID: "1080p-high", Label: "1080p High", Resolution: resolution1080p, Height: 1080, BitrateKbps: 10000},
	{ID: "1080p", Label: "1080p", Resolution: resolution1080p, Height: 1080, BitrateKbps: 6000},
	{ID: "720p-high", Label: "720p High", Resolution: "720p", Height: 720, BitrateKbps: 4000},
	{ID: "720p", Label: "720p", Resolution: "720p", Height: 720, BitrateKbps: 2000},
	{ID: "480p", Label: "480p", Resolution: "480p", Height: 480, BitrateKbps: 1500},
	{ID: "420p", Label: "420p", Resolution: "420p", Height: 420, BitrateKbps: 720},
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

// RungByID resolves a rung by its stable identifier.
func RungByID(id string) (QualityRung, bool) {
	for _, rung := range qualityLadder {
		if strings.EqualFold(rung.ID, id) {
			return rung, true
		}
	}
	return QualityRung{}, false
}

// RungForSession resolves the rung a running session is playing, from its target
// resolution and bitrate cap.
//
// Bitrate disambiguates rungs sharing a resolution. A cap matching no rung
// exactly resolves to the nearest rung at that resolution, so advice still works
// for a session started before a ladder change or by a client that chose its own
// bitrate.
//
// Reports false when there is no target resolution: "auto", "original" and
// direct play have no rung to step from, and a direct play is the source file
// itself, where reducing quality is a plan decision rather than a ladder move.
func RungForSession(resolution string, bitrateKbps int) (QualityRung, bool) {
	if strings.TrimSpace(resolution) == "" {
		return QualityRung{}, false
	}

	var best QualityRung
	found := false
	for _, rung := range qualityLadder {
		if !strings.EqualFold(rung.Resolution, resolution) {
			continue
		}
		if !found || absInt(rung.BitrateKbps-bitrateKbps) < absInt(best.BitrateKbps-bitrateKbps) {
			best, found = rung, true
		}
	}
	return best, found
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// StepDownFrom returns the next rung below the given one that the source can
// offer.
//
// Reports false at the bottom of the ladder, which the advice engine reads as
// "nothing left to give". Recommending the rung already in use would spend a
// replan slot and a rebuffer to arrive exactly where playback already is.
func StepDownFrom(id string, sourceHeight int) (QualityRung, bool) {
	ladder := QualityLadderFor(sourceHeight)
	for i, rung := range ladder {
		if strings.EqualFold(rung.ID, id) && i+1 < len(ladder) {
			return ladder[i+1], true
		}
	}
	return QualityRung{}, false
}

// StepUpFrom returns the next rung above the given one, bounded by the source.
func StepUpFrom(id string, sourceHeight int) (QualityRung, bool) {
	ladder := QualityLadderFor(sourceHeight)
	for i, rung := range ladder {
		if strings.EqualFold(rung.ID, id) && i > 0 {
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
