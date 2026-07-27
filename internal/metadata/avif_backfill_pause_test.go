package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

type pauseTestJobStore struct {
	claimCalls int
	jobs       []*models.ArtworkAVIFBackfillJob
}

func (s *pauseTestJobStore) EnqueueBatch(context.Context, []AVIFBackfillEnqueueInput) ([]int64, error) {
	return nil, nil
}

func (s *pauseTestJobStore) ClaimDue(context.Context, string, int) ([]*models.ArtworkAVIFBackfillJob, error) {
	s.claimCalls++
	if len(s.jobs) == 0 {
		return nil, nil
	}
	batch := s.jobs
	s.jobs = nil
	return batch, nil
}

func (s *pauseTestJobStore) MarkSucceeded(context.Context, int64, string) error { return nil }

func (s *pauseTestJobStore) MarkFailed(context.Context, int64, int, string, string) error {
	return nil
}

func (s *pauseTestJobStore) RequeueClaimed(context.Context, []int64, string) error { return nil }

func (s *pauseTestJobStore) DeleteSucceededBefore(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func (s *pauseTestJobStore) QueuedCount(context.Context) (int, error) { return len(s.jobs), nil }

type countingEnsurer struct{ calls int }

func (e *countingEnsurer) EnsureAVIFSiblings(context.Context, string, string) error {
	e.calls++
	return nil
}

// Artwork encoding must stand down entirely while a playback session is live:
// the queue is durable, so the next tick resumes the drain.
func TestAVIFBackfillPausesWhilePlaybackActive(t *testing.T) {
	store := &pauseTestJobStore{jobs: []*models.ArtworkAVIFBackfillJob{{ID: 1, OriginalPath: "a.webp", ImageType: "poster"}}}
	ensurer := &countingEnsurer{}
	p := NewAVIFBackfillProcessor(store, ensurer)
	p.SetPlaybackActivityCheck(func() bool { return true })

	stats, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if !stats.PausedForPlayback {
		t.Fatal("stats.PausedForPlayback = false, want true")
	}
	if store.claimCalls != 0 {
		t.Fatalf("ClaimDue calls = %d, want 0 while playback is active", store.claimCalls)
	}
	if ensurer.calls != 0 {
		t.Fatalf("encode calls = %d, want 0 while playback is active", ensurer.calls)
	}
}

func TestAVIFBackfillDrainsWhenPlaybackIdle(t *testing.T) {
	store := &pauseTestJobStore{jobs: []*models.ArtworkAVIFBackfillJob{{ID: 1, OriginalPath: "a.webp", ImageType: "poster"}}}
	ensurer := &countingEnsurer{}
	p := NewAVIFBackfillProcessor(store, ensurer)
	p.SetPlaybackActivityCheck(func() bool { return false })

	stats, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if stats.PausedForPlayback {
		t.Fatal("stats.PausedForPlayback = true, want false when idle")
	}
	if stats.Succeeded != 1 {
		t.Fatalf("stats.Succeeded = %d, want 1", stats.Succeeded)
	}
	if ensurer.calls != 1 {
		t.Fatalf("encode calls = %d, want 1", ensurer.calls)
	}
}

// A nil predicate must not pause: single-node installs without the wiring keep
// the previous always-on behavior.
func TestAVIFBackfillWithoutActivityCheckRuns(t *testing.T) {
	store := &pauseTestJobStore{jobs: []*models.ArtworkAVIFBackfillJob{{ID: 1, OriginalPath: "a.webp", ImageType: "poster"}}}
	ensurer := &countingEnsurer{}
	p := NewAVIFBackfillProcessor(store, ensurer)
	p.SetPlaybackActivityCheck(nil)

	stats, err := p.RunUntilIdle(context.Background(), 1, time.Minute, nil)
	if err != nil {
		t.Fatalf("RunUntilIdle: %v", err)
	}
	if stats.PausedForPlayback || stats.Succeeded != 1 {
		t.Fatalf("stats = %+v, want one success and no pause", stats)
	}
}
