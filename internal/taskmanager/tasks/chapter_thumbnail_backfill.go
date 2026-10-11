package tasks

import (
	"context"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

type ChapterThumbnailBackfiller interface {
	BackfillMissing(ctx context.Context, limit int) (int, error)
}

type ChapterThumbnailBackfillTask struct {
	backfiller ChapterThumbnailBackfiller
	limit      int
}

func NewChapterThumbnailBackfillTask(backfiller ChapterThumbnailBackfiller, limit int) *ChapterThumbnailBackfillTask {
	return &ChapterThumbnailBackfillTask{backfiller: backfiller, limit: limit}
}

func (t *ChapterThumbnailBackfillTask) Key() string  { return "chapter_thumbnail_backfill" }
func (t *ChapterThumbnailBackfillTask) Name() string { return "Chapter Thumbnail Backfill" }
func (t *ChapterThumbnailBackfillTask) Description() string {
	return "Generates missing chapter thumbnails for opted-in libraries"
}
func (t *ChapterThumbnailBackfillTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *ChapterThumbnailBackfillTask) IsHidden() bool { return true }

func (t *ChapterThumbnailBackfillTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 6 * 60 * 60 * 1000},
	}
}

func (t *ChapterThumbnailBackfillTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Finding files missing chapter thumbnails")
	processed, err := t.backfiller.BackfillMissing(ctx, t.limit)
	if err != nil {
		return fmt.Errorf("chapter thumbnail backfill: %w", err)
	}
	progress.Report(100, fmt.Sprintf("Processed %d files", processed))
	return nil
}
