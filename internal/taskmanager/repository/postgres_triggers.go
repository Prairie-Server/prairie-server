package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// PgTriggerRepository implements taskmanager.TriggerRepository using PostgreSQL.
type PgTriggerRepository struct {
	pool *pgxpool.Pool
}

func NewPgTriggerRepository(pool *pgxpool.Pool) *PgTriggerRepository {
	return &PgTriggerRepository{pool: pool}
}

func (r *PgTriggerRepository) GetTriggers(ctx context.Context, taskKey string) ([]taskmanager.TriggerConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT type, interval, time_of_day, day_of_week, max_runtime
		FROM task_triggers
		WHERE task_key = $1
		ORDER BY id`, taskKey,
	)
	if err != nil {
		return nil, fmt.Errorf("getting task triggers: %w", err)
	}
	defer rows.Close()

	var configs []taskmanager.TriggerConfig
	for rows.Next() {
		var (
			cfg       taskmanager.TriggerConfig
			trigType  string
			interval  *int64
			timeOfDay *string
			dayOfWeek *int
			maxRT     *int64
		)
		if err := rows.Scan(&trigType, &interval, &timeOfDay, &dayOfWeek, &maxRT); err != nil {
			return nil, fmt.Errorf("scanning task trigger: %w", err)
		}
		cfg.Type = taskmanager.TriggerType(trigType)
		if interval != nil {
			cfg.IntervalMs = *interval
		}
		if timeOfDay != nil {
			cfg.TimeOfDay = *timeOfDay
		}
		if dayOfWeek != nil {
			cfg.DayOfWeek = *dayOfWeek
		}
		if maxRT != nil {
			cfg.MaxRuntimeMs = *maxRT
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

func (r *PgTriggerRepository) SetTriggers(ctx context.Context, taskKey string, triggers []taskmanager.TriggerConfig) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM task_triggers WHERE task_key = $1`, taskKey); err != nil {
		return fmt.Errorf("deleting old triggers: %w", err)
	}

	for _, cfg := range triggers {
		var interval *int64
		if cfg.IntervalMs > 0 {
			interval = &cfg.IntervalMs
		}
		var timeOfDay *string
		if cfg.TimeOfDay != "" {
			timeOfDay = &cfg.TimeOfDay
		}
		var dayOfWeek *int
		if cfg.Type == taskmanager.TriggerTypeWeekly {
			dayOfWeek = &cfg.DayOfWeek
		}
		var maxRT *int64
		if cfg.MaxRuntimeMs > 0 {
			maxRT = &cfg.MaxRuntimeMs
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO task_triggers (task_key, type, interval, time_of_day, day_of_week, max_runtime)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			taskKey, string(cfg.Type), interval, timeOfDay, dayOfWeek, maxRT,
		); err != nil {
			return fmt.Errorf("inserting trigger: %w", err)
		}
	}

	return tx.Commit(ctx)
}
