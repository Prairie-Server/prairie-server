-- +goose NO TRANSACTION
-- +goose Up
-- Serve the per-item community rating aggregate; the primary key leads with
-- (user_id, profile_id), so it cannot answer a media_item_id lookup.
-- Remove an unusable remnant of an interrupted concurrent build before retry.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_index i ON i.indexrelid = c.oid
        WHERE n.nspname = 'public'
          AND c.relname = 'idx_user_ratings_media_item'
          AND NOT i.indisvalid
    ) THEN
        DROP INDEX public.idx_user_ratings_media_item;
    END IF;
END;
$$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_ratings_media_item
ON user_ratings (media_item_id) INCLUDE (rating);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_user_ratings_media_item;
