-- +goose Up
-- Durable queue for deferred AVIF sibling generation. WebP publishes first so
-- the library is browsable; AVIF encodes are slower and used to run as
-- fire-and-forget goroutines that vanished on deploy/OOM. Persisting them here
-- lets ClaimDue recover after restarts and lets a reconcile pass backfill the
-- historical webp-without-avif gap.
CREATE TABLE public.artwork_avif_backfill_jobs (
    id bigserial PRIMARY KEY,
    original_path text NOT NULL,
    image_type text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'queued',
    attempt_count integer NOT NULL DEFAULT 0,
    next_attempt_at timestamp with time zone NOT NULL DEFAULT now(),
    locked_at timestamp with time zone,
    locked_by text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    completed_at timestamp with time zone,
    CONSTRAINT artwork_avif_backfill_jobs_original_path_check
        CHECK (BTRIM(original_path) <> ''),
    CONSTRAINT artwork_avif_backfill_jobs_status_check
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    CONSTRAINT artwork_avif_backfill_jobs_attempt_count_check
        CHECK (attempt_count >= 0),
    CONSTRAINT artwork_avif_backfill_jobs_original_path_unique
        UNIQUE (original_path)
);

CREATE INDEX artwork_avif_backfill_jobs_due_idx
    ON public.artwork_avif_backfill_jobs (next_attempt_at, id)
    WHERE status = 'queued';

CREATE INDEX artwork_avif_backfill_jobs_running_lease_idx
    ON public.artwork_avif_backfill_jobs (locked_at, id)
    WHERE status = 'running';

CREATE INDEX artwork_avif_backfill_jobs_succeeded_retention_idx
    ON public.artwork_avif_backfill_jobs (completed_at, id)
    WHERE status = 'succeeded';

-- +goose Down
DROP TABLE IF EXISTS public.artwork_avif_backfill_jobs;
