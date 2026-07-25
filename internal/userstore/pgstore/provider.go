package pgstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

// PostgresProvider implements userstore.UserStoreProvider using shared Postgres tables.
type PostgresProvider struct {
	pool *pgxpool.Pool
}

// NewPostgresProvider creates a provider backed by a pgx pool.
func NewPostgresProvider(pool *pgxpool.Pool) *PostgresProvider {
	return &PostgresProvider{pool: pool}
}

// Compile-time interface check.
var _ userstore.UserStoreProvider = (*PostgresProvider)(nil)

// ForUser returns a PostgresUserStore scoped to the given user.
// This is lightweight — no per-user connection, just a struct with the pool + userID.
func (p *PostgresProvider) ForUser(_ context.Context, userID int) (userstore.UserStore, error) {
	return newStore(p.pool, userID), nil
}

// Close is a no-op for Postgres — the pool is managed externally.
func (p *PostgresProvider) Close() error {
	return nil
}
