package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/activitylog"
)

// AdminIPHandler serves admin endpoints for IP/user visibility.
type AdminIPHandler struct {
	repo *activitylog.Repo
}

// NewAdminIPHandler creates a new AdminIPHandler.
func NewAdminIPHandler(repo *activitylog.Repo) *AdminIPHandler {
	return &AdminIPHandler{repo: repo}
}

// HandleGetUserIPs handles GET /admin/users/{id}/ips.
func (h *AdminIPHandler) HandleGetUserIPs(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	userID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid user ID")
		return
	}

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, parseErr := strconv.Atoi(d); parseErr == nil && parsed > 0 {
			days = parsed
		}
	}

	entries, err := h.repo.UserIPs(r.Context(), userID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query user IPs")
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

// HandleGetIPUsers handles GET /admin/ips?ip=...
func (h *AdminIPHandler) HandleGetIPUsers(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "ip query parameter is required")
		return
	}

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, parseErr := strconv.Atoi(d); parseErr == nil && parsed > 0 {
			days = parsed
		}
	}

	entries, err := h.repo.IPUsers(r.Context(), ip, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to query IP users")
		return
	}

	writeJSON(w, http.StatusOK, entries)
}
