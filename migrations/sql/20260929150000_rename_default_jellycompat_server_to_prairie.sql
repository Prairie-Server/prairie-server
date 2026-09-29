-- +goose Up
-- Prairie: the initial schema seeded the Jellyfin compat server name as the
-- upstream default "Silo", so every install advertised itself as "Silo" to
-- Jellyfin clients even though the config default is "Prairie". Move the
-- untouched upstream defaults ("Silo", and the older "StreamApp") over.
UPDATE server_settings
SET value = 'Prairie'
WHERE key = 'jellyfin_compat.server_name'
  AND value IN ('Silo', 'StreamApp');

-- +goose Down
-- Intentionally a no-op: after the update there is no reliable way to
-- distinguish the migrated default from an operator-chosen "Prairie" name.
SELECT 1;
