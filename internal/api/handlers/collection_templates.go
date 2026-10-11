package handlers

import (
	"net/http"

	"github.com/prairie-server/prairie-server/internal/collections/templates"
)

// CollectionTemplateHandler exposes the curated collection-template catalog
// to the admin UI. It is a read-only window onto templates.Default — the
// gallery uses the response to populate cards, then submits via the existing
// /admin/collections/import/* endpoints.
type CollectionTemplateHandler struct {
	registry *templates.Registry
}

// NewCollectionTemplateHandler returns a handler bound to the supplied
// registry; pass templates.Default for the built-in catalog.
func NewCollectionTemplateHandler(registry *templates.Registry) *CollectionTemplateHandler {
	if registry == nil {
		registry = templates.Default
	}
	return &CollectionTemplateHandler{registry: registry}
}

// HandleListTemplates serves GET /admin/collections/templates.
func (h *CollectionTemplateHandler) HandleListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.registry.Catalog())
}
