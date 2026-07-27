package livetv

import (
	"context"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/imagecache"
	"github.com/prairie-server/prairie-server/internal/metadata"
)

type stubImageCacher struct {
	calls []imagecache.CacheRequest
}

func (s *stubImageCacher) Cache(_ context.Context, req imagecache.CacheRequest) (*imagecache.CacheResult, error) {
	s.calls = append(s.calls, req)
	return &imagecache.CacheResult{
		OriginalPath: "livetv/" + req.ContentType + "/" + req.ContentID + "/poster/original.abc.webp",
		Ext:          ".webp",
	}, nil
}

type stubResolver struct{}

func (stubResolver) ResolveImageURL(_ context.Context, path string, _ string) string {
	if path == "" {
		return ""
	}
	return "https://artwork.example/" + path
}

func TestArtworkCacheEnrichFallsThroughUntilReady(t *testing.T) {
	// Without a DB the cache is disabled — provider URLs must pass through.
	c := NewArtworkCache(nil, &stubImageCacher{}, stubResolver{})
	channels := []Channel{{ID: "ch1", LogoURL: "https://cdn.example/logo.png"}}
	out := c.EnrichChannels(context.Background(), channels)
	if out[0].LogoURL != "https://cdn.example/logo.png" {
		t.Fatalf("logo_url = %q, want provider URL", out[0].LogoURL)
	}
}

func TestArtworkCacheDisabledSkipsPrograms(t *testing.T) {
	c := NewArtworkCache(nil, &stubImageCacher{}, stubResolver{})
	now := time.Now().UTC()
	programs := []Program{{
		ID:       "p1",
		ImageURL: "https://cdn.example/show.jpg",
		Start:    now.Add(-10 * time.Minute),
		Stop:     now.Add(50 * time.Minute),
	}}
	out := c.EnrichPrograms(context.Background(), programs)
	if out[0].ImageURL != "https://cdn.example/show.jpg" {
		t.Fatalf("image_url = %q", out[0].ImageURL)
	}
}

func TestArtworkCacheRequestShape(t *testing.T) {
	// Document the livetv/ target path shape used by CacheRequest.
	req := imagecache.CacheRequest{
		SourceURL:   "https://cdn.example/logo.png",
		ProviderID:  "livetv",
		ContentType: "channels",
		ContentID:   "ch1",
		ImageType:   metadata.ImageLogo,
	}
	if req.ProviderID != "livetv" || req.ContentType != "channels" {
		t.Fatalf("unexpected request %+v", req)
	}
	reqProg := imagecache.CacheRequest{
		ProviderID:  "livetv",
		ContentType: "programs",
		ContentID:   "p1",
		ImageType:   metadata.ImagePoster,
	}
	if reqProg.ContentType != "programs" || reqProg.ImageType != metadata.ImagePoster {
		t.Fatalf("unexpected program request %+v", reqProg)
	}
}
