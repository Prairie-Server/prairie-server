package livetv

import (
	"context"
	"errors"
	"strings"
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
		{ChannelID: "", CallSign: "SKIP", ChannelNo: "9.1"}, // empty id skipped
		{ChannelID: "30001", CallSign: "WFAA", ChannelNo: "8.1"},
		{ChannelID: "30002", CallSign: "WFAA", ChannelNo: "8.2"},
		{ChannelID: "40001", CallSign: "KXAS", ChannelNo: "5.2"},
		{ChannelID: "50001", CallSign: "KABC", ChannelNo: "7.1"},
		{ChannelID: "50002", CallSign: "KXYZ", ChannelNo: "7.3"},
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
	// Name fallback when callsign empty.
	got = resolveXMLSyncStationID(Channel{Number: "99.1", Name: "KING-DT"}, stations)
	if got != "20454" {
		t.Fatalf("name fallback = %q", got)
	}
	// Duplicate callsign disambiguated by major → unique filtered id.
	got = resolveXMLSyncStationID(Channel{Number: "8.9", Callsign: "WFAA"}, stations)
	if got != "30001" {
		t.Fatalf("major-disambiguated callsign = %q want 30001 (.1)", got)
	}
	// Single station on major with no callsign → major-only match.
	got = resolveXMLSyncStationID(Channel{Number: "5.9", Callsign: "UNKNOWN"}, []gracenote.Channel{
		{ChannelID: "solo", CallSign: "OTHER", ChannelNo: "5.3"},
	})
	if got != "solo" {
		t.Fatalf("major-only = %q", got)
	}
	// Prefer .1 among same major when callsign misses.
	got = resolveXMLSyncStationID(Channel{Number: "7.9", Callsign: "ZZZZ"}, stations)
	if got != "50001" {
		t.Fatalf(".1 preference = %q", got)
	}
	// No match.
	got = resolveXMLSyncStationID(Channel{Number: "99.9", Callsign: "ZZZZ"}, stations)
	if got != "" {
		t.Fatalf("no match = %q", got)
	}
	// Empty callsign/name skips callsign branch entirely.
	got = resolveXMLSyncStationID(Channel{Number: "99.1"}, stations)
	if got != "" {
		t.Fatalf("empty identity = %q", got)
	}
	// Multiple callsign hits on different majors → unique after major filter.
	got = resolveXMLSyncStationID(Channel{Number: "7.9", Callsign: "WFAA"}, []gracenote.Channel{
		{ChannelID: "a", CallSign: "WFAA", ChannelNo: "7.1"},
		{ChannelID: "b", CallSign: "WFAA", ChannelNo: "8.1"},
	})
	if got != "a" {
		t.Fatalf("cross-major filter = %q", got)
	}
}

func TestProgramFromGracenoteEventBranches(t *testing.T) {
	if programFromGracenoteEvent("s", "c", gracenote.Event{StartTime: "bad"}, nil) != nil {
		t.Fatal("bad start")
	}
	// EndTime missing → duration fallback.
	ev := gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z",
		Duration:  "45",
		Program:   gracenote.Program{Title: "News"},
		Filter:    []string{"filter-", "filter-Sports", ""},
		Flag:      []string{"Live", "other"},
	}
	p := programFromGracenoteEvent("s", "c", ev, nil)
	if p == nil || !p.Stop.Equal(p.Start.Add(45*time.Minute)) || !p.IsLive {
		t.Fatalf("duration/live = %+v", p)
	}
	if len(p.Genres) != 1 || p.Genres[0] != "Sports" {
		t.Fatalf("genres = %+v", p.Genres)
	}
	// Bad end + bad duration → nil.
	if programFromGracenoteEvent("s", "c", gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z", EndTime: "nope", Duration: "x",
		Program: gracenote.Program{Title: "T"},
	}, nil) != nil {
		t.Fatal("expected nil for unusable end")
	}
	// stop <= start → nil.
	if programFromGracenoteEvent("s", "c", gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z", EndTime: "2026-07-27T08:00:00Z",
		Program: gracenote.Program{Title: "T"},
	}, nil) != nil {
		t.Fatal("expected nil for non-positive window")
	}
	// Title falls back to program id; empty both → nil.
	p = programFromGracenoteEvent("s", "c", gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z", EndTime: "2026-07-27T09:00:00Z",
		Program: gracenote.Program{ID: "SH1"},
	}, nil)
	if p == nil || p.Title != "SH1" {
		t.Fatalf("title from id = %+v", p)
	}
	if programFromGracenoteEvent("s", "c", gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z", EndTime: "2026-07-27T09:00:00Z",
	}, nil) != nil {
		t.Fatal("expected nil without title/id")
	}
	// Empty external id uses title; series id derived; asset URL applied.
	p = programFromGracenoteEvent("s", "c", gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z", EndTime: "2026-07-27T09:00:00Z",
		Thumbnail: "thumb",
		Program:   gracenote.Program{Title: "Solo"},
	}, func(th string) string { return "https://img/" + th })
	if p == nil || p.ImageURL != "https://img/thumb" || p.SeriesID == "" || !strings.Contains(p.ExternalID, "Solo:") {
		t.Fatalf("derived fields = %+v", p)
	}
	// Explicit series id is lowercased.
	p = programFromGracenoteEvent("s", "c", gracenote.Event{
		StartTime: "2026-07-27T08:00:00Z", EndTime: "2026-07-27T09:00:00Z",
		Program: gracenote.Program{Title: "T", SeriesID: "SHABC", ID: "ep1"},
	}, nil)
	if p == nil || p.SeriesID != "shabc" {
		t.Fatalf("series id = %+v", p)
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

func TestSyncXMLSyncErrorBranches(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	src, err := store.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLSync, Enabled: true, DisplayName: "XML",
		Config: map[string]string{
			"postalcode": "76052", "lineup": "USA-x-DEFAULT", "country": "USA",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.gn = nil
	if err := svc.SyncGuideSource(context.Background(), src.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil gn = %v", err)
	}

	svc.SetGracenoteClient(newFakeGN())
	// Channel that cannot map to any Gracenote station.
	store.channels["orphan"] = Channel{
		ID: "orphan", Number: "99.9", Callsign: "ZZZZ", Enabled: true, StreamURL: "http://x",
	}
	if err := svc.SyncGuideSource(context.Background(), src.ID); err == nil || !strings.Contains(err.Error(), "no channels mapped") {
		t.Fatalf("unmapable channels = %v", err)
	}

	badCfg, err := store.CreateGuideSource(context.Background(), &GuideSource{
		Type: GuideSourceXMLSync, Enabled: false, DisplayName: "bad",
		Config: map[string]string{"country": "USA"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncGuideSource(context.Background(), badCfg.ID); err == nil || !strings.Contains(err.Error(), "postalcode") {
		t.Fatalf("missing postal/lineup = %v", err)
	}
}
