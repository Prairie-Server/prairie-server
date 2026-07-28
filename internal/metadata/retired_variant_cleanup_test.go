package metadata

import (
	"context"
	"errors"
	"testing"
)

type fakeRetiredStore struct {
	present  map[string]bool
	probed   []string
	deletedK []string
	existErr error
	delErr   error
}

func (s *fakeRetiredStore) ObjectExists(_ context.Context, _, key string) (bool, error) {
	s.probed = append(s.probed, key)
	if s.existErr != nil {
		return false, s.existErr
	}
	return s.present[key], nil
}

func (s *fakeRetiredStore) DeleteObject(_ context.Context, _, key string) error {
	if s.delErr != nil {
		return s.delErr
	}
	s.deletedK = append(s.deletedK, key)
	return nil
}

func (s *fakeRetiredStore) Bucket() string { return "artwork" }

func TestRetiredVariantCleanerDeletesOnlyWhatExists(t *testing.T) {
	store := &fakeRetiredStore{present: map[string]bool{
		"art/ep1/still/w200.3.webp": true,
		"art/ep1/still/w200.3.avif": true,
		// no .png sibling on disk
	}}
	cleaner := &RetiredVariantCleaner{pool: nil, s3: store}

	checked, deleted, err := cleaner.sweepPaths(context.Background(), []string{"art/ep1/still/original.3.webp"})
	if err != nil {
		t.Fatalf("sweepPaths() error = %v", err)
	}
	if checked != 3 {
		t.Errorf("checked = %d, want 3 (webp + avif + png for the retired rung)", checked)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	want := []string{"art/ep1/still/w200.3.webp", "art/ep1/still/w200.3.avif"}
	if len(store.deletedK) != len(want) {
		t.Fatalf("deleted keys = %v, want %v", store.deletedK, want)
	}
	for i, key := range want {
		if store.deletedK[i] != key {
			t.Errorf("deleted[%d] = %q, want %q", i, store.deletedK[i], key)
		}
	}
}

// The live rungs must never be probed, let alone deleted — that would erase
// artwork the ladder is actively serving.
func TestRetiredVariantCleanerNeverTouchesLiveRungs(t *testing.T) {
	store := &fakeRetiredStore{present: map[string]bool{
		"art/ep1/still/w300.3.webp": true,
		"art/ep1/still/w500.3.webp": true,
	}}
	cleaner := &RetiredVariantCleaner{pool: nil, s3: store}

	if _, deleted, err := cleaner.sweepPaths(context.Background(), []string{"art/ep1/still/original.3.webp"}); err != nil || deleted != 0 {
		t.Fatalf("sweepPaths() = %d deleted, %v; want 0, nil", deleted, err)
	}
	for _, key := range store.probed {
		if key == "art/ep1/still/w300.3.webp" || key == "art/ep1/still/w500.3.webp" {
			t.Errorf("probed a live rung: %q", key)
		}
	}
	if len(store.deletedK) != 0 {
		t.Errorf("deleted live rungs: %v", store.deletedK)
	}
}

// Types that never retired a rung cost nothing: no HEAD requests at all.
func TestRetiredVariantCleanerSkipsTypesWithNoRetiredRungs(t *testing.T) {
	store := &fakeRetiredStore{present: map[string]bool{}}
	cleaner := &RetiredVariantCleaner{pool: nil, s3: store}

	paths := []string{
		"art/m1/poster/original.1.webp",
		"art/m1/backdrop/original.1.webp",
		"art/m1/logo/original.1.webp",
		"art/p1/profile/original.1.webp",
	}
	checked, deleted, err := cleaner.sweepPaths(context.Background(), paths)
	if err != nil {
		t.Fatalf("sweepPaths() error = %v", err)
	}
	if checked != 0 || deleted != 0 {
		t.Errorf("checked/deleted = %d/%d, want 0/0", checked, deleted)
	}
	if len(store.probed) != 0 {
		t.Errorf("probed %v, want no requests for types with nothing retired", store.probed)
	}
}

func TestRetiredVariantCleanerStopsOnProbeError(t *testing.T) {
	store := &fakeRetiredStore{present: map[string]bool{}, existErr: errors.New("head failed")}
	cleaner := &RetiredVariantCleaner{pool: nil, s3: store}

	if _, _, err := cleaner.sweepPaths(context.Background(), []string{"art/ep1/still/original.3.webp"}); err == nil {
		t.Fatal("expected the probe error to surface")
	}
}

func TestRetiredVariantCleanerStopsOnDeleteError(t *testing.T) {
	store := &fakeRetiredStore{
		present:  map[string]bool{"art/ep1/still/w200.3.webp": true},
		delErr:   errors.New("delete failed"),
		existErr: nil,
	}
	cleaner := &RetiredVariantCleaner{pool: nil, s3: store}

	if _, deleted, err := cleaner.sweepPaths(context.Background(), []string{"art/ep1/still/original.3.webp"}); err == nil || deleted != 0 {
		t.Fatalf("sweepPaths() = %d deleted, %v; want 0 and an error", deleted, err)
	}
}

func TestRetiredVariantCleanerHonoursCancellation(t *testing.T) {
	store := &fakeRetiredStore{present: map[string]bool{}}
	cleaner := &RetiredVariantCleaner{pool: nil, s3: store}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := cleaner.sweepPaths(ctx, []string{"art/ep1/still/original.3.webp"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("sweepPaths() error = %v, want context.Canceled", err)
	}
}

func TestNewRetiredVariantCleanerRequiresDependencies(t *testing.T) {
	if got := NewRetiredVariantCleaner(nil, &fakeRetiredStore{}); got != nil {
		t.Error("expected nil cleaner without a pool")
	}
	if got := NewRetiredVariantCleaner(nil, nil); got != nil {
		t.Error("expected nil cleaner without a store")
	}
}

// A nil cleaner must be inert rather than panic: main.go leaves the sweeper
// unset when the deployment has no artwork object deleter.
func TestRetiredVariantCleanerNilIsInert(t *testing.T) {
	var cleaner *RetiredVariantCleaner
	checked, deleted, err := cleaner.CleanupRetiredVariants(context.Background(), 10)
	if checked != 0 || deleted != 0 || err != nil {
		t.Errorf("nil cleaner = %d/%d/%v, want 0/0/nil", checked, deleted, err)
	}
}
