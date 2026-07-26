package livetv

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
)

func TestPublicSessionStreamPathAndSafePlayURL(t *testing.T) {
	if got := PublicSessionStreamPath("abc"); got != "/api/v1/livetv/sessions/abc/stream" {
		t.Fatalf("PublicSessionStreamPath = %q", got)
	}
	cases := map[string]bool{
		"":                                 false,
		"   ":                              false,
		"/api/v1/livetv/sessions/x/stream": true,
		"//evil.example/x":                 false,
		"https://tuner.lan/stream":         false,
		"http://169.254.169.254/":          false,
	}
	for raw, want := range cases {
		if got := IsClientSafePlayURL(raw); got != want {
			t.Fatalf("IsClientSafePlayURL(%q)=%v want %v", raw, got, want)
		}
	}
}

func TestChannelAndSessionMarshalJSONRedactUpstream(t *testing.T) {
	ch := Channel{ID: "c1", StreamURL: "http://192.168.1.5/auto/v5.1", Name: "KING"}
	raw, err := json.Marshal(ch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["stream_url"] != "" {
		t.Fatalf("channel stream_url = %#v, want empty string", m["stream_url"])
	}
	if m["name"] != "KING" {
		t.Fatalf("name = %#v", m["name"])
	}

	sess := LiveSession{
		ID:        "s1",
		ChannelID: "c1",
		HLSURL:    "http://192.168.1.5/auto/v5.1",
		StreamURL: "http://192.168.1.5/auto/v5.1",
		Status:    "active",
	}
	raw, err = json.Marshal(sess)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	wantPath := PublicSessionStreamPath("s1")
	if m["hls_url"] != wantPath {
		t.Fatalf("hls_url = %#v want %q", m["hls_url"], wantPath)
	}
	if m["stream_url"] != wantPath {
		t.Fatalf("stream_url = %#v want %q", m["stream_url"], wantPath)
	}
}

func TestOwnerMatchesAndSessionViewer(t *testing.T) {
	if ownerMatches(0, "", 1, "p") {
		t.Fatal("unscoped legacy must not match")
	}
	if !ownerMatches(7, "p1", 7, "p1") {
		t.Fatal("exact owner should match")
	}
	if ownerMatches(7, "p1", 8, "p1") {
		t.Fatal("wrong user must not match")
	}
	if ownerMatches(7, "p1", 7, "p2") {
		t.Fatal("wrong profile must not match")
	}

	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 2, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Number: "5.1", Enabled: true,
		StreamURL: "http://192.168.1.50/stream",
	}
	svc := NewServiceWithStore(store)
	ctx := context.Background()

	sess, err := store.CreateSession(ctx, SessionCreate{
		ChannelID: "ch1", TunerID: "t1", TunerIndex: 0, UserID: 7, ProfileID: "p1",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Keep status active for ResolveSessionUpstreamURL.
	store.mu.Lock()
	s := store.sessions[sess.ID]
	s.Status = "active"
	store.sessions[sess.ID] = s
	store.mu.Unlock()

	got, err := svc.GetSessionForViewer(ctx, sess.ID, 7, "p1", true)
	if err != nil || got == nil || got.ID != sess.ID {
		t.Fatalf("GetSessionForViewer owner = %+v err=%v", got, err)
	}
	if _, err := svc.GetSessionForViewer(ctx, sess.ID, 8, "p1", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user err = %v, want ErrNotFound", err)
	}

	up, err := svc.ResolveSessionUpstreamURL(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ResolveSessionUpstreamURL: %v", err)
	}
	if up != "http://192.168.1.50/stream" {
		t.Fatalf("upstream = %q", up)
	}
}

func TestStreamHTTPClientOmitsTimeout(t *testing.T) {
	media := NewMediaHTTPClient()
	if media.Timeout <= 0 {
		t.Fatal("media client should have a timeout")
	}
	stream := NewStreamHTTPClient()
	if stream.Timeout != 0 {
		t.Fatalf("stream client Timeout = %v, want 0", stream.Timeout)
	}
	if stream.CheckRedirect == nil || stream.Transport == nil {
		t.Fatal("stream client missing redirect/transport safeguards")
	}

	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/next", nil)
	via := []*http.Request{req, req, req}
	if err := mediaCheckRedirect(req, via); err == nil {
		t.Fatal("expected too many redirects")
	}
	bad, _ := http.NewRequest(http.MethodGet, "http://169.254.169.254/", nil)
	if err := mediaCheckRedirect(bad, nil); err == nil {
		t.Fatal("expected metadata redirect rejection")
	}
	okURL, _ := url.Parse("http://192.168.1.50/lineup.json")
	okReq := &http.Request{URL: okURL}
	if err := mediaCheckRedirect(okReq, nil); err != nil {
		t.Fatalf("lan redirect should pass: %v", err)
	}
}

func TestListRecordingsAndSeriesRulesOwnershipFilter(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	ctx := context.Background()

	store.recordings["r1"] = Recording{ID: "r1", UserID: 1, ProfileID: "a", Status: "scheduled", Title: "Mine"}
	store.recordings["r2"] = Recording{ID: "r2", UserID: 2, ProfileID: "b", Status: "scheduled", Title: "Other"}
	store.rules["s1"] = SeriesRule{ID: "s1", UserID: 1, ProfileID: "a", TitleMatch: "Mine", Enabled: true}
	store.rules["s2"] = SeriesRule{ID: "s2", UserID: 2, ProfileID: "b", TitleMatch: "Other", Enabled: true}

	recs, err := svc.ListRecordings(ctx, "", 1, "a", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ID != "r1" {
		t.Fatalf("ListRecordings = %+v", recs)
	}
	rules, err := svc.ListSeriesRules(ctx, 1, "a", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != "s1" {
		t.Fatalf("ListSeriesRules = %+v", rules)
	}

	// Cover DeleteSeriesRule ownership fail-closed for unscoped rows.
	store.rules["legacy"] = SeriesRule{ID: "legacy", UserID: 0, ProfileID: "", TitleMatch: "X", Enabled: true}
	if err := svc.DeleteSeriesRule(ctx, "legacy", 1, "a", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteSeriesRule unscoped = %v", err)
	}
}
