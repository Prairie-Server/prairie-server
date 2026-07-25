package plugins

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgUserThemeLookup reads the user's `ui_theme` setting from
// public.user_settings. This is the same row Prairie's web client reads
// via the /profile/settings API; surfacing it here lets the plugin proxy
// stamp X-Prairie-Theme on every plugin request.
type PgUserThemeLookup struct {
	pool *pgxpool.Pool
}

func NewPgUserThemeLookup(pool *pgxpool.Pool) *PgUserThemeLookup {
	return &PgUserThemeLookup{pool: pool}
}

func (l *PgUserThemeLookup) LookupUITheme(ctx context.Context, userID int) (string, error) {
	if l == nil || l.pool == nil || userID <= 0 {
		return "", nil
	}
	var value string
	err := l.pool.QueryRow(ctx,
		"SELECT value FROM user_settings WHERE user_id = $1 AND key = 'ui_theme'",
		userID,
	).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}
