package notifications

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

func (s *interestTrackingStore) ProgressSnapshotDatabase() (*pgxpool.Pool, int) {
	if source, ok := s.UserStore.(userstore.ProgressSnapshotSource); ok {
		return source.ProgressSnapshotDatabase()
	}
	return nil, 0
}
