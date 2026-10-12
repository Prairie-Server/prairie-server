package handlers

import (
	"context"
	"time"

	apimw "github.com/prairie-server/prairie-server/internal/api/middleware"
	"github.com/prairie-server/prairie-server/internal/livetv"
	"github.com/prairie-server/prairie-server/internal/livetv/gracenote"
	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

// The context-based Live TV operations below are shared by the frozen /api/v1
// HTTP handlers in livetv.go and the native /api/v2 operations
// (internal/apiv2/livetv.go). The caller, its profile and whether it is an
// administrator come from the request context the auth gates populated, the
// same keys on both listeners.

// liveTVViewer returns the caller and whether ownership must be enforced:
// administrators may act on any viewer's sessions, recordings and rules.
func liveTVViewer(ctx context.Context) (userID int, profileID string, enforceOwner bool) {
	return apimw.GetUserID(ctx), apimw.GetProfileID(ctx), !apimw.IsAdmin(ctx)
}

// LiveTVGuideWindow is the default guide window: from now for 12 hours.
const LiveTVGuideWindow = 12 * time.Hour

// LiveTVSessionStart is the client-facing result of starting a live channel
// session: the session, the ticket clients pass to the player, and a
// self-authorizing play URL that never names the upstream tuner.
type LiveTVSessionStart struct {
	Session        livetv.LiveSession
	PlaybackTicket string
	StreamURL      string
	Transport      string
}

// GuideSourcePatch is a partial guide-source update; nil members are kept.
type GuideSourcePatch struct {
	Type        *string
	Priority    *int
	Enabled     *bool
	DisplayName *string
	Config      *map[string]string
}

func (h *LiveTVHandler) ListTuners(ctx context.Context) ([]livetv.Tuner, error) {
	tuners, err := h.service.ListTuners(ctx)
	if tuners == nil && err == nil {
		tuners = []livetv.Tuner{}
	}
	return tuners, err
}

func (h *LiveTVHandler) DiscoverTuners(ctx context.Context, req livetv.DiscoverTunersRequest) (*livetv.DiscoverTunersResult, error) {
	return h.service.DiscoverTuners(ctx, req)
}

func (h *LiveTVHandler) AddTuner(ctx context.Context, in livetv.AddTunerInput) (*livetv.Tuner, error) {
	return h.service.AddTuner(ctx, in)
}

func (h *LiveTVHandler) DeleteTuner(ctx context.Context, id string) error {
	return h.service.DeleteTuner(ctx, id)
}

// ScanTuner rescans a tuner's lineup and returns the refreshed tuner, or nil
// when the tuner vanished between the scan and the re-read.
func (h *LiveTVHandler) ScanTuner(ctx context.Context, id string) (*livetv.Tuner, error) {
	if err := h.service.ScanTuner(ctx, id); err != nil {
		return nil, err
	}
	tuners, err := h.service.ListTuners(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tuners {
		if tuners[i].ID == id {
			return &tuners[i], nil
		}
	}
	return nil, nil
}

func (h *LiveTVHandler) ListChannels(ctx context.Context, tunerID string) ([]livetv.Channel, error) {
	channels, err := h.service.ListChannels(ctx, tunerID)
	if channels == nil && err == nil {
		channels = []livetv.Channel{}
	}
	return channels, err
}

func (h *LiveTVHandler) PatchChannel(ctx context.Context, id string, patch livetv.ChannelPatch) (*livetv.Channel, error) {
	return h.service.PatchChannel(ctx, id, patch)
}

func (h *LiveTVHandler) ListGuideSources(ctx context.Context) ([]livetv.GuideSource, error) {
	sources, err := h.service.ListGuideSources(ctx)
	if sources == nil && err == nil {
		sources = []livetv.GuideSource{}
	}
	return sources, err
}

func (h *LiveTVHandler) LookupSchedulesDirectLineups(ctx context.Context, req livetv.SchedulesDirectLineupsRequest) ([]schedulesdirect.LineupOption, error) {
	lineups, err := h.service.ListSchedulesDirectLineups(ctx, req)
	if lineups == nil && err == nil {
		lineups = []schedulesdirect.LineupOption{}
	}
	return lineups, err
}

func (h *LiveTVHandler) LookupXMLSyncLineups(ctx context.Context, req livetv.XMLSyncLineupsRequest) ([]gracenote.LineupOption, error) {
	lineups, err := h.service.ListXMLSyncLineups(ctx, req)
	if lineups == nil && err == nil {
		lineups = []gracenote.LineupOption{}
	}
	return lineups, err
}

func (h *LiveTVHandler) CreateGuideSource(ctx context.Context, source *livetv.GuideSource) (*livetv.GuideSource, error) {
	return h.service.CreateGuideSource(ctx, source)
}

// UpdateGuideSource merges patch over the stored source and saves the result.
func (h *LiveTVHandler) UpdateGuideSource(ctx context.Context, id string, patch GuideSourcePatch) (*livetv.GuideSource, error) {
	sources, err := h.service.ListGuideSources(ctx)
	if err != nil {
		return nil, err
	}
	var existing *livetv.GuideSource
	for i := range sources {
		if sources[i].ID == id {
			existing = &sources[i]
			break
		}
	}
	if existing == nil {
		return nil, livetv.ErrNotFound
	}
	merged := *existing
	if patch.Type != nil {
		merged.Type = *patch.Type
	}
	if patch.Priority != nil {
		merged.Priority = *patch.Priority
	}
	if patch.Enabled != nil {
		merged.Enabled = *patch.Enabled
	}
	if patch.DisplayName != nil {
		merged.DisplayName = *patch.DisplayName
	}
	if patch.Config != nil {
		merged.Config = *patch.Config
	}
	return h.service.UpdateGuideSource(ctx, &merged)
}

func (h *LiveTVHandler) DeleteGuideSource(ctx context.Context, id string) error {
	return h.service.DeleteGuideSource(ctx, id)
}

// SyncGuideSource runs a guide sync now and returns the refreshed source, or
// nil when it vanished between the sync and the re-read.
func (h *LiveTVHandler) SyncGuideSource(ctx context.Context, id string) (*livetv.GuideSource, error) {
	if err := h.service.SyncGuideSource(ctx, id); err != nil {
		return nil, err
	}
	sources, err := h.service.ListGuideSources(ctx)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		if sources[i].ID == id {
			return &sources[i], nil
		}
	}
	return nil, nil
}

func (h *LiveTVHandler) ListGuide(ctx context.Context, channelIDs []string, start, end time.Time) ([]livetv.Program, error) {
	programs, err := h.service.ListGuide(ctx, channelIDs, start, end)
	if programs == nil && err == nil {
		programs = []livetv.Program{}
	}
	return programs, err
}

func (h *LiveTVHandler) GetProgram(ctx context.Context, id string) (*livetv.Program, error) {
	return h.service.GetProgram(ctx, id)
}

// StartChannelSession claims a tuner for the caller's profile and returns a
// play URL safe to hand to a native player: raw tuner URLs are replaced by the
// authenticated proxy path, and a live-HLS URL carries a stream token bound to
// its delivery id because the player cannot set headers or refresh a token.
func (h *LiveTVHandler) StartChannelSession(ctx context.Context, channelID string, caps livetv.ClientCapabilities) (LiveTVSessionStart, error) {
	userID, profileID, _ := liveTVViewer(ctx)
	session, err := h.service.StartChannelSession(ctx, channelID, userID, profileID, caps)
	if err != nil {
		return LiveTVSessionStart{}, err
	}
	ticket := session.PlaybackSessionID
	if ticket == "" {
		ticket = session.ID
	}
	streamURL := session.HLSURL
	if !livetv.IsClientSafePlayURL(streamURL) {
		// Never return raw tuner URLs — clients play via the authenticated proxy.
		streamURL = livetv.PublicSessionStreamPath(session.ID)
	}
	if deliveryID, ok := livetv.LiveHLSDeliveryID(streamURL); ok {
		streamURL = appendStreamToken(streamURL, h.signLiveStreamToken(deliveryID, userID, profileID))
	}
	transport := session.Transport
	if transport == "" {
		transport = "mpegts"
	}
	return LiveTVSessionStart{Session: *session, PlaybackTicket: ticket, StreamURL: streamURL, Transport: transport}, nil
}

// HeartbeatSession keeps an owned active session's tuner claimed.
func (h *LiveTVHandler) HeartbeatSession(ctx context.Context, sessionID string) error {
	userID, profileID, enforceOwner := liveTVViewer(ctx)
	session, err := h.service.GetSessionForViewer(ctx, sessionID, userID, profileID, enforceOwner)
	if err != nil {
		return err
	}
	if session.Status != "active" {
		return livetv.ErrNotFound
	}
	return h.service.TouchSession(ctx, sessionID)
}

// ReleaseSession releases an owned session; administrators may release any.
func (h *LiveTVHandler) ReleaseSession(ctx context.Context, sessionID string) (*livetv.LiveSession, error) {
	userID, profileID, enforceOwner := liveTVViewer(ctx)
	return h.service.ReleaseSession(ctx, sessionID, userID, profileID, enforceOwner)
}

func (h *LiveTVHandler) ListRecordings(ctx context.Context, status string) ([]livetv.Recording, error) {
	userID, profileID, enforceOwner := liveTVViewer(ctx)
	recordings, err := h.service.ListRecordings(ctx, status, userID, profileID, enforceOwner)
	if recordings == nil && err == nil {
		recordings = []livetv.Recording{}
	}
	return recordings, err
}

// ScheduleRecording schedules rec for the caller's profile; rec's owner
// fields are overwritten from the context.
func (h *LiveTVHandler) ScheduleRecording(ctx context.Context, rec livetv.Recording) (*livetv.Recording, error) {
	rec.UserID, rec.ProfileID, _ = liveTVViewer(ctx)
	return h.service.ScheduleRecording(ctx, &rec)
}

func (h *LiveTVHandler) CancelRecording(ctx context.Context, id string) (*livetv.Recording, error) {
	userID, profileID, enforceOwner := liveTVViewer(ctx)
	return h.service.CancelRecording(ctx, id, userID, profileID, enforceOwner)
}

func (h *LiveTVHandler) ListSeriesRules(ctx context.Context) ([]livetv.SeriesRule, error) {
	userID, profileID, enforceOwner := liveTVViewer(ctx)
	rules, err := h.service.ListSeriesRules(ctx, userID, profileID, enforceOwner)
	if rules == nil && err == nil {
		rules = []livetv.SeriesRule{}
	}
	return rules, err
}

// CreateSeriesRule creates rule for the caller's profile; rule's owner fields
// are overwritten from the context.
func (h *LiveTVHandler) CreateSeriesRule(ctx context.Context, rule livetv.SeriesRule) (*livetv.SeriesRule, error) {
	rule.UserID, rule.ProfileID, _ = liveTVViewer(ctx)
	return h.service.CreateSeriesRule(ctx, &rule)
}

func (h *LiveTVHandler) DeleteSeriesRule(ctx context.Context, id string) error {
	userID, profileID, enforceOwner := liveTVViewer(ctx)
	return h.service.DeleteSeriesRule(ctx, id, userID, profileID, enforceOwner)
}
