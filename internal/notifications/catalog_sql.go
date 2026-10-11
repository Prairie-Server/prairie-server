package notifications

import "github.com/prairie-server/prairie-server/internal/userstore"

func (s *interestTrackingStore) CatalogStateInPostgres() bool {
	return userstore.HasCatalogSQLState(s.UserStore)
}
