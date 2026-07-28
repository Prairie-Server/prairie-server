package metadata

import (
	"context"
	"testing"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
)

// present marks every display-ladder AVIF key for path as absent, which is what
// a never-encoded original looks like to discovery.
func missingLadder(path, imageType string) map[string]bool {
	missing := map[string]bool{}
	for _, key := range displayAVIFKeys(path, imageType) {
		missing[key] = true
	}
	return missing
}

// A catalog row names a cache path as soon as artwork is known, whether or not
// the image was ever fetched. Enqueueing such a path is a closed loop: the job
// fails avifBackfillMaxAttempts times, retires to failed, and the next discovery
// pass resets it to queued — so the queue never drains and the failed count
// stops meaning anything. Discovery must drop it instead.
func TestProbeMissingSkipsCandidatesWhoseOriginalIsGone(t *testing.T) {
	const (
		cached  = "tmdb/people/1234/profile/original.aaaa.webp"
		orphan  = "tmdb/people/5678/profile/original.bbbb.webp"
		profile = "profile"
	)

	missing := missingLadder(cached, profile)
	for key := range missingLadder(orphan, profile) {
		missing[key] = true
	}
	// The distinguishing fact: only the orphan's own WebP original is absent.
	missing[orphan] = true

	checker := &fakeObjectChecker{missing: missing}
	r := &AVIFSiblingReconciler{s3: checker}

	found, err := r.probeMissing(context.Background(), []string{cached, orphan}, 10)
	if err != nil {
		t.Fatalf("probeMissing: %v", err)
	}

	if len(found) != 1 {
		t.Fatalf("enqueued %d candidates, want 1: %+v", len(found), found)
	}
	if found[0].OriginalPath != cached {
		t.Errorf("enqueued %q, want the path whose original still exists (%q)", found[0].OriginalPath, cached)
	}
	if got := r.SkippedMissingOriginals(); got != 1 {
		t.Errorf("SkippedMissingOriginals() = %d, want 1", got)
	}
	// Reading clears, so the next pass reports its own gap rather than a total.
	if got := r.SkippedMissingOriginals(); got != 0 {
		t.Errorf("second read = %d, want 0 — the counter must clear on read", got)
	}
}

// The original probe costs one extra HEAD, so it must only fire for candidates
// that were about to be enqueued. Fully covered artwork is the common case and
// must not pay for it.
func TestProbeMissingDoesNotProbeOriginalWhenLadderIsComplete(t *testing.T) {
	const covered = "tmdb/movie/42/poster/original.cccc.webp"

	checker := &fakeObjectChecker{}
	r := &AVIFSiblingReconciler{s3: checker}

	found, err := r.probeMissing(context.Background(), []string{covered}, 10)
	if err != nil {
		t.Fatalf("probeMissing: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("enqueued %d candidates for fully covered artwork, want 0", len(found))
	}

	checker.mu.Lock()
	probedOriginal := checker.checked[covered]
	checker.mu.Unlock()
	if probedOriginal != 0 {
		t.Errorf("probed the original %d time(s) for covered artwork, want 0", probedOriginal)
	}
	if got := r.SkippedMissingOriginals(); got != 0 {
		t.Errorf("SkippedMissingOriginals() = %d, want 0", got)
	}
}

// A storage error while probing the original must fail the pass rather than be
// read as "the original is gone" — otherwise a flaky backend would quietly stop
// enqueueing real work.
func TestProbeMissingPropagatesOriginalProbeError(t *testing.T) {
	const path = "tmdb/people/9999/profile/original.dddd.webp"
	const profile = "profile"

	checker := &fakeObjectChecker{
		missing:  missingLadder(path, profile),
		erroring: map[string]bool{path: true},
	}
	r := &AVIFSiblingReconciler{s3: checker}

	if _, err := r.probeMissing(context.Background(), []string{path}, 10); err == nil {
		t.Fatal("probeMissing returned nil error when the original probe failed")
	}
	if got := r.SkippedMissingOriginals(); got != 0 {
		t.Errorf("SkippedMissingOriginals() = %d, want 0 — an error is not a missing original", got)
	}
}

// Guards the assumption the skip depends on: the path discovery hands back is
// itself the WebP original's object key, so probing it directly is meaningful.
func TestDisplayAVIFKeysExcludeTheOriginalKey(t *testing.T) {
	const path = "tmdb/people/1234/profile/original.aaaa.webp"
	for _, key := range displayAVIFKeys(path, "profile") {
		if key == path {
			t.Fatalf("display ladder contains the original key %q", key)
		}
	}
	if artworkkey.WebPAVIFSibling(path) == "" {
		t.Fatalf("%q is not recognized as a WebP original", path)
	}
}
