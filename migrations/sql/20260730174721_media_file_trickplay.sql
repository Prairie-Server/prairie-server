-- +goose Up
-- +goose StatementBegin
ALTER TABLE public.media_folders
    ADD COLUMN trickplay_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE public.media_files
    ADD COLUMN trickplay jsonb;

-- Speeds the common backfill path for files that have never had trickplay
-- generated (ListMissingTrickplay ORDER BY probe_updated_at, id).
CREATE INDEX media_files_trickplay_missing_idx
    ON public.media_files (probe_updated_at ASC NULLS FIRST, id ASC)
    WHERE missing_since IS NULL AND trickplay IS NULL AND duration > 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS public.media_files_trickplay_missing_idx;

ALTER TABLE public.media_files
    DROP COLUMN IF EXISTS trickplay;

ALTER TABLE public.media_folders
    DROP COLUMN IF EXISTS trickplay_enabled;
-- +goose StatementEnd
