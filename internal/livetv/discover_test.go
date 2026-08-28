package livetv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv/hdhomerun"
)

func TestDiscoverTunersProbeDispatcharr(t *testing.T) {
	allowLoopbackMediaFetch(t)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/hdhr/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"FriendlyName":"Dispatcharr",
			"ModelNumber":"HDHR-DISPATCHARR",
			"FirmwareVersion":"1.0",
			"TunerCount":3,
			"DeviceID":"D15A7C01",
			"BaseURL":"` + srv.URL + `/hdhr"
		}`))
	})

	prev := lanDiscoverFn
	lanDiscoverFn = func(context.Context, time.Duration) ([]hdhomerun.LANCandidate, error) {
		return nil, nil
	}
	t.Cleanup(func() { lanDiscoverFn = prev })

	svc := NewServiceWithStore(newMemoryStore())
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))
	includeUDP := false
	result, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{
		IncludeUDP: &includeUDP,
		ProbeURLs:  []string{srv.URL},
	})
	if err != nil {
		t.Fatalf("DiscoverTuners: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates=%+v", result.Candidates)
	}
	c := result.Candidates[0]
	if c.Kind != DiscoveredKindDispatcharr || c.DeviceID != "D15A7C01" || c.Source != "probe" {
		t.Fatalf("unexpected candidate: %+v", c)
	}
	if c.DiscoverURL != srv.URL+"/hdhr/discover.json" {
		t.Fatalf("discover_url=%q", c.DiscoverURL)
	}
}

func TestDiscoverTunersUDPThenVerify(t *testing.T) {
	allowLoopbackMediaFetch(t)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"FriendlyName":"HDHomeRun FLEX",
			"ModelNumber":"HDHR5-4K",
			"FirmwareVersion":"2026",
			"TunerCount":4,
			"DeviceID":"ABCDEF01",
			"BaseURL":"` + srv.URL + `"
		}`))
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"GuideNumber":"7.1","GuideName":"KABC","URL":"` + srv.URL + `/auto/v7.1","HD":true}]`))
	})

	prev := lanDiscoverFn
	lanDiscoverFn = func(context.Context, time.Duration) ([]hdhomerun.LANCandidate, error) {
		return []hdhomerun.LANCandidate{{
			DeviceIDHex: "ABCDEF01",
			TunerCount:  4,
			BaseURL:     srv.URL,
			RemoteIP:    "127.0.0.1",
		}}, nil
	}
	t.Cleanup(func() { lanDiscoverFn = prev })

	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))

	result, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{})
	if err != nil {
		t.Fatalf("DiscoverTuners: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Kind != DiscoveredKindHDHomeRun {
		t.Fatalf("candidates=%+v", result.Candidates)
	}
	if result.Candidates[0].AlreadyAdded {
		t.Fatal("expected not already added")
	}

	_, err = svc.AddTuner(context.Background(), AddTunerInput{URL: result.Candidates[0].BaseURL})
	if err != nil {
		t.Fatalf("AddTuner: %v", err)
	}
	result2, err := svc.DiscoverTuners(context.Background(), DiscoverTunersRequest{})
	if err != nil {
		t.Fatalf("DiscoverTuners again: %v", err)
	}
	if len(result2.Candidates) != 1 || !result2.Candidates[0].AlreadyAdded {
		t.Fatalf("expected already_added: %+v", result2.Candidates)
	}
}
