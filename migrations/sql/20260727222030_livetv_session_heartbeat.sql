-- Live TV sessions held a tuner index until an explicit release, so any
-- abandoned tune (closed tab, killed app, failed playback, server restart)
-- leaked a tuner forever and StartChannelSession returned ErrNoTuner.
-- last_seen_at lets the service reclaim sessions that stopped being watched.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE public.livetv_sessions
    ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS livetv_sessions_active_last_seen_idx
    ON public.livetv_sessions (last_seen_at)
    WHERE status = 'active';

-- Pre-heartbeat active rows have no live ffmpeg behind them; release the leak.
UPDATE public.livetv_sessions
SET status = 'released', released_at = now()
WHERE status = 'active';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS public.livetv_sessions_active_last_seen_idx;

ALTER TABLE public.livetv_sessions
    DROP COLUMN IF EXISTS last_seen_at;
-- +goose StatementEnd
