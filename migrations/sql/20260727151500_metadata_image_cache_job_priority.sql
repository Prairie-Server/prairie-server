-- +goose Up
-- Prefer user-visible artwork (item posters/backdrops) over episode stills and
-- person photos when claiming due cache jobs. Existing rows are backfilled from
-- target_type/image_type so a library that already queued tens of thousands of
-- episode jobs still surfaces posters first after upgrade.
ALTER TABLE public.metadata_image_cache_jobs
    ADD COLUMN IF NOT EXISTS priority integer NOT NULL DEFAULT 0;

UPDATE public.metadata_image_cache_jobs
SET priority = CASE target_type
    WHEN 'item' THEN CASE image_type
        WHEN 'poster' THEN 100
        WHEN 'backdrop' THEN 90
        WHEN 'logo' THEN 80
        ELSE 70
    END
    WHEN 'item_localization' THEN CASE image_type
        WHEN 'poster' THEN 95
        WHEN 'backdrop' THEN 85
        WHEN 'logo' THEN 75
        ELSE 65
    END
    WHEN 'season' THEN 50
    WHEN 'season_localization' THEN 45
    WHEN 'episode' THEN 20
    WHEN 'person' THEN 10
    ELSE 0
END;

DROP INDEX IF EXISTS public.metadata_image_cache_jobs_due_idx;

CREATE INDEX metadata_image_cache_jobs_due_idx
    ON public.metadata_image_cache_jobs (priority DESC, next_attempt_at, id)
    WHERE status = 'queued';

-- +goose Down
DROP INDEX IF EXISTS public.metadata_image_cache_jobs_due_idx;

CREATE INDEX metadata_image_cache_jobs_due_idx
    ON public.metadata_image_cache_jobs (next_attempt_at, id)
    WHERE status = 'queued';

ALTER TABLE public.metadata_image_cache_jobs
    DROP COLUMN IF EXISTS priority;
