-- +goose Up
-- Prairie's original seek-preview generator stored sprite-sheet state in
-- media_files.trickplay. It is retired in favor of upstream's lease-based
-- service (media_file_trickplay table), so the column and its partial
-- backfill index go. media_folders.trickplay_enabled stays: upstream's
-- service reads the same per-library switch.
DROP INDEX IF EXISTS public.media_files_trickplay_missing_idx;

ALTER TABLE public.media_files
    DROP COLUMN IF EXISTS trickplay;

-- +goose Down
ALTER TABLE public.media_files
    ADD COLUMN IF NOT EXISTS trickplay jsonb;
