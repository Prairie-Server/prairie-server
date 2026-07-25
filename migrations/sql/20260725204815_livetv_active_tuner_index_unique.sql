-- +goose NO TRANSACTION

-- Enforce at most one active session per tuner index (closes TOCTOU between
-- ActiveSessionTunerIndices and CreateSession under concurrent StartChannelSession).
-- CONCURRENTLY avoids blocking livetv_sessions writes during CreateSession.

-- +goose Up
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
          AND c.relname = 'livetv_sessions_active_tuner_index_uidx'
          AND NOT i.indisvalid
    ) THEN
        DROP INDEX public.livetv_sessions_active_tuner_index_uidx;
    END IF;
END;
$$;
-- +goose StatementEnd

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS livetv_sessions_active_tuner_index_uidx
    ON public.livetv_sessions (tuner_id, tuner_index)
    WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS public.livetv_sessions_active_tuner_index_uidx;
