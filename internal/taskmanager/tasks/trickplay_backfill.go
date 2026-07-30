package tasks

import (
	"context"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

type TrickplayBackfiller interface {
	BackfillMissing(ctx context.Context, limit int) (int, error)
}

type TrickplayBackfillTask struct {
	backfiller TrickplayBackfiller
	limit      int
}

func NewTrickplayBackfillTask(backfiller TrickplayBackfiller, limit int) *TrickplayBackfillTask {
	return &TrickplayBackfillTask{backfiller: backfiller, limit: limit}
}

func (t *TrickplayBackfillTask) Key() string  { return "trickplay_backfill" }
func (t *TrickplayBackfillTask) Name() string { return "Trickplay Backfill" }
func (t *TrickplayBackfillTask) Description() string {
	return "Generates missing seek-preview sprite sheets for opted-in libraries"
}
func (t *TrickplayBackfillTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *TrickplayBackfillTask) IsHidden() bool { return true }

func (t *TrickplayBackfillTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 6 * 60 * 60 * 1000},
	}
}

func (t *TrickplayBackfillTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Finding files missing trickplay sheets")
	processed, err := t.backfiller.BackfillMissing(ctx, t.limit)
	if err != nil {
		return fmt.Errorf("trickplay backfill: %w", err)
	}
	progress.Report(100, fmt.Sprintf("Processed %d files", processed))
	return nil
}
