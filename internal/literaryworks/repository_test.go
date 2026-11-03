package literaryworks

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/envutil"
)

func newLiteraryWorksTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := envutil.Getenv("PRAIRIE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PRAIRIE_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	var tableName *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.literary_works')::text`).Scan(&tableName); err != nil {
		t.Fatalf("check literary_works table: %v", err)
	}
	if tableName == nil || *tableName == "" {
		t.Skip("test database has not applied literary works migration")
	}
	return pool
}
