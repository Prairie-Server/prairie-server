// Command backfill-sort-titles populates media_items.sort_title for movies and
// series that have an empty sort title and a title beginning with a leading
// English article (The, A, An).
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./scripts/backfill-sort-titles -dry-run=true
//	DATABASE_URL=postgres://... go run ./scripts/backfill-sort-titles -dry-run=false
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/titleutil"
)

const defaultBatchSize = 500

var (
	dryRun    = flag.Bool("dry-run", true, "report planned changes without writing")
	limit     = flag.Int("limit", 0, "maximum number of rows to update (0 = no limit)")
	batchSize = flag.Int("batch-size", defaultBatchSize, "rows per update batch")
)

type candidate struct {
	contentID string
	title     string
}

func main() {
	flag.Parse()

	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("backfill-sort-titles: DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("backfill-sort-titles: failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("backfill-sort-titles: failed to ping database", "error", err)
		os.Exit(1)
	}

	rows, err := pool.Query(ctx, `
		SELECT content_id, title
		FROM media_items
		WHERE type IN ('movie', 'series')
		  AND BTRIM(COALESCE(sort_title, '')) = ''
		  AND BTRIM(COALESCE(title, '')) <> ''
		ORDER BY content_id
	`)
	if err != nil {
		slog.Error("backfill-sort-titles: failed to query candidates", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	var candidates []candidate
	for rows.Next() {
		var row candidate
		if err := rows.Scan(&row.contentID, &row.title); err != nil {
			slog.Error("backfill-sort-titles: failed to scan row", "error", err)
			os.Exit(1)
		}
		if derived := titleutil.DeriveDefaultSortTitle(row.title); derived != "" {
			candidates = append(candidates, candidate{contentID: row.contentID, title: row.title})
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("backfill-sort-titles: row iteration failed", "error", err)
		os.Exit(1)
	}

	if *limit > 0 && len(candidates) > *limit {
		candidates = candidates[:*limit]
	}

	slog.Info("backfill-sort-titles: candidates loaded", "count", len(candidates), "dry_run", *dryRun)

	if len(candidates) == 0 {
		return
	}

	for i, row := range candidates {
		if i < 5 {
			slog.Info("backfill-sort-titles: sample",
				"content_id", row.contentID,
				"title", row.title,
				"sort_title", titleutil.DeriveDefaultSortTitle(row.title),
			)
		}
	}

	if *dryRun {
		slog.Info("backfill-sort-titles: dry run complete", "would_update", len(candidates))
		return
	}

	updated := 0
	for start := 0; start < len(candidates); start += *batchSize {
		end := start + *batchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]

		tx, err := pool.Begin(ctx)
		if err != nil {
			slog.Error("backfill-sort-titles: begin transaction failed", "error", err)
			os.Exit(1)
		}

		batchUpdated := 0
		for _, row := range batch {
			sortTitle := titleutil.DeriveDefaultSortTitle(row.title)
			tag, err := tx.Exec(ctx, `
				UPDATE media_items
				SET sort_title = $1, updated_at = NOW()
				WHERE content_id = $2
				  AND BTRIM(COALESCE(sort_title, '')) = ''
			`, sortTitle, row.contentID)
			if err != nil {
				_ = tx.Rollback(ctx)
				slog.Error("backfill-sort-titles: update failed", "content_id", row.contentID, "error", err)
				os.Exit(1)
			}
			batchUpdated += int(tag.RowsAffected())
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Error("backfill-sort-titles: commit failed", "error", err)
			os.Exit(1)
		}
		updated += batchUpdated
		slog.Info("backfill-sort-titles: batch committed",
			"batch_start", start,
			"batch_end", end,
			"batch_updated", batchUpdated,
		)
	}

	slog.Info("backfill-sort-titles: complete", "updated", updated)
}
