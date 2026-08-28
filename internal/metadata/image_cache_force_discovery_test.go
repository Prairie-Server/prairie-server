package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestImageCacheProcessorForceDiscoveryEnqueuesBeforeDrain(t *testing.T) {
	job := &models.MetadataImageCacheJob{
		ID:                1,
		TargetType:        ImageCacheTargetEpisode,
		TargetContentID:   "episode-1",
		SourcePath:        "tvdb://banners/episode-1.jpg",
		ProviderID:        "tvdb",
		ProviderContentID: "1",
		ContentType:       "series",
		ImageType:         ImageCacheImageStill,
		SeasonNumber:      intPointer(1),
		EpisodeNumber:     intPointer(1),
	}
	// Force discover must enqueue even while the queue is non-empty, so item
	// posters are not gated behind draining the entire episode backlog.
	jobs := &loopingImageCacheJobs{
		enqueueResults: []int{3},
		claimedResults: [][]*models.MetadataImageCacheJob{
			{job},
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
	processor.ForceDiscovery()
	stats, err := processor.RunUntilIdle(context.Background(), "test-worker", 1000, 2, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle() error = %v", err)
	}
	if stats.EnqueuedExisting < 3 {
		t.Fatalf("EnqueuedExisting = %d, want >= 3 from forced discover-at-start", stats.EnqueuedExisting)
	}
	if jobs.enqueueCalls < 1 {
		t.Fatal("expected EnqueueExistingProviderArtwork during forced discover")
	}
}
