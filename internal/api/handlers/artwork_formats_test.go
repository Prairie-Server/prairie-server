package handlers

import "testing"

func TestAppendArtworkFormatPathsAddsWebPSiblings(t *testing.T) {
	seen := map[string]struct{}{}
	paths := appendArtworkFormatPaths(nil, seen, "tmdb/movies/1/poster/w500.rev.webp")
	want := []string{
		"tmdb/movies/1/poster/w500.rev.webp",
		"tmdb/movies/1/poster/w500.rev.avif",
		"tmdb/movies/1/poster/w500.rev.png",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestAppendArtworkFormatPathsSkipsJPEGAndPluginPaths(t *testing.T) {
	seen := map[string]struct{}{}
	paths := appendArtworkFormatPaths(nil, seen, "/poster/w500.jpg")
	paths = appendArtworkFormatPaths(paths, seen, "plugin://tmdb/poster/x.webp")
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want only the two inputs", paths)
	}
}
