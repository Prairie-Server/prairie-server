package apiv2

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/prairie-server/prairie-server/internal/api/handlers"
	"github.com/prairie-server/prairie-server/internal/livetv"
)

const (
	opListLiveTVChannels       = "listLiveTVChannels"
	opGetLiveTVGuide           = "getLiveTVGuide"
	opGetLiveTVProgram         = "getLiveTVProgram"
	opStartLiveTVSession       = "startLiveTVSession"
	opHeartbeatLiveTVSession   = "heartbeatLiveTVSession"
	opStopLiveTVSession        = "stopLiveTVSession"
	opListLiveTVRecordings     = "listLiveTVRecordings"
	opScheduleLiveTVRecording  = "scheduleLiveTVRecording"
	opCancelLiveTVRecording    = "cancelLiveTVRecording"
	opListLiveTVSeriesRules    = "listLiveTVSeriesRules"
	opCreateLiveTVSeriesRule   = "createLiveTVSeriesRule"
	opDeleteLiveTVSeriesRule   = "deleteLiveTVSeriesRule"
	liveTVTag                  = "livetv"
	liveTVDomain               = "Live TV"
	liveTVTransportMPEGTS      = "mpegts"
	liveTVGuideMaxWindow       = 7 * 24 * time.Hour
)

// LiveTVService is the application seam behind the v2 Live TV operations.
// It is *handlers.LiveTVHandler in production; the same methods answer the
// frozen /api/v1/livetv routes, so both surfaces share one implementation.
// Caller identity, profile and administrator role come from the context.
type LiveTVService interface {
	ListChannels(context.Context, string) ([]livetv.Channel, error)
	ListGuide(context.Context, []string, time.Time, time.Time) ([]livetv.Program, error)
	GetProgram(context.Context, string) (*livetv.Program, error)
	StartChannelSession(context.Context, string, livetv.ClientCapabilities) (handlers.LiveTVSessionStart, error)
	HeartbeatSession(context.Context, string) error
	ReleaseSession(context.Context, string) (*livetv.LiveSession, error)
	ListRecordings(context.Context, string) ([]livetv.Recording, error)
	ScheduleRecording(context.Context, livetv.Recording) (*livetv.Recording, error)
	CancelRecording(context.Context, string) (*livetv.Recording, error)
	ListSeriesRules(context.Context) ([]livetv.SeriesRule, error)
	CreateSeriesRule(context.Context, livetv.SeriesRule) (*livetv.SeriesRule, error)
	DeleteSeriesRule(context.Context, string) error
	LiveTVAdminService
}

// LiveTVChannel is a tuner channel. The upstream tuner URL is never exposed;
// clients play a channel by starting a session.
type LiveTVChannel struct {
	ID             ID      `json:"id"`
	TunerID        ID      `json:"tuner_id"`
	Number         string  `json:"number" doc:"Effective channel number: the override when one is set"`
	NumberOverride *string `json:"number_override,omitempty" doc:"Administrator override of the tuner's channel number"`
	Callsign       string  `json:"callsign"`
	Name           string  `json:"name"`
	LogoURL        string  `json:"logo_url"`
	HD             bool    `json:"hd"`
	Enabled        bool    `json:"enabled"`
	GuideStationID string  `json:"guide_station_id"`
}

type LiveTVProgram struct {
	ID          ID       `json:"id"`
	ChannelID   ID       `json:"channel_id"`
	SourceID    ID       `json:"source_id,omitempty"`
	SeriesID    string   `json:"series_id"`
	ExternalID  string   `json:"external_id,omitempty"`
	Start       Instant  `json:"start"`
	Stop        Instant  `json:"stop"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Description string   `json:"description"`
	Season      *int     `json:"season,omitempty" nullable:"false"`
	Episode     *int     `json:"episode,omitempty" nullable:"false"`
	Genres      []string `json:"genres"`
	ImageURL    string   `json:"image_url"`
	IsNew       bool     `json:"is_new"`
	IsLive      bool     `json:"is_live"`
}

// LiveTVGuide is the programme grid for a window: every programme airing in
// [start, end) on the requested channels.
type LiveTVGuide struct {
	Items []LiveTVProgram `json:"items" doc:"Programmes in the window; empty, never null"`
	Start Instant         `json:"start" doc:"Effective window start"`
	End   Instant         `json:"end" doc:"Effective window end"`
}

// LiveTVClientCapabilities describes what the player decodes, so the server
// copies the broadcast streams when it can and re-encodes only when it must.
// Omitted entirely, the server copies the streams (pre-capability behavior).
type LiveTVClientCapabilities struct {
	CodecsVideo            []string `json:"codecs_video,omitempty" maxItems:"32" doc:"Video codecs the player decodes, for example h264, hevc, mpeg2video"`
	CodecsAudio            []string `json:"codecs_audio,omitempty" maxItems:"32" doc:"Audio codecs the player decodes, for example aac, ac3"`
	AudioPassthroughCodecs []string `json:"audio_passthrough_codecs,omitempty" maxItems:"32" doc:"Formats the client can pass to a receiver undecoded"`
	MaxAudioChannels       int      `json:"max_audio_channels,omitempty" minimum:"0" maximum:"16" doc:"Delivered channel cap; 0 or absent means stereo when re-encoding"`
	MaxResolution          string   `json:"max_resolution,omitempty" maxLength:"16" doc:"Optional encode ceiling such as 1080p or 720p"`
}

type LiveTVSessionStartBody struct {
	ClientCapabilities *LiveTVClientCapabilities `json:"client_capabilities,omitempty" nullable:"false"`
}

type LiveTVSessionStartInput struct {
	ChannelID ID                      `path:"channel_id" minLength:"1"`
	Body      *LiveTVSessionStartBody `required:"false" doc:"Absent copies the broadcast streams"`
}

// LiveTVSession is a started live channel session. stream_url is opaque and
// server-issued: it is either a self-authorizing live HLS playlist (it carries
// its own session-bound stream token, so a native player needs no headers) or
// the authenticated MPEG-TS proxy, which needs the account bearer and
// X-Profile-Id like any other request. Clients never construct it.
type LiveTVSession struct {
	SessionID         ID      `json:"session_id"`
	ChannelID         ID      `json:"channel_id"`
	PlaybackTicket    string  `json:"playback_ticket" doc:"Playback session the stream is delivered under; equals session_id when no bridge is in use"`
	StreamURL         string  `json:"stream_url" doc:"Opaque server-relative play URL; never an upstream tuner address"`
	Transport         string  `json:"transport" enum:"hls,mpegts" doc:"hls for a remuxed or transcoded live HLS playlist; mpegts for the raw transport-stream proxy"`
	Note              string  `json:"note,omitempty"`
	Status            string  `json:"status"`
	CreatedAt         Instant `json:"created_at"`
	StaleAfterSeconds int     `json:"stale_after_seconds" doc:"An active session neither fetched nor heartbeated for this long is reclaimed; heartbeat well within it while the player is open, including while paused"`
}

type LiveTVSessionOutput struct {
	Body LiveTVSession
}

type LiveTVSessionInput struct {
	SessionID ID `path:"session_id" minLength:"1"`
}

type LiveTVRecording struct {
	ID            ID       `json:"id"`
	ProgramID     ID       `json:"program_id,omitempty"`
	ChannelID     ID       `json:"channel_id"`
	SeriesRuleID  ID       `json:"series_rule_id,omitempty"`
	ProfileID     ID       `json:"profile_id,omitempty"`
	Status        string   `json:"status" doc:"scheduled, recording, completed, failed or cancelled"`
	LibraryItemID ID       `json:"library_item_id,omitempty" doc:"Catalog item of the finished recording, once imported"`
	Start         *Instant `json:"start,omitempty"`
	Stop          *Instant `json:"stop,omitempty"`
	Title         string   `json:"title"`
	LastError     string   `json:"last_error,omitempty"`
}

type LiveTVRecordingBody struct {
	ProgramID string   `json:"program_id,omitempty" maxLength:"256" doc:"Guide programme to record; its channel and times are used when the others are absent"`
	ChannelID string   `json:"channel_id,omitempty" maxLength:"256"`
	Start     *Instant `json:"start,omitempty" nullable:"false"`
	Stop      *Instant `json:"stop,omitempty" nullable:"false"`
	Title     string   `json:"title,omitempty" maxLength:"512"`
}

type LiveTVSeriesRule struct {
	ID         ID     `json:"id"`
	SeriesID   string `json:"series_id"`
	ChannelID  *ID    `json:"channel_id,omitempty" doc:"Restricts the rule to one channel; absent matches every channel"`
	ProfileID  ID     `json:"profile_id,omitempty"`
	TitleMatch string `json:"title_match"`
	NewOnly    bool   `json:"new_only"`
	KeepLast   int    `json:"keep_last" doc:"Recordings to keep; 0 keeps all"`
	Enabled    bool   `json:"enabled"`
}

type LiveTVSeriesRuleBody struct {
	SeriesID   string  `json:"series_id,omitempty" maxLength:"256"`
	ChannelID  *string `json:"channel_id,omitempty" nullable:"false" maxLength:"256"`
	TitleMatch string  `json:"title_match,omitempty" maxLength:"512"`
	NewOnly    bool    `json:"new_only,omitempty"`
	KeepLast   int     `json:"keep_last,omitempty" minimum:"0" maximum:"1000"`
	Enabled    *bool   `json:"enabled,omitempty" nullable:"false" doc:"Absent creates an enabled rule"`
}

type LiveTVChannelsInput struct {
	TunerID string `query:"tuner_id" maxLength:"256" doc:"Only channels of this tuner"`
}
type LiveTVChannelsOutput struct{ Body Collection[LiveTVChannel] }

type LiveTVGuideInput struct {
	ChannelIDs []string `query:"channel_id,explode" maxItems:"500" doc:"Restrict to these channels, repeated (channel_id=a&channel_id=b); absent lists every channel"`
	Start      string   `query:"start" format:"date-time" doc:"Window start; default now"`
	End        string   `query:"end" format:"date-time" doc:"Window end; default start plus 12 hours, at most 7 days after start"`
}
type LiveTVGuideOutput struct{ Body LiveTVGuide }

type LiveTVProgramInput struct {
	ProgramID ID `path:"program_id" minLength:"1"`
}
type LiveTVProgramOutput struct{ Body LiveTVProgram }

type LiveTVRecordingsInput struct {
	Status string `query:"status" enum:"scheduled,recording,completed,failed,cancelled" doc:"Only recordings in this state"`
}
type LiveTVRecordingsOutput struct{ Body Collection[LiveTVRecording] }
type LiveTVRecordingCreateInput struct{ Body LiveTVRecordingBody }
type LiveTVRecordingOutput struct{ Body LiveTVRecording }
type LiveTVRecordingInput struct {
	RecordingID ID `path:"recording_id" minLength:"1"`
}

type LiveTVSeriesRulesOutput struct{ Body Collection[LiveTVSeriesRule] }
type LiveTVSeriesRuleCreateInput struct{ Body LiveTVSeriesRuleBody }
type LiveTVSeriesRuleOutput struct{ Body LiveTVSeriesRule }
type LiveTVSeriesRuleInput struct {
	RuleID ID `path:"rule_id" minLength:"1"`
}

func registerLiveTV(reg *Registry) {
	summaries := map[string]string{
		opListLiveTVChannels:      "List Live TV channels across every tuner, or one tuner's.",
		opGetLiveTVGuide:          "Read the programme guide for a time window, optionally for chosen channels.",
		opGetLiveTVProgram:        "Read one guide programme.",
		opStartLiveTVSession:      "Claim a tuner and start watching a channel. The reply carries an opaque play URL and the transport to play it with.",
		opHeartbeatLiveTVSession:  "Keep an open session's tuner claimed while the player is open, including while paused.",
		opStopLiveTVSession:       "Release a session and its tuner. Administrators may release any session.",
		opListLiveTVRecordings:    "List this profile's DVR recordings; administrators see every recording.",
		opScheduleLiveTVRecording: "Schedule a DVR recording for this profile. Never retried automatically.",
		opCancelLiveTVRecording:   "Cancel a recording this profile owns; administrators may cancel any.",
		opListLiveTVSeriesRules:   "List this profile's series recording rules; administrators see every rule.",
		opCreateLiveTVSeriesRule:  "Create a series recording rule for this profile. Never retried automatically.",
		opDeleteLiveTVSeriesRule:  "Delete a series recording rule this profile owns; administrators may delete any.",
	}
	op := func(method, path, id string) Operation {
		operation := Operation{Operation: humaOp(method, Prefix+"/livetv"+path, id, liveTVTag, summaries[id]), Class: ClassProfileScoped, ServiceBacked: true}
		operation.Errors = []int{http.StatusConflict, http.StatusNotFound, http.StatusUnprocessableEntity}
		if method == http.MethodGet {
			// Guide reads mirror the v1 group: profile optional, narrowed for
			// households with a restricted profile.
			operation.ProfileOptional = true
			operation.HouseholdProfileGate = true
		} else {
			operation.DemoRestricted = true
		}
		switch id {
		case opStartLiveTVSession, opScheduleLiveTVRecording, opCreateLiveTVSeriesRule:
			operation.DefaultStatus = http.StatusCreated
			operation.RetrySafety = RetrySafetyNonRetryable
		case opHeartbeatLiveTVSession, opStopLiveTVSession, opDeleteLiveTVSeriesRule:
			operation.DefaultStatus = http.StatusNoContent
			operation.RetrySafety = RetrySafetyNaturalIdempotent
		case opCancelLiveTVRecording:
			operation.RetrySafety = RetrySafetyNaturalIdempotent
		}
		if id == opStartLiveTVSession {
			operation.MaxBodyBytes = 16 << 10
		}
		return operation
	}
	Register(reg, op(http.MethodGet, "/channels", opListLiveTVChannels), func(ctx context.Context, in *LiveTVChannelsInput) (*LiveTVChannelsOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		channels, err := svc.ListChannels(ctx, strings.TrimSpace(in.TunerID))
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVChannelsOutput{Body: NewCollection(mapSlice(channels, liveTVChannelOf))}, nil
	})
	Register(reg, op(http.MethodGet, "/guide", opGetLiveTVGuide), func(ctx context.Context, in *LiveTVGuideInput) (*LiveTVGuideOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		start, end, p := liveTVGuideWindow(in.Start, in.End, time.Now().UTC())
		if p != nil {
			return nil, p
		}
		var channelIDs []string
		for _, id := range in.ChannelIDs {
			if id = strings.TrimSpace(id); id != "" {
				channelIDs = append(channelIDs, id)
			}
		}
		programs, err := svc.ListGuide(ctx, channelIDs, start, end)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVGuideOutput{Body: LiveTVGuide{Items: mapSlice(programs, liveTVProgramOf), Start: NewInstant(start), End: NewInstant(end)}}, nil
	})
	Register(reg, op(http.MethodGet, "/programs/{program_id}", opGetLiveTVProgram), func(ctx context.Context, in *LiveTVProgramInput) (*LiveTVProgramOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		program, err := svc.GetProgram(ctx, string(in.ProgramID))
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVProgramOutput{Body: liveTVProgramOf(*program)}, nil
	})
	registerLiveTVSessions(reg, op)
	registerLiveTVRecordings(reg, op)
	registerLiveTVSeriesRules(reg, op)
	registerAdminLiveTV(reg)
}

func registerLiveTVSessions(reg *Registry, op func(method, path, id string) Operation) {
	Register(reg, op(http.MethodPost, "/channels/{channel_id}/sessions", opStartLiveTVSession), func(ctx context.Context, in *LiveTVSessionStartInput) (*LiveTVSessionOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		var caps livetv.ClientCapabilities
		if in.Body != nil && in.Body.ClientCapabilities != nil {
			c := in.Body.ClientCapabilities
			caps = livetv.ClientCapabilities{CodecsVideo: c.CodecsVideo, CodecsAudio: c.CodecsAudio, AudioPassthroughCodecs: c.AudioPassthroughCodecs, MaxAudioChannels: c.MaxAudioChannels, MaxResolution: c.MaxResolution}
		}
		started, err := svc.StartChannelSession(ctx, string(in.ChannelID), caps)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		s := started.Session
		body := LiveTVSession{SessionID: ID(s.ID), ChannelID: ID(s.ChannelID), PlaybackTicket: started.PlaybackTicket, StreamURL: started.StreamURL, Transport: started.Transport, Note: s.Note, Status: s.Status, StaleAfterSeconds: int(livetv.StaleSessionTTL / time.Second)}
		if body.Transport != "hls" {
			body.Transport = liveTVTransportMPEGTS
		}
		created := s.CreatedAt
		if created.IsZero() {
			created = time.Now()
		}
		body.CreatedAt = NewInstant(created)
		return &LiveTVSessionOutput{Body: body}, nil
	})
	Register(reg, op(http.MethodPost, "/sessions/{session_id}/heartbeat", opHeartbeatLiveTVSession), func(ctx context.Context, in *LiveTVSessionInput) (*struct{}, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		if err := svc.HeartbeatSession(ctx, string(in.SessionID)); err != nil {
			return nil, liveTVProblem(err)
		}
		return &struct{}{}, nil
	})
	Register(reg, op(http.MethodDelete, "/sessions/{session_id}", opStopLiveTVSession), func(ctx context.Context, in *LiveTVSessionInput) (*struct{}, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		if _, err := svc.ReleaseSession(ctx, string(in.SessionID)); err != nil {
			return nil, liveTVProblem(err)
		}
		return &struct{}{}, nil
	})
}

func registerLiveTVRecordings(reg *Registry, op func(method, path, id string) Operation) {
	Register(reg, op(http.MethodGet, "/recordings", opListLiveTVRecordings), func(ctx context.Context, in *LiveTVRecordingsInput) (*LiveTVRecordingsOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		recordings, err := svc.ListRecordings(ctx, in.Status)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVRecordingsOutput{Body: NewCollection(mapSlice(recordings, liveTVRecordingOf))}, nil
	})
	Register(reg, op(http.MethodPost, "/recordings", opScheduleLiveTVRecording), func(ctx context.Context, in *LiveTVRecordingCreateInput) (*LiveTVRecordingOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		rec := livetv.Recording{ProgramID: strings.TrimSpace(b.ProgramID), ChannelID: strings.TrimSpace(b.ChannelID), Title: b.Title}
		if b.Start != nil {
			rec.Start = b.Start.Time
		}
		if b.Stop != nil {
			rec.Stop = b.Stop.Time
		}
		created, err := svc.ScheduleRecording(ctx, rec)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVRecordingOutput{Body: liveTVRecordingOf(*created)}, nil
	})
	Register(reg, op(http.MethodDelete, "/recordings/{recording_id}", opCancelLiveTVRecording), func(ctx context.Context, in *LiveTVRecordingInput) (*LiveTVRecordingOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		cancelled, err := svc.CancelRecording(ctx, string(in.RecordingID))
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVRecordingOutput{Body: liveTVRecordingOf(*cancelled)}, nil
	})
}

func registerLiveTVSeriesRules(reg *Registry, op func(method, path, id string) Operation) {
	Register(reg, op(http.MethodGet, "/series-rules", opListLiveTVSeriesRules), func(ctx context.Context, _ *struct{}) (*LiveTVSeriesRulesOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		rules, err := svc.ListSeriesRules(ctx)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVSeriesRulesOutput{Body: NewCollection(mapSlice(rules, liveTVSeriesRuleOf))}, nil
	})
	Register(reg, op(http.MethodPost, "/series-rules", opCreateLiveTVSeriesRule), func(ctx context.Context, in *LiveTVSeriesRuleCreateInput) (*LiveTVSeriesRuleOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		rule := livetv.SeriesRule{SeriesID: strings.TrimSpace(b.SeriesID), ChannelID: b.ChannelID, TitleMatch: b.TitleMatch, NewOnly: b.NewOnly, KeepLast: b.KeepLast, Enabled: b.Enabled == nil || *b.Enabled}
		created, err := svc.CreateSeriesRule(ctx, rule)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &LiveTVSeriesRuleOutput{Body: liveTVSeriesRuleOf(*created)}, nil
	})
	Register(reg, op(http.MethodDelete, "/series-rules/{rule_id}", opDeleteLiveTVSeriesRule), func(ctx context.Context, in *LiveTVSeriesRuleInput) (*struct{}, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		if err := svc.DeleteSeriesRule(ctx, string(in.RuleID)); err != nil {
			return nil, liveTVProblem(err)
		}
		return &struct{}{}, nil
	})
}

func (reg *Registry) liveTV() (LiveTVService, *Problem) {
	if reg.deps.LiveTV == nil {
		return nil, CapabilityProblem(StateNotConfigured, liveTVDomain)
	}
	return reg.deps.LiveTV, nil
}

// liveTVGuideWindow resolves the guide window exactly as v1 does (now, then
// start plus 12 hours) and bounds it so one read cannot scan the whole guide.
func liveTVGuideWindow(rawStart, rawEnd string, now time.Time) (time.Time, time.Time, *Problem) {
	start := now
	if rawStart != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rawStart)
		if err != nil {
			return time.Time{}, time.Time{}, validationProblem("query.start", "invalid", "Expected an RFC 3339 instant.")
		}
		start = parsed.UTC()
	}
	end := start.Add(handlers.LiveTVGuideWindow)
	if rawEnd != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rawEnd)
		if err != nil {
			return time.Time{}, time.Time{}, validationProblem("query.end", "invalid", "Expected an RFC 3339 instant.")
		}
		end = parsed.UTC()
	}
	if !end.After(start) || end.Sub(start) > liveTVGuideMaxWindow {
		return time.Time{}, time.Time{}, validationProblem("query.end", "out_of_range", "end must be after start and at most 7 days later.")
	}
	return start, end, nil
}

// liveTVProblem maps the shared service's sentinel errors onto the problem
// catalog. The v1 400 invalid_argument is a v2 validation failure.
func liveTVProblem(err error) *Problem {
	switch {
	case errors.Is(err, livetv.ErrNotConfigured):
		return CapabilityProblem(StateNotConfigured, liveTVDomain)
	case errors.Is(err, livetv.ErrNotFound):
		return NewProblem(TypeNotFound, "The Live TV resource was not found.")
	case errors.Is(err, livetv.ErrInvalidArgument):
		return validationProblem("body", "invalid", liveTVDetail(err))
	case errors.Is(err, livetv.ErrLimitExceeded), errors.Is(err, livetv.ErrNoTuner):
		return NewProblem(TypeConflict, liveTVDetail(err))
	case errors.Is(err, livetv.ErrNotImplemented):
		return NewProblem(TypeCapabilityUnsupported, liveTVDetail(err))
	}
	return NewProblem(TypeDependencyUnavailable, "Live TV is temporarily unavailable.")
}

// liveTVDetail strips the package prefix the service's sentinel errors carry.
func liveTVDetail(err error) string {
	return strings.TrimPrefix(err.Error(), "livetv: ")
}

func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

func liveTVInstant(t time.Time) *Instant {
	if t.IsZero() {
		return nil
	}
	i := NewInstant(t)
	return &i
}

func liveTVChannelOf(c livetv.Channel) LiveTVChannel {
	return LiveTVChannel{ID: ID(c.ID), TunerID: ID(c.TunerID), Number: c.Number, NumberOverride: c.NumberOverride, Callsign: c.Callsign, Name: c.Name, LogoURL: c.LogoURL, HD: c.HD, Enabled: c.Enabled, GuideStationID: c.GuideStationID}
}

func liveTVProgramOf(p livetv.Program) LiveTVProgram {
	genres := p.Genres
	if genres == nil {
		genres = []string{}
	}
	out := LiveTVProgram{ID: ID(p.ID), ChannelID: ID(p.ChannelID), SourceID: ID(p.SourceID), SeriesID: p.SeriesID, ExternalID: p.ExternalID, Title: p.Title, Subtitle: p.Subtitle, Description: p.Description, Season: p.Season, Episode: p.Episode, Genres: genres, ImageURL: p.ImageURL, IsNew: p.IsNew, IsLive: p.IsLive}
	if s := liveTVInstant(p.Start); s != nil {
		out.Start = *s
	}
	if s := liveTVInstant(p.Stop); s != nil {
		out.Stop = *s
	}
	return out
}

func liveTVRecordingOf(r livetv.Recording) LiveTVRecording {
	return LiveTVRecording{ID: ID(r.ID), ProgramID: ID(r.ProgramID), ChannelID: ID(r.ChannelID), SeriesRuleID: ID(r.SeriesRuleID), ProfileID: ID(r.ProfileID), Status: r.Status, LibraryItemID: ID(r.LibraryItemID), Start: liveTVInstant(r.Start), Stop: liveTVInstant(r.Stop), Title: r.Title, LastError: r.LastError}
}

func liveTVSeriesRuleOf(r livetv.SeriesRule) LiveTVSeriesRule {
	out := LiveTVSeriesRule{ID: ID(r.ID), SeriesID: r.SeriesID, ProfileID: ID(r.ProfileID), TitleMatch: r.TitleMatch, NewOnly: r.NewOnly, KeepLast: r.KeepLast, Enabled: r.Enabled}
	if r.ChannelID != nil && *r.ChannelID != "" {
		id := ID(*r.ChannelID)
		out.ChannelID = &id
	}
	return out
}
