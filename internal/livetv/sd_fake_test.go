package livetv

import (
	"context"
	"sync"

	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

const (
	testSDPassword     = "secret"
	testSDPasswordSHA1 = "e5e9fa1ba31ecd1ae84f75caaa474f3a663f05f4" // sha1("secret")
	testSDLineup       = "USA-OTA-76052"
)

func validSDConfig() map[string]string {
	return map[string]string{
		"username":   "tester",
		"password":   testSDPassword,
		"country":    "USA",
		"postalcode": "76052",
		"lineup":     testSDLineup,
	}
}

func storedSDConfig() map[string]string {
	return map[string]string{
		"username":      "tester",
		"password_sha1": testSDPasswordSHA1,
		"country":       "USA",
		"postalcode":    "76052",
		"lineup":        testSDLineup,
	}
}

type fakeSD struct {
	mu sync.Mutex

	tokenErr    error
	headendsErr error
	addErr      error
	lineupErr   error
	schedErr    error
	programsErr error

	headends  []schedulesdirect.Headend
	lineup    *schedulesdirect.LineupDetail
	schedules []schedulesdirect.StationSchedule
	programs  []schedulesdirect.ProgramDetail

	tokenCalls int
	addCalls   int
}

func newFakeSD() *fakeSD {
	trueVal := true
	return &fakeSD{
		headends: []schedulesdirect.Headend{{
			Headend:   "76052",
			Transport: "Antenna",
			Location:  "76052",
			Lineups: []schedulesdirect.HeadendLineup{{
				Name:   "Antenna",
				Lineup: testSDLineup,
			}},
		}},
		lineup: &schedulesdirect.LineupDetail{
			Map: []schedulesdirect.LineupMapEntry{
				{Channel: "5.1", StationID: "20454"},
			},
			Stations: []schedulesdirect.Station{
				{StationID: "20454", Name: "KING-HD", Callsign: "KING-HD"},
			},
		},
		schedules: []schedulesdirect.StationSchedule{{
			StationID: "20454",
			Programs: []schedulesdirect.ScheduleProgram{{
				ProgramID:   "EP000000010001",
				AirDateTime: "2026-07-25T19:00:00Z",
				Duration:    3600,
				New:         &trueVal,
			}},
		}},
		programs: []schedulesdirect.ProgramDetail{{
			ProgramID:       "EP000000010001",
			Titles:          []schedulesdirect.Title{{Title120: "Evening News"}},
			EpisodeTitle150: "Weekend",
			Descriptions: schedulesdirect.DescBlock{
				Description1000: []schedulesdirect.LangText{{Description: "Headlines"}},
			},
			Genres: []string{"News"},
		}},
	}
}

func (f *fakeSD) Token(context.Context, string, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenCalls++
	if f.tokenErr != nil {
		return "", f.tokenErr
	}
	return "test-token", nil
}

func (f *fakeSD) Headends(context.Context, string, string, string) ([]schedulesdirect.Headend, error) {
	if f.headendsErr != nil {
		return nil, f.headendsErr
	}
	return f.headends, nil
}

func (f *fakeSD) AddLineup(context.Context, string, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls++
	return f.addErr
}

func (f *fakeSD) Lineup(context.Context, string, string) (*schedulesdirect.LineupDetail, error) {
	if f.lineupErr != nil {
		return nil, f.lineupErr
	}
	return f.lineup, nil
}

func (f *fakeSD) Schedules(context.Context, string, []schedulesdirect.ScheduleRequest) ([]schedulesdirect.StationSchedule, error) {
	if f.schedErr != nil {
		return nil, f.schedErr
	}
	return f.schedules, nil
}

func (f *fakeSD) Programs(context.Context, string, []string) ([]schedulesdirect.ProgramDetail, error) {
	if f.programsErr != nil {
		return nil, f.programsErr
	}
	return f.programs, nil
}

func newTestService(store Store) (*Service, *fakeSD) {
	svc := NewServiceWithStore(store)
	fake := newFakeSD()
	svc.SetSchedulesDirectClient(fake)
	return svc, fake
}
