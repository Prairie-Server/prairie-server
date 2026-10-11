package scanner

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/models"
)

// MediaGroupOverrideRepository persists operator-provided logical group overrides.
type MediaGroupOverrideRepository struct {
	pool *pgxpool.Pool
}

func NewMediaGroupOverrideRepository(pool *pgxpool.Pool) *MediaGroupOverrideRepository {
	return &MediaGroupOverrideRepository{pool: pool}
}

const mediaGroupOverrideColumns = `media_folder_id, group_key_version, content_group_key,
	forced_type, forced_title, forced_year, forced_tmdb_id, forced_imdb_id, forced_tvdb_id,
	note, created_by_user_id, updated_by_user_id, created_at, updated_at`

func scanMediaGroupOverride(row pgx.Row) (*models.MediaGroupOverride, error) {
	var override models.MediaGroupOverride
	if err := row.Scan(
		&override.MediaFolderID,
		&override.GroupKeyVersion,
		&override.ContentGroupKey,
		&override.ForcedType,
		&override.ForcedTitle,
		&override.ForcedYear,
		&override.ForcedTmdbID,
		&override.ForcedImdbID,
		&override.ForcedTvdbID,
		&override.Note,
		&override.CreatedByUserID,
		&override.UpdatedByUserID,
		&override.CreatedAt,
		&override.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scanning media group override: %w", err)
	}
	return &override, nil
}

func (r *MediaGroupOverrideRepository) Get(
	ctx context.Context,
	folderID int,
	groupKeyVersion int,
	contentGroupKey string,
) (*models.MediaGroupOverride, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+mediaGroupOverrideColumns+`
		FROM media_group_overrides
		WHERE media_folder_id = $1 AND group_key_version = $2 AND content_group_key = $3
	`, folderID, groupKeyVersion, contentGroupKey)
	override, err := scanMediaGroupOverride(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return override, nil
}

func (r *MediaGroupOverrideRepository) ListByFolder(
	ctx context.Context,
	folderID int,
) ([]models.MediaGroupOverride, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+mediaGroupOverrideColumns+`
		FROM media_group_overrides
		WHERE media_folder_id = $1
		ORDER BY group_key_version ASC, content_group_key ASC
	`, folderID)
	if err != nil {
		return nil, fmt.Errorf("listing media group overrides: %w", err)
	}
	defer rows.Close()

	overrides := make([]models.MediaGroupOverride, 0)
	for rows.Next() {
		override, err := scanMediaGroupOverride(rows)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, *override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating media group overrides: %w", err)
	}
	return overrides, nil
}

func (r *MediaGroupOverrideRepository) Upsert(
	ctx context.Context,
	override models.MediaGroupOverride,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO media_group_overrides (
			media_folder_id, group_key_version, content_group_key,
			forced_type, forced_title, forced_year, forced_tmdb_id, forced_imdb_id, forced_tvdb_id,
			note, created_by_user_id, updated_by_user_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (media_folder_id, group_key_version, content_group_key) DO UPDATE SET
			forced_type = EXCLUDED.forced_type,
			forced_title = EXCLUDED.forced_title,
			forced_year = EXCLUDED.forced_year,
			forced_tmdb_id = EXCLUDED.forced_tmdb_id,
			forced_imdb_id = EXCLUDED.forced_imdb_id,
			forced_tvdb_id = EXCLUDED.forced_tvdb_id,
			note = EXCLUDED.note,
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			updated_at = NOW()
	`,
		override.MediaFolderID,
		override.GroupKeyVersion,
		override.ContentGroupKey,
		override.ForcedType,
		override.ForcedTitle,
		override.ForcedYear,
		override.ForcedTmdbID,
		override.ForcedImdbID,
		override.ForcedTvdbID,
		override.Note,
		override.CreatedByUserID,
		override.UpdatedByUserID,
	)
	if err != nil {
		return fmt.Errorf("upserting media group override: %w", err)
	}
	return nil
}

func (r *MediaGroupOverrideRepository) Delete(
	ctx context.Context,
	folderID int,
	groupKeyVersion int,
	contentGroupKey string,
) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM media_group_overrides
		WHERE media_folder_id = $1 AND group_key_version = $2 AND content_group_key = $3
	`, folderID, groupKeyVersion, contentGroupKey)
	if err != nil {
		return fmt.Errorf("deleting media group override: %w", err)
	}
	return nil
}
