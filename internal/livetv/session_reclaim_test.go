package livetv

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingHistory struct {
	entries []SessionHistoryEntry
	err     error
}

func (r *recordingHistory) RecordLiveSession(_ context.Context, entry SessionHistoryEntry) error {
	r.entries = append(r.entries, entry)
	return r.err
}

func singleTunerService(t *testing.T) (*Service, *memoryStore) {
	t.Helper()
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 1, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Enabled: true, StreamURL: "http://192.168.1.2/auto/v1",
	}
	return NewServiceWithStore(store), store
}

// An abandoned session used to hold its tuner index forever, so the next tune
// failed with ErrNoTuner. StartChannelSession now reclaims it first.
func TestStartChannelSessionReclaimsAbandonedTuner(t *testing.T) {
	svc, store := singleTunerService(t)
	ctx := context.Background()

	first, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("first StartChannelSession: %v", err)
	}

	if _, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1"); !errors.Is(err, ErrNoTuner) {
		t.Fatalf("second session while watched = %v, want ErrNoTuner", err)
	}

	// Nobody has fetched a segment or sent a heartbeat since the TTL.
	stale := store.sessions[first.ID]
	stale.LastSeenAt = time.Now().Add(-2 * StaleSessionTTL)
	store.sessions[first.ID] = stale

	second, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("tune after abandonment: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("expected a new session id")
	}
	if got := store.sessions[first.ID].Status; got != "released" {
		t.Fatalf("abandoned session status = %q, want released", got)
	}
}

func TestTouchSessionKeepsTunerClaimed(t *testing.T) {
	svc, store := singleTunerService(t)
	ctx := context.Background()

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	stale := store.sessions[session.ID]
	stale.LastSeenAt = time.Now().Add(-2 * StaleSessionTTL)
	store.sessions[session.ID] = stale

	if err := svc.TouchSession(ctx, session.ID); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	reclaimed, err := svc.ReclaimStaleSessions(ctx)
	if err != nil {
		t.Fatalf("ReclaimStaleSessions: %v", err)
	}
	if reclaimed != 0 {
		t.Fatalf("reclaimed = %d, want 0 for a session still being watched", reclaimed)
	}
	if got := store.sessions[session.ID].Status; got != "active" {
		t.Fatalf("session status = %q, want active", got)
	}
}

func TestTouchSessionThrottlesRepeatWrites(t *testing.T) {
	svc, store := singleTunerService(t)
	ctx := context.Background()
	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}

	if err := svc.TouchSession(ctx, session.ID); err != nil {
		t.Fatalf("first touch: %v", err)
	}
	marker := time.Now().Add(-time.Hour)
	stale := store.sessions[session.ID]
	stale.LastSeenAt = marker
	store.sessions[session.ID] = stale

	// Within the throttle window the store is not touched again.
	if err := svc.TouchSession(ctx, session.ID); err != nil {
		t.Fatalf("throttled touch: %v", err)
	}
	if !store.sessions[session.ID].LastSeenAt.Equal(marker) {
		t.Fatal("expected throttled touch to skip the store write")
	}
}

func TestReclaimStaleSessionsRecordsHistory(t *testing.T) {
	svc, store := singleTunerService(t)
	history := &recordingHistory{}
	svc.SetHistoryRecorder(history)
	ctx := context.Background()

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	stale := store.sessions[session.ID]
	stale.CreatedAt = time.Now().Add(-10 * time.Minute)
	stale.LastSeenAt = time.Now().Add(-2 * StaleSessionTTL)
	store.sessions[session.ID] = stale

	if _, err := svc.ReclaimStaleSessions(ctx); err != nil {
		t.Fatalf("ReclaimStaleSessions: %v", err)
	}
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(history.entries))
	}
	entry := history.entries[0]
	if entry.SessionID != session.ID || entry.ChannelID != "ch1" || entry.UserID != 7 {
		t.Fatalf("unexpected history entry %+v", entry)
	}
	if entry.WatchedSeconds <= 0 {
		t.Fatalf("watched seconds = %v, want > 0", entry.WatchedSeconds)
	}
}

func TestReleaseSessionRecordsHistory(t *testing.T) {
	svc, _ := singleTunerService(t)
	history := &recordingHistory{}
	svc.SetHistoryRecorder(history)
	ctx := context.Background()

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	if _, err := svc.ReleaseSession(ctx, session.ID, 7, "profile-1", true); err != nil {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(history.entries))
	}
	if history.entries[0].Transport != "mpegts" {
		t.Fatalf("transport = %q, want mpegts", history.entries[0].Transport)
	}
}

func TestHistoryMediaItemID(t *testing.T) {
	if got := HistoryMediaItemID("ch1"); got != "livetv:ch1" {
		t.Fatalf("HistoryMediaItemID = %q", got)
	}
	if got := HistoryMediaItemID(""); got != "" {
		t.Fatalf("empty channel = %q, want empty", got)
	}
}
