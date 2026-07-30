-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE public.media_folders
    ADD COLUMN IF NOT EXISTS trickplay_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE public.media_files
    ADD COLUMN IF NOT EXISTS trickplay jsonb;

-- Speeds the common backfill path for files that have never had trickplay
-- generated (ListMissingTrickplay ORDER BY probe_updated_at, id).
-- CONCURRENTLY avoids blocking media_files writes; requires NO TRANSACTION.
-- A crashed CREATE INDEX CONCURRENTLY can leave an INVALID index which blocks
-- IF NOT EXISTS retries; drop it first if present.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_index i ON i.indexrelid = c.oid
        WHERE n.nspname = 'public'
          AND c.relname = 'media_files_trickplay_missing_idx'
          AND NOT i.indisvalid
    ) THEN
        DROP INDEX public.media_files_trickplay_missing_idx;
    END IF;
END;
$$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS media_files_trickplay_missing_idx
    ON public.media_files (probe_updated_at ASC NULLS FIRST, id ASC)
    WHERE missing_since IS NULL AND trickplay IS NULL AND duration > 0;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS public.media_files_trickplay_missing_idx;

ALTER TABLE public.media_files
    DROP COLUMN IF EXISTS trickplay;

ALTER TABLE public.media_folders
    DROP COLUMN IF EXISTS trickplay_enabled;
