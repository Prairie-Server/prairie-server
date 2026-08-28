package handlers

import (
	"net/http"
	"slices"
	"time"

	"github.com/prairie-server/prairie-server/internal/jellycompat"
)

type adminServerStatusResponse struct {
	StartedAt             time.Time  `json:"started_at"`
	RestartRequired       bool       `json:"restart_required"`
	RestartRequiredAt     *time.Time `json:"restart_required_at,omitempty"`
	RestartRequiredReason string     `json:"restart_required_reason,omitempty"`
	// RestartRequiredReasons accumulates every distinct reason marked since
	// boot ("setting:<key>" entries for settings saves), so a client can scope
	// a pending restart to the subsystem it belongs to. The singular field
	// above only remembers the last save.
	RestartRequiredReasons []string `json:"restart_required_reasons,omitempty"`
	// RestartMarkCount increments on every restart-required save. The boolean
	// above latches for the process lifetime, so this is the client's only
	// signal that a NEW requirement arrived after one was dismissed.
	RestartMarkCount   int        `json:"restart_mark_count"`
	RestartRequested   bool       `json:"restart_requested"`
	RestartRequestedAt *time.Time `json:"restart_requested_at,omitempty"`
}

// HandleGetServerStatus handles GET /admin/server/status.
func (h *AdminHandler) HandleGetServerStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := h.RestartStatus.Snapshot()
	resp := adminServerStatusResponse{
		StartedAt:              snapshot.StartedAt,
		RestartRequired:        snapshot.RestartRequired,
		RestartRequiredAt:      snapshot.RestartRequiredAt,
		RestartRequiredReason:  snapshot.RestartRequiredReason,
		RestartRequiredReasons: snapshot.RestartReasons,
		RestartMarkCount:       snapshot.RestartMarkCount,
		RestartRequested:       snapshot.RestartRequested,
		RestartRequestedAt:     snapshot.RestartRequestedAt,
	}

	if h.SettingsRepo != nil {
		settings, err := h.SettingsRepo.GetAll(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load settings")
			return
		}
		if jellycompat.WebComponentStatusForConfig(h.Config, settings).RestartRequired {
			resp.RestartRequired = true
			if resp.RestartRequiredReason == "" {
				resp.RestartRequiredReason = "jellyfin_compat"
			}
			// This requirement is derived here rather than marked on the
			// tracker, so the accumulated list has to gain it too — a client
			// scoping restarts by reason would otherwise never see it.
			if !slices.Contains(resp.RestartRequiredReasons, "jellyfin_compat") {
				resp.RestartRequiredReasons = append(resp.RestartRequiredReasons, "jellyfin_compat")
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *AdminHandler) markServerRestartRequired(reason string) {
	if h == nil {
		return
	}
	h.RestartStatus.MarkRequired(reason)
}
