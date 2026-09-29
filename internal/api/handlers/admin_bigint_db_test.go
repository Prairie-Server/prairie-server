package handlers

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/envutil"
)

func TestAdminLastActivityAcceptsBigintUserFilter(t *testing.T) {
	dsn := envutil.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := &AdminHandler{pool: pool}
	var wide int64 = 3_000_000_000
	activity, err := h.loadUserLastActiveAt(t.Context(), []int{int(wide)})
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 0 {
		t.Fatalf("unexpected activity for test filter: %v", activity)
	}
}
