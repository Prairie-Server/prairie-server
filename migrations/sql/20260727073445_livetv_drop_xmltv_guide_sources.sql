-- +goose Up
-- XMLTV URL guide sources are removed in favor of Schedules Direct.
DELETE FROM public.livetv_guide_sources WHERE type = 'xmltv_url';

COMMENT ON COLUMN public.livetv_guide_sources.type IS 'schedules_direct';

-- +goose Down
COMMENT ON COLUMN public.livetv_guide_sources.type IS 'schedules_direct | xmltv_url';
