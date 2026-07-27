-- +goose Up
-- +goose StatementBegin
-- Repoint persisted Continuum/Silo catalog repository URLs at the forked
-- prairie-plugins catalogs, then force auto-replace of installations whose
-- stored archives still carry pre-rebrand manifests (no prairie_api_version,
-- or a silo.* plugin_id). Migration 20260726223939 renamed plugin_id rows but
-- left plugin_archives.manifest_json / archive_bytes untouched; AutoUpdate can
-- only heal those rows once they track a live prairie catalog and compare as
-- older than the catalog version.
DO $$
DECLARE
    official_url TEXT := 'https://raw.githubusercontent.com/prairie-server/prairie-plugins/main/manifest.json';
    community_url TEXT := 'https://raw.githubusercontent.com/Prairie-Community/prairie-plugins/main/manifest.json';
    official_repository_id BIGINT;
    community_repository_id BIGINT;
    include_community BOOLEAN := false;
    legacy_official_id BIGINT;
    legacy_community_id BIGINT;
BEGIN
    SELECT LOWER(TRIM(value)) = 'true'
    INTO include_community
    FROM public.server_settings
    WHERE key = 'plugins.include_approved_community_plugins';
    include_community := COALESCE(include_community, false);

    -- Prefer the managed official row; fall back to an existing prairie URL row.
    SELECT id INTO official_repository_id
    FROM public.plugin_repositories
    WHERE managed_key = 'official'
    ORDER BY id
    LIMIT 1;

    IF official_repository_id IS NULL THEN
        SELECT id INTO official_repository_id
        FROM public.plugin_repositories
        WHERE url = official_url
        ORDER BY id
        LIMIT 1;
    END IF;

    IF official_repository_id IS NULL THEN
        INSERT INTO public.plugin_repositories (
            url, enabled, display_name, managed_key, source_kind
        ) VALUES (
            official_url, true, 'Prairie maintained', 'official', 'prairie'
        )
        RETURNING id INTO official_repository_id;
    ELSE
        -- Drop a duplicate prairie-URL row first when the managed official row
        -- still points at a legacy host, so the URL update cannot collide.
        IF EXISTS (
            SELECT 1 FROM public.plugin_repositories
            WHERE id = official_repository_id
              AND url <> official_url
        ) THEN
            UPDATE public.plugin_installations
            SET repository_id = official_repository_id,
                available_version = NULL,
                updated_at = NOW()
            WHERE repository_id IN (
                SELECT id FROM public.plugin_repositories
                WHERE url = official_url
                  AND id <> official_repository_id
            );

            DELETE FROM public.plugin_repositories
            WHERE url = official_url
              AND id <> official_repository_id;
        END IF;

        UPDATE public.plugin_repositories
        SET url = official_url,
            display_name = 'Prairie maintained',
            managed_key = 'official',
            source_kind = 'prairie',
            enabled = true,
            updated_at = NOW()
        WHERE id = official_repository_id;
    END IF;

    -- Merge every remaining Continuum/Silo official catalog URL into official.
    FOR legacy_official_id IN
        SELECT id
        FROM public.plugin_repositories
        WHERE id <> official_repository_id
          AND (
              url ILIKE '%/ContinuumApp/continuum-plugins/%'
              OR url ILIKE '%/Silo-Server/silo-plugins/%'
              OR url ILIKE '%/silo-server/silo-plugins/%'
          )
    LOOP
        UPDATE public.plugin_installations
        SET repository_id = official_repository_id,
            available_version = NULL,
            updated_at = NOW()
        WHERE repository_id = legacy_official_id;

        DELETE FROM public.plugin_repositories WHERE id = legacy_official_id;
    END LOOP;

    -- Same pattern for the approved-community channel.
    SELECT id INTO community_repository_id
    FROM public.plugin_repositories
    WHERE managed_key = 'approved-community'
    ORDER BY id
    LIMIT 1;

    IF community_repository_id IS NULL THEN
        SELECT id INTO community_repository_id
        FROM public.plugin_repositories
        WHERE url = community_url
        ORDER BY id
        LIMIT 1;
    END IF;

    IF community_repository_id IS NULL THEN
        INSERT INTO public.plugin_repositories (
            url, enabled, display_name, managed_key, source_kind
        ) VALUES (
            community_url,
            include_community,
            'Approved community',
            'approved-community',
            'approved_community'
        )
        RETURNING id INTO community_repository_id;
    ELSE
        IF EXISTS (
            SELECT 1 FROM public.plugin_repositories
            WHERE id = community_repository_id
              AND url <> community_url
        ) THEN
            UPDATE public.plugin_installations
            SET repository_id = community_repository_id,
                available_version = NULL,
                updated_at = NOW()
            WHERE repository_id IN (
                SELECT id FROM public.plugin_repositories
                WHERE url = community_url
                  AND id <> community_repository_id
            );

            DELETE FROM public.plugin_repositories
            WHERE url = community_url
              AND id <> community_repository_id;
        END IF;

        UPDATE public.plugin_repositories
        SET url = community_url,
            display_name = 'Approved community',
            managed_key = 'approved-community',
            source_kind = 'approved_community',
            enabled = include_community,
            updated_at = NOW()
        WHERE id = community_repository_id;
    END IF;

    FOR legacy_community_id IN
        SELECT id
        FROM public.plugin_repositories
        WHERE id <> community_repository_id
          AND (
              url ILIKE '%/Silo-Community/silo-plugins/%'
              OR url ILIKE '%/silo-community/silo-plugins/%'
          )
    LOOP
        UPDATE public.plugin_installations
        SET repository_id = community_repository_id,
            available_version = NULL,
            updated_at = NOW()
        WHERE repository_id = legacy_community_id;

        DELETE FROM public.plugin_repositories WHERE id = legacy_community_id;
    END LOOP;

    -- Default first-party installs with a NULL repository_id (uploads are left
    -- alone) start tracking the managed prairie catalog so AutoUpdate can
    -- replace any archives deleted below.
    UPDATE public.plugin_installations
    SET repository_id = official_repository_id,
        available_version = NULL,
        updated_at = NOW()
    WHERE kind = 'plugin'
      AND repository_id IS NULL
      AND plugin_id IN ('prairie.tmdb', 'prairie.tvdb');

    UPDATE public.plugin_installations
    SET repository_id = community_repository_id,
        available_version = NULL,
        updated_at = NOW()
    WHERE kind = 'plugin'
      AND repository_id IS NULL
      AND plugin_id IN ('prairie.requests.arr', 'prairie.requests.seerr');

    -- Drop unloadable silo-era archives and force AutoUpdate to replace them
    -- from the repointed prairie catalog (version 0.0.0 < any real release).
    WITH incompatible AS (
        SELECT plugin_installation_id AS installation_id
        FROM public.plugin_archives
        WHERE convert_from(manifest_json, 'UTF8') NOT LIKE '%"prairie_api_version"%'
           OR convert_from(manifest_json, 'UTF8') LIKE '%"plugin_id": "silo.%'
           OR convert_from(manifest_json, 'UTF8') LIKE '%"plugin_id":"silo.%'
    ),
    deleted AS (
        DELETE FROM public.plugin_archives
        WHERE plugin_installation_id IN (SELECT installation_id FROM incompatible)
        RETURNING plugin_installation_id
    )
    UPDATE public.plugin_installations AS installation
    SET version = '0.0.0',
        available_version = NULL,
        update_policy = CASE
            WHEN installation.update_policy IN ('off', 'manual', 'notify') THEN 'auto'
            ELSE installation.update_policy
        END,
        updated_at = NOW()
    FROM deleted
    WHERE installation.id = deleted.plugin_installation_id
      AND installation.kind = 'plugin';
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Irreversible data repair: catalog URL merges and archive deletes cannot be
-- reconstructed safely. Down is intentionally a no-op.
SELECT 1;
-- +goose StatementEnd
