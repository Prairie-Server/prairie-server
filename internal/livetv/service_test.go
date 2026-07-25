package livetv

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv/hdhomerun"
)

func TestServiceAddTunerScansLineup(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"hdhr-1","ModelNumber":"HDHR5-2US","FirmwareVersion":"20260701","TunerCount":2,"BaseURL":"` + srv.URL + `"}`))
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"GuideNumber":"5.1","GuideName":"KING-HD","URL":"` + srv.URL + `/auto/v5.1","HD":true}]`))
	})

	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	svc.SetHDHomeRunClient(hdhomerun.NewClient(srv.Client()))

	tuner, err := svc.AddTuner(context.Background(), srv.URL+"/discover.json", "")
	if err != nil {
		t.Fatalf("AddTuner() error = %v", err)
	}
	if tuner == nil || tuner.DeviceID != "hdhr-1" || tuner.ChannelCount != 1 {
		t.Fatalf("unexpected tuner: %+v", tuner)
	}
	channels, err := svc.ListChannels(context.Background(), tuner.ID)
	if err != nil {
		t.Fatalf("ListChannels() error = %v", err)
	}
	if len(channels) != 1 || channels[0].Number != "5.1" || channels[0].StreamURL == "" || !channels[0].HD {
		t.Fatalf("unexpected channels: %+v", channels)
	}
}

func TestGuideSourceMaxThreeEnabled(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)

	for i := 0; i < MaxGuideSources; i++ {
		_, err := svc.CreateGuideSource(context.Background(), &GuideSource{
			Type:        GuideSourceXMLTVURL,
			Priority:    100 + i,
			Enabled:     true,
			DisplayName: "XMLTV",
			Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
		})
		if err != nil {
			t.Fatalf("CreateGuideSource(%d) error = %v", i, err)
		}
	}
	_, err := svc.CreateGuideSource(context.Background(), &GuideSource{
		Type:        GuideSourceXMLTVURL,
		Priority:    400,
		Enabled:     true,
		DisplayName: "Too many",
		Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
	})
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("CreateGuideSource fourth enabled error = %v, want ErrLimitExceeded", err)
	}

	_, err = svc.CreateGuideSource(context.Background(), &GuideSource{
		Type:        GuideSourceXMLTVURL,
		Priority:    500,
		Enabled:     false,
		DisplayName: "Disabled",
		Config:      map[string]string{"url": "https://example.test/xmltv.xml"},
	})
	if err != nil {
		t.Fatalf("CreateGuideSource disabled error = %v", err)
	}
}

type memoryStore struct {
	mu           sync.Mutex
	next         int
	tuners       map[string]Tuner
	channels     map[string]Channel
	guideSources map[string]GuideSource
	programs     map[string]Program
	sessions     map[string]LiveSession
	recordings   map[string]Recording
	rules        map[string]SeriesRule
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		next:         1,
		tuners:       map[string]Tuner{},
		channels:     map[string]Channel{},
		guideSources: map[string]GuideSource{},
		programs:     map[string]Program{},
		sessions:     map[string]LiveSession{},
		recordings:   map[string]Recording{},
		rules:        map[string]SeriesRule{},
	}
}

func (s *memoryStore) id() string {
	id := s.next
	s.next++
	return string(rune('a' + id))
}

func (s *memoryStore) ListTuners(context.Context) ([]Tuner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Tuner, 0, len(s.tuners))
	for _, tuner := range s.tuners {
		tuner.ChannelCount = s.channelCountLocked(tuner.ID)
		out = append(out, tuner)
	}
	return out, nil
}

func (s *memoryStore) GetTuner(_ context.Context, id string) (*Tuner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tuner, ok := s.tuners[id]
	if !ok {
		return nil, nil
	}
	tuner.ChannelCount = s.channelCountLocked(id)
	return &tuner, nil
}

func (s *memoryStore) CreateTuner(_ context.Context, tuner *Tuner) (*Tuner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.tuners {
		if existing.Type == tuner.Type && existing.DeviceID == tuner.DeviceID {
			tuner.ID = id
			break
		}
	}
	if tuner.ID == "" {
		tuner.ID = s.id()
	}
	s.tuners[tuner.ID] = *tuner
	out := s.tuners[tuner.ID]
	out.ChannelCount = s.channelCountLocked(tuner.ID)
	return &out, nil
}

func (s *memoryStore) DeleteTuner(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tuners, id)
	return nil
}

func (s *memoryStore) ReplaceChannelsForTuner(_ context.Context, tunerID string, channels []Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ch := range s.channels {
		if ch.TunerID == tunerID {
			delete(s.channels, id)
		}
	}
	for i := range channels {
		if channels[i].ID == "" {
			channels[i].ID = s.id()
		}
		channels[i].TunerID = tunerID
		s.channels[channels[i].ID] = channels[i]
	}
	tuner := s.tuners[tunerID]
	tuner.Status = "ready"
	now := time.Now()
	tuner.LastScanAt = &now
	s.tuners[tunerID] = tuner
	return nil
}

func (s *memoryStore) ListChannels(_ context.Context, tunerID string) ([]Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Channel{}
	for _, ch := range s.channels {
		if tunerID == "" || ch.TunerID == tunerID {
			out = append(out, ch)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (s *memoryStore) GetChannel(_ context.Context, id string) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[id]
	if !ok {
		return nil, nil
	}
	return &ch, nil
}

func (s *memoryStore) UpdateChannel(_ context.Context, id string, patch ChannelPatch) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[id]
	if !ok {
		return nil, nil
	}
	if patch.Enabled != nil {
		ch.Enabled = *patch.Enabled
	}
	if patch.NumberOverride != nil {
		ch.NumberOverride = patch.NumberOverride
	}
	if patch.GuideStationID != nil {
		ch.GuideStationID = *patch.GuideStationID
	}
	s.channels[id] = ch
	return &ch, nil
}

func (s *memoryStore) ListGuideSources(_ context.Context, enabledOnly bool) ([]GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []GuideSource{}
	for _, source := range s.guideSources {
		if !enabledOnly || source.Enabled {
			out = append(out, source)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out, nil
}

func (s *memoryStore) GetGuideSource(_ context.Context, id string) (*GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok := s.guideSources[id]
	if !ok {
		return nil, nil
	}
	return &source, nil
}

func (s *memoryStore) CreateGuideSource(_ context.Context, source *GuideSource) (*GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if source.ID == "" {
		source.ID = s.id()
	}
	s.guideSources[source.ID] = *source
	out := s.guideSources[source.ID]
	return &out, nil
}

func (s *memoryStore) UpdateGuideSource(_ context.Context, source *GuideSource) (*GuideSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.guideSources[source.ID]; !ok {
		return nil, nil
	}
	s.guideSources[source.ID] = *source
	out := s.guideSources[source.ID]
	return &out, nil
}

func (s *memoryStore) DeleteGuideSource(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.guideSources, id)
	return nil
}

func (s *memoryStore) SetGuideSourceSyncStatus(_ context.Context, id, status, lastError string, lastSyncAt, nextSyncAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	source := s.guideSources[id]
	source.Status = status
	source.LastError = lastError
	source.LastSyncAt = lastSyncAt
	source.NextSyncAt = nextSyncAt
	s.guideSources[id] = source
	return nil
}

func (s *memoryStore) UpsertPrograms(_ context.Context, sourceID string, programs []Program) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range programs {
		if programs[i].ID == "" {
			programs[i].ID = s.id()
		}
		programs[i].SourceID = sourceID
		s.programs[programs[i].ID] = programs[i]
	}
	return nil
}

func (s *memoryStore) ListGuide(_ context.Context, channelIDs []string, start, end time.Time) ([]Program, error) {
	return nil, nil
}
func (s *memoryStore) GetProgram(_ context.Context, id string) (*Program, error) {
	return nil, nil
}
func (s *memoryStore) ListUpcomingPrograms(_ context.Context, until time.Time) ([]Program, error) {
	return nil, nil
}
func (s *memoryStore) ActiveSessionTunerIndices(_ context.Context, tunerID string) ([]int, error) {
	return nil, nil
}
func (s *memoryStore) CreateSession(_ context.Context, input SessionCreate) (*LiveSession, error) {
	return nil, nil
}
func (s *memoryStore) GetSession(_ context.Context, id string) (*LiveSession, error) {
	return nil, nil
}
func (s *memoryStore) ReleaseSession(_ context.Context, id string) (*LiveSession, error) {
	return nil, nil
}
func (s *memoryStore) ListRecordings(_ context.Context, status string) ([]Recording, error) {
	return nil, nil
}
func (s *memoryStore) CreateRecording(_ context.Context, rec *Recording) (*Recording, error) {
	return nil, nil
}
func (s *memoryStore) CancelRecording(_ context.Context, id string) (*Recording, error) {
	return nil, nil
}
func (s *memoryStore) RecordingExists(_ context.Context, programID, seriesRuleID string) (bool, error) {
	return false, nil
}
func (s *memoryStore) FailDueRecordings(_ context.Context, now time.Time, message string) (int, error) {
	return 0, nil
}
func (s *memoryStore) ListSeriesRules(_ context.Context) ([]SeriesRule, error) {
	return nil, nil
}
func (s *memoryStore) CreateSeriesRule(_ context.Context, rule *SeriesRule) (*SeriesRule, error) {
	return nil, nil
}
func (s *memoryStore) DeleteSeriesRule(_ context.Context, id string) error {
	return nil
}

func (s *memoryStore) channelCountLocked(tunerID string) int {
	count := 0
	for _, ch := range s.channels {
		if ch.TunerID == tunerID {
			count++
		}
	}
	return count
}
