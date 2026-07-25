package hdhomerun

import (
	"context"
	"net/http"
	"net/http/httptest"
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
}
