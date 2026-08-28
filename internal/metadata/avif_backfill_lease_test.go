package metadata

import "testing"

// A claim must outlive the longest pass that can hold it.
//
// ClaimDue calls recoverExpiredRunning first, so with a lease shorter than the
// runtime cap a single pass expires its own in-flight jobs at the next claim
// batch: the row returns to queued, another batch re-encodes it, and the
// original worker's MarkSucceeded no longer matches locked_by — the success is
// discarded and the work repeats. It was 5 minutes against an 8-minute cap.
func TestBackfillLeaseOutlivesAPass(t *testing.T) {
	if avifBackfillLeaseDuration <= avifBackfillMaxRuntime {
		t.Fatalf("lease %s must exceed the pass runtime cap %s, or a pass expires its own claims",
			avifBackfillLeaseDuration, avifBackfillMaxRuntime)
	}
}

// The scan cursor is what lets a window reach past the head of the corpus.
func TestScanCursorRotatesAndResets(t *testing.T) {
	var c scanCursor

	if got := c.next(100); got != 0 {
		t.Fatalf("first offset = %d, want 0", got)
	}
	if got := c.next(100); got != 100 {
		t.Fatalf("second offset = %d, want 100", got)
	}
	if got := c.next(50); got != 200 {
		t.Fatalf("third offset = %d, want 200", got)
	}

	c.reset()
	if got := c.next(100); got != 0 {
		t.Fatalf("offset after reset = %d, want 0", got)
	}
}

// Concurrent passes must not hand the same offset to two scanners.
func TestScanCursorIsUniquePerCall(t *testing.T) {
	var c scanCursor
	const calls = 64
	seen := make(chan int, calls)
	done := make(chan struct{})

	for range calls {
		go func() {
			seen <- c.next(10)
			done <- struct{}{}
		}()
	}
	for range calls {
		<-done
	}
	close(seen)

	offsets := map[int]bool{}
	for offset := range seen {
		if offsets[offset] {
			t.Fatalf("offset %d handed out twice", offset)
		}
		offsets[offset] = true
	}
	if len(offsets) != calls {
		t.Fatalf("got %d distinct offsets, want %d", len(offsets), calls)
	}
}
