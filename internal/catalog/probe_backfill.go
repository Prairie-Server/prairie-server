package catalog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

// Background repair of missing playback-critical probe metadata.
//
// `preparePlaybackFiles` runs ffprobe inline for any file whose stored probe
// data is incomplete. On the playback-decision paths that is correct and must
// stay: the transcode choice depends on knowing the real codecs, so answering
// from incomplete data would be answering wrongly.
//
// On the browse paths it is not. Opening a title's detail page blocked on a
// local ffprobe — up to five seconds, or up to a minute for a file whose
// duration is implausible enough to need a packet scan (see
// scanner.reprobeMayScanPackets). The page uses this data for resolution badges
// and track lists; none of it is worth making the viewer wait on, and the repair
// persists, so the *next* visit is fast either way. Deferring it keeps that
// self-healing property while taking the probe off the request.
//
// Three properties matter here:
//
//   - The work must outlive the request. The request context is cancelled as
//     soon as the response is written, so a probe inheriting it would be killed
//     before finishing and the row would never converge.
//   - The same file must not be probed twice concurrently. Browsing a library
//     issues many detail reads over overlapping files, and ffprobe is expensive.
//   - Total concurrency must be bounded. Without a ceiling, scrolling a library
//     of unprobed files would spawn one ffprobe process per file.

const (
	// Concurrent background probes. ffprobe is IO- and CPU-heavy and this runs
	// alongside transcodes, so keep it to a trickle: the goal is convergence
	// over time, not speed.
	probeBackfillConcurrency = 2

	// Ceiling for one deferred repair. Generous enough for the packet-scan
	// fallback, which is the case the request path could never afford.
	probeBackfillTimeout = 2 * time.Minute
)

// probeRepairChecker is the optional PlaybackProbeEnsurer extension that
// reports whether a file would actually be reprobed.
//
// `scanner` imports `catalog`, so the predicate itself cannot be called from
// here. Ensure already no-ops on a healthy file, so this is only an
// optimisation — without it every browse response would start a goroutine per
// file just to have it return immediately. An ensurer that does not implement
// it stays correct, only busier.
type probeRepairChecker interface {
	NeedsRepair(file *models.MediaFile) bool
}

// wantsProbeRepair reports whether `file` is worth handing to the backfiller.
func wantsProbeRepair(ensurer PlaybackProbeEnsurer, file *models.MediaFile) bool {
	if checker, ok := ensurer.(probeRepairChecker); ok {
		return checker.NeedsRepair(file)
	}
	return true
}

// probeBackfiller repairs probe metadata off the request path.
type probeBackfiller struct {
	ensurer PlaybackProbeEnsurer

	mu      sync.Mutex
	pending map[int]struct{}
	slots   chan struct{}
}

func newProbeBackfiller(ensurer PlaybackProbeEnsurer) *probeBackfiller {
	return &probeBackfiller{
		ensurer: ensurer,
		pending: make(map[int]struct{}),
		slots:   make(chan struct{}, probeBackfillConcurrency),
	}
}

// claim reserves a file for repair, reporting false when one is already queued
// or running for it.
func (b *probeBackfiller) claim(fileID int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.pending[fileID]; exists {
		return false
	}
	b.pending[fileID] = struct{}{}
	return true
}

func (b *probeBackfiller) release(fileID int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pending, fileID)
}

// schedule queues repair for every file that needs it and returns immediately.
//
// `ctx` is used only to carry request-scoped logging attributes; the repair
// itself runs on a detached context so finishing the response cannot cancel it.
func (b *probeBackfiller) schedule(ctx context.Context, files []*models.MediaFile) {
	if b == nil || b.ensurer == nil {
		return
	}
	for _, file := range files {
		if file == nil || file.ID <= 0 {
			continue
		}
		if !b.claim(file.ID) {
			continue
		}
		go b.run(ctx, file)
	}
}

func (b *probeBackfiller) run(ctx context.Context, file *models.MediaFile) {
	defer b.release(file.ID)

	b.slots <- struct{}{}
	defer func() { <-b.slots }()

	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeBackfillTimeout)
	defer cancel()

	if _, err := b.ensurer.EnsureProbeOnly(probeCtx, file); err != nil {
		slog.WarnContext(probeCtx, "background probe repair failed",
			"component", "catalog",
			"file_id", file.ID,
			"error", err,
		)
	}
}

// preparePlaybackFilesDeferred returns the stored rows as they are and repairs
// any missing probe metadata in the background.
//
// For the browse/detail responses only — playback decisions use
// preparePlaybackFiles, which waits.
func (s *DetailService) preparePlaybackFilesDeferred(
	ctx context.Context,
	files []*models.MediaFile,
) []*models.MediaFile {
	if len(files) == 0 {
		return files
	}

	prepared := make([]*models.MediaFile, 0, len(files))
	needsRepair := make([]*models.MediaFile, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		if s.probeEnsurer != nil && wantsProbeRepair(s.probeEnsurer, file) {
			needsRepair = append(needsRepair, file)
		}
		prepared = append(prepared, file)
	}

	if len(needsRepair) > 0 {
		s.probeBackfillOnce.Do(func() {
			s.probeBackfill = newProbeBackfiller(s.probeEnsurer)
		})
		s.probeBackfill.schedule(ctx, needsRepair)
	}

	return prepared
}
