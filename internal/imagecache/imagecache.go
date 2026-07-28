// Package imagecache downloads images from URLs, generates sized variants,
// computes thumbhashes, and uploads all variants to an ObjectPutter backend
// (public S3 or the local artwork filesystem store).
package imagecache

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
	"github.com/prairie-server/prairie-server/internal/imageutil"
	"github.com/prairie-server/prairie-server/internal/metadata"
)

const (
	maxDownloadBytes = 25 * 1024 * 1024 // allow oversized provider originals; cached variants are dimension-capped
	downloadTimeout  = 30 * time.Second
)

// ObjectPutter is the object-storage interface required by Cacher.
// Satisfied by *s3client.Client and *artworkstore.LocalStore.
type ObjectPutter interface {
	PutObject(ctx context.Context, bucket, key string, data []byte) error
	Bucket() string
}

type objectMatcher interface {
	ObjectMatches(ctx context.Context, bucket, key string, data []byte) (bool, error)
}

type objectGetter interface {
	GetObject(ctx context.Context, bucket, key string) ([]byte, error)
}

// ArtworkRevisionTracker persists the exact object manifest for an immutable
// revision before any object is uploaded.
type ArtworkRevisionTracker interface {
	TrackArtworkRevision(ctx context.Context, originalPath, imageType string, objectKeys []string) error
}

// AVIFBackfillStore persists deferred AVIF sibling work so it survives process
// restarts. When unset, scheduleAVIFBackfill falls back to in-memory only
// (unit tests without a DB).
type AVIFBackfillStore interface {
	Enqueue(ctx context.Context, originalPath, imageType string) (jobID int64, err error)
	TryClaim(ctx context.Context, jobID int64, workerID string) (bool, error)
	MarkSucceeded(ctx context.Context, jobID int64, workerID string) error
	MarkFailed(ctx context.Context, jobID int64, attemptCount int, workerID, errText string) error
}

// ImageURLResolver resolves plugin:// paths to HTTP URLs.
type ImageURLResolver interface {
	ResolveImageURL(ctx context.Context, path string, variant string) string
}

// CacheRequest describes a single image to cache. For season posters and
// episode stills, ContentID is the parent series's provider ID and the
// SeasonNumber / EpisodeNumber fields scope the S3 key so siblings do not
// collide. Both pointers are nil for item-level images.
type CacheRequest struct {
	SourceURL     string
	ProviderID    string
	ContentType   string // "movies" or "series"
	ContentID     string
	ImageType     metadata.ImageType
	SeasonNumber  *int
	EpisodeNumber *int
	Language      string
	ImageResolver ImageURLResolver // optional; used when SourceURL is a plugin:// path
	// KeyDiscriminator, when set, is inserted into the S3 key between the
	// content ID and the image type (local sidecar art uses the file's 8-hex
	// content hash) so re-cached art rotates to a fresh key.
	KeyDiscriminator string
}

// CacheResult is returned by Cache on success.
type CacheResult struct {
	BasePath         string // S3 key prefix, e.g. "tmdb/movies/550/poster"
	OriginalPath     string // exact immutable original-variant object key
	Revision         string // content revision shared by generated variants
	VariantPaths     map[string]string
	Thumbhash        string // base64-encoded
	Ext              string // file extension including dot (e.g. ".jpg", ".png")
	UploadedVariants int
	ExistingVariants int
}

// Cacher downloads and stores image variants via an ObjectPutter backend.
type Cacher struct {
	s3                ObjectPutter
	revisionTracker   ArtworkRevisionTracker
	avifJobs          AVIFBackfillStore
	httpClient        *http.Client
	enforcePublicURLs bool

	// AVIF backfill: WebP publishes first so the library is browsable; AVIF
	// siblings land asynchronously. When avifJobs is set the work is also
	// persisted so restarts can reclaim it. avifWorkers caps in-flight eager
	// encodes (0 = runtime.NumCPU).
	avifWorkers atomic.Int32
	avifCur     atomic.Int32
	avifWG      sync.WaitGroup
}

// New creates a new Cacher backed by the given ObjectPutter.
func New(s3 ObjectPutter) *Cacher {
	c := &Cacher{
		s3:                s3,
		httpClient:        newSecureHTTPClient(),
		enforcePublicURLs: true,
	}
	c.avifWorkers.Store(int32(avifBackfillConcurrency()))
	return c
}

// SetArtworkRevisionTracker wires durable revision lifecycle tracking. The
// production server configures this whenever object storage is available.
func (c *Cacher) SetArtworkRevisionTracker(tracker ArtworkRevisionTracker) {
	if c != nil {
		c.revisionTracker = tracker
	}
}

// SetAVIFBackfillStore wires durable deferred AVIF sibling jobs. Without it,
// AVIF backfill is in-memory only and is dropped on process exit.
func (c *Cacher) SetAVIFBackfillStore(store AVIFBackfillStore) {
	if c != nil {
		c.avifJobs = store
	}
}

// SetAVIFBackfillConcurrency sets the eager in-process AVIF encode concurrency.
// Values <= 0 mean runtime.NumCPU(). Hot-reloads without dropping in-flight work.
func (c *Cacher) SetAVIFBackfillConcurrency(n int) {
	if c == nil {
		return
	}
	if n <= 0 {
		n = avifBackfillConcurrency()
	}
	c.avifWorkers.Store(int32(n))
}

func newWithHTTPClient(s3 ObjectPutter, client *http.Client) *Cacher {
	if client == nil {
		client = http.DefaultClient
	}
	c := &Cacher{
		s3:         s3,
		httpClient: client,
	}
	c.avifWorkers.Store(int32(avifBackfillConcurrency()))
	return c
}

// WaitAVIFBackfill blocks until deferred AVIF sibling uploads finish. Tests
// call this after Cache/CacheBytes so assertions see the full object set.
// Production shutdown also waits so in-flight encodes drain during the
// termination grace period.
func (c *Cacher) WaitAVIFBackfill() {
	if c == nil {
		return
	}
	c.avifWG.Wait()
}

// WaitAVIFBackfillContext waits for deferred AVIF work or ctx cancellation,
// whichever comes first. Returns ctx.Err() when cancelled before drain completes.
func (c *Cacher) WaitAVIFBackfillContext(ctx context.Context) error {
	if c == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		c.avifWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// avifBackfillConcurrency is the default eager AVIF worker count. It tracks the
// shared artwork encode budget (well below the core count) so deferred encodes
// never crowd out playback ffmpeg.
func avifBackfillConcurrency() int {
	n := imageutil.DefaultEncodeBudgetSize()
	if n < 1 {
		return 1
	}
	return n
}

func (c *Cacher) acquireAVIFSlot() {
	for {
		max := c.avifWorkers.Load()
		if max < 1 {
			max = 1
		}
		for {
			cur := c.avifCur.Load()
			if cur >= max {
				break
			}
			if c.avifCur.CompareAndSwap(cur, cur+1) {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c *Cacher) releaseAVIFSlot() {
	c.avifCur.Add(-1)
}

// CacheImage implements metadata.ImageCacher using the internal Cache method.
func (c *Cacher) CacheImage(ctx context.Context, req metadata.CacheImageRequest) (*metadata.CacheImageResult, error) {
	result, err := c.Cache(ctx, CacheRequest{
		SourceURL:     req.SourceURL,
		ProviderID:    req.ProviderID,
		ContentType:   req.ContentType,
		ContentID:     req.ContentID,
		ImageType:     req.ImageType,
		SeasonNumber:  req.SeasonNumber,
		EpisodeNumber: req.EpisodeNumber,
		Language:      req.Language,
	})
	if err != nil {
		return nil, err
	}
	return cacheImageResultFromCacheResult(result), nil
}

// CacheImageBytes implements metadata.ImageByteCacher using CacheBytes. Used
// by the image cache processor for file:// sources that it reads itself.
func (c *Cacher) CacheImageBytes(ctx context.Context, data []byte, req metadata.CacheImageRequest) (*metadata.CacheImageResult, error) {
	result, err := c.CacheBytes(ctx, data, CacheRequest{
		ProviderID:       req.ProviderID,
		ContentType:      req.ContentType,
		ContentID:        req.ContentID,
		ImageType:        req.ImageType,
		SeasonNumber:     req.SeasonNumber,
		EpisodeNumber:    req.EpisodeNumber,
		Language:         req.Language,
		KeyDiscriminator: req.KeyDiscriminator,
	})
	if err != nil {
		return nil, err
	}
	return cacheImageResultFromCacheResult(result), nil
}

func cacheImageResultFromCacheResult(result *CacheResult) *metadata.CacheImageResult {
	return &metadata.CacheImageResult{
		BasePath:         result.BasePath,
		OriginalPath:     result.OriginalPath,
		Revision:         result.Revision,
		Thumbhash:        result.Thumbhash,
		Ext:              result.Ext,
		UploadedVariants: result.UploadedVariants,
		ExistingVariants: result.ExistingVariants,
	}
}

// CacheAudiobookCover is a thin convenience over CacheBytes specifically
// for the audiobook scanner. Avoids exporting the imagecache request
// struct to the scanner package (which would create an import cycle
// scanner -> imagecache -> metadata -> scanner). Stores under
// "local/audiobooks/{contentID}/poster/...".
func (c *Cacher) CacheAudiobookCover(ctx context.Context, data []byte, contentID string) (storedPath string, thumbhash string, err error) {
	res, err := c.CacheBytes(ctx, data, CacheRequest{
		ProviderID:  "local",
		ContentType: "audiobooks",
		ContentID:   contentID,
		ImageType:   metadata.ImagePoster,
	})
	if err != nil {
		return "", "", err
	}
	return res.OriginalPath, res.Thumbhash, nil
}

// CacheEbookCover stores an embedded ebook cover under
// "local/ebooks/{contentID}/poster/..." using the same poster variants as
// provider-hosted book artwork.
func (c *Cacher) CacheEbookCover(ctx context.Context, data []byte, contentID string) (storedPath string, thumbhash string, err error) {
	res, err := c.CacheBytes(ctx, data, CacheRequest{
		ProviderID:  "local",
		ContentType: "ebooks",
		ContentID:   contentID,
		ImageType:   metadata.ImagePoster,
	})
	if err != nil {
		return "", "", err
	}
	return res.OriginalPath, res.Thumbhash, nil
}

// validateCacheRequest checks the required identity fields and the
// episode/season invariant shared by Cache and CacheBytes. Keeping it in one
// place prevents the season/episode guard from drifting between the two paths:
// buildBasePath only appends the episode segment inside the SeasonNumber branch,
// so an episode without a season would silently collide distinct episodes' art
// under the same S3 key.
func validateCacheRequest(req CacheRequest) error {
	if strings.TrimSpace(req.ProviderID) == "" {
		return fmt.Errorf("imagecache: provider ID is required")
	}
	if strings.TrimSpace(req.ContentType) == "" {
		return fmt.Errorf("imagecache: content type is required")
	}
	if strings.TrimSpace(req.ContentID) == "" {
		return fmt.Errorf("imagecache: content ID is required")
	}
	if req.EpisodeNumber != nil && req.SeasonNumber == nil {
		return fmt.Errorf("imagecache: episode number requires a season number")
	}
	return nil
}

// CacheBytes performs the same variant generation, thumbhash, and S3 upload as
// Cache but starts from raw image bytes already in hand. Used by the
// audiobook scanner to push embedded M4B cover art into S3 without round-
// tripping through HTTP.
//
// Publish is two-phase: WebP (+ thumbhash) uploads complete before return so
// the metadata job can mark the library browsable immediately; AVIF siblings
// are encoded and uploaded on a deferred low-priority pass.
func (c *Cacher) CacheBytes(ctx context.Context, data []byte, req CacheRequest) (*CacheResult, error) {
	if err := validateCacheRequest(req); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("imagecache: image data is empty")
	}
	thumbhash, err := imageutil.Thumbhash(data)
	if err != nil {
		return nil, fmt.Errorf("imagecache: thumbhash: %w", err)
	}
	widths := variantWidths(req.ImageType)
	result, err := imageutil.GenerateWebPVariants(data, widths)
	if err != nil {
		return nil, fmt.Errorf("imagecache: generate variants: %w", err)
	}
	basePath := buildBasePath(req)
	bucket := c.s3.Bucket()
	revision := variantRevision(result)
	variantPaths := buildVariantPaths(basePath, revision, result)
	if err := c.trackRevision(ctx, req.ImageType, variantPaths); err != nil {
		return nil, err
	}

	uploadStats, err := c.uploadVariants(ctx, bucket, result, variantPaths)
	if err != nil {
		return nil, err
	}
	c.scheduleAVIFBackfill(ctx, data, widths, bucket, variantPaths, metadata.ImageTypeToString(req.ImageType))
	return &CacheResult{
		BasePath:         basePath,
		OriginalPath:     variantPaths[artworkkey.OriginalVariant],
		Revision:         revision,
		VariantPaths:     variantPaths,
		Thumbhash:        thumbhash,
		Ext:              result.Ext,
		UploadedVariants: uploadStats.uploaded,
		ExistingVariants: uploadStats.existing,
	}, nil
}

func (c *Cacher) scheduleAVIFBackfill(ctx context.Context, data []byte, widths []int, bucket string, variantPaths map[string]string, imageType string) {
	if c == nil || len(variantPaths) == 0 {
		return
	}
	originalPath := variantPaths[artworkkey.OriginalVariant]
	if originalPath == "" {
		return
	}
	if imageType == "" {
		imageType = artworkkey.ImageTypeFromPath(originalPath)
	}

	// Persist first so a crash after WebP publish cannot orphan the AVIF work.
	if c.avifJobs != nil {
		if _, err := c.avifJobs.Enqueue(ctx, originalPath, imageType); err != nil {
			slog.WarnContext(ctx, "imagecache: enqueue AVIF backfill failed", "component", "imagecache", "path", originalPath, "error", err)
		}
		// Durable queue is authoritative: skip eager encode so CacheBytes
		// workers and the backfill task do not double-consume the encode budget
		// (observed ~2× in-flight on 4-core nodes). The task drains the queue.
		return
	}

	// No durable store (tests / degraded mode): encode AVIF in-process.
	paths := make(map[string]string, len(variantPaths))
	for k, v := range variantPaths {
		paths[k] = v
	}
	src := append([]byte(nil), data...)
	widthCopy := append([]int(nil), widths...)

	c.avifWG.Add(1)
	go func() {
		defer c.avifWG.Done()
		c.acquireAVIFSlot()
		defer c.releaseAVIFSlot()

		bg := context.Background()
		avifResult, err := imageutil.GenerateAVIFSiblings(src, widthCopy)
		if err != nil {
			slog.WarnContext(bg, "imagecache: deferred AVIF encode failed", "component", "imagecache", "error", err)
			return
		}
		uploadCtx, cancel := context.WithTimeout(bg, 3*time.Minute)
		defer cancel()
		if _, err := c.uploadAVIFSiblings(uploadCtx, bucket, avifResult, paths); err != nil {
			slog.WarnContext(bg, "imagecache: deferred AVIF upload failed", "component", "imagecache", "error", err)
		}
	}()
}

// EnsureAVIFSiblings ensures every cached width variant of an already-cached
// WebP original exists — both the WebP rung and its AVIF sibling — regenerating
// from the stored original and uploading only what is missing. Used by the
// durable backfill processor when source bytes are no longer in memory (restart
// recovery / reconcile). It ensures the full ladder rather than AVIF alone so
// that adding a rung to artworkkey.VariantWidths (e.g. the w200 TV rung)
// backfills the missing WebP variant too; the AVIF-centric name is retained
// because it satisfies the durable queue's existing AVIFSiblingEnsuer interface.
// The original itself is never re-uploaded (it is immutable and already stored).
func (c *Cacher) EnsureAVIFSiblings(ctx context.Context, originalPath, imageType string) error {
	if c == nil || c.s3 == nil {
		return fmt.Errorf("imagecache: cacher not configured")
	}
	originalPath = strings.TrimSpace(originalPath)
	if originalPath == "" || artworkkey.WebPAVIFSibling(originalPath) == "" {
		return fmt.Errorf("imagecache: original path is not a cached WebP key")
	}
	if imageType == "" {
		imageType = artworkkey.ImageTypeFromPath(originalPath)
	}
	getter, ok := c.s3.(objectGetter)
	if !ok {
		return fmt.Errorf("imagecache: object store does not support GetObject")
	}
	data, err := getter.GetObject(ctx, c.s3.Bucket(), originalPath)
	if err != nil {
		return fmt.Errorf("imagecache: get WebP original: %w", err)
	}
	widths := artworkkey.VariantWidths(imageType)
	// GenerateAVIFSiblings yields WebP + AVIF for each display width (skipping the
	// costly full-size original AVIF); uploadVariants then writes both formats.
	result, err := imageutil.GenerateAVIFSiblings(data, widths)
	if err != nil {
		return fmt.Errorf("imagecache: generate variant ladder: %w", err)
	}
	// Width variants only: the original stays as stored, and uploadVariants skips
	// any rung that already matches, so only newly-added rungs are written.
	paths := make(map[string]string, len(artworkkey.VariantNames(imageType)))
	for _, name := range artworkkey.VariantNames(imageType) {
		if name == artworkkey.OriginalVariant {
			continue
		}
		paths[name] = artworkkey.Variant(originalPath, name)
	}
	if _, err := c.uploadVariants(ctx, c.s3.Bucket(), result, paths); err != nil {
		return err
	}
	return nil
}

func (c *Cacher) uploadAVIFSiblings(ctx context.Context, bucket string, result *imageutil.VariantResult, variantPaths map[string]string) (uploadVariantStats, error) {
	jobs := make([]uploadJob, 0, len(result.Variants))
	for _, variant := range result.Variants {
		key := variantPaths[variant.Key]
		if key == "" || len(variant.AVIF) == 0 {
			continue
		}
		if avifKey := artworkkey.WebPAVIFSibling(key); avifKey != "" {
			jobs = append(jobs, uploadJob{key: avifKey, data: variant.AVIF})
		}
	}
	return c.runUploadJobs(ctx, bucket, jobs)
}

// Cache downloads the image at req.SourceURL and stores it through the same
// variant, revision-tracking, and upload pipeline as CacheBytes.
func (c *Cacher) Cache(ctx context.Context, req CacheRequest) (*CacheResult, error) {
	if err := validateCacheRequest(req); err != nil {
		return nil, err
	}

	url := req.SourceURL

	// Resolve non-HTTP paths (e.g. plugin_id://path) via the resolver.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		if req.ImageResolver == nil {
			return nil, fmt.Errorf("imagecache: non-HTTP URL %q requires ImageResolver", url)
		}
		url = req.ImageResolver.ResolveImageURL(ctx, url, "original")
		if url == "" {
			return nil, fmt.Errorf("imagecache: resolver returned empty URL for %q", req.SourceURL)
		}
	}

	data, err := c.downloadImage(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("imagecache: download %s: %w", url, err)
	}

	return c.CacheBytes(ctx, data, req)
}

// variantWidths returns the resize widths for the given image type. The
// ladder itself is owned by artworkkey so key expansion and GC manifests can
// never drift from what is generated here.
func variantWidths(t metadata.ImageType) []int {
	return artworkkey.VariantWidths(metadata.ImageTypeToString(t))
}

type uploadVariantStats struct {
	uploaded int
	existing int
}

type uploadJob struct {
	key  string
	data []byte
}

func (c *Cacher) uploadVariants(ctx context.Context, bucket string, result *imageutil.VariantResult, variantPaths map[string]string) (uploadVariantStats, error) {
	jobs := make([]uploadJob, 0, len(result.Variants)*3)
	for _, variant := range result.Variants {
		key := variantPaths[variant.Key]
		if key == "" {
			continue
		}
		jobs = append(jobs, uploadJob{key: key, data: variant.Data})
		if len(variant.AVIF) > 0 {
			if avifKey := artworkkey.WebPAVIFSibling(key); avifKey != "" {
				jobs = append(jobs, uploadJob{key: avifKey, data: variant.AVIF})
			}
		}
		if len(variant.PNG) > 0 {
			if pngKey := artworkkey.WebPPNGSibling(key); pngKey != "" {
				jobs = append(jobs, uploadJob{key: pngKey, data: variant.PNG})
			}
		}
	}
	return c.runUploadJobs(ctx, bucket, jobs)
}

func (c *Cacher) runUploadJobs(ctx context.Context, bucket string, jobs []uploadJob) (uploadVariantStats, error) {
	var wg sync.WaitGroup
	uploadErrs := make([]error, len(jobs))
	stats := make([]uploadVariantStats, len(jobs))
	for i, j := range jobs {
		wg.Add(1)
		go func(idx int, item uploadJob) {
			defer wg.Done()
			if exists, err := objectMatches(ctx, c.s3, bucket, item.key, item.data); err != nil {
				uploadErrs[idx] = fmt.Errorf("imagecache: check existing %s: %w", item.key, err)
				return
			} else if exists {
				stats[idx].existing = 1
				return
			}
			if err := putObjectWithRetry(ctx, c.s3, bucket, item.key, item.data); err != nil {
				uploadErrs[idx] = fmt.Errorf("imagecache: upload %s: %w", item.key, err)
				return
			}
			stats[idx].uploaded = 1
		}(i, j)
	}
	wg.Wait()
	var total uploadVariantStats
	for _, err := range uploadErrs {
		if err != nil {
			return total, err
		}
	}
	for _, s := range stats {
		total.uploaded += s.uploaded
		total.existing += s.existing
	}
	return total, nil
}

func variantRevision(result *imageutil.VariantResult) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "silo-artwork-v1\x00")
	_, _ = io.WriteString(h, result.Ext)
	_, _ = h.Write([]byte{0})
	variants := append([]imageutil.Variant(nil), result.Variants...)
	sort.Slice(variants, func(i, j int) bool { return variants[i].Key < variants[j].Key })
	var size [8]byte
	for _, variant := range variants {
		_, _ = io.WriteString(h, variant.Key)
		_, _ = h.Write([]byte{0})
		binary.BigEndian.PutUint64(size[:], uint64(len(variant.Data)))
		_, _ = h.Write(size[:])
		_, _ = h.Write(variant.Data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func buildVariantPaths(basePath, revision string, result *imageutil.VariantResult) map[string]string {
	paths := make(map[string]string, len(result.Variants))
	for _, variant := range result.Variants {
		paths[variant.Key] = artworkkey.Build(basePath, variant.Key, revision, result.Ext)
	}
	return paths
}

func (c *Cacher) trackRevision(ctx context.Context, imageType metadata.ImageType, variantPaths map[string]string) error {
	if c == nil || c.revisionTracker == nil {
		return nil
	}
	originalPath := variantPaths[artworkkey.OriginalVariant]
	// Track WebP + AVIF keys for the display ladder. PNG siblings are no longer
	// generated; ObjectKeys still lists legacy PNG for GC of older revisions.
	// original.avif is intentionally omitted: GenerateAVIFSiblings skips AVIF
	// for the original key (clients fall through to WebP).
	keys := make([]string, 0, len(variantPaths)*2)
	for name, key := range variantPaths {
		keys = append(keys, key)
		if name == artworkkey.OriginalVariant {
			continue
		}
		if avifKey := artworkkey.WebPAVIFSibling(key); avifKey != "" {
			keys = append(keys, avifKey)
		}
	}
	sort.Strings(keys)
	if err := c.revisionTracker.TrackArtworkRevision(ctx, originalPath, metadata.ImageTypeToString(imageType), keys); err != nil {
		return fmt.Errorf("imagecache: track artwork revision: %w", err)
	}
	return nil
}

// objectMatches reports whether the object at key already holds exactly data.
// Backends that cannot verify content report false so the immutable object is
// rewritten; bare existence must never be accepted as a content match.
func objectMatches(ctx context.Context, putter ObjectPutter, bucket, key string, data []byte) (bool, error) {
	matcher, ok := putter.(objectMatcher)
	if !ok {
		return false, nil
	}
	return matcher.ObjectMatches(ctx, bucket, key, data)
}

func putObjectWithRetry(ctx context.Context, putter ObjectPutter, bucket, key string, data []byte) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := putter.PutObject(ctx, bucket, key, data); err != nil {
			lastErr = err
			if attempt == maxAttempts-1 {
				// Final attempt failed; return immediately without a pointless backoff.
				break
			}
			timer := time.NewTimer(time.Duration(attempt+1) * 500 * time.Millisecond)
			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}
		return nil
	}
	return lastErr
}

// buildBasePath constructs the S3 key prefix for a given image. Season
// posters and episode stills nest under their parent series so a single
// DeletePrefix on the series prefix cascades to all child images.
//
//	item-level:        {provider}/{type}/{id}/{imageType}
//	localized item:   {provider}/{type}/{id}/localizations/{lang}/{imageType}
//	season:           {provider}/{type}/{id}/seasons/{n}/{imageType}
//	localized season: {provider}/{type}/{id}/localizations/{lang}/seasons/{n}/{imageType}
//	episode:          {provider}/{type}/{id}/seasons/{n}/episodes/{m}/{imageType}
//
// A non-empty KeyDiscriminator (local sidecar content hash) is inserted
// immediately before the image type so the variant's parent directory stays
// the image type segment (the imageTypeFromCachedPath contract).
func buildBasePath(req CacheRequest) string {
	imageTypeName := imageTypeName(req.ImageType)
	base := fmt.Sprintf("%s/%s/%s", req.ProviderID, req.ContentType, req.ContentID)
	if lang := normalizeImageLanguage(req.Language); lang != "" {
		base = fmt.Sprintf("%s/localizations/%s", base, lang)
	}
	if req.SeasonNumber != nil {
		base = fmt.Sprintf("%s/seasons/%d", base, *req.SeasonNumber)
		if req.EpisodeNumber != nil {
			base = fmt.Sprintf("%s/episodes/%d", base, *req.EpisodeNumber)
		}
	}
	if discriminator := strings.TrimSpace(req.KeyDiscriminator); discriminator != "" {
		base = base + "/" + discriminator
	}
	return base + "/" + imageTypeName
}

// imageTypeName returns the lowercase string name for an ImageType.
func imageTypeName(t metadata.ImageType) string {
	switch t {
	case metadata.ImagePoster:
		return "poster"
	case metadata.ImageBackdrop:
		return "backdrop"
	case metadata.ImageLogo:
		return "logo"
	case metadata.ImageStill:
		return "still"
	case metadata.ImageProfile:
		return "profile"
	default:
		return "unknown"
	}
}

func normalizeImageLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range language {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// downloadImage fetches the image at the given URL, enforcing size, timeout,
// and public-network limits.
func (c *Cacher) downloadImage(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if c.enforcePublicURLs {
		if err := validatePublicImageURL(parsed); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	client := c.httpClient
	if client == nil {
		client = newSecureHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxDownloadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > maxDownloadBytes {
		return nil, fmt.Errorf("image exceeds %d byte limit", maxDownloadBytes)
	}

	return data, nil
}

func newSecureHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         secureImageDialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return validatePublicImageURL(req.URL)
		},
	}
}

func validatePublicImageURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("empty URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL host is required")
	}
	if addr, err := netip.ParseAddr(host); err == nil && !isPublicAddr(addr) {
		return fmt.Errorf("private image host %q is not allowed", host)
	}
	return nil
}

func secureImageDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addr, err := resolvePublicAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: downloadTimeout}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
}

func resolvePublicAddr(ctx context.Context, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if isPublicAddr(addr) {
			return addr, nil
		}
		return netip.Addr{}, fmt.Errorf("private image host %q is not allowed", host)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("resolve image host %q: %w", host, err)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if ok && isPublicAddr(addr) {
			return addr, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("image host %q did not resolve to a public address", host)
}

func isPublicAddr(addr netip.Addr) bool {
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	return addr.IsGlobalUnicast() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}
