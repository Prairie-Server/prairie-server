package policy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/envutil"
)

func newPolicyStoreTest(t *testing.T, ctx context.Context) (*pgxpool.Pool, *PolicyStore) {
	t.Helper()
	dsn := envutil.Getenv("PRAIRIE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PRAIRIE_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	var tableName *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.policy_documents')::text`).Scan(&tableName); err != nil {
		t.Fatalf("check policy_documents table: %v", err)
	}
	if tableName == nil || *tableName == "" {
		t.Skip("test database has not applied policy foundation migration")
	}

	resetPolicyTables(t, ctx, pool)
	return pool, NewPolicyStore(pool)
}

func resetPolicyTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE public.policy_decisions RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate policy_decisions: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE public.policy_documents, public.policy_document_versions RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate policy documents: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.policy_generation (id, generation)
		VALUES (true, 1)
		ON CONFLICT (id)
		DO UPDATE SET generation = 1, updated_at = now()`); err != nil {
		t.Fatalf("reset policy generation: %v", err)
	}
}

func validStorePolicySource() string {
	return `package prairie_custom.scope

import rego.v1

override(base, _) := base`
}
