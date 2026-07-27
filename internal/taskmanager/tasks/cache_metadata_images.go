package tasks

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/prairie-server/prairie-server/internal/imageutil"
	"github.com/prairie-server/prairie-server/internal/metadata"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

const (
	cacheMetadataImagesIntervalMs = int64(60 * 1000)
	// Discovery enqueue batch stays large; claim size is capped to concurrency
	// inside the processor so DB running ≈ actually in-flight work.
	cacheMetadataImagesBatchSize  = 1000
	cacheMetadataImagesMaxRuntime = 10 * time.Minute
)

// cacheMetadataImagesConcurrency caps WebP cache workers to the shared encode
// budget size (≈ NumCPU) so WebP + AVIF in-flight work does not oversubscribe.
func cacheMetadataImagesConcurrency() int {
	n := imageutil.EncodeBudgetSize()
	if n < 1 {
		n = runtime.NumCPU()
	}
	if n < 1 {
		return 1
	}
	return n
}

type MetadataImageCacheRunner interface {
	RunUntilIdle(ctx context.Context, workerID string, claimLimit int, concurrency int, maxRuntime time.Duration, onProgress metadata.ImageCacheProgressFunc) (metadata.ImageCacheRunStats, error)
	ForceDiscovery()
}

type CacheMetadataImagesTask struct {
	runner MetadataImageCacheRunner
}

func NewCacheMetadataImagesTask(runner MetadataImageCacheRunner) *CacheMetadataImagesTask {
	return &CacheMetadataImagesTask{runner: runner}
}

func (t *CacheMetadataImagesTask) Key() string  { return "cache_metadata_images" }
func (t *CacheMetadataImagesTask) Name() string { return "Cache Metadata Images" }
func (t *CacheMetadataImagesTask) Description() string {
	return "Caches provider metadata artwork into object storage"
}
func (t *CacheMetadataImagesTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryMetadata
}
func (t *CacheMetadataImagesTask) IsHidden() bool { return false }

func (t *CacheMetadataImagesTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeStartup},
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: cacheMetadataImagesIntervalMs},
	}
}

func (t *CacheMetadataImagesTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	if t.runner == nil {
		progress.Report(100, "Metadata image cache is not configured")
		return nil
	}
	// Manual runs and interval ticks both force a discovery pass so item
	// posters are enqueued even while an episode backlog occupies the queue.
	t.runner.ForceDiscovery()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "silo"
	}
	stats, err := t.runner.RunUntilIdle(
		ctx,
		hostname,
		cacheMetadataImagesBatchSize,
		cacheMetadataImagesConcurrency(),
		cacheMetadataImagesMaxRuntime,
		progress.Report,
	)
	if err != nil {
		return fmt.Errorf("caching metadata images: %w", err)
	}
	message := fmt.Sprintf(
		"Batches %d, enqueued %d existing, claimed %d, cached %d, failed %d, skipped %d, uploaded %d variants, found %d existing variants, deleted %d old successes",
		stats.Batches,
		stats.EnqueuedExisting,
		stats.Claimed,
		stats.Succeeded,
		stats.Failed,
		stats.Skipped,
		stats.UploadedVariants,
		stats.ExistingVariants,
		stats.DeletedSucceeded,
	)
	if stats.RuntimeLimited {
		message += ", runtime budget reached"
	}
	progress.Report(100, message)
	return nil
}
