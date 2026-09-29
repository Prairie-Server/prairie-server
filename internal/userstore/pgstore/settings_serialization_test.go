package pgstore

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/userstore"
	"github.com/prairie-server/prairie-server/internal/userstore/storetest"
)

func TestPostgresSettingsWriterSerialization(t *testing.T) {
	storetest.RunSettingsWriterSerialization(t, func(t *testing.T) userstore.UserStore {
		pool, userID := newConstraintTestUser(t)
		return newStore(pool, userID)
	})
}

func TestPostgresPreferenceSnapshot(t *testing.T) {
	storetest.RunPreferenceSnapshot(t, func(t *testing.T) userstore.UserStore {
		pool, userID := newConstraintTestUser(t)
		return newStore(pool, userID)
	})
}
