package imageutil

import (
	"runtime"
	"testing"
)

// The automatic artwork budget must stay well below the core count: artwork
// encoding is deferrable and shares cores with playback ffmpeg.
func TestDefaultEncodeBudgetStaysBelowCoreCount(t *testing.T) {
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}
	got := DefaultEncodeBudgetSize()
	if got < 1 {
		t.Fatalf("DefaultEncodeBudgetSize() = %d, want >= 1", got)
	}
	if got > encodeBudgetCeiling {
		t.Fatalf("DefaultEncodeBudgetSize() = %d, want <= ceiling %d", got, encodeBudgetCeiling)
	}
	if cores >= 4 && got > cores/2 {
		t.Fatalf("DefaultEncodeBudgetSize() = %d on %d cores, want well below the core count", got, cores)
	}
}

func TestResolveEncodeBudgetSize(t *testing.T) {
	if got := ResolveEncodeBudgetSize(2); got != 2 {
		t.Fatalf("ResolveEncodeBudgetSize(2) = %d, want 2", got)
	}
	want := DefaultEncodeBudgetSize()
	for _, configured := range []int{0, -1} {
		if got := ResolveEncodeBudgetSize(configured); got != want {
			t.Fatalf("ResolveEncodeBudgetSize(%d) = %d, want default %d", configured, got, want)
		}
	}
}

func TestSetEncodeBudgetSizeRoundTrip(t *testing.T) {
	original := EncodeBudgetSize()
	t.Cleanup(func() { SetEncodeBudgetSize(original) })

	SetEncodeBudgetSize(1)
	if got := EncodeBudgetSize(); got != 1 {
		t.Fatalf("EncodeBudgetSize() = %d, want 1", got)
	}
	SetEncodeBudgetSize(0)
	if got, want := EncodeBudgetSize(), DefaultEncodeBudgetSize(); got != want {
		t.Fatalf("EncodeBudgetSize() after reset = %d, want %d", got, want)
	}
}
