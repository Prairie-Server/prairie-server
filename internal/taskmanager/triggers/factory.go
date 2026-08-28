package triggers

import "github.com/prairie-server/prairie-server/internal/taskmanager"

// New creates a live Trigger from a TriggerConfig.
func New(cfg taskmanager.TriggerConfig) taskmanager.Trigger {
	switch cfg.Type {
	case taskmanager.TriggerTypeInterval:
		return NewIntervalTrigger(cfg)
	case taskmanager.TriggerTypeDaily:
		return NewDailyTrigger(cfg)
	case taskmanager.TriggerTypeWeekly:
		return NewWeeklyTrigger(cfg)
	case taskmanager.TriggerTypeStartup:
		return NewStartupTrigger(cfg)
	default:
		return nil
	}
}
