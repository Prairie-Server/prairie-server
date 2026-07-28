package taskmanager

import (
	"context"
	"encoding/json"
	"time"
)

// Terminal statuses written to ExecutionResult.Status. Kept as untyped string
// constants because the column and the JSON field are plain strings.
const (
	executionStatusCompleted = "completed"
	executionStatusFailed    = "failed"
	executionStatusCancelled = "cancelled"
)

// ExecutionResult records the outcome of a single task execution.
type ExecutionResult struct {
	ID           int64           `json:"id"`
	TaskKey      string          `json:"task_key"`
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  time.Time       `json:"completed_at"`
	Status       string          `json:"status"` // "completed", "failed", "cancelled"
	ErrorMessage string          `json:"error_message,omitempty"`
	ResultData   json.RawMessage `json:"result_data,omitempty"`
	DurationMs   int64           `json:"duration_ms"`
}

// ExecutionRepository persists task execution history.
type ExecutionRepository interface {
	Insert(ctx context.Context, result ExecutionResult) error
	GetLatest(ctx context.Context, taskKey string) (*ExecutionResult, error)
	List(ctx context.Context, taskKey string, limit int) ([]ExecutionResult, error)
}
