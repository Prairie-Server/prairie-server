-- +goose Up
-- +goose StatementBegin
-- Artwork caching no longer requires S3: a local filesystem backend always
-- exists. Flip the seeded default to true so new and untouched installs
-- convert provider artwork to WebP+AVIF+PNG.
INSERT INTO server_settings (key, value)
VALUES ('metadata.cache_images', 'true')
ON CONFLICT (key) DO NOTHING;

-- Existing installs still hold the original 'false' seed from migration 035.
-- Enable caching once so raw provider URLs are backfilled into local/S3
-- variants. Operators who want passthrough can turn the setting back off.
UPDATE server_settings
SET value = 'true'
WHERE key = 'metadata.cache_images' AND value = 'false';

INSERT INTO server_settings (key, value)
VALUES ('artwork.local_dir', '/var/lib/prairie/artwork')
ON CONFLICT (key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM server_settings WHERE key = 'artwork.local_dir';
-- Do not revert metadata.cache_images — operators may have intentionally kept true.
-- +goose StatementEnd
