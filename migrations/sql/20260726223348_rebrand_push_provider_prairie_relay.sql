-- +goose Up
-- +goose StatementBegin
ALTER TABLE public.push_devices DROP CONSTRAINT IF EXISTS push_devices_provider_check;
UPDATE public.push_devices SET provider = 'prairie_relay' WHERE provider = 'silo_relay';
ALTER TABLE public.push_devices
  ADD CONSTRAINT push_devices_provider_check CHECK (provider IN ('prairie_relay'));

ALTER TABLE public.push_delivery_attempts DROP CONSTRAINT IF EXISTS push_delivery_attempts_provider_check;
UPDATE public.push_delivery_attempts SET provider = 'prairie_relay' WHERE provider = 'silo_relay';
ALTER TABLE public.push_delivery_attempts
  ADD CONSTRAINT push_delivery_attempts_provider_check CHECK (provider IN ('prairie_relay'));

UPDATE public.push_devices SET apns_topic = 'org.prairieserver.prairie' WHERE apns_topic = 'org.siloserver.silo';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE public.push_devices DROP CONSTRAINT IF EXISTS push_devices_provider_check;
UPDATE public.push_devices SET provider = 'silo_relay' WHERE provider = 'prairie_relay';
ALTER TABLE public.push_devices
  ADD CONSTRAINT push_devices_provider_check CHECK (provider IN ('silo_relay'));

ALTER TABLE public.push_delivery_attempts DROP CONSTRAINT IF EXISTS push_delivery_attempts_provider_check;
UPDATE public.push_delivery_attempts SET provider = 'silo_relay' WHERE provider = 'prairie_relay';
ALTER TABLE public.push_delivery_attempts
  ADD CONSTRAINT push_delivery_attempts_provider_check CHECK (provider IN ('silo_relay'));

UPDATE public.push_devices SET apns_topic = 'org.siloserver.silo' WHERE apns_topic = 'org.prairieserver.prairie';
-- +goose StatementEnd
