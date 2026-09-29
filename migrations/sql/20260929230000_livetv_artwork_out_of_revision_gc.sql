-- +goose Up
-- Prairie: the shared image cache registered Live TV logos and programme
-- images with the artwork revision GC. No catalog surface references them
-- (livetv_artwork_cache does), so the GC treated them as garbage and deleted
-- every cached channel logo while the cache rows still read "ready", which
-- served 404s. Drop the Live TV candidates and re-queue the logos; the tracker
-- no longer records livetv/ paths.
DELETE FROM artwork_revision_gc_candidates WHERE original_path LIKE 'livetv/%';

UPDATE livetv_artwork_cache
SET status = 'pending', object_path = '', last_error = '', updated_at = now()
WHERE kind = 'channel_logo' AND status = 'ready';

-- +goose Down
-- Intentionally a no-op: the deleted rows only scheduled wrongful deletions.
SELECT 1;
