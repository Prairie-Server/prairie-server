package handlers

import (
	"net/http"

	"github.com/prairie-server/prairie-server/internal/recommendations"
)

// AdminRecommendationsHandler handles admin recommendation status and trigger endpoints.
type AdminRecommendationsHandler struct {
	worker *recommendations.Worker
}

// NewAdminRecommendationsHandler creates a new AdminRecommendationsHandler.
func NewAdminRecommendationsHandler(worker *recommendations.Worker) *AdminRecommendationsHandler {
	return &AdminRecommendationsHandler{worker: worker}
}

// jobStatusResponse is the status for a single job.
type jobStatusResponse struct {
	Running bool `json:"running"`
	Count   int  `json:"count"`
	Total   int  `json:"total,omitempty"`
}

// recommendationsStatusResponse is the full status response.
type recommendationsStatusResponse struct {
	Embeddings      jobStatusResponse `json:"embeddings"`
	TasteProfiles   jobStatusResponse `json:"taste_profiles"`
	Cowatch         jobStatusResponse `json:"cowatch"`
	Recommendations jobStatusResponse `json:"recommendations"`
}

// HandleStatus handles GET /admin/recommendations/status.
func (h *AdminRecommendationsHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	embedded, totalItems, tasteProfiles, cacheEntries, cowatchPairs, err := h.worker.StatusCounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch recommendation status")
		return
	}

	resp := recommendationsStatusResponse{
		Embeddings: jobStatusResponse{
			Running: h.worker.IsRunning(recommendations.JobEmbeddings),
			Count:   embedded,
			Total:   totalItems,
		},
		TasteProfiles: jobStatusResponse{
			Running: h.worker.IsRunning(recommendations.JobTasteProfiles),
			Count:   tasteProfiles,
		},
		Cowatch: jobStatusResponse{
			Running: h.worker.IsRunning(recommendations.JobCowatch),
			Count:   cowatchPairs,
		},
		Recommendations: jobStatusResponse{
			Running: h.worker.IsRunning(recommendations.JobRecommendations),
			Count:   cacheEntries,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleTriggerEmbeddings handles POST /admin/recommendations/trigger/embeddings.
func (h *AdminRecommendationsHandler) HandleTriggerEmbeddings(w http.ResponseWriter, r *http.Request) {
	if err := h.worker.TriggerEmbeddings(); err != nil {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// HandleTriggerTasteProfiles handles POST /admin/recommendations/trigger/taste-profiles.
func (h *AdminRecommendationsHandler) HandleTriggerTasteProfiles(w http.ResponseWriter, r *http.Request) {
	if err := h.worker.TriggerTasteProfiles(); err != nil {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// HandleTriggerCowatch handles POST /admin/recommendations/trigger/cowatch.
func (h *AdminRecommendationsHandler) HandleTriggerCowatch(w http.ResponseWriter, r *http.Request) {
	if err := h.worker.TriggerCowatch(); err != nil {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// HandleTriggerRecommendations handles POST /admin/recommendations/trigger/recommendations.
func (h *AdminRecommendationsHandler) HandleTriggerRecommendations(w http.ResponseWriter, r *http.Request) {
	if err := h.worker.TriggerRecommendations(); err != nil {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}
