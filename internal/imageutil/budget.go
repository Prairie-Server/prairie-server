package imageutil

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
)

// encodeBudget caps in-flight WebP + AVIF work so a node is not oversubscribed
// by (WebP workers) + (AVIF workers) × (encoder threads). Native ffmpeg paths
// also pin -threads 1 / OMP_NUM_THREADS=1 and SVT lp=1 so each slot ≈ one core.
//
// Artwork encoding is fully deferrable background work that competes with
// playback ffmpeg for the same cores, so the default budget stays well below
// the core count instead of tracking it.
var encodeBudget = newSlotBudget(DefaultEncodeBudgetSize())

// encodeBudgetCoreDivisor and encodeBudgetCeiling shape the automatic budget:
// a quarter of the cores (1 on a 4-core node, 2 on 8) and never more than the
// ceiling, so a large host still leaves most of its cores for playback.
const (
	encodeBudgetCoreDivisor = 4
	encodeBudgetCeiling     = 4
)

// DefaultEncodeBudgetSize is the automatic shared WebP+AVIF encode slot count
// used when no explicit admin budget is configured.
func DefaultEncodeBudgetSize() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	// Round up so a 1-3 core node still gets one slot.
	budget := (n + encodeBudgetCoreDivisor - 1) / encodeBudgetCoreDivisor
	if budget > encodeBudgetCeiling {
		budget = encodeBudgetCeiling
	}
	if budget < 1 {
		budget = 1
	}
	return budget
}

// ResolveEncodeBudgetSize maps a configured artwork encode budget to a concrete
// slot count. Values <= 0 mean the automatic default.
func ResolveEncodeBudgetSize(configured int) int {
	if configured > 0 {
		return configured
	}
	return DefaultEncodeBudgetSize()
}

// SetEncodeBudgetSize updates the shared WebP+AVIF encode slot count.
// Values <= 0 reset to DefaultEncodeBudgetSize(). Hot-reload safe.
func SetEncodeBudgetSize(n int) {
	encodeBudget.Resize(ResolveEncodeBudgetSize(n))
}

// EncodeBudgetSize returns the current shared encode slot count.
func EncodeBudgetSize() int {
	return encodeBudget.Size()
}

type slotBudget struct {
	size atomic.Int32
	cur  atomic.Int32
	mu   sync.Mutex
	ch   chan struct{}
}

func newSlotBudget(n int) *slotBudget {
	if n < 1 {
		n = 1
	}
	b := &slotBudget{ch: make(chan struct{}, n)}
	b.size.Store(int32(n))
	for range n {
		b.ch <- struct{}{}
	}
	return b
}

func (b *slotBudget) Size() int {
	if b == nil {
		return 0
	}
	n := int(b.size.Load())
	if n < 1 {
		return 1
	}
	return n
}

func (b *slotBudget) Resize(n int) {
	if b == nil || n < 1 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cur := int(b.size.Load())
	if n == cur {
		return
	}
	// Drain and rebuild. Waiters on the old channel keep working until they
	// release; new acquires use the replacement channel.
	old := b.ch
	next := make(chan struct{}, n)
	for range n {
		next <- struct{}{}
	}
	b.ch = next
	b.size.Store(int32(n))
	// Best-effort: empty the old channel so abandoned tokens do not linger.
	go func() {
		for {
			select {
			case <-old:
			default:
				return
			}
		}
	}()
}

func (b *slotBudget) Acquire(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	ch := b.ch
	b.mu.Unlock()
	select {
	case <-ch:
		b.cur.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *slotBudget) Release() {
	if b == nil {
		return
	}
	b.cur.Add(-1)
	b.mu.Lock()
	ch := b.ch
	b.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default:
		// Resize shrank the buffer; drop the token.
	}
}
