package metadata

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
)

const (
	avifReconcileHeadWorkers = 16
	// Rows one discovery pass may examine while sweeping past already-covered
	// windows. Enough to cross the whole corpus at this scale, so a cold cursor
	// still reaches the last surface (people) within a single pass.
	avifReconcileScanRowBudget = 24000
)

// scanCursor rotates a LIMIT/OFFSET window through the candidate query.
//
// Every scanner here shares one flaw if it does not rotate: the query is ordered
// (rank, path), so a fixed window sees only the head of the corpus. With 11k
// episode stills at rank 6, people photos at rank 7 are never reached — which is
// how cast portraits went undiscovered, and how a sweep for retired still rungs
// would never have found one.
type scanCursor struct {
	mu     sync.Mutex
	offset int
}

// next returns the offset to scan and advances past it.
func (c *scanCursor) next(window int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	offset := c.offset
	c.offset += window
	return offset
}

func (c *scanCursor) reset() {
	c.mu.Lock()
	c.offset = 0
	c.mu.Unlock()
}

// ArtworkObjectDeleter deletes individual artwork objects (legacy PNG sweep).
type ArtworkObjectDeleter interface {
	ObjectExists(ctx context.Context, bucket, key string) (bool, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	Bucket() string
}

// AVIFSiblingReconciler finds cached WebP originals whose AVIF sibling is
// missing so the durable backfill queue can generate them.
//
// Scanning rotates. The candidate query is ordered (rank, path) with a LIMIT, so
// a fixed window only ever sees the head of the corpus: with 11k episode stills
// at rank 6, people photos at rank 7 were unreachable and cast portraits could
// never be discovered at all. The cursor advances by the rows examined and wraps
// at the end, so every surface gets its turn.
type AVIFSiblingReconciler struct {
	pool   *pgxpool.Pool
	s3     ArtworkObjectChecker
	cursor scanCursor
}

func NewAVIFSiblingReconciler(pool *pgxpool.Pool, s3 ArtworkObjectChecker) *AVIFSiblingReconciler {
	if pool == nil || s3 == nil {
		return nil
	}
	return &AVIFSiblingReconciler{pool: pool, s3: s3}
}

// DiscoverMissing scans catalog artwork surfaces for cached WebP paths whose
// display-ladder AVIF siblings are absent. Original stays WebP-only (no
// original.avif), so coverage is judged by w300/w500/… AVIFs — which now
// includes the w200 TV rung, so art cached before that rung existed is flagged
// here and the backfill ensures its WebP + AVIF (see Cacher.EnsureAVIFSiblings).
// Does not reset path columns — only enqueues backfill work.
func (r *AVIFSiblingReconciler) DiscoverMissing(ctx context.Context, limit int) ([]AVIFBackfillEnqueueInput, error) {
	if r == nil || r.pool == nil || r.s3 == nil || limit <= 0 {
		return nil, nil
	}

	// Sweep forward from the cursor until this pass finds work, exhausts its row
	// budget, or wraps. A window whose artwork is already covered costs only its
	// HEAD probes, which run 16-wide, so skipping past covered regions is cheap —
	// and it is the only way a scan that starts at rank 0 ever reaches people.
	window := limit * 3
	budget := avifReconcileScanRowBudget
	var wrapped bool
	// Cancellation ends the sweep through the loop condition rather than an
	// if/return, matching RunUntilIdle: a cancelled pass reports no work, not a
	// failure, because the caller is unwinding on the same context.
	for scanned := 0; scanned < budget && ctx.Err() == nil; {
		offset := r.cursor.next(window)
		wrapped = wrapped || offset == 0
		paths, err := r.listCachedWebPOriginals(ctx, window, offset)
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			// Past the end of the corpus: restart from the top on the next window.
			r.cursor.reset()
			if wrapped {
				return nil, nil
			}
			wrapped = true
			continue
		}
		scanned += len(paths)
		if len(paths) < window {
			// Short read means this was the final window.
			r.cursor.reset()
		}
		missing, err := r.probeMissing(ctx, paths, limit)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			return missing, nil
		}
	}
	return nil, nil
}

// pageCachedWebPOriginals returns the next rotated page of cached WebP originals
// for the given cursor, resetting it once a short page marks the end of the
// corpus. Shared by every scanner here so the rotation cannot drift apart.
func (r *AVIFSiblingReconciler) pageCachedWebPOriginals(ctx context.Context, cursor *scanCursor, limit int) ([]string, error) {
	offset := cursor.next(limit)
	paths, err := r.listCachedWebPOriginals(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(paths) < limit {
		cursor.reset()
	}
	return paths, nil
}

// probeMissing HEADs the display-ladder keys for each candidate and returns the
// originals with at least one absent object, up to limit.
func (r *AVIFSiblingReconciler) probeMissing(ctx context.Context, paths []string, limit int) ([]AVIFBackfillEnqueueInput, error) {
	type candidate struct {
		path      string
		imageType string
		avifKeys  []string
	}
	candidates := make([]candidate, 0, len(paths))
	for _, p := range paths {
		if artworkkey.WebPAVIFSibling(p) == "" {
			continue
		}
		imageType := artworkkey.ImageTypeFromPath(p)
		avifKeys := displayAVIFKeys(p, imageType)
		if len(avifKeys) == 0 {
			continue
		}
		candidates = append(candidates, candidate{
			path:      p,
			imageType: imageType,
			avifKeys:  avifKeys,
		})
	}

	type result struct {
		in  AVIFBackfillEnqueueInput
		err error
	}
	results := make([]result, len(candidates))
	sem := make(chan struct{}, avifReconcileHeadWorkers)
	var wg sync.WaitGroup
	for i, c := range candidates {
		if ctx.Err() != nil {
			break
		}
		i, c := i, c
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			for _, avifKey := range c.avifKeys {
				exists, err := r.s3.ObjectExists(ctx, r.s3.Bucket(), avifKey)
				if err != nil {
					results[i] = result{err: err}
					return
				}
				if exists {
					continue
				}
				results[i] = result{in: AVIFBackfillEnqueueInput{
					OriginalPath: c.path,
					ImageType:    c.imageType,
				}}
				return
			}
		}()
	}
	wg.Wait()

	missing := make([]AVIFBackfillEnqueueInput, 0, limit)
	for _, res := range results {
		if res.err != nil {
			return missing, fmt.Errorf("probing AVIF siblings: %w", res.err)
		}
		if res.in.OriginalPath == "" {
			continue
		}
		missing = append(missing, res.in)
		if len(missing) >= limit {
			break
		}
	}
	return missing, nil
}

// displayAVIFKeys returns AVIF object keys for the display ladder (everything
// except the original WebP). Matches GenerateAVIFSiblings --no-avif-keys original.
func displayAVIFKeys(originalPath, imageType string) []string {
	names := artworkkey.VariantNames(imageType)
	keys := make([]string, 0, len(names))
	for _, name := range names {
		if name == artworkkey.OriginalVariant {
			continue
		}
		webpKey := artworkkey.Variant(originalPath, name)
		if avifKey := artworkkey.WebPAVIFSibling(webpKey); avifKey != "" {
			keys = append(keys, avifKey)
		}
	}
	return keys
}

func (r *AVIFSiblingReconciler) listCachedWebPOriginals(ctx context.Context, limit, offset int) ([]string, error) {
	// Prefer item posters/backdrops so the historical orphan gap fills user-visible
	// art first — same visibility ranking as the image-cache discovery sweep.
	query := `
		WITH paths AS (
			SELECT poster_path AS path, 0 AS rank FROM media_items
			WHERE coalesce(poster_path, '') NOT IN ('', '-') AND poster_path NOT LIKE '%://%' AND poster_path LIKE '%.webp'
			UNION ALL
			SELECT backdrop_path, 1 FROM media_items
			WHERE coalesce(backdrop_path, '') NOT IN ('', '-') AND backdrop_path NOT LIKE '%://%' AND backdrop_path LIKE '%.webp'
			UNION ALL
			SELECT logo_path, 2 FROM media_items
			WHERE coalesce(logo_path, '') NOT IN ('', '-') AND logo_path NOT LIKE '%://%' AND logo_path LIKE '%.webp'
			UNION ALL
			SELECT poster_path, 3 FROM media_item_localizations
			WHERE coalesce(poster_path, '') NOT IN ('', '-') AND poster_path NOT LIKE '%://%' AND poster_path LIKE '%.webp'
			UNION ALL
			SELECT backdrop_path, 4 FROM media_item_localizations
			WHERE coalesce(backdrop_path, '') NOT IN ('', '-') AND backdrop_path NOT LIKE '%://%' AND backdrop_path LIKE '%.webp'
			UNION ALL
			SELECT poster_path, 5 FROM seasons
			WHERE coalesce(poster_path, '') NOT IN ('', '-') AND poster_path NOT LIKE '%://%' AND poster_path LIKE '%.webp'
			UNION ALL
			SELECT still_path, 6 FROM episodes
			WHERE coalesce(still_path, '') NOT IN ('', '-') AND still_path NOT LIKE '%://%' AND still_path LIKE '%.webp'
			UNION ALL
			SELECT photo_path, 7 FROM people
			WHERE coalesce(photo_path, '') NOT IN ('', '-') AND photo_path NOT LIKE '%://%' AND photo_path LIKE '%.webp'
		)
		SELECT path FROM paths
		ORDER BY rank, path
		LIMIT $1 OFFSET $2
	`
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing cached WebP originals: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scanning cached WebP path: %w", err)
		}
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return out, rows.Err()
}

// LegacyPNGSiblingCleaner deletes leftover .png siblings for live WebP artwork
// (PNG generation was dropped; orphans remain until swept).
type LegacyPNGSiblingCleaner struct {
	pool   *pgxpool.Pool
	s3     ArtworkObjectDeleter
	cursor scanCursor
}

func NewLegacyPNGSiblingCleaner(pool *pgxpool.Pool, s3 ArtworkObjectDeleter) *LegacyPNGSiblingCleaner {
	if pool == nil || s3 == nil {
		return nil
	}
	return &LegacyPNGSiblingCleaner{pool: pool, s3: s3}
}

func (c *LegacyPNGSiblingCleaner) CleanupLegacyPNG(ctx context.Context, limit int) (checked, deleted int, err error) {
	if c == nil || c.pool == nil || c.s3 == nil || limit <= 0 {
		return 0, 0, nil
	}
	reconciler := &AVIFSiblingReconciler{pool: c.pool, s3: nil}
	paths, err := reconciler.pageCachedWebPOriginals(ctx, &c.cursor, limit)
	if err != nil {
		return 0, 0, err
	}
	bucket := c.s3.Bucket()
	for _, original := range paths {
		if ctx.Err() != nil {
			return checked, deleted, ctx.Err()
		}
		imageType := artworkkey.ImageTypeFromPath(original)
		for _, name := range artworkkey.VariantNames(imageType) {
			webpKey := artworkkey.Variant(original, name)
			pngKey := artworkkey.WebPPNGSibling(webpKey)
			if pngKey == "" {
				continue
			}
			checked++
			exists, existsErr := c.s3.ObjectExists(ctx, bucket, pngKey)
			if existsErr != nil {
				return checked, deleted, existsErr
			}
			if !exists {
				continue
			}
			if delErr := c.s3.DeleteObject(ctx, bucket, pngKey); delErr != nil {
				return checked, deleted, delErr
			}
			deleted++
		}
	}
	return checked, deleted, nil
}

// RetiredVariantCleaner deletes objects for width rungs that have left an image
// type's ladder (see artworkkey.RetiredVariantWidths).
//
// Retiring a rung does not reclaim anything on its own: VariantWidths stops
// naming it, so re-caching an item no longer overwrites it and no existence
// check looks for it. The objects simply stop being read. This sweeps them.
type RetiredVariantCleaner struct {
	pool   *pgxpool.Pool
	s3     ArtworkObjectDeleter
	cursor scanCursor
}

func NewRetiredVariantCleaner(pool *pgxpool.Pool, s3 ArtworkObjectDeleter) *RetiredVariantCleaner {
	if pool == nil || s3 == nil {
		return nil
	}
	return &RetiredVariantCleaner{pool: pool, s3: s3}
}

// CleanupRetiredVariants deletes retired-rung objects for up to limit cached
// originals. Returns how many keys were probed and how many were deleted.
//
// Originals whose type has retired nothing are skipped without a single HEAD,
// so this costs nothing for the types that never lost a rung.
func (c *RetiredVariantCleaner) CleanupRetiredVariants(ctx context.Context, limit int) (checked, deleted int, err error) {
	if c == nil || c.pool == nil || c.s3 == nil || limit <= 0 {
		return 0, 0, nil
	}
	reconciler := &AVIFSiblingReconciler{pool: c.pool, s3: nil}
	// Rotates for the same reason discovery does: the retired rung being swept
	// is a still, and stills sit at rank 6 — far past any fixed head window.
	paths, err := reconciler.pageCachedWebPOriginals(ctx, &c.cursor, limit)
	if err != nil {
		return 0, 0, err
	}
	return c.sweepPaths(ctx, paths)
}

// sweepPaths deletes retired-rung objects for the given originals. Split from
// the query so the delete decisions are testable without a database.
func (c *RetiredVariantCleaner) sweepPaths(ctx context.Context, paths []string) (checked, deleted int, err error) {
	bucket := c.s3.Bucket()
	for _, original := range paths {
		if ctx.Err() != nil {
			return checked, deleted, ctx.Err()
		}
		imageType := artworkkey.ImageTypeFromPath(original)
		for _, key := range artworkkey.RetiredVariantKeys(original, imageType) {
			checked++
			exists, existsErr := c.s3.ObjectExists(ctx, bucket, key)
			if existsErr != nil {
				return checked, deleted, existsErr
			}
			if !exists {
				continue
			}
			if delErr := c.s3.DeleteObject(ctx, bucket, key); delErr != nil {
				return checked, deleted, delErr
			}
			deleted++
		}
	}
	return checked, deleted, nil
}
