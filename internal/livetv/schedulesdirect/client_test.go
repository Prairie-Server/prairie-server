package schedulesdirect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	got := HashPassword("secret")
	if got != "e5e9fa1ba31ecd1ae84f75caaa474f3a663f05f4" {
		t.Fatalf("HashPassword = %q", got)
	}
}

func TestClientTokenHeadendsLineupSchedulesPrograms(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/20141201/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing user-agent")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "token": "abc"})
	})
	mux.HandleFunc("/20141201/headends", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("token") != "abc" {
			t.Fatalf("token header = %q", r.Header.Get("token"))
		}
		if r.URL.Query().Get("postalcode") != "76052" {
			t.Fatalf("postalcode = %q", r.URL.Query().Get("postalcode"))
		}
		_ = json.NewEncoder(w).Encode([]Headend{{
			Headend: "76052", Transport: "Antenna", Location: "76052",
			Lineups: []HeadendLineup{{Name: "Antenna", Lineup: "USA-OTA-76052"}},
		}})
	})
	mux.HandleFunc("/20141201/lineups/USA-OTA-76052", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "Added lineup."})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(LineupDetail{
				Map:      []LineupMapEntry{{Channel: "5.1", StationID: "20454"}},
				Stations: []Station{{StationID: "20454", Callsign: "KING"}},
			})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/20141201/schedules", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]StationSchedule{{
			StationID: "20454",
			Programs: []ScheduleProgram{{
				ProgramID: "EP1", AirDateTime: "2026-07-25T19:00:00Z", Duration: 1800,
			}},
		}})
	})
	mux.HandleFunc("/20141201/programs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ProgramDetail{{
			ProgramID: "EP1",
			Titles:    []Title{{Title120: "News"}},
		}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.Client())
	client.BaseURL = srv.URL + "/20141201"

	token, err := client.Token(context.Background(), "u", HashPassword("secret"))
	if err != nil || token != "abc" {
		t.Fatalf("Token = %q %v", token, err)
	}
	heads, err := client.Headends(context.Background(), token, "USA", "76052")
	if err != nil || len(heads) != 1 {
		t.Fatalf("Headends = %+v %v", heads, err)
	}
	opts := FlattenLineups(heads)
	if len(opts) != 1 || opts[0].Lineup != "USA-OTA-76052" {
		t.Fatalf("FlattenLineups = %+v", opts)
	}
	if err := client.AddLineup(context.Background(), token, "USA-OTA-76052"); err != nil {
		t.Fatalf("AddLineup: %v", err)
	}
	detail, err := client.Lineup(context.Background(), token, "USA-OTA-76052")
	if err != nil || len(detail.Map) != 1 {
		t.Fatalf("Lineup = %+v %v", detail, err)
	}
	sched, err := client.Schedules(context.Background(), token, []ScheduleRequest{{StationID: "20454"}})
	if err != nil || len(sched) != 1 {
		t.Fatalf("Schedules = %+v %v", sched, err)
	}
	progs, err := client.Programs(context.Background(), token, []string{"EP1"})
	if err != nil || progs[0].Title() != "News" {
		t.Fatalf("Programs = %+v %v", progs, err)
	}
}

func TestClientTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":4003,"message":"Invalid username or password.","response":"INVALID_USER"}`))
	}))
	defer srv.Close()
	client := NewClient(srv.Client())
	client.BaseURL = srv.URL
	_, err := client.Token(context.Background(), "u", "bad")
	if err == nil || !strings.Contains(err.Error(), "Invalid username") {
		t.Fatalf("Token error = %v", err)
	}
}

func TestProgramDetailHelpers(t *testing.T) {
	p := ProgramDetail{
		Titles: []Title{{Title120: "Show"}},
		Descriptions: DescBlock{
			Description100: []LangText{{Description: "Short"}},
		},
		Metadata: []map[string]struct {
			Season  int `json:"season"`
			Episode int `json:"episode"`
		}{{
			"Gracenote": {Season: 2, Episode: 4},
		}},
	}
	if p.Title() != "Show" || p.Description() != "Short" {
		t.Fatalf("title/desc = %q %q", p.Title(), p.Description())
	}
	season, episode := p.SeasonEpisode()
	if season == nil || episode == nil || *season != 2 || *episode != 4 {
		t.Fatalf("season/episode = %v %v", season, episode)
	}
}
