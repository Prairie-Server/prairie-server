package artworkstore_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
	"github.com/prairie-server/prairie-server/internal/artworkstore"
	"github.com/prairie-server/prairie-server/internal/imagecache"
	"github.com/prairie-server/prairie-server/internal/metadata"
)

func TestImageCacheWritesLocalWebPAVIFPNG(t *testing.T) {
	store, err := artworkstore.NewLocalStore(artworkstore.LocalConfig{
		Root:      t.TempDir(),
		URLSecret: "integration-secret",
		URLTTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	cacher := imagecache.New(store)
	result, err := cacher.CacheImageBytes(context.Background(), makeJPEG(t), metadata.CacheImageRequest{
		SourceURL:   "https://example.test/poster.jpg",
		ProviderID:  "tmdb",
		ContentType: "movies",
		ContentID:   "550",
		ImageType:   metadata.ImagePoster,
	})
	if err != nil {
		t.Fatalf("CacheImageBytes: %v", err)
	}
	if result == nil || result.OriginalPath == "" {
		t.Fatal("missing original path")
	}
	if !strings.HasSuffix(result.OriginalPath, ".webp") {
		t.Fatalf("canonical path should be webp: %s", result.OriginalPath)
	}

	// AVIF siblings are encoded off the request path, so the assertions below
	// have to wait for that work. Without this the test was reading the volume
	// before the encoder had written anything.
	cacher.WaitAVIFBackfill()

	ctx := context.Background()
	// The original stays WebP-only and PNG generation was dropped — this test
	// asserted both siblings for the original until now, which is why it failed
	// on any machine that ran it. internal/artworkstore is not in CI's gated
	// package list, so nothing caught the drift.
	exists, err := store.ObjectExists(ctx, store.Bucket(), result.OriginalPath)
	if err != nil || !exists {
		t.Fatalf("expected original %s exists=%v err=%v", result.OriginalPath, exists, err)
	}
	for _, absent := range []string{
		strings.TrimSuffix(result.OriginalPath, ".webp") + ".avif",
		strings.TrimSuffix(result.OriginalPath, ".webp") + ".png",
	} {
		if got, existsErr := store.ObjectExists(ctx, store.Bucket(), absent); got || existsErr != nil {
			t.Fatalf("expected %s to be absent, got exists=%v err=%v", absent, got, existsErr)
		}
	}
	// The display ladder is what carries AVIF. Widths come from the poster ladder.
	for _, name := range artworkkey.VariantNames(metadata.ImageTypeToString(metadata.ImagePoster)) {
		if name == artworkkey.OriginalVariant {
			continue
		}
		webpKey := artworkkey.Variant(result.OriginalPath, name)
		avifKey := artworkkey.WebPAVIFSibling(webpKey)
		for _, key := range []string{webpKey, avifKey} {
			got, existsErr := store.ObjectExists(ctx, store.Bucket(), key)
			if existsErr != nil || !got {
				t.Fatalf("expected display-ladder object %s exists=%v err=%v", key, got, existsErr)
			}
		}
	}

	url, err := store.PresignGetURL(ctx, store.Bucket(), result.OriginalPath, time.Hour)
	if err != nil {
		t.Fatalf("PresignGetURL: %v", err)
	}
	if !strings.HasPrefix(url, "/artwork/") || !strings.Contains(url, "sig=") {
		t.Fatalf("local URL = %q", url)
	}
}

func makeJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 96))
	for y := range 96 {
		for x := range 64 {
			img.SetRGBA(x, y, color.RGBA{R: 40, G: 80, B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}
