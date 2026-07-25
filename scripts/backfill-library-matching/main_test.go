package main

import (
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestGroupByCanonicalRoot_SeriesCollapsesSeasons(t *testing.T) {
	now := time.Now()
	skipped := []*models.SkippedMediaRoot{
		{
			MediaFolderID:  1,
			RootPath:       "/media/tv/Breaking Bad/Season 01",
			SampleFilePath: "/media/tv/Breaking Bad/Season 01/S01E01.mkv",
			FileCount:      7,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
		{
			MediaFolderID:  1,
			RootPath:       "/media/tv/Breaking Bad/Season 02",
			SampleFilePath: "/media/tv/Breaking Bad/Season 02/S02E01.mkv",
			FileCount:      13,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
	}

	groups := GroupByCanonicalRoot(skipped, "series")

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	g := groups[0]
	if g.CanonicalRoot != "/media/tv/Breaking Bad" {
		t.Errorf("canonical root = %q, want %q", g.CanonicalRoot, "/media/tv/Breaking Bad")
	}
	if g.Type != "series" {
		t.Errorf("type = %q, want %q", g.Type, "series")
	}
	if len(g.SkippedRoots) != 2 {
		t.Errorf("skipped roots = %d, want 2", len(g.SkippedRoots))
	}
}

func TestGroupByCanonicalRoot_DistinctMovies(t *testing.T) {
	now := time.Now()
	skipped := []*models.SkippedMediaRoot{
		{
			MediaFolderID:  1,
			RootPath:       "/media/movies/Arrival (2016)",
			SampleFilePath: "/media/movies/Arrival (2016)/Arrival (2016).mkv",
			FileCount:      1,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
		{
			MediaFolderID:  1,
			RootPath:       "/media/movies/Inception (2010)",
			SampleFilePath: "/media/movies/Inception (2010)/Inception (2010).mkv",
			FileCount:      1,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
	}

	groups := GroupByCanonicalRoot(skipped, "movies")

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	if groups[0].CanonicalRoot != "/media/movies/Arrival (2016)" {
		t.Errorf("group 0 root = %q, want %q", groups[0].CanonicalRoot, "/media/movies/Arrival (2016)")
	}
	if groups[1].CanonicalRoot != "/media/movies/Inception (2010)" {
		t.Errorf("group 1 root = %q, want %q", groups[1].CanonicalRoot, "/media/movies/Inception (2010)")
	}
	for i, g := range groups {
		if g.Type != "movie" {
			t.Errorf("group %d type = %q, want %q", i, g.Type, "movie")
		}
	}
}

func TestGroupByCanonicalRoot_EmptySampleFallback(t *testing.T) {
	now := time.Now()
	skipped := []*models.SkippedMediaRoot{
		{
			MediaFolderID:  1,
			RootPath:       "/media/movies/Unknown Movie",
			SampleFilePath: "", // No sample file path.
			FileCount:      1,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
	}

	groups := GroupByCanonicalRoot(skipped, "movies")

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	// With no sample file, it should still produce a group (using synthetic probe).
	g := groups[0]
	if g.CanonicalRoot == "" {
		t.Error("canonical root should not be empty")
	}
	if len(g.SkippedRoots) != 1 {
		t.Errorf("skipped roots = %d, want 1", len(g.SkippedRoots))
	}
}

func TestGroupByCanonicalRoot_PreservesOrder(t *testing.T) {
	now := time.Now()
	skipped := []*models.SkippedMediaRoot{
		{
			MediaFolderID:  1,
			RootPath:       "/media/movies/Zzz Movie (2020)",
			SampleFilePath: "/media/movies/Zzz Movie (2020)/Zzz Movie (2020).mkv",
			FileCount:      1,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
		{
			MediaFolderID:  1,
			RootPath:       "/media/movies/Aaa Movie (2019)",
			SampleFilePath: "/media/movies/Aaa Movie (2019)/Aaa Movie (2019).mkv",
			FileCount:      1,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
	}

	groups := GroupByCanonicalRoot(skipped, "movies")

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Should preserve insertion order (Zzz first, Aaa second).
	if groups[0].CanonicalRoot != "/media/movies/Zzz Movie (2020)" {
		t.Errorf("group 0 root = %q, want Zzz first", groups[0].CanonicalRoot)
	}
	if groups[1].CanonicalRoot != "/media/movies/Aaa Movie (2019)" {
		t.Errorf("group 1 root = %q, want Aaa second", groups[1].CanonicalRoot)
	}
}

func TestGroupByCanonicalRoot_EmptyInput(t *testing.T) {
	groups := GroupByCanonicalRoot(nil, "movies")
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups for nil input, got %d", len(groups))
	}

	groups = GroupByCanonicalRoot([]*models.SkippedMediaRoot{}, "movies")
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups for empty input, got %d", len(groups))
	}
}

func TestGroupByCanonicalRoot_UsesLibraryTypeForEpisodeShapedMovieFolder(t *testing.T) {
	now := time.Now()
	skipped := []*models.SkippedMediaRoot{
		{
			MediaFolderID:  1,
			RootPath:       "/media/movies/s01e03 (2020) {imdb-tt12261772} {tmdb-588077}",
			SampleFilePath: "/media/movies/s01e03 (2020) {imdb-tt12261772} {tmdb-588077}/s01e03 (2020).mkv",
			FileCount:      1,
			FirstSeenAt:    now,
			LastSeenAt:     now,
		},
	}

	tests := []struct {
		name        string
		libraryType string
		wantRoot    string
		wantType    string
	}{
		{
			name:        "movies library",
			libraryType: "movies",
			wantRoot:    "/media/movies/s01e03 (2020) {imdb-tt12261772} {tmdb-588077}",
			wantType:    "movie",
		},
		{
			name:        "mixed library",
			libraryType: "mixed",
			wantRoot:    "/media/movies/s01e03 (2020) {imdb-tt12261772} {tmdb-588077}",
			wantType:    "movie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := GroupByCanonicalRoot(skipped, tt.libraryType)
			if len(groups) != 1 {
				t.Fatalf("expected 1 group, got %d", len(groups))
			}

			group := groups[0]
			if group.CanonicalRoot != tt.wantRoot {
				t.Errorf("canonical root = %q, want %q", group.CanonicalRoot, tt.wantRoot)
			}
			if group.Type != tt.wantType {
				t.Errorf("type = %q, want %q", group.Type, tt.wantType)
			}
		})
	}
}
