package userdb

import (
	"context"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

// SQLiteProvider implements userstore.UserStoreProvider using the SQLite pool.
type SQLiteProvider struct {
	pool *UserDBPool
}

// NewSQLiteProvider wraps an existing UserDBPool as a UserStoreProvider.
func NewSQLiteProvider(pool *UserDBPool) *SQLiteProvider {
	return &SQLiteProvider{pool: pool}
}

// Compile-time interface check.
var _ userstore.UserStoreProvider = (*SQLiteProvider)(nil)

// ForUser returns a SQLiteUserStore for the given user.
func (p *SQLiteProvider) ForUser(ctx context.Context, userID int) (userstore.UserStore, error) {
	udb, err := p.pool.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return NewSQLiteUserStore(udb.DB), nil
}

// Close closes the underlying pool.
func (p *SQLiteProvider) Close() error {
	return p.pool.Close()
}

// Pin marks a userID as having active playback (SQLite-specific).
func (p *SQLiteProvider) Pin(userID int) {
	p.pool.Pin(userID)
}

// Unpin removes the active-playback mark (SQLite-specific).
func (p *SQLiteProvider) Unpin(userID int) {
	p.pool.Unpin(userID)
}
