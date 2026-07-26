package jellycompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/prairie-server/prairie-server/internal/config"
	"github.com/prairie-server/prairie-server/internal/livetv"
)

type livetvTestStore struct {
	tuners     []livetv.Tuner
	channels   map[string]livetv.Channel
	programs   map[string]livetv.Program
	recordings map[string]livetv.Recording
	rules      map[string]livetv.SeriesRule
	sessions   map[string]livetv.LiveSession
}

func newLivetvTestStore() *livetvTestStore {
	return &livetvTestStore{
		channels:   map[string]livetv.Channel{},
		programs:   map[string]livetv.Program{},
		recordings: map[string]livetv.Recording{},
		rules:      map[string]livetv.SeriesRule{},
		sessions:   map[string]livetv.LiveSession{},
	}
}


func (s *livetvTestStore) GetRecording(context.Context, string) (*livetv.Recording, error) { return nil, nil }
func (s *livetvTestStore) GetSeriesRule(context.Context, string) (*livetv.SeriesRule, error) { return nil, nil }
func (s *livetvTestStore) ListTuners(context.Context) ([]livetv.Tuner, error) {
	return s.tuners, nil
}
func (s *livetvTestStore) GetTuner(_ context.Context, id string) (*livetv.Tuner, error) {
	for i := range s.tuners {
		if s.tuners[i].ID == id {
			t := s.tuners[i]
			return &t, nil
		}
	}
	return nil, nil
}
func (s *livetvTestStore) CreateTuner(context.Context, *livetv.Tuner) (*livetv.Tuner, error) {
	return nil, errors.New("not implemented")
}
func (s *livetvTestStore) DeleteTuner(context.Context, string) error { return nil }
func (s *livetvTestStore) ReplaceChannelsForTuner(context.Context, string, []livetv.Channel) error {
	return nil
}
func (s *livetvTestStore) ListChannels(context.Context, string) ([]livetv.Channel, error) {
	out := make([]livetv.Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		out = append(out, ch)
	}
	return out, nil
}
func (s *livetvTestStore) GetChannel(_ context.Context, id string) (*livetv.Channel, error) {
	ch, ok := s.channels[id]
	if !ok {
		return nil, nil
	}
	return &ch, nil
}
func (s *livetvTestStore) UpdateChannel(context.Context, string, livetv.ChannelPatch) (*livetv.Channel, error) {
	return nil, nil
}
func (s *livetvTestStore) ListGuideSources(context.Context, bool) ([]livetv.GuideSource, error) {
	return nil, nil
}
func (s *livetvTestStore) GetGuideSource(context.Context, string) (*livetv.GuideSource, error) {
	return nil, nil
}
func (s *livetvTestStore) CreateGuideSource(context.Context, *livetv.GuideSource) (*livetv.GuideSource, error) {
	return nil, nil
}
func (s *livetvTestStore) UpdateGuideSource(context.Context, *livetv.GuideSource) (*livetv.GuideSource, error) {
	return nil, nil
}
func (s *livetvTestStore) DeleteGuideSource(context.Context, string) error { return nil }
func (s *livetvTestStore) SetGuideSourceSyncStatus(context.Context, string, string, string, *time.Time, *time.Time) error {
	return nil
}
func (s *livetvTestStore) UpsertPrograms(context.Context, string, []livetv.Program) error {
	return nil
}
func (s *livetvTestStore) ListGuide(_ context.Context, channelIDs []string, start, end time.Time) ([]livetv.Program, error) {
	want := map[string]bool{}
	for _, id := range channelIDs {
		want[id] = true
	}
	out := make([]livetv.Program, 0)
	for _, p := range s.programs {
		if len(want) > 0 && !want[p.ChannelID] {
			continue
		}
		if p.Stop.Before(start) || p.Start.After(end) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
func (s *livetvTestStore) GetProgram(_ context.Context, id string) (*livetv.Program, error) {
	p, ok := s.programs[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}
func (s *livetvTestStore) ListUpcomingPrograms(context.Context, time.Time) ([]livetv.Program, error) {
	return nil, nil
}
func (s *livetvTestStore) ActiveSessionTunerIndices(context.Context, string) ([]int, error) {
	return nil, nil
}
func (s *livetvTestStore) CreateSession(_ context.Context, input livetv.SessionCreate) (*livetv.LiveSession, error) {
	sess := livetv.LiveSession{
		ID: "sess-" + input.ChannelID, ChannelID: input.ChannelID, TunerID: input.TunerID,
		TunerIndex: input.TunerIndex, Status: "active", CreatedAt: time.Now().UTC(),
	}
	s.sessions[sess.ID] = sess
	return &sess, nil
}
func (s *livetvTestStore) GetSession(_ context.Context, id string) (*livetv.LiveSession, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return nil, nil
	}
	return &sess, nil
}
func (s *livetvTestStore) ReleaseSession(_ context.Context, id string) (*livetv.LiveSession, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return nil, nil
	}
	sess.Status = "released"
	s.sessions[id] = sess
	return &sess, nil
}
func (s *livetvTestStore) ListRecordings(_ context.Context, status string) ([]livetv.Recording, error) {
	out := make([]livetv.Recording, 0, len(s.recordings))
	for _, rec := range s.recordings {
		if status != "" && rec.Status != status {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}
func (s *livetvTestStore) CreateRecording(_ context.Context, rec *livetv.Recording) (*livetv.Recording, error) {
	if rec.ID == "" {
		rec.ID = "rec-" + uuid.NewString()[:8]
	}
	if rec.Status == "" {
		rec.Status = "scheduled"
	}
	s.recordings[rec.ID] = *rec
	cp := *rec
	return &cp, nil
}
func (s *livetvTestStore) CancelRecording(_ context.Context, id string) (*livetv.Recording, error) {
	rec, ok := s.recordings[id]
	if !ok {
		return nil, nil
	}
	rec.Status = "cancelled"
	s.recordings[id] = rec
	return &rec, nil
}
func (s *livetvTestStore) RecordingExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *livetvTestStore) ListActiveRecordingPairs(context.Context) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}
func (s *livetvTestStore) FailDueRecordings(context.Context, time.Time, string) (int, error) {
	return 0, nil
}
func (s *livetvTestStore) ListSeriesRules(context.Context) ([]livetv.SeriesRule, error) {
	out := make([]livetv.SeriesRule, 0, len(s.rules))
	for _, rule := range s.rules {
		out = append(out, rule)
	}
	return out, nil
}
func (s *livetvTestStore) CreateSeriesRule(_ context.Context, rule *livetv.SeriesRule) (*livetv.SeriesRule, error) {
	if rule.ID == "" {
		rule.ID = "rule-" + uuid.NewString()[:8]
	}
	s.rules[rule.ID] = *rule
	cp := *rule
	return &cp, nil
}
func (s *livetvTestStore) DeleteSeriesRule(_ context.Context, id string) error {
	delete(s.rules, id)
	return nil
}

func newTestLiveTVHandler(store *livetvTestStore) *LiveTVHandler {
	cfg := &config.Config{}
	cfg.JellyfinCompat.ServerID = "test-server"
	svc := livetv.NewServiceWithStore(store)
	h := NewLiveTVHandler(svc, NewResourceIDCodec(), cfg)
	h.now = func() time.Time { return time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC) }
	return h
}

func TestLiveTVHandleInfo(t *testing.T) {
	store := newLivetvTestStore()
	store.tuners = []livetv.Tuner{{ID: "t1", DeviceID: "HDHR1", Model: "HDHR5", TunerCount: 2}}
	h := newTestLiveTVHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/LiveTv/Info", nil)
	rr := httptest.NewRecorder()
	h.HandleInfo(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var info liveTVInfoDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !info.IsEnabled || len(info.Services) != 1 || info.Services[0].Status != "Ok" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if len(info.Services[0].Tuners) != 1 || !strings.Contains(info.Services[0].Tuners[0], "HDHR1") {
		t.Fatalf("tuners = %+v", info.Services[0].Tuners)
	}
}

func TestLiveTVHandleChannelsMapping(t *testing.T) {
	store := newLivetvTestStore()
	override := "7.1"
	store.channels["ch1"] = livetv.Channel{
		ID: "ch1", TunerID: "t1", Number: "5.1", NumberOverride: &override,
		Callsign: "KING", Name: "KING-HD", HD: true, Enabled: true, StreamURL: "http://x/v5.1",
	}
	store.channels["ch2"] = livetv.Channel{
		ID: "ch2", TunerID: "t1", Number: "9.1", Name: "Disabled", Enabled: false, StreamURL: "http://x/v9.1",
	}
	now := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC)
	store.programs["p1"] = livetv.Program{
		ID: "p1", ChannelID: "ch1", Title: "News", Start: now.Add(-30 * time.Minute),
		Stop: now.Add(30 * time.Minute), IsLive: true, IsNew: true,
	}
	h := newTestLiveTVHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/LiveTv/Channels?AddCurrentProgram=true", nil)
	rr := httptest.NewRecorder()
	h.HandleChannels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result queryResultDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.TotalRecordCount != 1 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}
	item := result.Items[0]
	if item.Type != "TvChannel" || item.ChannelNumber != "7.1" || item.Name != "KING-HD" || !item.IsHD {
		t.Fatalf("channel mapping = %+v", item)
	}
	if item.CurrentProgram == nil || item.CurrentProgram.Name != "News" || !item.CurrentProgram.IsLive {
		t.Fatalf("current program = %+v", item.CurrentProgram)
	}
	wantID := h.codec.EncodeStringID(EncodedIDLiveTVChannel, "ch1")
	if item.ID != wantID {
		t.Fatalf("id = %s want %s", item.ID, wantID)
	}
}

func TestLiveTVHandlePrograms(t *testing.T) {
	store := newLivetvTestStore()
	store.channels["ch1"] = livetv.Channel{ID: "ch1", Enabled: true, Number: "5.1", Name: "KING"}
	start := time.Date(2026, 7, 25, 19, 0, 0, 0, time.UTC)
	store.programs["p1"] = livetv.Program{
		ID: "p1", ChannelID: "ch1", Title: "Drama", Subtitle: "Ep 1",
		Start: start, Stop: start.Add(time.Hour), IsNew: true, Season: intPtr(2), Episode: intPtr(3),
	}
	h := newTestLiveTVHandler(store)
	chID := h.codec.EncodeStringID(EncodedIDLiveTVChannel, "ch1")

	req := httptest.NewRequest(http.MethodGet,
		"/LiveTv/Programs?ChannelIds="+chID+"&MinStartDate=2026-07-25T18:00:00Z&MaxEndDate=2026-07-25T23:00:00Z", nil)
	rr := httptest.NewRecorder()
	h.HandlePrograms(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result queryResultDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %+v", result.Items)
	}
	p := result.Items[0]
	if p.Type != "Program" || p.Name != "Drama" || p.EpisodeTitle != "Ep 1" || !p.IsNew {
		t.Fatalf("program = %+v", p)
	}
	if p.ChannelID == nil || *p.ChannelID != chID {
		t.Fatalf("channel id = %v", p.ChannelID)
	}
}

func TestLiveTVTimersMapping(t *testing.T) {
	store := newLivetvTestStore()
	store.channels["ch1"] = livetv.Channel{ID: "ch1", Enabled: true, Number: "5.1", Name: "KING"}
	start := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	store.recordings["rec1"] = livetv.Recording{
		ID: "rec1", ChannelID: "ch1", ProgramID: "p1", Title: "Drama",
		UserID: 7, ProfileID: "profile-1",
		Status: "scheduled", Start: start, Stop: start.Add(time.Hour),
	}
	store.rules["rule1"] = livetv.SeriesRule{
		ID: "rule1", SeriesID: "drama", TitleMatch: "Drama", NewOnly: true, KeepLast: 5, Enabled: true,
		UserID: 7, ProfileID: "profile-1",
	}
	h := newTestLiveTVHandler(store)
	compatSession := &Session{Token: "tok", StreamAppUserID: 7, ProfileID: "profile-1"}

	req := httptest.NewRequest(http.MethodGet, "/LiveTv/Timers", nil)
	req = req.WithContext(context.WithValue(req.Context(), compatSessionKey, compatSession))
	rr := httptest.NewRecorder()
	h.HandleTimers(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("timers status = %d", rr.Code)
	}
	var timers timerQueryResultDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &timers); err != nil {
		t.Fatalf("decode timers: %v", err)
	}
	if len(timers.Items) != 1 || timers.Items[0].Status != "New" || timers.Items[0].Type != "Timer" {
		t.Fatalf("timers = %+v", timers.Items)
	}
	wantTimerID := h.codec.EncodeStringID(EncodedIDLiveTVTimer, "rec1")
	if timers.Items[0].ID != wantTimerID {
		t.Fatalf("timer id = %s want %s", timers.Items[0].ID, wantTimerID)
	}

	req = httptest.NewRequest(http.MethodGet, "/LiveTv/SeriesTimers", nil)
	req = req.WithContext(context.WithValue(req.Context(), compatSessionKey, compatSession))
	rr = httptest.NewRecorder()
	h.HandleSeriesTimers(rr, req)
	var series timerQueryResultDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &series); err != nil {
		t.Fatalf("decode series: %v", err)
	}
	if len(series.Items) != 1 || !series.Items[0].RecordNewOnly || series.Items[0].KeepUpTo != 5 {
		t.Fatalf("series timers = %+v", series.Items)
	}

	unauth := httptest.NewRequest(http.MethodGet, "/LiveTv/Timers", nil)
	unauthRR := httptest.NewRecorder()
	h.HandleTimers(unauthRR, unauth)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("unmapped session timers status = %d, want 401", unauthRR.Code)
	}
}

func TestCopyLiveStreamWithReconnect(t *testing.T) {
	var opens atomic.Int32
	payloads := [][]byte{[]byte("AAAA"), []byte("BBBB")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	err := copyLiveStreamWithReconnect(ctx, &out, nil, func(context.Context) (io.ReadCloser, error) {
		n := int(opens.Add(1))
		if n > len(payloads) {
			cancel()
			return nil, errors.New("done")
		}
		return io.NopCloser(&flakyReader{data: payloads[n-1], failAfter: len(payloads[n-1])}), nil
	}, liveStreamReconnectOpts{
		InitialBackoff: time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
		MaxRetries:     5,
	})
	if !errors.Is(err, context.Canceled) && err != nil {
		// MaxRetries or cancel both acceptable after successful reconnect copies.
		t.Logf("ended with: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "AAAA") || !strings.Contains(got, "BBBB") {
		t.Fatalf("output = %q, want both chunks after reconnect", got)
	}
	if opens.Load() < 2 {
		t.Fatalf("opens = %d, want at least 2 (reconnect)", opens.Load())
	}
}

// flakyReader returns data then a permanent read error (simulating corrupt TS /
// jellyfin#11415 upstream death) so the proxy reconnects.
type flakyReader struct {
	data      []byte
	failAfter int
	off       int
}

func (r *flakyReader) Read(p []byte) (int, error) {
	if r.off >= r.failAfter {
		return 0, errors.New("corrupt ts")
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	if r.off >= r.failAfter {
		return n, errors.New("corrupt ts")
	}
	return n, nil
}

func TestLiveTVGetChannelService(t *testing.T) {
	store := newLivetvTestStore()
	store.channels["ch1"] = livetv.Channel{ID: "ch1", Number: "5.1", Enabled: true}
	svc := livetv.NewServiceWithStore(store)
	ch, err := svc.GetChannel(context.Background(), "ch1")
	if err != nil || ch == nil || ch.Number != "5.1" {
		t.Fatalf("GetChannel = %+v err=%v", ch, err)
	}
	_, err = svc.GetChannel(context.Background(), "missing")
	if !errors.Is(err, livetv.ErrNotFound) {
		t.Fatalf("missing channel err = %v", err)
	}
}

func TestLiveTVHandleChannelRoute(t *testing.T) {
	store := newLivetvTestStore()
	store.channels["ch1"] = livetv.Channel{ID: "ch1", Number: "5.1", Name: "KING", Enabled: true}
	h := newTestLiveTVHandler(store)
	id := h.codec.EncodeStringID(EncodedIDLiveTVChannel, "ch1")

	req := httptest.NewRequest(http.MethodGet, "/LiveTv/Channels/"+id, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.HandleChannel(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var item baseItemDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item.ID != id || item.Name != "KING" {
		t.Fatalf("item = %+v", item)
	}
}
