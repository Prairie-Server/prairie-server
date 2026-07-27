-- +goose Up
-- Lazy Live TV artwork cache index. Channel logos are stable (no expiry).
-- Programme images are cached on first On-now/guide request and reaped after
-- the airing ends (+ grace) so the rolling EPG does not fill object storage.
CREATE TABLE public.livetv_artwork_cache (
    id bigserial PRIMARY KEY,
    kind text NOT NULL,
    subject_id text NOT NULL,
    source_url text NOT NULL DEFAULT '',
    object_path text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending',
    expires_at timestamp with time zone,
    last_error text NOT NULL DEFAULT '',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT livetv_artwork_cache_kind_check
        CHECK (kind IN ('channel_logo', 'program')),
    CONSTRAINT livetv_artwork_cache_status_check
        CHECK (status IN ('pending', 'ready', 'failed')),
    CONSTRAINT livetv_artwork_cache_subject_unique
        UNIQUE (kind, subject_id)
);

CREATE INDEX livetv_artwork_cache_expires_idx
    ON public.livetv_artwork_cache (expires_at, id)
    WHERE expires_at IS NOT NULL AND status = 'ready';

CREATE INDEX livetv_artwork_cache_pending_idx
    ON public.livetv_artwork_cache (updated_at, id)
    WHERE status = 'pending';

-- +goose Down
DROP TABLE IF EXISTS public.livetv_artwork_cache;
