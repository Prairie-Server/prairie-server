package gracenote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFlattenProvidersAndAssetURL(t *testing.T) {
	opts := FlattenProviders([]Provider{{
		Type: "OTA", Name: "Local Over the Air Broadcast", HeadendID: "lineupId",
		LineupID: "USA-lineupId-DEFAULT", Device: "",
	}})
	if len(opts) != 1 || opts[0].Device != "-" || opts[0].Headend != "lineupId" {
		t.Fatalf("FlattenProviders = %+v", opts)
	}

	c := NewClient(nil)
	if got := c.AssetURL("p123_b_v13_ae"); got != DefaultAssetsURL+"p123_b_v13_ae.jpg" {
		t.Fatalf("AssetURL = %q", got)
	}
	if got := c.AssetURL("https://cdn.example/x.jpg"); got != "https://cdn.example/x.jpg" {
		t.Fatalf("AssetURL absolute = %q", got)
	}
	if got := StationLogoURL("//zpmc.tmsimg.com/h3/x.png?w=55"); got != "https://zpmc.tmsimg.com/h3/x.png" {
		t.Fatalf("StationLogoURL = %q", got)
	}
	if headendFromLineup("USA-DISH623-DEFAULT") != "DISH623" {
		t.Fatalf("headendFromLineup dish")
	}
	if headendFromLineup("USA-lineupId-DEFAULT") != "lineupId" {
		t.Fatalf("headendFromLineup ota")
	}
}

func TestClientProvidersAndGrid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gapzap_webapi/api/Providers/getPostalCodeProviders/USA/76052/gapzap/en", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing user-agent")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Providers": []map[string]string{{
				"type": "OTA", "name": "Local Over the Air Broadcast",
				"headendId": "lineupId", "lineupId": "USA-lineupId-DEFAULT", "device": "",
			}},
		})
	})
	mux.HandleFunc("/api/grid", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("lineupId") != "USA-lineupId-DEFAULT" || q.Get("postalCode") != "76052" {
			t.Fatalf("grid query = %v", q)
		}
		if q.Get("headendId") != "lineupId" || q.Get("device") != "-" {
			t.Fatalf("grid headend/device = %q %q", q.Get("headendId"), q.Get("device"))
		}
		if q.Get("AffiliateID") != DefaultAffiliate {
			t.Fatalf("AffiliateID = %q", q.Get("AffiliateID"))
		}
		_ = json.NewEncoder(w).Encode(GridResponse{
			Channels: []Channel{{
				ChannelID: "20371", CallSign: "KDTNDT", ChannelNo: "2.1",
				Events: []Event{{
					StartTime: "2026-07-27T08:00:00Z",
					EndTime:   "2026-07-27T08:30:00Z",
					Duration:  "30",
					Thumbnail: "p8555922_b_v13_ae",
					Flag:      []string{"New"},
					Filter:    []string{"filter-News"},
					Program: Program{
						ID: "SH013903630000", Title: "Reflections", ShortDesc: "Hymns",
						SeriesID: "SH01390363",
					},
				}},
			}},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.Client())
	client.BaseURL = srv.URL

	providers, err := client.Providers(context.Background(), "USA", "76052", "")
	if err != nil || len(providers) != 1 {
		t.Fatalf("Providers = %+v %v", providers, err)
	}
	grid, err := client.Grid(context.Background(), GridParams{
		Country: "USA", PostalCode: "76052", LineupID: "USA-lineupId-DEFAULT",
	}, time.Unix(1785140903, 0).UTC(), 3)
	if err != nil || len(grid.Channels) != 1 {
		t.Fatalf("Grid = %+v %v", grid, err)
	}
	if grid.Channels[0].Events[0].Program.Title != "Reflections" {
		t.Fatalf("title = %q", grid.Channels[0].Events[0].Program.Title)
	}
}

func TestClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"No lineups"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	client := NewClient(srv.Client())
	client.BaseURL = srv.URL
	_, err := client.Providers(context.Background(), "USA", "00000", "en")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("Providers error = %v", err)
	}
	_, err = client.Grid(context.Background(), GridParams{}, time.Now(), 3)
	if err == nil {
		t.Fatal("expected grid validation error")
	}
	_, err = NewClient(nil).Providers(context.Background(), "USA", "", "en")
	if err == nil {
		t.Fatal("expected postal required")
	}
}
