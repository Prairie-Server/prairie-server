package scanner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/models"
)

// MediaRootOverrideRepository persists operator-provided root overrides.
type MediaRootOverrideRepository struct {
	pool *pgxpool.Pool
}

func NewMediaRootOverrideRepository(pool *pgxpool.Pool) *MediaRootOverrideRepository {
	return &MediaRootOverrideRepository{pool: pool}
}

const mediaRootOverrideColumns = `media_folder_id, root_path, forced_type, forced_title, forced_year,
	forced_tmdb_id, forced_imdb_id, forced_tvdb_id, note, created_by_user_id,
	updated_by_user_id, created_at, updated_at`

func scanMediaRootOverride(row pgx.Row) (*models.MediaRootOverride, error) {
	var override models.MediaRootOverride
	if err := row.Scan(
		&override.MediaFolderID,
		&override.RootPath,
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
		return nil, fmt.Errorf("scanning media root override: %w", err)
	}
	return &override, nil
}

func (r *MediaRootOverrideRepository) Get(
	ctx context.Context,
	folderID int,
	rootPath string,
) (*models.MediaRootOverride, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+mediaRootOverrideColumns+`
		FROM media_root_overrides
		WHERE media_folder_id = $1 AND root_path = $2
	`, folderID, filepath.Clean(rootPath))
	override, err := scanMediaRootOverride(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return override, nil
}

func (r *MediaRootOverrideRepository) ListByFolder(
	ctx context.Context,
	folderID int,
) ([]models.MediaRootOverride, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+mediaRootOverrideColumns+`
		FROM media_root_overrides
		WHERE media_folder_id = $1
		ORDER BY root_path ASC
	`, folderID)
	if err != nil {
		return nil, fmt.Errorf("listing media root overrides: %w", err)
	}
	defer rows.Close()

	overrides := make([]models.MediaRootOverride, 0)
	for rows.Next() {
		override, err := scanMediaRootOverride(rows)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, *override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating media root overrides: %w", err)
	}
	return overrides, nil
}

func (r *MediaRootOverrideRepository) Upsert(
	ctx context.Context,
	override models.MediaRootOverride,
) error {
	override.RootPath = filepath.Clean(override.RootPath)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO media_root_overrides (
			media_folder_id, root_path, forced_type, forced_title, forced_year,
			forced_tmdb_id, forced_imdb_id, forced_tvdb_id, note,
			created_by_user_id, updated_by_user_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (media_folder_id, root_path) DO UPDATE SET
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
		override.RootPath,
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
		return fmt.Errorf("upserting media root override: %w", err)
	}
	return nil
}

func (r *MediaRootOverrideRepository) Delete(
	ctx context.Context,
	folderID int,
	rootPath string,
) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM media_root_overrides
		WHERE media_folder_id = $1 AND root_path = $2
	`, folderID, filepath.Clean(rootPath))
	if err != nil {
		return fmt.Errorf("deleting media root override: %w", err)
	}
	return nil
}
