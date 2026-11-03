-- +goose Up
-- Prairie stored API keys only as an HMAC (api_key_hash, 20260727043000) and
-- cleared the bearer, so 20261019181724 could not derive their SHA-256
-- verifier and left key_hash NULL. The server fills key_hash and key_prefix on
-- such a key's first use, when it has the bearer; let the trigger accept that
-- one fill of a NULL key_hash and nothing else.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION advance_api_key_configuration_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.revision := nextval('api_key_configuration_revision_seq');
    ELSE
        IF ROW(NEW.id,NEW.user_id) IS DISTINCT FROM ROW(OLD.id,OLD.user_id)
           OR (OLD.key_hash IS NOT NULL
               AND ROW(NEW.key_hash,NEW.key_prefix) IS DISTINCT FROM ROW(OLD.key_hash,OLD.key_prefix)) THEN
            RAISE EXCEPTION 'API key identity, owner, and credential are immutable' USING ERRCODE='23514';
        END IF;
        IF ROW(NEW.label,NEW.rate_tier,COALESCE(NEW.scopes,'{}'::text[]),NEW.created_at)
           IS DISTINCT FROM ROW(OLD.label,OLD.rate_tier,COALESCE(OLD.scopes,'{}'::text[]),OLD.created_at) THEN
            NEW.revision := nextval('api_key_configuration_revision_seq');
        ELSE
            NEW.revision := OLD.revision;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION advance_api_key_configuration_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.revision := nextval('api_key_configuration_revision_seq');
    ELSE
        IF ROW(NEW.id,NEW.user_id,NEW.key_hash,NEW.key_prefix) IS DISTINCT FROM ROW(OLD.id,OLD.user_id,OLD.key_hash,OLD.key_prefix) THEN
            RAISE EXCEPTION 'API key identity, owner, and credential are immutable' USING ERRCODE='23514';
        END IF;
        IF ROW(NEW.label,NEW.rate_tier,COALESCE(NEW.scopes,'{}'::text[]),NEW.created_at)
           IS DISTINCT FROM ROW(OLD.label,OLD.rate_tier,COALESCE(OLD.scopes,'{}'::text[]),OLD.created_at) THEN
            NEW.revision := nextval('api_key_configuration_revision_seq');
        ELSE
            NEW.revision := OLD.revision;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
