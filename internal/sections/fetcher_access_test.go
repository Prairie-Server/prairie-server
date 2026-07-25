package sections

import (
	"slices"
	"testing"

	"github.com/prairie-server/prairie-server/internal/catalog"
)

func TestEffectiveFetchLibraryIDsUsesAllowedLibraries(t *testing.T) {
	filter := catalog.AccessFilter{AllowedLibraryIDs: []int{7, 8}}

	got := effectiveFetchLibraryIDs(nil, filter)

	if !slices.Equal(got, []int{7, 8}) {
		t.Fatalf("effective library IDs = %#v, want [7 8]", got)
	}
}

func TestEffectiveFetchLibraryIDsKeepsExplicitScope(t *testing.T) {
	filter := catalog.AccessFilter{AllowedLibraryIDs: []int{7, 8}}

	got := effectiveFetchLibraryIDs([]int{3}, filter)

	if !slices.Equal(got, []int{3}) {
		t.Fatalf("effective library IDs = %#v, want [3]", got)
	}
}

func TestEffectiveFetchLibraryIDsPreservesEmptyAllowedScope(t *testing.T) {
	filter := catalog.AccessFilter{AllowedLibraryIDs: []int{}}

	got := effectiveFetchLibraryIDs(nil, filter)

	if got == nil {
		t.Fatalf("effective library IDs = nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("effective library IDs = %#v, want empty slice", got)
	}
}
