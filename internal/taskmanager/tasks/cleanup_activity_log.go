package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/activitylog"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// ActivityLogCleanupTask prunes expired activity log rows and partitions.
type ActivityLogCleanupTask struct {
	pool  *pgxpool.Pool
	store activitylog.SettingsStore
	pm    activitylog.PartitionManager
}

// NewActivityLogCleanupTask creates a scheduled task for activity log retention.
func NewActivityLogCleanupTask(
	pool *pgxpool.Pool,
	store activitylog.SettingsStore,
	pm activitylog.PartitionManager,
) *ActivityLogCleanupTask {
	return &ActivityLogCleanupTask{
		pool:  pool,
		store: store,
		pm:    pm,
	}
}

func (t *ActivityLogCleanupTask) Key() string  { return "cleanup_activity_log" }
func (t *ActivityLogCleanupTask) Name() string { return "Cleanup Activity Log" }
func (t *ActivityLogCleanupTask) Description() string {
	return "Prunes expired activity log rows and partitions"
}
func (t *ActivityLogCleanupTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategorySystem
}
func (t *ActivityLogCleanupTask) IsHidden() bool { return false }

func (t *ActivityLogCleanupTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeStartup},
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: int64((24 * time.Hour) / time.Millisecond)},
	}
}

func (t *ActivityLogCleanupTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Pruning activity logs")
	deleted := activitylog.CleanupOnce(ctx, t.pool, t.store, t.pm)
	progress.Report(100, fmt.Sprintf("Pruned %d activity log rows", deleted))
	return nil
}
