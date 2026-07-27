-- +goose Up
-- Native Zap2XML-style Gracenote guide sync (type xml_sync) alongside Schedules Direct.
COMMENT ON COLUMN public.livetv_guide_sources.type IS 'schedules_direct | xml_sync';

-- +goose Down
COMMENT ON COLUMN public.livetv_guide_sources.type IS 'schedules_direct';
