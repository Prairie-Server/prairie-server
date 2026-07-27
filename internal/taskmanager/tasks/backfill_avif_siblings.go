package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/prairie-server/prairie-server/internal/metadata"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

const (
	backfillAVIFSiblingsIntervalMs = int64(2 * 60 * 1000)
	backfillAVIFSiblingsWorkers    = 2
	backfillAVIFSiblingsMaxRuntime = 8 * time.Minute
)

type AVIFBackfillRunner interface {
	RunUntilIdle(ctx context.Context, concurrency int, maxRuntime time.Duration, onProgress func(float64, string)) (metadata.AVIFBackfillStats, error)
}

type BackfillAVIFSiblingsTask struct {
	runner AVIFBackfillRunner
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
	stats, err := t.runner.RunUntilIdle(ctx, backfillAVIFSiblingsWorkers, backfillAVIFSiblingsMaxRuntime, progress.Report)
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
	if stats.RuntimeLimited {
		message += ", runtime budget reached"
	}
	progress.Report(100, message)
	return nil
}
