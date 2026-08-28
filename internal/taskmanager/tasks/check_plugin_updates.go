package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/plugins"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

type PluginUpdateChecker interface {
	Check(ctx context.Context, opts plugins.AutoUpdateOptions) (plugins.AutoUpdateSummary, error)
}

type CheckPluginUpdatesTask struct {
	checker PluginUpdateChecker
}

func NewCheckPluginUpdatesTask(checker PluginUpdateChecker) *CheckPluginUpdatesTask {
	return &CheckPluginUpdatesTask{checker: checker}
}

func (t *CheckPluginUpdatesTask) Key() string  { return "check_plugin_updates" }
func (t *CheckPluginUpdatesTask) Name() string { return "Check Plugin Updates" }
func (t *CheckPluginUpdatesTask) Description() string {
	return "Fetches plugin repositories and applies or records available plugin updates"
}
func (t *CheckPluginUpdatesTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategorySystem
}
func (t *CheckPluginUpdatesTask) IsHidden() bool { return false }

func (t *CheckPluginUpdatesTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 24 * 60 * 60 * 1000},
	}
}

func (t *CheckPluginUpdatesTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Checking plugin repositories")

	summary, err := t.checker.Check(ctx, plugins.AutoUpdateOptions{
		SeedDefaultRepository: true,
		AutoInstallDefaults:   false,
	})
	if err != nil {
		return fmt.Errorf("check plugin updates: %w", err)
	}

	resultData, err := json.Marshal(summary)
	if err == nil {
		progress.SetResultData(resultData)
	}

	progress.Report(100, fmt.Sprintf(
		"Checked %d catalog entries, applied %d updates, recorded %d available updates",
		summary.CatalogEntries,
		summary.UpdatesApplied,
		summary.UpdatesAvailable,
	))
	return nil
}
