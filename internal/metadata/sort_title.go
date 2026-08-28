package metadata

import (
	"slices"
	"strings"

	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/titleutil"
)

// ApplyDefaultSortTitle sets result.SortTitle from result.Title when sort title
// is empty and the title field is not locked.
func ApplyDefaultSortTitle(result *MetadataResult, titleLocked bool) {
	if result == nil || titleLocked || strings.TrimSpace(result.SortTitle) != "" {
		return
	}
	if derived := titleutil.DeriveDefaultSortTitle(result.Title); derived != "" {
		result.SortTitle = derived
	}
}

// ApplyDefaultSortTitleToLocalization derives sort_title for a localization row
// when it is empty after merge and the title field is not locked.
func ApplyDefaultSortTitleToLocalization(loc *models.MediaItemLocalization, titleLocked bool) {
	if loc == nil || titleLocked || strings.TrimSpace(loc.SortTitle) != "" {
		return
	}
	if derived := titleutil.DeriveDefaultSortTitle(loc.Title); derived != "" {
		loc.SortTitle = derived
	}
}

func isFieldLocked(locked []MetadataField, field MetadataField) bool {
	return slices.Contains(locked, field)
}
