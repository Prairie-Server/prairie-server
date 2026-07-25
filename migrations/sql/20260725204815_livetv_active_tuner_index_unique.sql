-- +goose Up
-- +goose StatementBegin
-- Enforce at most one active session per tuner index (closes TOCTOU between
-- ActiveSessionTunerIndices and CreateSession under concurrent StartChannelSession).
CREATE UNIQUE INDEX IF NOT EXISTS livetv_sessions_active_tuner_index_uidx
    ON public.livetv_sessions (tuner_id, tuner_index)
    WHERE status = 'active';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS public.livetv_sessions_active_tuner_index_uidx;
-- +goose StatementEnd
