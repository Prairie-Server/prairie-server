package plugins

import (
	"os"
	"strings"
	"testing"
)

func TestRepointLegacyPluginCatalogsMigration(t *testing.T) {
	data, err := os.ReadFile("../../migrations/sql/20260727001309_repoint_legacy_plugin_catalogs_and_repair_archives.sql")
	if err != nil {
		t.Fatalf("read repoint migration: %v", err)
	}
	sql := string(data)

	required := []string{
		"https://raw.githubusercontent.com/prairie-server/prairie-plugins/main/manifest.json",
		"https://raw.githubusercontent.com/Prairie-Community/prairie-plugins/main/manifest.json",
		"ContinuumApp/continuum-plugins",
		"Silo-Server/silo-plugins",
		"Silo-Community/silo-plugins",
		`"prairie_api_version"`,
		`"plugin_id": "silo.`,
		"version = '0.0.0'",
		"update_policy",
		"DELETE FROM public.plugin_archives",
		"prairie.tmdb",
		"prairie.tvdb",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}

	for _, dependentTable := range []string{
		"plugin_runtime_configs",
		"plugin_task_bindings",
		"plugin_auth_bindings",
		"plugin_capabilities",
	} {
		if strings.Contains(sql, "UPDATE public."+dependentTable) || strings.Contains(sql, "DELETE FROM public."+dependentTable) {
			t.Fatalf("migration must preserve installation-owned data in %s", dependentTable)
		}
	}
}
