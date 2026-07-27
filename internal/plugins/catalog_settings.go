package plugins

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type CatalogSettings struct {
	IncludeApprovedCommunityPlugins bool
	ApprovedCommunityPluginCount    int
	InstalledCommunityPluginCount   int
	MigratedPluginCount             int
	CommunityUpdatesPaused          bool
}

type ManagedRepositoryReconcileResult struct {
	RepositoriesCreated int
}

func (s *RepositoryStore) ReconcileManaged(ctx context.Context) (ManagedRepositoryReconcileResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ManagedRepositoryReconcileResult{}, fmt.Errorf("begin managed plugin repository reconciliation: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO server_settings (key, value)
		VALUES ($1, 'false')
		ON CONFLICT (key) DO NOTHING
	`, IncludeApprovedCommunityPluginsSetting); err != nil {
		return ManagedRepositoryReconcileResult{}, fmt.Errorf("seed approved community plugin setting: %w", err)
	}

	includeCommunity, err := readIncludeApprovedCommunityForUpdate(ctx, tx)
	if err != nil {
		return ManagedRepositoryReconcileResult{}, err
	}
	created, err := reconcileManagedRepositories(ctx, tx, includeCommunity)
	if err != nil {
		return ManagedRepositoryReconcileResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ManagedRepositoryReconcileResult{}, fmt.Errorf("commit managed plugin repository reconciliation: %w", err)
	}
	return ManagedRepositoryReconcileResult{RepositoriesCreated: created}, nil
}

func (s *RepositoryStore) GetCatalogSettings(ctx context.Context) (CatalogSettings, error) {
	includeCommunity, err := readIncludeApprovedCommunity(ctx, s.pool)
	if err != nil {
		return CatalogSettings{}, err
	}

	var installedCommunityCount int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM plugin_installations AS installation
		JOIN plugin_repositories AS repository ON repository.id = installation.repository_id
		WHERE repository.source_kind = $1
	`, RepositorySourceApprovedCommunity).Scan(&installedCommunityCount); err != nil {
		return CatalogSettings{}, fmt.Errorf("count installed approved community plugins: %w", err)
	}

	migratedPluginCount, err := readIntegerSetting(ctx, s.pool, MigratedApprovedCommunityCountSetting)
	if err != nil {
		return CatalogSettings{}, err
	}

	return CatalogSettings{
		IncludeApprovedCommunityPlugins: includeCommunity,
		ApprovedCommunityPluginCount:    len(approvedCommunityPluginIDs),
		InstalledCommunityPluginCount:   installedCommunityCount,
		MigratedPluginCount:             migratedPluginCount,
		CommunityUpdatesPaused:          !includeCommunity && installedCommunityCount > 0,
	}, nil
}

func (s *RepositoryStore) SetIncludeApprovedCommunity(ctx context.Context, include bool) (CatalogSettings, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogSettings{}, fmt.Errorf("begin approved community plugin setting update: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO server_settings (key, value)
		VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, IncludeApprovedCommunityPluginsSetting, strconv.FormatBool(include)); err != nil {
		return CatalogSettings{}, fmt.Errorf("update approved community plugin setting: %w", err)
	}

	if _, err := reconcileManagedRepositories(ctx, tx, include); err != nil {
		return CatalogSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CatalogSettings{}, fmt.Errorf("commit approved community plugin setting update: %w", err)
	}

	return s.GetCatalogSettings(ctx)
}

type catalogSettingsQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type catalogSettingsExecutor interface {
	catalogSettingsQuerier
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func readIncludeApprovedCommunity(ctx context.Context, querier catalogSettingsQuerier) (bool, error) {
	return readIncludeApprovedCommunityQuery(ctx, querier, `SELECT value FROM server_settings WHERE key = $1`)
}

func readIncludeApprovedCommunityForUpdate(ctx context.Context, querier catalogSettingsQuerier) (bool, error) {
	return readIncludeApprovedCommunityQuery(ctx, querier, `SELECT value FROM server_settings WHERE key = $1 FOR UPDATE`)
}

func readIncludeApprovedCommunityQuery(ctx context.Context, querier catalogSettingsQuerier, query string) (bool, error) {
	var value string
	err := querier.QueryRow(ctx, query, IncludeApprovedCommunityPluginsSetting).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("read approved community plugin setting: %w", err)
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, nil
	}
	return parsed, nil
}

func readIntegerSetting(ctx context.Context, querier catalogSettingsQuerier, key string) (int, error) {
	var value string
	err := querier.QueryRow(ctx, `SELECT value FROM server_settings WHERE key = $1`, key).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("read plugin setting %q: %w", key, err)
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0, nil
	}
	return parsed, nil
}

func reconcileManagedRepositories(ctx context.Context, executor catalogSettingsExecutor, includeCommunity bool) (int, error) {
	created := 0
	for _, definition := range managedRepositoryDefinitions {
		enabled := definition.Key == OfficialRepositoryManagedKey || includeCommunity
		existed, err := reconcileManagedRepository(ctx, executor, definition, enabled)
		if err != nil {
			return 0, err
		}
		if !existed {
			created++
		}
	}
	return created, nil
}

// reconcileManagedRepository upserts one managed catalog row and merges any
// Continuum/Silo URL leftovers into it. Managed rows are unique on managed_key,
// so a pre-rebrand official row still pointing at Silo-Server/silo-plugins must
// be rewritten before inserting the prairie-plugins URL.
func reconcileManagedRepository(
	ctx context.Context,
	executor catalogSettingsExecutor,
	definition managedRepositoryDefinition,
	enabled bool,
) (existed bool, err error) {
	var managedID int64
	lookupErr := executor.QueryRow(ctx, `
		SELECT id FROM plugin_repositories WHERE managed_key = $1
	`, definition.Key).Scan(&managedID)
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		return false, fmt.Errorf("lookup managed plugin repository %q: %w", definition.Key, lookupErr)
	}

	if lookupErr == nil {
		if err := mergeRepositoryURLInto(ctx, executor, managedID, definition.URL); err != nil {
			return false, fmt.Errorf("repoint managed plugin repository %q: %w", definition.Key, err)
		}
		if _, err := executor.Exec(ctx, `
			UPDATE plugin_repositories
			SET url = $2,
			    display_name = $3,
			    enabled = $4,
			    managed_key = $5,
			    source_kind = $6,
			    updated_at = NOW()
			WHERE id = $1
		`, managedID, definition.URL, definition.DisplayName, enabled, definition.Key, definition.SourceKind); err != nil {
			return false, fmt.Errorf("update managed plugin repository %q: %w", definition.Key, err)
		}
		if err := mergeLegacyCatalogURLs(ctx, executor, managedID, definition.Key); err != nil {
			return false, err
		}
		return true, nil
	}

	var exists bool
	if err := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM plugin_repositories WHERE url = $1)`, definition.URL).Scan(&exists); err != nil {
		return false, fmt.Errorf("check managed plugin repository %q: %w", definition.Key, err)
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO plugin_repositories (url, display_name, enabled, managed_key, source_kind)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (url) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			enabled = EXCLUDED.enabled,
			managed_key = EXCLUDED.managed_key,
			source_kind = EXCLUDED.source_kind,
			updated_at = NOW()
	`, definition.URL, definition.DisplayName, enabled, definition.Key, definition.SourceKind); err != nil {
		return false, fmt.Errorf("reconcile managed plugin repository %q: %w", definition.Key, err)
	}

	var repositoryID int64
	if err := executor.QueryRow(ctx, `SELECT id FROM plugin_repositories WHERE url = $1`, definition.URL).Scan(&repositoryID); err != nil {
		return false, fmt.Errorf("load managed plugin repository %q: %w", definition.Key, err)
	}
	if err := mergeLegacyCatalogURLs(ctx, executor, repositoryID, definition.Key); err != nil {
		return false, err
	}
	return exists, nil
}

func mergeRepositoryURLInto(ctx context.Context, executor catalogSettingsExecutor, targetID int64, url string) error {
	if _, err := executor.Exec(ctx, `
		UPDATE plugin_installations
		SET repository_id = $1,
		    available_version = NULL,
		    updated_at = NOW()
		WHERE repository_id IN (
		    SELECT id FROM plugin_repositories WHERE url = $2 AND id <> $1
		)
	`, targetID, url); err != nil {
		return fmt.Errorf("reassign installations for url %q: %w", url, err)
	}
	if _, err := executor.Exec(ctx, `
		DELETE FROM plugin_repositories WHERE url = $1 AND id <> $2
	`, url, targetID); err != nil {
		return fmt.Errorf("delete duplicate repository url %q: %w", url, err)
	}
	return nil
}

func mergeLegacyCatalogURLs(ctx context.Context, executor catalogSettingsExecutor, targetID int64, managedKey string) error {
	markers := legacyCatalogURLMarkersForManagedKey(managedKey)
	if len(markers) == 0 {
		return nil
	}

	for _, marker := range markers {
		if _, err := executor.Exec(ctx, `
			UPDATE plugin_installations
			SET repository_id = $1,
			    available_version = NULL,
			    updated_at = NOW()
			WHERE repository_id IN (
			    SELECT id FROM plugin_repositories
			    WHERE id <> $1 AND position(lower($2) in lower(url)) > 0
			)
		`, targetID, marker); err != nil {
			return fmt.Errorf("reassign legacy catalog installations for %q: %w", managedKey, err)
		}
		if _, err := executor.Exec(ctx, `
			DELETE FROM plugin_repositories
			WHERE id <> $1 AND position(lower($2) in lower(url)) > 0
		`, targetID, marker); err != nil {
			return fmt.Errorf("delete legacy catalog repository for %q: %w", managedKey, err)
		}
	}
	return nil
}
