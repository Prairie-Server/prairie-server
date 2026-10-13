package apiv2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/api/handlers"
	"github.com/prairie-server/prairie-server/internal/livetv"
)

// fakeLiveTV implements the viewer operations the tests exercise; any other
// method panics through the nil embedded interface.
type fakeLiveTV struct {
	LiveTVService
	channels []livetv.Channel
	start    handlers.LiveTVSessionStart
	startErr error
	caps     livetv.ClientCapabilities
	tunerID  string
}

func (f *fakeLiveTV) ListChannels(_ context.Context, tunerID string) ([]livetv.Channel, error) {
	f.tunerID = tunerID
	return f.channels, nil
}

func (f *fakeLiveTV) StartChannelSession(_ context.Context, _ string, caps livetv.ClientCapabilities) (handlers.LiveTVSessionStart, error) {
	f.caps = caps
	return f.start, f.startErr
}

func TestLiveTVV2ChannelsAndSessionStart(t *testing.T) {
	deps, _ := catalogDeps(t)
	h := newTestHandler(t, deps)
	requireProblem(t, do(t, h, http.MethodGet, Prefix+"/livetv/channels", "", viewerHeaders()), TypeCapabilityNotConfigured)

	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fake := &fakeLiveTV{
		channels: []livetv.Channel{{ID: "ch-1", TunerID: "t-1", Number: "4.1", Callsign: "WXYZ", Name: "Channel 4", HD: true, Enabled: true, StreamURL: "http://10.0.0.5:5004/auto/v4.1"}},
		start: handlers.LiveTVSessionStart{
			Session:        livetv.LiveSession{ID: "sess-1", ChannelID: "ch-1", Status: "active", CreatedAt: created, HLSURL: "http://10.0.0.5/raw"},
			PlaybackTicket: "ticket-1",
			StreamURL:      "/api/v1/livetv/live-hls/pb-1/index.m3u8?st=signed",
			Transport:      "hls",
		},
	}
	deps.LiveTV = fake
	h = newTestHandler(t, deps)

	rec := do(t, h, http.MethodGet, Prefix+"/livetv/channels?tuner_id=t-1", "", viewerHeaders())
	if rec.Code != 200 {
		t.Fatalf("channels: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"items":[{"id":"ch-1"`) || strings.Contains(body, "10.0.0.5") || fake.tunerID != "t-1" {
		t.Fatalf("channels body %s tuner=%q", body, fake.tunerID)
	}

	rec = do(t, h, http.MethodPost, Prefix+"/livetv/channels/ch-1/sessions", `{"client_capabilities":{"codecs_video":["h264"],"max_audio_channels":6}}`, viewerHeaders())
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	var session LiveTVSession
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.SessionID != "sess-1" || session.StreamURL != "/api/v1/livetv/live-hls/pb-1/index.m3u8?st=signed" || session.Transport != "hls" || session.PlaybackTicket != "ticket-1" || session.StaleAfterSeconds != 90 {
		t.Fatalf("session %+v", session)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("raw tuner url leaked: %s", rec.Body.String())
	}
	if len(fake.caps.CodecsVideo) != 1 || fake.caps.MaxAudioChannels != 6 {
		t.Fatalf("caps %+v", fake.caps)
	}
	// The body is optional.
	if rec = do(t, h, http.MethodPost, Prefix+"/livetv/channels/ch-1/sessions", "", viewerHeaders()); rec.Code >= 300 {
		t.Fatalf("start without body: %d %s", rec.Code, rec.Body.String())
	}
	fake.startErr = livetv.ErrNoTuner
	requireProblem(t, do(t, h, http.MethodPost, Prefix+"/livetv/channels/ch-1/sessions", "", viewerHeaders()), TypeConflict)
	fake.startErr = livetv.ErrNotFound
	requireProblem(t, do(t, h, http.MethodPost, Prefix+"/livetv/channels/ch-1/sessions", "", viewerHeaders()), TypeNotFound)
	requireProblem(t, do(t, h, http.MethodGet, Prefix+"/livetv/channels", "", nil), TypeAuthenticationRequired)
}

func TestPlaybackV2QualityLadder(t *testing.T) {
	deps, _ := catalogDeps(t)
	h := newTestHandler(t, deps)
	var full, capped PlaybackQualityLadder
	for path, into := range map[string]*PlaybackQualityLadder{"": &full, "?source_height=720": &capped} {
		rec := do(t, h, http.MethodGet, Prefix+"/playback/quality-ladder"+path, "", viewerHeaders())
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("ladder%s: %d %q %s", path, rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatal(err)
		}
	}
	if len(full.Rungs) == 0 || len(capped.Rungs) >= len(full.Rungs) || capped.SourceHeight != 720 || full.SourceHeight != 0 {
		t.Fatalf("full %+v capped %+v", full, capped)
	}
	for _, rung := range capped.Rungs {
		if rung.Height > 720 {
			t.Fatalf("rung above source: %+v", rung)
		}
	}
	if strings.Join(full.Modes, ",") != "auto,original" {
		t.Fatalf("modes %v", full.Modes)
	}
	requireProblem(t, do(t, h, http.MethodGet, Prefix+"/playback/quality-ladder?source_height=-1", "", viewerHeaders()), TypeValidationFailed)
	requireProblem(t, do(t, h, http.MethodGet, Prefix+"/playback/quality-ladder", "", nil), TypeAuthenticationRequired)
}

func clientEventsBody(n int, name string) string {
	events := make([]map[string]any, n)
	for i := range events {
		events[i] = map[string]any{"event": name, "playback_session_id": "sess-1"}
	}
	events[0]["occurred_at"] = "2026-10-01T12:00:00.000Z"
	data, _ := json.Marshal(map[string]any{"events": events})
	return string(data)
}

func TestClientDiagnosticEvents(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	deps, _ := catalogDeps(t)
	h := newTestHandler(t, deps)
	path := Prefix + "/diagnostics/client-events"
	headers := with(viewerHeaders(), "X-Prairie-Device-Id", "shield-living-room")

	rec := do(t, h, http.MethodPost, path, clientEventsBody(2, "player_buffering_start"), headers)
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), `"accepted":2`) {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body.String())
	}
	var logged []string
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "component=client_diagnostics") {
			logged = append(logged, line)
		}
	}
	if len(logged) != 2 {
		t.Fatalf("logged %d events: %s", len(logged), logs.String())
	}
	for _, want := range []string{"event=player_buffering_start", "device_id=shield-living-room", "profile_id=p-owner", "user_id=", "playback_session_id=sess-1", "occurred_at=2026-10-01T12:00:00Z"} {
		if !strings.Contains(logged[0], want) {
			t.Fatalf("log line missing %q: %s", want, logged[0])
		}
	}

	for name, body := range map[string]string{
		"too many events": clientEventsBody(21, "x"),
		"empty batch":     `{"events":[]}`,
		"bad name":        clientEventsBody(1, "has spaces"),
		"long name":       clientEventsBody(1, strings.Repeat("x", 65)),
		"unknown field":   `{"events":[{"event":"x","extra":1}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			requireProblem(t, do(t, h, http.MethodPost, path, body, headers), TypeValidationFailed)
		})
	}
	oversize := `{"events":[{"event":"x"}]` + strings.Repeat(" ", clientEventsMaxBodyBytes) + `}`
	requireProblem(t, do(t, h, http.MethodPost, path, oversize, headers), TypePayloadTooLarge)
	requireProblem(t, do(t, h, http.MethodPost, path, clientEventsBody(1, "x"), nil), TypeAuthenticationRequired)

	// A fresh registration has a fresh window: the 31st batch in a minute is refused.
	h = newTestHandler(t, deps)
	for i := 0; i < clientEventsPerMinute; i++ {
		if rec := do(t, h, http.MethodPost, path, clientEventsBody(1, fmt.Sprintf("e%d", i)), headers); rec.Code != 202 {
			t.Fatalf("batch %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec = do(t, h, http.MethodPost, path, clientEventsBody(1, "x"), headers)
	requireProblem(t, rec, TypeRateLimited)
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After %q", rec.Header().Get("Retry-After"))
	}
}

func TestClientEventLimiterWindow(t *testing.T) {
	var l clientEventLimiter
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < clientEventsPerMinute; i++ {
		if !l.allow(1, now) {
			t.Fatalf("batch %d refused", i)
		}
	}
	if l.allow(1, now.Add(59*time.Second)) || !l.allow(2, now) || !l.allow(1, now.Add(time.Minute)) {
		t.Fatal("window not per account or not reset after a minute")
	}
}
