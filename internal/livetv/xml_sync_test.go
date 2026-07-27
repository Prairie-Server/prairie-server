package livetv

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv/gracenote"
)

type fakeGN struct {
	mu sync.Mutex

	providersErr error
	gridErr      error
	providers    []gracenote.Provider
	grid         *gracenote.GridResponse
	gridCalls    int
}

func newFakeGN() *fakeGN {
	season := 1
	episode := 2
	return &fakeGN{
		providers: []gracenote.Provider{{
			Type: "OTA", Name: "Local Over the Air Broadcast",
			HeadendID: "lineupId", LineupID: "USA-lineupId-DEFAULT", Device: "",
		}},
		grid: &gracenote.GridResponse{
			Channels: []gracenote.Channel{{
				ChannelID: "20371", CallSign: "KDTNDT", ChannelNo: "2.1",
				Events: []gracenote.Event{{
					StartTime: "2026-07-27T08:00:00Z",
					EndTime:   "2026-07-27T08:30:00Z",
					Duration:  "30",
					Thumbnail: "p8555922_b_v13_ae",
					Flag:      []string{"New"},
					Filter:    []string{"filter-News"},
					Program: gracenote.Program{
						ID: "SH013903630000", Title: "Reflections", ShortDesc: "Hymns",
						EpisodeTitle: "Morning", SeriesID: "SH01390363",
						Season: &season, Episode: &episode,
					},
				}},
			}},
		},
	}
}

func (f *fakeGN) Providers(context.Context, string, string, string) ([]gracenote.Provider, error) {
	if f.providersErr != nil {
		return nil, f.providersErr
	}
	return f.providers, nil
}

func (f *fakeGN) Grid(context.Context, gracenote.GridParams, time.Time, int) (*gracenote.GridResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gridCalls++
	if f.gridErr != nil {
		return nil, f.gridErr
	}
	return f.grid, nil
}

func (f *fakeGN) AssetURL(thumbnail string) string {
	return gracenote.DefaultAssetsURL + thumbnail + ".jpg"
}

func TestXMLSyncStationMatch(t *testing.T) {
	stations := []gracenote.Channel{
		{ChannelID: "20371", CallSign: "KDTNDT", ChannelNo: "2.1"},
		{ChannelID: "20454", CallSign: "KINGDT", ChannelNo: "5.1"},
	}
	got := resolveXMLSyncStationID(Channel{Number: "2.1", Callsign: "KDTN-DT"}, stations)
	if got != "20371" {
		t.Fatalf("number match = %q", got)
	}
	got = resolveXMLSyncStationID(Channel{Number: "9.1", Callsign: "KING-DT"}, stations)
	if got != "20454" {
		t.Fatalf("callsign match = %q", got)
	}
	got = resolveXMLSyncStationID(Channel{Number: "2.1", GuideStationID: "manual"}, stations)
	if got != "manual" {
		t.Fatalf("override = %q", got)
	}
}

func TestSyncGuideSourceXMLSyncMapsChannels(t *testing.T) {
	store := newMemoryStore()
	store.tuners["t1"] = Tuner{ID: "t1", Type: TunerTypeHDHomeRun, DeviceID: "d1", TunerCount: 2, Status: "ready"}
	store.channels["ch1"] = Channel{
		ID: "ch1", TunerID: "t1", Number: "2.1", Callsign: "KDTN-DT", Name: "KDTN-DT",
		Enabled: true, StreamURL: "http://x/auto/v2.1",
	}

	svc := NewServiceWithStore(store)
	fake := newFakeGN()
	svc.SetGracenoteClient(fake)
	svc.now = func() time.Time { return time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC) }

	source, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLSync, Enabled: true, DisplayName: "XML",
		Config: map[string]string{
			"country": "USA", "postalcode": "76052",
			"lineup": "USA-lineupId-DEFAULT", "days": "1",
		},
	})
	if err != nil {
		t.Fatalf("CreateGuideSource: %v", err)
	}
	if source.Config["headend"] != "lineupId" || source.Config["device"] != "-" {
		t.Fatalf("config = %+v", source.Config)
	}

	if err := svc.SyncGuideSource(context.Background(), source.ID); err != nil {
		t.Fatalf("SyncGuideSource: %v", err)
	}
	ch, err := store.GetChannel(context.Background(), "ch1")
	if err != nil || ch == nil || ch.GuideStationID != "20371" {
		t.Fatalf("auto-mapped guide station = %+v err=%v", ch, err)
	}

	start := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	programs, err := svc.ListGuide(context.Background(), []string{"ch1"}, start, end)
	if err != nil {
		t.Fatalf("ListGuide: %v", err)
	}
	if len(programs) == 0 || programs[0].Title != "Reflections" {
		t.Fatalf("programs = %+v", programs)
	}
	if programs[0].ImageURL != gracenote.DefaultAssetsURL+"p8555922_b_v13_ae.jpg" || !programs[0].IsNew {
		t.Fatalf("program fields = %+v", programs[0])
	}
	if fake.gridCalls < 1 {
		t.Fatalf("gridCalls = %d", fake.gridCalls)
	}

	lineups, err := svc.ListXMLSyncLineups(context.Background(), XMLSyncLineupsRequest{
		Country: "USA", PostalCode: "76052",
	})
	if err != nil || len(lineups) != 1 || lineups[0].Lineup != "USA-lineupId-DEFAULT" {
		t.Fatalf("ListXMLSyncLineups = %+v %v", lineups, err)
	}
}

func TestCreateXMLSyncRequiresPostalAndLineup(t *testing.T) {
	svc := NewServiceWithStore(newMemoryStore())
	svc.SetGracenoteClient(newFakeGN())
	_, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLSync, Enabled: true, DisplayName: "XML",
		Config: map[string]string{"postalcode": "76052"},
	})
	if err == nil {
		t.Fatal("expected lineup required")
	}
}
