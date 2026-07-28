package metadata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/prairie-server/prairie-server/internal/imageutil"
	"github.com/prairie-server/prairie-server/internal/models"
)

const (
	avifBackfillClaimBatch    = 32
	avifBackfillMaxRuntime    = 8 * time.Minute
	avifBackfillDiscoverLimit = 500
	avifBackfillDiscoverEvery = 15 * time.Minute
	avifBackfillSucceededKeep = 30 * 24 * time.Hour
)

// AVIFSiblingEnsuer generates and uploads missing AVIF siblings for a cached
// WebP original. Satisfied by *imagecache.Cacher.
type AVIFSiblingEnsuer interface {
	EnsureAVIFSiblings(ctx context.Context, originalPath, imageType string) error
}

// AVIFBackfillJobStore is the durable queue surface used by the processor.
type AVIFBackfillJobStore interface {
	EnqueueBatch(ctx context.Context, inputs []AVIFBackfillEnqueueInput) ([]int64, error)
	ClaimDue(ctx context.Context, workerID string, limit int) ([]*models.ArtworkAVIFBackfillJob, error)
	MarkSucceeded(ctx context.Context, id int64, lockedBy string) error
	MarkFailed(ctx context.Context, id int64, attemptCount int, lockedBy, errText string) error
	RequeueClaimed(ctx context.Context, ids []int64, workerID string) error
	DeleteSucceededBefore(ctx context.Context, before time.Time, limit int) (int, error)
	QueuedCount(ctx context.Context) (int, error)
}

// AVIFMissingDiscoverer finds cached WebP artwork whose AVIF sibling is absent
// and returns enqueue inputs. Satisfied by *AVIFSiblingReconciler.
type AVIFMissingDiscoverer interface {
	DiscoverMissing(ctx context.Context, limit int) ([]AVIFBackfillEnqueueInput, error)
}

// LegacyPNGCleaner deletes orphaned pre-PNG-drop sibling objects.
type LegacyPNGCleaner interface {
	CleanupLegacyPNG(ctx context.Context, limit int) (checked, deleted int, err error)
}

// RetiredVariantSweeper deletes objects for width rungs that have left an image
// type's ladder. Satisfied by *RetiredVariantCleaner.
type RetiredVariantSweeper interface {
	CleanupRetiredVariants(ctx context.Context, limit int) (checked, deleted int, err error)
}

// AVIFBackfillStats summarizes one processor run.
type AVIFBackfillStats struct {
	EnqueuedExisting int `json:"enqueued_existing"`
	Claimed          int `json:"claimed"`
	Succeeded        int `json:"succeeded"`
	Failed           int `json:"failed"`
	DeletedSucceeded int `json:"deleted_succeeded"`
	PNGChecked       int `json:"png_checked"`
	PNGDeleted       int `json:"png_deleted"`
	RetiredChecked   int `json:"retired_checked"`
	RetiredDeleted   int `json:"retired_deleted"`
	// ClaimsLost counts encodes whose status update matched no row because the
	// lease had been recovered underneath them. Any non-zero value means the
	// same artwork is being encoded more than once.
	ClaimsLost     int  `json:"claims_lost,omitempty"`
	RuntimeLimited bool `json:"runtime_limited,omitempty"`
	// PausedForPlayback records that the pass yielded because playback or a
	// transcode was active. The queue is durable, so the next tick resumes.
	PausedForPlayback bool `json:"paused_for_playback,omitempty"`
}

// AVIFBackfillProcessor drains the durable AVIF queue and periodically
// discovers webp-without-avif orphans.
type AVIFBackfillProcessor struct {
	jobs     AVIFBackfillJobStore
	ensurer  AVIFSiblingEnsuer
	discover AVIFMissingDiscoverer
	pngClean LegacyPNGCleaner
	retired  RetiredVariantSweeper
	// claimsLost counts status updates that matched no row because the lease had
	// already been recovered. Non-zero means work is being repeated.
	claimsLost atomic.Int64
	logger     *slog.Logger
	enabled    atomic.Bool
	workers    atomic.Int32

	// playbackActive reports whether any playback/transcode session is live.
	// Artwork encoding yields to it entirely rather than competing for cores.
	playbackActive atomic.Pointer[func() bool]

	discoveryMu   sync.Mutex
	lastDiscovery time.Time
}

func NewAVIFBackfillProcessor(jobs AVIFBackfillJobStore, ensurer AVIFSiblingEnsuer) *AVIFBackfillProcessor {
	p := &AVIFBackfillProcessor{
		jobs:    jobs,
		ensurer: ensurer,
		logger:  slog.Default(),
	}
	p.enabled.Store(true)
	p.workers.Store(int32(ResolveAVIFBackfillWorkers(0)))
	return p
}

// SetWorkers updates the configured AVIF encode concurrency. Values <= 0 mean
// runtime.NumCPU(). Hot-reloads without interrupting in-flight claims.
func (p *AVIFBackfillProcessor) SetWorkers(configured int) {
	if p == nil {
		return
	}
	p.workers.Store(int32(ResolveAVIFBackfillWorkers(configured)))
}

// Workers returns the effective AVIF backfill concurrency.
func (p *AVIFBackfillProcessor) Workers() int {
	if p == nil {
		return ResolveAVIFBackfillWorkers(0)
	}
	n := int(p.workers.Load())
	if n < 1 {
		return ResolveAVIFBackfillWorkers(0)
	}
	return n
}

func (p *AVIFBackfillProcessor) SetDiscoverer(d AVIFMissingDiscoverer) {
	if p != nil {
		p.discover = d
	}
}

func (p *AVIFBackfillProcessor) SetPNGCleaner(c LegacyPNGCleaner) {
	if p != nil {
		p.pngClean = c
	}
}

func (p *AVIFBackfillProcessor) SetRetiredVariantSweeper(s RetiredVariantSweeper) {
	if p != nil {
		p.retired = s
	}
}

func (p *AVIFBackfillProcessor) SetEnabled(enabled bool) {
	if p != nil {
		p.enabled.Store(enabled)
	}
}

// SetPlaybackActivityCheck wires the predicate that reports live playback or
// transcode sessions. While it returns true the processor stops claiming work:
// jobs stay in the durable queue and the next tick picks them up.
func (p *AVIFBackfillProcessor) SetPlaybackActivityCheck(fn func() bool) {
	if p == nil {
		return
	}
	if fn == nil {
		p.playbackActive.Store(nil)
		return
	}
	p.playbackActive.Store(&fn)
}

func (p *AVIFBackfillProcessor) playbackIsActive() bool {
	if p == nil {
		return false
	}
	fn := p.playbackActive.Load()
	if fn == nil || *fn == nil {
		return false
	}
	return (*fn)()
}

// RunUntilIdle claims and processes AVIF backfill jobs until the queue is idle
// or maxRuntime elapses. When discovery is due it also enqueues missing siblings.
func (p *AVIFBackfillProcessor) RunUntilIdle(ctx context.Context, concurrency int, maxRuntime time.Duration, onProgress func(float64, string)) (AVIFBackfillStats, error) {
	stats := AVIFBackfillStats{}
	if p == nil || p.jobs == nil || p.ensurer == nil || !p.enabled.Load() {
		return stats, nil
	}
	if concurrency <= 0 {
		concurrency = p.Workers()
	}
	if maxRuntime <= 0 {
		maxRuntime = avifBackfillMaxRuntime
	}
	if onProgress == nil {
		onProgress = func(float64, string) {}
	}

	// Yield the whole pass while someone is watching: artwork encodes and
	// playback ffmpeg contend for the same cores, and this queue is durable.
	if p.playbackIsActive() {
		stats.PausedForPlayback = true
		onProgress(100, "Paused: playback session active")
		return stats, nil
	}

	if p.discoveryDue() {
		if err := p.discoverMissing(ctx, &stats, onProgress); err != nil {
			return stats, err
		}
	}

	workerID := "avif-" + uuid.NewString()
	deadline := time.Now().Add(maxRuntime)
	for ctx.Err() == nil {
		if time.Now().After(deadline) {
			stats.RuntimeLimited = true
			break
		}
		// Re-check between claim passes so playback starting mid-run stops the
		// drain at the next batch boundary instead of at the runtime cap.
		if p.playbackIsActive() {
			stats.PausedForPlayback = true
			break
		}
		claimLimit := concurrency
		if claimLimit > avifBackfillClaimBatch {
			claimLimit = avifBackfillClaimBatch
		}
		jobs, err := p.jobs.ClaimDue(ctx, workerID, claimLimit)
		if err != nil {
			return stats, err
		}
		if len(jobs) == 0 {
			break
		}
		stats.Claimed += len(jobs)
		onProgress(p.progressPercent(ctx), fmt.Sprintf("Encoding AVIF siblings (%d claimed this pass)", len(jobs)))

		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var unstarted []int64
	loop:
		for i, job := range jobs {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				for _, rem := range jobs[i:] {
					unstarted = append(unstarted, rem.ID)
				}
				break loop
			case <-time.After(time.Until(deadline)):
				stats.RuntimeLimited = true
				for _, rem := range jobs[i:] {
					unstarted = append(unstarted, rem.ID)
				}
				break loop
			}
			wg.Add(1)
			go func(job *models.ArtworkAVIFBackfillJob) {
				defer wg.Done()
				defer func() { <-sem }()
				ok := p.processOne(ctx, job, workerID)
				mu.Lock()
				if ok {
					stats.Succeeded++
				} else {
					stats.Failed++
				}
				mu.Unlock()
			}(job)
		}
		wg.Wait()
		if len(unstarted) > 0 {
			_ = p.jobs.RequeueClaimed(context.WithoutCancel(ctx), unstarted, workerID)
		}
		if ctx.Err() != nil || stats.RuntimeLimited {
			break
		}
	}

	stats.ClaimsLost = int(p.claimsLost.Swap(0))

	deleted, err := p.jobs.DeleteSucceededBefore(ctx, time.Now().Add(-avifBackfillSucceededKeep), 1000)
	if err != nil {
		p.logger.WarnContext(ctx, "avif backfill: cleanup succeeded jobs failed", "error", err)
	} else {
		stats.DeletedSucceeded = deleted
	}

	// Both sweeps only run once the queue is idle: encoding real artwork is
	// always the better use of the cores, and neither sweep is urgent.
	if p.pngClean != nil && stats.Claimed == 0 {
		checked, deletedPNG, pngErr := p.pngClean.CleanupLegacyPNG(ctx, 200)
		stats.PNGChecked = checked
		stats.PNGDeleted = deletedPNG
		if pngErr != nil {
			p.logger.WarnContext(ctx, "avif backfill: legacy PNG cleanup failed", "error", pngErr)
		}
	}

	if p.retired != nil && stats.Claimed == 0 {
		checked, deletedRetired, retiredErr := p.retired.CleanupRetiredVariants(ctx, 200)
		stats.RetiredChecked = checked
		stats.RetiredDeleted = deletedRetired
		if retiredErr != nil {
			p.logger.WarnContext(ctx, "avif backfill: retired variant cleanup failed", "error", retiredErr)
		}
	}

	message := fmt.Sprintf(
		"AVIF backfill: enqueued %d, claimed %d, succeeded %d, failed %d, deleted %d old successes, png checked %d deleted %d, retired checked %d deleted %d",
		stats.EnqueuedExisting, stats.Claimed, stats.Succeeded, stats.Failed, stats.DeletedSucceeded,
		stats.PNGChecked, stats.PNGDeleted, stats.RetiredChecked, stats.RetiredDeleted,
	)
	if stats.ClaimsLost > 0 {
		message += fmt.Sprintf(", %d claim(s) lost (work repeated)", stats.ClaimsLost)
	}
	if stats.PausedForPlayback {
		message += ", paused for playback"
	}
	onProgress(100, message)
	return stats, nil
}

func (p *AVIFBackfillProcessor) processOne(ctx context.Context, job *models.ArtworkAVIFBackfillJob, workerID string) bool {
	err := p.ensurer.EnsureAVIFSiblings(ctx, job.OriginalPath, job.ImageType)
	if err != nil {
		p.logger.WarnContext(ctx, "avif backfill: ensure failed",
			"path", job.OriginalPath, "attempt", job.AttemptCount, "error", err)
		if markErr := p.jobs.MarkFailed(ctx, job.ID, job.AttemptCount, workerID, err.Error()); markErr != nil {
			if errors.Is(markErr, ErrBackfillClaimLost) {
				p.claimsLost.Add(1)
			}
			p.logger.WarnContext(ctx, "avif backfill: mark failed", "job_id", job.ID, "error", markErr)
		}
		return false
	}
	if markErr := p.jobs.MarkSucceeded(ctx, job.ID, workerID); markErr != nil {
		// A lost claim means the encode landed but the bookkeeping did not: the
		// row will be handed out again and re-encoded. Worth a distinct message,
		// because a run of these is the signature of a lease shorter than the
		// pass that holds it.
		if errors.Is(markErr, ErrBackfillClaimLost) {
			p.logger.WarnContext(ctx, "avif backfill: claim lost before recording success; job will be retried",
				"job_id", job.ID, "path", job.OriginalPath, "worker", workerID)
			p.claimsLost.Add(1)
			return true
		}
		p.logger.WarnContext(ctx, "avif backfill: mark succeeded", "job_id", job.ID, "error", markErr)
	}
	return true
}

func (p *AVIFBackfillProcessor) discoverMissing(ctx context.Context, stats *AVIFBackfillStats, onProgress func(float64, string)) error {
	if p.discover == nil {
		p.markDiscovered()
		return nil
	}
	onProgress(2, "Discovering WebP artwork missing AVIF siblings")
	inputs, err := p.discover.DiscoverMissing(ctx, avifBackfillDiscoverLimit)
	if err != nil {
		return fmt.Errorf("discovering missing AVIF siblings: %w", err)
	}
	if len(inputs) > 0 {
		ids, err := p.jobs.EnqueueBatch(ctx, inputs)
		if err != nil {
			return err
		}
		stats.EnqueuedExisting += len(ids)
	}
	// A full batch means the sweep stopped because it hit the batch size, not
	// because the corpus is covered — so let the next tick discover again instead
	// of idling for the discovery interval. With the queue now draining in
	// seconds, waiting fifteen minutes between batches is the difference between
	// clearing a backlog in minutes and in hours.
	if len(inputs) >= avifBackfillDiscoverLimit {
		return nil
	}
	p.markDiscovered()
	return nil
}

func (p *AVIFBackfillProcessor) discoveryDue() bool {
	p.discoveryMu.Lock()
	defer p.discoveryMu.Unlock()
	return p.lastDiscovery.IsZero() || time.Since(p.lastDiscovery) >= avifBackfillDiscoverEvery
}

func (p *AVIFBackfillProcessor) markDiscovered() {
	p.discoveryMu.Lock()
	p.lastDiscovery = time.Now()
	p.discoveryMu.Unlock()
}

func (p *AVIFBackfillProcessor) progressPercent(ctx context.Context) float64 {
	queued, err := p.jobs.QueuedCount(ctx)
	if err != nil || queued == 0 {
		return 50
	}
	// Rough: more queued → lower percent.
	pct := 90.0 / (1.0 + float64(queued)/50.0)
	if pct < 5 {
		return 5
	}
	return pct
}

func avifBackfillConcurrencyDefault() int {
	return ResolveAVIFBackfillWorkers(0)
}

// ResolveAVIFBackfillWorkers maps a configured worker count to a concrete
// concurrency. Values <= 0 mean the shared artwork encode budget.
func ResolveAVIFBackfillWorkers(configured int) int {
	return ResolveAVIFBackfillWorkersFor(configured, "", 0)
}

// ResolveAVIFBackfillWorkersFor is the backend-aware variant used at wiring
// time. When backend is "nvenc" and configured <= 0, concurrency follows the
// NVENC session cap (default 3); CPU backends follow the shared artwork encode
// budget, which stays well below the core count so playback keeps headroom.
func ResolveAVIFBackfillWorkersFor(configured int, backend string, nvencSessions int) int {
	if configured > 0 {
		return configured
	}
	if strings.EqualFold(strings.TrimSpace(backend), "nvenc") {
		if nvencSessions > 0 {
			return nvencSessions
		}
		return 3
	}
	n := imageutil.DefaultEncodeBudgetSize()
	if n < 1 {
		return 1
	}
	return n
}
