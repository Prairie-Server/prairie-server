package handlers

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/recommendations"
)

func embeddingLockJSON(t *testing.T, lock recommendations.EmbeddingLock) string {
	t.Helper()
	raw, err := lock.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
