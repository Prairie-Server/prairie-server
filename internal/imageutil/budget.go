package imageutil

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
)

// encodeBudget caps in-flight WebP (WASM) + AVIF (native/WASM) work so a
// 4-core node is not oversubscribed by (WebP workers) + (AVIF workers) ×
// (SVT threads per encode). Size defaults to runtime.NumCPU().
var encodeBudget = newSlotBudget(defaultEncodeBudgetSize())

func defaultEncodeBudgetSize() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

// SetEncodeBudgetSize updates the shared WebP+AVIF encode slot count.
// Values <= 0 reset to runtime.NumCPU(). Hot-reload safe.
func SetEncodeBudgetSize(n int) {
	if n <= 0 {
		n = defaultEncodeBudgetSize()
	}
	encodeBudget.Resize(n)
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
