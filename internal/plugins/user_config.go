package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	"github.com/prairie-server/prairie-server/internal/userstore"
)

type userConfigSchemaResolver interface {
	UserConfigSchema(ctx context.Context, installationID int) ([]*pluginv1.ConfigSchema, error)
}

type UserConfigStore struct {
	provider userstore.UserStoreProvider
	schemas  userConfigSchemaResolver
}

func NewUserConfigStore(provider userstore.UserStoreProvider, schemas userConfigSchemaResolver) *UserConfigStore {
	return &UserConfigStore{
		provider: provider,
		schemas:  schemas,
	}
}

func (s *UserConfigStore) Get(ctx context.Context, userID, installationID int) (map[string]string, error) {
	store, err := s.provider.ForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user store: %w", err)
	}

	entries, err := store.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list user settings: %w", err)
	}

	prefix := userConfigPrefix(installationID)
	values := map[string]string{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Key, prefix) {
			continue
		}
		values[strings.TrimPrefix(entry.Key, prefix)] = entry.Value
	}
	return values, nil
}

func (s *UserConfigStore) Set(ctx context.Context, userID, installationID int, values map[string]string) error {
	store, err := s.provider.ForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("load user store: %w", err)
	}

	allowed, required, err := s.schemaKeys(ctx, installationID)
	if err != nil {
		return err
	}

	for key := range values {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("plugin user config key %q is not declared in schema", key)
		}
	}
	for key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("plugin user config key %q is required", key)
		}
	}

	existing, err := s.Get(ctx, userID, installationID)
	if err != nil {
		return err
	}
	for key := range existing {
		if _, ok := values[key]; ok {
			continue
		}
		if err := store.DeleteSetting(ctx, namespacedUserConfigKey(installationID, key)); err != nil {
			return fmt.Errorf("delete plugin user config key %q: %w", key, err)
		}
	}
	for key, value := range values {
		if err := store.SetSetting(ctx, namespacedUserConfigKey(installationID, key), value); err != nil {
			return fmt.Errorf("persist plugin user config key %q: %w", key, err)
		}
	}

	return nil
}

func (s *UserConfigStore) schemaKeys(
	ctx context.Context,
	installationID int,
) (map[string]struct{}, map[string]struct{}, error) {
	if s.schemas == nil {
		return nil, nil, fmt.Errorf("plugin user config schema resolver is required")
	}

	schemas, err := s.schemas.UserConfigSchema(ctx, installationID)
	if err != nil {
		return nil, nil, fmt.Errorf("load plugin user config schema for installation %d: %w", installationID, err)
	}

	allowed := make(map[string]struct{}, len(schemas))
	required := make(map[string]struct{})
	for _, schema := range schemas {
		if schema == nil || schema.GetKey() == "" {
			continue
		}
		allowed[schema.GetKey()] = struct{}{}
		if schema.GetRequired() {
			required[schema.GetKey()] = struct{}{}
		}
	}
	return allowed, required, nil
}

func userConfigPrefix(installationID int) string {
	return "plugin." + strconv.Itoa(installationID) + "."
}

func namespacedUserConfigKey(installationID int, key string) string {
	return userConfigPrefix(installationID) + key
}
