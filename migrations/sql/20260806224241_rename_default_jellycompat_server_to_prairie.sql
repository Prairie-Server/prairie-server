-- +goose Up
UPDATE server_settings
SET value = 'Prairie'
WHERE key = 'jellyfin_compat.server_name'
  AND value = 'StreamApp';

-- +goose Down
-- Intentionally a no-op: after the update there is no reliable way to
-- distinguish the migrated default from an operator-chosen "Prairie" name.
SELECT 1;
