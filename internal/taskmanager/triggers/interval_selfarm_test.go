package triggers

import (
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// The manager consumes a firing and then continues without re-arming on
// several paths (trigger update racing the firing, run rejected as already
// running, losing the merge race). A one-shot timer left the task silently
// unscheduled until the process restarted, so the trigger must keep ticking on
// its own.
func TestIntervalTriggerFiresAgainWithoutBeingRearmed(t *testing.T) {
	trigger := NewIntervalTrigger(taskmanager.TriggerConfig{
		Type:       taskmanager.TriggerTypeInterval,
		IntervalMs: 25,
	})
	t.Cleanup(trigger.Stop)

	trigger.Start(nil)

	for i := range 3 {
		select {
		case <-trigger.C():
		case <-time.After(2 * time.Second):
			t.Fatalf("interval trigger stopped firing after %d fires without a rearm", i)
		}
	}
}

func TestIntervalTriggerStopEndsTheSchedule(t *testing.T) {
	trigger := NewIntervalTrigger(taskmanager.TriggerConfig{
		Type:       taskmanager.TriggerTypeInterval,
		IntervalMs: 20,
	})

	trigger.Start(nil)
	select {
	case <-trigger.C():
	case <-time.After(2 * time.Second):
		t.Fatal("interval trigger never fired")
	}

	trigger.Stop()
	// Drain a firing that may have landed concurrently with Stop, then require
	// silence: a stopped trigger must not keep arming itself.
	select {
	case <-trigger.C():
	default:
	}

	select {
	case <-trigger.C():
		t.Fatal("interval trigger fired after Stop")
	case <-time.After(150 * time.Millisecond):
	}
}

// Restarting must not leave the previous arming loop running: two loops sharing
// the channel would double the effective rate and outlive a single Stop.
func TestIntervalTriggerRestartDoesNotLeakPreviousLoop(t *testing.T) {
	trigger := NewIntervalTrigger(taskmanager.TriggerConfig{
		Type:       taskmanager.TriggerTypeInterval,
		IntervalMs: 20,
	})

	trigger.Start(nil)
	trigger.Start(nil)
	trigger.Start(nil)

	select {
	case <-trigger.C():
	case <-time.After(2 * time.Second):
		t.Fatal("interval trigger never fired after repeated Start")
	}

	trigger.Stop()
	select {
	case <-trigger.C():
	default:
	}

	select {
	case <-trigger.C():
		t.Fatal("a leaked arming loop kept firing after Stop")
	case <-time.After(150 * time.Millisecond):
	}
}

// A zero interval cannot self-arm without spinning, so it keeps the original
// one-shot behavior.
func TestIntervalTriggerZeroIntervalFiresOnceWithoutSpinning(t *testing.T) {
	trigger := NewIntervalTrigger(taskmanager.TriggerConfig{
		Type:       taskmanager.TriggerTypeInterval,
		IntervalMs: 0,
	})
	t.Cleanup(trigger.Stop)

	trigger.Start(nil)
	select {
	case <-trigger.C():
	case <-time.After(2 * time.Second):
		t.Fatal("zero-interval trigger did not fire")
	}

	select {
	case <-trigger.C():
		t.Fatal("zero-interval trigger re-armed and would spin")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestIntervalTriggerNextRunAdvancesAfterFiring(t *testing.T) {
	trigger := NewIntervalTrigger(taskmanager.TriggerConfig{
		Type:       taskmanager.TriggerTypeInterval,
		IntervalMs: 30,
	})
	t.Cleanup(trigger.Stop)

	trigger.Start(nil)
	first := trigger.NextRunTime()

	select {
	case <-trigger.C():
	case <-time.After(2 * time.Second):
		t.Fatal("interval trigger never fired")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if trigger.NextRunTime().After(first) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("next run time did not advance past %s after firing", first)
}
