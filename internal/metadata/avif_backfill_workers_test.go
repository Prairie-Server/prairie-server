package metadata

import (
	"runtime"
	"testing"

	"github.com/prairie-server/prairie-server/internal/imageutil"
)

func TestResolveAVIFBackfillWorkers(t *testing.T) {
	t.Parallel()
	if got := ResolveAVIFBackfillWorkers(4); got != 4 {
		t.Fatalf("ResolveAVIFBackfillWorkers(4) = %d, want 4", got)
	}
	want := imageutil.DefaultEncodeBudgetSize()
	if want < 1 {
		want = 1
	}
	if got := ResolveAVIFBackfillWorkers(0); got != want {
		t.Fatalf("ResolveAVIFBackfillWorkers(0) = %d, want artwork encode budget %d", got, want)
	}
	if cores := runtime.NumCPU(); cores >= 4 && want > cores/2 {
		t.Fatalf("auto AVIF workers = %d on %d cores, want well below the core count", want, cores)
	}
	if got := ResolveAVIFBackfillWorkersFor(0, "nvenc", 0); got != 3 {
		t.Fatalf("ResolveAVIFBackfillWorkersFor(nvenc) = %d, want 3", got)
	}
	if got := ResolveAVIFBackfillWorkersFor(0, "nvenc", 5); got != 5 {
		t.Fatalf("ResolveAVIFBackfillWorkersFor(nvenc,5) = %d, want 5", got)
	}
}

func TestAVIFBackfillProcessorWorkersDefaultToEncodeBudget(t *testing.T) {
	t.Parallel()
	p := NewAVIFBackfillProcessor(nil, nil)
	want := ResolveAVIFBackfillWorkers(0)
	if got := p.Workers(); got != want {
		t.Fatalf("Workers() = %d, want %d", got, want)
	}
	p.SetWorkers(3)
	if got := p.Workers(); got != 3 {
		t.Fatalf("Workers() after SetWorkers(3) = %d, want 3", got)
	}
	p.SetWorkers(0)
	if got := p.Workers(); got != want {
		t.Fatalf("Workers() after SetWorkers(0) = %d, want %d", got, want)
	}
}
