package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/prairie-server/prairie-server/internal/taskmanager"
)

type fakeWatchProviderSyncer struct {
	calls int
	err   error
}

func (f *fakeWatchProviderSyncer) SyncDueConnections(context.Context) error {
	f.calls++
	return f.err
}

type watchProviderProgressReporter struct {
	reports []string
}

func (p *watchProviderProgressReporter) Report(_ float64, message string) {
	p.reports = append(p.reports, message)
}

func (p *watchProviderProgressReporter) SetResultData(json.RawMessage) {}

func TestSyncWatchProvidersTask(t *testing.T) {
	syncer := &fakeWatchProviderSyncer{}
	task := NewSyncWatchProvidersTask(syncer)

	if task.Key() != "sync_watch_providers" {
		t.Fatalf("Key() = %q, want sync_watch_providers", task.Key())
	}
	if task.Name() == "" {
		t.Fatal("Name() should not be empty")
	}
	if task.Description() == "" {
		t.Fatal("Description() should not be empty")
	}
	if task.Category() != taskmanager.TaskCategoryLibrary {
		t.Fatalf("Category() = %q, want %q", task.Category(), taskmanager.TaskCategoryLibrary)
	}
	if task.IsHidden() {
		t.Fatal("IsHidden() = true, want false")
	}

	triggers := task.DefaultTriggers()
	if len(triggers) != 1 {
		t.Fatalf("DefaultTriggers() length = %d, want 1", len(triggers))
	}
	if triggers[0].Type != taskmanager.TriggerTypeInterval {
		t.Fatalf("trigger type = %q, want %q", triggers[0].Type, taskmanager.TriggerTypeInterval)
	}
	if triggers[0].IntervalMs != 60*60*1000 {
		t.Fatalf("trigger interval = %d, want %d", triggers[0].IntervalMs, 60*60*1000)
	}

	progress := &watchProviderProgressReporter{}
	if err := task.Execute(context.Background(), progress); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if syncer.calls != 1 {
		t.Fatalf("SyncDueConnections calls = %d, want 1", syncer.calls)
	}
	if len(progress.reports) == 0 {
		t.Fatal("expected progress reports")
	}
	if progress.reports[len(progress.reports)-1] != "Watch provider sync check complete" {
		t.Fatalf("last progress report = %q, want sync check complete", progress.reports[len(progress.reports)-1])
	}
}

func TestSyncWatchProvidersTaskExecuteReturnsSyncError(t *testing.T) {
	wantErr := errors.New("sync failed")
	syncer := &fakeWatchProviderSyncer{err: wantErr}
	task := NewSyncWatchProvidersTask(syncer)

	err := task.Execute(context.Background(), &watchProviderProgressReporter{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute error = %v, want %v", err, wantErr)
	}
	if syncer.calls != 1 {
		t.Fatalf("SyncDueConnections calls = %d, want 1", syncer.calls)
	}
}
