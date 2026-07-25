// Package worker provides background processing workers for Prairie.
package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prairie-server/prairie-server/internal/metadata"
)

// RefreshCandidate identifies a metadata target ready for scheduled refresh.
type RefreshCandidate struct {
	TargetType string
	ContentID  string
}

// RefreshWorker finds media items that need their metadata refreshed.
// It implements the RefreshCandidateFinder interface used by the task manager's
// RefreshMetadataTask by claiming due rows from the durable refresh-debt queue.
type RefreshWorker struct {
	pool  *pgxpool.Pool
	debts *metadata.RefreshDebtRepository
}

// NewRefreshWorker creates a new RefreshWorker backed by the given database pool.
func NewRefreshWorker(pool *pgxpool.Pool) *RefreshWorker {
	return &RefreshWorker{
		pool:  pool,
		debts: metadata.NewRefreshDebtRepository(pool),
	}
}

// FindCandidates claims due durable metadata refresh debt rows.
func (w *RefreshWorker) FindCandidates(ctx context.Context, limit int) ([]RefreshCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}

	debts, err := w.debts.ClaimDue(ctx, limit)
	if err != nil {
		return nil, err
	}

	candidates := make([]RefreshCandidate, 0, len(debts))
	for _, debt := range debts {
		if debt == nil {
			continue
		}
		candidates = append(candidates, RefreshCandidate{
			TargetType: debt.TargetType,
			ContentID:  debt.ContentID,
		})
	}
	return candidates, nil
}

func (w *RefreshWorker) PruneDisabledLibraryDebt(ctx context.Context) error {
	if w == nil || w.debts == nil {
		return nil
	}
	return w.debts.PruneDisabledLibraryDebt(ctx)
}
