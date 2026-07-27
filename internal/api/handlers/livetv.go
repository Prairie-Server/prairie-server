package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "github.com/prairie-server/prairie-server/internal/api/middleware"
	"github.com/prairie-server/prairie-server/internal/livetv"
	"github.com/prairie-server/prairie-server/internal/livetv/gracenote"
	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

// LiveTVHandler exposes Live TV / OTA / DVR APIs under /api/v1/livetv.
type LiveTVHandler struct {
	service *livetv.Service
}

func NewLiveTVHandler(service *livetv.Service) *LiveTVHandler {
	if service == nil {
		return nil
	}
	return &LiveTVHandler{service: service}
}

func (h *LiveTVHandler) HandleListTuners(w http.ResponseWriter, r *http.Request) {
	tuners, err := h.service.ListTuners(r.Context())
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if tuners == nil {
		tuners = []livetv.Tuner{}
	}
	writeJSON(w, http.StatusOK, struct {
		Tuners []livetv.Tuner `json:"tuners"`
	}{Tuners: tuners})
}

func (h *LiveTVHandler) HandleAddTuner(w http.ResponseWriter, r *http.Request) {
	var body livetv.AddTunerInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	tuner, err := h.service.AddTuner(r.Context(), body)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tuner)
}

func (h *LiveTVHandler) HandleDiscoverTuners(w http.ResponseWriter, r *http.Request) {
	var body livetv.DiscoverTunersRequest
	if r.Body != nil {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
	}
	result, err := h.service.DiscoverTuners(r.Context(), body)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LiveTVHandler) HandleDeleteTuner(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteTuner(r.Context(), chi.URLParam(r, "tunerId")); err != nil {
		writeLiveTVError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LiveTVHandler) HandleScanTuner(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "tunerId")
	if err := h.service.ScanTuner(r.Context(), id); err != nil {
		writeLiveTVError(w, err)
		return
	}
	tuners, err := h.service.ListTuners(r.Context())
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	for i := range tuners {
		if tuners[i].ID == id {
			writeJSON(w, http.StatusOK, tuners[i])
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "scanned"})
}

func (h *LiveTVHandler) HandleListChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.service.ListChannels(r.Context(), r.URL.Query().Get("tuner_id"))
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if channels == nil {
		channels = []livetv.Channel{}
	}
	writeJSON(w, http.StatusOK, struct {
		Channels []livetv.Channel `json:"channels"`
	}{Channels: channels})
}

func (h *LiveTVHandler) HandlePatchChannel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled        *bool   `json:"enabled"`
		NumberOverride *string `json:"number_override"`
		GuideStationID *string `json:"guide_station_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	channel, err := h.service.PatchChannel(r.Context(), chi.URLParam(r, "channelId"), livetv.ChannelPatch{
		Enabled:        body.Enabled,
		NumberOverride: body.NumberOverride,
		GuideStationID: body.GuideStationID,
	})
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, channel)
}

func (h *LiveTVHandler) HandleListGuideSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.service.ListGuideSources(r.Context())
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if sources == nil {
		sources = []livetv.GuideSource{}
	}
	writeJSON(w, http.StatusOK, struct {
		GuideSources []livetv.GuideSource `json:"guide_sources"`
	}{GuideSources: sources})
}

func (h *LiveTVHandler) HandleLookupSchedulesDirectLineups(w http.ResponseWriter, r *http.Request) {
	var body livetv.SchedulesDirectLineupsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	lineups, err := h.service.ListSchedulesDirectLineups(r.Context(), body)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if lineups == nil {
		lineups = []schedulesdirect.LineupOption{}
	}
	writeJSON(w, http.StatusOK, struct {
		Lineups []schedulesdirect.LineupOption `json:"lineups"`
	}{Lineups: lineups})
}

func (h *LiveTVHandler) HandleLookupXMLSyncLineups(w http.ResponseWriter, r *http.Request) {
	var body livetv.XMLSyncLineupsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	lineups, err := h.service.ListXMLSyncLineups(r.Context(), body)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if lineups == nil {
		lineups = []gracenote.LineupOption{}
	}
	writeJSON(w, http.StatusOK, struct {
		Lineups []gracenote.LineupOption `json:"lineups"`
	}{Lineups: lineups})
}

func (h *LiveTVHandler) HandleCreateGuideSource(w http.ResponseWriter, r *http.Request) {
	var source livetv.GuideSource
	if err := json.NewDecoder(r.Body).Decode(&source); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	created, err := h.service.CreateGuideSource(r.Context(), &source)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *LiveTVHandler) HandleUpdateGuideSource(w http.ResponseWriter, r *http.Request) {
	var patch struct {
		Type        *string            `json:"type"`
		Priority    *int               `json:"priority"`
		Enabled     *bool              `json:"enabled"`
		DisplayName *string            `json:"display_name"`
		Config      *map[string]string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	id := chi.URLParam(r, "sourceId")
	sources, err := h.service.ListGuideSources(r.Context())
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	var existing *livetv.GuideSource
	for i := range sources {
		if sources[i].ID == id {
			existing = &sources[i]
			break
		}
	}
	if existing == nil {
		writeLiveTVError(w, livetv.ErrNotFound)
		return
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
	updated, err := h.service.UpdateGuideSource(r.Context(), &merged)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *LiveTVHandler) HandleDeleteGuideSource(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteGuideSource(r.Context(), chi.URLParam(r, "sourceId")); err != nil {
		writeLiveTVError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LiveTVHandler) HandleSyncGuideSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sourceId")
	if err := h.service.SyncGuideSource(r.Context(), id); err != nil {
		writeLiveTVError(w, err)
		return
	}
	sources, err := h.service.ListGuideSources(r.Context())
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	for i := range sources {
		if sources[i].ID == id {
			writeJSON(w, http.StatusOK, sources[i])
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "ready"})
}

func (h *LiveTVHandler) HandleListGuide(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var channelIDs []string
	if raw := strings.TrimSpace(q.Get("channels")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				channelIDs = append(channelIDs, part)
			}
		}
	}
	start, err := parseOptionalTime(q.Get("start"), time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_start", "start must be RFC3339")
		return
	}
	end, err := parseOptionalTime(q.Get("end"), start.Add(12*time.Hour))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_end", "end must be RFC3339")
		return
	}
	programs, err := h.service.ListGuide(r.Context(), channelIDs, start, end)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if programs == nil {
		programs = []livetv.Program{}
	}
	writeJSON(w, http.StatusOK, struct {
		Programs []livetv.Program `json:"programs"`
		Start    time.Time        `json:"start"`
		End      time.Time        `json:"end"`
	}{Programs: programs, Start: start, End: end})
}

func (h *LiveTVHandler) HandleGetProgram(w http.ResponseWriter, r *http.Request) {
	program, err := h.service.GetProgram(r.Context(), chi.URLParam(r, "programId"))
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, program)
}

func (h *LiveTVHandler) HandleStartChannelSession(w http.ResponseWriter, r *http.Request) {
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	session, err := h.service.StartChannelSession(r.Context(), chi.URLParam(r, "channelId"), userID, profileID)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	ticket := session.PlaybackSessionID
	if ticket == "" {
		ticket = session.ID
	}
	hlsURL := session.HLSURL
	if !livetv.IsClientSafePlayURL(hlsURL) {
		// Never return raw tuner URLs — clients play via the authenticated proxy.
		hlsURL = livetv.PublicSessionStreamPath(session.ID)
	}
	transport := session.Transport
	if transport == "" {
		transport = "mpegts"
	}
	writeJSON(w, http.StatusCreated, struct {
		SessionID      string `json:"session_id"`
		PlaybackTicket string `json:"playback_ticket"`
		HLSURL         string `json:"hls_url"`
		StreamURL      string `json:"stream_url"`
		Transport      string `json:"transport,omitempty"`
		Note           string `json:"note,omitempty"`
	}{
		SessionID:      session.ID,
		PlaybackTicket: ticket,
		HLSURL:         hlsURL,
		StreamURL:      hlsURL,
		Transport:      transport,
		Note:           session.Note,
	})
}

// HandleSessionStream proxies the upstream tuner for an owned active session.
func (h *LiveTVHandler) HandleSessionStream(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	enforceOwner := !apimw.IsAdmin(r.Context())
	session, err := h.service.GetSessionForViewer(r.Context(), sessionID, userID, profileID, enforceOwner)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if session.Status != "active" {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	upstream, err := h.service.ResolveSessionUpstreamURL(r.Context(), sessionID)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "failed to open upstream")
		return
	}
	// Long-lived MPEG-TS proxy — no overall Client.Timeout.
	resp, err := livetv.NewStreamHTTPClient().Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "upstream fetch failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "upstream_error", fmt.Sprintf("upstream status %d", resp.StatusCode))
		return
	}
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, resp.Body); err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(r.Context(), "livetv session stream copy ended", "session_id", sessionID, "error", err)
	}
}

func (h *LiveTVHandler) HandleReleaseSession(w http.ResponseWriter, r *http.Request) {
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	// Admins may release any session (tuner capacity recovery); everyone else
	// may only release sessions they own.
	enforceOwner := !apimw.IsAdmin(r.Context())
	session, err := h.service.ReleaseSession(
		r.Context(),
		chi.URLParam(r, "sessionId"),
		userID,
		profileID,
		enforceOwner,
	)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (h *LiveTVHandler) HandleListRecordings(w http.ResponseWriter, r *http.Request) {
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	enforceOwner := !apimw.IsAdmin(r.Context())
	recordings, err := h.service.ListRecordings(
		r.Context(), r.URL.Query().Get("status"), userID, profileID, enforceOwner,
	)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if recordings == nil {
		recordings = []livetv.Recording{}
	}
	writeJSON(w, http.StatusOK, struct {
		Recordings []livetv.Recording `json:"recordings"`
	}{Recordings: recordings})
}

func (h *LiveTVHandler) HandleScheduleRecording(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProgramID string    `json:"program_id"`
		ChannelID string    `json:"channel_id"`
		Start     time.Time `json:"start"`
		Stop      time.Time `json:"stop"`
		Title     string    `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	rec, err := h.service.ScheduleRecording(r.Context(), &livetv.Recording{
		ProgramID: body.ProgramID,
		ChannelID: body.ChannelID,
		UserID:    apimw.GetUserID(r.Context()),
		ProfileID: apimw.GetProfileID(r.Context()),
		Start:     body.Start,
		Stop:      body.Stop,
		Title:     body.Title,
	})
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (h *LiveTVHandler) HandleCancelRecording(w http.ResponseWriter, r *http.Request) {
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	enforceOwner := !apimw.IsAdmin(r.Context())
	rec, err := h.service.CancelRecording(
		r.Context(), chi.URLParam(r, "recordingId"), userID, profileID, enforceOwner,
	)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *LiveTVHandler) HandleListSeriesRules(w http.ResponseWriter, r *http.Request) {
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	enforceOwner := !apimw.IsAdmin(r.Context())
	rules, err := h.service.ListSeriesRules(r.Context(), userID, profileID, enforceOwner)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	if rules == nil {
		rules = []livetv.SeriesRule{}
	}
	writeJSON(w, http.StatusOK, struct {
		SeriesRules []livetv.SeriesRule `json:"series_rules"`
	}{SeriesRules: rules})
}

func (h *LiveTVHandler) HandleCreateSeriesRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SeriesID   string  `json:"series_id"`
		ChannelID  *string `json:"channel_id"`
		TitleMatch string  `json:"title_match"`
		NewOnly    bool    `json:"new_only"`
		KeepLast   int     `json:"keep_last"`
		Enabled    *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	created, err := h.service.CreateSeriesRule(r.Context(), &livetv.SeriesRule{
		SeriesID:   body.SeriesID,
		ChannelID:  body.ChannelID,
		UserID:     apimw.GetUserID(r.Context()),
		ProfileID:  apimw.GetProfileID(r.Context()),
		TitleMatch: body.TitleMatch,
		NewOnly:    body.NewOnly,
		KeepLast:   body.KeepLast,
		Enabled:    enabled,
	})
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *LiveTVHandler) HandleDeleteSeriesRule(w http.ResponseWriter, r *http.Request) {
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	enforceOwner := !apimw.IsAdmin(r.Context())
	if err := h.service.DeleteSeriesRule(
		r.Context(), chi.URLParam(r, "ruleId"), userID, profileID, enforceOwner,
	); err != nil {
		writeLiveTVError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleLiveHLS serves a remuxed live HLS playlist or segment for a playback
// bridge session.
func (h *LiveTVHandler) HandleLiveHLS(w http.ResponseWriter, r *http.Request) {
	playbackID := chi.URLParam(r, "playbackId")
	name := chi.URLParam(r, "name")
	if name == "" {
		name = "index.m3u8"
	}
	bridge, ok := h.service.PlaybackBridge().(*livetv.HLSBridge)
	if !ok || bridge == nil {
		writeError(w, http.StatusNotFound, "not_found", "live hls not available")
		return
	}
	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	enforceOwner := !apimw.IsAdmin(r.Context())
	if err := bridge.Authorize(playbackID, userID, profileID, enforceOwner); err != nil {
		writeLiveTVError(w, err)
		return
	}
	path, err := bridge.ResolvePlaylistFile(playbackID, name)
	if err != nil {
		writeLiveTVError(w, err)
		return
	}
	http.ServeFile(w, r, path)
}

func parseOptionalTime(raw string, fallback time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func writeLiveTVError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, livetv.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, livetv.ErrInvalidArgument):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, livetv.ErrLimitExceeded):
		writeError(w, http.StatusConflict, "limit_exceeded", err.Error())
	case errors.Is(err, livetv.ErrNoTuner):
		writeError(w, http.StatusConflict, "no_tuner", err.Error())
	case errors.Is(err, livetv.ErrNotImplemented):
		writeError(w, http.StatusNotImplemented, "not_implemented", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
