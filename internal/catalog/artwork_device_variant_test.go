package catalog

import (
	"context"
	"testing"

	"github.com/prairie-server/prairie-server/internal/deviceclass"
)

func tvCtx() context.Context {
	return deviceclass.SetContext(context.Background(), deviceclass.TV)
}

func TestCachedImageVariantKeyForTV(t *testing.T) {
	ctx := tvCtx()
	for imageType, want := range map[string]string{
		"poster":   "w200",
		"profile":  "w200",
		"backdrop": "w1280",
		// Episode cards render ~358 px on a UHD chrome scale; w300 only just
		// covers that and w200 would upscale.
		"still": "w500",
		// w500 is the only logo rung generated.
		"logo": "w500",
	} {
		if got := cachedImageVariantKeyFor(ctx, imageType, ""); got != want {
			t.Errorf("TV %s = %q, want %q", imageType, got, want)
		}
	}
}

// Anything that does not identify as a TV must resolve exactly as it did before
// device awareness existed.
func TestCachedImageVariantKeyForNonTVUnchanged(t *testing.T) {
	for _, ctx := range []context.Context{
		context.Background(),
		deviceclass.SetContext(context.Background(), deviceclass.Unknown),
	} {
		for _, imageType := range []string{"poster", "profile", "backdrop", "still", "logo", "banner"} {
			for _, size := range []string{"", "small", "original", "medium"} {
				want := cachedImageVariantKey(imageType, size)
				if got := cachedImageVariantKeyFor(ctx, imageType, size); got != want {
					t.Errorf("non-TV %s/%q = %q, want %q", imageType, size, got, want)
				}
			}
		}
	}
}

// An explicit size encodes a display context the caller knows and the device
// does not — a rail thumbnail, or a download that needs the source image.
func TestExplicitSizeWinsOverDeviceClass(t *testing.T) {
	ctx := tvCtx()
	for _, tc := range []struct{ imageType, size, want string }{
		{"poster", "small", "w300"},
		{"poster", "original", "original"},
		{"backdrop", "small", "w300"},
		{"backdrop", "original", "original"},
		{"profile", "original", "original"},
	} {
		if got := cachedImageVariantKeyFor(ctx, tc.imageType, tc.size); got != tc.want {
			t.Errorf("TV %s/%q = %q, want %q", tc.imageType, tc.size, got, tc.want)
		}
	}
}

func TestCachedImageVariantPathRewritesForTV(t *testing.T) {
	const original = "tmdb/movies/550/poster/original.rev.webp"

	if got := cachedImageVariantPath(tvCtx(), original, "poster", ""); got != "tmdb/movies/550/poster/w200.rev.webp" {
		t.Errorf("TV poster path = %q", got)
	}
	if got := cachedImageVariantPath(context.Background(), original, "poster", ""); got != "tmdb/movies/550/poster/w500.rev.webp" {
		t.Errorf("desktop poster path = %q, want the unchanged w500 rung", got)
	}
}

// Remote URLs are not ours to resize.
func TestCachedImageVariantPathLeavesURLsAlone(t *testing.T) {
	const remote = "https://image.tmdb.org/t/p/original/abc.jpg"
	if got := cachedImageVariantPath(tvCtx(), remote, "poster", ""); got != remote {
		t.Errorf("remote URL = %q, want it untouched", got)
	}
}
