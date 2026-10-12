package apiv2

import (
	"context"
	"net/http"

	"github.com/prairie-server/prairie-server/internal/playback"
)

const opGetPlaybackQualityLadder = "getPlaybackQualityLadder"

// PlaybackQualityRung is one transcode rung, highest first.
type PlaybackQualityRung struct {
	ID          string `json:"id" doc:"Stable rung id clients send as quality_preference" example:"1080p_8m"`
	Label       string `json:"label" example:"1080p (8 Mbps)"`
	Resolution  string `json:"resolution" example:"1080p"`
	Height      int    `json:"height" example:"1080"`
	BitrateKbps int    `json:"bitrate_kbps" example:"8000"`
}

// PlaybackQualityLadder is the quality picker payload: the transcode rungs the
// server advises, filtered to the source height when one is given, and the
// modes that are not rungs.
type PlaybackQualityLadder struct {
	Rungs        []PlaybackQualityRung `json:"rungs"`
	Modes        []string              `json:"modes" doc:"Preferences that are not rungs: auto lets the server decide, original keeps the source"`
	SourceHeight int                   `json:"source_height" doc:"The source height the rungs were filtered to; 0 when none was given"`
}

type PlaybackQualityLadderInput struct {
	SourceHeight int `query:"source_height" minimum:"0" maximum:"8640" doc:"Source video height in pixels; rungs above it are omitted. Absent or 0 lists the whole ladder."`
}
type PlaybackQualityLadderOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         PlaybackQualityLadder
}

// registerPlaybackQualityLadder serves the server's transcode ladder so every
// client renders the same rungs instead of keeping its own drifting copy. It
// answers from the build alone, so it needs no profile and no service.
func registerPlaybackQualityLadder(reg *Registry) {
	op := Operation{Operation: humaOp(http.MethodGet, Prefix+"/playback/quality-ladder", opGetPlaybackQualityLadder, playbackTag, "Read the transcode quality ladder clients render in their quality picker, optionally limited to a source height."), Class: ClassAuthenticated}
	Register(reg, op, func(_ context.Context, in *PlaybackQualityLadderInput) (*PlaybackQualityLadderOutput, error) {
		options := playback.QualityOptionsFor(in.SourceHeight)
		body := PlaybackQualityLadder{Rungs: mapSlice(options.Rungs, func(r playback.QualityRung) PlaybackQualityRung { return PlaybackQualityRung(r) }), Modes: append([]string{}, options.Modes...), SourceHeight: options.SourceHeight}
		return &PlaybackQualityLadderOutput{CacheControl: playbackCacheControl, Body: body}, nil
	})
}
