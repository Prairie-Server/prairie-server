package taskmanager_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

// capturingHandler collects log records so a test can assert on what an
// operator would actually see.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) messagesAtLevel(level slog.Level) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, record := range h.records {
		if record.Level == level {
			out = append(out, record.Message)
		}
	}
	return out
}

// A trigger config the factory does not recognize is dropped, and a task left
// with no live trigger parks forever looking exactly like an idle one. That has
// to be loud: it is the difference between "nothing to do" and "this queue will
// never be drained again".
func TestTaskManagerWarnsWhenATaskEndsUpWithNoLiveTriggers(t *testing.T) {
	const taskKey = "unschedulable"
	triggerRepo := &fakeTriggerRepository{
		triggers: map[string][]taskmanager.TriggerConfig{
			taskKey: {{Type: taskmanager.TriggerType("interval-typo")}},
		},
	}
	handler := &capturingHandler{}
	manager := taskmanager.New(
		triggerRepo,
		fakeExecutionRepository{},
		func(cfg taskmanager.TriggerConfig) taskmanager.Trigger {
			if cfg.Type != taskmanager.TriggerTypeInterval {
				return nil // mirrors triggers.New for an unknown type
			}
			return newFakeTrigger(cfg)
		},
		slog.New(handler),
	)
	manager.Register(stubTask{key: taskKey})

	ctx, cancel := context.WithCancel(context.Background())
	defer manager.Stop()
	defer cancel()
	manager.Start(ctx)

	var found bool
	for _, message := range handler.messagesAtLevel(slog.LevelWarn) {
		if strings.Contains(message, "no live triggers") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a warning that the task has no live triggers, got %v",
			handler.messagesAtLevel(slog.LevelWarn))
	}
}

// Dropping some but not all triggers still leaves the task running on a
// narrower schedule than configured, which is worth saying out loud.
func TestTaskManagerWarnsWhenSomeTriggersAreDropped(t *testing.T) {
	const taskKey = "partially_schedulable"
	triggerRepo := &fakeTriggerRepository{
		triggers: map[string][]taskmanager.TriggerConfig{
			taskKey: {
				{Type: taskmanager.TriggerTypeInterval, IntervalMs: 1000},
				{Type: taskmanager.TriggerType("weekley")},
			},
		},
	}
	handler := &capturingHandler{}
	manager := taskmanager.New(
		triggerRepo,
		fakeExecutionRepository{},
		func(cfg taskmanager.TriggerConfig) taskmanager.Trigger {
			if cfg.Type != taskmanager.TriggerTypeInterval {
				return nil
			}
			return newFakeTrigger(cfg)
		},
		slog.New(handler),
	)
	manager.Register(stubTask{key: taskKey})

	ctx, cancel := context.WithCancel(context.Background())
	defer manager.Stop()
	defer cancel()
	manager.Start(ctx)

	var found bool
	for _, message := range handler.messagesAtLevel(slog.LevelWarn) {
		if strings.Contains(message, "not recognized") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a warning about dropped triggers, got %v",
			handler.messagesAtLevel(slog.LevelWarn))
	}
}

// The healthy path must stay quiet, or the warnings above become noise an
// operator learns to ignore.
func TestTaskManagerDoesNotWarnWhenAllTriggersAreLive(t *testing.T) {
	const taskKey = "schedulable"
	triggerRepo := &fakeTriggerRepository{
		triggers: map[string][]taskmanager.TriggerConfig{
			taskKey: {{Type: taskmanager.TriggerTypeInterval, IntervalMs: 1000}},
		},
	}
	handler := &capturingHandler{}
	manager := taskmanager.New(triggerRepo, fakeExecutionRepository{}, newFakeTrigger, slog.New(handler))
	manager.Register(stubTask{key: taskKey})

	ctx, cancel := context.WithCancel(context.Background())
	defer manager.Stop()
	defer cancel()
	manager.Start(ctx)

	if warnings := handler.messagesAtLevel(slog.LevelWarn); len(warnings) > 0 {
		t.Fatalf("expected no warnings for a fully scheduled task, got %v", warnings)
	}
}
