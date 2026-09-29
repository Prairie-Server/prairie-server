package database

import (
	"os"
	"strings"
	"testing"
)

func TestJellycompatServerNameDefaultsToPrairie(t *testing.T) {
	initialSchema, err := os.ReadFile("../../migrations/sql/001_schema.sql")
	if err != nil {
		t.Fatalf("read initial schema: %v", err)
	}
	if !strings.Contains(string(initialSchema), "('jellyfin_compat.server_name', 'Prairie')") {
		t.Fatal("initial schema does not seed the Jellyfin compat server name to Prairie")
	}

	migration, err := os.ReadFile("../../migrations/sql/20260929150000_rename_default_jellycompat_server_to_prairie.sql")
	if err != nil {
		t.Fatalf("read server-name migration: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(migration)), " ")
	for _, fragment := range []string{
		"SET value = 'Prairie'",
		"WHERE key = 'jellyfin_compat.server_name' AND value IN ('Silo', 'StreamApp')",
	} {
		if !strings.Contains(normalized, fragment) {
			t.Fatalf("server-name migration missing %q:\n%s", fragment, migration)
		}
	}
}
