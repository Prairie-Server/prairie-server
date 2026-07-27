package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
	"github.com/prairie-server/prairie-server/internal/models"
)

const (
	AVIFBackfillStatusQueued    = "queued"
	AVIFBackfillStatusRunning   = "running"
	AVIFBackfillStatusSucceeded = "succeeded"
	AVIFBackfillStatusFailed    = "failed"

	avifBackfillLeaseDuration = 5 * time.Minute
	avifBackfillMaxAttempts   = 8
)

// AVIFBackfillJobRepository persists deferred AVIF sibling work.
type AVIFBackfillJobRepository struct {
	pool *pgxpool.Pool
}

func NewAVIFBackfillJobRepository(pool *pgxpool.Pool) *AVIFBackfillJobRepository {
	return &AVIFBackfillJobRepository{pool: pool}
}

func avifBackfillRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	delay := time.Minute << min(attempt-1, 7)
	if delay > 2*time.Hour {
		return 2 * time.Hour
	}
	return delay
}

// Enqueue inserts or requeues an AVIF backfill job for the given WebP original.
// Returns the job id. Requeueing a succeeded/failed row resets it to queued so
// reconcile can recover orphans after a prior incomplete run.
func (r *AVIFBackfillJobRepository) Enqueue(ctx context.Context, originalPath, imageType string) (int64, error) {
	ids, err := r.EnqueueBatch(ctx, []AVIFBackfillEnqueueInput{{
		OriginalPath: originalPath,
		ImageType:    imageType,
	}})
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

// AVIFBackfillEnqueueInput is one original WebP path to ensure AVIF siblings for.
type AVIFBackfillEnqueueInput struct {
	OriginalPath string
	ImageType    string
}

// EnqueueBatch upserts many AVIF backfill jobs. Returns the ids that were
// inserted or requeued (already-running/queued rows keep their id but may be
// omitted from the returned slice when unchanged).
func (r *AVIFBackfillJobRepository) EnqueueBatch(ctx context.Context, inputs []AVIFBackfillEnqueueInput) ([]int64, error) {
	if r == nil || r.pool == nil || len(inputs) == 0 {
		return nil, nil
	}
	valid := make([]AVIFBackfillEnqueueInput, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, in := range inputs {
		path := strings.TrimSpace(in.OriginalPath)
		if path == "" || strings.Contains(path, "://") {
			continue
		}
		if artworkkey.WebPAVIFSibling(path) == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		imageType := strings.TrimSpace(in.ImageType)
		if imageType == "" {
			imageType = artworkkey.ImageTypeFromPath(path)
		}
		valid = append(valid, AVIFBackfillEnqueueInput{OriginalPath: path, ImageType: imageType})
	}
	if len(valid) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(valid))
	for start := 0; start < len(valid); start += 250 {
		end := start + 250
		if end > len(valid) {
			end = len(valid)
		}
		chunkIDs, err := r.enqueueBatchChunk(ctx, valid[start:end])
		if err != nil {
			return ids, err
		}
		ids = append(ids, chunkIDs...)
	}
	return ids, nil
}

func (r *AVIFBackfillJobRepository) enqueueBatchChunk(ctx context.Context, inputs []AVIFBackfillEnqueueInput) ([]int64, error) {
	paths := make([]string, len(inputs))
	types := make([]string, len(inputs))
	for i, in := range inputs {
		paths[i] = in.OriginalPath
		types[i] = in.ImageType
	}
	// Always RETURNING id (even for already-queued rows) so the eager CacheBytes
	// path can TryClaim the job. Succeeded/failed rows are requeued; queued/
	// running rows keep their lease and attempt state.
	rows, err := r.pool.Query(ctx, `
		INSERT INTO artwork_avif_backfill_jobs (original_path, image_type)
		SELECT p, t
		FROM unnest($1::text[], $2::text[]) AS u(p, t)
		ON CONFLICT (original_path) DO UPDATE SET
			image_type = CASE
				WHEN EXCLUDED.image_type <> '' THEN EXCLUDED.image_type
				ELSE artwork_avif_backfill_jobs.image_type
			END,
			status = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN 'queued'
				ELSE artwork_avif_backfill_jobs.status
			END,
			attempt_count = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN 0
				ELSE artwork_avif_backfill_jobs.attempt_count
			END,
			next_attempt_at = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN NOW()
				ELSE artwork_avif_backfill_jobs.next_attempt_at
			END,
			locked_at = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN NULL
				ELSE artwork_avif_backfill_jobs.locked_at
			END,
			locked_by = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN ''
				ELSE artwork_avif_backfill_jobs.locked_by
			END,
			last_error = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN ''
				ELSE artwork_avif_backfill_jobs.last_error
			END,
			completed_at = CASE
				WHEN artwork_avif_backfill_jobs.status IN ('succeeded', 'failed')
					THEN NULL
				ELSE artwork_avif_backfill_jobs.completed_at
			END,
			updated_at = NOW()
		RETURNING id
	`, paths, types)
	if err != nil {
		return nil, fmt.Errorf("enqueuing artwork AVIF backfill jobs: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0, len(inputs))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning artwork AVIF backfill job id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artwork AVIF backfill job ids: %w", err)
	}
	return ids, nil
}

// TryClaim claims a specific queued job for eager in-process processing.
// Returns false when another worker already holds it or it is no longer queued.
func (r *AVIFBackfillJobRepository) TryClaim(ctx context.Context, jobID int64, workerID string) (bool, error) {
	if r == nil || r.pool == nil || jobID <= 0 || strings.TrimSpace(workerID) == "" {
		return false, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE artwork_avif_backfill_jobs
		SET status = 'running',
			locked_at = NOW(),
			locked_by = $2,
			updated_at = NOW()
		WHERE id = $1
		  AND status = 'queued'
		  AND next_attempt_at <= NOW()
	`, jobID, workerID)
	if err != nil {
		return false, fmt.Errorf("claiming artwork AVIF backfill job %d: %w", jobID, err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *AVIFBackfillJobRepository) recoverExpiredRunning(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE artwork_avif_backfill_jobs
		SET status = 'queued',
			next_attempt_at = NOW(),
			locked_at = NULL,
			locked_by = '',
			updated_at = NOW()
		WHERE status = 'running'
		  AND locked_at < NOW() - $1::interval
	`, intervalLiteral(avifBackfillLeaseDuration))
	if err != nil {
		return fmt.Errorf("recovering expired artwork AVIF backfill jobs: %w", err)
	}
	return nil
}

// ClaimDue claims up to limit due queued jobs.
func (r *AVIFBackfillJobRepository) ClaimDue(ctx context.Context, workerID string, limit int) ([]*models.ArtworkAVIFBackfillJob, error) {
	if r == nil || r.pool == nil || limit <= 0 {
		return nil, nil
	}
	if err := r.recoverExpiredRunning(ctx); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		WITH due AS (
			SELECT id
			FROM artwork_avif_backfill_jobs
			WHERE status = 'queued'
			  AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at ASC, id ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE artwork_avif_backfill_jobs j
		SET status = 'running',
			locked_at = NOW(),
			locked_by = $2,
			updated_at = NOW()
		FROM due
		WHERE j.id = due.id
		RETURNING
			j.id, j.original_path, j.image_type, j.status, j.attempt_count,
			j.next_attempt_at, j.locked_at, j.locked_by, j.last_error,
			j.created_at, j.updated_at, j.completed_at
	`, limit, workerID)
	if err != nil {
		return nil, fmt.Errorf("claiming artwork AVIF backfill jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]*models.ArtworkAVIFBackfillJob, 0, limit)
	for rows.Next() {
		job := new(models.ArtworkAVIFBackfillJob)
		if err := rows.Scan(
			&job.ID, &job.OriginalPath, &job.ImageType, &job.Status, &job.AttemptCount,
			&job.NextAttemptAt, &job.LockedAt, &job.LockedBy, &job.LastError,
			&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning artwork AVIF backfill job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating artwork AVIF backfill jobs: %w", err)
	}
	return jobs, nil
}

func (r *AVIFBackfillJobRepository) MarkSucceeded(ctx context.Context, id int64, lockedBy string) error {
	if r == nil || r.pool == nil {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE artwork_avif_backfill_jobs
		SET status = 'succeeded',
			completed_at = NOW(),
			locked_at = NULL,
			locked_by = '',
			last_error = '',
			updated_at = NOW()
		WHERE id = $1
		  AND status = 'running'
		  AND locked_by = $2
	`, id, lockedBy)
	if err != nil {
		return fmt.Errorf("marking artwork AVIF backfill job succeeded: %w", err)
	}
	return nil
}

func (r *AVIFBackfillJobRepository) MarkFailed(ctx context.Context, id int64, attemptCount int, lockedBy, errText string) error {
	if r == nil || r.pool == nil {
		return nil
	}
	nextAttempt := attemptCount + 1
	status := AVIFBackfillStatusQueued
	if nextAttempt >= avifBackfillMaxAttempts {
		status = AVIFBackfillStatusFailed
	}
	delay := avifBackfillRetryDelay(nextAttempt)
	_, err := r.pool.Exec(ctx, `
		UPDATE artwork_avif_backfill_jobs
		SET status = $2,
			attempt_count = $3,
			next_attempt_at = NOW() + $4::interval,
			locked_at = NULL,
			locked_by = '',
			last_error = left($5, 2000),
			updated_at = NOW()
		WHERE id = $1
		  AND status = 'running'
		  AND locked_by = $6
	`, id, status, nextAttempt, intervalLiteral(delay), errText, lockedBy)
	if err != nil {
		return fmt.Errorf("marking artwork AVIF backfill job failed: %w", err)
	}
	return nil
}

// RequeueClaimed returns claimed-but-unprocessed jobs to the queue without
// burning a retry attempt.
func (r *AVIFBackfillJobRepository) RequeueClaimed(ctx context.Context, ids []int64, workerID string) error {
	if r == nil || r.pool == nil || len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE artwork_avif_backfill_jobs
		SET status = 'queued',
			next_attempt_at = NOW(),
			locked_at = NULL,
			locked_by = '',
			updated_at = NOW()
		WHERE id = ANY($1)
		  AND status = 'running'
		  AND locked_by = $2
	`, ids, workerID)
	if err != nil {
		return fmt.Errorf("requeueing artwork AVIF backfill jobs: %w", err)
	}
	return nil
}

// DeleteSucceededBefore removes old succeeded rows for retention.
func (r *AVIFBackfillJobRepository) DeleteSucceededBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if r == nil || r.pool == nil || limit <= 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM artwork_avif_backfill_jobs
		WHERE id IN (
			SELECT id FROM artwork_avif_backfill_jobs
			WHERE status = 'succeeded'
			  AND completed_at IS NOT NULL
			  AND completed_at < $1
			ORDER BY completed_at ASC, id ASC
			LIMIT $2
		)
	`, before, limit)
	if err != nil {
		return 0, fmt.Errorf("deleting succeeded artwork AVIF backfill jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// QueuedCount returns how many jobs are currently queued (for progress).
func (r *AVIFBackfillJobRepository) QueuedCount(ctx context.Context) (int, error) {
	if r == nil || r.pool == nil {
		return 0, nil
	}
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM artwork_avif_backfill_jobs WHERE status = 'queued'
	`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting queued artwork AVIF backfill jobs: %w", err)
	}
	return n, nil
}

// LookupID returns the job id for an original path, or 0 when absent.
func (r *AVIFBackfillJobRepository) LookupID(ctx context.Context, originalPath string) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, nil
	}
	var id int64
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM artwork_avif_backfill_jobs WHERE original_path = $1
	`, strings.TrimSpace(originalPath)).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("looking up artwork AVIF backfill job: %w", err)
	}
	return id, nil
}
