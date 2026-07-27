package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// PgExecutionRepository implements taskmanager.ExecutionRepository using PostgreSQL.
type PgExecutionRepository struct {
	pool *pgxpool.Pool
}

func NewPgExecutionRepository(pool *pgxpool.Pool) *PgExecutionRepository {
	return &PgExecutionRepository{pool: pool}
}

func (r *PgExecutionRepository) Insert(ctx context.Context, result taskmanager.ExecutionResult) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO task_executions (task_key, started_at, completed_at, status, error_message, result_data, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		result.TaskKey, result.StartedAt, result.CompletedAt,
		result.Status, result.ErrorMessage, result.ResultData, result.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("inserting task execution: %w", err)
	}
	return nil
}

func (r *PgExecutionRepository) GetLatest(ctx context.Context, taskKey string) (*taskmanager.ExecutionResult, error) {
	var result taskmanager.ExecutionResult
	err := r.pool.QueryRow(ctx, `
		SELECT id, task_key, started_at, completed_at, status, COALESCE(error_message, ''), result_data, duration_ms
		FROM task_executions
		WHERE task_key = $1
		ORDER BY completed_at DESC
		LIMIT 1`, taskKey,
	).Scan(
		&result.ID, &result.TaskKey, &result.StartedAt, &result.CompletedAt,
		&result.Status, &result.ErrorMessage, &result.ResultData, &result.DurationMs,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting latest task execution: %w", err)
	}
	return &result, nil
}

func (r *PgExecutionRepository) List(ctx context.Context, taskKey string, limit int) ([]taskmanager.ExecutionResult, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, task_key, started_at, completed_at, status, COALESCE(error_message, ''), result_data, duration_ms
		FROM task_executions
		WHERE task_key = $1
		ORDER BY completed_at DESC
		LIMIT $2`, taskKey, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing task executions: %w", err)
	}
	defer rows.Close()

	var results []taskmanager.ExecutionResult
	for rows.Next() {
		var r taskmanager.ExecutionResult
		if err := rows.Scan(
			&r.ID, &r.TaskKey, &r.StartedAt, &r.CompletedAt,
			&r.Status, &r.ErrorMessage, &r.ResultData, &r.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("scanning task execution: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
