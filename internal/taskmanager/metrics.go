package taskmanager

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Scheduler state was previously observable only by querying task_executions
// directly: nothing was logged when a task fired, and no metric said when a
// task last ran or when it will run next. Answering "is this task still
// scheduled?" meant opening a psql session against the production database,
// which is exactly the question you have when a background queue looks stalled.
var (
	taskLastExecutionAt = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_task_last_execution_timestamp_seconds",
		Help: "Unix timestamp of the last completed execution of a scheduled task, regardless of outcome.",
	}, []string{"task"})
	taskLastSuccessAt = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_task_last_success_timestamp_seconds",
		Help: "Unix timestamp of the last successful execution of a scheduled task.",
	}, []string{"task"})
	taskLastDuration = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_task_last_execution_duration_seconds",
		Help: "Wall-clock duration of the last completed execution of a scheduled task.",
	}, []string{"task"})
	taskNextRunAt = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_task_next_run_timestamp_seconds",
		Help: "Unix timestamp of the earliest scheduled next run of a task. Absent when the task has no live trigger.",
	}, []string{"task"})
	taskRunning = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_task_running",
		Help: "1 while a scheduled task is executing, 0 otherwise.",
	}, []string{"task"})
	taskTriggers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_task_triggers",
		Help: "Number of live triggers arming a task. Zero means the task will never run on a schedule.",
	}, []string{"task"})
)

// MetricsObserver publishes scheduling state to Prometheus as tasks change.
//
// Deliberately a gauge set rather than counters: the useful alert is staleness
// ("this task has not succeeded in an hour") and absence ("this task has no
// trigger"), both of which need a timestamp, not a rate.
type MetricsObserver struct{}

func NewMetricsObserver() *MetricsObserver { return &MetricsObserver{} }

func (o *MetricsObserver) TaskUpdated(info TaskInfo) {
	if o == nil || info.Key == "" {
		return
	}
	labels := prometheus.Labels{"task": info.Key}

	running := 0.0
	if info.State == TaskStateRunning || info.State == TaskStateCancelling {
		running = 1
	}
	taskRunning.With(labels).Set(running)
	taskTriggers.With(labels).Set(float64(len(info.Triggers)))

	// A missing next run is meaningfully different from "next run at epoch
	// zero", so drop the series rather than reporting a misleading timestamp.
	if info.NextRunAt != nil && !info.NextRunAt.IsZero() {
		taskNextRunAt.With(labels).Set(float64(info.NextRunAt.Unix()))
	} else {
		taskNextRunAt.Delete(labels)
	}

	last := info.LastExecution
	if last == nil {
		return
	}
	if !last.CompletedAt.IsZero() {
		taskLastExecutionAt.With(labels).Set(float64(last.CompletedAt.Unix()))
		if last.Status == executionStatusCompleted {
			taskLastSuccessAt.With(labels).Set(float64(last.CompletedAt.Unix()))
		}
	}
	taskLastDuration.With(labels).Set(float64(last.DurationMs) / 1000)
}
