package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/livetv"
)

type fakeLiveTVStore struct {
	tuners []livetv.Tuner
}

func (s *fakeLiveTVStore) GetRecording(context.Context, string) (*livetv.Recording, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) GetSeriesRule(context.Context, string) (*livetv.SeriesRule, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) ListTuners(context.Context) ([]livetv.Tuner, error) {
	return s.tuners, nil
}
func (s *fakeLiveTVStore) GetTuner(context.Context, string) (*livetv.Tuner, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) CreateTuner(context.Context, *livetv.Tuner) (*livetv.Tuner, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) DeleteTuner(context.Context, string) error { return nil }
func (s *fakeLiveTVStore) ReplaceChannelsForTuner(context.Context, string, []livetv.Channel) error {
	return nil
}
func (s *fakeLiveTVStore) ListChannels(context.Context, string) ([]livetv.Channel, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) GetChannel(context.Context, string) (*livetv.Channel, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) UpdateChannel(context.Context, string, livetv.ChannelPatch) (*livetv.Channel, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) ListGuideSources(context.Context, bool) ([]livetv.GuideSource, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) GetGuideSource(context.Context, string) (*livetv.GuideSource, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) CreateGuideSource(context.Context, *livetv.GuideSource) (*livetv.GuideSource, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) UpdateGuideSource(context.Context, *livetv.GuideSource) (*livetv.GuideSource, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) DeleteGuideSource(context.Context, string) error { return nil }
func (s *fakeLiveTVStore) SetGuideSourceSyncStatus(context.Context, string, string, string, *time.Time, *time.Time) error {
	return nil
}
func (s *fakeLiveTVStore) UpsertPrograms(context.Context, string, []livetv.Program) error {
	return nil
}
func (s *fakeLiveTVStore) ListGuide(context.Context, []string, time.Time, time.Time) ([]livetv.Program, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) GetProgram(context.Context, string) (*livetv.Program, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) ListUpcomingPrograms(context.Context, time.Time) ([]livetv.Program, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) ActiveSessionTunerIndices(context.Context, string) ([]int, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) CreateSession(context.Context, livetv.SessionCreate) (*livetv.LiveSession, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) GetSession(context.Context, string) (*livetv.LiveSession, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) ReleaseSession(context.Context, string) (*livetv.LiveSession, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) ListRecordings(context.Context, string) ([]livetv.Recording, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) CreateRecording(context.Context, *livetv.Recording) (*livetv.Recording, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) UpdateRecording(context.Context, *livetv.Recording) (*livetv.Recording, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) CancelRecording(context.Context, string) (*livetv.Recording, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) RecordingExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (s *fakeLiveTVStore) ListActiveRecordingPairs(context.Context) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}
func (s *fakeLiveTVStore) FailDueRecordings(context.Context, time.Time, string) (int, error) {
	return 0, nil
}
func (s *fakeLiveTVStore) ListSeriesRules(context.Context) ([]livetv.SeriesRule, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) CreateSeriesRule(context.Context, *livetv.SeriesRule) (*livetv.SeriesRule, error) {
	return nil, nil
}
func (s *fakeLiveTVStore) DeleteSeriesRule(context.Context, string) error { return nil }

func TestLiveTVHandlerListTuners(t *testing.T) {
	svc := livetv.NewServiceWithStore(&fakeLiveTVStore{
		tuners: []livetv.Tuner{{ID: "t1", Type: livetv.TunerTypeHDHomeRun, DeviceID: "abc", Status: "ready"}},
	})
	h := NewLiveTVHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/livetv/tuners", nil)
	rec := httptest.NewRecorder()
	h.HandleListTuners(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Tuners []livetv.Tuner `json:"tuners"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tuners) != 1 || body.Tuners[0].DeviceID != "abc" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestLiveTVHandlerGetProgramNotFound(t *testing.T) {
	svc := livetv.NewServiceWithStore(&fakeLiveTVStore{})
	h := NewLiveTVHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/livetv/programs/missing", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("programId", "missing")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.HandleGetProgram(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}
