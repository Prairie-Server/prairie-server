package tasks

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prairie-server/prairie-server/internal/metadata"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

const (
	backfillAVIFSiblingsIntervalMs = int64(2 * 60 * 1000)
	backfillAVIFSiblingsMaxRuntime = 8 * time.Minute
)

type AVIFBackfillRunner interface {
	RunUntilIdle(ctx context.Context, concurrency int, maxRuntime time.Duration, onProgress func(float64, string)) (metadata.AVIFBackfillStats, error)
	Workers() int
}

type BackfillAVIFSiblingsTask struct {
	runner AVIFBackfillRunner
	// running closes the startup+interval overlap gap: taskmanager already
	// returns ErrTaskAlreadyRunning, but a second Execute entry (manual run
	// racing a scheduled fire across re-register) must no-op instead of
	// stacking another NumCPU worker set on a 4-core box.
	running atomic.Bool
}

func NewBackfillAVIFSiblingsTask(runner AVIFBackfillRunner) *BackfillAVIFSiblingsTask {
	return &BackfillAVIFSiblingsTask{runner: runner}
}

func (t *BackfillAVIFSiblingsTask) Key() string  { return "backfill_avif_siblings" }
func (t *BackfillAVIFSiblingsTask) Name() string { return "Backfill AVIF Siblings" }
func (t *BackfillAVIFSiblingsTask) Description() string {
	return "Generates missing AVIF siblings for cached WebP artwork and drains the durable AVIF backfill queue"
}
func (t *BackfillAVIFSiblingsTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryMetadata
}
func (t *BackfillAVIFSiblingsTask) IsHidden() bool { return false }

func (t *BackfillAVIFSiblingsTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeStartup},
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: backfillAVIFSiblingsIntervalMs},
	}
}

func (t *BackfillAVIFSiblingsTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	if t == nil || t.runner == nil {
		progress.Report(100, "AVIF backfill is not configured")
		return nil
	}
	if !t.running.CompareAndSwap(false, true) {
		progress.Report(100, "AVIF backfill already running; skipped overlapping trigger")
		return nil
	}
	defer t.running.Store(false)

	workers := t.runner.Workers()
	stats, err := t.runner.RunUntilIdle(ctx, workers, backfillAVIFSiblingsMaxRuntime, progress.Report)
	if err != nil {
		return fmt.Errorf("backfilling AVIF siblings: %w", err)
	}
	message := fmt.Sprintf(
		"Enqueued %d missing, claimed %d, succeeded %d, failed %d, deleted %d old successes",
		stats.EnqueuedExisting, stats.Claimed, stats.Succeeded, stats.Failed, stats.DeletedSucceeded,
	)
	if stats.PNGDeleted > 0 || stats.PNGChecked > 0 {
		message += fmt.Sprintf(", legacy PNG checked %d deleted %d", stats.PNGChecked, stats.PNGDeleted)
	}
	if stats.RetiredDeleted > 0 || stats.RetiredChecked > 0 {
		message += fmt.Sprintf(", retired rungs checked %d deleted %d", stats.RetiredChecked, stats.RetiredDeleted)
	}
	if stats.RuntimeLimited {
		message += ", runtime budget reached"
	}
	if stats.PausedForPlayback {
		message += ", paused while playback was active"
	}
	progress.Report(100, message)
	return nil
}
