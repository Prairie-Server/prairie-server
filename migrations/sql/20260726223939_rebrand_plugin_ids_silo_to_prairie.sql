-- +goose Up
-- +goose StatementBegin
-- Rebrand first-party plugin_id values from silo.* to prairie.*.
UPDATE public.plugin_installations
SET plugin_id = regexp_replace(plugin_id, '^silo\.', 'prairie.')
WHERE plugin_id LIKE 'silo.%';

UPDATE public.autoscan_sources
SET plugin_id = regexp_replace(plugin_id, '^silo\.', 'prairie.')
WHERE plugin_id LIKE 'silo.%';

UPDATE public.autoscan_events
SET plugin_id = regexp_replace(plugin_id, '^silo\.', 'prairie.')
WHERE plugin_id LIKE 'silo.%';

-- Alias backfill rows used a synthetic silo.backfill provider id.
UPDATE public.media_item_aliases
SET provider = regexp_replace(provider, '^silo\.', 'prairie.')
WHERE provider LIKE 'silo.%';

-- Custom policy documents and decision logs embed silo_/silo. package names.
UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'package silo_custom.', 'package prairie_custom.')
WHERE rego_source LIKE '%package silo_custom.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'data.silo_custom.', 'data.prairie_custom.')
WHERE rego_source LIKE '%data.silo_custom.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'package silo.', 'package prairie.')
WHERE rego_source LIKE '%package silo.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'data.silo.', 'data.prairie.')
WHERE rego_source LIKE '%data.silo.%';

UPDATE public.policy_decisions
SET decision_name = regexp_replace(decision_name, '^silo\.', 'prairie.')
WHERE decision_name LIKE 'silo.%';

UPDATE public.plugin_repositories
SET source_kind = 'prairie'
WHERE source_kind = 'silo';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE public.plugin_installations
SET plugin_id = regexp_replace(plugin_id, '^prairie\.', 'silo.')
WHERE plugin_id LIKE 'prairie.%';

UPDATE public.autoscan_sources
SET plugin_id = regexp_replace(plugin_id, '^prairie\.', 'silo.')
WHERE plugin_id LIKE 'prairie.%';

UPDATE public.autoscan_events
SET plugin_id = regexp_replace(plugin_id, '^prairie\.', 'silo.')
WHERE plugin_id LIKE 'prairie.%';

UPDATE public.media_item_aliases
SET provider = regexp_replace(provider, '^prairie\.', 'silo.')
WHERE provider LIKE 'prairie.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'package prairie_custom.', 'package silo_custom.')
WHERE rego_source LIKE '%package prairie_custom.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'data.prairie_custom.', 'data.silo_custom.')
WHERE rego_source LIKE '%data.prairie_custom.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'package prairie.', 'package silo.')
WHERE rego_source LIKE '%package prairie.%';

UPDATE public.policy_document_versions
SET rego_source = replace(rego_source, 'data.prairie.', 'data.silo.')
WHERE rego_source LIKE '%data.prairie.%';

UPDATE public.policy_decisions
SET decision_name = regexp_replace(decision_name, '^prairie\.', 'silo.')
WHERE decision_name LIKE 'prairie.%';

-- source_kind 'prairie' is also the post-rebrand canonical value for new
-- installs, so do not reverse it on Down.
-- +goose StatementEnd
