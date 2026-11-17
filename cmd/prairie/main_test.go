package main

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/backup/backuptest"
)

// TestMain drops the template database the backup command tests copy.
func TestMain(m *testing.M) { backuptest.Main(m) }
