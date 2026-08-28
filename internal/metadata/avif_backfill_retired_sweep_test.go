package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

type countingRetiredSweeper struct {
	calls   int
	checked int
	deleted int
	err     error
}

func (s *countingRetiredSweeper) CleanupRetiredVariants(context.Context, int) (int, int, error) {
	s.calls++
	return s.checked, s.deleted, s.err
}

// Sweeping competes with real encodes for the same cores, so it waits until the
// queue has nothing to claim.
func TestRetiredSweepWaitsUntilTheQueueIsIdle(t *testing.T) {
	store := &pauseTestJobStore{jobs: []*models.ArtworkAVIFBackfillJob{
		{ID: 1, OriginalPath: "a.webp", ImageType: "poster"},
	}}
	sweeper := &countingRetiredSweeper{}
	p := NewAVIFBackfillProcessor(store, &countingEnsurer{})
	p.SetRetiredVariantSweeper(sweeper)

	stats, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if stats.Claimed == 0 {
		t.Fatal("expected the run to claim the queued job")
	}
	if sweeper.calls != 0 {
		t.Errorf("sweeper ran %d times during a working pass, want 0", sweeper.calls)
	}
}

func TestRetiredSweepRunsAndReportsWhenIdle(t *testing.T) {
	sweeper := &countingRetiredSweeper{checked: 12, deleted: 5}
	p := NewAVIFBackfillProcessor(&pauseTestJobStore{}, &countingEnsurer{})
	p.SetRetiredVariantSweeper(sweeper)

	stats, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if sweeper.calls != 1 {
		t.Fatalf("sweeper ran %d times, want 1", sweeper.calls)
	}
	if stats.RetiredChecked != 12 || stats.RetiredDeleted != 5 {
		t.Errorf("stats retired = %d checked / %d deleted, want 12/5", stats.RetiredChecked, stats.RetiredDeleted)
	}
}

// A failing sweep is a cleanup problem, not a backfill problem: the encode pass
// must still report success so the schedule keeps draining artwork.
func TestRetiredSweepFailureDoesNotFailTheRun(t *testing.T) {
	sweeper := &countingRetiredSweeper{err: errors.New("bucket unreachable")}
	p := NewAVIFBackfillProcessor(&pauseTestJobStore{}, &countingEnsurer{})
	p.SetRetiredVariantSweeper(sweeper)

	if _, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil); err != nil {
		t.Fatalf("RunUntilIdle returned %v, want nil despite the sweep failing", err)
	}
}

// Playback pre-empts the whole pass, sweeping included.
func TestRetiredSweepSkippedWhilePlaybackActive(t *testing.T) {
	sweeper := &countingRetiredSweeper{}
	p := NewAVIFBackfillProcessor(&pauseTestJobStore{}, &countingEnsurer{})
	p.SetRetiredVariantSweeper(sweeper)
	p.SetPlaybackActivityCheck(func() bool { return true })

	if _, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil); err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if sweeper.calls != 0 {
		t.Errorf("sweeper ran %d times while playback was active, want 0", sweeper.calls)
	}
}

// An unset sweeper is the normal state for deployments without an artwork
// object deleter, and must not affect the run.
func TestRunUntilIdleWithoutASweeper(t *testing.T) {
	p := NewAVIFBackfillProcessor(&pauseTestJobStore{}, &countingEnsurer{})

	stats, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if stats.RetiredChecked != 0 || stats.RetiredDeleted != 0 {
		t.Errorf("stats retired = %d/%d, want 0/0 with no sweeper", stats.RetiredChecked, stats.RetiredDeleted)
	}
}

type batchDiscoverer struct {
	calls int
	batch []AVIFBackfillEnqueueInput
}

func (d *batchDiscoverer) DiscoverMissing(context.Context, int) ([]AVIFBackfillEnqueueInput, error) {
	d.calls++
	return d.batch, nil
}

type recordingJobStore struct {
	pauseTestJobStore
	enqueued int
}

func (s *recordingJobStore) EnqueueBatch(_ context.Context, in []AVIFBackfillEnqueueInput) ([]int64, error) {
	s.enqueued += len(in)
	ids := make([]int64, len(in))
	for i := range in {
		ids[i] = int64(i + 1)
	}
	return ids, nil
}

func fullBatch(n int) []AVIFBackfillEnqueueInput {
	out := make([]AVIFBackfillEnqueueInput, n)
	for i := range out {
		out[i] = AVIFBackfillEnqueueInput{OriginalPath: "p.webp", ImageType: "profile"}
	}
	return out
}

// A full batch means the sweep stopped on batch size, not because the corpus is
// covered, so the next pass must discover again rather than idling out the
// discovery interval — otherwise a backlog clears one batch per interval.
func TestDiscoveryRepeatsImmediatelyAfterAFullBatch(t *testing.T) {
	discoverer := &batchDiscoverer{batch: fullBatch(avifBackfillDiscoverLimit)}
	p := NewAVIFBackfillProcessor(&recordingJobStore{}, &countingEnsurer{})
	p.SetDiscoverer(discoverer)

	for range 3 {
		if _, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil); err != nil {
			t.Fatalf("RunUntilIdle: %v", err)
		}
	}
	if discoverer.calls != 3 {
		t.Fatalf("discovery ran %d times, want 3 (a full batch must not start the interval)", discoverer.calls)
	}
}

// A short batch means the sweep saw everything it could, so the interval applies.
func TestDiscoveryWaitsForTheIntervalAfterAShortBatch(t *testing.T) {
	discoverer := &batchDiscoverer{batch: fullBatch(1)}
	p := NewAVIFBackfillProcessor(&recordingJobStore{}, &countingEnsurer{})
	p.SetDiscoverer(discoverer)

	for range 3 {
		if _, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil); err != nil {
			t.Fatalf("RunUntilIdle: %v", err)
		}
	}
	if discoverer.calls != 1 {
		t.Fatalf("discovery ran %d times, want 1 (a short batch starts the interval)", discoverer.calls)
	}
}
