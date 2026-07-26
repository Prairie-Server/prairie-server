package artworkkey

import "testing"

func TestRevisionedArtworkKeys(t *testing.T) {
	base := "tmdb/movies/550/poster"
	original := Original(base, "abc123", ".webp")
	if original != base+"/original.abc123.webp" {
		t.Fatalf("Original() = %q", original)
	}
	if got := Variant(original, "w500"); got != base+"/w500.abc123.webp" {
		t.Fatalf("Variant() = %q", got)
	}
	if got := Revision(original); got != "abc123" {
		t.Fatalf("Revision() = %q", got)
	}
	if got := Directory(original); got != base+"/" {
		t.Fatalf("Directory() = %q", got)
	}
}

func TestLegacyArtworkKeysRemainSupported(t *testing.T) {
	original := "tmdb/movies/550/poster/original.webp"
	if got := Variant(original, "w300"); got != "tmdb/movies/550/poster/w300.webp" {
		t.Fatalf("Variant() = %q", got)
	}
	if got := Revision(original); got != "" {
		t.Fatalf("Revision() = %q, want empty", got)
	}
}

func TestVariantOnlyRewritesOriginalFilename(t *testing.T) {
	original := "tmdb/movies/original.segment/550/poster/original.abc123.webp"
	want := "tmdb/movies/original.segment/550/poster/w500.abc123.webp"
	if got := Variant(original, "w500"); got != want {
		t.Fatalf("Variant() = %q, want %q", got, want)
	}
}

func TestFormatSiblingAndObjectKeysIncludeAVIF(t *testing.T) {
	original := "tmdb/movies/550/poster/original.abc123.webp"
	if got := WebPAVIFSibling(original); got != "tmdb/movies/550/poster/original.abc123.avif" {
		t.Fatalf("WebPAVIFSibling() = %q", got)
	}
	url := "https://cdn.example/tmdb/movies/550/poster/original.abc123.webp?token=1"
	wantURL := "https://cdn.example/tmdb/movies/550/poster/original.abc123.avif?token=1"
	if got := WebPAVIFSibling(url); got != wantURL {
		t.Fatalf("WebPAVIFSibling(url) = %q, want %q", got, wantURL)
	}
	if got := WebPAVIFSibling("tmdb/movies/550/poster/original.jpg"); got != "" {
		t.Fatalf("WebPAVIFSibling(jpeg) = %q, want empty", got)
	}
	keys := ObjectKeys(original, "poster")
	want := map[string]bool{
		"tmdb/movies/550/poster/original.abc123.webp": true,
		"tmdb/movies/550/poster/original.abc123.avif": true,
		"tmdb/movies/550/poster/w500.abc123.webp":     true,
		"tmdb/movies/550/poster/w500.abc123.avif":     true,
		"tmdb/movies/550/poster/w300.abc123.webp":     true,
		"tmdb/movies/550/poster/w300.abc123.avif":     true,
	}
	if len(keys) != len(want) {
		t.Fatalf("ObjectKeys len = %d, want %d (%v)", len(keys), len(want), keys)
	}
	for _, key := range keys {
		if !want[key] {
			t.Fatalf("unexpected key %q", key)
		}
	}
}

func TestPrefersAVIF(t *testing.T) {
	cases := []struct {
		accept string
		want   bool
	}{
		{"", false},
		{"image/webp", false},
		{"image/avif,image/webp", true},
		{"image/avif;q=0,image/webp", false},
		{"image/webp,image/*,*/*;q=0.8", false},
		{"text/html", false},
	}
	for _, tc := range cases {
		if got := PrefersAVIF(tc.accept); got != tc.want {
			t.Fatalf("PrefersAVIF(%q) = %v, want %v", tc.accept, got, tc.want)
		}
	}
}
