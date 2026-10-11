package metadata

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestFindContentIDPrefersRequestedProviderButFallsBack(t *testing.T) {
	item := &models.MediaItem{
		ContentID: "content-1",
		TvdbID:    "tvdb-1",
		ImdbID:    "tt1234567",
	}

	if got := findContentID(item, "tmdb"); got != "tvdb-1" {
		t.Fatalf("findContentID(tmdb) = %q, want tvdb fallback", got)
	}

	if got := findContentID(item, "imdb"); got != "tt1234567" {
		t.Fatalf("findContentID(imdb) = %q, want imdb id", got)
	}

	if got := findContentID(item, "unknown"); got != "tvdb-1" {
		t.Fatalf("findContentID(unknown) = %q, want best available external id", got)
	}
}

func TestFindContentIDFallsBackToContentID(t *testing.T) {
	item := &models.MediaItem{
		ContentID: "content-1",
	}

	if got := findContentID(item, "tmdb"); got != "content-1" {
		t.Fatalf("findContentID(tmdb) = %q, want content id fallback", got)
	}
}
