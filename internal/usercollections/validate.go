package usercollections

import (
	"fmt"
	"strings"
	"time"

	"github.com/prairie-server/prairie-server/internal/catalog"
)

// AllowedSyncSchedules maps a UI-friendly cadence label to the cron
// expression we persist. The set is fixed (no user-supplied cron) so we can
// guarantee the >= 24h minimum-interval cap without re-parsing the cron tree.
// All schedules fire at 04:30 UTC to spread load across the cluster.
var AllowedSyncSchedules = map[string]string{
	"daily":   "30 4 * * *",
	"weekly":  "30 4 * * 0",
	"monthly": "30 4 1 * *",
}

func ResolveSyncSchedule(label string) (*string, error) {
	label = strings.TrimSpace(strings.ToLower(label))
	if label == "" {
		return nil, nil
	}
	expr, ok := AllowedSyncSchedules[label]
	if !ok {
		return nil, fmt.Errorf("invalid sync_schedule %q: must be one of daily, weekly, monthly", label)
	}
	return &expr, nil
}

func InitialNextSyncAt(schedule *string) *time.Time {
	if schedule == nil || *schedule == "" {
		return nil
	}
	return catalog.ComputeNextSyncAtFrom(*schedule, time.Now())
}
