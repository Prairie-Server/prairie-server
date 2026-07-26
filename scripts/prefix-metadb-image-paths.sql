-- prefix-metadb-image-paths.sql
--
-- One-time script to prefix bare MetaDB image paths with "metadb://"
-- so the server can route image resolution to the correct plugin.
--
-- Run after deploying the plugin image resolution feature.
-- Safe to run multiple times (skips already-prefixed paths).
--
-- Usage: psql -d prairie -f scripts/prefix-metadb-image-paths.sql

BEGIN;

-- media_items.poster_path
UPDATE media_items
SET poster_path = 'metadb://' || poster_path
WHERE poster_path IS NOT NULL
  AND poster_path != ''
  AND poster_path != '-'
  AND poster_path NOT LIKE 'http://%'
  AND poster_path NOT LIKE 'https://%'
  AND poster_path NOT LIKE '%://%';

-- media_items.backdrop_path
UPDATE media_items
SET backdrop_path = 'metadb://' || backdrop_path
WHERE backdrop_path IS NOT NULL
  AND backdrop_path != ''
  AND backdrop_path != '-'
  AND backdrop_path NOT LIKE 'http://%'
  AND backdrop_path NOT LIKE 'https://%'
  AND backdrop_path NOT LIKE '%://%';

-- media_items.logo_path
UPDATE media_items
SET logo_path = 'metadb://' || logo_path
WHERE logo_path IS NOT NULL
  AND logo_path != ''
  AND logo_path != '-'
  AND logo_path NOT LIKE 'http://%'
  AND logo_path NOT LIKE 'https://%'
  AND logo_path NOT LIKE '%://%';

-- seasons.poster_path
UPDATE seasons
SET poster_path = 'metadb://' || poster_path
WHERE poster_path IS NOT NULL
  AND poster_path != ''
  AND poster_path != '-'
  AND poster_path NOT LIKE 'http://%'
  AND poster_path NOT LIKE 'https://%'
  AND poster_path NOT LIKE '%://%';

-- episodes.still_path
UPDATE episodes
SET still_path = 'metadb://' || still_path
WHERE still_path IS NOT NULL
  AND still_path != ''
  AND still_path != '-'
  AND still_path NOT LIKE 'http://%'
  AND still_path NOT LIKE 'https://%'
  AND still_path NOT LIKE '%://%';

-- people.photo_path
UPDATE people
SET photo_path = 'metadb://' || photo_path
WHERE photo_path IS NOT NULL
  AND photo_path != ''
  AND photo_path != '-'
  AND photo_path NOT LIKE 'http://%'
  AND photo_path NOT LIKE 'https://%'
  AND photo_path NOT LIKE '%://%';

COMMIT;
