package livetv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
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

// ArtworkCache lazily caches Live TV channel logos and visible programme images
// through the shared WebP/AVIF pipeline. It does not pre-cache the full EPG.
type ArtworkCache struct {
	db       *pgxpool.Pool
	cacher   ImageCacher
	resolver ImageURLResolver
	deleter  artworkObjectDeleter

	inFlight sync.Map // kind\0subjectID → struct{}
	enabled  bool
	now      func() time.Time
}

// NewArtworkCache wires a Live TV artwork cache. nil cacher disables caching.
func NewArtworkCache(db *pgxpool.Pool, cacher ImageCacher, resolver ImageURLResolver) *ArtworkCache {
	return &ArtworkCache{
		db:       db,
		cacher:   cacher,
		resolver: resolver,
		enabled:  db != nil && cacher != nil,
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
		c.enabled = enabled && c.db != nil && c.cacher != nil
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
	rows, err := c.lookupMany(ctx, ArtworkKindChannelLogo, ids)
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
	rows, err := c.lookupMany(ctx, ArtworkKindProgram, ids)
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
				// Keep TTL fresh while the programme remains visible.
				if expires.After(now) {
					_ = c.touchExpiry(ctx, ArtworkKindProgram, out[i].ID, expires)
				}
				continue
			}
		}
		// Only cache programmes that have not already expired past grace.
		if expires.Before(now) {
			continue
		}
		c.kick(ArtworkKindProgram, out[i].ID, src, expires)
	}
	return out
}

// ReapExpired deletes expired programme artwork objects and index rows.
func (c *ArtworkCache) ReapExpired(ctx context.Context, limit int) (int, error) {
	if c == nil || c.db == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := c.db.Query(ctx, `
		SELECT id, kind, subject_id, object_path
		FROM livetv_artwork_cache
		WHERE expires_at IS NOT NULL AND expires_at < now() AND status = 'ready'
		ORDER BY expires_at
		LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("livetv artwork reap query: %w", err)
	}
	defer rows.Close()

	type doomed struct {
		id         int64
		kind       string
		subjectID  string
		objectPath string
	}
	var batch []doomed
	for rows.Next() {
		var d doomed
		if err := rows.Scan(&d.id, &d.kind, &d.subjectID, &d.objectPath); err != nil {
			return 0, err
		}
		batch = append(batch, d)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	reaped := 0
	for _, d := range batch {
		if err := c.deleteObjects(ctx, d.objectPath); err != nil {
			slog.WarnContext(ctx, "livetv artwork: delete objects failed",
				"component", "livetv", "path", d.objectPath, "error", err)
		}
		tag, err := c.db.Exec(ctx, `DELETE FROM livetv_artwork_cache WHERE id = $1`, d.id)
		if err != nil {
			return reaped, fmt.Errorf("livetv artwork reap delete: %w", err)
		}
		if tag.RowsAffected() > 0 {
			reaped++
		}
	}
	return reaped, nil
}

type artworkRow struct {
	Kind       string
	SubjectID  string
	SourceURL  string
	ObjectPath string
	Status     string
	ExpiresAt  *time.Time
}

func (c *ArtworkCache) lookupMany(ctx context.Context, kind string, subjectIDs []string) (map[string]*artworkRow, error) {
	out := map[string]*artworkRow{}
	if len(subjectIDs) == 0 {
		return out, nil
	}
	rows, err := c.db.Query(ctx, `
		SELECT kind, subject_id, source_url, object_path, status, expires_at
		FROM livetv_artwork_cache
		WHERE kind = $1 AND subject_id = ANY($2)`, kind, subjectIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r artworkRow
		if err := rows.Scan(&r.Kind, &r.SubjectID, &r.SourceURL, &r.ObjectPath, &r.Status, &r.ExpiresAt); err != nil {
			return nil, err
		}
		cp := r
		out[r.SubjectID] = &cp
	}
	return out, rows.Err()
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
	go func() {
		defer c.inFlight.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := c.cacheOne(ctx, kind, subjectID, sourceURL, expiresAt); err != nil {
			slog.WarnContext(ctx, "livetv artwork: cache failed",
				"component", "livetv", "kind", kind, "subject_id", subjectID, "error", err)
		}
	}()
}

func (c *ArtworkCache) cacheOne(ctx context.Context, kind, subjectID, sourceURL string, expiresAt time.Time) error {
	if err := c.upsertPending(ctx, kind, subjectID, sourceURL, expiresAt); err != nil {
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
		_ = c.markFailed(ctx, kind, subjectID, err.Error())
		return err
	}
	return c.markReady(ctx, kind, subjectID, sourceURL, result.OriginalPath, expiresAt)
}

func (c *ArtworkCache) upsertPending(ctx context.Context, kind, subjectID, sourceURL string, expiresAt time.Time) error {
	var expires any
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC()
	}
	_, err := c.db.Exec(ctx, `
		INSERT INTO livetv_artwork_cache (kind, subject_id, source_url, status, expires_at, updated_at)
		VALUES ($1, $2, $3, 'pending', $4, now())
		ON CONFLICT (kind, subject_id) DO UPDATE SET
			source_url = EXCLUDED.source_url,
			status = CASE
				WHEN livetv_artwork_cache.status = 'ready'
					AND livetv_artwork_cache.source_url = EXCLUDED.source_url
					AND livetv_artwork_cache.object_path <> ''
				THEN livetv_artwork_cache.status
				ELSE 'pending'
			END,
			expires_at = COALESCE(EXCLUDED.expires_at, livetv_artwork_cache.expires_at),
			updated_at = now()`,
		kind, subjectID, sourceURL, expires)
	return err
}

func (c *ArtworkCache) markReady(ctx context.Context, kind, subjectID, sourceURL, objectPath string, expiresAt time.Time) error {
	var expires any
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC()
	}
	_, err := c.db.Exec(ctx, `
		UPDATE livetv_artwork_cache SET
			source_url = $3,
			object_path = $4,
			status = 'ready',
			expires_at = $5,
			last_error = '',
			updated_at = now()
		WHERE kind = $1 AND subject_id = $2`,
		kind, subjectID, sourceURL, objectPath, expires)
	return err
}

func (c *ArtworkCache) markFailed(ctx context.Context, kind, subjectID, errText string) error {
	_, err := c.db.Exec(ctx, `
		UPDATE livetv_artwork_cache SET
			status = 'failed',
			last_error = $3,
			updated_at = now()
		WHERE kind = $1 AND subject_id = $2`,
		kind, subjectID, truncateErr(errText))
	return err
}

func (c *ArtworkCache) touchExpiry(ctx context.Context, kind, subjectID string, expiresAt time.Time) error {
	_, err := c.db.Exec(ctx, `
		UPDATE livetv_artwork_cache SET expires_at = $3, updated_at = now()
		WHERE kind = $1 AND subject_id = $2 AND status = 'ready'
			AND (expires_at IS NULL OR expires_at < $3)`,
		kind, subjectID, expiresAt.UTC())
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
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
