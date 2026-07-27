package jellycompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/prairie-server/prairie-server/internal/config"
	"github.com/prairie-server/prairie-server/internal/livetv"
)

// liveTVViewID is a stable CollectionFolder id for the Live TV library tab.
// Clients key primarily on CollectionType=livetv; the compact form matches how
// Jellyfin emits built-in views.
const liveTVViewID = "c8f3e1a0b2d44e6f9a1c3b5d7e9f0123"

var liveTVViewUUID = uuid.MustParse(liveTVViewID)

// LiveTVHandler wraps *livetv.Service with Jellyfin Live TV HTTP endpoints.
type LiveTVHandler struct {
	service    *livetv.Service
	codec      *ResourceIDCodec
	serverID   string
	httpClient *http.Client
	now        func() time.Time

	mu      sync.Mutex
	streams map[string]*openLiveStream
}

type openLiveStream struct {
	ID            string
	ChannelID     string
	NativeSession string
	SourceURL     string
	OpenedAt      time.Time
	OpenerToken   string
}

type liveTVInfoDTO struct {
	Services     []liveTVServiceInfoDTO `json:"Services"`
	IsEnabled    bool                   `json:"IsEnabled"`
	EnabledUsers []string               `json:"EnabledUsers"`
}

type liveTVServiceInfoDTO struct {
	Name               string   `json:"Name"`
	HomePageURL        string   `json:"HomePageUrl,omitempty"`
	Status             string   `json:"Status"`
	StatusMessage      string   `json:"StatusMessage,omitempty"`
	Version            string   `json:"Version,omitempty"`
	HasUpdateAvailable bool     `json:"HasUpdateAvailable"`
	IsVisible          bool     `json:"IsVisible"`
	Tuners             []string `json:"Tuners"`
}

type liveTVGuideInfoDTO struct {
	StartDate string `json:"StartDate"`
	EndDate   string `json:"EndDate"`
}

type liveStreamResponseDTO struct {
	MediaSource mediaSourceDTO `json:"MediaSource"`
}

type timerInfoDTO struct {
	ID                    string       `json:"Id"`
	Type                  string       `json:"Type"`
	ServerID              string       `json:"ServerId,omitempty"`
	ChannelID             string       `json:"ChannelId,omitempty"`
	ChannelName           string       `json:"ChannelName,omitempty"`
	ProgramID             string       `json:"ProgramId,omitempty"`
	Name                  string       `json:"Name"`
	Overview              string       `json:"Overview,omitempty"`
	StartDate             string       `json:"StartDate"`
	EndDate               string       `json:"EndDate"`
	Status                string       `json:"Status,omitempty"`
	SeriesTimerID         string       `json:"SeriesTimerId,omitempty"`
	RunTimeTicks          int64        `json:"RunTimeTicks,omitempty"`
	PrePaddingSeconds     int          `json:"PrePaddingSeconds"`
	PostPaddingSeconds    int          `json:"PostPaddingSeconds"`
	IsPrePaddingRequired  bool         `json:"IsPrePaddingRequired"`
	IsPostPaddingRequired bool         `json:"IsPostPaddingRequired"`
	KeepUntil             string       `json:"KeepUntil,omitempty"`
	ProgramInfo           *baseItemDTO `json:"ProgramInfo,omitempty"`
	RecordAnyTime         bool         `json:"RecordAnyTime,omitempty"`
	RecordAnyChannel      bool         `json:"RecordAnyChannel,omitempty"`
	RecordNewOnly         bool         `json:"RecordNewOnly,omitempty"`
	KeepUpTo              int          `json:"KeepUpTo,omitempty"`
	Days                  []string     `json:"Days,omitempty"`
}

type timerQueryResultDTO struct {
	Items            []timerInfoDTO `json:"Items"`
	TotalRecordCount int            `json:"TotalRecordCount"`
	StartIndex       int            `json:"StartIndex"`
}

type openLiveStreamRequestDTO struct {
	OpenToken          string `json:"OpenToken"`
	ItemID             string `json:"ItemId"`
	UserID             string `json:"UserId"`
	PlaySessionID      string `json:"PlaySessionId"`
	EnableDirectPlay   *bool  `json:"EnableDirectPlay"`
	EnableDirectStream *bool  `json:"EnableDirectStream"`
}

// NewLiveTVHandler creates a Jellyfin-compat Live TV handler.
func NewLiveTVHandler(service *livetv.Service, codec *ResourceIDCodec, cfg *config.Config) *LiveTVHandler {
	serverID := ""
	if cfg != nil {
		serverID = cfg.JellyfinCompat.ServerID
	}
	return &LiveTVHandler{
		service:  service,
		codec:    codec,
		serverID: serverID,
		httpClient: &http.Client{
			// No overall Timeout: live MPEG-TS proxies run indefinitely.
			Timeout: 0,
		},
		now:     time.Now,
		streams: make(map[string]*openLiveStream),
	}
}

func isLiveTVViewID(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := uuid.Parse(raw)
	return err == nil && parsed == liveTVViewUUID
}

func (h *LiveTVHandler) liveTVView() baseItemDTO {
	return baseItemDTO{
		ID:             liveTVViewID,
		Type:           "CollectionFolder",
		CollectionType: "livetv",
		MediaType:      "Unknown",
		IsFolder:       true,
		Name:           "Live TV",
		ServerID:       h.serverID,
		SortName:       "live tv",
		ImageTags:      map[string]string{},
		UserData: &itemUserDataDTO{
			Key:    liveTVViewID,
			ItemID: liveTVViewID,
		},
	}
}

// HandleInfo serves GET /LiveTv/Info.
func (h *LiveTVHandler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	tuners, err := h.service.ListTuners(r.Context())
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	tunerNames := make([]string, 0, len(tuners))
	for _, t := range tuners {
		label := t.DeviceID
		if t.Model != "" {
			label = t.Model + " (" + t.DeviceID + ")"
		}
		tunerNames = append(tunerNames, label)
	}
	status := "Ok"
	if len(tuners) == 0 {
		status = "Unavailable"
	}
	writeJSON(w, http.StatusOK, liveTVInfoDTO{
		IsEnabled:    true,
		EnabledUsers: []string{},
		Services: []liveTVServiceInfoDTO{{
			Name:      "Prairie",
			Status:    status,
			IsVisible: true,
			Tuners:    tunerNames,
		}},
	})
}

// HandleGuideInfo serves GET /LiveTv/GuideInfo.
func (h *LiveTVHandler) HandleGuideInfo(w http.ResponseWriter, r *http.Request) {
	now := h.now().UTC()
	writeJSON(w, http.StatusOK, liveTVGuideInfoDTO{
		StartDate: now.Add(-12 * time.Hour).Format(time.RFC3339Nano),
		EndDate:   now.Add(14 * 24 * time.Hour).Format(time.RFC3339Nano),
	})
}

// HandleChannels serves GET /LiveTv/Channels.
func (h *LiveTVHandler) HandleChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.service.ListChannels(r.Context(), "")
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	addCurrent := strings.EqualFold(r.URL.Query().Get("AddCurrentProgram"), "true") ||
		r.URL.Query().Get("addCurrentProgram") == "true"
	startIndex, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))
	if startIndex < 0 {
		startIndex = 0
	}

	items := make([]baseItemDTO, 0, len(channels))
	now := h.now().UTC()
	var currentByChannel map[string]livetv.Program
	if addCurrent {
		programs, gerr := h.service.ListGuide(r.Context(), nil, now.Add(-6*time.Hour), now.Add(6*time.Hour))
		if gerr == nil {
			currentByChannel = map[string]livetv.Program{}
			for _, p := range programs {
				if !p.Start.After(now) && p.Stop.After(now) {
					currentByChannel[p.ChannelID] = p
				}
			}
		}
	}
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		dto := h.channelDTO(ch)
		if addCurrent {
			if p, ok := currentByChannel[ch.ID]; ok {
				prog := h.programDTO(p)
				dto.CurrentProgram = &prog
			}
		}
		items = append(items, dto)
	}
	total := len(items)
	items = sliceBaseItems(items, startIndex, limit)
	writeJSON(w, http.StatusOK, queryResultDTO{
		Items:            items,
		TotalRecordCount: total,
		StartIndex:       startIndex,
	})
}

// HandleChannel serves GET /LiveTv/Channels/{id}.
func (h *LiveTVHandler) HandleChannel(w http.ResponseWriter, r *http.Request) {
	channelID, err := h.decodeChannelID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "NotFound", "Channel not found")
		return
	}
	ch, err := h.service.GetChannel(r.Context(), channelID)
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.channelDTO(*ch))
}

// HandlePrograms serves GET|POST /LiveTv/Programs.
func (h *LiveTVHandler) HandlePrograms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			for k, v := range body {
				if s, ok := v.(string); ok && q.Get(k) == "" {
					q.Set(k, s)
				}
			}
		}
	}
	channelIDs := h.parseChannelIDs(q.Get("ChannelIds"))
	start := parseFlexibleTime(firstNonEmpty(q.Get("MinStartDate"), q.Get("StartDate")), h.now().UTC().Add(-1*time.Hour))
	end := parseFlexibleTime(firstNonEmpty(q.Get("MaxEndDate"), q.Get("EndDate")), h.now().UTC().Add(24*time.Hour))
	programs, err := h.service.ListGuide(r.Context(), channelIDs, start, end)
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	startIndex, _ := strconv.Atoi(q.Get("StartIndex"))
	limit, _ := strconv.Atoi(q.Get("Limit"))
	items := make([]baseItemDTO, 0, len(programs))
	for _, p := range programs {
		items = append(items, h.programDTO(p))
	}
	total := len(items)
	items = sliceBaseItems(items, startIndex, limit)
	writeJSON(w, http.StatusOK, queryResultDTO{
		Items:            items,
		TotalRecordCount: total,
		StartIndex:       startIndex,
	})
}

// HandleProgram serves GET /LiveTv/Programs/{id}.
func (h *LiveTVHandler) HandleProgram(w http.ResponseWriter, r *http.Request) {
	programID, err := h.decodeProgramID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "NotFound", "Program not found")
		return
	}
	p, err := h.service.GetProgram(r.Context(), programID)
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.programDTO(*p))
}

// HandleRecommendedPrograms serves GET /LiveTv/Programs/Recommended.
func (h *LiveTVHandler) HandleRecommendedPrograms(w http.ResponseWriter, r *http.Request) {
	now := h.now().UTC()
	programs, err := h.service.ListGuide(r.Context(), nil, now.Add(-2*time.Hour), now.Add(6*time.Hour))
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))
	items := make([]baseItemDTO, 0)
	for _, p := range programs {
		if p.Stop.After(now) {
			items = append(items, h.programDTO(p))
		}
	}
	total := len(items)
	items = sliceBaseItems(items, 0, limit)
	writeJSON(w, http.StatusOK, queryResultDTO{
		Items:            items,
		TotalRecordCount: total,
		StartIndex:       0,
	})
}

// livetvOwner resolves the mapped Prairie app user for Live TV ownership checks.
// Fail closed: unmapped / missing sessions are unauthorized (same as stream file).
func (h *LiveTVHandler) livetvOwner(w http.ResponseWriter, r *http.Request) (userID int, profileID string, ok bool) {
	session := SessionFromContext(r.Context())
	if session == nil || session.StreamAppUserID == 0 {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing authentication token")
		return 0, "", false
	}
	return session.StreamAppUserID, session.ProfileID, true
}

// HandleTimers serves GET /LiveTv/Timers and POST /LiveTv/Timers.
func (h *LiveTVHandler) HandleTimers(w http.ResponseWriter, r *http.Request) {
	userID, profileID, ok := h.livetvOwner(w, r)
	if !ok {
		return
	}
	enforceOwner := true
	switch r.Method {
	case http.MethodGet:
		recs, err := h.service.ListRecordings(r.Context(), "", userID, profileID, enforceOwner)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		items := make([]timerInfoDTO, 0, len(recs))
		for _, rec := range recs {
			if rec.Status == "cancelled" {
				continue
			}
			items = append(items, h.timerDTO(rec))
		}
		writeJSON(w, http.StatusOK, timerQueryResultDTO{
			Items:            items,
			TotalRecordCount: len(items),
		})
	case http.MethodPost:
		var body struct {
			ProgramID string `json:"ProgramId"`
			ChannelID string `json:"ChannelId"`
			Name      string `json:"Name"`
			StartDate string `json:"StartDate"`
			EndDate   string `json:"EndDate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "BadRequest", "Invalid request body")
			return
		}
		rec := &livetv.Recording{Title: body.Name, UserID: userID, ProfileID: profileID}
		if body.ProgramID != "" {
			if id, err := h.decodeProgramID(body.ProgramID); err == nil {
				rec.ProgramID = id
			} else {
				rec.ProgramID = body.ProgramID
			}
		}
		if body.ChannelID != "" {
			if id, err := h.decodeChannelID(body.ChannelID); err == nil {
				rec.ChannelID = id
			}
		}
		if body.StartDate != "" {
			rec.Start = parseFlexibleTime(body.StartDate, time.Time{})
		}
		if body.EndDate != "" {
			rec.Stop = parseFlexibleTime(body.EndDate, time.Time{})
		}
		created, err := h.service.ScheduleRecording(r.Context(), rec)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, h.timerDTO(*created))
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

// HandleTimer serves GET|POST|DELETE /LiveTv/Timers/{id}.
func (h *LiveTVHandler) HandleTimer(w http.ResponseWriter, r *http.Request) {
	userID, profileID, ok := h.livetvOwner(w, r)
	if !ok {
		return
	}
	enforceOwner := true
	timerID, err := h.decodeTimerID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "NotFound", "Timer not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := h.findRecording(r.Context(), timerID, userID, profileID, enforceOwner)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		if rec == nil {
			writeError(w, http.StatusNotFound, "NotFound", "Timer not found")
			return
		}
		writeJSON(w, http.StatusOK, h.timerDTO(*rec))
	case http.MethodPost:
		// Jellyfin updates timers in place; Prairie cancels and recreates when
		// schedule fields change. Absent body fields keep the existing timer.
		existing, err := h.findRecording(r.Context(), timerID, userID, profileID, enforceOwner)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		if existing == nil {
			writeError(w, http.StatusNotFound, "NotFound", "Timer not found")
			return
		}
		writeJSON(w, http.StatusOK, h.timerDTO(*existing))
	case http.MethodDelete:
		if _, err := h.service.CancelRecording(r.Context(), timerID, userID, profileID, enforceOwner); err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

// HandleSeriesTimers serves GET /LiveTv/SeriesTimers and POST /LiveTv/SeriesTimers.
func (h *LiveTVHandler) HandleSeriesTimers(w http.ResponseWriter, r *http.Request) {
	userID, profileID, ok := h.livetvOwner(w, r)
	if !ok {
		return
	}
	enforceOwner := true
	switch r.Method {
	case http.MethodGet:
		rules, err := h.service.ListSeriesRules(r.Context(), userID, profileID, enforceOwner)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		items := make([]timerInfoDTO, 0, len(rules))
		for _, rule := range rules {
			items = append(items, h.seriesTimerDTO(rule))
		}
		writeJSON(w, http.StatusOK, timerQueryResultDTO{
			Items:            items,
			TotalRecordCount: len(items),
		})
	case http.MethodPost:
		var body struct {
			Name             string `json:"Name"`
			SeriesID         string `json:"SeriesId"`
			ChannelID        string `json:"ChannelId"`
			RecordNewOnly    bool   `json:"RecordNewOnly"`
			KeepUpTo         int    `json:"KeepUpTo"`
			RecordAnyTime    bool   `json:"RecordAnyTime"`
			RecordAnyChannel bool   `json:"RecordAnyChannel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "BadRequest", "Invalid request body")
			return
		}
		rule := &livetv.SeriesRule{
			SeriesID:   body.SeriesID,
			UserID:     userID,
			ProfileID:  profileID,
			TitleMatch: body.Name,
			NewOnly:    body.RecordNewOnly,
			KeepLast:   body.KeepUpTo,
			Enabled:    true,
		}
		if body.ChannelID != "" && !body.RecordAnyChannel {
			if id, err := h.decodeChannelID(body.ChannelID); err == nil {
				rule.ChannelID = &id
			}
		}
		created, err := h.service.CreateSeriesRule(r.Context(), rule)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, h.seriesTimerDTO(*created))
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

// HandleSeriesTimer serves GET|POST|DELETE /LiveTv/SeriesTimers/{id}.
func (h *LiveTVHandler) HandleSeriesTimer(w http.ResponseWriter, r *http.Request) {
	userID, profileID, ok := h.livetvOwner(w, r)
	if !ok {
		return
	}
	enforceOwner := true
	ruleID, err := h.decodeSeriesTimerID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "NotFound", "Series timer not found")
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodPost:
		rule, err := h.findSeriesRule(r.Context(), ruleID, userID, profileID, enforceOwner)
		if err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		if rule == nil {
			writeError(w, http.StatusNotFound, "NotFound", "Series timer not found")
			return
		}
		writeJSON(w, http.StatusOK, h.seriesTimerDTO(*rule))
	case http.MethodDelete:
		if err := h.service.DeleteSeriesRule(r.Context(), ruleID, userID, profileID, enforceOwner); err != nil {
			writeLiveTVCompatError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

// HandleRecordings serves GET /LiveTv/Recordings.
func (h *LiveTVHandler) HandleRecordings(w http.ResponseWriter, r *http.Request) {
	userID, profileID, ok := h.livetvOwner(w, r)
	if !ok {
		return
	}
	enforceOwner := true
	recs, err := h.service.ListRecordings(r.Context(), "completed", userID, profileID, enforceOwner)
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	items := make([]baseItemDTO, 0, len(recs))
	for _, rec := range recs {
		chID := h.codec.EncodeStringID(EncodedIDLiveTVChannel, rec.ChannelID)
		items = append(items, baseItemDTO{
			ID:           h.codec.EncodeStringID(EncodedIDLiveTVTimer, rec.ID),
			ServerID:     h.serverID,
			Type:         "Recording",
			Name:         rec.Title,
			ChannelID:    &chID,
			StartDate:    rec.Start.UTC().Format(time.RFC3339Nano),
			EndDate:      rec.Stop.UTC().Format(time.RFC3339Nano),
			RunTimeTicks: rec.Stop.Sub(rec.Start).Nanoseconds() / 100,
			ImageTags:    map[string]string{},
			UserData:     &itemUserDataDTO{Key: rec.ID, ItemID: h.codec.EncodeStringID(EncodedIDLiveTVTimer, rec.ID)},
			LocationType: "FileSystem",
			MediaType:    streamTypeVideo,
		})
	}
	writeJSON(w, http.StatusOK, queryResultDTO{
		Items:            items,
		TotalRecordCount: len(items),
	})
}

// HandleOpenLiveStream serves POST /LiveStreams/Open.
func (h *LiveTVHandler) HandleOpenLiveStream(w http.ResponseWriter, r *http.Request) {
	session := SessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing authentication token")
		return
	}
	var body openLiveStreamRequestDTO
	_ = json.NewDecoder(r.Body).Decode(&body)
	openToken := firstNonEmpty(r.URL.Query().Get("OpenToken"), body.OpenToken)
	itemID := firstNonEmpty(r.URL.Query().Get("ItemId"), body.ItemID)
	channelID := ""
	if openToken != "" {
		if id, err := h.decodeChannelID(openToken); err == nil {
			channelID = id
		}
	}
	if channelID == "" && itemID != "" {
		if id, err := h.decodeChannelID(itemID); err == nil {
			channelID = id
		}
	}
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "BadRequest", "OpenToken or ItemId with a Live TV channel is required")
		return
	}
	opened, err := h.openChannelStream(r.Context(), session, channelID)
	if err != nil {
		writeLiveTVCompatError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, liveStreamResponseDTO{MediaSource: opened})
}

// HandleCloseLiveStream serves POST /LiveStreams/Close.
func (h *LiveTVHandler) HandleCloseLiveStream(w http.ResponseWriter, r *http.Request) {
	session := SessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing authentication token")
		return
	}
	liveStreamID := r.URL.Query().Get("LiveStreamId")
	if liveStreamID == "" {
		writeError(w, http.StatusBadRequest, "BadRequest", "LiveStreamId is required")
		return
	}
	h.mu.Lock()
	stream := h.streams[liveStreamID]
	h.mu.Unlock()
	if stream != nil && stream.OpenerToken != "" && stream.OpenerToken != session.Token {
		writeError(w, http.StatusForbidden, "Forbidden", "Live stream belongs to another session")
		return
	}
	h.closeLiveStream(r.Context(), liveStreamID)
	w.WriteHeader(http.StatusNoContent)
}

// HandleLiveStreamFile serves GET /LiveTv/LiveStreamFiles/{id}/stream[.{container}].
func (h *LiveTVHandler) HandleLiveStreamFile(w http.ResponseWriter, r *http.Request) {
	streamID := chi.URLParam(r, "id")
	if streamID == "" {
		streamID = chi.URLParam(r, "streamId")
	}
	session := SessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing authentication token")
		return
	}
	h.mu.Lock()
	stream, ok := h.streams[streamID]
	h.mu.Unlock()
	if !ok || stream == nil || stream.SourceURL == "" {
		writeError(w, http.StatusNotFound, "NotFound", "Live stream not found")
		return
	}
	// Same ownership gate as Close: knowing the UUID is not enough to pull or
	// terminate another client's upstream tuner session.
	if stream.OpenerToken != "" && stream.OpenerToken != session.Token {
		writeError(w, http.StatusForbidden, "Forbidden", "Live stream belongs to another session")
		return
	}
	if err := livetv.ValidateMediaFetchURL(stream.SourceURL); err != nil {
		writeError(w, http.StatusBadGateway, "BadGateway", "Live stream source is not allowed")
		return
	}

	w.Header().Set("Content-Type", "video/mp2t")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	sourceURL := stream.SourceURL
	defer h.closeLiveStream(r.Context(), streamID)

	err := copyLiveStreamWithReconnect(r.Context(), w, flusher, func(ctx context.Context) (io.ReadCloser, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := h.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
		}
		return resp.Body, nil
	}, liveStreamReconnectOpts{})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		slog.WarnContext(r.Context(), "jellycompat livetv stream ended",
			"component", "jellycompat",
			"live_stream_id", streamID,
			"error", err,
		)
	}
}

// PlaybackMediaSource builds an infinite-stream MediaSource for a Live TV channel.
// When autoOpen is true the upstream tuner session is opened here (Jellyfin
// AutoOpenLiveStream pattern) so RequiresOpening can be false and DirectStreamUrl
// is immediately usable.
func (h *LiveTVHandler) PlaybackMediaSource(ctx context.Context, session *Session, channelRouteID string, autoOpen bool, existingLiveStreamID string) (mediaSourceDTO, error) {
	channelID, err := h.decodeChannelID(channelRouteID)
	if err != nil {
		return mediaSourceDTO{}, err
	}
	ch, err := h.service.GetChannel(ctx, channelID)
	if err != nil {
		return mediaSourceDTO{}, err
	}
	sourceID := h.codec.EncodeStringID(EncodedIDLiveTVChannel, ch.ID)
	openToken := sourceID
	dto := mediaSourceDTO{
		Protocol:             "Http",
		ID:                   sourceID,
		Path:                 "",
		Type:                 "Default",
		Container:            "ts",
		Name:                 channelDisplayName(*ch),
		IsRemote:             true,
		SupportsTranscoding:  false,
		SupportsDirectStream: true,
		SupportsDirectPlay:   true,
		IsInfiniteStream:     true,
		RequiresOpening:      true,
		RequiresClosing:      true,
		OpenToken:            openToken,
		Formats:              []string{},
		RequiredHTTPHeaders:  map[string]string{},
		MediaAttachments:     []map[string]any{},
		MediaStreams: []mediaStreamDTO{{
			Index:        0,
			Type:         streamTypeVideo,
			Codec:        "mpeg2video",
			IsDefault:    true,
			DisplayTitle: streamTypeVideo,
		}},
	}
	if autoOpen {
		if existingLiveStreamID != "" {
			if reused, ok := h.mediaSourceForOpenStream(ctx, existingLiveStreamID, ch.ID); ok {
				return reused, nil
			}
		}
		opened, err := h.openChannelStream(ctx, session, ch.ID)
		if err != nil {
			return mediaSourceDTO{}, err
		}
		return opened, nil
	}
	return dto, nil
}

func (h *LiveTVHandler) DecodeLiveTVChannelID(raw string) (string, bool) {
	id, err := h.decodeChannelID(raw)
	return id, err == nil
}

func (h *LiveTVHandler) mediaSourceForOpenStream(ctx context.Context, liveStreamID, channelID string) (mediaSourceDTO, bool) {
	h.mu.Lock()
	stream := h.streams[liveStreamID]
	h.mu.Unlock()
	if stream == nil || stream.ChannelID != channelID || stream.SourceURL == "" {
		return mediaSourceDTO{}, false
	}
	ch, _ := h.service.GetChannel(ctx, channelID)
	name := channelID
	if ch != nil {
		name = channelDisplayName(*ch)
	}
	directURL := "/LiveTv/LiveStreamFiles/" + liveStreamID + "/stream.ts"
	if stream.OpenerToken != "" {
		directURL += "?api_key=" + url.QueryEscape(stream.OpenerToken)
	}
	return mediaSourceDTO{
		Protocol:             "Http",
		ID:                   h.codec.EncodeStringID(EncodedIDLiveTVChannel, channelID),
		Path:                 "",
		Type:                 "Default",
		Container:            "ts",
		Name:                 name,
		IsRemote:             true,
		SupportsTranscoding:  false,
		SupportsDirectStream: true,
		SupportsDirectPlay:   true,
		IsInfiniteStream:     true,
		RequiresOpening:      false,
		RequiresClosing:      true,
		LiveStreamID:         liveStreamID,
		DirectStreamURL:      directURL,
		Formats:              []string{},
		RequiredHTTPHeaders:  map[string]string{},
		MediaAttachments:     []map[string]any{},
		MediaStreams: []mediaStreamDTO{{
			Index:        0,
			Type:         streamTypeVideo,
			Codec:        "mpeg2video",
			IsDefault:    true,
			DisplayTitle: streamTypeVideo,
		}},
	}, true
}

func (h *LiveTVHandler) openChannelStream(ctx context.Context, session *Session, channelID string) (mediaSourceDTO, error) {
	userID := 0
	profileID := ""
	if session != nil {
		userID = session.StreamAppUserID
		profileID = session.ProfileID
	}
	native, err := h.service.StartChannelSession(ctx, channelID, userID, profileID)
	if err != nil {
		return mediaSourceDTO{}, err
	}
	liveStreamID := uuid.NewString()
	// Compat clients always consume MPEG-TS via HandleLiveStreamFile. Keep the
	// upstream tuner URL even when the native PlaybackBridge remuxes to HLS.
	sourceURL, resolveErr := h.service.ResolveSessionUpstreamURL(ctx, native.ID)
	if resolveErr != nil || sourceURL == "" {
		_, _ = h.service.ReleaseSession(ctx, native.ID, userID, profileID, false)
		if resolveErr != nil {
			return mediaSourceDTO{}, resolveErr
		}
		return mediaSourceDTO{}, livetv.ErrNotFound
	}
	openerToken := ""
	if session != nil {
		openerToken = session.Token
	}
	h.mu.Lock()
	h.streams[liveStreamID] = &openLiveStream{
		ID:            liveStreamID,
		ChannelID:     channelID,
		NativeSession: native.ID,
		SourceURL:     sourceURL,
		OpenedAt:      h.now(),
		OpenerToken:   openerToken,
	}
	h.mu.Unlock()

	ch, _ := h.service.GetChannel(ctx, channelID)
	name := channelID
	if ch != nil {
		name = channelDisplayName(*ch)
	}
	directURL := "/LiveTv/LiveStreamFiles/" + liveStreamID + "/stream.ts"
	if openerToken != "" {
		directURL += "?api_key=" + url.QueryEscape(openerToken)
	}
	return mediaSourceDTO{
		Protocol:             "Http",
		ID:                   h.codec.EncodeStringID(EncodedIDLiveTVChannel, channelID),
		Path:                 "",
		Type:                 "Default",
		Container:            "ts",
		Name:                 name,
		IsRemote:             true,
		SupportsTranscoding:  false,
		SupportsDirectStream: true,
		SupportsDirectPlay:   true,
		IsInfiniteStream:     true,
		RequiresOpening:      false,
		RequiresClosing:      true,
		LiveStreamID:         liveStreamID,
		DirectStreamURL:      directURL,
		Formats:              []string{},
		RequiredHTTPHeaders:  map[string]string{},
		MediaAttachments:     []map[string]any{},
		MediaStreams: []mediaStreamDTO{{
			Index:        0,
			Type:         streamTypeVideo,
			Codec:        "mpeg2video",
			IsDefault:    true,
			DisplayTitle: streamTypeVideo,
		}},
	}, nil
}

func (h *LiveTVHandler) closeLiveStream(ctx context.Context, liveStreamID string) {
	h.mu.Lock()
	stream := h.streams[liveStreamID]
	delete(h.streams, liveStreamID)
	h.mu.Unlock()
	if stream == nil {
		return
	}
	if stream.NativeSession != "" {
		_, _ = h.service.ReleaseSession(ctx, stream.NativeSession, 0, "", false)
	}
}

func (h *LiveTVHandler) channelDTO(ch livetv.Channel) baseItemDTO {
	id := h.codec.EncodeStringID(EncodedIDLiveTVChannel, ch.ID)
	number := channelNumber(ch)
	return baseItemDTO{
		ID:            id,
		ServerID:      h.serverID,
		Type:          "TvChannel",
		Name:          channelDisplayName(ch),
		Number:        number,
		ChannelNumber: number,
		IsFolder:      false,
		IsHD:          ch.HD,
		MediaType:     streamTypeVideo,
		LocationType:  "Remote",
		ChannelID:     &id,
		ImageTags:     map[string]string{},
		UserData: &itemUserDataDTO{
			Key:    ch.ID,
			ItemID: id,
		},
		PlayAccess: "Full",
	}
}

func (h *LiveTVHandler) programDTO(p livetv.Program) baseItemDTO {
	id := h.codec.EncodeStringID(EncodedIDLiveTVProgram, p.ID)
	chID := h.codec.EncodeStringID(EncodedIDLiveTVChannel, p.ChannelID)
	return baseItemDTO{
		ID:                id,
		ServerID:          h.serverID,
		Type:              "Program",
		Name:              p.Title,
		Overview:          p.Description,
		EpisodeTitle:      p.Subtitle,
		ChannelID:         &chID,
		StartDate:         p.Start.UTC().Format(time.RFC3339Nano),
		EndDate:           p.Stop.UTC().Format(time.RFC3339Nano),
		RunTimeTicks:      p.Stop.Sub(p.Start).Nanoseconds() / 100,
		IsLive:            p.IsLive,
		IsNew:             p.IsNew,
		Genres:            p.Genres,
		IndexNumber:       p.Episode,
		ParentIndexNumber: p.Season,
		SeriesID:          p.SeriesID,
		ImageTags:         map[string]string{},
		UserData: &itemUserDataDTO{
			Key:    p.ID,
			ItemID: id,
		},
		MediaType:    streamTypeVideo,
		LocationType: "Remote",
	}
}

func (h *LiveTVHandler) timerDTO(rec livetv.Recording) timerInfoDTO {
	return timerInfoDTO{
		ID:            h.codec.EncodeStringID(EncodedIDLiveTVTimer, rec.ID),
		Type:          "Timer",
		ServerID:      h.serverID,
		ChannelID:     h.codec.EncodeStringID(EncodedIDLiveTVChannel, rec.ChannelID),
		ProgramID:     encodeOptional(h.codec, EncodedIDLiveTVProgram, rec.ProgramID),
		Name:          rec.Title,
		StartDate:     rec.Start.UTC().Format(time.RFC3339Nano),
		EndDate:       rec.Stop.UTC().Format(time.RFC3339Nano),
		Status:        mapRecordingStatus(rec.Status),
		SeriesTimerID: encodeOptional(h.codec, EncodedIDLiveTVSeriesTimer, rec.SeriesRuleID),
		RunTimeTicks:  rec.Stop.Sub(rec.Start).Nanoseconds() / 100,
		KeepUntil:     "UntilDeleted",
	}
}

func (h *LiveTVHandler) seriesTimerDTO(rule livetv.SeriesRule) timerInfoDTO {
	dto := timerInfoDTO{
		ID:            h.codec.EncodeStringID(EncodedIDLiveTVSeriesTimer, rule.ID),
		Type:          "SeriesTimer",
		ServerID:      h.serverID,
		Name:          firstNonEmpty(rule.TitleMatch, rule.SeriesID),
		RecordNewOnly: rule.NewOnly,
		KeepUpTo:      rule.KeepLast,
		KeepUntil:     "UntilDeleted",
		RecordAnyTime: rule.ChannelID == nil,
		Days:          []string{},
	}
	if rule.ChannelID != nil {
		dto.ChannelID = h.codec.EncodeStringID(EncodedIDLiveTVChannel, *rule.ChannelID)
	} else {
		dto.RecordAnyChannel = true
	}
	return dto
}

func (h *LiveTVHandler) findRecording(ctx context.Context, id string, userID int, profileID string, enforceOwner bool) (*livetv.Recording, error) {
	recs, err := h.service.ListRecordings(ctx, "", userID, profileID, enforceOwner)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		if recs[i].ID == id {
			return &recs[i], nil
		}
	}
	return nil, nil
}

func (h *LiveTVHandler) findSeriesRule(ctx context.Context, id string, userID int, profileID string, enforceOwner bool) (*livetv.SeriesRule, error) {
	rules, err := h.service.ListSeriesRules(ctx, userID, profileID, enforceOwner)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i], nil
		}
	}
	return nil, nil
}

func (h *LiveTVHandler) decodeChannelID(raw string) (string, error) {
	return h.codec.DecodeStringID(EncodedIDLiveTVChannel, raw)
}

func (h *LiveTVHandler) decodeProgramID(raw string) (string, error) {
	return h.codec.DecodeStringID(EncodedIDLiveTVProgram, raw)
}

func (h *LiveTVHandler) decodeTimerID(raw string) (string, error) {
	return h.codec.DecodeStringID(EncodedIDLiveTVTimer, raw)
}

func (h *LiveTVHandler) decodeSeriesTimerID(raw string) (string, error) {
	return h.codec.DecodeStringID(EncodedIDLiveTVSeriesTimer, raw)
}

func (h *LiveTVHandler) parseChannelIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := h.decodeChannelID(part); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func channelNumber(ch livetv.Channel) string {
	if ch.NumberOverride != nil && strings.TrimSpace(*ch.NumberOverride) != "" {
		return *ch.NumberOverride
	}
	return ch.Number
}

func channelDisplayName(ch livetv.Channel) string {
	if strings.TrimSpace(ch.Name) != "" {
		return ch.Name
	}
	if strings.TrimSpace(ch.Callsign) != "" {
		return ch.Callsign
	}
	return channelNumber(ch)
}

func mapRecordingStatus(status string) string {
	switch strings.ToLower(status) {
	case "scheduled":
		return "New"
	case "recording", "in_progress":
		return "InProgress"
	case "completed":
		return "Completed"
	case "cancelled", "canceled":
		return "Cancelled"
	case "failed", "error":
		return "Error"
	default:
		return "New"
	}
}

func encodeOptional(codec *ResourceIDCodec, kind EncodedIDType, value string) string {
	if value == "" {
		return ""
	}
	return codec.EncodeStringID(kind, value)
}

func parseFlexibleTime(raw string, fallback time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	return fallback
}

func writeLiveTVCompatError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, livetv.ErrNotFound):
		writeError(w, http.StatusNotFound, "NotFound", err.Error())
	case errors.Is(err, livetv.ErrInvalidArgument):
		writeError(w, http.StatusBadRequest, "BadRequest", err.Error())
	case errors.Is(err, livetv.ErrNoTuner):
		writeError(w, http.StatusConflict, "Conflict", err.Error())
	case errors.Is(err, livetv.ErrLimitExceeded):
		writeError(w, http.StatusConflict, "Conflict", err.Error())
	case errors.Is(err, livetv.ErrNotImplemented):
		writeError(w, http.StatusNotImplemented, "NotImplemented", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "InternalError", err.Error())
	}
}

type liveStreamReconnectOpts struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxRetries     int // 0 = unlimited while ctx is alive
}

// copyLiveStreamWithReconnect copies an MPEG-TS live upstream to dst, recreating
// the upstream reader when reads fail.
//
// jellyfin#11415: HDHomeRun (and similar tuner) streams can emit corrupt TS
// during weak signal; stock Jellyfin treats the upstream EOF/error as terminal
// and the client stays on a black screen until the channel is reopened. Prairie
// keeps the client connection and liveStreamId stable, backing off briefly and
// reopening the source URL so playback can recover without a full retune dance.
func copyLiveStreamWithReconnect(
	ctx context.Context,
	dst io.Writer,
	flusher http.Flusher,
	openReader func(context.Context) (io.ReadCloser, error),
	opts liveStreamReconnectOpts,
) error {
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = 250 * time.Millisecond
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 5 * time.Second
	}
	backoff := opts.InitialBackoff
	attempts := 0
	buf := make([]byte, 32*1024)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		reader, err := openReader(ctx)
		if err != nil {
			attempts++
			if opts.MaxRetries > 0 && attempts >= opts.MaxRetries {
				return err
			}
			if waitErr := waitBackoff(ctx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = nextBackoff(backoff, opts.MaxBackoff)
			continue
		}

		readErr := copyLiveStreamOnce(ctx, dst, flusher, reader, buf)
		_ = reader.Close()
		if readErr == nil || errors.Is(readErr, io.EOF) {
			// Clean EOF from a live source is unexpected; reconnect.
			readErr = io.ErrUnexpectedEOF
		}
		if errors.Is(readErr, context.Canceled) {
			return readErr
		}
		// Client gone.
		if errors.Is(readErr, io.ErrClosedPipe) {
			return readErr
		}
		attempts++
		if opts.MaxRetries > 0 && attempts >= opts.MaxRetries {
			return readErr
		}
		if waitErr := waitBackoff(ctx, backoff); waitErr != nil {
			return waitErr
		}
		backoff = nextBackoff(backoff, opts.MaxBackoff)
	}
}

func copyLiveStreamOnce(ctx context.Context, dst io.Writer, flusher http.Flusher, src io.Reader, buf []byte) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return readErr
		}
	}
}

func waitBackoff(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
