package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	apimw "github.com/prairie-server/prairie-server/internal/api/middleware"
	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/sections"
	"github.com/prairie-server/prairie-server/internal/sections/recipes"
)

// sectionPreviewFetcher is the minimal interface needed by HandlePreview.
// *sections.Fetcher satisfies this interface.
type sectionPreviewFetcher interface {
	FetchOne(ctx context.Context, resolved sections.ResolvedSection, libraryID *int, libraryIDs []int, userID int, profileID string, filter catalog.AccessFilter) (sections.SectionWithItems, error)
}

// previewRequest is the request body for POST /api/admin/sections/preview.
type previewRequest struct {
	SectionType string          `json:"section_type"`
	Config      json.RawMessage `json:"config"`
	ItemLimit   int             `json:"item_limit"`
	LibraryID   *int            `json:"library_id,omitempty"`
	LibraryIDs  []int           `json:"library_ids,omitempty"`
}

// previewResponse is the response body for POST /api/admin/sections/preview.
type previewResponse struct {
	Items      []*models.MediaItem `json:"items"`
	TotalCount int                 `json:"total_count"`
}

// HandlePreview is POST /api/admin/sections/preview. Returns a sample of items without persisting.
func (h *SectionHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}

	rec, ok := recipes.Get(req.SectionType)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown_type", "section_type not registered")
		return
	}
	if err := rec.Validate(req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}

	limit := req.ItemLimit
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	resolved := sections.ResolvedSection{
		SectionType: sections.SectionType(req.SectionType),
		Title:       "preview",
		ItemLimit:   limit,
		Config:      req.Config,
	}

	fetcher := h.previewFetcher
	if fetcher == nil {
		fetcher = h.fetcher
	}
	if fetcher == nil {
		writeError(w, http.StatusInternalServerError, "preview_unavailable", "section fetcher not configured")
		return
	}

	userID := apimw.GetUserID(r.Context())
	profileID := apimw.GetProfileID(r.Context())
	filter := requestAccessFilter(r)

	result, err := fetcher.FetchOne(r.Context(), resolved, req.LibraryID, req.LibraryIDs, userID, profileID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "preview_failed", err.Error())
		return
	}

	items := result.Items
	if items == nil {
		items = []*models.MediaItem{}
	}
	writeJSON(w, http.StatusOK, previewResponse{Items: items, TotalCount: result.TotalCount})
}
