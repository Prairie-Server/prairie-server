package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/prairie-server/prairie-server/internal/sections"
	"github.com/prairie-server/prairie-server/internal/sections/recipes"
)

// BulkSectionRepo is the minimal repo surface needed for bulk-create.
// CreateMany must run all inserts in a single transaction.
type BulkSectionRepo interface {
	CreateMany(ctx context.Context, sections []*sections.PageSection) error
}

// SectionBulkHandler handles the bulk-create sections endpoint.
type SectionBulkHandler struct {
	Repo BulkSectionRepo
}

type bulkCreateSectionRequest struct {
	Scope       string          `json:"scope"`
	LibraryIDs  []int           `json:"library_ids"`
	SectionType string          `json:"section_type"`
	Title       string          `json:"title"`
	Featured    bool            `json:"featured"`
	ItemLimit   int             `json:"item_limit"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
}

type bulkCreateSectionResponse struct {
	Created int `json:"created"`
}

// HandleBulkCreate handles POST /api/admin/sections/bulk-create.
// It creates the same section across multiple libraries (library scope) or
// a single home-scope section, all within a single transaction.
func (h *SectionBulkHandler) HandleBulkCreate(w http.ResponseWriter, r *http.Request) {
	var req bulkCreateSectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
		return
	}

	scope := req.Scope
	if scope == "" {
		scope = "home"
	}

	// Validate scope.
	switch scope {
	case "home", sectionTypeLibrary:
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "scope must be 'home' or 'library'")
		return
	}

	// Library scope requires at least one library_id.
	if scope == sectionTypeLibrary && len(req.LibraryIDs) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "library_ids must not be empty for library scope")
		return
	}

	// Validate section_type via recipe registry (catches unknown types and invalid config).
	if req.SectionType == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "section_type is required")
		return
	}
	rec, ok := recipes.Get(req.SectionType)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "unknown section_type")
		return
	}
	if err := rec.Validate(req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	itemLimit := req.ItemLimit
	if itemLimit <= 0 {
		itemLimit = 20
	}

	// Build the rows to insert.
	var rows []*sections.PageSection

	switch scope {
	case "home":
		rows = append(rows, &sections.PageSection{
			Scope:       "home",
			LibraryID:   nil,
			SectionType: sections.SectionType(req.SectionType),
			Title:       req.Title,
			Featured:    req.Featured,
			ItemLimit:   itemLimit,
			Config:      req.Config,
			Enabled:     req.Enabled,
		})
	case sectionTypeLibrary:
		for _, libID := range req.LibraryIDs {
			id := libID // capture loop variable
			rows = append(rows, &sections.PageSection{
				Scope:       sectionTypeLibrary,
				LibraryID:   &id,
				SectionType: sections.SectionType(req.SectionType),
				Title:       req.Title,
				Featured:    req.Featured,
				ItemLimit:   itemLimit,
				Config:      req.Config,
				Enabled:     req.Enabled,
			})
		}
	}

	if err := h.Repo.CreateMany(r.Context(), rows); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create sections")
		return
	}

	writeJSON(w, http.StatusOK, bulkCreateSectionResponse{Created: len(rows)})
}
