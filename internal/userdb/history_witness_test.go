package userdb

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/userstore/storetest"
)

func TestSQLiteHistoryWitness(t *testing.T) { storetest.RunHistoryWitness(t, newConformanceStore) }
