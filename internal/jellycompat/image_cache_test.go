package jellycompat

import (
	"testing"
	"time"
)

func TestImageCacheLookupSizedUsesSizeBucket(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	cache := NewImageCache(time.Hour, func() time.Time { return now })

	cache.RememberSized("item-1", "Primary", "https://example.com/small.jpg", compatCardImageSize)

	got, ok := cache.LookupSized("item-1", "Primary", "", compatCardImageSize)
	if !ok || got != "https://example.com/small.jpg" {
		t.Fatalf("LookupSized(small) = (%q, %v), want small image", got, ok)
	}

	if _, ok := cache.LookupSized("item-1", "Primary", "", "original"); ok {
		t.Fatal("LookupSized(original) unexpectedly returned small image")
	}
}

func TestImageCacheLookupSizedUsesTagWithinMatchingSizeBucket(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	cache := NewImageCache(time.Hour, func() time.Time { return now })

	smallURL := "https://example.com/small.jpg"
	largeURL := "https://example.com/large.jpg"
	cache.RememberSized("item-1", "Primary", smallURL, compatCardImageSize)
	cache.RememberSized("item-1", "Primary", largeURL, "original")

	got, ok := cache.LookupSized("item-1", "Primary", tagValue(largeURL), "original")
	if !ok || got != largeURL {
		t.Fatalf("LookupSized(tag=large, original) = (%q, %v), want large image", got, ok)
	}
}

// TestImageCacheLookupSizedReturnsHTTPPassthroughAcrossSizes locks in the
// behavior PR #28's review feedback discussed: HTTP-passthrough URLs (e.g.
// direct TMDB image URLs) are the same string regardless of requested size,
// so their tag is identical across sizes. The list path seeds these at
// compatCardImageSize, but Jellyfin-web requests them at "medium" / "original"
// using the same tag — the lookup must succeed.
func TestImageCacheLookupSizedReturnsHTTPPassthroughAcrossSizes(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	cache := NewImageCache(time.Hour, func() time.Time { return now })

	httpURL := "https://image.tmdb.org/t/p/original/poster.jpg"
	cache.RememberSized("item-1", "Primary", httpURL, compatCardImageSize)

	for _, size := range []string{compatCardImageSize, "medium", "original"} {
		got, ok := cache.LookupSized("item-1", "Primary", tagValue(httpURL), size)
		if !ok || got != httpURL {
			t.Fatalf("LookupSized(tag, size=%q) = (%q, %v), want %q", size, got, ok, httpURL)
		}
	}
}

func TestImageCacheRememberSizedUntilCapsRouteExpiry(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	cache := NewImageCache(time.Hour, func() time.Time { return now })
	urlExpiresAt := now.Add(10 * time.Minute)

	cache.RememberSizedUntil("item-1", "Primary", "https://example.com/presigned.jpg", compatCardImageSize, &urlExpiresAt)

	now = now.Add(4 * time.Minute)
	if got, ok := cache.LookupSized("item-1", "Primary", "", compatCardImageSize); !ok || got == "" {
		t.Fatalf("LookupSized before capped expiry = (%q, %v), want hit", got, ok)
	}

	now = now.Add(2 * time.Minute)
	if got, ok := cache.LookupSized("item-1", "Primary", "", compatCardImageSize); ok {
		t.Fatalf("LookupSized after capped expiry = (%q, %v), want miss", got, ok)
	}
}
