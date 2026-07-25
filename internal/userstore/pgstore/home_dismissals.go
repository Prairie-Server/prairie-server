package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

func (s *PostgresUserStore) ListHomeDismissals(ctx context.Context, profileID, surface string) ([]userstore.HomeItemDismissal, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT profile_id, surface, media_item_id, series_id, progress_updated_at, dismissed_at
		 FROM user_home_item_dismissals
		 WHERE user_id = $1 AND profile_id = $2 AND surface = $3
		 ORDER BY dismissed_at DESC`,
		s.userID, profileID, surface,
	)
	if err != nil {
		return nil, fmt.Errorf("listing home dismissals: %w", err)
	}
	defer rows.Close()

	var dismissals []userstore.HomeItemDismissal
	for rows.Next() {
		var dismissal userstore.HomeItemDismissal
		var progressUpdatedAt *time.Time
		var dismissedAt time.Time
		if err := rows.Scan(
			&dismissal.ProfileID,
			&dismissal.Surface,
			&dismissal.MediaItemID,
			&dismissal.SeriesID,
			&progressUpdatedAt,
			&dismissedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning home dismissal: %w", err)
		}
		if progressUpdatedAt != nil {
			value := timeToString(*progressUpdatedAt)
			dismissal.ProgressUpdatedAt = &value
		}
		dismissal.DismissedAt = timeToString(dismissedAt)
		dismissals = append(dismissals, dismissal)
	}
	return dismissals, rows.Err()
}

func (s *PostgresUserStore) UpsertHomeDismissal(ctx context.Context, dismissal userstore.HomeItemDismissal) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_home_item_dismissals (
			user_id, profile_id, surface, media_item_id, series_id, progress_updated_at, dismissed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(user_id, profile_id, surface, media_item_id) DO UPDATE SET
			series_id = excluded.series_id,
			progress_updated_at = excluded.progress_updated_at,
			dismissed_at = excluded.dismissed_at`,
		s.userID,
		dismissal.ProfileID,
		dismissal.Surface,
		dismissal.MediaItemID,
		dismissal.SeriesID,
		dismissal.ProgressUpdatedAt,
		dismissal.DismissedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting home dismissal: %w", err)
	}
	return nil
}

func (s *PostgresUserStore) DeleteHomeDismissal(ctx context.Context, profileID, surface, mediaItemID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_home_item_dismissals
		 WHERE user_id = $1 AND profile_id = $2 AND surface = $3 AND media_item_id = $4`,
		s.userID, profileID, surface, mediaItemID,
	)
	if err != nil {
		return fmt.Errorf("deleting home dismissal: %w", err)
	}
	return nil
}
