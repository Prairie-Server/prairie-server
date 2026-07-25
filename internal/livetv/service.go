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
	ErrNotFound        = errors.New("livetv: not found")
	ErrInvalidArgument = errors.New("livetv: invalid argument")
	ErrLimitExceeded   = errors.New("livetv: limit exceeded")
	ErrNoTuner         = errors.New("livetv: no tuner available")
	ErrNotImplemented  = errors.New("livetv: not implemented")
	ErrNotConfigured   = errors.New("livetv service not configured")
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
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return &Service{
		store:      store,
		hdhr:       hdhomerun.NewClient(httpClient),
		httpClient: httpClient,
		now:        time.Now,
	}
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

func (s *Service) AddTuner(ctx context.Context, discoverURL, deviceID string) (*Tuner, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if s.hdhr == nil {
		return nil, ErrNotConfigured
	}
	info, err := s.hdhr.Discover(ctx, discoverURL, deviceID)
	if err != nil {
		return nil, fmt.Errorf("discover hdhomerun: %w", err)
	}
	if strings.TrimSpace(info.DeviceID) == "" {
		return nil, fmt.Errorf("%w: device_id is required", ErrInvalidArgument)
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
	programs := make([]Program, 0, len(parsed.Programmes))
	for _, p := range parsed.Programmes {
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
	indices, err := s.store.ActiveSessionTunerIndices(ctx, tuner.ID)
	if err != nil {
		return nil, err
	}
	index, ok := firstFreeIndex(tuner.TunerCount, indices)
	if !ok {
		return nil, ErrNoTuner
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
	session, err := s.store.CreateSession(ctx, SessionCreate{
		ChannelID:         channel.ID,
		TunerID:           tuner.ID,
		TunerIndex:        index,
		UserID:            userID,
		ProfileID:         profileID,
		PlaybackSessionID: playbackID,
	})
	if err != nil {
		return nil, err
	}
	session.StreamURL = streamURL
	session.HLSURL = streamURL
	session.Note = note
	return session, nil
}

func (s *Service) ReleaseSession(ctx context.Context, id string) (*LiveSession, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
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

func (s *Service) ListRecordings(ctx context.Context, status string) ([]Recording, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.ListRecordings(ctx, status)
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

func (s *Service) CancelRecording(ctx context.Context, id string) (*Recording, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
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

func (s *Service) ListSeriesRules(ctx context.Context) ([]SeriesRule, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	return s.store.ListSeriesRules(ctx)
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

func (s *Service) DeleteSeriesRule(ctx context.Context, id string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	return s.store.DeleteSeriesRule(ctx, id)
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
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, p := range programs {
			if !matchesRule(rule, p) {
				continue
			}
			exists, err := s.store.RecordingExists(ctx, p.ID, rule.ID)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			_, err = s.store.CreateRecording(ctx, &Recording{
				ProgramID:    p.ID,
				ChannelID:    p.ChannelID,
				SeriesRuleID: rule.ID,
				Status:       "scheduled",
				Start:        p.Start,
				Stop:         p.Stop,
				Title:        p.Title,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
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
