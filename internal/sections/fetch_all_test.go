package sections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestFetchAllWithRunnerPreservesOrderAndFallsBackOnErrors(t *testing.T) {
	t.Parallel()

	resolved := []ResolvedSection{
		{ID: "a", SectionType: SectionRecentlyAdded},
		{ID: "b", SectionType: SectionRandom},
		{ID: "c", SectionType: SectionRecentlyReleased},
	}

	results := fetchAllWithRunner(context.Background(), resolved, 2, func(_ context.Context, sec ResolvedSection) (SectionWithItems, error) {
		if sec.ID == "b" {
			return SectionWithItems{}, errors.New("boom")
		}
		return SectionWithItems{
			ResolvedSection: sec,
			Items:           []*models.MediaItem{{ContentID: sec.ID}},
			TotalCount:      1,
		}, nil
	})

	if len(results) != len(resolved) {
		t.Fatalf("results length = %d, want %d", len(results), len(resolved))
	}
	for i, result := range results {
		if result.ID != resolved[i].ID {
			t.Fatalf("result[%d].ID = %q, want %q", i, result.ID, resolved[i].ID)
		}
	}
	if len(results[1].Items) != 0 {
		t.Fatalf("error fallback item count = %d, want 0", len(results[1].Items))
	}
	if results[1].SectionType != SectionRandom {
		t.Fatalf("error fallback section type = %q, want %q", results[1].SectionType, SectionRandom)
	}
}

func TestFetchAllWithRunnerLimitsConcurrency(t *testing.T) {
	t.Parallel()

	resolved := []ResolvedSection{
		{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"},
		{ID: "e"}, {ID: "f"}, {ID: "g"}, {ID: "h"},
	}
	entered := make(chan string, len(resolved))
	release := make(chan struct{})

	done := make(chan []SectionWithItems, 1)
	go func() {
		done <- fetchAllWithRunner(context.Background(), resolved, 4, func(_ context.Context, sec ResolvedSection) (SectionWithItems, error) {
			entered <- sec.ID
			<-release
			return SectionWithItems{ResolvedSection: sec}, nil
		})
	}()

	for range 4 {
		<-entered
	}
	select {
	case id := <-entered:
		t.Fatalf("section %q started before a concurrency slot was released", id)
	case <-time.After(50 * time.Millisecond):
	}

	for range 4 {
		release <- struct{}{}
	}
	for range 4 {
		<-entered
	}
	for range 4 {
		release <- struct{}{}
	}

	select {
	case results := <-done:
		if len(results) != len(resolved) {
			t.Fatalf("results length = %d, want %d", len(results), len(resolved))
		}
	case <-time.After(time.Second):
		t.Fatal("fetchAllWithRunner did not finish")
	}
}
