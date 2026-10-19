package catalog

import (
	"context"
	"testing"

	"github.com/prairie-server/prairie-server/internal/deviceclass"
	"github.com/prairie-server/prairie-server/internal/imagesize"
)

// The literals below are the variants this package returned before the
// image_size parameter existed. Routing the size hint through
// internal/imagesize must not move any of them: an absent parameter, and the
// "small"/"original" literals the existing call sites already pass, have to
// keep resolving to exactly the same object keys.
func TestCachedImageVariantKeyMatchesPreImageSizeBehavior(t *testing.T) {
	want := map[string]map[string]string{
		"": {
			"backdrop": "w1920",
			"logo":     "w500",
			"poster":   "w500",
			"still":    "w500",
			"profile":  "w500",
			"mystery":  "w500",
		},
		"small": {
			"backdrop": "w300",
			"logo":     "w500",
			"poster":   "w200",
			"still":    "w300",
			"profile":  "w200",
			"mystery":  "w200",
		},
		"medium": {
			"backdrop": "w1920",
			"logo":     "w500",
			"poster":   "w500",
			"still":    "w500",
			"profile":  "w500",
			"mystery":  "w500",
		},
		"original": {
			"backdrop": "original",
			"logo":     "original",
			"poster":   "original",
			"still":    "original",
			"profile":  "original",
			"mystery":  "original",
		},
	}

	for size, byType := range want {
		for imageType, wantVariant := range byType {
			if got := cachedImageVariantKey(imageType, size); got != wantVariant {
				t.Errorf("cachedImageVariantKey(%q, %q) = %q, want %q", imageType, size, got, wantVariant)
			}
		}
	}
}

// "large" must reach the widest cached rung short of the original. Prairie's
// ladder has no w780/w1280 rung, so posters, stills and logos stop at w500.
func TestCachedImageVariantKeyLarge(t *testing.T) {
	want := map[string]string{
		"backdrop": "w2560",
		"logo":     "w500",
		"poster":   "w500",
		"still":    "w500",
		"profile":  "w500",
	}
	for imageType, wantVariant := range want {
		if got := cachedImageVariantKey(imageType, "large"); got != wantVariant {
			t.Errorf("cachedImageVariantKey(%q, large) = %q, want %q", imageType, got, wantVariant)
		}
	}
}

// An unparseable hint must degrade to the historical default rather than
// producing an empty variant, which would leave the object key on "original".
func TestCachedImageVariantKeyUnknownSize(t *testing.T) {
	if got := cachedImageVariantKey("poster", "gigantic"); got != "w500" {
		t.Fatalf("cachedImageVariantKey(poster, gigantic) = %q, want w500", got)
	}
}

func TestSizeToVariant(t *testing.T) {
	want := map[string]string{
		"":         "featured",
		"small":    "card",
		"medium":   "featured",
		"large":    "large",
		"original": "original",
		"nonsense": "featured",
	}
	for size, wantVariant := range want {
		if got := sizeToVariant(size); got != wantVariant {
			t.Errorf("sizeToVariant(%q) = %q, want %q", size, got, wantVariant)
		}
	}
}

func TestCachedImageVariantPathRewritesKey(t *testing.T) {
	const original = "tmdb/movies/550/poster/original.abc123.webp"
	tests := map[string]string{
		"":         "tmdb/movies/550/poster/w500.abc123.webp",
		"small":    "tmdb/movies/550/poster/w200.abc123.webp",
		"medium":   "tmdb/movies/550/poster/w500.abc123.webp",
		"large":    "tmdb/movies/550/poster/w500.abc123.webp",
		"original": original,
	}
	for size, wantPath := range tests {
		if got := cachedImageVariantPath(context.Background(), original, "poster", size); got != wantPath {
			t.Errorf("cachedImageVariantPath(%q, poster, %q) = %q, want %q", original, size, got, wantPath)
		}
	}

	// The key's own image type picks the ladder: an episode still passed in a
	// poster slot resolves to still rungs, not a poster rung it never had.
	const still = "tvdb/series/73141/seasons/22/episodes/9/still/original.webp"
	if got := cachedImageVariantPath(context.Background(), still, "poster", "large"); got != "tvdb/series/73141/seasons/22/episodes/9/still/w500.webp" {
		t.Errorf("cachedImageVariantPath(context.Background(), still, poster, large) = %q, want the still ladder's w500", got)
	}
	if got := cachedImageVariantPath(context.Background(), still, "backdrop", ""); got != "tvdb/series/73141/seasons/22/episodes/9/still/w500.webp" {
		t.Errorf("cachedImageVariantPath(context.Background(), still, backdrop, \"\") = %q, want the still default w500", got)
	}

	const pluginPath = "plugin://tmdb/poster/original.webp"
	if got := cachedImageVariantPath(context.Background(), pluginPath, "poster", "large"); got != pluginPath {
		t.Errorf("cachedImageVariantPath(plugin path) = %q, want it unchanged", got)
	}
}

// A width size resolves on the key's own ladder: the narrowest rung at least
// that wide, else the original.
func TestCachedImageVariantPathForWidth(t *testing.T) {
	for _, tc := range []struct {
		path      string
		imageType string
		px        int
		want      string
	}{
		{"tmdb/movies/1/backdrop/original.r1.webp", "backdrop", 2688, "tmdb/movies/1/backdrop/original.r1.webp"},
		{"tmdb/movies/1/backdrop/original.r1.webp", "backdrop", 1344, "tmdb/movies/1/backdrop/w1920.r1.webp"},
		{"tmdb/series/1/seasons/1/episodes/2/still/original.r1.webp", "backdrop", 1344, "tmdb/series/1/seasons/1/episodes/2/still/original.r1.webp"},
		{"tmdb/series/1/seasons/1/episodes/2/still/original.r1.webp", "still", 480, "tmdb/series/1/seasons/1/episodes/2/still/w500.r1.webp"},
		{"tmdb/series/1/seasons/1/episodes/2/still/original.r1.webp", "still", 720, "tmdb/series/1/seasons/1/episodes/2/still/original.r1.webp"},
		{"tmdb/movies/1/poster/original.r1.webp", "poster", 352, "tmdb/movies/1/poster/w500.r1.webp"},
		{"tmdb/movies/1/poster/original.r1.webp", "poster", 280, "tmdb/movies/1/poster/w300.r1.webp"},
	} {
		if got := cachedImageVariantPath(context.Background(), tc.path, tc.imageType, string(imagesize.Width(tc.px))); got != tc.want {
			t.Errorf("cachedImageVariantPath(%q, %q, %dpx) = %q, want %q", tc.path, tc.imageType, tc.px, got, tc.want)
		}
	}
}

// An explicit width is a size hint, so the TV default (the w200 poster rung)
// does not override it.
func TestCachedImageVariantPathForWidthOnTV(t *testing.T) {
	ctx := deviceclass.SetContext(context.Background(), deviceclass.TV)
	const path = "tmdb/movies/1/poster/original.r1.webp"
	if got := cachedImageVariantPath(ctx, path, "poster", string(imagesize.Width(280))); got != "tmdb/movies/1/poster/w300.r1.webp" {
		t.Errorf("TV poster at 280px = %q, want w300", got)
	}
	if got := cachedImageVariantPath(ctx, path, "poster", ""); got != "tmdb/movies/1/poster/w200.r1.webp" {
		t.Errorf("TV poster without a hint = %q, want w200", got)
	}
}
