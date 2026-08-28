package livetv

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
	"github.com/prairie-server/prairie-server/internal/imagecache"
	"github.com/prairie-server/prairie-server/internal/metadata"
)

const (
	ArtworkKindChannelLogo = "channel_logo"
	ArtworkKindProgram     = "program"

	artworkStatusPending = "pending"
	artworkStatusReady   = "ready"
	artworkStatusFailed  = "failed"

	// programArtworkGrace keeps guide art around briefly after the airing ends
	// so On-now / guide refreshes do not thrash the cache on every tick.
	programArtworkGrace = 6 * time.Hour
)

// ImageCacher downloads/encodes artwork into object storage.
type ImageCacher interface {
	Cache(ctx context.Context, req imagecache.CacheRequest) (*imagecache.CacheResult, error)
}

// ImageURLResolver turns a cached object path into a client-facing URL.
type ImageURLResolver interface {
	ResolveImageURL(ctx context.Context, path string, variant string) string
}

type artworkObjectDeleter interface {
	DeleteObjects(ctx context.Context, bucket string, keys []string) (int, error)
	Bucket() string
}

type artworkRow struct {
	ID         int64
	Kind       string
	SubjectID  string
	SourceURL  string
	ObjectPath string
	Status     string
	ExpiresAt  *time.Time
}

// artworkIndex is the durable index for Live TV artwork cache rows.
type artworkIndex interface {
	LookupMany(ctx context.Context, kind string, subjectIDs []string) (map[string]*artworkRow, error)
	UpsertPending(ctx context.Context, kind, subjectID, sourceURL string, expiresAt time.Time) error
	MarkReady(ctx context.Context, kind, subjectID, sourceURL, objectPath string, expiresAt time.Time) error
	MarkFailed(ctx context.Context, kind, subjectID, errText string) error
	TouchExpiry(ctx context.Context, kind, subjectID string, expiresAt time.Time) error
	ListExpired(ctx context.Context, limit int) ([]artworkRow, error)
	Delete(ctx context.Context, id int64) (bool, error)
}

// ArtworkCache lazily caches Live TV channel logos and visible programme images
// through the shared WebP/AVIF pipeline. It does not pre-cache the full EPG.
type ArtworkCache struct {
	index    artworkIndex
	cacher   ImageCacher
	resolver ImageURLResolver
	deleter  artworkObjectDeleter

	inFlight sync.Map // kind\0subjectID → struct{}
	enabled  bool
	syncKick bool // when true, cacheOne runs inline (tests)
	now      func() time.Time
}

// NewArtworkCache wires a Live TV artwork cache. nil cacher/db disables caching.
func NewArtworkCache(db *pgxpool.Pool, cacher ImageCacher, resolver ImageURLResolver) *ArtworkCache {
	var index artworkIndex
	if db != nil {
		index = newPgArtworkIndex(db)
	}
	return newArtworkCache(index, cacher, resolver)
}

func newArtworkCache(index artworkIndex, cacher ImageCacher, resolver ImageURLResolver) *ArtworkCache {
	return &ArtworkCache{
		index:    index,
		cacher:   cacher,
		resolver: resolver,
		enabled:  index != nil && cacher != nil,
		now:      time.Now,
	}
}

// SetObjectDeleter enables reaping expired programme artwork from object storage.
func (c *ArtworkCache) SetObjectDeleter(deleter artworkObjectDeleter) {
	if c != nil {
		c.deleter = deleter
	}
}

// SetEnabled toggles lazy caching (follows metadata.cache_images).
func (c *ArtworkCache) SetEnabled(enabled bool) {
	if c != nil {
		c.enabled = enabled && c.index != nil && c.cacher != nil
	}
}

// EnrichChannels rewrites logo_url to a cached WebP/AVIF URL when ready and
// kicks off a background cache for provider logos that are not yet cached.
func (c *ArtworkCache) EnrichChannels(ctx context.Context, channels []Channel) []Channel {
	if c == nil || !c.enabled || len(channels) == 0 {
		return channels
	}
	ids := make([]string, 0, len(channels))
	for _, ch := range channels {
		ids = append(ids, ch.ID)
	}
	rows, err := c.index.LookupMany(ctx, ArtworkKindChannelLogo, ids)
	if err != nil {
		slog.WarnContext(ctx, "livetv artwork: lookup channel logos failed", "component", "livetv", "error", err)
		return channels
	}
	out := make([]Channel, len(channels))
	copy(out, channels)
	for i := range out {
		src := strings.TrimSpace(out[i].LogoURL)
		if src == "" {
			continue
		}
		row := rows[out[i].ID]
		if row != nil && row.Status == artworkStatusReady && row.ObjectPath != "" &&
			(row.SourceURL == "" || row.SourceURL == src) {
			if url := c.resolve(ctx, row.ObjectPath); url != "" {
				out[i].LogoURL = url
				continue
			}
		}
		c.kick(ArtworkKindChannelLogo, out[i].ID, src, time.Time{})
	}
	return out
}

// EnrichPrograms rewrites image_url when cached and lazily caches images for
// programmes still on-air or upcoming in the requested guide window.
func (c *ArtworkCache) EnrichPrograms(ctx context.Context, programs []Program) []Program {
	if c == nil || !c.enabled || len(programs) == 0 {
		return programs
	}
	now := c.now().UTC()
	ids := make([]string, 0, len(programs))
	for _, p := range programs {
		ids = append(ids, p.ID)
	}
	rows, err := c.index.LookupMany(ctx, ArtworkKindProgram, ids)
	if err != nil {
		slog.WarnContext(ctx, "livetv artwork: lookup program images failed", "component", "livetv", "error", err)
		return programs
	}
	out := make([]Program, len(programs))
	copy(out, programs)
	for i := range out {
		src := strings.TrimSpace(out[i].ImageURL)
		if src == "" {
			continue
		}
		expires := out[i].Stop.UTC().Add(programArtworkGrace)
		row := rows[out[i].ID]
		if row != nil && row.Status == artworkStatusReady && row.ObjectPath != "" &&
			(row.SourceURL == "" || row.SourceURL == src) {
			if url := c.resolve(ctx, row.ObjectPath); url != "" {
				out[i].ImageURL = url
				if expires.After(now) {
					_ = c.index.TouchExpiry(ctx, ArtworkKindProgram, out[i].ID, expires)
				}
				continue
			}
		}
		if expires.Before(now) {
			continue
		}
		c.kick(ArtworkKindProgram, out[i].ID, src, expires)
	}
	return out
}

// ReapExpired deletes expired programme artwork objects and index rows.
func (c *ArtworkCache) ReapExpired(ctx context.Context, limit int) (int, error) {
	if c == nil || c.index == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	batch, err := c.index.ListExpired(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("livetv artwork reap query: %w", err)
	}
	reaped := 0
	for _, d := range batch {
		if err := c.deleteObjects(ctx, d.ObjectPath); err != nil {
			slog.WarnContext(ctx, "livetv artwork: delete objects failed",
				"component", "livetv", "path", d.ObjectPath, "error", err)
		}
		ok, err := c.index.Delete(ctx, d.ID)
		if err != nil {
			return reaped, fmt.Errorf("livetv artwork reap delete: %w", err)
		}
		if ok {
			reaped++
		}
	}
	return reaped, nil
}

func (c *ArtworkCache) resolve(ctx context.Context, objectPath string) string {
	if c.resolver == nil {
		return ""
	}
	preferred := artworkkey.Variant(objectPath, "w500")
	if preferred == "" || preferred == objectPath {
		preferred = objectPath
	}
	if url := c.resolver.ResolveImageURL(ctx, preferred, "card"); url != "" {
		return url
	}
	return c.resolver.ResolveImageURL(ctx, objectPath, "original")
}

func (c *ArtworkCache) kick(kind, subjectID, sourceURL string, expiresAt time.Time) {
	if !c.enabled || sourceURL == "" || subjectID == "" {
		return
	}
	key := kind + "\x00" + subjectID
	if _, loaded := c.inFlight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	run := func() {
		defer c.inFlight.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := c.cacheOne(ctx, kind, subjectID, sourceURL, expiresAt); err != nil {
			slog.WarnContext(ctx, "livetv artwork: cache failed",
				"component", "livetv", "kind", kind, "subject_id", subjectID, "error", err)
		}
	}
	if c.syncKick {
		run()
		return
	}
	go run()
}

func (c *ArtworkCache) cacheOne(ctx context.Context, kind, subjectID, sourceURL string, expiresAt time.Time) error {
	if err := c.index.UpsertPending(ctx, kind, subjectID, sourceURL, expiresAt); err != nil {
		return err
	}
	imageType := metadata.ImagePoster
	contentType := "programs"
	if kind == ArtworkKindChannelLogo {
		imageType = metadata.ImageLogo
		contentType = "channels"
	}
	result, err := c.cacher.Cache(ctx, imagecache.CacheRequest{
		SourceURL:   sourceURL,
		ProviderID:  "livetv",
		ContentType: contentType,
		ContentID:   subjectID,
		ImageType:   imageType,
	})
	if err != nil {
		_ = c.index.MarkFailed(ctx, kind, subjectID, err.Error())
		return err
	}
	return c.index.MarkReady(ctx, kind, subjectID, sourceURL, result.OriginalPath, expiresAt)
}

func (c *ArtworkCache) deleteObjects(ctx context.Context, objectPath string) error {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" || c.deleter == nil {
		return nil
	}
	imageType := artworkkey.ImageTypeFromPath(objectPath)
	keys := artworkkey.ObjectKeys(objectPath, imageType)
	if len(keys) == 0 {
		keys = []string{objectPath}
	}
	_, err := c.deleter.DeleteObjects(ctx, c.deleter.Bucket(), keys)
	return err
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
