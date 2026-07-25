package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/metadata"
)

// MetadataHandler handles metadata refresh, identify, and search endpoints.
type MetadataHandler struct {
	service *metadata.MetadataService
	queue   *metadata.RefreshQueue
}

// NewMetadataHandler creates a new MetadataHandler.
func NewMetadataHandler(service *metadata.MetadataService, queue *metadata.RefreshQueue) *MetadataHandler {
	return &MetadataHandler{service: service, queue: queue}
}

// RefreshItem handles POST /api/v1/items/{id}/refresh.
func (h *MetadataHandler) RefreshItem(w http.ResponseWriter, r *http.Request) {
	contentID := chi.URLParam(r, "id")
	if contentID == "" {
		http.Error(w, "missing item id", http.StatusBadRequest)
		return
	}

	h.queue.Enqueue(contentID, metadata.PriorityHigh, metadata.ModeManualRefresh)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "queued",
		"content_id": contentID,
	})
}

// RefreshLibrary handles POST /api/v1/libraries/{id}/refresh.
func (h *MetadataHandler) RefreshLibrary(w http.ResponseWriter, r *http.Request) {
	// This would need to look up all items in the library and enqueue them.
	// For now, return accepted.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "queued"})
}

// identifyRequest is the body for POST /api/v1/items/{id}/identify.
type identifyRequest struct {
	ProviderIDs map[string]string `json:"provider_ids"`
}

// IdentifyItem handles POST /api/v1/items/{id}/identify.
func (h *MetadataHandler) IdentifyItem(w http.ResponseWriter, r *http.Request) {
	contentID := chi.URLParam(r, "id")
	if contentID == "" {
		http.Error(w, "missing item id", http.StatusBadRequest)
		return
	}

	var req identifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.service.Process(r.Context(), metadata.ProcessRequest{
		ContentID:   contentID,
		ProviderIDs: req.ProviderIDs,
		Mode:        metadata.ModeIdentify,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// SearchProviders handles GET /api/v1/items/{id}/search.
func (h *MetadataHandler) SearchProviders(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	yearStr := r.URL.Query().Get("year")
	contentType := r.URL.Query().Get("type")

	year := 0
	if yearStr != "" {
		year, _ = strconv.Atoi(yearStr)
	}

	results, err := h.service.SearchProviders(r.Context(), metadata.SearchQuery{
		Title:       query,
		Year:        year,
		ContentType: contentType,
	}, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
