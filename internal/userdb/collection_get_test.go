package userdb

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/userstore/storetest"
)

func TestSQLiteGetMissingCollection(t *testing.T) {
	storetest.RunGetMissingCollection(t, newConformanceStore(t))
}
