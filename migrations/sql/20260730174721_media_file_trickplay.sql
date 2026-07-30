-- +goose Up
-- +goose StatementBegin
ALTER TABLE public.media_folders
    ADD COLUMN trickplay_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE public.media_files
    ADD COLUMN trickplay jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE public.media_files
    DROP COLUMN IF EXISTS trickplay;

ALTER TABLE public.media_folders
    DROP COLUMN IF EXISTS trickplay_enabled;
-- +goose StatementEnd
