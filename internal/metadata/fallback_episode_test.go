package metadata

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/models"
)

// ---------------------------------------------------------------------------
// Fake repositories for episode/season testing
// ---------------------------------------------------------------------------

// fakeEpisodeRepo implements metadataEpisodeRepo with an in-memory store keyed
// by (series_id, season_number, episode_number).
type fakeEpisodeRepo struct {
	mu       sync.Mutex
	episodes map[string]*models.Episode // key: "seriesID:season:episode"
	upserts  int
}

func newFakeEpisodeRepo() *fakeEpisodeRepo {
	return &fakeEpisodeRepo{episodes: make(map[string]*models.Episode)}
}

func episodeKey(seriesID string, season, episode int) string {
	return fmt.Sprintf("%s:%d:%d", seriesID, season, episode)
}

func (r *fakeEpisodeRepo) GetBySeriesAndNumber(_ context.Context, seriesID string, season, episode int) (*models.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := episodeKey(seriesID, season, episode)
	if ep, ok := r.episodes[key]; ok {
		cp := *ep
		return &cp, nil
	}
	return nil, catalog.ErrEpisodeNotFound
}

func (r *fakeEpisodeRepo) GetByID(_ context.Context, contentID string) (*models.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ep := range r.episodes {
		if ep.ContentID == contentID {
			cp := *ep
			return &cp, nil
		}
	}
	return nil, catalog.ErrEpisodeNotFound
}

func (r *fakeEpisodeRepo) ListBySeriesAndAirDates(_ context.Context, seriesID string, airDates []string) (map[string][]*models.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dateSet := make(map[string]struct{}, len(airDates))
	for _, airDate := range airDates {
		dateSet[airDate] = struct{}{}
	}
	result := make(map[string][]*models.Episode, len(dateSet))
	for _, ep := range r.episodes {
		if ep.SeriesID != seriesID || ep.AirDate == nil {
			continue
		}
		key := ep.AirDate.Format("2006-01-02")
		if _, ok := dateSet[key]; !ok {
			continue
		}
		cp := *ep
		result[key] = append(result[key], &cp)
	}
	for key := range result {
		sort.Slice(result[key], func(i, j int) bool {
			left := result[key][i]
			right := result[key][j]
			if left.SeasonNumber != right.SeasonNumber {
				return left.SeasonNumber < right.SeasonNumber
			}
			return left.EpisodeNumber < right.EpisodeNumber
		})
	}
	return result, nil
}

func (r *fakeEpisodeRepo) ListBySeries(_ context.Context, seriesID string) ([]*models.Episode, error) {
	return r.listBySeries(seriesID), nil
}

func (r *fakeEpisodeRepo) ListBySeasonID(_ context.Context, seasonID string) ([]*models.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*models.Episode
	for _, ep := range r.episodes {
		if ep.SeasonID == seasonID {
			cp := *ep
			result = append(result, &cp)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i]
		right := result[j]
		if left.SeasonNumber != right.SeasonNumber {
			return left.SeasonNumber < right.SeasonNumber
		}
		return left.EpisodeNumber < right.EpisodeNumber
	})
	return result, nil
}

func (r *fakeEpisodeRepo) Upsert(_ context.Context, ep *models.Episode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserts++
	key := episodeKey(ep.SeriesID, ep.SeasonNumber, ep.EpisodeNumber)
	if existing, ok := r.episodes[key]; ok {
		// Preserve the existing content_id (mirrors the ON CONFLICT behavior).
		ep.ContentID = existing.ContentID
		// Update fields in place.
		cp := *ep
		r.episodes[key] = &cp
	} else {
		cp := *ep
		r.episodes[key] = &cp
	}
	return nil
}

func (r *fakeEpisodeRepo) BulkUpsert(ctx context.Context, _ string, episodes []*models.Episode) error {
	for _, ep := range episodes {
		if err := r.Upsert(ctx, ep); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeEpisodeRepo) UpsertCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.upserts
}

// listBySeries returns all episodes for a series, for test assertions.
func (r *fakeEpisodeRepo) listBySeries(seriesID string) []*models.Episode {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*models.Episode
	for _, ep := range r.episodes {
		if ep.SeriesID == seriesID {
			cp := *ep
			result = append(result, &cp)
		}
	}
	return result
}

// fakeSeasonRepo implements metadataSeasonRepo with an in-memory store keyed
// by (series_id, season_number).
type fakeSeasonRepo struct {
	mu      sync.Mutex
	seasons map[string]*models.Season // key: "seriesID:seasonNum"
	upserts int
}

func newFakeSeasonRepo() *fakeSeasonRepo {
	return &fakeSeasonRepo{seasons: make(map[string]*models.Season)}
}

func seasonKey(seriesID string, seasonNum int) string {
	return fmt.Sprintf("%s:%d", seriesID, seasonNum)
}

func (r *fakeSeasonRepo) GetBySeriesAndNumber(_ context.Context, seriesID string, seasonNum int) (*models.Season, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := seasonKey(seriesID, seasonNum)
	if s, ok := r.seasons[key]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, catalog.ErrSeasonNotFound
}

func (r *fakeSeasonRepo) GetByID(_ context.Context, contentID string) (*models.Season, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, season := range r.seasons {
		if season.ContentID == contentID {
			cp := *season
			return &cp, nil
		}
	}
	return nil, catalog.ErrSeasonNotFound
}

func (r *fakeSeasonRepo) Upsert(_ context.Context, s *models.Season) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserts++
	key := seasonKey(s.SeriesID, s.SeasonNumber)
	if existing, ok := r.seasons[key]; ok {
		// Preserve existing content_id (mirrors ON CONFLICT behavior).
		s.ContentID = existing.ContentID
		cp := *s
		r.seasons[key] = &cp
	} else {
		cp := *s
		r.seasons[key] = &cp
	}
	return nil
}

func (r *fakeSeasonRepo) BulkUpsert(ctx context.Context, seasons []*models.Season) error {
	for _, s := range seasons {
		if err := r.Upsert(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeSeasonRepo) UpsertCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.upserts
}

// fakeEpisodeLinkerFileRepo extends fakeFileRepo with EpisodeLinker methods,
// allowing the fallback synthesis to list unlinked files and link them.
type fakeEpisodeLinkerFileRepo struct {
	fakeFileRepo
	files        map[int]*models.MediaFile // fileID -> file
	episodeLinks map[int]string            // fileID -> episodeID
}

func newFakeEpisodeLinkerFileRepo() *fakeEpisodeLinkerFileRepo {
	return &fakeEpisodeLinkerFileRepo{
		fakeFileRepo: *newFakeFileRepo(),
		files:        make(map[int]*models.MediaFile),
		episodeLinks: make(map[int]string),
	}
}

func (r *fakeEpisodeLinkerFileRepo) addFile(file *models.MediaFile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *file
	r.files[file.ID] = &cp
}

func (r *fakeEpisodeLinkerFileRepo) UpdateEpisodeLink(_ context.Context, fileID int, episodeID string, seasonNum, episodeNum int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.episodeLinks[fileID] = episodeID
	if file, ok := r.files[fileID]; ok {
		file.EpisodeID = episodeID
		file.SeasonNumber = seasonNum
		file.EpisodeNumber = episodeNum
	}
	return nil
}

func (r *fakeEpisodeLinkerFileRepo) ListBySeriesUnlinked(_ context.Context, seriesContentID string) ([]*models.MediaFile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*models.MediaFile
	for _, file := range r.files {
		cid := r.contentIDs[file.ID]
		if cid != seriesContentID {
			continue
		}
		if _, linked := r.episodeLinks[file.ID]; linked {
			continue
		}
		cp := *file
		result = append(result, &cp)
	}
	return result, nil
}

type blockingEpisodeLinkerFileRepo struct {
	*fakeEpisodeLinkerFileRepo
	mu        sync.Mutex
	listCalls int
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
}

func newBlockingEpisodeLinkerFileRepo() *blockingEpisodeLinkerFileRepo {
	return &blockingEpisodeLinkerFileRepo{
		fakeEpisodeLinkerFileRepo: newFakeEpisodeLinkerFileRepo(),
		entered:                   make(chan struct{}),
		release:                   make(chan struct{}),
	}
}

func (r *blockingEpisodeLinkerFileRepo) ListBySeriesUnlinked(ctx context.Context, seriesContentID string) ([]*models.MediaFile, error) {
	r.mu.Lock()
	r.listCalls++
	callNum := r.listCalls
	r.mu.Unlock()

	if callNum == 1 {
		r.enterOnce.Do(func() { close(r.entered) })
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return r.fakeEpisodeLinkerFileRepo.ListBySeriesUnlinked(ctx, seriesContentID)
}

func (r *blockingEpisodeLinkerFileRepo) ListCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listCalls
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type fallbackTestHarness struct {
	service     *MetadataService
	itemRepo    *fakeItemRepo
	fileRepo    *fakeEpisodeLinkerFileRepo
	episodeRepo *fakeEpisodeRepo
	seasonRepo  *fakeSeasonRepo
	libraryRepo *fakeLibraryRepo
}

func newFallbackTestHarness() *fallbackTestHarness {
	h := &fallbackTestHarness{
		itemRepo:    newFakeItemRepo(),
		fileRepo:    newFakeEpisodeLinkerFileRepo(),
		episodeRepo: newFakeEpisodeRepo(),
		seasonRepo:  newFakeSeasonRepo(),
		libraryRepo: newFakeLibraryRepo(),
	}
	h.service = &MetadataService{
		itemRepo:    h.itemRepo,
		fileRepo:    h.fileRepo,
		episodeRepo: h.episodeRepo,
		seasonRepo:  h.seasonRepo,
		libraryRepo: h.libraryRepo,
	}
	return h
}

func mustDate(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("parse date %q: %v", value, err)
	}
	return &parsed
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestFallbackEpisode_UnmatchedSeriesGetsFallbackStructure verifies that an
// unmatched series with parseable S01E01-style files gets fallback season and
// episode rows immediately, without waiting for a provider match.
func TestFallbackEpisode_UnmatchedSeriesGetsFallbackStructure(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-unmatched-1"

	// Create the series item (status pending, no provider data).
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Niche Anime",
		Type:      "series",
		Status:    "pending",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})

	// Add files with parseable S01E01 patterns, linked to the series.
	files := []*models.MediaFile{
		{ID: 1, MediaFolderID: 10, FilePath: "/media/tv/Niche Anime/Season 01/Niche.Anime.S01E01.mkv", SeasonNumber: 1, EpisodeNumber: 1},
		{ID: 2, MediaFolderID: 10, FilePath: "/media/tv/Niche Anime/Season 01/Niche.Anime.S01E02.mkv", SeasonNumber: 1, EpisodeNumber: 2},
		{ID: 3, MediaFolderID: 10, FilePath: "/media/tv/Niche Anime/Season 02/Niche.Anime.S02E01.mkv", SeasonNumber: 2, EpisodeNumber: 1},
	}
	for _, f := range files {
		h.fileRepo.addFile(f)
		h.fileRepo.contentIDs[f.ID] = seriesID
	}

	// Call SynthesizeFallbackEpisodes — this is what the scanner/worker would
	// call right after skeleton creation for an unmatched series.
	if err := h.service.SynthesizeFallbackEpisodes(ctx, seriesID); err != nil {
		t.Fatalf("SynthesizeFallbackEpisodes failed: %v", err)
	}

	// Verify: should have 2 seasons.
	s1, err := h.seasonRepo.GetBySeriesAndNumber(ctx, seriesID, 1)
	if err != nil {
		t.Fatalf("season 1 not created: %v", err)
	}
	if s1.MetadataSource != "scanner_fallback" {
		t.Errorf("season 1 metadata_source: want scanner_fallback, got %q", s1.MetadataSource)
	}

	s2, err := h.seasonRepo.GetBySeriesAndNumber(ctx, seriesID, 2)
	if err != nil {
		t.Fatalf("season 2 not created: %v", err)
	}
	if s2.MetadataSource != "scanner_fallback" {
		t.Errorf("season 2 metadata_source: want scanner_fallback, got %q", s2.MetadataSource)
	}

	// Verify: should have 3 episodes.
	episodes := h.episodeRepo.listBySeries(seriesID)
	if len(episodes) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(episodes))
	}

	// Check specific episodes exist with correct metadata source.
	ep1, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 1)
	if err != nil {
		t.Fatalf("S01E01 not created: %v", err)
	}
	if ep1.MetadataSource != "scanner_fallback" {
		t.Errorf("S01E01 metadata_source: want scanner_fallback, got %q", ep1.MetadataSource)
	}
	if ep1.Title != "Episode 1" {
		t.Errorf("S01E01 title: want %q, got %q", "Episode 1", ep1.Title)
	}

	ep2, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 2)
	if err != nil {
		t.Fatalf("S01E02 not created: %v", err)
	}
	if ep2.MetadataSource != "scanner_fallback" {
		t.Errorf("S01E02 metadata_source: want scanner_fallback, got %q", ep2.MetadataSource)
	}

	ep3, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 2, 1)
	if err != nil {
		t.Fatalf("S02E01 not created: %v", err)
	}
	if ep3.MetadataSource != "scanner_fallback" {
		t.Errorf("S02E01 metadata_source: want scanner_fallback, got %q", ep3.MetadataSource)
	}

	// Verify: the series item should have EpisodeMetadataIncomplete = true.
	item, err := h.itemRepo.GetByID(ctx, seriesID)
	if err != nil {
		t.Fatalf("series item not found: %v", err)
	}
	if !item.EpisodeMetadataIncomplete {
		t.Error("expected EpisodeMetadataIncomplete=true after fallback synthesis")
	}
}

func TestEnsureSeriesEpisodeLinks_LinksDateNamedFileByAirDate(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-daily-1"
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Jeopardy!",
		Type:      "series",
		Status:    "matched",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})
	h.episodeRepo.Upsert(ctx, &models.Episode{
		ContentID:      "ep-jeopardy-2026-82",
		SeriesID:       seriesID,
		SeasonID:       "season-2026",
		SeasonNumber:   2026,
		EpisodeNumber:  82,
		Title:          "Fri, Apr 24, 2026",
		AirDate:        mustDate(t, "2026-04-24"),
		MetadataSource: "provider",
	})

	file := &models.MediaFile{
		ID:            100,
		MediaFolderID: 10,
		FilePath:      "/media/tv/Jeopardy! (1984)/Season 2026/Jeopardy! (1984) - 2026-04-24 - Jamie Ding.mkv",
	}
	h.fileRepo.addFile(file)
	h.fileRepo.contentIDs[file.ID] = seriesID

	if err := h.service.ensureSeriesEpisodeLinks(ctx, seriesID); err != nil {
		t.Fatalf("ensureSeriesEpisodeLinks failed: %v", err)
	}

	if got := h.fileRepo.episodeLinks[file.ID]; got != "ep-jeopardy-2026-82" {
		t.Fatalf("episode link = %q, want ep-jeopardy-2026-82", got)
	}
	linked := h.fileRepo.files[file.ID]
	if linked.SeasonNumber != 2026 || linked.EpisodeNumber != 82 {
		t.Fatalf("linked season/episode = S%dE%d, want S2026E82", linked.SeasonNumber, linked.EpisodeNumber)
	}
}

func TestEnsureSeriesEpisodeLinks_SkipsMissingAirDateMatch(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-daily-missing"
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Daily Show",
		Type:      "series",
		Status:    "matched",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})
	file := &models.MediaFile{
		ID:            101,
		MediaFolderID: 10,
		FilePath:      "/media/tv/Daily Show/Season 2026/Daily Show - 2026-04-24.mkv",
	}
	h.fileRepo.addFile(file)
	h.fileRepo.contentIDs[file.ID] = seriesID

	if err := h.service.ensureSeriesEpisodeLinks(ctx, seriesID); err != nil {
		t.Fatalf("ensureSeriesEpisodeLinks failed: %v", err)
	}

	if got := h.fileRepo.episodeLinks[file.ID]; got != "" {
		t.Fatalf("unexpected episode link = %q", got)
	}
	if episodes := h.episodeRepo.listBySeries(seriesID); len(episodes) != 0 {
		t.Fatalf("date-only file synthesized %d fallback episodes, want 0", len(episodes))
	}
}

func TestEnsureSeriesEpisodeLinks_SkipsAmbiguousAirDateMatch(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-daily-ambiguous"
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Two A Day",
		Type:      "series",
		Status:    "matched",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})
	h.episodeRepo.Upsert(ctx, &models.Episode{
		ContentID:      "ep-one",
		SeriesID:       seriesID,
		SeasonID:       "season-1",
		SeasonNumber:   1,
		EpisodeNumber:  10,
		AirDate:        mustDate(t, "2026-04-24"),
		MetadataSource: "provider",
	})
	h.episodeRepo.Upsert(ctx, &models.Episode{
		ContentID:      "ep-two",
		SeriesID:       seriesID,
		SeasonID:       "season-1",
		SeasonNumber:   1,
		EpisodeNumber:  11,
		AirDate:        mustDate(t, "2026-04-24"),
		MetadataSource: "provider",
	})
	file := &models.MediaFile{
		ID:            102,
		MediaFolderID: 10,
		FilePath:      "/media/tv/Two A Day/Season 01/Two A Day - 2026.04.24.mkv",
	}
	h.fileRepo.addFile(file)
	h.fileRepo.contentIDs[file.ID] = seriesID

	if err := h.service.ensureSeriesEpisodeLinks(ctx, seriesID); err != nil {
		t.Fatalf("ensureSeriesEpisodeLinks failed: %v", err)
	}

	if got := h.fileRepo.episodeLinks[file.ID]; got != "" {
		t.Fatalf("ambiguous date linked to %q, want no link", got)
	}
}

func TestEnsureSeriesEpisodeLinks_PrefersSeriesProviderForAirDateMatch(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-daily-provider-preference"
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Jeopardy!",
		Type:      "series",
		Status:    "matched",
		TvdbID:    "77075",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})
	h.episodeRepo.Upsert(ctx, &models.Episode{
		ContentID:      "ep-tmdb-season-42",
		SeriesID:       seriesID,
		SeasonID:       "season-42",
		SeasonNumber:   42,
		EpisodeNumber:  165,
		Title:          "Show #9550",
		AirDate:        mustDate(t, "2026-04-24"),
		TmdbID:         "7178079",
		MetadataSource: "provider",
	})
	h.episodeRepo.Upsert(ctx, &models.Episode{
		ContentID:      "ep-tvdb-2026-82",
		SeriesID:       seriesID,
		SeasonID:       "season-2026",
		SeasonNumber:   2026,
		EpisodeNumber:  82,
		Title:          "Jamie Ding, Zach Pollock, Nicco Martinez",
		AirDate:        mustDate(t, "2026-04-24"),
		TvdbID:         "11733849",
		MetadataSource: "provider",
	})
	file := &models.MediaFile{
		ID:            103,
		MediaFolderID: 10,
		FilePath:      "/media/tv/Jeopardy! (1984)/Season 2026/Jeopardy! (1984) - 2026-04-24 - Jamie Ding Zach Pollock Nicco Martinez.mkv",
	}
	h.fileRepo.addFile(file)
	h.fileRepo.contentIDs[file.ID] = seriesID

	if err := h.service.ensureSeriesEpisodeLinks(ctx, seriesID); err != nil {
		t.Fatalf("ensureSeriesEpisodeLinks failed: %v", err)
	}

	if got := h.fileRepo.episodeLinks[file.ID]; got != "ep-tvdb-2026-82" {
		t.Fatalf("episode link = %q, want ep-tvdb-2026-82", got)
	}
	linked := h.fileRepo.files[file.ID]
	if linked.SeasonNumber != 2026 || linked.EpisodeNumber != 82 {
		t.Fatalf("linked season/episode = S%dE%d, want S2026E82", linked.SeasonNumber, linked.EpisodeNumber)
	}
}

// TestFallbackEpisode_PartialProviderCoverageKeepsScannerEpisodes verifies that
// when a provider supplies metadata for some episodes but not all, the
// scanner-derived fallback rows are preserved for the missing episodes.
func TestFallbackEpisode_PartialProviderCoverageKeepsScannerEpisodes(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-partial-1"

	// Create the series item (already matched).
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Partial Show",
		Type:      "series",
		Status:    "matched",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})

	// Provider supplied S01E01 only.
	h.seasonRepo.Upsert(ctx, &models.Season{
		ContentID:      "season-provider-1",
		SeriesID:       seriesID,
		SeasonNumber:   1,
		Title:          "Season 1",
		MetadataSource: "provider",
	})
	h.episodeRepo.Upsert(ctx, &models.Episode{
		ContentID:      "ep-provider-s01e01",
		SeriesID:       seriesID,
		SeasonID:       "season-provider-1",
		SeasonNumber:   1,
		EpisodeNumber:  1,
		Title:          "Pilot",
		MetadataSource: "provider",
	})

	// But there are also files for S01E02 and S01E03 that the provider didn't
	// know about.
	files := []*models.MediaFile{
		{ID: 10, MediaFolderID: 10, FilePath: "/media/tv/Partial Show/Season 01/Partial.Show.S01E02.mkv", SeasonNumber: 1, EpisodeNumber: 2},
		{ID: 11, MediaFolderID: 10, FilePath: "/media/tv/Partial Show/Season 01/Partial.Show.S01E03.mkv", SeasonNumber: 1, EpisodeNumber: 3},
	}
	for _, f := range files {
		h.fileRepo.addFile(f)
		h.fileRepo.contentIDs[f.ID] = seriesID
	}

	// Synthesize fallback episodes for the gaps.
	if err := h.service.SynthesizeFallbackEpisodes(ctx, seriesID); err != nil {
		t.Fatalf("SynthesizeFallbackEpisodes failed: %v", err)
	}

	// Provider episode S01E01 should be untouched.
	ep1, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 1)
	if err != nil {
		t.Fatalf("S01E01 missing: %v", err)
	}
	if ep1.ContentID != "ep-provider-s01e01" {
		t.Errorf("S01E01 content_id changed: want ep-provider-s01e01, got %q", ep1.ContentID)
	}
	if ep1.Title != "Pilot" {
		t.Errorf("S01E01 title changed: want Pilot, got %q", ep1.Title)
	}
	if ep1.MetadataSource != "provider" {
		t.Errorf("S01E01 metadata_source changed: want provider, got %q", ep1.MetadataSource)
	}

	// Scanner-derived episodes should exist for the gaps.
	ep2, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 2)
	if err != nil {
		t.Fatalf("S01E02 not created: %v", err)
	}
	if ep2.MetadataSource != "scanner_fallback" {
		t.Errorf("S01E02 metadata_source: want scanner_fallback, got %q", ep2.MetadataSource)
	}

	ep3, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 3)
	if err != nil {
		t.Fatalf("S01E03 not created: %v", err)
	}
	if ep3.MetadataSource != "scanner_fallback" {
		t.Errorf("S01E03 metadata_source: want scanner_fallback, got %q", ep3.MetadataSource)
	}

	// Total episodes should be 3.
	allEpisodes := h.episodeRepo.listBySeries(seriesID)
	if len(allEpisodes) != 3 {
		t.Errorf("expected 3 total episodes, got %d", len(allEpisodes))
	}
}

// TestFallbackEpisode_ProviderUpsertReusesExistingRow verifies that when a
// provider later supplies an episode that was previously created as
// scanner_fallback, the upsert reuses the same row (same content_id) and
// upgrades its metadata in place.
func TestFallbackEpisode_ProviderUpsertReusesExistingRow(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	seriesID := "series-upgrade-1"

	// Create the series item.
	h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: seriesID,
		Title:     "Upgrade Show",
		Type:      "series",
		Status:    "pending",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	})

	// Add a file for S01E01.
	h.fileRepo.addFile(&models.MediaFile{
		ID: 20, MediaFolderID: 10,
		FilePath:      "/media/tv/Upgrade Show/Season 01/Upgrade.Show.S01E01.mkv",
		SeasonNumber:  1,
		EpisodeNumber: 1,
	})
	h.fileRepo.contentIDs[20] = seriesID

	// First: synthesize fallback (scanner-derived) episode.
	if err := h.service.SynthesizeFallbackEpisodes(ctx, seriesID); err != nil {
		t.Fatalf("initial SynthesizeFallbackEpisodes failed: %v", err)
	}

	// Record the scanner-created episode's content_id.
	scannerEp, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 1)
	if err != nil {
		t.Fatalf("scanner episode not found: %v", err)
	}
	originalContentID := scannerEp.ContentID
	if scannerEp.MetadataSource != "scanner_fallback" {
		t.Fatalf("expected scanner_fallback source, got %q", scannerEp.MetadataSource)
	}
	if scannerEp.Title != "Episode 1" {
		t.Fatalf("expected fallback title 'Episode 1', got %q", scannerEp.Title)
	}

	// Second: provider supplies richer metadata for the same episode.
	providerEp := &models.Episode{
		ContentID:      "ep-provider-new-id", // provider would use a new ID, but Upsert should preserve the original
		SeriesID:       seriesID,
		SeasonID:       scannerEp.SeasonID,
		SeasonNumber:   1,
		EpisodeNumber:  1,
		Title:          "The Real Pilot",
		Overview:       "An amazing first episode",
		TmdbID:         "tmdb-ep-999",
		MetadataSource: "provider",
	}
	if err := h.episodeRepo.Upsert(ctx, providerEp); err != nil {
		t.Fatalf("provider upsert failed: %v", err)
	}

	// The content_id should be preserved from the original scanner row.
	if providerEp.ContentID != originalContentID {
		t.Errorf("content_id changed: want %q (original), got %q", originalContentID, providerEp.ContentID)
	}

	// The metadata should be upgraded.
	upgraded, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 1)
	if err != nil {
		t.Fatalf("upgraded episode not found: %v", err)
	}
	if upgraded.Title != "The Real Pilot" {
		t.Errorf("title not upgraded: want %q, got %q", "The Real Pilot", upgraded.Title)
	}
	if upgraded.MetadataSource != "provider" {
		t.Errorf("metadata_source not upgraded: want provider, got %q", upgraded.MetadataSource)
	}
	if upgraded.TmdbID != "tmdb-ep-999" {
		t.Errorf("tmdb_id not set: want tmdb-ep-999, got %q", upgraded.TmdbID)
	}

	// Should still be only 1 episode — no duplicates.
	allEpisodes := h.episodeRepo.listBySeries(seriesID)
	if len(allEpisodes) != 1 {
		t.Errorf("expected 1 episode (no duplicates), got %d", len(allEpisodes))
	}
}

// TestFallbackEpisode_WorkerSynthesizesFallbackOnEnrichmentFailure verifies the
// end-to-end worker path: when the enrichment pipeline fails for a series file,
// the worker still synthesizes fallback episodes so the content is browsable.
func TestFallbackEpisode_WorkerSynthesizesFallbackOnEnrichmentFailure(t *testing.T) {
	h := newFallbackTestHarness()
	ctx := context.Background()

	file := &models.MediaFile{
		ID:            30,
		MediaFolderID: 10,
		FilePath:      "/media/tv/Obscure Show (2024)/Season 01/Obscure.Show.S01E01.mkv",
	}

	// Hook createOrFindSkeleton to return a series skeleton and set up
	// the file linkage needed for ListBySeriesUnlinked to find this file.
	seriesID := "series-worker-fallback"
	h.service.hooks.createOrFindSkeleton = func(_ context.Context, f *models.MediaFile, _ int) (*skeletonResult, error) {
		h.itemRepo.Upsert(ctx, &models.MediaItem{
			ContentID: seriesID,
			Title:     "Obscure Show",
			Year:      2024,
			Type:      "series",
			Status:    "pending",
			Studios:   []string{},
			Networks:  []string{},
			Countries: []string{},
			Genres:    []string{},
		})
		h.fileRepo.addFile(f)
		h.fileRepo.contentIDs[f.ID] = seriesID
		return &skeletonResult{
			ContentID: seriesID,
			IsNew:     true,
			Type:      "series",
			Title:     "Obscure Show",
			Year:      2024,
		}, nil
	}

	// Simulate enrichment failure (no providers matched).
	h.service.hooks.process = func(_ context.Context, _ ProcessRequest) (*ProcessResult, error) {
		return &ProcessResult{Updated: false}, nil
	}

	worker := NewMatchWorker(h.service, h.fileRepo, 1, 10, 0)
	worker.ProcessFile(ctx, file)

	// The item should be marked "unmatched".
	item, err := h.itemRepo.GetByID(ctx, seriesID)
	if err != nil {
		t.Fatalf("series item not found: %v", err)
	}
	if item.Status != "unmatched" {
		t.Errorf("expected status=unmatched, got %q", item.Status)
	}

	// But a fallback episode should have been synthesized.
	ep, err := h.episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 1)
	if err != nil {
		t.Fatalf("expected fallback S01E01 to exist after worker enrichment failure: %v", err)
	}
	if ep.MetadataSource != "scanner_fallback" {
		t.Errorf("expected scanner_fallback source, got %q", ep.MetadataSource)
	}

	// Season should also exist.
	s, err := h.seasonRepo.GetBySeriesAndNumber(ctx, seriesID, 1)
	if err != nil {
		t.Fatalf("expected fallback Season 1 to exist: %v", err)
	}
	if s.MetadataSource != "scanner_fallback" {
		t.Errorf("expected scanner_fallback source for season, got %q", s.MetadataSource)
	}
}

func TestEnsureSeriesEpisodeLinks_CoalescesConcurrentFallbackWork(t *testing.T) {
	ctx := context.Background()

	itemRepo := newFakeItemRepo()
	fileRepo := newBlockingEpisodeLinkerFileRepo()
	episodeRepo := newFakeEpisodeRepo()
	seasonRepo := newFakeSeasonRepo()
	libraryRepo := newFakeLibraryRepo()

	service := &MetadataService{
		itemRepo:    itemRepo,
		fileRepo:    fileRepo,
		episodeRepo: episodeRepo,
		seasonRepo:  seasonRepo,
		libraryRepo: libraryRepo,
	}

	seriesID := "series-coalesced-1"
	if err := itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID:                 seriesID,
		Title:                     "EastEnders",
		Type:                      "series",
		Status:                    "unmatched",
		EpisodeMetadataIncomplete: true,
		Studios:                   []string{},
		Networks:                  []string{},
		Countries:                 []string{},
		Genres:                    []string{},
	}); err != nil {
		t.Fatalf("upserting series item: %v", err)
	}

	file := &models.MediaFile{
		ID:            1,
		MediaFolderID: 10,
		FilePath:      "/media/tv/EastEnders/Season 01/EastEnders.S01E01.mkv",
		SeasonNumber:  1,
		EpisodeNumber: 1,
	}
	fileRepo.addFile(file)
	fileRepo.contentIDs[file.ID] = seriesID

	const concurrentCalls = 16
	errCh := make(chan error, concurrentCalls)

	go func() {
		errCh <- service.ensureSeriesEpisodeLinks(ctx, seriesID)
	}()

	<-fileRepo.entered

	var wg sync.WaitGroup
	for i := 1; i < concurrentCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- service.ensureSeriesEpisodeLinks(ctx, seriesID)
		}()
	}

	close(fileRepo.release)
	wg.Wait()

	for i := 0; i < concurrentCalls; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("ensureSeriesEpisodeLinks returned error: %v", err)
		}
	}

	if got := seasonRepo.UpsertCalls(); got != 1 {
		t.Fatalf("expected 1 fallback season upsert, got %d", got)
	}
	if got := episodeRepo.UpsertCalls(); got != 1 {
		t.Fatalf("expected 1 fallback episode upsert, got %d", got)
	}

	ep, err := episodeRepo.GetBySeriesAndNumber(ctx, seriesID, 1, 1)
	if err != nil {
		t.Fatalf("expected synthesized episode to exist: %v", err)
	}
	if ep.MetadataSource != "scanner_fallback" {
		t.Fatalf("expected synthesized episode metadata source scanner_fallback, got %q", ep.MetadataSource)
	}
}
