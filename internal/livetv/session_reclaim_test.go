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

	first, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
	if err != nil {
		t.Fatalf("first StartChannelSession: %v", err)
	}

	if _, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{}); !errors.Is(err, ErrNoTuner) {
		t.Fatalf("second session while watched = %v, want ErrNoTuner", err)
	}

	// Nobody has fetched a segment or sent a heartbeat since the TTL.
	stale := store.sessions[first.ID]
	stale.LastSeenAt = time.Now().Add(-2 * StaleSessionTTL)
	store.sessions[first.ID] = stale

	second, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
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

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
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
	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
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

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
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

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
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

func TestTouchSessionEdgeCases(t *testing.T) {
	ctx := context.Background()

	unconfigured := NewServiceWithStore(nil)
	if err := unconfigured.TouchSession(ctx, "sess"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured TouchSession = %v, want ErrNotConfigured", err)
	}

	svc, _ := singleTunerService(t)
	if err := svc.TouchSession(ctx, ""); err != nil {
		t.Fatalf("empty id TouchSession = %v, want nil", err)
	}

	// A service built without the touch map (zero value) still records touches.
	bare := &Service{store: newMemoryStore(), now: time.Now}
	if err := bare.TouchSession(ctx, "sess"); err != nil {
		t.Fatalf("bare TouchSession = %v", err)
	}
	if _, ok := bare.lastTouch["sess"]; !ok {
		t.Fatal("expected the touch map to be created on demand")
	}
}

func TestReclaimStaleSessionsErrors(t *testing.T) {
	ctx := context.Background()

	unconfigured := NewServiceWithStore(nil)
	if _, err := unconfigured.ReclaimStaleSessions(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured reclaim = %v, want ErrNotConfigured", err)
	}

	svc := NewServiceWithStore(&reclaimErrorStore{memoryStore: newMemoryStore()})
	if _, err := svc.ReclaimStaleSessions(ctx); !errors.Is(err, errStoreBoom) {
		t.Fatalf("store error reclaim = %v, want errStoreBoom", err)
	}
}

// A bridge-backed session reports hls transport, and a history writer failure is
// logged rather than failing the release.
func TestRecordSessionHistoryTransportAndWriteFailure(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 1, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Enabled: true, StreamURL: "http://192.168.1.2/auto/v1",
	}
	svc := NewServiceWithStore(store)
	svc.SetPlaybackBridge(stubPlaybackBridge{})
	history := &recordingHistory{err: errStoreBoom}
	svc.SetHistoryRecorder(history)
	ctx := context.Background()

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	if _, err := svc.ReleaseSession(ctx, session.ID, 7, "profile-1", true); err != nil {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(history.entries))
	}
	if history.entries[0].Transport != "hls" {
		t.Fatalf("transport = %q, want hls", history.entries[0].Transport)
	}
}

// A clock skew (session created after it ended) must not report negative watch time.
func TestRecordSessionHistoryClampsNegativeWatchTime(t *testing.T) {
	svc, _ := singleTunerService(t)
	history := &recordingHistory{}
	svc.SetHistoryRecorder(history)
	now := time.Now()
	svc.now = func() time.Time { return now }

	svc.recordSessionHistory(context.Background(), LiveSession{
		ID:        "sess-skew",
		ChannelID: "ch1",
		UserID:    7,
		CreatedAt: now.Add(time.Minute),
	})
	if len(history.entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(history.entries))
	}
	if got := history.entries[0].WatchedSeconds; got != 0 {
		t.Fatalf("watched seconds = %v, want 0", got)
	}
}

// Sessions with no history recorder or no channel are skipped.
func TestRecordSessionHistorySkipped(t *testing.T) {
	svc, _ := singleTunerService(t)
	svc.recordSessionHistory(context.Background(), LiveSession{ID: "s", ChannelID: "ch1"})

	history := &recordingHistory{}
	svc.SetHistoryRecorder(history)
	svc.recordSessionHistory(context.Background(), LiveSession{ID: "s"})
	if len(history.entries) != 0 {
		t.Fatalf("history entries = %d, want 0 without a channel", len(history.entries))
	}
}

// Reclaiming a bridge-backed session must stop its remux, and a failing stop is
// logged rather than leaving the session claimed.
func TestReclaimStaleSessionsStopsBridge(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 1, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Enabled: true, StreamURL: "http://192.168.1.2/auto/v1",
	}
	svc := NewServiceWithStore(store)
	var stopped []string
	svc.SetPlaybackBridge(failingStopBridge{onStop: func(id string) { stopped = append(stopped, id) }})
	ctx := context.Background()

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	// Keep the playback id in the throttle map so reclaim clears both keys.
	if err := svc.TouchSession(ctx, session.PlaybackSessionID); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	stale := store.sessions[session.ID]
	stale.LastSeenAt = time.Now().Add(-2 * StaleSessionTTL)
	store.sessions[session.ID] = stale

	reclaimed, err := svc.ReclaimStaleSessions(ctx)
	if err != nil {
		t.Fatalf("ReclaimStaleSessions: %v", err)
	}
	if reclaimed != 1 {
		t.Fatalf("reclaimed = %d, want 1", reclaimed)
	}
	if len(stopped) != 1 || stopped[0] != session.PlaybackSessionID {
		t.Fatalf("stopped = %v, want [%s]", stopped, session.PlaybackSessionID)
	}
	if _, ok := svc.lastTouch[session.PlaybackSessionID]; ok {
		t.Fatal("expected the reclaimed session to be dropped from the touch map")
	}
}

// Releasing a session whose remux teardown fails still releases the tuner.
func TestReleaseSessionLogsBridgeStopFailure(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 1, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Enabled: true, StreamURL: "http://192.168.1.2/auto/v1",
	}
	svc := NewServiceWithStore(store)
	svc.SetPlaybackBridge(failingStopBridge{})
	ctx := context.Background()

	session, err := svc.StartChannelSession(ctx, "ch1", 7, "profile-1", ClientCapabilities{})
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	released, err := svc.ReleaseSession(ctx, session.ID, 7, "profile-1", true)
	if err != nil {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if released.Status != "released" {
		t.Fatalf("status = %q, want released", released.Status)
	}
}

type reclaimErrorStore struct {
	*memoryStore
}

func (s *reclaimErrorStore) ReleaseSessionsLastSeenBefore(context.Context, time.Time) ([]LiveSession, error) {
	return nil, errStoreBoom
}

type failingStopBridge struct {
	onStop func(id string)
}

func (failingStopBridge) StartLiveStream(context.Context, LiveStreamRequest) (string, string, error) {
	return "pb-stop", "/api/v1/livetv/live-hls/pb-stop/index.m3u8", nil
}

func (b failingStopBridge) StopLiveStream(_ context.Context, id string) error {
	if b.onStop != nil {
		b.onStop(id)
	}
	return errStoreBoom
}
