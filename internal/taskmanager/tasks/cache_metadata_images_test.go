package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/metadata"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

type fakeMetadataImageCacheRunner struct {
	stats       metadata.ImageCacheRunStats
	err         error
	claimLimit  int
	concurrency int
	maxRuntime  time.Duration
	progressed  bool
}

func (f *fakeMetadataImageCacheRunner) RunUntilIdle(_ context.Context, _ string, claimLimit int, concurrency int, maxRuntime time.Duration, onProgress metadata.ImageCacheProgressFunc) (metadata.ImageCacheRunStats, error) {
	f.claimLimit = claimLimit
	f.concurrency = concurrency
	f.maxRuntime = maxRuntime
	if onProgress != nil {
		f.progressed = true
		onProgress(12.5, "Cached 1/8 (queued 5, running 2, failed 0)")
	}
	return f.stats, f.err
}

func (f *fakeMetadataImageCacheRunner) ForceDiscovery() {}

type recordingProgress struct {
	percents []float64
	messages []string
}

func (r *recordingProgress) Report(percent float64, message string) {
	r.percents = append(r.percents, percent)
	r.messages = append(r.messages, message)
}

func (r *recordingProgress) SetResultData(json.RawMessage) {}

func TestCacheMetadataImagesTaskProperties(t *testing.T) {
	task := NewCacheMetadataImagesTask(&fakeMetadataImageCacheRunner{})
	if task.Key() != "cache_metadata_images" {
		t.Fatalf("Key() = %q", task.Key())
	}
	if task.Category() != taskmanager.TaskCategoryMetadata {
		t.Fatalf("Category() = %q", task.Category())
	}
	if len(task.DefaultTriggers()) != 2 {
		t.Fatalf("DefaultTriggers count = %d, want 2", len(task.DefaultTriggers()))
	}
}

func TestCacheMetadataImagesTaskReportsStats(t *testing.T) {
	runner := &fakeMetadataImageCacheRunner{
		stats: metadata.ImageCacheRunStats{
			Batches:          3,
			EnqueuedExisting: 5,
			Claimed:          4,
			Succeeded:        3,
			Failed:           1,
			UploadedVariants: 7,
			ExistingVariants: 2,
		},
	}
	task := NewCacheMetadataImagesTask(runner)
	progress := &recordingProgress{}
	if err := task.Execute(context.Background(), progress); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.claimLimit != 1000 {
		t.Fatalf("claimLimit = %d, want 1000", runner.claimLimit)
	}
	if runner.concurrency != cacheMetadataImagesConcurrency() {
		t.Fatalf("concurrency = %d, want %d (encode budget / NumCPU)", runner.concurrency, cacheMetadataImagesConcurrency())
	}
	if runner.maxRuntime != 10*time.Minute {
		t.Fatalf("maxRuntime = %s, want 10m", runner.maxRuntime)
	}
	if !runner.progressed {
		t.Fatal("expected RunUntilIdle to receive a progress callback")
	}
	if len(progress.messages) < 2 {
		t.Fatalf("progress reports = %d, want at least mid-run + final", len(progress.messages))
	}
	if progress.messages[0] != "Cached 1/8 (queued 5, running 2, failed 0)" {
		t.Fatalf("mid-run progress = %q", progress.messages[0])
	}
	wantFinal := "Batches 3, enqueued 5 existing, claimed 4, cached 3, failed 1, skipped 0, uploaded 7 variants, found 2 existing variants, deleted 0 old successes"
	if progress.messages[len(progress.messages)-1] != wantFinal {
		t.Fatalf("final progress message = %q", progress.messages[len(progress.messages)-1])
	}
	if progress.percents[len(progress.percents)-1] != 100 {
		t.Fatalf("final percent = %v, want 100", progress.percents[len(progress.percents)-1])
	}
}

// Artwork caching keeps running while someone streams — users are waiting on
// those posters — but it drops to a single encode slot so playback keeps cores.
func TestCacheMetadataImagesTaskThrottlesDuringPlayback(t *testing.T) {
	runner := &fakeMetadataImageCacheRunner{}
	task := NewCacheMetadataImagesTask(runner)
	task.SetPlaybackActivityCheck(func() bool { return true })

	if err := task.Execute(context.Background(), &recordingProgress{}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.concurrency != 1 {
		t.Fatalf("concurrency = %d, want 1 while playback is active", runner.concurrency)
	}
}

func TestCacheMetadataImagesTaskUsesFullBudgetWhenIdle(t *testing.T) {
	runner := &fakeMetadataImageCacheRunner{}
	task := NewCacheMetadataImagesTask(runner)
	task.SetPlaybackActivityCheck(func() bool { return false })

	if err := task.Execute(context.Background(), &recordingProgress{}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := cacheMetadataImagesConcurrency(); runner.concurrency != want {
		t.Fatalf("concurrency = %d, want %d", runner.concurrency, want)
	}
}
