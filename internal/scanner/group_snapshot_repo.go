package scanner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/models"
)

type ScannedGroupRepository struct {
	pool *pgxpool.Pool
}

func NewScannedGroupRepository(pool *pgxpool.Pool) *ScannedGroupRepository {
	return &ScannedGroupRepository{pool: pool}
}

const scannedGroupColumns = `media_folder_id, group_key_version, content_group_key, state, inferred_type,
	type_confidence, base_title, base_year, tmdb_id, imdb_id, tvdb_id, observed_file_count,
	sample_file_path, sample_observed_root_path, evidence_json, override_source, first_seen_at, last_seen_at`

func scanScannedGroup(row pgx.Row) (*models.ScannedMediaGroup, error) {
	var group models.ScannedMediaGroup
	if err := row.Scan(
		&group.MediaFolderID,
		&group.GroupKeyVersion,
		&group.ContentGroupKey,
		&group.State,
		&group.InferredType,
		&group.TypeConfidence,
		&group.BaseTitle,
		&group.BaseYear,
		&group.TmdbID,
		&group.ImdbID,
		&group.TvdbID,
		&group.ObservedFileCount,
		&group.SampleFilePath,
		&group.SampleObservedRootPath,
		&group.EvidenceJSON,
		&group.OverrideSource,
		&group.FirstSeenAt,
		&group.LastSeenAt,
	); err != nil {
		return nil, fmt.Errorf("scanning scanned media group: %w", err)
	}
	return &group, nil
}

func (r *ScannedGroupRepository) Get(
	ctx context.Context,
	folderID int,
	groupKeyVersion int,
	contentGroupKey string,
) (*models.ScannedMediaGroup, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+scannedGroupColumns+`
		FROM scanned_media_groups
		WHERE media_folder_id = $1 AND group_key_version = $2 AND content_group_key = $3
	`, folderID, groupKeyVersion, contentGroupKey)
	group, err := scanScannedGroup(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return group, nil
}

func (r *ScannedGroupRepository) ListByFolder(
	ctx context.Context,
	folderID int,
	state string,
	limit,
	offset int,
) ([]models.ScannedMediaGroup, int, error) {
	filterState := strings.TrimSpace(state)

	countQuery := `SELECT COUNT(*) FROM scanned_media_groups WHERE media_folder_id = $1`
	countArgs := []any{folderID}
	if filterState != "" {
		countQuery += ` AND state = $2`
		countArgs = append(countArgs, filterState)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting scanned media groups: %w", err)
	}

	listQuery := `
		SELECT ` + scannedGroupColumns + `
		FROM scanned_media_groups
		WHERE media_folder_id = $1`
	listArgs := []any{folderID}
	argPos := 2
	if filterState != "" {
		listQuery += fmt.Sprintf(` AND state = $%d`, argPos)
		listArgs = append(listArgs, filterState)
		argPos++
	}
	listQuery += fmt.Sprintf(` ORDER BY last_seen_at DESC, sample_observed_root_path ASC, content_group_key ASC LIMIT $%d OFFSET $%d`, argPos, argPos+1)
	listArgs = append(listArgs, limit, offset)

	rows, err := r.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing scanned media groups: %w", err)
	}
	defer rows.Close()

	groups := make([]models.ScannedMediaGroup, 0)
	for rows.Next() {
		group, err := scanScannedGroup(rows)
		if err != nil {
			return nil, 0, err
		}
		groups = append(groups, *group)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating scanned media groups: %w", err)
	}
	return groups, total, nil
}

func (r *ScannedGroupRepository) UpsertMany(ctx context.Context, groups []models.ScannedMediaGroup) error {
	if len(groups) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, group := range groups {
		batch.Queue(`
			INSERT INTO scanned_media_groups (
				media_folder_id, group_key_version, content_group_key, state, inferred_type,
				type_confidence, base_title, base_year, tmdb_id, imdb_id, tvdb_id, observed_file_count,
				sample_file_path, sample_observed_root_path, evidence_json, override_source, first_seen_at, last_seen_at
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10, $11, $12,
				$13, $14, COALESCE($15, '{}'::jsonb), $16, NOW(), NOW()
			)
			ON CONFLICT (media_folder_id, group_key_version, content_group_key) DO UPDATE SET
				state = EXCLUDED.state,
				inferred_type = EXCLUDED.inferred_type,
				type_confidence = EXCLUDED.type_confidence,
				base_title = EXCLUDED.base_title,
				base_year = EXCLUDED.base_year,
				tmdb_id = EXCLUDED.tmdb_id,
				imdb_id = EXCLUDED.imdb_id,
				tvdb_id = EXCLUDED.tvdb_id,
				observed_file_count = EXCLUDED.observed_file_count,
				sample_file_path = EXCLUDED.sample_file_path,
				sample_observed_root_path = EXCLUDED.sample_observed_root_path,
				evidence_json = EXCLUDED.evidence_json,
				override_source = EXCLUDED.override_source,
				last_seen_at = NOW()
		`,
			group.MediaFolderID,
			group.GroupKeyVersion,
			group.ContentGroupKey,
			group.State,
			group.InferredType,
			group.TypeConfidence,
			group.BaseTitle,
			group.BaseYear,
			group.TmdbID,
			group.ImdbID,
			group.TvdbID,
			group.ObservedFileCount,
			group.SampleFilePath,
			group.SampleObservedRootPath,
			group.EvidenceJSON,
			group.OverrideSource,
		)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()
	for range groups {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting scanned media groups: %w", err)
		}
	}
	return nil
}

func (r *ScannedGroupRepository) ReplaceForFolder(ctx context.Context, folderID int, groups []models.ScannedMediaGroup) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin scanned group replace transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM scanned_media_groups WHERE media_folder_id = $1`, folderID); err != nil {
		return fmt.Errorf("clearing scanned media groups: %w", err)
	}

	batch := &pgx.Batch{}
	for _, group := range groups {
		batch.Queue(`
			INSERT INTO scanned_media_groups (
				media_folder_id, group_key_version, content_group_key, state, inferred_type,
				type_confidence, base_title, base_year, tmdb_id, imdb_id, tvdb_id, observed_file_count,
				sample_file_path, sample_observed_root_path, evidence_json, override_source, first_seen_at, last_seen_at
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10, $11, $12,
				$13, $14, COALESCE($15, '{}'::jsonb), $16, NOW(), NOW()
			)
			ON CONFLICT (media_folder_id, group_key_version, content_group_key) DO UPDATE SET
				state = EXCLUDED.state,
				inferred_type = EXCLUDED.inferred_type,
				type_confidence = EXCLUDED.type_confidence,
				base_title = EXCLUDED.base_title,
				base_year = EXCLUDED.base_year,
				tmdb_id = EXCLUDED.tmdb_id,
				imdb_id = EXCLUDED.imdb_id,
				tvdb_id = EXCLUDED.tvdb_id,
				observed_file_count = EXCLUDED.observed_file_count,
				sample_file_path = EXCLUDED.sample_file_path,
				sample_observed_root_path = EXCLUDED.sample_observed_root_path,
				evidence_json = EXCLUDED.evidence_json,
				override_source = EXCLUDED.override_source,
				last_seen_at = NOW()
		`,
			group.MediaFolderID,
			group.GroupKeyVersion,
			group.ContentGroupKey,
			group.State,
			group.InferredType,
			group.TypeConfidence,
			group.BaseTitle,
			group.BaseYear,
			group.TmdbID,
			group.ImdbID,
			group.TvdbID,
			group.ObservedFileCount,
			group.SampleFilePath,
			group.SampleObservedRootPath,
			group.EvidenceJSON,
			group.OverrideSource,
		)
	}
	results := tx.SendBatch(ctx, batch)
	for range groups {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("inserting scanned media groups: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("closing scanned group batch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit scanned group replace transaction: %w", err)
	}
	return nil
}

func (r *ScannedGroupRepository) ReplaceInScope(
	ctx context.Context,
	folderID int,
	scopePath string,
	groups []models.ScannedMediaGroup,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin scanned group scoped replace transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `
		DELETE FROM scanned_media_groups
		WHERE media_folder_id = $1
		  AND (sample_observed_root_path = $2 OR strpos(sample_observed_root_path, $2 || '/') = 1)
	`, folderID, scopePath); err != nil {
		return fmt.Errorf("clearing scanned media groups in scope: %w", err)
	}

	batch := &pgx.Batch{}
	for _, group := range groups {
		batch.Queue(`
			INSERT INTO scanned_media_groups (
				media_folder_id, group_key_version, content_group_key, state, inferred_type,
				type_confidence, base_title, base_year, tmdb_id, imdb_id, tvdb_id, observed_file_count,
				sample_file_path, sample_observed_root_path, evidence_json, override_source, first_seen_at, last_seen_at
			)
				VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10, $11, $12,
					$13, $14, COALESCE($15, '{}'::jsonb), $16, NOW(), NOW()
				)
				ON CONFLICT (media_folder_id, group_key_version, content_group_key) DO UPDATE SET
					state = EXCLUDED.state,
					inferred_type = EXCLUDED.inferred_type,
					type_confidence = EXCLUDED.type_confidence,
					base_title = EXCLUDED.base_title,
					base_year = EXCLUDED.base_year,
					tmdb_id = EXCLUDED.tmdb_id,
					imdb_id = EXCLUDED.imdb_id,
					tvdb_id = EXCLUDED.tvdb_id,
					observed_file_count = EXCLUDED.observed_file_count,
					sample_file_path = EXCLUDED.sample_file_path,
					sample_observed_root_path = EXCLUDED.sample_observed_root_path,
					evidence_json = EXCLUDED.evidence_json,
					override_source = EXCLUDED.override_source,
					last_seen_at = NOW()
			`,
			group.MediaFolderID,
			group.GroupKeyVersion,
			group.ContentGroupKey,
			group.State,
			group.InferredType,
			group.TypeConfidence,
			group.BaseTitle,
			group.BaseYear,
			group.TmdbID,
			group.ImdbID,
			group.TvdbID,
			group.ObservedFileCount,
			group.SampleFilePath,
			group.SampleObservedRootPath,
			group.EvidenceJSON,
			group.OverrideSource,
		)
	}
	results := tx.SendBatch(ctx, batch)
	for range groups {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("inserting scanned media groups in scope: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("closing scanned group scoped batch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit scanned group scoped replace transaction: %w", err)
	}
	return nil
}
