package watchstate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

// ReconcileStats holds counters from a single reconciliation run.
type ReconcileStats struct {
	OrphansFound int
	Resolved     int
	Unresolvable int
	Errors       int
}

type historyDB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const orphanedHistoryRowsQuery = `
		SELECT h.user_id, h.id, h.media_item_id, h.watch_identity::text
		FROM user_watch_history h
		WHERE h.watch_identity <> '{}'::jsonb
		  AND (
		    (
		      h.watch_identity->>'stable_type' = 'movie'
		      AND NOT EXISTS (
		        SELECT 1 FROM media_items mi WHERE mi.content_id = h.media_item_id
		      )
		    )
		    OR (
		      h.watch_identity->>'stable_type' = 'episode'
		      AND NOT EXISTS (
		        SELECT 1 FROM episodes ep WHERE ep.content_id = h.media_item_id
		      )
		    )
		  )
		ORDER BY h.user_id, h.id
	`

const updateHistoryMediaItemQuery = `
		UPDATE user_watch_history
		SET media_item_id = $1
		WHERE user_id = $2
		  AND id = $3
		  AND media_item_id = $4
	`

// HistoryReconciler repairs orphaned watch-history rows by resolving each row's
// stored watch_identity back to a current media_item_id via the catalog.
type HistoryReconciler struct {
	pool     historyDB
	resolver *StableIdentityResolver
}

// NewHistoryReconciler creates a HistoryReconciler backed by the given pool and resolver.
func NewHistoryReconciler(pool *pgxpool.Pool, resolver *StableIdentityResolver) *HistoryReconciler {
	return &HistoryReconciler{pool: pool, resolver: resolver}
}

// Run performs one reconciliation sweep over all orphaned rows in user_watch_history.
// It respects context cancellation and returns partial stats on early exit.
func (r *HistoryReconciler) Run(ctx context.Context) (ReconcileStats, error) {
	var stats ReconcileStats

	rows, err := r.pool.Query(ctx, orphanedHistoryRowsQuery)
	if err != nil {
		return stats, fmt.Errorf("querying orphaned history rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}

		var userID int
		var id, currentItemID, identityJSON string
		if err := rows.Scan(&userID, &id, &currentItemID, &identityJSON); err != nil {
			stats.Errors++
			continue
		}
		stats.OrphansFound++

		var identity userstore.WatchIdentity
		if err := json.Unmarshal([]byte(identityJSON), &identity); err != nil || strings.TrimSpace(identity.StableType) == "" {
			stats.Unresolvable++
			continue
		}

		newID, err := r.resolve(ctx, identity)
		if err != nil {
			stats.Errors++
			continue
		}
		if newID == "" || newID == currentItemID {
			stats.Unresolvable++
			continue
		}

		tag, err := r.pool.Exec(ctx, updateHistoryMediaItemQuery, newID, userID, id, currentItemID)
		if err != nil {
			stats.Errors++
			continue
		}
		if tag.RowsAffected() == 0 {
			stats.Unresolvable++
			continue
		}
		stats.Resolved++
	}
	if err := rows.Err(); err != nil && ctx.Err() == nil {
		return stats, fmt.Errorf("iterating orphaned history rows: %w", err)
	}
	return stats, ctx.Err()
}

func (r *HistoryReconciler) resolve(ctx context.Context, identity userstore.WatchIdentity) (string, error) {
	switch identity.StableType {
	case "movie":
		if len(identity.ProviderIDs) == 0 {
			return "", nil
		}
		return r.resolver.ResolveMovieContentID(ctx, identity.ProviderIDs)
	case "episode":
		seriesProviderIDs := episodeSeriesProviderIDs(identity)
		if len(seriesProviderIDs) == 0 || identity.Season == nil || identity.Episode == nil {
			return "", nil
		}
		return r.resolver.ResolveEpisodeContentID(ctx, seriesProviderIDs, *identity.Season, *identity.Episode)
	default:
		return "", nil
	}
}

func episodeSeriesProviderIDs(identity userstore.WatchIdentity) map[string]string {
	if len(identity.SeriesProviderIDs) > 0 {
		return identity.SeriesProviderIDs
	}
	return identity.ProviderIDs
}
