package userdb

import (
	"database/sql"
	"fmt"
	"sync"

	"modernc.org/sqlite"
)

func init() {
	// Register modernc under the legacy "sqlite3" name so existing
	// sql.Open("sqlite3", ...) call sites (and tests) keep working while
	// the server builds with CGO_ENABLED=0. mattn/go-sqlite3 is a stub
	// under !cgo and cannot open databases.
	sql.Register("sqlite3", &sqlite.Driver{})
}

// UserDB wraps a per-user SQLite database connection with WAL mode and
// appropriate PRAGMAs for concurrent access and Litestream compatibility.
type UserDB struct {
	DB     *sql.DB
	Path   string
	UserID int
	mu     sync.RWMutex
	dirty  bool // tracks if data changed since last reconciliation
}

// NewUserDB opens (or creates) a SQLite database at the given path with
// WAL mode and recommended PRAGMAs.
func NewUserDB(path string, userID int) (*UserDB, error) {
	// modernc.org/sqlite uses `_pragma=` query params (not mattn's `_journal_mode=`).
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)",
		path,
	)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}

	// Verify connection and PRAGMAs.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite %s: %w", path, err)
	}

	// Initialize schema.
	if err := InitSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing schema %s: %w", path, err)
	}
	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating schema %s: %w", path, err)
	}

	return &UserDB{
		DB:     db,
		Path:   path,
		UserID: userID,
	}, nil
}

// Close closes the underlying SQLite database connection.
func (u *UserDB) Close() error {
	return u.DB.Close()
}

// MarkDirty flags the database as having unsyncronized changes.
func (u *UserDB) MarkDirty() {
	u.mu.Lock()
	u.dirty = true
	u.mu.Unlock()
}

// ClearDirty resets the dirty flag after reconciliation.
func (u *UserDB) ClearDirty() {
	u.mu.Lock()
	u.dirty = false
	u.mu.Unlock()
}

// IsDirty returns whether the database has unsynced changes.
func (u *UserDB) IsDirty() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.dirty
}
