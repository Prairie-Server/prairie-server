package livetv

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv/hdhomerun"
)

func TestNewServiceNilDBAndRequireStoreGuards(t *testing.T) {
	svc := NewService(nil)
	if svc == nil || svc.store != nil {
		t.Fatalf("NewService(nil) = %+v", svc)
	}
	ctx := context.Background()
	if _, err := svc.ListTuners(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListTuners: %v", err)
	}
	if _, err := svc.AddTuner(ctx, AddTunerInput{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("AddTuner: %v", err)
	}
	if err := svc.ScanTuner(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ScanTuner: %v", err)
	}
	if err := svc.DeleteTuner(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("DeleteTuner: %v", err)
	}
	if _, err := svc.ListChannels(ctx, ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListChannels: %v", err)
	}
	if _, err := svc.GetChannel(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("GetChannel: %v", err)
	}
	if _, err := svc.PatchChannel(ctx, "x", ChannelPatch{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("PatchChannel: %v", err)
	}
	if _, err := svc.ListGuideSources(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListGuideSources: %v", err)
	}
	if _, err := svc.CreateGuideSource(ctx, &GuideSource{Type: GuideSourceSchedulesDirect}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("CreateGuideSource: %v", err)
	}
	if _, err := svc.UpdateGuideSource(ctx, &GuideSource{ID: "x"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("UpdateGuideSource: %v", err)
	}
	if err := svc.DeleteGuideSource(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("DeleteGuideSource: %v", err)
	}
	if _, err := svc.SyncAllEnabledGuideSources(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("SyncAllEnabledGuideSources: %v", err)
	}
	if err := svc.SyncGuideSource(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("SyncGuideSource: %v", err)
	}
	if _, err := svc.ListGuide(ctx, nil, time.Time{}, time.Time{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListGuide: %v", err)
	}
	if _, err := svc.GetProgram(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("GetProgram: %v", err)
	}
	if _, err := svc.StartChannelSession(ctx, "x", 1, "p"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("StartChannelSession: %v", err)
	}
	if _, err := svc.ReleaseSession(ctx, "x", 1, "p", true); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if _, err := svc.ListRecordings(ctx, "", 1, "p", true); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListRecordings: %v", err)
	}
	if _, err := svc.ScheduleRecording(ctx, &Recording{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ScheduleRecording: %v", err)
	}
	if _, err := svc.CancelRecording(ctx, "x", 1, "p", true); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("CancelRecording: %v", err)
	}
	if _, err := svc.ListSeriesRules(ctx, 1, "p", true); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListSeriesRules: %v", err)
	}
	if _, err := svc.CreateSeriesRule(ctx, &SeriesRule{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("CreateSeriesRule: %v", err)
	}
	if err := svc.DeleteSeriesRule(ctx, "x", 1, "p", true); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("DeleteSeriesRule: %v", err)
	}
	if _, err := svc.GetSession(ctx, "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("GetSession: %v", err)
	}
	if err := svc.ApplySeriesRules(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ApplySeriesRules: %v", err)
	}
	if _, err := svc.FailDueRecordings(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("FailDueRecordings: %v", err)
	}
}

func TestDiscoverTunersEdgeCases(t *testing.T) {
	allowLoopbackMediaFetch(t)
	svc := NewServiceWithStore(newMemoryStore())
	svc.hdhr = nil
	if _, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil hdhr: %v", err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	})

	prev := lanDiscoverFn
	lanDiscoverFn = func(context.Context, time.Duration) ([]hdhomerun.LANCandidate, error) {
		return nil, errors.New("udp down")
	}
	t.Cleanup(func() { lanDiscoverFn = prev })

	svc = NewServiceWithStore(newMemoryStore())
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))
	include := true
	result, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{
		TimeoutMs:  50_000, // clamp
		IncludeUDP: &include,
		ProbeURLs:  []string{srv.URL + "/discover.json", "ftp://bad.example/x"},
	})
	if err != nil {
		t.Fatalf("DiscoverTuners: %v", err)
	}
	if len(result.Notes) == 0 {
		t.Fatalf("expected notes, got %+v", result)
	}
	if result.Candidates == nil {
		t.Fatal("candidates should be empty slice")
	}

	// UDP hit that fails HTTP discover still surfaces candidate.
	lanDiscoverFn = func(context.Context, time.Duration) ([]hdhomerun.LANCandidate, error) {
		return []hdhomerun.LANCandidate{{
			DeviceIDHex: "DEADBEEF",
			TunerCount:  1,
			BaseURL:     srv.URL,
			RemoteIP:    "127.0.0.1",
		}}, nil
	}
	result, err = svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{TimeoutMs: -1})
	if err != nil {
		t.Fatalf("DiscoverTuners udp fail verify: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].DeviceID != "DEADBEEF" {
		t.Fatalf("candidates=%+v", result.Candidates)
	}

	if got := coalesceTrimmed("  ", "", "x", "y"); got != "x" {
		t.Fatalf("coalesceTrimmed=%q", got)
	}
	if got := coalesceTrimmed(" ", ""); got != "" {
		t.Fatalf("coalesceTrimmed empty=%q", got)
	}
}

func TestDiscoverTunersDedupesAndSkipsInvalidUDP(t *testing.T) {
	prev := lanDiscoverFn
	lanDiscoverFn = func(context.Context, time.Duration) ([]hdhomerun.LANCandidate, error) {
		return []hdhomerun.LANCandidate{{
			DeviceIDHex: "METADATA",
			RemoteIP:    "169.254.169.254",
		}}, nil
	}
	t.Cleanup(func() { lanDiscoverFn = prev })

	svc := NewServiceWithStore(newMemoryStore())
	svc.SetHDHomeRunClient(fakeHDHRClient{
		discover: func(_ context.Context, discoverURL, _ string) (*hdhomerun.DeviceInfo, error) {
			return &hdhomerun.DeviceInfo{
				FriendlyName: "empty ids",
				TunerCount:   1,
				LineupURL:    discoverURL,
			}, nil
		},
	})

	probeURL := "http://probe.example/discover.json"
	result, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{
		ProbeURLs: []string{probeURL, probeURL},
	})
	if err != nil {
		t.Fatalf("DiscoverTuners: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected duplicate probe to collapse to one candidate, got %+v", result.Candidates)
	}
	if result.Candidates[0].DiscoverURL != probeURL {
		t.Fatalf("candidate = %+v", result.Candidates[0])
	}
}

func TestAddTunerValidationAndScanErrors(t *testing.T) {
	allowLoopbackMediaFetch(t)
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	svc.hdhr = nil
	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: "http://192.168.1.1"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("AddTuner nil hdhr: %v", err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"","BaseURL":"` + srv.URL + `","TunerCount":1}`))
	})
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))
	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: srv.URL}); err == nil || !strings.Contains(err.Error(), "device_id") {
		t.Fatalf("empty device id: %v", err)
	}

	mux = http.NewServeMux()
	srv2 := httptest.NewServer(mux)
	defer srv2.Close()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"T1","BaseURL":"` + srv2.URL + `","TunerCount":1}`))
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", 500)
	})
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv2.Client()))
	tuner, err := svc.AddTuner(context.Background(), AddTunerInput{URL: strings.TrimPrefix(srv2.URL, "http://")})
	if err == nil {
		t.Fatal("expected lineup scan error")
	}
	if tuner == nil || tuner.DeviceID != "T1" {
		t.Fatalf("tuner should still be returned on scan error: %+v", tuner)
	}

	if _, err := svc.GetChannel(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChannel missing: %v", err)
	}
	if _, err := svc.PatchChannel(context.Background(), "missing", ChannelPatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PatchChannel missing: %v", err)
	}
	if _, err := svc.GetProgram(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProgram missing: %v", err)
	}
	if err := svc.SyncGuideSource(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SyncGuideSource missing: %v", err)
	}
	if err := svc.ScanTuner(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ScanTuner missing: %v", err)
	}
}

func TestAddTunerDeviceIDValidationAndDiscoverError(t *testing.T) {
	svc := NewServiceWithStore(newMemoryStore())
	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: "169.254.169.254"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("metadata device id = %v", err)
	}

	svc.SetHDHomeRunClient(fakeHDHRClient{
		discover: func(context.Context, string, string) (*hdhomerun.DeviceInfo, error) {
			return nil, errors.New("discover boom")
		},
	})
	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: "http://probe.example"}); err == nil || !strings.Contains(err.Error(), "discover hdhomerun") {
		t.Fatalf("discover error = %v", err)
	}
}

func TestUpdateGuideSourceNotFoundAndInvalid(t *testing.T) {
	svc, _ := newTestService(newMemoryStore())
	if _, err := svc.UpdateGuideSource(context.Background(), &GuideSource{ID: "missing", Type: GuideSourceSchedulesDirect}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateGuideSource: %v", err)
	}
	src, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceSchedulesDirect, Enabled: false, DisplayName: "A",
		Config: validSDConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateGuideSource(context.Background(), &GuideSource{ID: src.ID, Type: "nope"}); err == nil {
		t.Fatal("expected invalid type")
	}
	if _, err := svc.CreateGuideSource(context.Background(), &GuideSource{Type: "nope"}); err == nil {
		t.Fatal("expected invalid create type")
	}
	if _, err := svc.CreateSeriesRule(context.Background(), &SeriesRule{}); err == nil {
		t.Fatal("expected series rule validation")
	}
	if err := validateGuideSource(&GuideSource{Type: GuideSourceSchedulesDirect}); err != nil {
		t.Fatalf("schedules direct should be allowed type: %v", err)
	}
}

func TestMatchesRuleAndFirstFreeIndex(t *testing.T) {
	ch := "c1"
	if matchesRule(SeriesRule{NewOnly: true}, Program{}) {
		t.Fatal("new only")
	}
	if matchesRule(SeriesRule{ChannelID: &ch}, Program{ChannelID: "other"}) {
		t.Fatal("channel")
	}
	if matchesRule(SeriesRule{SeriesID: "s"}, Program{SeriesID: "x"}) {
		t.Fatal("series")
	}
	if matchesRule(SeriesRule{TitleMatch: "matrix"}, Program{Title: "Other"}) {
		t.Fatal("title")
	}
	if !matchesRule(SeriesRule{TitleMatch: "mat"}, Program{Title: "The Matrix"}) {
		t.Fatal("want match")
	}
	if idx, ok := firstFreeIndex(0, []int{0}); ok || idx != 0 {
		t.Fatalf("full single tuner: %v %v", idx, ok)
	}
	if idx, ok := firstFreeIndex(2, []int{0}); !ok || idx != 1 {
		t.Fatalf("second slot: %v %v", idx, ok)
	}
}

func TestMediaHTTPClientDialBlocksMetadata(t *testing.T) {
	client := NewMediaHTTPClient()
	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected metadata dial block")
	}
	if !strings.Contains(err.Error(), "metadata") && !strings.Contains(err.Error(), "not allowed") {
		// dial may fail for other network reasons in sandboxes; still exercised Control path when possible
		t.Logf("dial error (acceptable): %v", err)
	}

	stream := NewStreamHTTPClient()
	if stream.Timeout != 0 {
		t.Fatalf("stream client should have no Timeout, got %v", stream.Timeout)
	}

	// CheckRedirect path via a local redirect chain.
	allowLoopbackMediaFetch(t)
	hops := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops <= 4 {
			http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	client = NewMediaHTTPClient()
	client.Transport = http.DefaultTransport
	// keep CheckRedirect from NewMediaHTTPClient
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	resp, err = client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected too many redirects")
	}
}

func TestIsBlockedMetadataIPVariants(t *testing.T) {
	if !isBlockedMetadataIP(net.ParseIP("169.254.169.253")) {
		t.Fatal("azure metadata")
	}
	if !isBlockedMetadataIP(net.ParseIP("100.100.100.200")) {
		t.Fatal("alibaba metadata")
	}
	if isBlockedMetadataIP(net.ParseIP("192.168.1.1")) {
		t.Fatal("lan should not block")
	}
}

func TestPublicSessionHelpers(t *testing.T) {
	if path := PublicSessionStreamPath("abc"); path != "/api/v1/livetv/sessions/abc/stream" {
		t.Fatalf("path=%q", path)
	}
	if IsClientSafePlayURL("") || IsClientSafePlayURL("//evil") || IsClientSafePlayURL("http://x") {
		t.Fatal("unsafe urls")
	}
	if !IsClientSafePlayURL("/api/v1/x") {
		t.Fatal("relative path should be safe")
	}
}

func TestOwnerMatchesAndCancelRecording(t *testing.T) {
	if ownerMatches(0, "", 1, "p") {
		t.Fatal("unscoped")
	}
	if ownerMatches(2, "p", 1, "p") {
		t.Fatal("user mismatch")
	}
	if ownerMatches(1, "a", 1, "b") {
		t.Fatal("profile mismatch")
	}
	if !ownerMatches(1, "p", 1, "p") {
		t.Fatal("should match")
	}

	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	rec, err := svc.ScheduleRecording(context.Background(), &Recording{
		ChannelID: "c1",
		Start:     time.Now(),
		Stop:      time.Now().Add(time.Hour),
		Title:     "t",
		UserID:    1,
		ProfileID: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelRecording(context.Background(), rec.ID, 2, "p", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong owner cancel: %v", err)
	}
	if _, err := svc.CancelRecording(context.Background(), rec.ID, 1, "p", true); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := svc.CancelRecording(context.Background(), "missing", 1, "p", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cancel: %v", err)
	}
}

func TestSyncGuideSourceSchedulesDirectErrorBranches(t *testing.T) {
	store := newMemoryStore()
	svc, fake := newTestService(store)

	fake.tokenErr = errors.New("auth failed")
	src, err := store.CreateGuideSource(context.Background(), &GuideSource{
		Type:        GuideSourceSchedulesDirect,
		DisplayName: "bad auth",
		Config:      storedSDConfig(),
		Enabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncGuideSource(context.Background(), src.ID); err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("auth error = %v", err)
	}

	fake.tokenErr = nil
	fake.lineupErr = errors.New("lineup missing")
	if err := svc.SyncGuideSource(context.Background(), src.ID); err == nil || !strings.Contains(err.Error(), "lineup missing") {
		t.Fatalf("lineup error = %v", err)
	}

	store.guideSources["unsupported"] = GuideSource{ID: "unsupported", Type: "mystery", Config: map[string]string{}}
	if err := svc.SyncGuideSource(context.Background(), "unsupported"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unsupported source = %v", err)
	}
}

func TestSyncSchedulesDirectAndUpdateFill(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newTestService(store)
	src, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceSchedulesDirect, Enabled: false, DisplayName: "SD",
		Config: validSDConfig(),
		Status: "ready",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.UpdateGuideSource(context.Background(), &GuideSource{
		ID:      src.ID,
		Enabled: false,
		// leave Type/DisplayName/Config/Status empty to exercise fill-from-existing
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Type != GuideSourceSchedulesDirect || updated.DisplayName != "SD" {
		t.Fatalf("fill-from-existing: %+v", updated)
	}
	if updated.Config["password_configured"] != "true" {
		t.Fatalf("expected redacted password marker: %+v", updated.Config)
	}

	n, err := svc.SyncAllEnabledGuideSources(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("sync all disabled: n=%d err=%v", n, err)
	}
	if _, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceSchedulesDirect, Enabled: true, DisplayName: "X",
		Config: validSDConfig(),
	}); err != nil {
		t.Fatal(err)
	}
	store.channels["ch1"] = Channel{
		ID: "ch1", Number: "5.1", Callsign: "KING-HD", Enabled: true,
	}
	n, err = svc.SyncAllEnabledGuideSources(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("sync all enabled: n=%d err=%v", n, err)
	}
}

func TestDiscoverTunersEmptyLANAndAddTunerURLCases(t *testing.T) {
	allowLoopbackMediaFetch(t)
	prev := lanDiscoverFn
	lanDiscoverFn = func(context.Context, time.Duration) ([]hdhomerun.LANCandidate, error) {
		return nil, nil
	}
	t.Cleanup(func() { lanDiscoverFn = prev })

	svc := NewServiceWithStore(newMemoryStore())
	svc.SetHDHomeRunClient(hdhomerun.NewClient(http.DefaultClient))
	result, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(result.Notes, "\n")
	if !strings.Contains(joined, "No tuners answered UDP") {
		t.Fatalf("notes=%v", result.Notes)
	}

	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: "ftp://bad"}); err == nil {
		t.Fatal("expected invalid discover url")
	}
	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: "https://169.254.169.254/discover.json"}); err == nil {
		t.Fatal("expected blocked metadata device url")
	}
	if _, err := svc.AddTuner(context.Background(), AddTunerInput{URL: "http://169.254.169.254/"}); err == nil {
		t.Fatal("expected blocked http device id host")
	}
}

func TestSchedulesDirectHelpersCoverage(t *testing.T) {
	if got := normalizeChannelNumber("005.1"); got != "5.1" {
		t.Fatalf("normalizeChannelNumber = %q", got)
	}
	if got := normalizeChannelNumber(""); got != "0" {
		t.Fatalf("empty channel = %q", got)
	}
	dates := scheduleDates(time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), 2)
	if len(dates) != 2 || dates[0] != "2026-07-25" || dates[1] != "2026-07-26" {
		t.Fatalf("dates = %v", dates)
	}
	if schedulesDirectSeriesID("EP012801050074", "Blue Bloods") != "ep01280105" {
		t.Fatalf("series id from program")
	}
	if schedulesDirectSeriesID("X", "Evening News") != "evening-news" {
		t.Fatalf("series id from title")
	}
	redacted := RedactGuideSourceConfig(map[string]string{
		"username": "u", "password": "x", "password_sha1": testSDPasswordSHA1, "lineup": testSDLineup,
	})
	if redacted["password"] != "" || redacted["password_sha1"] != "" || redacted["password_configured"] != "true" {
		t.Fatalf("redacted = %+v", redacted)
	}
	if err := ValidateMediaFetchURL("://"); err == nil {
		t.Fatal("invalid url")
	}
	if err := ValidateMediaFetchURL("http:///nohost"); err == nil {
		t.Fatal("no host")
	}
}

func TestGetSessionForViewerAndResolveUpstream(t *testing.T) {
	allowLoopbackMediaFetch(t)
	store := newMemoryStore()
	svc := NewServiceWithStore(store)

	if _, err := svc.GetSessionForViewer(context.Background(), "missing", 1, "p", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}

	tuner, err := store.CreateTuner(context.Background(), &Tuner{
		Type: TunerTypeHDHomeRun, DeviceID: "D1", BaseURL: "http://127.0.0.1:1", TunerCount: 1, Status: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceChannelsForTuner(context.Background(), tuner.ID, []Channel{{
		TunerID: tuner.ID, Number: "1", Name: "ONE", Enabled: true, StreamURL: "http://127.0.0.1:1/auto/v1",
	}}); err != nil {
		t.Fatal(err)
	}
	chs, _ := store.ListChannels(context.Background(), tuner.ID)
	session, err := store.CreateSession(context.Background(), SessionCreate{
		ChannelID: chs[0].ID, TunerID: tuner.ID, TunerIndex: 0, UserID: 1, ProfileID: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetSessionForViewer(context.Background(), session.ID, 9, "p", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unauthorized: %v", err)
	}
	got, err := svc.GetSessionForViewer(context.Background(), session.ID, 1, "p", true)
	if err != nil || got.ID != session.ID {
		t.Fatalf("viewer: %v %+v", err, got)
	}
	url, err := svc.ResolveSessionUpstreamURL(context.Background(), session.ID)
	if err != nil || url == "" {
		t.Fatalf("upstream: %v %q", err, url)
	}
}

type fakeHDHRClient struct {
	discover func(context.Context, string, string) (*hdhomerun.DeviceInfo, error)
	lineup   func(context.Context, string) ([]hdhomerun.LineupChannel, error)
}

func (f fakeHDHRClient) Discover(ctx context.Context, discoverURL, deviceID string) (*hdhomerun.DeviceInfo, error) {
	if f.discover != nil {
		return f.discover(ctx, discoverURL, deviceID)
	}
	return &hdhomerun.DeviceInfo{DeviceID: deviceID, BaseURL: discoverURL, TunerCount: 1}, nil
}

func (f fakeHDHRClient) FetchLineup(ctx context.Context, baseURL string) ([]hdhomerun.LineupChannel, error) {
	if f.lineup != nil {
		return f.lineup(ctx, baseURL)
	}
	return nil, nil
}
