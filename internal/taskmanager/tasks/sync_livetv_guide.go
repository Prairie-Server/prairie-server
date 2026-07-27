package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/livetv"
	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// SyncLiveTVGuideTask refreshes enabled Live TV guide sources on an interval.
type SyncLiveTVGuideTask struct {
	service *livetv.Service
}

func NewSyncLiveTVGuideTask(service *livetv.Service) *SyncLiveTVGuideTask {
	return &SyncLiveTVGuideTask{service: service}
}

func (t *SyncLiveTVGuideTask) Key() string  { return "sync_livetv_guide" }
func (t *SyncLiveTVGuideTask) Name() string { return "Sync Live TV Guide" }
func (t *SyncLiveTVGuideTask) Description() string {
	return "Syncs enabled Live TV guide sources (Schedules Direct / XML sync) and applies series recording rules"
}
func (t *SyncLiveTVGuideTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *SyncLiveTVGuideTask) IsHidden() bool { return false }

func (t *SyncLiveTVGuideTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 60 * 60 * 1000}, // hourly
	}
}

func (t *SyncLiveTVGuideTask) ShouldRun(ctx context.Context) (bool, error) {
	if t == nil || t.service == nil {
		return false, nil
	}
	sources, err := t.service.ListGuideSources(ctx)
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		if source.Enabled {
			return true, nil
		}
	}
	return false, nil
}

func (t *SyncLiveTVGuideTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Syncing Live TV guide sources")
	count, err := t.service.SyncAllEnabledGuideSources(ctx)
	reaped, reapErr := t.service.ReapExpiredArtwork(ctx)
	result, _ := json.Marshal(map[string]int{
		"sources_attempted": count,
		"artwork_reaped":    reaped,
	})
	progress.SetResultData(result)
	if err != nil {
		return fmt.Errorf("livetv guide sync: %w", err)
	}
	if reapErr != nil {
		return fmt.Errorf("livetv artwork reap: %w", reapErr)
	}
	progress.Report(100, fmt.Sprintf("Live TV guide sync complete (%d sources, reaped %d art)", count, reaped))
	return nil
}

// LiveTVDVRTickTask applies series rules and runs the FFmpeg DVR recorder
// against due / in-progress Live TV recordings.
type LiveTVDVRTickTask struct {
	service *livetv.Service
}

func NewLiveTVDVRTickTask(service *livetv.Service) *LiveTVDVRTickTask {
	return &LiveTVDVRTickTask{service: service}
}

func (t *LiveTVDVRTickTask) Key() string  { return "livetv_dvr_tick" }
func (t *LiveTVDVRTickTask) Name() string { return "Live TV DVR Tick" }
func (t *LiveTVDVRTickTask) Description() string {
	return "Applies Live TV series recording rules and starts/finishes due recordings"
}
func (t *LiveTVDVRTickTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *LiveTVDVRTickTask) IsHidden() bool { return false }

func (t *LiveTVDVRTickTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 60 * 1000}, // every minute
	}
}

func (t *LiveTVDVRTickTask) ShouldRun(_ context.Context) (bool, error) {
	return t != nil && t.service != nil, nil
}

func (t *LiveTVDVRTickTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Applying Live TV series rules")
	if err := t.service.ApplySeriesRules(ctx); err != nil {
		return fmt.Errorf("livetv apply series rules: %w", err)
	}
	progress.Report(40, "Processing Live TV recordings")
	started, completed, failed, err := t.service.ProcessRecordings(ctx)
	if err != nil {
		return fmt.Errorf("livetv process recordings: %w", err)
	}
	result, _ := json.Marshal(map[string]int{
		"started":   started,
		"completed": completed,
		"failed":    failed,
	})
	progress.SetResultData(result)
	progress.Report(100, fmt.Sprintf(
		"Live TV DVR tick complete (started=%d completed=%d failed=%d)",
		started, completed, failed,
	))
	return nil
}
