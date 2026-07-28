-- Record the device-side transcode profiles a tuner advertises in
-- discover.json. Only the discontinued HDHomeRun EXTEND ever shipped them;
-- current models omit the field and silently ignore a ?transcode= query, so
-- the admin UI needs this to tell "unsupported" from "not configured" instead
-- of sending a parameter the tuner drops on the floor.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE public.livetv_tuners
    ADD COLUMN IF NOT EXISTS transcode_codecs text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE public.livetv_tuners
    DROP COLUMN IF EXISTS transcode_codecs;
-- +goose StatementEnd
