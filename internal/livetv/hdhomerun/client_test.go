package hdhomerun

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverAndFetchLineup(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"FriendlyName":"HDHomeRun FLEX 4K",
			"ModelNumber":"HDHR5-4K",
			"FirmwareVersion":"20260201",
			"TunerCount":4,
			"DeviceID":"12345678",
			"BaseURL":"` + srv.URL + `"
		}`))
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"GuideNumber":"7.1","GuideName":"KABC-HD","URL":"` + srv.URL + `/auto/v7.1","HD":true,"Favorite":true},
			{"GuideNumber":"9.1","GuideName":"KCAL","URL":"` + srv.URL + `/auto/v9.1","HD":false}
		]`))
	})

	client := NewClient(srv.Client())
	info, err := client.Discover(context.Background(), srv.URL+"/discover.json", "")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if info.DeviceID != "12345678" || info.ModelNumber != "HDHR5-4K" || info.TunerCount != 4 || info.BaseURL != srv.URL {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.LineupURL != srv.URL+"/lineup.json" {
		t.Fatalf("LineupURL = %q", info.LineupURL)
	}

	lineup, err := client.FetchLineup(context.Background(), info.BaseURL)
	if err != nil {
		t.Fatalf("FetchLineup() error = %v", err)
	}
	if len(lineup) != 2 {
		t.Fatalf("len(lineup) = %d, want 2", len(lineup))
	}
	if lineup[0].GuideNumber != "7.1" || lineup[0].GuideName != "KABC-HD" || !lineup[0].HD || !lineup[0].Favorite {
		t.Fatalf("unexpected first channel: %+v", lineup[0])
	}
}

func TestDiscoverAcceptsLineupJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"GuideNumber":"2.1","GuideName":"WGBH","URL":"http://example/auto/v2.1"}]`))
	}))
	defer srv.Close()

	info, err := NewClient(srv.Client()).Discover(context.Background(), srv.URL, "dev-1")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if info.DeviceID != "dev-1" || info.BaseURL != srv.URL || info.TunerCount != 1 {
		t.Fatalf("unexpected lineup discover info: %+v", info)
	}

	info, err = NewClient(srv.Client()).Discover(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("Discover lineup without device id: %v", err)
	}
	if info.DeviceID == "" || info.DeviceID != hostWithoutPort(srv.URL) {
		t.Fatalf("lineup fallback device id = %+v", info)
	}
}

func TestDiscoverFromDeviceID(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"ABCD","BaseURL":"` + srv.URL + `","TunerCount":2}`))
	})

	host := strings.TrimPrefix(srv.URL, "http://")
	info, err := NewClient(srv.Client()).Discover(context.Background(), "", host)
	if err != nil {
		t.Fatalf("Discover by host device_id: %v", err)
	}
	if info.DeviceID != "ABCD" {
		t.Fatalf("DeviceID = %q", info.DeviceID)
	}

	info, err = NewClient(srv.Client()).Discover(context.Background(), "", srv.URL)
	if err != nil {
		t.Fatalf("Discover by http device_id: %v", err)
	}
	if info.BaseURL != srv.URL {
		t.Fatalf("BaseURL = %q", info.BaseURL)
	}
}

func TestDiscoverErrors(t *testing.T) {
	client := NewClient(nil)
	_, err := client.Discover(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected missing discover args error")
	}
	if _, _, err := client.get(context.Background(), "://"); err == nil {
		t.Fatal("expected malformed request error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err = NewClient(srv.Client()).Discover(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatal("expected status error")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`"just a string"`))
	}))
	defer bad.Close()
	_, err = NewClient(bad.Client()).Discover(context.Background(), bad.URL, "")
	if err == nil {
		t.Fatal("expected unsupported response")
	}

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{`))
	}))
	defer malformed.Close()
	if _, _, err := NewClient(malformed.Client()).get(context.Background(), malformed.URL); err == nil {
		t.Fatal("expected JSON decode error")
	}
}

func TestFetchLineupErrors(t *testing.T) {
	client := NewClient(http.DefaultClient)
	_, err := client.FetchLineup(context.Background(), "")
	if err == nil {
		t.Fatal("expected empty base url error")
	}
	_, err = NewClient(&http.Client{Transport: errorTransport{}}).FetchLineup(context.Background(), "http://example.test")
	if err == nil {
		t.Fatal("expected transport error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not":"array"}`))
	}))
	defer srv.Close()
	_, err = NewClient(srv.Client()).FetchLineup(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestNormalizeHelpers(t *testing.T) {
	info := normalizeDeviceInfo(discoverJSON{
		FriendlyName: "  Flex  ",
		TunerCount:   3,
	}, "http://192.168.1.50:80/discover.json", "fallback")
	if info.DeviceID != "fallback" || info.BaseURL != "http://192.168.1.50:80" {
		t.Fatalf("normalize = %+v", info)
	}
	if info.LineupURL != "http://192.168.1.50:80/lineup.json" {
		t.Fatalf("LineupURL = %q", info.LineupURL)
	}
	if origin("not a url") != "" {
		t.Fatal("origin should be empty")
	}
	if hostWithoutPort("http://example.com:5004/x") != "example.com" {
		t.Fatalf("hostWithoutPort unexpected")
	}
	if hostWithoutPort("not a url") != "" {
		t.Fatal("hostWithoutPort should be empty")
	}
}

type errorTransport struct{}

func (errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport boom")
}
