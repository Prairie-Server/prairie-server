package catalog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

// fakeEnsurer records Ensure calls and blocks until released, so a test can
// observe repairs that are still running.
type fakeEnsurer struct {
	calls   atomic.Int32
	started chan int
	release chan struct{}

	mu          sync.Mutex
	needsRepair map[int]bool
}

func newFakeEnsurer() *fakeEnsurer {
	return &fakeEnsurer{
		started:     make(chan int, 64),
		release:     make(chan struct{}),
		needsRepair: map[int]bool{},
	}
}

func (f *fakeEnsurer) EnsureProbeOnly(ctx context.Context, file *models.MediaFile) (*models.MediaFile, error) {
	f.calls.Add(1)
	select {
	case f.started <- file.ID:
	default:
	}
	select {
	case <-f.release:
	case <-ctx.Done():
	}
	return file, nil
}

func (f *fakeEnsurer) EnsureCopySafetyCached(_ context.Context, file *models.MediaFile) (*models.MediaFile, error) {
	return file, nil
}

func (f *fakeEnsurer) NeedsRepair(file *models.MediaFile) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.needsRepair[file.ID]
}

func (f *fakeEnsurer) setNeedsRepair(ids ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		f.needsRepair[id] = true
	}
}

// fakeEnsurerNoCheck omits the optional NeedsRepair extension.
type fakeEnsurerNoCheck struct{ calls atomic.Int32 }

func (f *fakeEnsurerNoCheck) EnsureProbeOnly(_ context.Context, file *models.MediaFile) (*models.MediaFile, error) {
	f.calls.Add(1)
	return file, nil
}

func (f *fakeEnsurerNoCheck) EnsureCopySafetyCached(_ context.Context, file *models.MediaFile) (*models.MediaFile, error) {
	return file, nil
}

func mediaFiles(ids ...int) []*models.MediaFile {
	out := make([]*models.MediaFile, 0, len(ids))
	for _, id := range ids {
		out = append(out, &models.MediaFile{ID: id})
	}
	return out
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func TestPreparePlaybackFilesDeferredReturnsWithoutWaiting(t *testing.T) {
	ensurer := newFakeEnsurer()
	ensurer.setNeedsRepair(1, 2)
	defer close(ensurer.release)

	svc := &DetailService{probeEnsurer: ensurer}

	// Ensure blocks until released; the browse path must not wait on it.
	done := make(chan []*models.MediaFile, 1)
	go func() { done <- svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(1, 2)) }()

	select {
	case got := <-done:
		if len(got) != 2 {
			t.Fatalf("returned %d files, want 2", len(got))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("preparePlaybackFilesDeferred blocked on the probe")
	}

	if !waitFor(func() bool { return ensurer.calls.Load() > 0 }) {
		t.Fatal("repair was never scheduled")
	}
}

func TestPreparePlaybackFilesDeferredSurvivesRequestCancellation(t *testing.T) {
	ensurer := newFakeEnsurer()
	ensurer.setNeedsRepair(7)
	defer close(ensurer.release)

	svc := &DetailService{probeEnsurer: ensurer}

	// The request context is cancelled once the response is written; the repair
	// has to outlive it or the row never converges.
	ctx, cancel := context.WithCancel(context.Background())
	svc.preparePlaybackFilesDeferred(ctx, mediaFiles(7))
	cancel()

	select {
	case id := <-ensurer.started:
		if id != 7 {
			t.Fatalf("probed file %d, want 7", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("repair did not start after the request context was cancelled")
	}
}

func TestPreparePlaybackFilesDeferredSkipsHealthyFiles(t *testing.T) {
	ensurer := newFakeEnsurer()
	ensurer.setNeedsRepair(2) // file 1 is already complete
	defer close(ensurer.release)

	svc := &DetailService{probeEnsurer: ensurer}
	svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(1, 2))

	select {
	case id := <-ensurer.started:
		if id != 2 {
			t.Fatalf("probed file %d, want only 2", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("repair never started")
	}
	if got := ensurer.calls.Load(); got != 1 {
		t.Fatalf("Ensure called %d times, want 1", got)
	}
}

func TestPreparePlaybackFilesDeferredDeduplicatesInFlightFiles(t *testing.T) {
	ensurer := newFakeEnsurer()
	ensurer.setNeedsRepair(5)
	defer close(ensurer.release)

	svc := &DetailService{probeEnsurer: ensurer}

	// Browsing issues overlapping detail reads over the same files; ffprobe must
	// not start once per read.
	for range 5 {
		svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(5))
	}

	if !waitFor(func() bool { return ensurer.calls.Load() >= 1 }) {
		t.Fatal("repair never started")
	}
	time.Sleep(50 * time.Millisecond)
	if got := ensurer.calls.Load(); got != 1 {
		t.Fatalf("Ensure called %d times for one in-flight file, want 1", got)
	}
}

func TestPreparePlaybackFilesDeferredBoundsConcurrency(t *testing.T) {
	ensurer := newFakeEnsurer()
	ids := []int{1, 2, 3, 4, 5, 6}
	ensurer.setNeedsRepair(ids...)
	defer close(ensurer.release)

	svc := &DetailService{probeEnsurer: ensurer}
	svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(ids...))

	// Every file is claimed, but only probeBackfillConcurrency may run at once.
	if !waitFor(func() bool { return ensurer.calls.Load() >= probeBackfillConcurrency }) {
		t.Fatal("no probes started")
	}
	time.Sleep(50 * time.Millisecond)
	if got := int(ensurer.calls.Load()); got > probeBackfillConcurrency {
		t.Fatalf("%d probes running at once, want at most %d", got, probeBackfillConcurrency)
	}
}

func TestPreparePlaybackFilesDeferredReleasesFileAfterRepair(t *testing.T) {
	ensurer := newFakeEnsurer()
	ensurer.setNeedsRepair(9)
	svc := &DetailService{probeEnsurer: ensurer}

	svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(9))
	if !waitFor(func() bool { return ensurer.calls.Load() == 1 }) {
		t.Fatal("first repair never started")
	}
	close(ensurer.release)
	if !waitFor(func() bool {
		svc.probeBackfill.mu.Lock()
		defer svc.probeBackfill.mu.Unlock()
		return len(svc.probeBackfill.pending) == 0
	}) {
		t.Fatal("file was never released from the pending set")
	}

	// A later request for a still-broken file can schedule again.
	svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(9))
	if !waitFor(func() bool { return ensurer.calls.Load() == 2 }) {
		t.Fatal("repair was not re-scheduled after the previous one finished")
	}
}

func TestPreparePlaybackFilesDeferredWithoutEnsurer(t *testing.T) {
	svc := &DetailService{}
	got := svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(1, 2))
	if len(got) != 2 {
		t.Fatalf("returned %d files, want 2", len(got))
	}
}

func TestPreparePlaybackFilesDeferredEmptyAndNilEntries(t *testing.T) {
	ensurer := newFakeEnsurer()
	defer close(ensurer.release)
	svc := &DetailService{probeEnsurer: ensurer}

	if got := svc.preparePlaybackFilesDeferred(context.Background(), nil); got != nil {
		t.Fatalf("nil input returned %v, want nil", got)
	}
	got := svc.preparePlaybackFilesDeferred(context.Background(), []*models.MediaFile{nil, {ID: 3}})
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("nil entries were not dropped: %v", got)
	}
}

func TestPreparePlaybackFilesDeferredSkipsUnsavedFiles(t *testing.T) {
	ensurer := newFakeEnsurer()
	ensurer.setNeedsRepair(0)
	defer close(ensurer.release)

	svc := &DetailService{probeEnsurer: ensurer}
	// A row with no id cannot be upserted, so there is nothing to converge on.
	svc.preparePlaybackFilesDeferred(context.Background(), mediaFiles(0))

	time.Sleep(50 * time.Millisecond)
	if got := ensurer.calls.Load(); got != 0 {
		t.Fatalf("Ensure called %d times for an unsaved file, want 0", got)
	}
}

func TestWantsProbeRepairWithoutOptionalChecker(t *testing.T) {
	// An ensurer that cannot report need is asked to repair everything; Ensure
	// no-ops on healthy files, so this stays correct.
	if !wantsProbeRepair(&fakeEnsurerNoCheck{}, &models.MediaFile{ID: 1}) {
		t.Fatal("want true when the ensurer has no NeedsRepair extension")
	}
}
