package metadata

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

type fakeImageCacheJobs struct {
	claimed       []*models.MetadataImageCacheJob
	claimLimit    int
	succeededID   int64
	failedID      int64
	failedText    string
	deletedCount  int
	requeuedIDs   []int64
	currentSource *string // when set, overrides CurrentTargetSourcePath
	statusCounts  ImageCacheJobStatusCounts
}

func (f *fakeImageCacheJobs) ClaimDue(_ context.Context, _ string, limit int) ([]*models.MetadataImageCacheJob, error) {
	f.claimLimit = limit
	return f.claimed, nil
}

func (f *fakeImageCacheJobs) MarkSucceeded(_ context.Context, id int64, _ string) error {
	f.succeededID = id
	return nil
}

func (f *fakeImageCacheJobs) MarkFailed(_ context.Context, id int64, _ int, _ string, errText string) error {
	f.failedID = id
	f.failedText = errText
	return nil
}

func (f *fakeImageCacheJobs) RequeueClaimed(_ context.Context, ids []int64, _ string) error {
	f.requeuedIDs = append(f.requeuedIDs, ids...)
	return nil
}

func (f *fakeImageCacheJobs) CurrentTargetSourcePath(_ context.Context, job *models.MetadataImageCacheJob) (string, error) {
	if f.currentSource != nil {
		return *f.currentSource, nil
	}
	return job.SourcePath, nil
}

func (f *fakeImageCacheJobs) EnqueueExistingProviderArtwork(context.Context, int) (int, error) {
	return 0, nil
}

func (f *fakeImageCacheJobs) DeleteSucceededBefore(context.Context, time.Time, int) (int, error) {
	return f.deletedCount, nil
}

func (f *fakeImageCacheJobs) StatusCounts(context.Context) (ImageCacheJobStatusCounts, error) {
	return f.statusCounts, nil
}

func (f *fakeImageCacheJobs) Heartbeat(context.Context, int64, string) (bool, error) {
	return true, nil
}

type loopingImageCacheJobs struct {
	enqueueResults []int
	claimedResults [][]*models.MetadataImageCacheJob
	succeededIDs   []int64
	enqueueCalls   int
	claimCalls     int
}

func (f *loopingImageCacheJobs) EnqueueExistingProviderArtwork(context.Context, int) (int, error) {
	result := 0
	if f.enqueueCalls < len(f.enqueueResults) {
		result = f.enqueueResults[f.enqueueCalls]
	}
	f.enqueueCalls++
	return result, nil
}

func (f *loopingImageCacheJobs) ClaimDue(context.Context, string, int) ([]*models.MetadataImageCacheJob, error) {
	var result []*models.MetadataImageCacheJob
	if f.claimCalls < len(f.claimedResults) {
		result = f.claimedResults[f.claimCalls]
	}
	f.claimCalls++
	return result, nil
}

func (f *loopingImageCacheJobs) MarkSucceeded(_ context.Context, id int64, _ string) error {
	f.succeededIDs = append(f.succeededIDs, id)
	return nil
}

func (f *loopingImageCacheJobs) MarkFailed(context.Context, int64, int, string, string) error {
	return nil
}

func (f *loopingImageCacheJobs) RequeueClaimed(context.Context, []int64, string) error {
	return nil
}

func (f *loopingImageCacheJobs) CurrentTargetSourcePath(_ context.Context, job *models.MetadataImageCacheJob) (string, error) {
	return job.SourcePath, nil
}

func (f *loopingImageCacheJobs) DeleteSucceededBefore(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func (f *loopingImageCacheJobs) StatusCounts(context.Context) (ImageCacheJobStatusCounts, error) {
	return ImageCacheJobStatusCounts{}, nil
}

func (f *loopingImageCacheJobs) Heartbeat(context.Context, int64, string) (bool, error) {
	return true, nil
}

type fakeImageCacher struct {
	result *CacheImageResult
	err    error
	reqs   []CacheImageRequest
}

func (f *fakeImageCacher) CacheImage(_ context.Context, req CacheImageRequest) (*CacheImageResult, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type fakeImageResolver struct {
	url string
}

func (f *fakeImageResolver) ResolveImageURL(context.Context, string, string) string {
	return f.url
}

type fakeEpisodeStillUpdater struct {
	updated    bool
	contentID  string
	sourcePath string
	cachedPath string
	thumbhash  string
}

func (f *fakeEpisodeStillUpdater) UpdateStillIfSourceMatches(_ context.Context, contentID, sourcePath, cachedPath, thumbhash string) (bool, error) {
	f.contentID = contentID
	f.sourcePath = sourcePath
	f.cachedPath = cachedPath
	f.thumbhash = thumbhash
	return f.updated, nil
}

type fakeItemArtworkUpdater struct {
	mu         sync.Mutex
	updated    bool
	contentID  string
	imageType  string
	sourcePath string
	cachedPath string
	thumbhash  string
}

func (f *fakeItemArtworkUpdater) UpdateArtworkIfSourceMatches(_ context.Context, contentID, imageType, sourcePath, cachedPath, thumbhash string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contentID = contentID
	f.imageType = imageType
	f.sourcePath = sourcePath
	f.cachedPath = cachedPath
	f.thumbhash = thumbhash
	return f.updated, nil
}

type fakeItemLocalizationArtworkUpdater struct {
	updated    bool
	contentID  string
	language   string
	imageType  string
	sourcePath string
	cachedPath string
	thumbhash  string
}

func (f *fakeItemLocalizationArtworkUpdater) UpdateArtworkIfSourceMatches(_ context.Context, contentID, language, imageType, sourcePath, cachedPath, thumbhash string) (bool, error) {
	f.contentID = contentID
	f.language = language
	f.imageType = imageType
	f.sourcePath = sourcePath
	f.cachedPath = cachedPath
	f.thumbhash = thumbhash
	return f.updated, nil
}

type fakePersonPhotoUpdater struct {
	updated    bool
	personID   int64
	sourcePath string
	cachedPath string
	thumbhash  string
}

func (f *fakePersonPhotoUpdater) UpdatePhotoIfSourceMatches(_ context.Context, personID int64, sourcePath, cachedPath, thumbhash string) (bool, error) {
	f.personID = personID
	f.sourcePath = sourcePath
	f.cachedPath = cachedPath
	f.thumbhash = thumbhash
	return f.updated, nil
}

func TestImageCacheProcessorUpdatesEpisodeOnSuccess(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                1,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-tvdb-1-1-1",
		SourcePath:        "tvdb://banners/episode.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(1),
	}}}
	cacher := &fakeImageCacher{result: &CacheImageResult{
		BasePath:  "tvdb/series/1/seasons/1/episodes/1/still",
		Ext:       ".webp",
		Thumbhash: "thumb",
	}}
	resolver := &fakeImageResolver{url: "https://artworks.thetvdb.com/banners/episode.jpg"}
	episodes := &fakeEpisodeStillUpdater{updated: true}

	processor := NewImageCacheProcessor(jobs, cacher, resolver, nil, episodes)
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1", stats.Succeeded)
	}
	if episodes.cachedPath != "tvdb/series/1/seasons/1/episodes/1/still/original.webp" {
		t.Fatalf("cachedPath = %q", episodes.cachedPath)
	}
	if episodes.sourcePath != "tvdb://banners/episode.jpg" {
		t.Fatalf("sourcePath = %q", episodes.sourcePath)
	}
	if jobs.succeededID != 1 {
		t.Fatalf("succeededID = %d", jobs.succeededID)
	}
}

func TestImageCacheProcessorUpdatesItemArtworkOnSuccess(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                20,
		TargetType:        ImageCacheTargetItem,
		TargetContentID:   "series-1",
		SourcePath:        "tmdb://poster/series.jpg",
		ProviderID:        "tmdb",
		ProviderContentID: "1396",
		ContentType:       "series",
		ImageType:         ImageCacheImageBackdrop,
	}}}
	cacher := &fakeImageCacher{result: &CacheImageResult{
		BasePath:  "tmdb/series/1396/backdrop",
		Ext:       ".webp",
		Thumbhash: "thumb",
	}}
	resolver := &fakeImageResolver{url: "https://image.tmdb.org/t/p/original/backdrop.jpg"}
	items := &fakeItemArtworkUpdater{updated: true}

	processor := NewImageCacheProcessorWithTargets(jobs, cacher, resolver, ImageCacheProcessorTargets{
		Items: items,
	})
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1", stats.Succeeded)
	}
	if items.imageType != ImageCacheImageBackdrop {
		t.Fatalf("imageType = %q, want backdrop", items.imageType)
	}
	if items.cachedPath != "tmdb/series/1396/backdrop/original.webp" {
		t.Fatalf("cachedPath = %q", items.cachedPath)
	}
}

func TestImageCacheProcessorPassesLanguageToLocalizedItemArtwork(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                21,
		TargetType:        ImageCacheTargetItemLocalization,
		TargetContentID:   "series-1",
		TargetLanguage:    "fr",
		SourcePath:        "tmdb://logo/fr.png",
		ProviderID:        "tmdb",
		ProviderContentID: "1396",
		ContentType:       "series",
		ImageType:         ImageCacheImageLogo,
	}}}
	cacher := &fakeImageCacher{result: &CacheImageResult{
		BasePath: "tmdb/series/1396/localizations/fr/logo",
		Ext:      ".webp",
	}}
	resolver := &fakeImageResolver{url: "https://image.tmdb.org/t/p/original/logo.png"}
	localizations := &fakeItemLocalizationArtworkUpdater{updated: true}

	processor := NewImageCacheProcessorWithTargets(jobs, cacher, resolver, ImageCacheProcessorTargets{
		ItemLocalizations: localizations,
	})
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1", stats.Succeeded)
	}
	if len(cacher.reqs) != 1 || cacher.reqs[0].Language != "fr" {
		t.Fatalf("cached request language = %#v, want fr", cacher.reqs)
	}
	if localizations.language != "fr" || localizations.imageType != ImageCacheImageLogo {
		t.Fatalf("localization update = language %q image %q", localizations.language, localizations.imageType)
	}
}

func TestImageCacheProcessorUpdatesPersonProfileOnSuccess(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                22,
		TargetType:        ImageCacheTargetPerson,
		TargetContentID:   "287",
		SourcePath:        "tmdb://profile/287.jpg",
		ProviderID:        "tmdb",
		ProviderContentID: "287",
		ContentType:       "people",
		ImageType:         ImageCacheImageProfile,
	}}}
	cacher := &fakeImageCacher{result: &CacheImageResult{
		BasePath:  "tmdb/people/287/profile",
		Ext:       ".webp",
		Thumbhash: "person-thumb",
	}}
	resolver := &fakeImageResolver{url: "https://image.tmdb.org/t/p/original/person.jpg"}
	people := &fakePersonPhotoUpdater{updated: true}

	processor := NewImageCacheProcessorWithTargets(jobs, cacher, resolver, ImageCacheProcessorTargets{
		People: people,
	})
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1", stats.Succeeded)
	}
	if people.personID != 287 {
		t.Fatalf("personID = %d, want 287", people.personID)
	}
	if people.cachedPath != "tmdb/people/287/profile/original.webp" {
		t.Fatalf("cachedPath = %q", people.cachedPath)
	}
	if people.thumbhash != "person-thumb" {
		t.Fatalf("thumbhash = %q", people.thumbhash)
	}
}

func TestImageCacheProcessorMarksSkippedWhenSourceNoLongerMatches(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                2,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-tvdb-1-1-1",
		SourcePath:        "tvdb://banners/episode.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(1),
	}}}
	cacher := &fakeImageCacher{result: &CacheImageResult{BasePath: "tvdb/series/1/seasons/1/episodes/1/still", Ext: ".webp"}}
	resolver := &fakeImageResolver{url: "https://artworks.thetvdb.com/banners/episode.jpg"}
	episodes := &fakeEpisodeStillUpdater{updated: false}

	processor := NewImageCacheProcessor(jobs, cacher, resolver, nil, episodes)
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", stats.Skipped)
	}
	if jobs.succeededID != 2 {
		t.Fatalf("succeededID = %d, want 2", jobs.succeededID)
	}
}

func TestImageCacheProcessorMarksFailureOnCacheError(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                3,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-tvdb-1-1-1",
		SourcePath:        "tvdb://banners/episode.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(1),
	}}}
	cacher := &fakeImageCacher{err: errors.New("cache failed")}
	resolver := &fakeImageResolver{url: "https://artworks.thetvdb.com/banners/episode.jpg"}
	episodes := &fakeEpisodeStillUpdater{updated: false}

	processor := NewImageCacheProcessor(jobs, cacher, resolver, nil, episodes)
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", stats.Failed)
	}
	if jobs.failedID != 3 {
		t.Fatalf("failedID = %d, want 3", jobs.failedID)
	}
}

func TestImageCacheProcessorMarksFailureOnEmptyCacheResult(t *testing.T) {
	jobs := &fakeImageCacheJobs{claimed: []*models.MetadataImageCacheJob{{
		ID:                4,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-tvdb-1-1-1",
		SourcePath:        "tvdb://banners/episode.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(1),
	}}}
	cacher := &fakeImageCacher{result: &CacheImageResult{}}
	resolver := &fakeImageResolver{url: "https://artworks.thetvdb.com/banners/episode.jpg"}
	episodes := &fakeEpisodeStillUpdater{updated: true}

	processor := NewImageCacheProcessor(jobs, cacher, resolver, nil, episodes)
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", stats.Failed)
	}
	if jobs.failedID != 4 {
		t.Fatalf("failedID = %d, want 4", jobs.failedID)
	}
	if episodes.cachedPath != "" {
		t.Fatalf("episode updater was called with cachedPath = %q", episodes.cachedPath)
	}
}

func TestImageCacheProcessorDeletesOldSucceededJobsWithoutClaimedJobs(t *testing.T) {
	jobs := &fakeImageCacheJobs{deletedCount: 7}
	processor := NewImageCacheProcessor(jobs, &fakeImageCacher{}, nil, nil, nil)

	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.DeletedSucceeded != 7 {
		t.Fatalf("DeletedSucceeded = %d, want 7", stats.DeletedSucceeded)
	}
}

func TestImageCacheProcessorRunUntilIdleDrainsNewWorkAddedDuringRun(t *testing.T) {
	job1 := &models.MetadataImageCacheJob{
		ID:                10,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-tvdb-1-1-1",
		SourcePath:        "tvdb://banners/episode-1.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(1),
	}
	job2 := &models.MetadataImageCacheJob{
		ID:                11,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-tvdb-1-1-2",
		SourcePath:        "tvdb://banners/episode-2.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(2),
	}
	// Discovery now runs only when the queue drains, not on every batch: drain
	// job1, find the queue empty and sweep (enqueues 1 more), drain job2, find
	// the queue empty and sweep again (enqueues 0 -> idle).
	jobs := &loopingImageCacheJobs{
		enqueueResults: []int{1, 0},
		claimedResults: [][]*models.MetadataImageCacheJob{
			{job1},
			{},
			{job2},
			{},
		},
	}
	cacher := &fakeImageCacher{result: &CacheImageResult{
		BasePath: "tvdb/series/1/seasons/1/episodes/1/still",
		Ext:      ".webp",
	}}
	resolver := &fakeImageResolver{url: "https://artworks.thetvdb.com/banners/episode.jpg"}
	episodes := &fakeEpisodeStillUpdater{updated: true}

	processor := NewImageCacheProcessor(jobs, cacher, resolver, nil, episodes)
	stats, err := processor.RunUntilIdle(context.Background(), "test-worker", 1000, 2, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle() error = %v", err)
	}
	if stats.EnqueuedExisting != 1 || stats.Claimed != 2 || stats.Succeeded != 2 {
		t.Fatalf("stats = %+v, want enqueued=1 claimed=2 succeeded=2", stats)
	}
	if jobs.enqueueCalls != 2 {
		t.Fatalf("enqueueCalls=%d, want 2", jobs.enqueueCalls)
	}
	// Rolling pool refills while work is in flight, so claim count is at least
	// the old batch-barrier sequence (4) and may be higher.
	if jobs.claimCalls < 4 {
		t.Fatalf("claimCalls=%d, want >= 4", jobs.claimCalls)
	}
	if len(jobs.succeededIDs) != 2 || jobs.succeededIDs[0] != 10 || jobs.succeededIDs[1] != 11 {
		t.Fatalf("succeededIDs = %#v, want [10 11]", jobs.succeededIDs)
	}
}

func TestImageCacheProcessorSkipsWhenTargetSourceChanged(t *testing.T) {
	// A stale job whose target no longer references its source must not upload.
	changed := "tmdb://poster/new.jpg"
	jobs := &fakeImageCacheJobs{
		claimed: []*models.MetadataImageCacheJob{{
			ID:                40,
			TargetType:        ImageCacheTargetItem,
			TargetContentID:   "series-1",
			SourcePath:        "tmdb://poster/old.jpg",
			ProviderID:        "tmdb",
			ProviderContentID: "1396",
			ContentType:       "series",
			ImageType:         ImageCacheImagePoster,
		}},
		currentSource: &changed,
	}
	cacher := &fakeImageCacher{result: &CacheImageResult{BasePath: "tmdb/series/1396/poster", Ext: ".webp"}}
	resolver := &fakeImageResolver{url: "https://image.tmdb.org/t/p/original/poster.jpg"}
	items := &fakeItemArtworkUpdater{updated: true}

	processor := NewImageCacheProcessorWithTargets(jobs, cacher, resolver, ImageCacheProcessorTargets{Items: items})
	stats, err := processor.RunOnce(context.Background(), "test-worker", 10, 1)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", stats.Skipped)
	}
	if len(cacher.reqs) != 0 {
		t.Fatalf("CacheImage called %d times, want 0 (stale job must not upload)", len(cacher.reqs))
	}
	if items.cachedPath != "" {
		t.Fatalf("item updater called with cachedPath = %q, want none", items.cachedPath)
	}
	if jobs.succeededID != 40 {
		t.Fatalf("succeededID = %d, want 40", jobs.succeededID)
	}
}

func TestImageCacheProcessorCapsClaimToConcurrency(t *testing.T) {
	jobs := &fakeImageCacheJobs{}
	processor := NewImageCacheProcessor(jobs, &fakeImageCacher{}, nil, nil, nil)
	if _, err := processor.RunOnce(context.Background(), "test-worker", 1000, 12); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if jobs.claimLimit != 12 {
		t.Fatalf("claimLimit = %d, want 12 (capped to concurrency)", jobs.claimLimit)
	}
}

func TestImageCacheProcessorReportsSucceededOverTotalProgress(t *testing.T) {
	jobs := &fakeImageCacheJobs{
		statusCounts: ImageCacheJobStatusCounts{
			Queued:    125,
			Running:   12,
			Succeeded: 150,
			Failed:    0,
		},
	}
	processor := NewImageCacheProcessor(jobs, &fakeImageCacher{}, nil, nil, nil)
	var percent float64
	var message string
	processor.reportQueueProgress(context.Background(), func(p float64, msg string) {
		percent = p
		message = msg
	})
	wantPercent := 150.0 / 287.0 * 100
	if percent < wantPercent-0.01 || percent > wantPercent+0.01 {
		t.Fatalf("percent = %v, want ~%v", percent, wantPercent)
	}
	if message != "Cached 150/287 (queued 125, running 12, failed 0)" {
		t.Fatalf("message = %q", message)
	}
}

// queueImageCacheJobs is a ClaimDue-backed queue that tracks in-flight and
// terminal transitions for rolling-pool concurrency tests.
type queueImageCacheJobs struct {
	mu          sync.Mutex
	queue       []*models.MetadataImageCacheJob
	inFlight    int
	maxInFlight int
	terminal    map[int64]string // id -> succeeded|failed|requeued
	claimLimits []int
	requeuedIDs []int64
}

func (q *queueImageCacheJobs) ClaimDue(_ context.Context, _ string, limit int) ([]*models.MetadataImageCacheJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claimLimits = append(q.claimLimits, limit)
	if limit <= 0 || len(q.queue) == 0 {
		return nil, nil
	}
	n := limit
	if n > len(q.queue) {
		n = len(q.queue)
	}
	out := q.queue[:n]
	q.queue = q.queue[n:]
	q.inFlight += len(out)
	if q.inFlight > q.maxInFlight {
		q.maxInFlight = q.inFlight
	}
	return out, nil
}

func (q *queueImageCacheJobs) markTerminal(id int64, status string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.terminal == nil {
		q.terminal = make(map[int64]string)
	}
	q.terminal[id] = status
	if q.inFlight > 0 {
		q.inFlight--
	}
}

func (q *queueImageCacheJobs) MarkSucceeded(_ context.Context, id int64, _ string) error {
	q.markTerminal(id, "succeeded")
	return nil
}

func (q *queueImageCacheJobs) MarkFailed(_ context.Context, id int64, _ int, _ string, _ string) error {
	q.markTerminal(id, "failed")
	return nil
}

func (q *queueImageCacheJobs) RequeueClaimed(_ context.Context, ids []int64, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.requeuedIDs = append(q.requeuedIDs, ids...)
	if q.terminal == nil {
		q.terminal = make(map[int64]string)
	}
	for _, id := range ids {
		q.terminal[id] = "requeued"
		if q.inFlight > 0 {
			q.inFlight--
		}
	}
	return nil
}

func (q *queueImageCacheJobs) CurrentTargetSourcePath(_ context.Context, job *models.MetadataImageCacheJob) (string, error) {
	return job.SourcePath, nil
}

func (q *queueImageCacheJobs) EnqueueExistingProviderArtwork(context.Context, int) (int, error) {
	return 0, nil
}

func (q *queueImageCacheJobs) DeleteSucceededBefore(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func (q *queueImageCacheJobs) StatusCounts(context.Context) (ImageCacheJobStatusCounts, error) {
	return ImageCacheJobStatusCounts{}, nil
}

func (q *queueImageCacheJobs) Heartbeat(context.Context, int64, string) (bool, error) {
	return true, nil
}

type slowImageCacher struct {
	delay time.Duration
	gate  chan struct{} // optional: block until closed
}

func (s *slowImageCacher) CacheImage(ctx context.Context, req CacheImageRequest) (*CacheImageResult, error) {
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &CacheImageResult{
		BasePath:  "tmdb/series/" + req.ContentID + "/poster",
		Ext:       ".webp",
		Thumbhash: "t",
	}, nil
}

func TestImageCacheProcessorRollingPoolCapsInFlightAndTerminatesOnce(t *testing.T) {
	const concurrency = 3
	const jobCount = 12
	queue := make([]*models.MetadataImageCacheJob, 0, jobCount)
	for i := 1; i <= jobCount; i++ {
		queue = append(queue, &models.MetadataImageCacheJob{
			ID:                int64(i),
			TargetType:        ImageCacheTargetItem,
			TargetContentID:   "series-1",
			SourcePath:        "https://example.test/poster.jpg",
			ProviderID:        "tmdb",
			ProviderContentID: "1396",
			ContentType:       "series",
			ImageType:         ImageCacheImagePoster,
			LockedBy:          "test-worker",
		})
	}
	jobs := &queueImageCacheJobs{queue: queue}
	cacher := &slowImageCacher{delay: 20 * time.Millisecond}
	items := &fakeItemArtworkUpdater{updated: true}
	processor := NewImageCacheProcessorWithTargets(jobs, cacher, nil, ImageCacheProcessorTargets{Items: items})
	// Skip discovery so the run ends when the queue drains.
	processor.lastDiscovery = time.Now()

	stats, err := processor.RunUntilIdle(context.Background(), "test-worker", 1000, concurrency, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle() error = %v", err)
	}
	if stats.Claimed != jobCount || stats.Succeeded != jobCount {
		t.Fatalf("stats = %+v, want claimed=%d succeeded=%d", stats, jobCount, jobCount)
	}
	if stats.MaxInFlight > concurrency {
		t.Fatalf("MaxInFlight = %d, want <= %d", stats.MaxInFlight, concurrency)
	}
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.maxInFlight > concurrency {
		t.Fatalf("observed maxInFlight = %d, want <= %d", jobs.maxInFlight, concurrency)
	}
	if len(jobs.terminal) != jobCount {
		t.Fatalf("terminal count = %d, want %d: %#v", len(jobs.terminal), jobCount, jobs.terminal)
	}
	for id, status := range jobs.terminal {
		if status != "succeeded" {
			t.Fatalf("job %d terminal status = %q, want succeeded", id, status)
		}
	}
	for _, limit := range jobs.claimLimits {
		if limit > concurrency {
			t.Fatalf("ClaimDue limit %d exceeds concurrency %d", limit, concurrency)
		}
	}
}

func TestImageCacheProcessorRollingPoolRequeuesUnstartedOnCancel(t *testing.T) {
	const concurrency = 2
	gate := make(chan struct{})
	queue := make([]*models.MetadataImageCacheJob, 0, 6)
	for i := 1; i <= 6; i++ {
		queue = append(queue, &models.MetadataImageCacheJob{
			ID:                int64(i),
			TargetType:        ImageCacheTargetItem,
			TargetContentID:   "series-1",
			SourcePath:        "https://example.test/poster.jpg",
			ProviderID:        "tmdb",
			ProviderContentID: "1396",
			ContentType:       "series",
			ImageType:         ImageCacheImagePoster,
			LockedBy:          "test-worker",
		})
	}
	jobs := &queueImageCacheJobs{queue: queue}
	cacher := &slowImageCacher{gate: gate}
	items := &fakeItemArtworkUpdater{updated: true}
	processor := NewImageCacheProcessorWithTargets(jobs, cacher, nil, ImageCacheProcessorTargets{Items: items})
	processor.lastDiscovery = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan ImageCacheRunStats, 1)
	go func() {
		stats, _ := processor.RunUntilIdle(ctx, "test-worker", 1000, concurrency, time.Minute, nil)
		done <- stats
	}()

	// Let the pool claim and start workers, then cancel before unblocking cache.
	time.Sleep(50 * time.Millisecond)
	cancel()
	close(gate) // unblock any in-flight CacheImage calls

	stats := <-done
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if stats.Claimed == 0 {
		t.Fatal("expected some jobs claimed before cancel")
	}
	for id, status := range jobs.terminal {
		if status != "succeeded" && status != "failed" && status != "requeued" {
			t.Fatalf("job %d unexpected terminal %q", id, status)
		}
	}
	// Every claimed job must appear exactly once in terminal (succeeded/failed/requeued).
	if len(jobs.terminal) != stats.Claimed {
		t.Fatalf("terminal=%d claimed=%d requeued=%v terminal=%v", len(jobs.terminal), stats.Claimed, jobs.requeuedIDs, jobs.terminal)
	}
	seen := map[int64]int{}
	for id := range jobs.terminal {
		seen[id]++
		if seen[id] != 1 {
			t.Fatalf("job %d terminated more than once", id)
		}
	}
}

func intPointer(v int) *int {
	return &v
}
