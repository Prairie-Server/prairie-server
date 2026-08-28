package livetv

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

func TestSchedulesDirectCoverageBranches(t *testing.T) {
	t.Run("list lineups validation and errors", func(t *testing.T) {
		var nilSvc *Service
		if _, err := nilSvc.ListSchedulesDirectLineups(context.Background(), SchedulesDirectLineupsRequest{}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("nil service: %v", err)
		}
		svc, fake := newTestService(newMemoryStore())
		svc.SetSchedulesDirectClient(nil)
		if _, err := svc.ListSchedulesDirectLineups(context.Background(), SchedulesDirectLineupsRequest{
			Username: "u", Password: "p", PostalCode: "76052",
		}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("nil sd: %v", err)
		}
		svc.SetSchedulesDirectClient(fake)
		if _, err := svc.ListSchedulesDirectLineups(context.Background(), SchedulesDirectLineupsRequest{}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("missing fields: %v", err)
		}
		fake.tokenErr = errors.New("token boom")
		if _, err := svc.ListSchedulesDirectLineups(context.Background(), SchedulesDirectLineupsRequest{
			Username: "u", PasswordSHA1: testSDPasswordSHA1, PostalCode: "76052",
		}); err == nil || !strings.Contains(err.Error(), "token boom") {
			t.Fatalf("token err: %v", err)
		}
		fake.tokenErr = nil
		fake.headendsErr = errors.New("headends boom")
		if _, err := svc.ListSchedulesDirectLineups(context.Background(), SchedulesDirectLineupsRequest{
			Username: "u", Password: testSDPassword, PostalCode: "76052",
		}); err == nil || !strings.Contains(err.Error(), "headends boom") {
			t.Fatalf("headends err: %v", err)
		}
		fake.headendsErr = nil
		lineups, err := svc.ListSchedulesDirectLineups(context.Background(), SchedulesDirectLineupsRequest{
			Username: "u", Password: testSDPassword, PostalCode: "76052",
		})
		if err != nil || len(lineups) != 1 {
			t.Fatalf("default country: %v %+v", err, lineups)
		}
	})

	t.Run("prepare config edges", func(t *testing.T) {
		svc, fake := newTestService(newMemoryStore())
		src := &GuideSource{Config: nil}
		if err := svc.prepareGuideSourceConfig(context.Background(), src, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("missing fields with nil config: %v", err)
		}
		if src.Type != GuideSourceSchedulesDirect {
			t.Fatalf("type default = %q", src.Type)
		}
		badHash := validSDConfig()
		badHash["password"] = ""
		badHash["password_sha1"] = "short"
		if err := svc.prepareGuideSourceConfig(context.Background(), &GuideSource{Config: badHash}, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("short hash: %v", err)
		}
		fake.tokenErr = errors.New("prep token")
		if err := svc.prepareGuideSourceConfig(context.Background(), &GuideSource{Config: validSDConfig()}, nil); err == nil || !strings.Contains(err.Error(), "prep token") {
			t.Fatalf("prep token: %v", err)
		}
		fake.tokenErr = nil
		fake.addErr = errors.New("prep add")
		if err := svc.prepareGuideSourceConfig(context.Background(), &GuideSource{Config: validSDConfig()}, nil); err == nil || !strings.Contains(err.Error(), "prep add") {
			t.Fatalf("prep add: %v", err)
		}
		fake.addErr = nil
		cfg := validSDConfig()
		cfg["days"] = "10"
		src = &GuideSource{Config: cfg}
		if err := svc.prepareGuideSourceConfig(context.Background(), src, nil); err != nil {
			t.Fatal(err)
		}
		if src.Config["days"] != "10" || src.DisplayName != "Schedules Direct" {
			t.Fatalf("prepared = %+v", src)
		}
		// unchanged credentials skip online verify
		calls := fake.tokenCalls
		existing := &GuideSource{Config: storedSDConfig()}
		if err := svc.prepareGuideSourceConfig(context.Background(), &GuideSource{
			Type: GuideSourceSchedulesDirect, Config: map[string]string{"lineup": testSDLineup},
		}, existing); err != nil {
			t.Fatal(err)
		}
		if fake.tokenCalls != calls {
			t.Fatalf("expected no token call on unchanged creds, got %d->%d", calls, fake.tokenCalls)
		}
	})

	t.Run("sync mapping and schedule edges", func(t *testing.T) {
		store := newMemoryStore()
		override := "5.1"
		store.channels["ch1"] = Channel{
			ID: "ch1", Number: "99", NumberOverride: &override, Callsign: "OTHER", Enabled: true,
		}
		store.channels["ch2"] = Channel{
			ID: "ch2", Number: "7.1", Callsign: "KING", GuideStationID: "", Enabled: true,
		}
		store.channels["ch3"] = Channel{
			ID: "ch3", Number: "8.1", Callsign: "", GuideStationID: "99999", Enabled: true,
		}
		svc, fake := newTestService(store)
		fake.lineup = &schedulesdirect.LineupDetail{
			Map: []schedulesdirect.LineupMapEntry{
				{Channel: "5.1", StationID: "111"},
			},
			Stations: []schedulesdirect.Station{
				{StationID: "222", Callsign: "KING-DT"},
				{StationID: "99999", Callsign: "Z"},
			},
		}
		live := true
		fake.schedules = []schedulesdirect.StationSchedule{
			{StationID: "111", Code: 1, Message: "skip"},
			{
				StationID: "111",
				Programs: []schedulesdirect.ScheduleProgram{
					{ProgramID: "", AirDateTime: "2026-07-25T10:00:00Z", Duration: 60},
					{ProgramID: "EP000000990099", AirDateTime: "bad-time", Duration: 60},
					{ProgramID: "EP000000990099", AirDateTime: "2026-07-25T11:00:00Z", Duration: 0},
					{ProgramID: "EP000000990099", AirDateTime: "2026-07-25T12:00:00Z", Duration: 1800, Live: &live},
					{ProgramID: "EP000000990099", AirDateTime: "2026-07-25T13:00:00Z", Duration: 1800},
				},
			},
			{
				StationID: "222",
				Programs: []schedulesdirect.ScheduleProgram{
					{ProgramID: "SH000000010000", AirDateTime: "2026-07-25T14:00:00Z", Duration: 1800},
				},
			},
			{StationID: "orphan", Programs: []schedulesdirect.ScheduleProgram{
				{ProgramID: "EP1", AirDateTime: "2026-07-25T15:00:00Z", Duration: 60},
			}},
		}
		fake.programs = []schedulesdirect.ProgramDetail{
			{ProgramID: "SH000000010000", Titles: []schedulesdirect.Title{{Title120: "Movie"}}},
		}
		svc.now = func() time.Time { return time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC) }
		source, err := store.CreateGuideSource(context.Background(), &GuideSource{
			Type: GuideSourceSchedulesDirect, Enabled: true, DisplayName: "SD",
			Config: map[string]string{
				"username": "tester", "password_sha1": testSDPasswordSHA1,
				"lineup": testSDLineup, "days": "3",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.SyncGuideSource(context.Background(), source.ID); err != nil {
			t.Fatalf("sync: %v", err)
		}
		ch1, _ := store.GetChannel(context.Background(), "ch1")
		ch2, _ := store.GetChannel(context.Background(), "ch2")
		if ch1.GuideStationID != "111" || ch2.GuideStationID != "222" {
			t.Fatalf("mapping ch1=%q ch2=%q", ch1.GuideStationID, ch2.GuideStationID)
		}
		programs, err := svc.ListGuide(context.Background(), []string{"ch1", "ch2"},
			time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(programs) < 2 {
			t.Fatalf("programs = %+v", programs)
		}
		var sawLive, sawFallbackTitle, sawEmptyGenres bool
		for _, p := range programs {
			if p.IsLive {
				sawLive = true
			}
			if p.Title == "EP000000990099" {
				sawFallbackTitle = true
			}
			if p.Title == "Movie" {
				if p.Genres == nil {
					t.Fatalf("Movie genres must be non-nil empty slice, got nil")
				}
				sawEmptyGenres = true
			}
		}
		if !sawLive || !sawFallbackTitle || !sawEmptyGenres {
			t.Fatalf("expected live + fallback title + empty genres in %+v", programs)
		}
	})

	t.Run("sync error branches", func(t *testing.T) {
		svc, fake := newTestService(newMemoryStore())
		svc.SetSchedulesDirectClient(nil)
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{Config: storedSDConfig()}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("nil sd sync: %v", err)
		}
		svc.SetSchedulesDirectClient(fake)
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{Config: map[string]string{}}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("missing creds: %v", err)
		}
		fake.addErr = errors.New("add boom")
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{Config: storedSDConfig()}); err == nil || !strings.Contains(err.Error(), "add boom") {
			t.Fatalf("add: %v", err)
		}
		fake.addErr = nil
		fake.tokenErr = schedulesdirect.APIError{Code: 4003, Message: "Invalid username or password.", Response: "INVALID_USER"}
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{Config: storedSDConfig()}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("token api error: %v", err)
		}
		if err := wrapSchedulesDirectErr(nil); err != nil {
			t.Fatalf("wrap nil: %v", err)
		}
		if err := wrapSchedulesDirectErr(errors.New("network")); errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("wrap network should stay opaque: %v", err)
		}
		fake.tokenErr = nil
		fake.schedErr = errors.New("sched boom")
		store := newMemoryStore()
		store.channels["ch1"] = Channel{ID: "ch1", Number: "5.1", Callsign: "KING-HD", Enabled: true}
		svc, fake = newTestService(store)
		fake.schedErr = errors.New("sched boom")
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{ID: "s", Config: storedSDConfig()}); err == nil || !strings.Contains(err.Error(), "sched boom") {
			t.Fatalf("sched: %v", err)
		}
		fake.schedErr = nil
		fake.programsErr = errors.New("programs boom")
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{ID: "s", Config: storedSDConfig()}); err == nil || !strings.Contains(err.Error(), "programs boom") {
			t.Fatalf("programs: %v", err)
		}
		fake.programsErr = nil
		fake.schedules = []schedulesdirect.StationSchedule{{StationID: "20454", Programs: nil}}
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{ID: "s", Config: storedSDConfig()}); err != nil {
			t.Fatalf("empty program ids: %v", err)
		}
		empty := newMemoryStore()
		svc, _ = newTestService(empty)
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{ID: "s", Config: storedSDConfig()}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("no mapped channels: %v", err)
		}
	})

	t.Run("map update channel error and helpers", func(t *testing.T) {
		errStore := &erroringStore{memoryStore: newMemoryStore(), updateChannelErr: true}
		errStore.channels["ch1"] = Channel{ID: "ch1", Number: "5.1", Callsign: "KING-HD", Enabled: true}
		svc, _ := newTestService(errStore)
		if err := svc.syncSchedulesDirect(context.Background(), &GuideSource{ID: "s", Config: storedSDConfig()}); !errors.Is(err, errStoreBoom) {
			t.Fatalf("update channel: %v", err)
		}
		if RedactGuideSourceConfig(nil) == nil {
			t.Fatal("nil redact")
		}
		if scheduleDates(time.Now().UTC(), 0)[0] == "" {
			t.Fatal("default days")
		}
		if len(uniqueStrings([]string{"", "a", "a", "b"})) != 2 {
			t.Fatal("uniqueStrings")
		}
		override := " 12.3 "
		if channelDisplayNumber(Channel{Number: "1", NumberOverride: &override}) != " 12.3 " {
			t.Fatal("override number")
		}
	})

	t.Run("create update store error paths", func(t *testing.T) {
		createErr := &erroringStore{memoryStore: newMemoryStore(), createGuideErr: true}
		svc, _ := newTestService(createErr)
		if _, err := svc.CreateGuideSource(context.Background(), &GuideSource{
			Type: GuideSourceSchedulesDirect, Config: validSDConfig(),
		}); !errors.Is(err, errStoreBoom) {
			t.Fatalf("create store: %v", err)
		}
		listErr := &erroringStore{memoryStore: newMemoryStore(), listGuideErr: true}
		svc, _ = newTestService(listErr)
		if _, err := svc.ListGuideSources(context.Background()); !errors.Is(err, errStoreBoom) {
			t.Fatalf("list store: %v", err)
		}
		getErr := &erroringStore{memoryStore: newMemoryStore(), getGuideErr: true}
		svc, _ = newTestService(getErr)
		if _, err := svc.UpdateGuideSource(context.Background(), &GuideSource{ID: "x"}); !errors.Is(err, errStoreBoom) {
			t.Fatalf("update get: %v", err)
		}
		updateErr := &erroringStore{memoryStore: newMemoryStore(), updateGuideErr: true}
		src, err := updateErr.CreateGuideSource(context.Background(), &GuideSource{
			Type: GuideSourceSchedulesDirect, Config: storedSDConfig(), DisplayName: "SD",
		})
		if err != nil {
			t.Fatal(err)
		}
		svc, _ = newTestService(updateErr)
		if _, err := svc.UpdateGuideSource(context.Background(), &GuideSource{
			ID: src.ID, Enabled: false, Config: storedSDConfig(),
		}); !errors.Is(err, errStoreBoom) {
			t.Fatalf("update store: %v", err)
		}
		delErr := &erroringStore{memoryStore: newMemoryStore(), deleteGuideErr: true}
		svc, _ = newTestService(delErr)
		if err := svc.DeleteGuideSource(context.Background(), "x"); !errors.Is(err, errStoreBoom) {
			t.Fatalf("delete store: %v", err)
		}
		syncListErr := &erroringStore{memoryStore: newMemoryStore(), listGuideErr: true}
		svc, _ = newTestService(syncListErr)
		if _, err := svc.SyncAllEnabledGuideSources(context.Background()); !errors.Is(err, errStoreBoom) {
			t.Fatalf("sync all list: %v", err)
		}
	})
}
