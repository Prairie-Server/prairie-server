-- +goose Up
-- Scope Live TV DVR rows to the scheduling user/profile (session ownership pattern).

ALTER TABLE public.livetv_recordings
    ADD COLUMN IF NOT EXISTS user_id integer,
    ADD COLUMN IF NOT EXISTS profile_id text NOT NULL DEFAULT '';

ALTER TABLE public.livetv_series_rules
    ADD COLUMN IF NOT EXISTS user_id integer,
    ADD COLUMN IF NOT EXISTS profile_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS livetv_recordings_owner_idx
    ON public.livetv_recordings (user_id, profile_id);

CREATE INDEX IF NOT EXISTS livetv_series_rules_owner_idx
    ON public.livetv_series_rules (user_id, profile_id);

-- +goose Down
DROP INDEX IF EXISTS public.livetv_series_rules_owner_idx;
DROP INDEX IF EXISTS public.livetv_recordings_owner_idx;
ALTER TABLE public.livetv_series_rules DROP COLUMN IF EXISTS profile_id;
ALTER TABLE public.livetv_series_rules DROP COLUMN IF EXISTS user_id;
ALTER TABLE public.livetv_recordings DROP COLUMN IF EXISTS profile_id;
ALTER TABLE public.livetv_recordings DROP COLUMN IF EXISTS user_id;
