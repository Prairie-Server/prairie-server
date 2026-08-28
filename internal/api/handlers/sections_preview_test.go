package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/sections"
)

// stubPreviewFetcher is a test double that returns canned items for any inputs.
type stubPreviewFetcher struct {
	items []*models.MediaItem
	total int
}

func (s *stubPreviewFetcher) FetchOne(_ context.Context, _ sections.ResolvedSection, _ *int, _ []int, _ int, _ string, _ catalog.AccessFilter) (sections.SectionWithItems, error) {
	return sections.SectionWithItems{
		Items:      s.items,
		TotalCount: s.total,
	}, nil
}

func TestHandlePreviewRejectsUnknownType(t *testing.T) {
	h := &SectionHandler{}
	body, _ := json.Marshal(map[string]any{
		"section_type": "not_a_real_type",
		"config":       map[string]any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sections/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandlePreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePreviewValidConfig(t *testing.T) {
	h := &SectionHandler{previewFetcher: &stubPreviewFetcher{
		items: []*models.MediaItem{{ContentID: "abc"}},
		total: 1,
	}}
	body, _ := json.Marshal(map[string]any{
		"section_type": string(sections.SectionRecentlyAdded),
		"config":       map[string]any{},
		"item_limit":   10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sections/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandlePreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
