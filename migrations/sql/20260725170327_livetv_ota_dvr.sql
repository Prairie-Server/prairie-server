-- +goose Up
-- +goose StatementBegin
-- Live TV / OTA (HDHomeRun) + synced guide + DVR foundation (issue #1).

CREATE TABLE public.livetv_tuners (
    id              text PRIMARY KEY,
    type            text NOT NULL DEFAULT 'hdhomerun',
    device_id       text NOT NULL,
    discover_url    text NOT NULL DEFAULT '',
    base_url        text NOT NULL DEFAULT '',
    model           text NOT NULL DEFAULT '',
    firmware        text NOT NULL DEFAULT '',
    tuner_count     integer NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'pending',
    last_error      text NOT NULL DEFAULT '',
    last_scan_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (type, device_id)
);

CREATE TABLE public.livetv_channels (
    id                  text PRIMARY KEY,
    tuner_id            text NOT NULL REFERENCES public.livetv_tuners(id) ON DELETE CASCADE,
    number              text NOT NULL DEFAULT '',
    number_override     text,
    callsign            text NOT NULL DEFAULT '',
    name                text NOT NULL DEFAULT '',
    logo_url            text NOT NULL DEFAULT '',
    hd                  boolean NOT NULL DEFAULT false,
    enabled             boolean NOT NULL DEFAULT true,
    stream_url          text NOT NULL DEFAULT '',
    guide_station_id    text NOT NULL DEFAULT '',
    sort_key            integer NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX livetv_channels_tuner_id_idx ON public.livetv_channels (tuner_id);
CREATE INDEX livetv_channels_enabled_sort_idx ON public.livetv_channels (enabled, sort_key, number);

CREATE TABLE public.livetv_guide_sources (
    id              text PRIMARY KEY,
    type            text NOT NULL, -- schedules_direct | xmltv_url
    priority        integer NOT NULL DEFAULT 100,
    enabled         boolean NOT NULL DEFAULT true,
    display_name    text NOT NULL DEFAULT '',
    -- type-specific config (url, lineup id, username); secrets encrypted via server_settings pattern later
    config_json     jsonb NOT NULL DEFAULT '{}'::jsonb,
    status          text NOT NULL DEFAULT 'idle',
    last_error      text NOT NULL DEFAULT '',
    last_sync_at    timestamptz,
    next_sync_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX livetv_guide_sources_priority_idx
    ON public.livetv_guide_sources (priority, enabled);

CREATE TABLE public.livetv_programs (
    id              text PRIMARY KEY,
    channel_id      text NOT NULL REFERENCES public.livetv_channels(id) ON DELETE CASCADE,
    source_id       text REFERENCES public.livetv_guide_sources(id) ON DELETE SET NULL,
    series_id       text NOT NULL DEFAULT '',
    external_id     text NOT NULL DEFAULT '',
    start_at        timestamptz NOT NULL,
    stop_at         timestamptz NOT NULL,
    title           text NOT NULL DEFAULT '',
    subtitle        text NOT NULL DEFAULT '',
    description     text NOT NULL DEFAULT '',
    season          integer,
    episode         integer,
    genres          text[] NOT NULL DEFAULT '{}',
    image_url       text NOT NULL DEFAULT '',
    is_new          boolean NOT NULL DEFAULT false,
    is_live         boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX livetv_programs_channel_window_idx
    ON public.livetv_programs (channel_id, start_at, stop_at);
CREATE INDEX livetv_programs_series_id_idx
    ON public.livetv_programs (series_id)
    WHERE series_id <> '';

CREATE TABLE public.livetv_sessions (
    id              text PRIMARY KEY,
    channel_id      text NOT NULL REFERENCES public.livetv_channels(id) ON DELETE CASCADE,
    tuner_id        text NOT NULL REFERENCES public.livetv_tuners(id) ON DELETE CASCADE,
    tuner_index     integer NOT NULL DEFAULT 0,
    user_id         integer,
    profile_id      text NOT NULL DEFAULT '',
    playback_session_id text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'active',
    created_at      timestamptz NOT NULL DEFAULT now(),
    released_at     timestamptz
);

CREATE INDEX livetv_sessions_active_tuner_idx
    ON public.livetv_sessions (tuner_id)
    WHERE status = 'active';

CREATE TABLE public.livetv_recordings (
    id                  text PRIMARY KEY,
    program_id          text REFERENCES public.livetv_programs(id) ON DELETE SET NULL,
    channel_id          text NOT NULL REFERENCES public.livetv_channels(id) ON DELETE CASCADE,
    series_rule_id      text,
    status              text NOT NULL DEFAULT 'scheduled', -- scheduled|recording|completed|failed|cancelled
    path                text NOT NULL DEFAULT '',
    library_item_id     text,
    start_at            timestamptz NOT NULL,
    stop_at             timestamptz NOT NULL,
    title               text NOT NULL DEFAULT '',
    last_error          text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX livetv_recordings_status_start_idx
    ON public.livetv_recordings (status, start_at);

CREATE TABLE public.livetv_series_rules (
    id              text PRIMARY KEY,
    series_id       text NOT NULL DEFAULT '',
    channel_id      text REFERENCES public.livetv_channels(id) ON DELETE CASCADE,
    title_match     text NOT NULL DEFAULT '',
    new_only        boolean NOT NULL DEFAULT false,
    keep_last       integer NOT NULL DEFAULT 0, -- 0 = unlimited
    enabled         boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (series_id <> '' OR channel_id IS NOT NULL OR title_match <> '')
);

ALTER TABLE public.livetv_recordings
    ADD CONSTRAINT livetv_recordings_series_rule_id_fkey
    FOREIGN KEY (series_rule_id) REFERENCES public.livetv_series_rules(id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS public.livetv_recordings;
DROP TABLE IF EXISTS public.livetv_series_rules;
DROP TABLE IF EXISTS public.livetv_sessions;
DROP TABLE IF EXISTS public.livetv_programs;
DROP TABLE IF EXISTS public.livetv_guide_sources;
DROP TABLE IF EXISTS public.livetv_channels;
DROP TABLE IF EXISTS public.livetv_tuners;
-- +goose StatementEnd
