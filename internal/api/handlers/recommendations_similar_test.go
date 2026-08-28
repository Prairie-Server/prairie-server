package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/recommendations"
	"github.com/prairie-server/prairie-server/internal/sections"
)

type stubSimilarEngine struct {
	items []recommendations.ScoredItem
	err   error
}

func (s *stubSimilarEngine) SimilarItems(context.Context, string, int) ([]recommendations.ScoredItem, error) {
	return s.items, s.err
}

func (s *stubSimilarEngine) BecauseYouWatched(context.Context, int, string, string, int) ([]recommendations.ScoredItem, error) {
	return nil, nil
}

func (s *stubSimilarEngine) GetTasteProfileSummary(context.Context, int, string) (*recommendations.TasteProfileSummary, error) {
	return nil, nil
}

type stubCardFetcher struct {
	items    []*models.MediaItem
	err      error
	gotIDs   []string
	overlays map[string]*models.OverlaySummary
}

func (s *stubCardFetcher) FetchItemsByContentIDs(_ context.Context, ids []string, _ catalog.AccessFilter) ([]*models.MediaItem, error) {
	s.gotIDs = ids
	return s.items, s.err
}

func (s *stubCardFetcher) FetchEpisodesByContentIDs(context.Context, []string, catalog.AccessFilter) ([]*models.MediaItem, map[string]sections.SectionItemMeta, error) {
	return nil, nil, nil
}

func (s *stubCardFetcher) ListOverlaySummaries(context.Context, []string, catalog.AccessFilter) (map[string]*models.OverlaySummary, error) {
	return s.overlays, nil
}

type stubPresigner struct{}

func (stubPresigner) PresignURL(_ context.Context, path string, _ string) string {
	if path == "" {
		return ""
	}
	return "https://cdn.example.com/" + path
}

func similarRequest(itemID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/recommendations/similar/"+itemID, nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("item_id", itemID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func decodeSimilar(t *testing.T, rec *httptest.ResponseRecorder) similarItemsResponse {
	t.Helper()
	var body similarItemsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestHandleSimilar_ReturnsHydratedCards(t *testing.T) {
	t.Parallel()

	engine := &stubSimilarEngine{items: []recommendations.ScoredItem{
		{MediaItemID: "movie-a", Score: 0.9, Reason: "genre"},
		{MediaItemID: "movie-b", Score: 0.7},
	}}
	fetcher := &stubCardFetcher{items: []*models.MediaItem{
		{ContentID: "movie-a", Type: "movie", Title: "Movie A", Year: 2021, PosterPath: "library/1/poster/original.rev.webp"},
		{ContentID: "movie-b", Type: "movie", Title: "Movie B", Year: 2019, PosterPath: "library/2/poster/original.rev.webp"},
	}}

	h := NewRecommendationsHandler(engine, nil, nil, nil, nil, true)
	h.Fetcher = fetcher
	h.DetailSvc = stubPresigner{}

	rec := httptest.NewRecorder()
	h.HandleSimilar(rec, similarRequest("movie-source"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodeSimilar(t, rec)

	// Scored ids stay for older clients.
	if len(body.Items) != 2 || body.Items[0].MediaItemID != "movie-a" {
		t.Fatalf("items = %+v, want the scored ids in order", body.Items)
	}
	// Cards let a client render the row without a request per recommendation.
	if len(body.Cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(body.Cards))
	}
	if body.Cards[0].ContentID != "movie-a" || body.Cards[0].Title != "Movie A" {
		t.Fatalf("first card = %+v, want movie-a", body.Cards[0])
	}
	if body.Cards[0].PosterURL == "" {
		t.Fatalf("card poster URL should be presigned, got empty")
	}
	if body.Cards[1].Year != 2019 {
		t.Fatalf("second card year = %d, want 2019", body.Cards[1].Year)
	}
	if len(fetcher.gotIDs) != 2 {
		t.Fatalf("fetcher ids = %v, want both scored ids", fetcher.gotIDs)
	}
}

func TestHandleSimilar_DropsItemsTheProfileCannotSee(t *testing.T) {
	t.Parallel()

	engine := &stubSimilarEngine{items: []recommendations.ScoredItem{
		{MediaItemID: "visible"},
		{MediaItemID: "restricted"},
	}}
	// The access filter is applied by the fetcher, which simply omits the row.
	fetcher := &stubCardFetcher{items: []*models.MediaItem{
		{ContentID: "visible", Type: "movie", Title: "Visible"},
	}}

	h := NewRecommendationsHandler(engine, nil, nil, nil, nil, true)
	h.Fetcher = fetcher
	h.DetailSvc = stubPresigner{}

	rec := httptest.NewRecorder()
	h.HandleSimilar(rec, similarRequest("src"))

	body := decodeSimilar(t, rec)
	if len(body.Cards) != 1 || body.Cards[0].ContentID != "visible" {
		t.Fatalf("cards = %+v, want only the accessible item", body.Cards)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, scored ids should be untouched", len(body.Items))
	}
}

func TestHandleSimilar_OmitsCardsWhenHydrationUnavailable(t *testing.T) {
	t.Parallel()

	engine := &stubSimilarEngine{items: []recommendations.ScoredItem{{MediaItemID: "movie-a"}}}
	h := NewRecommendationsHandler(engine, nil, nil, nil, nil, true)
	// No Fetcher configured.

	rec := httptest.NewRecorder()
	h.HandleSimilar(rec, similarRequest("src"))

	body := decodeSimilar(t, rec)
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Items))
	}
	if body.Cards != nil {
		t.Fatalf("cards = %+v, want omitted so clients fall back", body.Cards)
	}
}

func TestHandleSimilar_DegradesWhenHydrationFails(t *testing.T) {
	t.Parallel()

	engine := &stubSimilarEngine{items: []recommendations.ScoredItem{{MediaItemID: "movie-a"}}}
	h := NewRecommendationsHandler(engine, nil, nil, nil, nil, true)
	h.Fetcher = &stubCardFetcher{err: errors.New("db down")}
	h.DetailSvc = stubPresigner{}

	rec := httptest.NewRecorder()
	h.HandleSimilar(rec, similarRequest("src"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — scored ids are still useful", rec.Code)
	}
	body := decodeSimilar(t, rec)
	if len(body.Items) != 1 || body.Cards != nil {
		t.Fatalf("body = %+v, want ids without cards", body)
	}
}

func TestHandleSimilar_DisabledAndMissingID(t *testing.T) {
	t.Parallel()

	disabled := NewRecommendationsHandler(&stubSimilarEngine{}, nil, nil, nil, nil, false)
	rec := httptest.NewRecorder()
	disabled.HandleSimilar(rec, similarRequest("src"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := decodeSimilar(t, rec); len(body.Items) != 0 || body.Cards != nil {
		t.Fatalf("body = %+v, want empty items", body)
	}

	enabled := NewRecommendationsHandler(&stubSimilarEngine{}, nil, nil, nil, nil, true)
	missing := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recommendations/similar/", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	enabled.HandleSimilar(missing, req)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", missing.Code)
	}
}
