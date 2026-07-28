package taskmanager

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// gaugeFor returns the sample of metric carrying task="taskKey", and whether
// such a series exists at all. Read by gathering rather than through
// GaugeVec.With, which would create the series a test may be asserting is
// absent.
func gaugeFor(t *testing.T, metric, taskKey string) (float64, bool) {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != metric {
			continue
		}
		for _, sample := range family.GetMetric() {
			for _, label := range sample.GetLabel() {
				if label.GetName() == "task" && label.GetValue() == taskKey {
					return sample.GetGauge().GetValue(), true
				}
			}
		}
	}
	return 0, false
}

func requireGauge(t *testing.T, metric, taskKey string, want float64) {
	t.Helper()
	got, ok := gaugeFor(t, metric, taskKey)
	if !ok {
		t.Fatalf("%s has no series for task %q", metric, taskKey)
	}
	if got != want {
		t.Errorf("%s{task=%q} = %v, want %v", metric, taskKey, got, want)
	}
}

func TestMetricsObserverPublishesSchedulingState(t *testing.T) {
	const key = "metrics_success"
	completed := time.Unix(1700000000, 0)
	next := time.Unix(1700000120, 0)

	NewMetricsObserver().TaskUpdated(TaskInfo{
		Key:       key,
		State:     TaskStateIdle,
		Triggers:  []TriggerConfig{{Type: TriggerTypeStartup}, {Type: TriggerTypeInterval, IntervalMs: 1000}},
		NextRunAt: &next,
		LastExecution: &ExecutionResult{
			TaskKey:     key,
			CompletedAt: completed,
			Status:      executionStatusCompleted,
			DurationMs:  2500,
		},
	})

	requireGauge(t, "streamapp_task_last_execution_timestamp_seconds", key, float64(completed.Unix()))
	requireGauge(t, "streamapp_task_last_success_timestamp_seconds", key, float64(completed.Unix()))
	requireGauge(t, "streamapp_task_last_execution_duration_seconds", key, 2.5)
	requireGauge(t, "streamapp_task_next_run_timestamp_seconds", key, float64(next.Unix()))
	requireGauge(t, "streamapp_task_triggers", key, 2)
	requireGauge(t, "streamapp_task_running", key, 0)
}

// A failed run still advances last-execution; only last-success must stay put,
// otherwise a task that fails every single time looks healthy.
func TestMetricsObserverFailureDoesNotAdvanceLastSuccess(t *testing.T) {
	const key = "metrics_failure"
	observer := NewMetricsObserver()
	success := time.Unix(1700000000, 0)
	failure := time.Unix(1700000600, 0)

	observer.TaskUpdated(TaskInfo{
		Key:           key,
		LastExecution: &ExecutionResult{CompletedAt: success, Status: executionStatusCompleted},
	})
	observer.TaskUpdated(TaskInfo{
		Key:           key,
		State:         TaskStateRunning,
		LastExecution: &ExecutionResult{CompletedAt: failure, Status: executionStatusFailed},
	})

	requireGauge(t, "streamapp_task_last_execution_timestamp_seconds", key, float64(failure.Unix()))
	requireGauge(t, "streamapp_task_last_success_timestamp_seconds", key, float64(success.Unix()))
	requireGauge(t, "streamapp_task_running", key, 1)
}

// A cancelled run is not a success either.
func TestMetricsObserverCancelledDoesNotCountAsSuccess(t *testing.T) {
	const key = "metrics_cancelled"
	cancelled := time.Unix(1700001000, 0)

	NewMetricsObserver().TaskUpdated(TaskInfo{
		Key:           key,
		LastExecution: &ExecutionResult{CompletedAt: cancelled, Status: executionStatusCancelled},
	})

	requireGauge(t, "streamapp_task_last_execution_timestamp_seconds", key, float64(cancelled.Unix()))
	if _, ok := gaugeFor(t, "streamapp_task_last_success_timestamp_seconds", key); ok {
		t.Error("a cancelled execution must not publish a last-success timestamp")
	}
}

// "No next run" is the signal that a task has gone unscheduled, so it must be
// an absent series rather than a timestamp of zero — zero would graph as 1970
// and read as "ran long ago" instead of "not scheduled at all".
func TestMetricsObserverDropsNextRunWhenUnscheduled(t *testing.T) {
	const key = "metrics_unscheduled"
	observer := NewMetricsObserver()
	next := time.Unix(1700000120, 0)

	observer.TaskUpdated(TaskInfo{Key: key, NextRunAt: &next})
	if _, ok := gaugeFor(t, "streamapp_task_next_run_timestamp_seconds", key); !ok {
		t.Fatal("next run series missing while scheduled")
	}

	observer.TaskUpdated(TaskInfo{Key: key, NextRunAt: nil})
	if _, ok := gaugeFor(t, "streamapp_task_next_run_timestamp_seconds", key); ok {
		t.Error("next run series survived after the task became unscheduled")
	}
	requireGauge(t, "streamapp_task_triggers", key, 0)
}

// A zero NextRunAt is as meaningless as a missing one.
func TestMetricsObserverTreatsZeroNextRunAsUnscheduled(t *testing.T) {
	const key = "metrics_zero_next_run"
	var zero time.Time

	NewMetricsObserver().TaskUpdated(TaskInfo{Key: key, NextRunAt: &zero})

	if _, ok := gaugeFor(t, "streamapp_task_next_run_timestamp_seconds", key); ok {
		t.Error("a zero next-run time must not be published as a timestamp")
	}
}

func TestMetricsObserverIgnoresEmptyKey(t *testing.T) {
	NewMetricsObserver().TaskUpdated(TaskInfo{Key: ""})
	if _, ok := gaugeFor(t, "streamapp_task_running", ""); ok {
		t.Error("an empty task key must not create a series")
	}
}

// A task that has never run reports its schedule without inventing a run.
func TestMetricsObserverHandlesTaskWithNoHistory(t *testing.T) {
	const key = "metrics_no_history"
	next := time.Unix(1700002000, 0)

	NewMetricsObserver().TaskUpdated(TaskInfo{
		Key:       key,
		Triggers:  []TriggerConfig{{Type: TriggerTypeInterval, IntervalMs: 1000}},
		NextRunAt: &next,
	})

	requireGauge(t, "streamapp_task_next_run_timestamp_seconds", key, float64(next.Unix()))
	requireGauge(t, "streamapp_task_triggers", key, 1)
	if _, ok := gaugeFor(t, "streamapp_task_last_execution_timestamp_seconds", key); ok {
		t.Error("a task with no history must not publish a last-execution timestamp")
	}
}
