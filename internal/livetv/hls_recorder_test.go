package livetv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFakeFFmpeg(t *testing.T, dir, script string) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

const fakeFFmpegStayAlive = `#!/bin/sh
# Write HLS playlist (when -hls_segment_filename is present) or MPEG-TS output
# (last arg), then stay alive until killed.
outdir=""
out=""
prev=""
for arg in "$@"; do
  out="$arg"
  if [ "$prev" = "-hls_segment_filename" ]; then
    outdir=$(dirname "$arg")
  fi
  prev="$arg"
done
if [ -n "$outdir" ]; then
  mkdir -p "$outdir"
  printf '#EXTM3U\n' > "$outdir/index.m3u8"
  printf 'seg' > "$outdir/seg_00000.ts"
fi
if [ -n "$out" ] && [ -z "$outdir" ]; then
  mkdir -p "$(dirname "$out")"
  printf 'tsdata' > "$out"
fi
while true; do sleep 1; done
`

func TestHLSBridgeStartStopAndAuthorize(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegStayAlive)

	bridge := NewHLSBridge(root, ffmpeg)
	if bridge == nil {
		t.Fatal("expected bridge")
	}
	if PublicLiveHLSPath("abc") != "/api/v1/livetv/live-hls/abc/index.m3u8" {
		t.Fatalf("path = %s", PublicLiveHLSPath("abc"))
	}

	ctx := context.Background()
	id, url, err := bridge.StartLiveStream(ctx, "ch1", "http://127.0.0.1/auto/v5.1", 7, "prof")
	if err != nil {
		t.Fatalf("StartLiveStream: %v", err)
	}
	if id == "" || url != PublicLiveHLSPath(id) {
		t.Fatalf("id=%q url=%q", id, url)
	}
	if err := bridge.Authorize(id, 7, "prof", true); err != nil {
		t.Fatalf("Authorize owner: %v", err)
	}
	if err := bridge.Authorize(id, 8, "prof", true); err == nil {
		t.Fatal("expected Authorize deny for other user")
	}
	if err := bridge.Authorize(id, 8, "other", false); err != nil {
		t.Fatalf("Authorize without enforceOwner: %v", err)
	}
	path, err := bridge.ResolvePlaylistFile(id, "index.m3u8")
	if err != nil {
		t.Fatalf("ResolvePlaylistFile: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("playlist missing: %v", err)
	}
	if _, err := bridge.ResolvePlaylistFile(id, "../etc/passwd"); err == nil {
		t.Fatal("expected path escape rejection")
	}
	if _, err := bridge.ResolvePlaylistFile(id, ""); err == nil {
		t.Fatal("expected empty name rejection")
	}
	if _, err := bridge.ResolvePlaylistFile("missing", "index.m3u8"); err == nil {
		t.Fatal("expected missing session")
	}
	if _, err := bridge.ResolvePlaylistFile(id, "nope.ts"); err == nil {
		t.Fatal("expected missing file")
	}
	if err := bridge.StopLiveStream(ctx, id); err != nil {
		t.Fatalf("StopLiveStream: %v", err)
	}
	if err := bridge.Authorize(id, 7, "prof", true); err == nil {
		t.Fatal("expected missing after stop")
	}
	_ = bridge.StopLiveStream(ctx, "")
	_ = bridge.StopLiveStream(ctx, "already-gone")
	_ = NewHLSBridge("", "")

	var nilBridge *HLSBridge
	if _, _, err := nilBridge.StartLiveStream(ctx, "c", "http://127.0.0.1/x", 1, "p"); err == nil {
		t.Fatal("expected nil bridge start error")
	}
	_ = nilBridge.StopLiveStream(ctx, "x")
	if err := nilBridge.Authorize("x", 1, "p", true); err == nil {
		t.Fatal("expected nil authorize error")
	}
	if _, err := nilBridge.ResolvePlaylistFile("x", "index.m3u8"); err == nil {
		t.Fatal("expected nil resolve error")
	}
}

func TestHLSBridgeRejectsBadURL(t *testing.T) {
	bridge := NewHLSBridge(t.TempDir(), "ffmpeg")
	_, _, err := bridge.StartLiveStream(context.Background(), "ch", "file:///etc/passwd", 1, "p")
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestHLSBridgeStartLiveHLSFailure(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, `#!/bin/sh
echo boom >&2
exit 1
`)
	bridge := NewHLSBridge(root, ffmpeg)
	_, _, err := bridge.StartLiveStream(context.Background(), "ch", "http://127.0.0.1/auto/v1", 1, "p")
	if err == nil {
		t.Fatal("expected start failure")
	}
}

func TestRecorderProcessStartFinishAndFail(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegStayAlive)

	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 2, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Number: "5.1", Name: "KING", Enabled: true,
		StreamURL: "http://127.0.0.1/auto/v5.1",
	}
	svc := NewServiceWithStore(store)
	// Use wall clock so StopAt deadlines inside StartLiveRecord remain valid.
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	rec := NewRecorder(svc, filepath.Join(root, "dvr"), ffmpeg)
	svc.SetRecorder(rec)

	due := &Recording{
		ID: "rec-due", ChannelID: "ch1", Title: "News @ Night!", Status: "scheduled",
		Start: now.Add(-time.Minute), Stop: now.Add(2 * time.Minute), UserID: 1, ProfileID: "p",
	}
	if _, err := store.CreateRecording(context.Background(), due); err != nil {
		t.Fatalf("CreateRecording: %v", err)
	}
	elapsed := &Recording{
		ID: "rec-late", ChannelID: "ch1", Title: "Late", Status: "scheduled",
		Start: now.Add(-2 * time.Hour), Stop: now.Add(-time.Hour), UserID: 1, ProfileID: "p",
	}
	if _, err := store.CreateRecording(context.Background(), elapsed); err != nil {
		t.Fatalf("CreateRecording elapsed: %v", err)
	}

	started, completed, failed, err := svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatalf("ProcessRecordings: %v", err)
	}
	if started != 1 || failed != 1 {
		t.Fatalf("started=%d completed=%d failed=%d", started, completed, failed)
	}
	got, _ := store.GetRecording(context.Background(), "rec-due")
	if got.Status != "recording" || got.Path == "" {
		t.Fatalf("due recording = %+v", got)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, statErr := os.Stat(got.Path); statErr == nil && st.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recording output never appeared: %s", got.Path)
		}
		time.Sleep(20 * time.Millisecond)
	}
	late, _ := store.GetRecording(context.Background(), "rec-late")
	if late.Status != "failed" {
		t.Fatalf("late = %+v", late)
	}

	// Idempotent re-entry while already active.
	started, _, _, err = svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatalf("ProcessRecordings idle: %v", err)
	}
	if started != 0 {
		t.Fatalf("expected no re-start, started=%d", started)
	}

	// Advance clock past stop and finish.
	svc.now = func() time.Time { return now.Add(5 * time.Minute) }
	started, completed, failed, err = svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatalf("ProcessRecordings finish: %v", err)
	}
	if completed < 1 {
		t.Fatalf("expected completion, started=%d completed=%d failed=%d", started, completed, failed)
	}
	got, _ = store.GetRecording(context.Background(), "rec-due")
	if got.Status != "completed" {
		t.Fatalf("finished = %+v", got)
	}

	if sanitizeFilename("  Hello/World\x00  ") == "" {
		t.Fatal("sanitizeFilename empty")
	}
	if sanitizeFilename("") != "" {
		t.Fatal("sanitizeFilename blank")
	}
	long := strings.Repeat("a", 100)
	if len(sanitizeFilename(long)) != 80 {
		t.Fatalf("sanitizeFilename truncate = %d", len(sanitizeFilename(long)))
	}
	if shortID("abcdefghijklmnop") != "ijklmnop" {
		t.Fatalf("shortID = %s", shortID("abcdefghijklmnop"))
	}
	if shortID("short") != "short" {
		t.Fatalf("shortID short = %s", shortID("short"))
	}
}

func TestRecorderStartFailuresAndCancel(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegStayAlive)

	store := newMemoryStore()
	store.channels["ch-bad"] = Channel{ID: "ch-bad", Enabled: true, StreamURL: ""}
	store.channels["ch-file"] = Channel{ID: "ch-file", Enabled: true, StreamURL: "file:///etc/passwd"}
	store.channels["ch1"] = Channel{
		ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1",
	}
	svc := NewServiceWithStore(store)
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	rec := NewRecorder(svc, filepath.Join(root, "dvr"), ffmpeg)
	svc.SetRecorder(rec)

	for _, id := range []string{"missing-ch", "empty-url", "bad-url"} {
		chID := "nope"
		switch id {
		case "empty-url":
			chID = "ch-bad"
		case "bad-url":
			chID = "ch-file"
		}
		if _, err := store.CreateRecording(context.Background(), &Recording{
			ID: id, ChannelID: chID, Title: id, Status: "scheduled",
			Start: now.Add(-time.Minute), Stop: now.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	started, _, failed, err := svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if started != 0 || failed < 3 {
		t.Fatalf("started=%d failed=%d", started, failed)
	}

	// Successful start then cancel via finishRecording(cancel=true) through reapStale.
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "rec-cancel", ChannelID: "ch1", Title: "", Status: "scheduled",
		Start: now.Add(-time.Minute), Stop: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	started, _, failed, err = svc.ProcessRecordings(context.Background())
	if err != nil || started != 1 {
		t.Fatalf("start cancel target: started=%d failed=%d err=%v", started, failed, err)
	}
	active, _ := store.GetRecording(context.Background(), "rec-cancel")
	active.Status = "cancelled"
	if _, err := store.UpdateRecording(context.Background(), active); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRecording(context.Background(), "rec-cancel")
	if got.Status != "cancelled" {
		t.Fatalf("cancel = %+v", got)
	}
}

func TestRecorderFinishEmptyAndMissingPath(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	rec := NewRecorder(svc, t.TempDir(), "ffmpeg")
	svc.SetRecorder(rec)

	emptyPath := filepath.Join(t.TempDir(), "empty.ts")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "rec-empty", ChannelID: "ch", Title: "E", Status: "recording",
		Start: now.Add(-time.Hour), Stop: now.Add(-time.Minute), Path: emptyPath,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "rec-missing", ChannelID: "ch", Title: "M", Status: "recording",
		Start: now.Add(-time.Hour), Stop: now.Add(-time.Minute), Path: filepath.Join(t.TempDir(), "nope.ts"),
	}); err != nil {
		t.Fatal(err)
	}
	_, completed, failed, err := svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if completed != 0 || failed < 2 {
		t.Fatalf("completed=%d failed=%d", completed, failed)
	}
	empty, _ := store.GetRecording(context.Background(), "rec-empty")
	if empty.Status != "failed" || empty.LastError != "recording file empty" {
		t.Fatalf("empty = %+v", empty)
	}
}

func TestRecorderNilAndDefaultRoot(t *testing.T) {
	var nilRec *Recorder
	if _, _, _, err := nilRec.Process(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil recorder: %v", err)
	}
	rec := NewRecorder(nil, "", "")
	if rec.root == "" {
		t.Fatal("expected default root")
	}
	if _, _, _, err := rec.Process(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil service: %v", err)
	}
}

func TestProcessRecordingsWithoutRecorderFallsBack(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "r1", ChannelID: "ch", Title: "X", Status: "scheduled",
		Start: now.Add(-time.Minute), Stop: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	started, completed, failed, err := svc.ProcessRecordings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if started != 0 || completed != 0 || failed != 1 {
		t.Fatalf("started=%d completed=%d failed=%d", started, completed, failed)
	}
}

func TestReleaseSessionStopsBridge(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 1, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Enabled: true, StreamURL: "http://192.168.1.2/auto/v1",
	}
	svc := NewServiceWithStore(store)
	stopped := ""
	svc.SetPlaybackBridge(stopRecordingBridge{
		startID:  "pb-stop",
		startURL: PublicLiveHLSPath("pb-stop"),
		onStop:   func(id string) { stopped = id },
	})
	if svc.PlaybackBridge() == nil {
		t.Fatal("expected bridge")
	}
	session, err := svc.StartChannelSession(context.Background(), "ch1", 1, "p")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	if session.Transport != "hls" || session.PlaybackSessionID != "pb-stop" {
		t.Fatalf("session = %+v", session)
	}
	if _, err := svc.ReleaseSession(context.Background(), session.ID, 1, "p", true); err != nil {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if stopped != "pb-stop" {
		t.Fatalf("bridge not stopped, got %q", stopped)
	}
}

type stopRecordingBridge struct {
	startID  string
	startURL string
	onStop   func(string)
}

func (b stopRecordingBridge) StartLiveStream(context.Context, string, string, int, string) (string, string, error) {
	return b.startID, b.startURL, nil
}

func (b stopRecordingBridge) StopLiveStream(_ context.Context, id string) error {
	if b.onStop != nil {
		b.onStop(id)
	}
	return nil
}

func TestUpdateRecordingStore(t *testing.T) {
	store := newMemoryStore()
	created, err := store.CreateRecording(context.Background(), &Recording{
		ChannelID: "ch", Title: "T", Status: "scheduled",
		Start: time.Now(), Stop: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = "recording"
	created.Path = "/tmp/x.ts"
	updated, err := store.UpdateRecording(context.Background(), created)
	if err != nil || updated == nil || updated.Status != "recording" {
		t.Fatalf("UpdateRecording = %+v err=%v", updated, err)
	}
	missing, err := store.UpdateRecording(context.Background(), &Recording{ID: "nope"})
	if err != nil || missing != nil {
		t.Fatalf("missing update = %+v err=%v", missing, err)
	}
}

func TestRecorderEarlyFFmpegExitMarksFailed(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, `#!/bin/sh
out=""
for arg in "$@"; do out="$arg"; done
mkdir -p "$(dirname "$out")"
printf 'x' > "$out"
exit 1
`)
	store := newMemoryStore()
	store.channels["ch1"] = Channel{ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1"}
	svc := NewServiceWithStore(store)
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	rec := NewRecorder(svc, filepath.Join(root, "dvr"), ffmpeg)
	svc.SetRecorder(rec)
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "rec-early", ChannelID: "ch1", Title: "Early", Status: "scheduled",
		Start: now.Add(-time.Minute), Stop: now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	started, _, _, err := svc.ProcessRecordings(context.Background())
	if err != nil || started != 1 {
		t.Fatalf("started=%d err=%v", started, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		got, _ := store.GetRecording(context.Background(), "rec-early")
		if got != nil && got.Status == "failed" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected early-exit failure, got %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestProcessRecordingsRequireStore(t *testing.T) {
	svc := &Service{}
	if _, _, _, err := svc.ProcessRecordings(context.Background()); err == nil {
		t.Fatal("expected requireStore error")
	}
}

func TestRecorderReapCompletedActive(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegStayAlive)
	store := newMemoryStore()
	store.channels["ch1"] = Channel{ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1"}
	svc := NewServiceWithStore(store)
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	rec := NewRecorder(svc, filepath.Join(root, "dvr"), ffmpeg)
	svc.SetRecorder(rec)
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "rec-reap", ChannelID: "ch1", Title: "Reap", Status: "scheduled",
		Start: now.Add(-time.Minute), Stop: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.ProcessRecordings(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRecording(context.Background(), "rec-reap")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, err := os.Stat(got.Path); err == nil && st.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("output missing")
		}
		time.Sleep(20 * time.Millisecond)
	}
	got.Status = "completed"
	if _, err := store.UpdateRecording(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.ProcessRecordings(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFinishRecordingUsesSessionPath(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegStayAlive)
	store := newMemoryStore()
	store.channels["ch1"] = Channel{ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1"}
	svc := NewServiceWithStore(store)
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	rec := NewRecorder(svc, filepath.Join(root, "dvr"), ffmpeg)
	svc.SetRecorder(rec)
	if _, err := store.CreateRecording(context.Background(), &Recording{
		ID: "rec-path", ChannelID: "ch1", Title: "P", Status: "scheduled",
		Start: now.Add(-time.Minute), Stop: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.ProcessRecordings(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRecording(context.Background(), "rec-path")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, err := os.Stat(got.Path); err == nil && st.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("output missing")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Clear path so finishRecording falls back to session.OutputPath.
	got.Path = ""
	if _, err := store.UpdateRecording(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return now.Add(5 * time.Minute) }
	_, completed, failed, err := svc.ProcessRecordings(context.Background())
	if err != nil || completed != 1 || failed != 0 {
		t.Fatalf("completed=%d failed=%d err=%v", completed, failed, err)
	}
}

type recorderBoomStore struct {
	*memoryStore
	listErr         bool
	getChannelErr   bool
	getRecordingErr bool
	updateErrAfter  int
	updates         int
}

func (s *recorderBoomStore) ListRecordings(ctx context.Context, status string) ([]Recording, error) {
	if s.listErr {
		return nil, errStoreBoom
	}
	return s.memoryStore.ListRecordings(ctx, status)
}

func (s *recorderBoomStore) GetChannel(ctx context.Context, id string) (*Channel, error) {
	if s.getChannelErr {
		return nil, errStoreBoom
	}
	return s.memoryStore.GetChannel(ctx, id)
}

func (s *recorderBoomStore) GetRecording(ctx context.Context, id string) (*Recording, error) {
	if s.getRecordingErr {
		return nil, errStoreBoom
	}
	return s.memoryStore.GetRecording(ctx, id)
}

func (s *recorderBoomStore) UpdateRecording(ctx context.Context, rec *Recording) (*Recording, error) {
	s.updates++
	if s.updateErrAfter > 0 && s.updates >= s.updateErrAfter {
		return nil, errStoreBoom
	}
	return s.memoryStore.UpdateRecording(ctx, rec)
}

func TestRecorderStoreErrorPaths(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegStayAlive)
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("list recordings", func(t *testing.T) {
		store := &recorderBoomStore{memoryStore: newMemoryStore(), listErr: true}
		svc := NewServiceWithStore(store)
		svc.now = func() time.Time { return now }
		svc.SetRecorder(NewRecorder(svc, root, ffmpeg))
		if _, _, _, err := svc.ProcessRecordings(context.Background()); !errors.Is(err, errStoreBoom) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("get channel", func(t *testing.T) {
		store := &recorderBoomStore{memoryStore: newMemoryStore(), getChannelErr: true}
		store.channels["ch1"] = Channel{ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1"}
		svc := NewServiceWithStore(store)
		svc.now = func() time.Time { return now }
		svc.SetRecorder(NewRecorder(svc, root, ffmpeg))
		if _, err := store.CreateRecording(context.Background(), &Recording{
			ID: "r", ChannelID: "ch1", Title: "T", Status: "scheduled",
			Start: now.Add(-time.Minute), Stop: now.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
		started, _, failed, err := svc.ProcessRecordings(context.Background())
		if err != nil || started != 0 || failed != 1 {
			t.Fatalf("started=%d failed=%d err=%v", started, failed, err)
		}
	})

	t.Run("update after start", func(t *testing.T) {
		store := &recorderBoomStore{memoryStore: newMemoryStore(), updateErrAfter: 1}
		store.channels["ch1"] = Channel{ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1"}
		svc := NewServiceWithStore(store)
		svc.now = func() time.Time { return now }
		svc.SetRecorder(NewRecorder(svc, root, ffmpeg))
		if _, err := store.CreateRecording(context.Background(), &Recording{
			ID: "r2", ChannelID: "ch1", Title: "T", Status: "scheduled",
			Start: now.Add(-time.Minute), Stop: now.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := svc.ProcessRecordings(context.Background()); !errors.Is(err, errStoreBoom) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("update elapsed fail", func(t *testing.T) {
		store := &recorderBoomStore{memoryStore: newMemoryStore(), updateErrAfter: 1}
		svc := NewServiceWithStore(store)
		svc.now = func() time.Time { return now }
		svc.SetRecorder(NewRecorder(svc, root, ffmpeg))
		if _, err := store.CreateRecording(context.Background(), &Recording{
			ID: "late", ChannelID: "ch", Title: "L", Status: "scheduled",
			Start: now.Add(-2 * time.Hour), Stop: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := svc.ProcessRecordings(context.Background()); !errors.Is(err, errStoreBoom) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("reap get recording error", func(t *testing.T) {
		store := &recorderBoomStore{memoryStore: newMemoryStore()}
		store.channels["ch1"] = Channel{ID: "ch1", Enabled: true, StreamURL: "http://127.0.0.1/auto/v1"}
		svc := NewServiceWithStore(store)
		svc.now = func() time.Time { return now }
		recorder := NewRecorder(svc, root, ffmpeg)
		svc.SetRecorder(recorder)
		if _, err := store.CreateRecording(context.Background(), &Recording{
			ID: "reap-err", ChannelID: "ch1", Title: "R", Status: "scheduled",
			Start: now.Add(-time.Minute), Stop: now.Add(2 * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := svc.ProcessRecordings(context.Background()); err != nil {
			t.Fatal(err)
		}
		store.getRecordingErr = true
		if _, _, _, err := svc.ProcessRecordings(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}
