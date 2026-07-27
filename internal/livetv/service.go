package livetv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/livetv/hdhomerun"
)

var (
	ErrNotFound           = errors.New("livetv: not found")
	ErrInvalidArgument    = errors.New("livetv: invalid argument")
	ErrLimitExceeded      = errors.New("livetv: limit exceeded")
	ErrNoTuner            = errors.New("livetv: no tuner available")
	ErrTunerIndexConflict = errors.New("livetv: tuner index conflict")
	ErrNotImplemented     = errors.New("livetv: not implemented")
	ErrNotConfigured      = errors.New("livetv service not configured")
)

type HDHomeRunClient interface {
	Discover(ctx context.Context, discoverURL, deviceID string) (*hdhomerun.DeviceInfo, error)
	FetchLineup(ctx context.Context, baseURL string) ([]hdhomerun.LineupChannel, error)
}

type PlaybackBridge interface {
	StartLiveStream(ctx context.Context, channelID, sourceStreamURL string, userID int, profileID string) (playbackSessionID, playbackURL string, err error)
}

type Service struct {
	store          Store
	hdhr           HDHomeRunClient
	httpClient     *http.Client
	playbackBridge PlaybackBridge
	now            func() time.Time
}

func NewService(db *pgxpool.Pool) *Service {
	var store Store
	if db != nil {
		store = NewPgStore(db)
	}
	return NewServiceWithStore(store)
}

func NewServiceWithStore(store Store) *Service {
	httpClient := NewMediaHTTPClient()
	return &Service{
		store:      store,
		hdhr:       hdhomerun.NewClient(httpClient),
		httpClient: httpClient,
		now:        time.Now,
	}
}

// PublicSessionStreamPath is the authenticated relative URL clients should play.
func PublicSessionStreamPath(sessionID string) string {
	return "/api/v1/livetv/sessions/" + sessionID + "/stream"
}

// IsClientSafePlayURL reports whether url is safe to return to clients (relative
// or same-app path). Absolute remote tuner URLs are not.
func IsClientSafePlayURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return true
	}
	return false
}

func (s *Service) SetHDHomeRunClient(client HDHomeRunClient) {
	s.hdhr = client
}

func (s *Service) SetPlaybackBridge(bridge PlaybackBridge) {
	s.playbackBridge = bridge
}

func (s *Service) requireStore() error {
	if s == nil || s.store == nil {
		return ErrNotConfigured
	}
	return nil
}

func (s *Service) ListTuners(ctx context.Context) ([]Tuner, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.ListTuners(ctx)
}

// lanDiscoverFn is overridden in tests.
var lanDiscoverFn = hdhomerun.DiscoverLAN

// DiscoverTuners finds HDHomeRun tuners via UDP and/or probes Dispatcharr/HDHR URLs.
func (s *Service) DiscoverTuners(ctx context.Context, req DiscoverTunersRequest) (*DiscoverTunersResult, error) {
	if s.hdhr == nil {
		return nil, ErrNotConfigured
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	if timeoutMs > 10_000 {
		timeoutMs = 10_000
	}
	includeUDP := true
	if req.IncludeUDP != nil {
		includeUDP = *req.IncludeUDP
	}

	existing := map[string]struct{}{}
	if err := s.requireStore(); err == nil {
		if tuners, listErr := s.store.ListTuners(ctx); listErr == nil {
			for _, t := range tuners {
				if id := strings.ToUpper(strings.TrimSpace(t.DeviceID)); id != "" {
					existing[id] = struct{}{}
				}
				if base := strings.TrimRight(strings.TrimSpace(t.BaseURL), "/"); base != "" {
					existing[strings.ToLower(base)] = struct{}{}
				}
			}
		}
	}

	seen := map[string]struct{}{}
	var candidates []DiscoveredTuner
	var notes []string
	add := func(c DiscoveredTuner) {
		key := strings.ToUpper(c.DeviceID) + "|" + strings.ToLower(strings.TrimRight(c.BaseURL, "/"))
		if key == "|" {
			key = strings.ToLower(c.DiscoverURL)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if _, ok := existing[strings.ToUpper(c.DeviceID)]; ok {
			c.AlreadyAdded = true
		}
		if _, ok := existing[strings.ToLower(strings.TrimRight(c.BaseURL, "/"))]; ok {
			c.AlreadyAdded = true
		}
		candidates = append(candidates, c)
	}

	if includeUDP {
		lan, err := lanDiscoverFn(ctx, time.Duration(timeoutMs)*time.Millisecond)
		if err != nil {
			notes = append(notes, "UDP discovery unavailable: "+err.Error())
		} else if len(lan) == 0 {
			notes = append(notes, "No tuners answered UDP discovery. Bridge-mode Docker blocks LAN broadcast — use docker-compose.livetv.yml (Linux host networking) or probe a URL. See docs/livetv-tuner-discovery.md.")
		}
		for _, item := range lan {
			discoverURL := hdhomerun.DiscoverURLForBase(item.BaseURL)
			if discoverURL == "" {
				discoverURL = "http://" + item.RemoteIP + "/discover.json"
			}
			if err := ValidateMediaFetchURL(discoverURL); err != nil {
				continue
			}
			info, err := s.hdhr.Discover(ctx, discoverURL, item.DeviceIDHex)
			if err != nil {
				// Still surface the UDP hit with limited metadata.
				add(DiscoveredTuner{
					Kind:        DiscoveredKindHDHomeRun,
					DeviceID:    item.DeviceIDHex,
					TunerCount:  item.TunerCount,
					DiscoverURL: discoverURL,
					BaseURL:     item.BaseURL,
					Source:      "udp",
				})
				continue
			}
			kind := hdhomerun.ClassifyKind(info, discoverURL)
			add(DiscoveredTuner{
				Kind:         kind,
				DeviceID:     coalesceTrimmed(info.DeviceID, item.DeviceIDHex),
				FriendlyName: info.FriendlyName,
				Model:        info.ModelNumber,
				Firmware:     info.FirmwareVersion,
				TunerCount:   info.TunerCount,
				DiscoverURL:  discoverURL,
				BaseURL:      coalesceTrimmed(info.BaseURL, item.BaseURL),
				Source:       "udp",
			})
		}
	}

	for _, raw := range req.ProbeURLs {
		urls := hdhomerun.ProbeCandidateURLs(raw)
		var lastErr error
		found := false
		for _, discoverURL := range urls {
			if err := ValidateMediaFetchURL(discoverURL); err != nil {
				lastErr = err
				continue
			}
			info, err := s.hdhr.Discover(ctx, discoverURL, "")
			if err != nil {
				lastErr = err
				continue
			}
			kind := hdhomerun.ClassifyKind(info, discoverURL)
			add(DiscoveredTuner{
				Kind:         kind,
				DeviceID:     info.DeviceID,
				FriendlyName: info.FriendlyName,
				Model:        info.ModelNumber,
				Firmware:     info.FirmwareVersion,
				TunerCount:   info.TunerCount,
				DiscoverURL:  discoverURL,
				BaseURL:      info.BaseURL,
				Source:       "probe",
			})
			found = true
			break
		}
		if !found && lastErr != nil {
			notes = append(notes, fmt.Sprintf("probe %q failed: %v", strings.TrimSpace(raw), lastErr))
		}
	}

	if candidates == nil {
		candidates = []DiscoveredTuner{}
	}
	return &DiscoverTunersResult{Candidates: candidates, Notes: notes}, nil
}

func coalesceTrimmed(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (s *Service) AddTuner(ctx context.Context, discoverURL, deviceID string) (*Tuner, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if s.hdhr == nil {
		return nil, ErrNotConfigured
	}
	discoverURL = strings.TrimSpace(discoverURL)
	deviceID = strings.TrimSpace(deviceID)
	switch {
	case discoverURL != "":
		if err := ValidateMediaFetchURL(discoverURL); err != nil {
			return nil, err
		}
	case strings.HasPrefix(deviceID, "http://"), strings.HasPrefix(deviceID, "https://"):
		if err := ValidateMediaFetchURL(deviceID); err != nil {
			return nil, err
		}
	case deviceID != "":
		if err := ValidateMediaFetchURL("http://" + deviceID + "/discover.json"); err != nil {
			return nil, err
		}
	}
	info, err := s.hdhr.Discover(ctx, discoverURL, deviceID)
	if err != nil {
		return nil, fmt.Errorf("discover hdhomerun: %w", err)
	}
	if strings.TrimSpace(info.DeviceID) == "" {
		return nil, fmt.Errorf("%w: device_id is required", ErrInvalidArgument)
	}
	if discoverURL == "" {
		discoverURL = hdhomerun.DiscoverURLForBase(info.BaseURL)
	}
	tuner, err := s.store.CreateTuner(ctx, &Tuner{
		Type:        TunerTypeHDHomeRun,
		DeviceID:    info.DeviceID,
		DiscoverURL: strings.TrimSpace(discoverURL),
		BaseURL:     info.BaseURL,
		Model:       info.ModelNumber,
		Firmware:    info.FirmwareVersion,
		TunerCount:  info.TunerCount,
		Status:      "discovered",
	})
	if err != nil {
		return nil, err
	}
	if err := s.ScanTuner(ctx, tuner.ID); err != nil {
		return tuner, err
	}
	return s.store.GetTuner(ctx, tuner.ID)
}

func (s *Service) ScanTuner(ctx context.Context, tunerID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	tuner, err := s.store.GetTuner(ctx, tunerID)
	if err != nil {
		return err
	}
	if tuner == nil {
		return ErrNotFound
	}
	if err := ValidateMediaFetchURL(tuner.BaseURL); err != nil {
		return err
	}
	lineup, err := s.hdhr.FetchLineup(ctx, tuner.BaseURL)
	if err != nil {
		_, _ = s.store.CreateTuner(ctx, &Tuner{
			ID:          tuner.ID,
			Type:        tuner.Type,
			DeviceID:    tuner.DeviceID,
			DiscoverURL: tuner.DiscoverURL,
			BaseURL:     tuner.BaseURL,
			Model:       tuner.Model,
			Firmware:    tuner.Firmware,
			TunerCount:  tuner.TunerCount,
			Status:      "error",
			LastError:   err.Error(),
		})
		return fmt.Errorf("scan tuner: %w", err)
	}
	channels := make([]Channel, 0, len(lineup))
	for _, ch := range lineup {
		channels = append(channels, Channel{
			TunerID:   tuner.ID,
			Number:    ch.GuideNumber,
			Callsign:  ch.GuideName,
			Name:      ch.GuideName,
			HD:        ch.HD,
			Enabled:   true,
			StreamURL: ch.URL,
		})
	}
	return s.store.ReplaceChannelsForTuner(ctx, tuner.ID, channels)
}

func (s *Service) DeleteTuner(ctx context.Context, id string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	return s.store.DeleteTuner(ctx, id)
}

func (s *Service) ListChannels(ctx context.Context, tunerID string) ([]Channel, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.ListChannels(ctx, tunerID)
}

func (s *Service) GetChannel(ctx context.Context, id string) (*Channel, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	channel, err := s.store.GetChannel(ctx, id)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrNotFound
	}
	return channel, nil
}

func (s *Service) PatchChannel(ctx context.Context, id string, patch ChannelPatch) (*Channel, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	channel, err := s.store.UpdateChannel(ctx, id, patch)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrNotFound
	}
	return channel, nil
}

func (s *Service) ListGuideSources(ctx context.Context) ([]GuideSource, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.ListGuideSources(ctx, false)
}

func (s *Service) CreateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := validateGuideSource(source); err != nil {
		return nil, err
	}
	if err := s.enforceEnabledGuideSourceLimit(ctx, source.ID, source.Enabled); err != nil {
		return nil, err
	}
	created, err := s.store.CreateGuideSource(ctx, source)
	if err != nil {
		return nil, err
	}
	return created, s.reorderGuideSourcePriorities(ctx)
}

func (s *Service) UpdateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	existing, err := s.store.GetGuideSource(ctx, source.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if source.Type == "" {
		source.Type = existing.Type
	}
	if source.DisplayName == "" {
		source.DisplayName = existing.DisplayName
	}
	if source.Config == nil {
		source.Config = existing.Config
	}
	if source.Status == "" {
		source.Status = existing.Status
	}
	source.LastError = existing.LastError
	source.LastSyncAt = existing.LastSyncAt
	if err := validateGuideSource(source); err != nil {
		return nil, err
	}
	if err := s.enforceEnabledGuideSourceLimit(ctx, source.ID, source.Enabled); err != nil {
		return nil, err
	}
	updated, err := s.store.UpdateGuideSource(ctx, source)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrNotFound
	}
	return updated, s.reorderGuideSourcePriorities(ctx)
}

func (s *Service) DeleteGuideSource(ctx context.Context, id string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.store.DeleteGuideSource(ctx, id); err != nil {
		return err
	}
	return s.reorderGuideSourcePriorities(ctx)
}

func (s *Service) SyncAllEnabledGuideSources(ctx context.Context) (int, error) {
	if err := s.requireStore(); err != nil {
		return 0, err
	}
	sources, err := s.store.ListGuideSources(ctx, true)
	if err != nil {
		return 0, err
	}
	var firstErr error
	count := 0
	for _, source := range sources {
		if err := s.SyncGuideSource(ctx, source.ID); err != nil && firstErr == nil {
			firstErr = err
		}
		count++
	}
	return count, firstErr
}

func (s *Service) SyncGuideSource(ctx context.Context, id string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	source, err := s.store.GetGuideSource(ctx, id)
	if err != nil {
		return err
	}
	if source == nil {
		return ErrNotFound
	}
	_ = s.store.SetGuideSourceSyncStatus(ctx, id, "syncing", "", nil, source.NextSyncAt)
	var syncErr error
	switch source.Type {
	case GuideSourceXMLTVURL:
		syncErr = s.syncXMLTV(ctx, source)
	case GuideSourceSchedulesDirect:
		syncErr = fmt.Errorf("%w: schedules direct guide sync is not implemented yet", ErrNotImplemented)
	default:
		syncErr = fmt.Errorf("%w: unsupported guide source type %q", ErrInvalidArgument, source.Type)
	}
	now := s.now()
	next := now.Add(6 * time.Hour)
	if syncErr != nil {
		_ = s.store.SetGuideSourceSyncStatus(ctx, id, "error", syncErr.Error(), nil, &next)
		return syncErr
	}
	return s.store.SetGuideSourceSyncStatus(ctx, id, "ready", "", &now, &next)
}

func (s *Service) syncXMLTV(ctx context.Context, source *GuideSource) error {
	url := strings.TrimSpace(source.Config["url"])
	if url == "" {
		return fmt.Errorf("%w: xmltv url is required", ErrInvalidArgument)
	}
	if err := ValidateMediaFetchURL(url); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch xmltv: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch xmltv: status %d", resp.StatusCode)
	}
	parsed, err := ParseXMLTV(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	channels, err := s.store.ListChannels(ctx, "")
	if err != nil {
		return err
	}
	xmlChannelNames := map[string]string{}
	for _, ch := range parsed.Channels {
		xmlChannelNames[strings.ToLower(ch.ID)] = strings.ToLower(ch.DisplayName)
	}
	channelByKey := map[string]string{}
	for _, ch := range channels {
		addKey := func(key string) {
			key = strings.ToLower(strings.TrimSpace(key))
			if key != "" {
				channelByKey[key] = ch.ID
			}
		}
		addKey(ch.GuideStationID)
		addKey(ch.Callsign)
		addKey(ch.Number)
		addKey(ch.ID)
		addKey(ch.Name)
	}
	programs := make([]Program, 0, len(parsed.Programs))
	for _, p := range parsed.Programs {
		channelID := channelByKey[strings.ToLower(strings.TrimSpace(p.ChannelID))]
		if channelID == "" {
			channelID = channelByKey[xmlChannelNames[strings.ToLower(strings.TrimSpace(p.ChannelID))]]
		}
		if channelID == "" {
			continue
		}
		programs = append(programs, Program{
			ChannelID:   channelID,
			SourceID:    source.ID,
			SeriesID:    stableSeriesID(p.Title),
			ExternalID:  p.ChannelID + ":" + p.Start.UTC().Format(time.RFC3339) + ":" + p.Title,
			Start:       p.Start,
			Stop:        p.Stop,
			Title:       p.Title,
			Subtitle:    p.Subtitle,
			Description: p.Description,
			Season:      p.Season,
			Episode:     p.Episode,
			Genres:      p.Genres,
			ImageURL:    p.ImageURL,
			IsNew:       p.IsNew,
			IsLive:      p.IsLive,
		})
	}
	if err := s.store.UpsertPrograms(ctx, source.ID, programs); err != nil {
		return err
	}
	return s.ApplySeriesRules(ctx)
}

func (s *Service) ListGuide(ctx context.Context, channelIDs []string, start, end time.Time) ([]Program, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.ListGuide(ctx, channelIDs, start, end)
}

func (s *Service) GetProgram(ctx context.Context, id string) (*Program, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	program, err := s.store.GetProgram(ctx, id)
	if err != nil {
		return nil, err
	}
	if program == nil {
		return nil, ErrNotFound
	}
	return program, nil
}

func (s *Service) StartChannelSession(ctx context.Context, channelID string, userID int, profileID string) (*LiveSession, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	channel, err := s.store.GetChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || !channel.Enabled {
		return nil, ErrNotFound
	}
	tuner, err := s.store.GetTuner(ctx, channel.TunerID)
	if err != nil {
		return nil, err
	}
	if tuner == nil {
		return nil, ErrNotFound
	}
	playbackID := ""
	streamURL := channel.StreamURL
	note := "raw HDHomeRun stream URL; playback bridge not configured"
	if s.playbackBridge != nil {
		var err error
		playbackID, streamURL, err = s.playbackBridge.StartLiveStream(ctx, channel.ID, channel.StreamURL, userID, profileID)
		if err != nil {
			return nil, fmt.Errorf("start playback bridge: %w", err)
		}
		note = ""
	}

	// Allocate a free tuner index, then insert. A concurrent StartChannelSession
	// can race past ActiveSessionTunerIndices; the partial unique index turns
	// that into ErrTunerIndexConflict — retry once, then surface ErrNoTuner.
	var session *LiveSession
	for attempt := 0; attempt < 2; attempt++ {
		indices, err := s.store.ActiveSessionTunerIndices(ctx, tuner.ID)
		if err != nil {
			return nil, err
		}
		index, ok := firstFreeIndex(tuner.TunerCount, indices)
		if !ok {
			return nil, ErrNoTuner
		}
		session, err = s.store.CreateSession(ctx, SessionCreate{
			ChannelID:         channel.ID,
			TunerID:           tuner.ID,
			TunerIndex:        index,
			UserID:            userID,
			ProfileID:         profileID,
			PlaybackSessionID: playbackID,
		})
		if err == nil {
			break
		}
		if errors.Is(err, ErrTunerIndexConflict) && attempt == 0 {
			continue
		}
		if errors.Is(err, ErrTunerIndexConflict) {
			return nil, ErrNoTuner
		}
		return nil, err
	}
	session.StreamURL = streamURL
	session.HLSURL = streamURL
	session.Note = note
	return session, nil
}

// ReleaseSession releases a live tuner session.
// When enforceOwner is true, the caller must own the session (matching user_id,
// and profile_id when the session recorded one). Pass enforceOwner=false for
// trusted internal teardown (e.g. jellycompat after opener-token checks).
func (s *Service) ReleaseSession(ctx context.Context, id string, userID int, profileID string, enforceOwner bool) (*LiveSession, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if enforceOwner {
		existing, err := s.store.GetSession(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		if !ownerMatches(existing.UserID, existing.ProfileID, userID, profileID) {
			return nil, ErrNotFound
		}
	}
	session, err := s.store.ReleaseSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrNotFound
	}
	return session, nil
}

func (s *Service) ListRecordings(ctx context.Context, status string, userID int, profileID string, enforceOwner bool) ([]Recording, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	all, err := s.store.ListRecordings(ctx, status)
	if err != nil || !enforceOwner {
		return all, err
	}
	out := make([]Recording, 0, len(all))
	for _, rec := range all {
		if ownerMatches(rec.UserID, rec.ProfileID, userID, profileID) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (s *Service) ScheduleRecording(ctx context.Context, rec *Recording) (*Recording, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if rec.ProgramID != "" {
		program, err := s.store.GetProgram(ctx, rec.ProgramID)
		if err != nil {
			return nil, err
		}
		if program == nil {
			return nil, ErrNotFound
		}
		rec.ChannelID = program.ChannelID
		rec.Start = program.Start
		rec.Stop = program.Stop
		rec.Title = program.Title
	}
	if rec.ChannelID == "" || rec.Start.IsZero() || rec.Stop.IsZero() {
		return nil, fmt.Errorf("%w: channel_id, start, and stop are required", ErrInvalidArgument)
	}
	rec.ID = ""
	rec.Path = ""
	rec.LibraryItemID = ""
	rec.LastError = ""
	rec.Status = "scheduled"
	return s.store.CreateRecording(ctx, rec)
}

func (s *Service) CancelRecording(ctx context.Context, id string, userID int, profileID string, enforceOwner bool) (*Recording, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if enforceOwner {
		existing, err := s.store.GetRecording(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing == nil || !ownerMatches(existing.UserID, existing.ProfileID, userID, profileID) {
			return nil, ErrNotFound
		}
	}
	rec, err := s.store.CancelRecording(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, ErrNotFound
	}
	return rec, nil
}

func (s *Service) ListSeriesRules(ctx context.Context, userID int, profileID string, enforceOwner bool) ([]SeriesRule, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	all, err := s.store.ListSeriesRules(ctx)
	if err != nil || !enforceOwner {
		return all, err
	}
	out := make([]SeriesRule, 0, len(all))
	for _, rule := range all {
		if ownerMatches(rule.UserID, rule.ProfileID, userID, profileID) {
			out = append(out, rule)
		}
	}
	return out, nil
}

func (s *Service) CreateSeriesRule(ctx context.Context, rule *SeriesRule) (*SeriesRule, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if rule.SeriesID == "" && rule.ChannelID == nil && strings.TrimSpace(rule.TitleMatch) == "" {
		return nil, fmt.Errorf("%w: series_id, channel_id, or title_match is required", ErrInvalidArgument)
	}
	rule.ID = ""
	return s.store.CreateSeriesRule(ctx, rule)
}

func (s *Service) DeleteSeriesRule(ctx context.Context, id string, userID int, profileID string, enforceOwner bool) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if enforceOwner {
		existing, err := s.store.GetSeriesRule(ctx, id)
		if err != nil {
			return err
		}
		if existing == nil || !ownerMatches(existing.UserID, existing.ProfileID, userID, profileID) {
			return ErrNotFound
		}
	}
	return s.store.DeleteSeriesRule(ctx, id)
}

func (s *Service) GetSession(ctx context.Context, id string) (*LiveSession, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.GetSession(ctx, id)
}

// GetSessionForViewer returns a session after applying the centralized ownership
// check used by ReleaseSession / CancelRecording. Missing or unauthorized
// sessions surface as ErrNotFound.
func (s *Service) GetSessionForViewer(
	ctx context.Context,
	sessionID string,
	userID int,
	profileID string,
	enforceOwner bool,
) (*LiveSession, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrNotFound
	}
	if enforceOwner && !ownerMatches(session.UserID, session.ProfileID, userID, profileID) {
		return nil, ErrNotFound
	}
	return session, nil
}

// ResolveSessionUpstreamURL returns the upstream tuner URL for an active session.
func (s *Service) ResolveSessionUpstreamURL(ctx context.Context, sessionID string) (string, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if session == nil || session.Status != "active" {
		return "", ErrNotFound
	}
	ch, err := s.GetChannel(ctx, session.ChannelID)
	if err != nil {
		return "", err
	}
	if ch == nil || strings.TrimSpace(ch.StreamURL) == "" {
		return "", ErrNotFound
	}
	if err := ValidateMediaFetchURL(ch.StreamURL); err != nil {
		return "", err
	}
	return ch.StreamURL, nil
}

func ownerMatches(ownerUser int, ownerProfile string, userID int, profileID string) bool {
	// Unscoped legacy rows are not visible to non-admin callers (enforceOwner path).
	if ownerUser == 0 && ownerProfile == "" {
		return false
	}
	if ownerUser != 0 && ownerUser != userID {
		return false
	}
	if ownerProfile != "" && profileID != "" && ownerProfile != profileID {
		return false
	}
	return true
}

func (s *Service) ApplySeriesRules(ctx context.Context) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	rules, err := s.store.ListSeriesRules(ctx)
	if err != nil {
		return err
	}
	programs, err := s.store.ListUpcomingPrograms(ctx, s.now().Add(14*24*time.Hour))
	if err != nil {
		return err
	}
	existingPairs, err := s.store.ListActiveRecordingPairs(ctx)
	if err != nil {
		return err
	}
	byChannel := map[string][]Program{}
	for _, p := range programs {
		byChannel[p.ChannelID] = append(byChannel[p.ChannelID], p)
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		candidates := programs
		if rule.ChannelID != nil {
			candidates = byChannel[*rule.ChannelID]
		}
		for _, p := range candidates {
			if !matchesRule(rule, p) {
				continue
			}
			key := recordingPairKey(p.ID, rule.ID)
			if _, exists := existingPairs[key]; exists {
				continue
			}
			_, err = s.store.CreateRecording(ctx, &Recording{
				ProgramID:    p.ID,
				ChannelID:    p.ChannelID,
				SeriesRuleID: rule.ID,
				UserID:       rule.UserID,
				ProfileID:    rule.ProfileID,
				Status:       "scheduled",
				Start:        p.Start,
				Stop:         p.Stop,
				Title:        p.Title,
			})
			if err != nil {
				return err
			}
			existingPairs[key] = struct{}{}
		}
	}
	return nil
}

func recordingPairKey(programID, seriesRuleID string) string {
	return programID + "\x00" + seriesRuleID
}

func (s *Service) FailDueRecordings(ctx context.Context) (int, error) {
	if err := s.requireStore(); err != nil {
		return 0, err
	}
	// Placeholder until the actual FFmpeg/DVR recorder lands. This makes due
	// rows visible as failed instead of silently accumulating as scheduled.
	return s.store.FailDueRecordings(ctx, s.now(), "Live TV recorder is not implemented yet")
}

func validateGuideSource(source *GuideSource) error {
	switch source.Type {
	case GuideSourceXMLTVURL, GuideSourceSchedulesDirect:
	default:
		return fmt.Errorf("%w: unsupported guide source type %q", ErrInvalidArgument, source.Type)
	}
	if source.Config == nil {
		source.Config = map[string]string{}
	}
	return nil
}

func (s *Service) enforceEnabledGuideSourceLimit(ctx context.Context, updatingID string, enabled bool) error {
	if !enabled {
		return nil
	}
	sources, err := s.store.ListGuideSources(ctx, true)
	if err != nil {
		return err
	}
	count := 0
	for _, source := range sources {
		if source.ID != updatingID {
			count++
		}
	}
	if count >= MaxGuideSources {
		return fmt.Errorf("%w: at most %d enabled guide sources are allowed", ErrLimitExceeded, MaxGuideSources)
	}
	return nil
}

func (s *Service) reorderGuideSourcePriorities(ctx context.Context) error {
	sources, err := s.store.ListGuideSources(ctx, false)
	if err != nil {
		return err
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Priority == sources[j].Priority {
			return sources[i].ID < sources[j].ID
		}
		return sources[i].Priority < sources[j].Priority
	})
	for i := range sources {
		want := (i + 1) * 100
		if sources[i].Priority == want {
			continue
		}
		sources[i].Priority = want
		if _, err := s.store.UpdateGuideSource(ctx, &sources[i]); err != nil {
			return err
		}
	}
	return nil
}

func firstFreeIndex(tunerCount int, active []int) (int, bool) {
	if tunerCount <= 0 {
		tunerCount = 1
	}
	used := map[int]bool{}
	for _, idx := range active {
		used[idx] = true
	}
	for i := 0; i < tunerCount; i++ {
		if !used[i] {
			return i, true
		}
	}
	return 0, false
}

func stableSeriesID(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), "-"))
}

func matchesRule(rule SeriesRule, p Program) bool {
	if rule.NewOnly && !p.IsNew {
		return false
	}
	if rule.ChannelID != nil && *rule.ChannelID != p.ChannelID {
		return false
	}
	if rule.SeriesID != "" && rule.SeriesID != p.SeriesID {
		return false
	}
	if rule.TitleMatch != "" && !strings.Contains(strings.ToLower(p.Title), strings.ToLower(rule.TitleMatch)) {
		return false
	}
	return true
}
