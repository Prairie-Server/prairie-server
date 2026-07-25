package metadata

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestApplyDefaultSortTitle(t *testing.T) {
	t.Run("derives when empty", func(t *testing.T) {
		result := &MetadataResult{Title: "The Matrix"}
		ApplyDefaultSortTitle(result, false)
		if result.SortTitle != "Matrix, The" {
			t.Fatalf("SortTitle = %q, want %q", result.SortTitle, "Matrix, The")
		}
	})

	t.Run("skips when provider supplied sort title", func(t *testing.T) {
		result := &MetadataResult{Title: "The Matrix", SortTitle: "Matrix, The"}
		ApplyDefaultSortTitle(result, false)
		if result.SortTitle != "Matrix, The" {
			t.Fatalf("SortTitle = %q, want unchanged", result.SortTitle)
		}
	})

	t.Run("skips when title locked", func(t *testing.T) {
		result := &MetadataResult{Title: "The Matrix"}
		ApplyDefaultSortTitle(result, true)
		if result.SortTitle != "" {
			t.Fatalf("SortTitle = %q, want empty when locked", result.SortTitle)
		}
	})

	t.Run("no op for titles without articles", func(t *testing.T) {
		result := &MetadataResult{Title: "Inception"}
		ApplyDefaultSortTitle(result, false)
		if result.SortTitle != "" {
			t.Fatalf("SortTitle = %q, want empty", result.SortTitle)
		}
	})
}

func TestApplyDefaultSortTitleToLocalization(t *testing.T) {
	loc := &models.MediaItemLocalization{Title: "The Matrix"}
	ApplyDefaultSortTitleToLocalization(loc, false)
	if loc.SortTitle != "Matrix, The" {
		t.Fatalf("SortTitle = %q, want %q", loc.SortTitle, "Matrix, The")
	}

	locked := &models.MediaItemLocalization{Title: "The Matrix"}
	ApplyDefaultSortTitleToLocalization(locked, true)
	if locked.SortTitle != "" {
		t.Fatalf("SortTitle = %q, want empty when title locked", locked.SortTitle)
	}
}
