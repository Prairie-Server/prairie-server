package pgstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

func sectionOverridesKey(scope, libraryID string) string {
	return fmt.Sprintf("section_overrides:%s:%s", scope, libraryID)
}

func (s *PostgresUserStore) ListSectionOverrides(ctx context.Context, profileID, scope, libraryID string) ([]userstore.SectionOverride, error) {
	overrides, err := s.listAllSectionOverrides(ctx, scope, libraryID)
	if err != nil || len(overrides) == 0 {
		return nil, err
	}

	// Filter by profile.
	var result []userstore.SectionOverride
	for _, o := range overrides {
		if o.ProfileID == profileID {
			result = append(result, o)
		}
	}
	return result, nil
}

func (s *PostgresUserStore) SaveSectionOverrides(ctx context.Context, profileID, scope, libraryID string, overrides []userstore.SectionOverride) error {
	existing, err := s.listAllSectionOverrides(ctx, scope, libraryID)
	if err != nil {
		return err
	}

	// Set profile ID on all overrides.
	for i := range overrides {
		overrides[i].ProfileID = profileID
		if overrides[i].ID == "" {
			overrides[i].ID = generateUUID()
		}
		now := nowUTC()
		overrides[i].CreatedAt = now
		overrides[i].UpdatedAt = now
	}

	merged := make([]userstore.SectionOverride, 0, len(existing)+len(overrides))
	for _, override := range existing {
		if override.ProfileID != profileID {
			merged = append(merged, override)
		}
	}
	merged = append(merged, overrides...)

	// JSON marshalling preserves the full override payload, including removed.
	data, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling section overrides: %w", err)
	}

	key := sectionOverridesKey(scope, libraryID)
	return s.SetSetting(ctx, key, string(data))
}

func (s *PostgresUserStore) ResetSectionOverrides(ctx context.Context, profileID, scope, libraryID string) error {
	existing, err := s.listAllSectionOverrides(ctx, scope, libraryID)
	if err != nil {
		return err
	}

	remaining := existing[:0]
	for _, override := range existing {
		if override.ProfileID != profileID {
			remaining = append(remaining, override)
		}
	}

	key := sectionOverridesKey(scope, libraryID)
	if len(remaining) == 0 {
		return s.DeleteSetting(ctx, key)
	}

	data, err := json.Marshal(remaining)
	if err != nil {
		return fmt.Errorf("marshaling section overrides: %w", err)
	}

	return s.SetSetting(ctx, key, string(data))
}

func (s *PostgresUserStore) listAllSectionOverrides(ctx context.Context, scope, libraryID string) ([]userstore.SectionOverride, error) {
	key := sectionOverridesKey(scope, libraryID)
	val, err := s.GetSetting(ctx, key)
	if err != nil || val == "" {
		return nil, err
	}

	var overrides []userstore.SectionOverride
	if err := json.Unmarshal([]byte(val), &overrides); err != nil {
		return nil, fmt.Errorf("unmarshaling section overrides: %w", err)
	}

	return overrides, nil
}
