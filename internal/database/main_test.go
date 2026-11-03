package database

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("PRAIRIE_QUERY_ATTRIBUTION_ENABLED", "true"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
