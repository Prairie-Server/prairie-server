package tasks

import (
	"context"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// WatchProviderSyncer runs scheduled sync work for configured watch providers.
type WatchProviderSyncer interface {
	SyncDueConnections(ctx context.Context) error
}

// SyncWatchProvidersTask checks linked watch-provider accounts for due sync work.
type SyncWatchProvidersTask struct {
	syncer WatchProviderSyncer
}

// NewSyncWatchProvidersTask creates a task for scheduled watch-provider sync.
func NewSyncWatchProvidersTask(syncer WatchProviderSyncer) *SyncWatchProvidersTask {
	return &SyncWatchProvidersTask{syncer: syncer}
}

func (t *SyncWatchProvidersTask) Key() string  { return "sync_watch_providers" }
func (t *SyncWatchProvidersTask) Name() string { return "Sync Watch Providers" }
func (t *SyncWatchProvidersTask) Description() string {
	return "Checks linked watch providers for due automatic sync work"
}
func (t *SyncWatchProvidersTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *SyncWatchProvidersTask) IsHidden() bool { return false }

func (t *SyncWatchProvidersTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 60 * 60 * 1000},
	}
}

func (t *SyncWatchProvidersTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Checking for due watch provider connections")
	if t.syncer == nil {
		progress.Report(100, "Watch provider sync unavailable")
		return nil
	}
	if err := t.syncer.SyncDueConnections(ctx); err != nil {
		return fmt.Errorf("sync watch providers: %w", err)
	}
	progress.Report(100, "Watch provider sync check complete")
	return nil
}
