package triggers

import (
	"sync"
	"time"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// IntervalTrigger fires every N milliseconds, measured from completion of the
// previous run (not from start). If no previous run exists, it fires
// interval-after-Start().
//
// After firing it re-arms itself rather than waiting to be restarted. The
// manager still calls Start again once the run completes, which re-anchors the
// schedule to the completion time; self-arming only decides what happens when
// that call never comes. It has to, because several manager paths consume a
// firing and then continue without re-arming — a trigger update racing the
// firing, a run rejected as already-running, or a second trigger winning the
// merge and the loser's signal being dropped. A one-shot timer left the task
// silently unscheduled until the next process restart, with nothing logged and
// no way to tell from the outside.
//
// An interval of zero or less cannot self-arm: it would spin. Such a trigger
// keeps the old one-shot behavior of firing immediately, once.
type IntervalTrigger struct {
	cfg      taskmanager.TriggerConfig
	interval time.Duration
	ch       chan struct{}
	nextRun  time.Time
	timer    *time.Timer
	stopCh   chan struct{}
	mu       sync.Mutex
}

func NewIntervalTrigger(cfg taskmanager.TriggerConfig) *IntervalTrigger {
	return &IntervalTrigger{
		cfg:      cfg,
		interval: time.Duration(cfg.IntervalMs) * time.Millisecond,
		ch:       make(chan struct{}, 1),
	}
}

func (t *IntervalTrigger) Start(lastResult *taskmanager.ExecutionResult) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Stop any arming loop still running from a previous Start so two loops
	// never share t.ch. Callers are expected to Stop first, but Start alone
	// must not leak a ticking goroutine.
	t.stopLocked()

	// Drain any stale signal from a previous timer fire.
	select {
	case <-t.ch:
	default:
	}

	var base time.Time
	if lastResult != nil && !lastResult.CompletedAt.IsZero() {
		base = lastResult.CompletedAt
	} else {
		base = time.Now()
	}

	t.nextRun = base.Add(t.interval)
	delay := time.Until(t.nextRun)
	if delay < 0 {
		delay = 0
		t.nextRun = time.Now()
	}

	stopCh := make(chan struct{})
	timer := time.NewTimer(delay)
	t.stopCh = stopCh
	t.timer = timer
	selfArming := t.interval > 0

	go func() {
		defer timer.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-timer.C:
				select {
				case t.ch <- struct{}{}:
				default:
				}
				if !selfArming {
					return
				}
				t.mu.Lock()
				// A concurrent Start replaced this loop: let that one own the
				// schedule instead of arming a second timer against t.ch.
				if t.stopCh != stopCh {
					t.mu.Unlock()
					return
				}
				t.nextRun = time.Now().Add(t.interval)
				t.mu.Unlock()
				timer.Reset(t.interval)
			}
		}
	}()
}

func (t *IntervalTrigger) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopLocked()
}

// stopLocked signals the arming loop to exit. Caller holds mu.
func (t *IntervalTrigger) stopLocked() {
	if t.stopCh == nil {
		return
	}
	select {
	case <-t.stopCh:
	default:
		close(t.stopCh)
	}
}

func (t *IntervalTrigger) NextRunTime() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.nextRun
}

func (t *IntervalTrigger) Config() taskmanager.TriggerConfig { return t.cfg }
func (t *IntervalTrigger) C() <-chan struct{}                { return t.ch }
