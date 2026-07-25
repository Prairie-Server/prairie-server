package userdb

import (
	"database/sql"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

type HomeItemDismissal = userstore.HomeItemDismissal

func ListHomeDismissals(db *sql.DB, profileID, surface string) ([]HomeItemDismissal, error) {
	rows, err := db.Query(
		`SELECT profile_id, surface, media_item_id, series_id, progress_updated_at, dismissed_at
		 FROM home_item_dismissals
		 WHERE profile_id = ? AND surface = ?
		 ORDER BY dismissed_at DESC`,
		profileID, surface,
	)
	if err != nil {
		return nil, fmt.Errorf("listing home dismissals: %w", err)
	}
	defer rows.Close()

	var dismissals []HomeItemDismissal
	for rows.Next() {
		var dismissal HomeItemDismissal
		if err := rows.Scan(
			&dismissal.ProfileID,
			&dismissal.Surface,
			&dismissal.MediaItemID,
			&dismissal.SeriesID,
			&dismissal.ProgressUpdatedAt,
			&dismissal.DismissedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning home dismissal: %w", err)
		}
		dismissals = append(dismissals, dismissal)
	}
	return dismissals, rows.Err()
}

func UpsertHomeDismissal(db *sql.DB, dismissal HomeItemDismissal) error {
	_, err := db.Exec(
		`INSERT INTO home_item_dismissals (
			profile_id, surface, media_item_id, series_id, progress_updated_at, dismissed_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, surface, media_item_id) DO UPDATE SET
			series_id = excluded.series_id,
			progress_updated_at = excluded.progress_updated_at,
			dismissed_at = excluded.dismissed_at`,
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

func DeleteHomeDismissal(db *sql.DB, profileID, surface, mediaItemID string) error {
	_, err := db.Exec(
		`DELETE FROM home_item_dismissals WHERE profile_id = ? AND surface = ? AND media_item_id = ?`,
		profileID,
		surface,
		mediaItemID,
	)
	if err != nil {
		return fmt.Errorf("deleting home dismissal: %w", err)
	}
	return nil
}
