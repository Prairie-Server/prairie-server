package livetv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv/hdhomerun"
)

func TestServiceAddTunerScansLineup(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"hdhr-1","ModelNumber":"HDHR5-2US","FirmwareVersion":"20260701","TunerCount":2,"BaseURL":"` + srv.URL + `"}`))
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"GuideNumber":"5.1","GuideName":"KING-HD","URL":"` + srv.URL + `/auto/v5.1","HD":true}]`))
	})

	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))

	tuner, err := svc.AddTuner(context.Background(), srv.URL+"/discover.json", "")
	if err != nil {
		t.Fatalf("AddTuner() error = %v", err)
	}
	if tuner == nil || tuner.DeviceID != "hdhr-1" || tuner.ChannelCount != 1 {
		t.Fatalf("unexpected tuner: %+v", tuner)
	}
	channels, err := svc.ListChannels(context.Background(), tuner.ID)
	if err != nil {
		t.Fatalf("ListChannels() error = %v", err)
	}
	if len(channels) != 1 || channels[0].Number != "5.1" || channels[0].StreamURL == "" || !channels[0].HD {
		t.Fatalf("unexpected channels: %+v", channels)
	}

	listed, err := svc.ListTuners(context.Background())
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListTuners() = %+v, err=%v", listed, err)
	}
	if err := svc.DeleteTuner(context.Background(), tuner.ID); err != nil {
		t.Fatalf("DeleteTuner() error = %v", err)
	}
}

func TestGuideSourceMaxThreeEnabled(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)

	for i := 0; i < MaxGuideSources; i++ {
		_, err := svc.CreateGuideSource(context.Background(), &GuideSource{
			Type:        GuideSourceXMLTVURL,
			Priority:    100 + i,
			Enabled:     true,
			DisplayName: "XMLTV",
			Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
		})
		if err != nil {
			t.Fatalf("CreateGuideSource(%d) error = %v", i, err)
		}
	}
	_, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type:        GuideSourceXMLTVURL,
		Priority:    400,
		Enabled:     true,
		DisplayName: "Too many",
		Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
	})
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("CreateGuideSource fourth enabled error = %v, want ErrLimitExceeded", err)
	}

	disabled, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type:        GuideSourceXMLTVURL,
		Priority:    500,
		Enabled:     false,
		DisplayName: "Disabled",
		Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
	})
	if err != nil {
		t.Fatalf("CreateGuideSource disabled error = %v", err)
	}

	_, err = svc.UpdateGuideSource(context.Background(), &GuideSource{
		ID:          disabled.ID,
		Type:        GuideSourceXMLTVURL,
		Enabled:     true,
		DisplayName: "Disabled",
		Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
	})
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("UpdateGuideSource enable error = %v, want ErrLimitExceeded", err)
	}
}

func TestGuideSourceReorderPriorities(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)

	a, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLTVURL, Priority: 50, Enabled: false, DisplayName: "A",
		Config: map[string]string{"url": "https://example.test/a.xml"},
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLTVURL, Priority: 10, Enabled: false, DisplayName: "B",
		Config: map[string]string{"url": "https://example.test/b.xml"},
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	c, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLTVURL, Priority: 200, Enabled: false, DisplayName: "C",
		Config: map[string]string{"url": "https://example.test/c.xml"},
	})
	if err != nil {
		t.Fatalf("create C: %v", err)
	}

	sources, err := svc.ListGuideSources(context.Background())
	if err != nil {
		t.Fatalf("ListGuideSources: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("len(sources)=%d", len(sources))
	}
	if sources[0].Priority != 100 || sources[1].Priority != 200 || sources[2].Priority != 300 {
		t.Fatalf("priorities = %d,%d,%d want 100,200,300", sources[0].Priority, sources[1].Priority, sources[2].Priority)
	}
	if sources[0].ID != b.ID || sources[1].ID != a.ID || sources[2].ID != c.ID {
		t.Fatalf("order by original priority wrong: %+v", sources)
	}

	if err := svc.DeleteGuideSource(context.Background(), a.ID); err != nil {
		t.Fatalf("DeleteGuideSource: %v", err)
	}
	sources, err = svc.ListGuideSources(context.Background())
	if err != nil {
		t.Fatalf("ListGuideSources after delete: %v", err)
	}
	if len(sources) != 2 || sources[0].Priority != 100 || sources[1].Priority != 200 {
		t.Fatalf("after delete priorities = %+v", sources)
	}
}

func TestSyncGuideSourceXMLTVMapsChannels(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="KING"><display-name>KING-HD</display-name></channel>
  <channel id="orphan"><display-name>Orphan</display-name></channel>
  <programme start="20260725190000 +0000" stop="20260725200000 +0000" channel="KING">
    <title>Evening News</title>
    <sub-title>Weekend</sub-title>
    <desc>Headlines</desc>
    <category>News</category>
    <new/>
  </programme>
  <programme start="20260725200000 +0000" stop="20260725210000 +0000" channel="orphan">
    <title>Unmapped</title>
  </programme>
</tv>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(xmlBody))
	}))
	defer srv.Close()

	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 2, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Number: "5.1", Callsign: "KING-HD", Name: "KING-HD", Enabled: true, StreamURL: "http://x/auto/v5.1",
	}

	svc := NewServiceWithStore(store)
	svc.httpClient = srv.Client()
	fixed := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	source, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLTVURL, Enabled: true, DisplayName: "XMLTV",
		Config: map[string]string{"url": srv.URL + "/xmltv.xml"},
	})
	if err != nil {
		t.Fatalf("CreateGuideSource: %v", err)
	}

	if err := svc.SyncGuideSource(context.Background(), source.ID); err != nil {
		t.Fatalf("SyncGuideSource: %v", err)
	}
	got, err := store.GetGuideSource(context.Background(), source.ID)
	if err != nil || got == nil || got.Status != "ready" {
		t.Fatalf("source status = %+v err=%v", got, err)
	}

	start := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 25, 22, 0, 0, 0, time.UTC)
	programs, err := svc.ListGuide(context.Background(), []string{"ch1"}, start, end)
	if err != nil {
		t.Fatalf("ListGuide: %v", err)
	}
	if len(programs) != 1 || programs[0].Title != "Evening News" || programs[0].ChannelID != "ch1" {
		t.Fatalf("programs = %+v", programs)
	}
	if programs[0].SeriesID != "evening-news" || !programs[0].IsNew {
		t.Fatalf("unexpected program fields: %+v", programs[0])
	}

	program, err := svc.GetProgram(context.Background(), programs[0].ID)
	if err != nil || program == nil || program.Title != "Evening News" {
		t.Fatalf("GetProgram = %+v err=%v", program, err)
	}

	count, err := svc.SyncAllEnabledGuideSources(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("SyncAllEnabledGuideSources = %d, %v", count, err)
	}
}

func TestSyncGuideSourceSchedulesDirectNotImplemented(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	source, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceSchedulesDirect, Enabled: true, DisplayName: "SD",
		Config: map[string]string{"username": "u"},
	})
	if err != nil {
		t.Fatalf("CreateGuideSource: %v", err)
	}
	err = svc.SyncGuideSource(context.Background(), source.ID)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
}

func TestSyncGuideSourceMissingURL(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	source, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLTVURL, Enabled: true, DisplayName: "XMLTV", Config: map[string]string{},
	})
	if err != nil {
		t.Fatalf("CreateGuideSource: %v", err)
	}
	err = svc.SyncGuideSource(context.Background(), source.ID)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
}

func TestPatchChannelAndScanErrors(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"hdhr-2","ModelNumber":"HDHR","TunerCount":1,"BaseURL":"` + srv.URL + `"}`))
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))

	tuner, err := store.CreateTuner(context.Background(), &Tuner{
		Type: TunerTypeHDHomeRun, DeviceID: "hdhr-2", BaseURL: srv.URL, TunerCount: 1, Status: "discovered",
	})
	if err != nil {
		t.Fatalf("CreateTuner: %v", err)
	}
	if err := svc.ScanTuner(context.Background(), tuner.ID); err == nil {
		t.Fatal("ScanTuner expected error")
	}
	got, _ := store.GetTuner(context.Background(), tuner.ID)
	if got == nil || got.Status != "error" || got.LastError == "" {
		t.Fatalf("tuner after scan error = %+v", got)
	}

	ch, err := store.UpdateChannel(context.Background(), "missing", ChannelPatch{})
	if err != nil || ch != nil {
		t.Fatalf("UpdateChannel missing = %+v err=%v", ch, err)
	}
	_ = store.ReplaceChannelsForTuner(context.Background(), tuner.ID, []Channel{{
		Number: "2.1", Callsign: "TEST", Name: "TEST", Enabled: true, StreamURL: srv.URL + "/auto/v2.1",
	}})
	channels, _ := store.ListChannels(context.Background(), tuner.ID)
	if len(channels) != 1 {
		t.Fatalf("channels = %+v", channels)
	}
	enabled := false
	station := "GUIDE1"
	override := "99.1"
	patched, err := svc.PatchChannel(context.Background(), channels[0].ID, ChannelPatch{
		Enabled: &enabled, NumberOverride: &override, GuideStationID: &station,
	})
	if err != nil || patched == nil || patched.Enabled || patched.GuideStationID != "GUIDE1" {
		t.Fatalf("PatchChannel = %+v err=%v", patched, err)
	}
	_, err = svc.PatchChannel(context.Background(), "missing", ChannelPatch{Enabled: &enabled})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("PatchChannel missing = %v", err)
	}
	if err := svc.ScanTuner(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ScanTuner missing = %v", err)
	}
}

func TestStartAndReleaseChannelSession(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 1, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Number: "5.1", Name: "KING", Enabled: true, StreamURL: "http://hdhr/auto/v5.1",
	}
	svc := NewServiceWithStore(store)

	session, err := svc.StartChannelSession(context.Background(), "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	if session.TunerIndex != 0 || session.StreamURL == "" || session.Status != "active" {
		t.Fatalf("session = %+v", session)
	}
	if !strings.Contains(session.Note, "playback bridge") {
		t.Fatalf("expected note about missing bridge, got %q", session.Note)
	}

	_, err = svc.StartChannelSession(context.Background(), "ch1", 7, "profile-1")
	if !errors.Is(err, ErrNoTuner) {
		t.Fatalf("second session error = %v, want ErrNoTuner", err)
	}

	released, err := svc.ReleaseSession(context.Background(), session.ID)
	if err != nil || released == nil || released.Status != "released" {
		t.Fatalf("ReleaseSession = %+v err=%v", released, err)
	}

	session2, err := svc.StartChannelSession(context.Background(), "ch1", 7, "profile-1")
	if err != nil {
		t.Fatalf("StartChannelSession after release: %v", err)
	}
	if session2.ID == session.ID {
		t.Fatalf("expected new session id")
	}

	_, err = svc.StartChannelSession(context.Background(), "missing", 1, "p")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing channel = %v", err)
	}
	_, err = svc.ReleaseSession(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session = %v", err)
	}
}

func TestStartChannelSessionWithPlaybackBridge(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 2, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Number: "5.1", Name: "KING", Enabled: true, StreamURL: "http://hdhr/auto/v5.1",
	}
	svc := NewServiceWithStore(store)
	svc.SetPlaybackBridge(stubPlaybackBridge{})

	session, err := svc.StartChannelSession(context.Background(), "ch1", 3, "prof")
	if err != nil {
		t.Fatalf("StartChannelSession: %v", err)
	}
	if session.PlaybackSessionID != "pb-1" || session.HLSURL != "http://play/hls.m3u8" || session.Note != "" {
		t.Fatalf("session = %+v", session)
	}
}

func TestScheduleCancelAndFailDueRecordings(t *testing.T) {
	store := newMemoryStore()
	fixed := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC)
	svc := NewServiceWithStore(store)
	svc.now = func() time.Time { return fixed }

	prog := &Program{
		ID: "p1", ChannelID: "ch1", Title: "News", SeriesID: "news",
		Start: fixed.Add(-time.Hour), Stop: fixed.Add(time.Hour),
	}
	store.programs[prog.ID] = *prog

	rec, err := svc.ScheduleRecording(context.Background(), &Recording{ProgramID: "p1"})
	if err != nil {
		t.Fatalf("ScheduleRecording: %v", err)
	}
	if rec.Status != "scheduled" || rec.ChannelID != "ch1" || rec.Title != "News" {
		t.Fatalf("recording = %+v", rec)
	}

	manual, err := svc.ScheduleRecording(context.Background(), &Recording{
		ChannelID: "ch1",
		Start:     fixed.Add(2 * time.Hour),
		Stop:      fixed.Add(3 * time.Hour),
		Title:     "Manual",
	})
	if err != nil {
		t.Fatalf("ScheduleRecording manual: %v", err)
	}

	_, err = svc.ScheduleRecording(context.Background(), &Recording{Title: "bad"})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid schedule = %v", err)
	}
	_, err = svc.ScheduleRecording(context.Background(), &Recording{ProgramID: "missing"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing program = %v", err)
	}

	listed, err := svc.ListRecordings(context.Background(), "scheduled")
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListRecordings = %+v err=%v", listed, err)
	}

	cancelled, err := svc.CancelRecording(context.Background(), manual.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("CancelRecording = %+v err=%v", cancelled, err)
	}
	_, err = svc.CancelRecording(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CancelRecording missing = %v", err)
	}

	n, err := svc.FailDueRecordings(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("FailDueRecordings = %d, %v", n, err)
	}
	due, _ := store.GetRecording(rec.ID)
	if due == nil || due.Status != "failed" || due.LastError == "" {
		t.Fatalf("due recording = %+v", due)
	}
}

func TestSeriesRulesApplyAndCRUD(t *testing.T) {
	store := newMemoryStore()
	fixed := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	svc := NewServiceWithStore(store)
	svc.now = func() time.Time { return fixed }

	channelID := "ch1"
	_, err := svc.CreateSeriesRule(context.Background(), &SeriesRule{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty rule = %v", err)
	}

	rule, err := svc.CreateSeriesRule(context.Background(), &SeriesRule{
		TitleMatch: "news", NewOnly: true, Enabled: true, ChannelID: &channelID,
	})
	if err != nil {
		t.Fatalf("CreateSeriesRule: %v", err)
	}

	// Seed relative to the fixed service clock so this test does not expire.
	store.now = func() time.Time { return fixed }
	store.programs["p-new"] = Program{
		ID: "p-new", ChannelID: channelID, Title: "Evening News", SeriesID: "evening-news", IsNew: true,
		Start: fixed.Add(2 * time.Hour), Stop: fixed.Add(3 * time.Hour),
	}
	store.programs["p-old"] = Program{
		ID: "p-old", ChannelID: channelID, Title: "Evening News", SeriesID: "evening-news", IsNew: false,
		Start: fixed.Add(4 * time.Hour), Stop: fixed.Add(5 * time.Hour),
	}
	store.programs["p-other"] = Program{
		ID: "p-other", ChannelID: "ch2", Title: "Evening News", SeriesID: "evening-news", IsNew: true,
		Start: fixed.Add(2 * time.Hour), Stop: fixed.Add(3 * time.Hour),
	}

	if err := svc.ApplySeriesRules(context.Background()); err != nil {
		t.Fatalf("ApplySeriesRules: %v", err)
	}
	recs, _ := svc.ListRecordings(context.Background(), "")
	if len(recs) != 1 || recs[0].ProgramID != "p-new" || recs[0].SeriesRuleID != rule.ID {
		t.Fatalf("recordings after apply = %+v", recs)
	}

	// Second apply should not duplicate (pair set is preloaded once per apply).
	if err := svc.ApplySeriesRules(context.Background()); err != nil {
		t.Fatalf("ApplySeriesRules second: %v", err)
	}
	recs, _ = svc.ListRecordings(context.Background(), "")
	if len(recs) != 1 {
		t.Fatalf("duplicate recordings = %+v", recs)
	}

	rules, err := svc.ListSeriesRules(context.Background())
	if err != nil || len(rules) != 1 {
		t.Fatalf("ListSeriesRules = %+v err=%v", rules, err)
	}
	if err := svc.DeleteSeriesRule(context.Background(), rule.ID); err != nil {
		t.Fatalf("DeleteSeriesRule: %v", err)
	}
	rules, _ = svc.ListSeriesRules(context.Background())
	if len(rules) != 0 {
		t.Fatalf("rules after delete = %+v", rules)
	}
}

func TestApplySeriesRulesSkipsPreloadedPairs(t *testing.T) {
	store := newMemoryStore()
	fixed := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	svc := NewServiceWithStore(store)
	svc.now = func() time.Time { return fixed }
	store.now = func() time.Time { return fixed }

	channelID := "ch1"
	rule, err := svc.CreateSeriesRule(context.Background(), &SeriesRule{
		TitleMatch: "news", Enabled: true, ChannelID: &channelID,
	})
	if err != nil {
		t.Fatalf("CreateSeriesRule: %v", err)
	}
	store.programs["p1"] = Program{
		ID: "p1", ChannelID: channelID, Title: "News Hour", SeriesID: "news-hour", IsNew: true,
		Start: fixed.Add(time.Hour), Stop: fixed.Add(2 * time.Hour),
	}
	store.recordings["r1"] = Recording{
		ID: "r1", ProgramID: "p1", ChannelID: channelID, SeriesRuleID: rule.ID,
		Status: "scheduled", Start: fixed.Add(time.Hour), Stop: fixed.Add(2 * time.Hour), Title: "News Hour",
	}

	if err := svc.ApplySeriesRules(context.Background()); err != nil {
		t.Fatalf("ApplySeriesRules: %v", err)
	}
	recs, err := svc.ListRecordings(context.Background(), "")
	if err != nil {
		t.Fatalf("ListRecordings: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected no double-schedule with preloaded pair, got %+v", recs)
	}
}

func TestReplaceChannelsForTunerPreservesIDAndDeletesRemoved(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 2, Status: "ready"}
	override := "99.1"
	if err := store.ReplaceChannelsForTuner(context.Background(), "t1", []Channel{
		{Number: "5.1", Callsign: "KING", Name: "KING", Enabled: true, StreamURL: "http://x/auto/v5.1"},
		{Number: "7.1", Callsign: "KIRO", Name: "KIRO", Enabled: true, StreamURL: "http://x/auto/v7.1"},
	}); err != nil {
		t.Fatalf("initial replace: %v", err)
	}
	channels, err := store.ListChannels(context.Background(), "t1")
	if err != nil || len(channels) != 2 {
		t.Fatalf("initial channels = %+v err=%v", channels, err)
	}
	var keptID, removedID string
	for _, ch := range channels {
		switch ch.Number {
		case "5.1":
			keptID = ch.ID
			enabled := false
			_, err := store.UpdateChannel(context.Background(), ch.ID, ChannelPatch{
				Enabled: &enabled, NumberOverride: &override, GuideStationID: strPtr("GUIDE-KING"),
			})
			if err != nil {
				t.Fatalf("UpdateChannel: %v", err)
			}
		case "7.1":
			removedID = ch.ID
		}
	}
	if keptID == "" || removedID == "" {
		t.Fatalf("missing channel ids: kept=%q removed=%q", keptID, removedID)
	}

	if err := store.ReplaceChannelsForTuner(context.Background(), "t1", []Channel{
		{Number: "5.1", Callsign: "KING", Name: "KING HD", Enabled: true, StreamURL: "http://x/auto/v5.1", HD: true},
		{Number: "9.1", Callsign: "KCTS", Name: "KCTS", Enabled: true, StreamURL: "http://x/auto/v9.1"},
	}); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	channels, err = store.ListChannels(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("after replace channels = %+v", channels)
	}
	var foundKept, foundNew bool
	for _, ch := range channels {
		switch ch.Number {
		case "5.1":
			foundKept = true
			if ch.ID != keptID {
				t.Fatalf("kept channel id changed: got %q want %q", ch.ID, keptID)
			}
			if ch.Enabled {
				t.Fatalf("expected enabled override preserved as false: %+v", ch)
			}
			if ch.NumberOverride == nil || *ch.NumberOverride != override {
				t.Fatalf("number_override not preserved: %+v", ch)
			}
			if ch.GuideStationID != "GUIDE-KING" {
				t.Fatalf("guide_station_id not preserved: %+v", ch)
			}
			if ch.Name != "KING HD" || !ch.HD {
				t.Fatalf("scanned fields not updated: %+v", ch)
			}
		case "9.1":
			foundNew = true
			if ch.ID == "" || ch.ID == keptID || ch.ID == removedID {
				t.Fatalf("new channel unexpected id: %+v", ch)
			}
		case "7.1":
			t.Fatalf("removed channel still present: %+v", ch)
		}
	}
	if !foundKept || !foundNew {
		t.Fatalf("expected kept+new channels, got %+v", channels)
	}
	if _, ok := store.channels[removedID]; ok {
		t.Fatalf("removed channel id %q still in store", removedID)
	}
}

func strPtr(v string) *string { return &v }

func TestMatchesRuleHelpers(t *testing.T) {
	ch := "ch1"
	rule := SeriesRule{SeriesID: "news", ChannelID: &ch, TitleMatch: "Evening", NewOnly: true, Enabled: true}
	p := Program{ChannelID: "ch1", SeriesID: "news", Title: "Evening News", IsNew: true}
	if !matchesRule(rule, p) {
		t.Fatal("expected match")
	}
	p.IsNew = false
	if matchesRule(rule, p) {
		t.Fatal("new_only should reject")
	}
	idx, ok := firstFreeIndex(2, []int{0})
	if !ok || idx != 1 {
		t.Fatalf("firstFreeIndex = %d,%v", idx, ok)
	}
	_, ok = firstFreeIndex(1, []int{0})
	if ok {
		t.Fatal("expected no free index")
	}
	if stableSeriesID(" Evening  News ") != "evening-news" {
		t.Fatalf("stableSeriesID = %q", stableSeriesID(" Evening  News "))
	}
}

func TestGetChannel(t *testing.T) {
	store := newMemoryStore()
	store.channels["ch1"] = Channel{ID: "ch1", Number: "5.1", Enabled: true}
	svc := NewServiceWithStore(store)
	ch, err := svc.GetChannel(context.Background(), "ch1")
	if err != nil || ch == nil || ch.Number != "5.1" {
		t.Fatalf("GetChannel = %+v err=%v", ch, err)
	}
	_, err = svc.GetChannel(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
	_, err = NewServiceWithStore(nil).GetChannel(context.Background(), "ch1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	_, err = NewServiceWithStore(nil).ListTuners(context.Background())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListTuners nil store = %v", err)
	}
}

func TestCreateGuideSourceInvalidType(t *testing.T) {
	svc := NewServiceWithStore(newMemoryStore())
	_, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: "bogus", Enabled: true, DisplayName: "x",
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error = %v", err)
	}
	_, err = svc.UpdateGuideSource(context.Background(), &GuideSource{ID: "missing", Type: GuideSourceXMLTVURL})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing = %v", err)
	}
	_, err = svc.GetProgram(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProgram missing = %v", err)
	}
	if err := svc.SyncGuideSource(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SyncGuideSource missing = %v", err)
	}
}

func TestAddTunerRequiresConfiguredService(t *testing.T) {
	svc := NewServiceWithStore(nil)
	_, err := svc.AddTuner(context.Background(), "http://x", "")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

type stubPlaybackBridge struct{}

func (stubPlaybackBridge) StartLiveStream(context.Context, string, string, int, string) (string, string, error) {
	return "pb-1", "http://play/hls.m3u8", nil
}

type memoryStore struct {
	mu           sync.Mutex
	next         int
	now          func() time.Time
	tuners       map[string]Tuner
	channels     map[string]Channel
	guideSources map[string]GuideSource
	programs     map[string]Program
	sessions     map[string]LiveSession
	recordings   map[string]Recording
	rules        map[string]SeriesRule
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		next:         1,
		tuners:       map[string]Tuner{},
		channels:     map[string]Channel{},
		guideSources: map[string]GuideSource{},
		programs:     map[string]Program{},
		sessions:     map[string]LiveSession{},
		recordings:   map[string]Recording{},
		rules:        map[string]SeriesRule{},
	}
}

func (s *memoryStore) id() string {
	id := s.next
	s.next++
	return fmt.Sprintf("id-%d", id)
}

func (s *memoryStore) ListTuners(context.Context) ([]Tuner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Tuner, 0, len(s.tuners))
	for _, tuner := range s.tuners {
		tuner.ChannelCount = s.channelCountLocked(tuner.ID)
		out = append(out, tuner)
	}
	return out, nil
}

func (s *memoryStore) GetTuner(_ context.Context, id string) (*Tuner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tuner, ok := s.tuners[id]
	if !ok {
		return nil, nil
	}
	tuner.ChannelCount = s.channelCountLocked(id)
	return &tuner, nil
}

func (s *memoryStore) CreateTuner(_ context.Context, tuner *Tuner) (*Tuner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.tuners {
		if existing.Type == tuner.Type && existing.DeviceID == tuner.DeviceID {
			tuner.ID = id
			break
		}
	}
	if tuner.ID == "" {
		tuner.ID = s.id()
	}
	s.tuners[tuner.ID] = *tuner
	out := s.tuners[tuner.ID]
	out.ChannelCount = s.channelCountLocked(tuner.ID)
	return &out, nil
}

func (s *memoryStore) DeleteTuner(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tuners, id)
	return nil
}

func (s *memoryStore) ReplaceChannelsForTuner(_ context.Context, tunerID string, channels []Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := map[string]Channel{}
	for _, ch := range s.channels {
		if ch.TunerID == tunerID {
			existing[channelKey(ch.Number, ch.Callsign, ch.StreamURL)] = ch
		}
	}
	kept := map[string]struct{}{}
	for i := range channels {
		ch := channels[i]
		ch.TunerID = tunerID
		key := channelKey(ch.Number, ch.Callsign, ch.StreamURL)
		if prev, ok := existing[key]; ok {
			ch.ID = prev.ID
			ch.NumberOverride = prev.NumberOverride
			ch.Enabled = prev.Enabled
			ch.GuideStationID = prev.GuideStationID
			kept[key] = struct{}{}
		} else if ch.ID == "" {
			ch.ID = s.id()
		}
		s.channels[ch.ID] = ch
	}
	for key, prev := range existing {
		if _, ok := kept[key]; ok {
			continue
		}
		delete(s.channels, prev.ID)
	}
	tuner := s.tuners[tunerID]
	tuner.Status = "ready"
	now := time.Now()
	tuner.LastScanAt = &now
	s.tuners[tunerID] = tuner
	return nil
}

func (s *memoryStore) ListChannels(_ context.Context, tunerID string) ([]Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Channel{}
	for _, ch := range s.channels {
		if tunerID == "" || ch.TunerID == tunerID {
			out = append(out, ch)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (s *memoryStore) GetChannel(_ context.Context, id string) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[id]
	if !ok {
		return nil, nil
	}
	return &ch, nil
}

func (s *memoryStore) UpdateChannel(_ context.Context, id string, patch ChannelPatch) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[id]
	if !ok {
		return nil, nil
	}
	if patch.Enabled != nil {
		ch.Enabled = *patch.Enabled
	}
	if patch.NumberOverride != nil {
		ch.NumberOverride = patch.NumberOverride
	}
	if patch.GuideStationID != nil {
		ch.GuideStationID = *patch.GuideStationID
	}
	s.channels[id] = ch
	return &ch, nil
}

func (s *memoryStore) ListGuideSources(_ context.Context, enabledOnly bool) ([]GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []GuideSource{}
	for _, source := range s.guideSources {
		if !enabledOnly || source.Enabled {
			out = append(out, source)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out, nil
}

func (s *memoryStore) GetGuideSource(_ context.Context, id string) (*GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok := s.guideSources[id]
	if !ok {
		return nil, nil
	}
	return &source, nil
}

func (s *memoryStore) CreateGuideSource(_ context.Context, source *GuideSource) (*GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if source.ID == "" {
		source.ID = s.id()
	}
	s.guideSources[source.ID] = *source
	out := s.guideSources[source.ID]
	return &out, nil
}

func (s *memoryStore) UpdateGuideSource(_ context.Context, source *GuideSource) (*GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.guideSources[source.ID]; !ok {
		return nil, nil
	}
	s.guideSources[source.ID] = *source
	out := s.guideSources[source.ID]
	return &out, nil
}

func (s *memoryStore) DeleteGuideSource(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.guideSources, id)
	return nil
}

func (s *memoryStore) SetGuideSourceSyncStatus(_ context.Context, id, status, lastError string, lastSyncAt, nextSyncAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	source := s.guideSources[id]
	source.Status = status
	source.LastError = lastError
	source.LastSyncAt = lastSyncAt
	source.NextSyncAt = nextSyncAt
	s.guideSources[id] = source
	return nil
}

func (s *memoryStore) UpsertPrograms(_ context.Context, sourceID string, programs []Program) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range programs {
		if programs[i].ID == "" {
			programs[i].ID = s.id()
		}
		programs[i].SourceID = sourceID
		s.programs[programs[i].ID] = programs[i]
	}
	return nil
}

func (s *memoryStore) ListGuide(_ context.Context, channelIDs []string, start, end time.Time) ([]Program, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]struct{}{}
	for _, id := range channelIDs {
		want[id] = struct{}{}
	}
	out := []Program{}
	for _, p := range s.programs {
		if len(want) > 0 {
			if _, ok := want[p.ChannelID]; !ok {
				continue
			}
		}
		if p.Start.Before(end) && p.Stop.After(start) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].ChannelID < out[j].ChannelID
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out, nil
}

func (s *memoryStore) GetProgram(_ context.Context, id string) (*Program, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.programs[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (s *memoryStore) ListUpcomingPrograms(_ context.Context, until time.Time) ([]Program, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	out := []Program{}
	for _, p := range s.programs {
		if p.Stop.After(now) && !p.Start.After(until) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *memoryStore) ActiveSessionTunerIndices(_ context.Context, tunerID string) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var indices []int
	for _, session := range s.sessions {
		if session.TunerID == tunerID && session.Status == "active" {
			indices = append(indices, session.TunerIndex)
		}
	}
	return indices, nil
}

func (s *memoryStore) CreateSession(_ context.Context, input SessionCreate) (*LiveSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.sessions {
		if existing.TunerID == input.TunerID && existing.TunerIndex == input.TunerIndex && existing.Status == "active" {
			return nil, ErrTunerIndexConflict
		}
	}
	session := LiveSession{
		ID:                s.id(),
		ChannelID:         input.ChannelID,
		TunerID:           input.TunerID,
		TunerIndex:        input.TunerIndex,
		UserID:            input.UserID,
		ProfileID:         input.ProfileID,
		PlaybackSessionID: input.PlaybackSessionID,
		Status:            "active",
		CreatedAt:         time.Now().UTC(),
	}
	s.sessions[session.ID] = session
	return &session, nil
}

func (s *memoryStore) GetSession(_ context.Context, id string) (*LiveSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, nil
	}
	return &session, nil
}

func (s *memoryStore) ReleaseSession(_ context.Context, id string) (*LiveSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, nil
	}
	now := time.Now().UTC()
	session.Status = "released"
	session.ReleasedAt = &now
	s.sessions[id] = session
	return &session, nil
}

func (s *memoryStore) ListRecordings(_ context.Context, status string) ([]Recording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Recording{}
	for _, rec := range s.recordings {
		if status == "" || rec.Status == status {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.After(out[j].Start) })
	return out, nil
}

func (s *memoryStore) CreateRecording(_ context.Context, rec *Recording) (*Recording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.ID == "" {
		rec.ID = s.id()
	}
	if rec.Status == "" {
		rec.Status = "scheduled"
	}
	s.recordings[rec.ID] = *rec
	out := s.recordings[rec.ID]
	return &out, nil
}

func (s *memoryStore) CancelRecording(_ context.Context, id string) (*Recording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.recordings[id]
	if !ok {
		return nil, nil
	}
	rec.Status = "cancelled"
	s.recordings[id] = rec
	return &rec, nil
}

func (s *memoryStore) RecordingExists(_ context.Context, programID, seriesRuleID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range s.recordings {
		if rec.ProgramID == programID && rec.SeriesRuleID == seriesRuleID && rec.Status != "cancelled" {
			return true, nil
		}
	}
	return false, nil
}

func (s *memoryStore) ListActiveRecordingPairs(context.Context) (map[string]struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]struct{}{}
	for _, rec := range s.recordings {
		if rec.Status == "cancelled" {
			continue
		}
		out[recordingPairKey(rec.ProgramID, rec.SeriesRuleID)] = struct{}{}
	}
	return out, nil
}

func (s *memoryStore) FailDueRecordings(_ context.Context, now time.Time, message string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for id, rec := range s.recordings {
		if rec.Status == "scheduled" && !rec.Start.After(now) {
			rec.Status = "failed"
			rec.LastError = message
			s.recordings[id] = rec
			count++
		}
	}
	return count, nil
}

func (s *memoryStore) ListSeriesRules(context.Context) ([]SeriesRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SeriesRule, 0, len(s.rules))
	for _, rule := range s.rules {
		out = append(out, rule)
	}
	return out, nil
}

func (s *memoryStore) CreateSeriesRule(_ context.Context, rule *SeriesRule) (*SeriesRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rule.ID == "" {
		rule.ID = s.id()
	}
	s.rules[rule.ID] = *rule
	out := s.rules[rule.ID]
	return &out, nil
}

func (s *memoryStore) DeleteSeriesRule(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rules, id)
	return nil
}

func (s *memoryStore) GetRecording(id string) (*Recording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.recordings[id]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}

func (s *memoryStore) channelCountLocked(tunerID string) int {
	count := 0
	for _, ch := range s.channels {
		if ch.TunerID == tunerID {
			count++
		}
	}
	return count
}
