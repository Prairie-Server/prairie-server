-- +goose Up
-- +goose StatementBegin

ALTER TABLE public.api_keys
    ADD COLUMN IF NOT EXISTS api_key_hash bytea;

ALTER TABLE public.api_keys
    ADD COLUMN IF NOT EXISTS api_key_prefix text;

-- Allow key material to be cleared after backfill.
ALTER TABLE public.api_keys
    ALTER COLUMN api_key DROP NOT NULL;

-- Deterministic equality lookup for key verification.
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_api_key_hash
    ON public.api_keys USING btree (api_key_hash)
    WHERE api_key_hash IS NOT NULL;

-- Human-friendly non-secret identifier for admin UIs.
CREATE INDEX IF NOT EXISTS idx_api_keys_api_key_prefix
    ON public.api_keys USING btree (api_key_prefix);

-- +goose StatementEnd

