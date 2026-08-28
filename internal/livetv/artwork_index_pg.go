package livetv

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgArtworkIndex is the Postgres-backed artwork index. Excluded from the
// livetv coverage gate alongside store.go (DB I/O impractical to unit-test).
type pgArtworkIndex struct {
	db *pgxpool.Pool
}

func newPgArtworkIndex(db *pgxpool.Pool) *pgArtworkIndex {
	return &pgArtworkIndex{db: db}
}

func (p *pgArtworkIndex) LookupMany(ctx context.Context, kind string, subjectIDs []string) (map[string]*artworkRow, error) {
	out := map[string]*artworkRow{}
	if len(subjectIDs) == 0 {
		return out, nil
	}
	rows, err := p.db.Query(ctx, `
		SELECT kind, subject_id, source_url, object_path, status, expires_at
		FROM livetv_artwork_cache
		WHERE kind = $1 AND subject_id = ANY($2)`, kind, subjectIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r artworkRow
		if err := rows.Scan(&r.Kind, &r.SubjectID, &r.SourceURL, &r.ObjectPath, &r.Status, &r.ExpiresAt); err != nil {
			return nil, err
		}
		cp := r
		out[r.SubjectID] = &cp
	}
	return out, rows.Err()
}

func (p *pgArtworkIndex) UpsertPending(ctx context.Context, kind, subjectID, sourceURL string, expiresAt time.Time) error {
	var expires any
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC()
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO livetv_artwork_cache (kind, subject_id, source_url, status, expires_at, updated_at)
		VALUES ($1, $2, $3, 'pending', $4, now())
		ON CONFLICT (kind, subject_id) DO UPDATE SET
			source_url = EXCLUDED.source_url,
			status = CASE
				WHEN livetv_artwork_cache.status = 'ready'
					AND livetv_artwork_cache.source_url = EXCLUDED.source_url
					AND livetv_artwork_cache.object_path <> ''
				THEN livetv_artwork_cache.status
				ELSE 'pending'
			END,
			expires_at = COALESCE(EXCLUDED.expires_at, livetv_artwork_cache.expires_at),
			updated_at = now()`,
		kind, subjectID, sourceURL, expires)
	return err
}

func (p *pgArtworkIndex) MarkReady(ctx context.Context, kind, subjectID, sourceURL, objectPath string, expiresAt time.Time) error {
	var expires any
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC()
	}
	_, err := p.db.Exec(ctx, `
		UPDATE livetv_artwork_cache SET
			source_url = $3,
			object_path = $4,
			status = 'ready',
			expires_at = $5,
			last_error = '',
			updated_at = now()
		WHERE kind = $1 AND subject_id = $2`,
		kind, subjectID, sourceURL, objectPath, expires)
	return err
}

func (p *pgArtworkIndex) MarkFailed(ctx context.Context, kind, subjectID, errText string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE livetv_artwork_cache SET
			status = 'failed',
			last_error = $3,
			updated_at = now()
		WHERE kind = $1 AND subject_id = $2`,
		kind, subjectID, truncateErr(errText))
	return err
}

func (p *pgArtworkIndex) TouchExpiry(ctx context.Context, kind, subjectID string, expiresAt time.Time) error {
	_, err := p.db.Exec(ctx, `
		UPDATE livetv_artwork_cache SET expires_at = $3, updated_at = now()
		WHERE kind = $1 AND subject_id = $2 AND status = 'ready'
			AND (expires_at IS NULL OR expires_at < $3)`,
		kind, subjectID, expiresAt.UTC())
	return err
}

func (p *pgArtworkIndex) ListExpired(ctx context.Context, limit int) ([]artworkRow, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, kind, subject_id, object_path
		FROM livetv_artwork_cache
		WHERE expires_at IS NOT NULL AND expires_at < now() AND status = 'ready'
		ORDER BY expires_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []artworkRow
	for rows.Next() {
		var r artworkRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.SubjectID, &r.ObjectPath); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *pgArtworkIndex) Delete(ctx context.Context, id int64) (bool, error) {
	tag, err := p.db.Exec(ctx, `DELETE FROM livetv_artwork_cache WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
