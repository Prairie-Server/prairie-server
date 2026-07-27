package livetv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/livetv/gracenote"
	"github.com/prairie-server/prairie-server/internal/livetv/hdhomerun"
	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
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

type SchedulesDirectClient interface {
	Token(ctx context.Context, username, passwordSHA1 string) (string, error)
	Headends(ctx context.Context, token, country, postalCode string) ([]schedulesdirect.Headend, error)
	AddLineup(ctx context.Context, token, lineupID string) error
	Lineup(ctx context.Context, token, lineupID string) (*schedulesdirect.LineupDetail, error)
	Schedules(ctx context.Context, token string, reqs []schedulesdirect.ScheduleRequest) ([]schedulesdirect.StationSchedule, error)
	Programs(ctx context.Context, token string, ids []string) ([]schedulesdirect.ProgramDetail, error)
}

// GracenoteClient fetches Zap2XML-style listings from tvlistings.gracenote.com.
type GracenoteClient interface {
	Providers(ctx context.Context, country, postalCode, lang string) ([]gracenote.Provider, error)
	Grid(ctx context.Context, params gracenote.GridParams, start time.Time, timespanHours int) (*gracenote.GridResponse, error)
	AssetURL(thumbnail string) string
}

type PlaybackBridge interface {
	StartLiveStream(ctx context.Context, channelID, sourceStreamURL string, userID int, profileID string) (playbackSessionID, playbackURL string, err error)
}

type Service struct {
	store          Store
	hdhr           HDHomeRunClient
	sd             SchedulesDirectClient
	gn             GracenoteClient
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
	sdHTTP := &http.Client{Timeout: 2 * time.Minute}
	gnHTTP := &http.Client{Timeout: 2 * time.Minute}
	return &Service{
		store:      store,
		hdhr:       hdhomerun.NewClient(httpClient),
		sd:         schedulesdirect.NewClient(sdHTTP),
		gn:         gracenote.NewClient(gnHTTP),
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

func (s *Service) SetSchedulesDirectClient(client SchedulesDirectClient) {
	s.sd = client
}

func (s *Service) SetGracenoteClient(client GracenoteClient) {
	s.gn = client
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

func (s *Service) AddTuner(ctx context.Context, in AddTunerInput) (*Tuner, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if s.hdhr == nil {
		return nil, ErrNotConfigured
	}
	raw := coalesceTrimmed(in.URL, in.DiscoverURL, in.DeviceID)
	if raw == "" {
		return nil, fmt.Errorf("%w: url is required", ErrInvalidArgument)
	}

	endpoints := hdhomerun.ProbeCandidateURLs(raw)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("%w: invalid url", ErrInvalidArgument)
	}

	var (
		info        *hdhomerun.DeviceInfo
		discoverURL string
		lastErr     error
	)
	for _, endpoint := range endpoints {
		if err := ValidateMediaFetchURL(endpoint); err != nil {
			lastErr = err
			continue
		}
		discovered, err := s.hdhr.Discover(ctx, endpoint, "")
		if err != nil {
			lastErr = err
			continue
		}
		info = discovered
		discoverURL = endpoint
		break
	}
	if info == nil {
		return nil, fmt.Errorf("discover hdhomerun: %w", lastErr)
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
	sources, err := s.store.ListGuideSources(ctx, false)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		sources[i].Config = RedactGuideSourceConfig(sources[i].Config)
	}
	return sources, nil
}

func (s *Service) CreateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.prepareGuideSourceConfig(ctx, source, nil); err != nil {
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
	if err := s.reorderGuideSourcePriorities(ctx); err != nil {
		return nil, err
	}
	created.Config = RedactGuideSourceConfig(created.Config)
	return created, nil
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
	if err := s.prepareGuideSourceConfig(ctx, source, existing); err != nil {
		return nil, err
	}
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
	if err := s.reorderGuideSourcePriorities(ctx); err != nil {
		return nil, err
	}
	updated.Config = RedactGuideSourceConfig(updated.Config)
	return updated, nil
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
	case GuideSourceSchedulesDirect:
		syncErr = s.syncSchedulesDirect(ctx, source)
	case GuideSourceXMLSync:
		syncErr = s.syncXMLSync(ctx, source)
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

// ListSchedulesDirectLineups authenticates and returns lineups for a postal code.
func (s *Service) ListSchedulesDirectLineups(ctx context.Context, req SchedulesDirectLineupsRequest) ([]schedulesdirect.LineupOption, error) {
	if s == nil || s.sd == nil {
		return nil, ErrNotConfigured
	}
	username := strings.TrimSpace(req.Username)
	passwordSHA1 := strings.TrimSpace(req.PasswordSHA1)
	if password := strings.TrimSpace(req.Password); password != "" {
		passwordSHA1 = schedulesdirect.HashPassword(password)
	}
	postal := strings.TrimSpace(req.PostalCode)
	if username == "" || passwordSHA1 == "" || postal == "" {
		return nil, fmt.Errorf("%w: username, password, and postalcode are required", ErrInvalidArgument)
	}
	country := strings.TrimSpace(req.Country)
	if country == "" {
		country = DefaultSchedulesDirectCountry
	}
	token, err := s.sd.Token(ctx, username, passwordSHA1)
	if err != nil {
		return nil, err
	}
	headends, err := s.sd.Headends(ctx, token, country, postal)
	if err != nil {
		return nil, err
	}
	return schedulesdirect.FlattenLineups(headends), nil
}

// ListXMLSyncLineups returns Gracenote providers for a postal code (no account).
func (s *Service) ListXMLSyncLineups(ctx context.Context, req XMLSyncLineupsRequest) ([]gracenote.LineupOption, error) {
	if s == nil || s.gn == nil {
		return nil, ErrNotConfigured
	}
	postal := strings.TrimSpace(req.PostalCode)
	if postal == "" {
		return nil, fmt.Errorf("%w: postalcode is required", ErrInvalidArgument)
	}
	country := strings.TrimSpace(req.Country)
	if country == "" {
		country = DefaultXMLSyncCountry
	}
	providers, err := s.gn.Providers(ctx, country, postal, strings.TrimSpace(req.Lang))
	if err != nil {
		return nil, err
	}
	return gracenote.FlattenProviders(providers), nil
}

func (s *Service) syncSchedulesDirect(ctx context.Context, source *GuideSource) error {
	if s.sd == nil {
		return ErrNotConfigured
	}
	username := strings.TrimSpace(source.Config["username"])
	passwordSHA1 := strings.TrimSpace(source.Config["password_sha1"])
	lineupID := strings.TrimSpace(source.Config["lineup"])
	if username == "" || passwordSHA1 == "" || lineupID == "" {
		return fmt.Errorf("%w: schedules direct username, password_sha1, and lineup are required", ErrInvalidArgument)
	}
	days := DefaultSchedulesDirectDays
	if raw := strings.TrimSpace(source.Config["days"]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 21 {
			days = n
		}
	}

	token, err := s.sd.Token(ctx, username, passwordSHA1)
	if err != nil {
		return err
	}
	if err := s.sd.AddLineup(ctx, token, lineupID); err != nil {
		return err
	}
	detail, err := s.sd.Lineup(ctx, token, lineupID)
	if err != nil {
		return err
	}

	channels, err := s.store.ListChannels(ctx, "")
	if err != nil {
		return err
	}
	stationByChannelID, err := s.mapChannelsToStations(ctx, channels, detail)
	if err != nil {
		return err
	}
	if len(stationByChannelID) == 0 {
		return fmt.Errorf("%w: no channels mapped to Schedules Direct stations; set guide station IDs or match channel numbers/callsigns", ErrInvalidArgument)
	}

	stationIDs := uniqueStrings(valuesOf(stationByChannelID))
	dates := scheduleDates(s.now(), days)
	reqs := make([]schedulesdirect.ScheduleRequest, 0, len(stationIDs))
	for _, stationID := range stationIDs {
		reqs = append(reqs, schedulesdirect.ScheduleRequest{StationID: stationID, Date: dates})
	}
	schedules, err := s.sd.Schedules(ctx, token, reqs)
	if err != nil {
		return err
	}

	programIDs := make([]string, 0)
	seenProgram := map[string]bool{}
	for _, sched := range schedules {
		if sched.Code != 0 {
			continue
		}
		for _, p := range sched.Programs {
			id := strings.TrimSpace(p.ProgramID)
			if id == "" || seenProgram[id] {
				continue
			}
			seenProgram[id] = true
			programIDs = append(programIDs, id)
		}
	}
	programDetails, err := s.sd.Programs(ctx, token, programIDs)
	if err != nil {
		return err
	}
	byProgramID := map[string]schedulesdirect.ProgramDetail{}
	for _, p := range programDetails {
		byProgramID[p.ProgramID] = p
	}

	channelByStation := map[string]string{}
	for channelID, stationID := range stationByChannelID {
		channelByStation[stationID] = channelID
	}

	programs := make([]Program, 0)
	for _, sched := range schedules {
		if sched.Code != 0 {
			continue
		}
		channelID := channelByStation[sched.StationID]
		if channelID == "" {
			continue
		}
		for _, airing := range sched.Programs {
			start, err := time.Parse(time.RFC3339, airing.AirDateTime)
			if err != nil {
				continue
			}
			if airing.Duration <= 0 {
				continue
			}
			stop := start.Add(time.Duration(airing.Duration) * time.Second)
			detail := byProgramID[airing.ProgramID]
			title := detail.Title()
			if title == "" {
				title = airing.ProgramID
			}
			season, episode := detail.SeasonEpisode()
			isNew := airing.New != nil && *airing.New
			isLive := airing.Live != nil && *airing.Live
			programs = append(programs, Program{
				ChannelID:   channelID,
				SourceID:    source.ID,
				SeriesID:    schedulesDirectSeriesID(airing.ProgramID, title),
				ExternalID:  airing.ProgramID + ":" + start.UTC().Format(time.RFC3339),
				Start:       start.UTC(),
				Stop:        stop.UTC(),
				Title:       title,
				Subtitle:    strings.TrimSpace(detail.EpisodeTitle150),
				Description: detail.Description(),
				Season:      season,
				Episode:     episode,
				Genres:      nonNilStringSlice(detail.Genres),
				IsNew:       isNew,
				IsLive:      isLive,
			})
		}
	}
	if err := s.store.UpsertPrograms(ctx, source.ID, programs); err != nil {
		return err
	}
	return s.ApplySeriesRules(ctx)
}

func (s *Service) syncXMLSync(ctx context.Context, source *GuideSource) error {
	if s.gn == nil {
		return ErrNotConfigured
	}
	postal := strings.TrimSpace(source.Config["postalcode"])
	lineupID := strings.TrimSpace(source.Config["lineup"])
	if postal == "" || lineupID == "" {
		return fmt.Errorf("%w: xml sync postalcode and lineup are required", ErrInvalidArgument)
	}
	country := strings.TrimSpace(source.Config["country"])
	if country == "" {
		country = DefaultXMLSyncCountry
	}
	headend := strings.TrimSpace(source.Config["headend"])
	device := strings.TrimSpace(source.Config["device"])
	days := DefaultXMLSyncDays
	if raw := strings.TrimSpace(source.Config["days"]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= MaxXMLSyncDays {
			days = n
		}
	}

	channels, err := s.store.ListChannels(ctx, "")
	if err != nil {
		return err
	}

	params := gracenote.GridParams{
		Country:    country,
		PostalCode: postal,
		LineupID:   lineupID,
		HeadendID:  headend,
		Device:     device,
	}

	// Pull the first window to discover stations for channel mapping, then the rest.
	start := s.now().UTC().Truncate(time.Hour)
	first, err := s.gn.Grid(ctx, params, start, gracenote.DefaultGridHours)
	if err != nil {
		return err
	}
	stations := first.Channels
	stationByChannelID, err := s.mapChannelsToXMLSyncStations(ctx, channels, stations)
	if err != nil {
		return err
	}
	if len(stationByChannelID) == 0 {
		return fmt.Errorf("%w: no channels mapped to XML sync stations; set guide station IDs or match channel numbers/callsigns", ErrInvalidArgument)
	}

	channelByStation := map[string]string{}
	for channelID, stationID := range stationByChannelID {
		channelByStation[stationID] = channelID
	}

	programs := make([]Program, 0)
	appendGrid := func(grid *gracenote.GridResponse) {
		if grid == nil {
			return
		}
		for _, st := range grid.Channels {
			channelID := channelByStation[strings.TrimSpace(st.ChannelID)]
			if channelID == "" {
				continue
			}
			for _, ev := range st.Events {
				prog := programFromGracenoteEvent(source.ID, channelID, ev, s.gn.AssetURL)
				if prog == nil {
					continue
				}
				programs = append(programs, *prog)
			}
		}
	}
	appendGrid(first)

	windows := days * (24 / gracenote.DefaultGridHours)
	for i := 1; i < windows; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		windowStart := start.Add(time.Duration(i*gracenote.DefaultGridHours) * time.Hour)
		grid, err := s.gn.Grid(ctx, params, windowStart, gracenote.DefaultGridHours)
		if err != nil {
			return err
		}
		appendGrid(grid)
	}

	if err := s.store.UpsertPrograms(ctx, source.ID, programs); err != nil {
		return err
	}
	return s.ApplySeriesRules(ctx)
}

func programFromGracenoteEvent(sourceID, channelID string, ev gracenote.Event, assetURL func(string) string) *Program {
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(ev.StartTime))
	if err != nil {
		return nil
	}
	stop, err := time.Parse(time.RFC3339, strings.TrimSpace(ev.EndTime))
	if err != nil {
		if mins, convErr := strconv.Atoi(strings.TrimSpace(ev.Duration)); convErr == nil && mins > 0 {
			stop = start.Add(time.Duration(mins) * time.Minute)
		} else {
			return nil
		}
	}
	if !stop.After(start) {
		return nil
	}
	title := strings.TrimSpace(ev.Program.Title)
	if title == "" {
		title = strings.TrimSpace(ev.Program.ID)
	}
	if title == "" {
		return nil
	}
	externalID := strings.TrimSpace(ev.Program.ID)
	if externalID == "" {
		externalID = title
	}
	externalID += ":" + start.UTC().Format(time.RFC3339)

	seriesID := strings.TrimSpace(ev.Program.SeriesID)
	if seriesID == "" {
		seriesID = schedulesDirectSeriesID(ev.Program.ID, title)
	} else {
		seriesID = strings.ToLower(seriesID)
	}

	genres := make([]string, 0, len(ev.Filter))
	for _, g := range ev.Filter {
		g = strings.TrimSpace(strings.TrimPrefix(g, "filter-"))
		if g == "" {
			continue
		}
		genres = append(genres, g)
	}
	isNew := false
	isLive := false
	for _, flag := range ev.Flag {
		switch strings.ToLower(strings.TrimSpace(flag)) {
		case "new":
			isNew = true
		case "live":
			isLive = true
		}
	}

	imageURL := ""
	if assetURL != nil {
		imageURL = assetURL(ev.Thumbnail)
	}

	return &Program{
		ChannelID:   channelID,
		SourceID:    sourceID,
		SeriesID:    seriesID,
		ExternalID:  externalID,
		Start:       start.UTC(),
		Stop:        stop.UTC(),
		Title:       title,
		Subtitle:    strings.TrimSpace(ev.Program.EpisodeTitle),
		Description: strings.TrimSpace(ev.Program.ShortDesc),
		Season:      ev.Program.Season,
		Episode:     ev.Program.Episode,
		Genres:      genres,
		ImageURL:    imageURL,
		IsNew:       isNew,
		IsLive:      isLive,
	}
}

func (s *Service) mapChannelsToXMLSyncStations(ctx context.Context, channels []Channel, stations []gracenote.Channel) (map[string]string, error) {
	out := map[string]string{}
	for _, ch := range channels {
		stationID := resolveXMLSyncStationID(ch, stations)
		if stationID == "" {
			continue
		}
		out[ch.ID] = stationID
		if strings.TrimSpace(ch.GuideStationID) == "" {
			id := stationID
			if _, err := s.store.UpdateChannel(ctx, ch.ID, ChannelPatch{GuideStationID: &id}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s *Service) mapChannelsToStations(ctx context.Context, channels []Channel, detail *schedulesdirect.LineupDetail) (map[string]string, error) {
	out := map[string]string{}
	for _, ch := range channels {
		stationID := resolveGuideStationID(ch, detail)
		if stationID == "" {
			continue
		}
		out[ch.ID] = stationID
		if strings.TrimSpace(ch.GuideStationID) == "" {
			id := stationID
			if _, err := s.store.UpdateChannel(ctx, ch.ID, ChannelPatch{GuideStationID: &id}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s *Service) prepareGuideSourceConfig(ctx context.Context, source *GuideSource, existing *GuideSource) error {
	if source.Type == "" {
		source.Type = GuideSourceSchedulesDirect
	}
	if source.Config == nil {
		source.Config = map[string]string{}
	}
	cfg := map[string]string{}
	for k, v := range source.Config {
		cfg[k] = v
	}
	if existing != nil {
		for k, v := range existing.Config {
			if _, ok := cfg[k]; !ok || strings.TrimSpace(cfg[k]) == "" {
				cfg[k] = v
			}
		}
	}

	switch source.Type {
	case GuideSourceSchedulesDirect:
		return s.prepareSchedulesDirectConfig(ctx, source, existing, cfg)
	case GuideSourceXMLSync:
		return prepareXMLSyncConfig(source, cfg)
	default:
		return fmt.Errorf("%w: unsupported guide source type %q", ErrInvalidArgument, source.Type)
	}
}

func (s *Service) prepareSchedulesDirectConfig(ctx context.Context, source *GuideSource, existing *GuideSource, cfg map[string]string) error {
	username := strings.TrimSpace(cfg["username"])
	passwordSHA1 := strings.ToLower(strings.TrimSpace(cfg["password_sha1"]))
	if password := strings.TrimSpace(cfg["password"]); password != "" {
		passwordSHA1 = schedulesdirect.HashPassword(password)
	}
	country := strings.TrimSpace(cfg["country"])
	if country == "" {
		country = DefaultSchedulesDirectCountry
	}
	postal := strings.TrimSpace(cfg["postalcode"])
	lineup := strings.TrimSpace(cfg["lineup"])
	if username == "" || passwordSHA1 == "" || lineup == "" {
		return fmt.Errorf("%w: schedules direct username, password, and lineup are required", ErrInvalidArgument)
	}
	if len(passwordSHA1) != 40 {
		return fmt.Errorf("%w: schedules direct password_sha1 must be a 40-character sha1 hex digest", ErrInvalidArgument)
	}

	if s.sd != nil {
		credsChanged := existing == nil ||
			strings.TrimSpace(existing.Config["username"]) != username ||
			strings.TrimSpace(existing.Config["password_sha1"]) != passwordSHA1 ||
			strings.TrimSpace(existing.Config["lineup"]) != lineup
		if credsChanged {
			token, err := s.sd.Token(ctx, username, passwordSHA1)
			if err != nil {
				return err
			}
			if err := s.sd.AddLineup(ctx, token, lineup); err != nil {
				return err
			}
		}
	}

	source.Config = map[string]string{
		"username":      username,
		"password_sha1": passwordSHA1,
		"country":       country,
		"postalcode":    postal,
		"lineup":        lineup,
	}
	if days := strings.TrimSpace(cfg["days"]); days != "" {
		source.Config["days"] = days
	}
	if source.DisplayName == "" {
		source.DisplayName = "Schedules Direct"
	}
	return nil
}

func prepareXMLSyncConfig(source *GuideSource, cfg map[string]string) error {
	country := strings.TrimSpace(cfg["country"])
	if country == "" {
		country = DefaultXMLSyncCountry
	}
	postal := strings.TrimSpace(cfg["postalcode"])
	lineup := strings.TrimSpace(cfg["lineup"])
	if postal == "" || lineup == "" {
		return fmt.Errorf("%w: xml sync postalcode and lineup are required", ErrInvalidArgument)
	}
	headend := strings.TrimSpace(cfg["headend"])
	if headend == "" {
		headend = gracenoteHeadendFromLineup(lineup)
	}
	device := strings.TrimSpace(cfg["device"])
	if device == "" {
		device = "-"
	}

	source.Config = map[string]string{
		"country":    country,
		"postalcode": postal,
		"lineup":     lineup,
		"headend":    headend,
		"device":     device,
	}
	if days := strings.TrimSpace(cfg["days"]); days != "" {
		source.Config["days"] = days
	}
	if source.DisplayName == "" {
		source.DisplayName = "XML sync"
	}
	return nil
}

func gracenoteHeadendFromLineup(lineupID string) string {
	parts := strings.Split(strings.TrimSpace(lineupID), "-")
	if len(parts) >= 3 && strings.EqualFold(parts[len(parts)-1], "DEFAULT") {
		return strings.Join(parts[1:len(parts)-1], "-")
	}
	if len(parts) >= 2 {
		return parts[1]
	}
	return lineupID
}

// RedactGuideSourceConfig removes secrets from guide source config for API responses.
func RedactGuideSourceConfig(cfg map[string]string) map[string]string {
	if cfg == nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range cfg {
		switch k {
		case "password", "password_sha1":
			continue
		default:
			out[k] = v
		}
	}
	if strings.TrimSpace(cfg["password_sha1"]) != "" || strings.TrimSpace(cfg["password"]) != "" {
		out["password_configured"] = "true"
	}
	return out
}

func channelDisplayNumber(ch Channel) string {
	if ch.NumberOverride != nil && strings.TrimSpace(*ch.NumberOverride) != "" {
		return *ch.NumberOverride
	}
	return ch.Number
}

func normalizeChannelNumber(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		return "0"
	}
	return raw
}

func scheduleDates(now time.Time, days int) []string {
	if days <= 0 {
		days = DefaultSchedulesDirectDays
	}
	out := make([]string, 0, days)
	day := now.UTC()
	for i := 0; i < days; i++ {
		out = append(out, day.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return out
}

func schedulesDirectSeriesID(programID, title string) string {
	programID = strings.TrimSpace(programID)
	if len(programID) >= 10 && (strings.HasPrefix(programID, "EP") || strings.HasPrefix(programID, "SH") || strings.HasPrefix(programID, "MV")) {
		return strings.ToLower(programID[:10])
	}
	return stableSeriesID(title)
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func valuesOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
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
	case GuideSourceSchedulesDirect, GuideSourceXMLSync:
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
