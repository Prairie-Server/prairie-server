package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "github.com/prairie-server/prairie-server/internal/api/middleware"
	"github.com/prairie-server/prairie-server/internal/auth"
	"github.com/prairie-server/prairie-server/internal/models"
)

// InviteCodeHandler handles admin endpoints for invite code management.
type InviteCodeHandler struct {
	repo *auth.InviteCodeRepository
}

// NewInviteCodeHandler creates a new InviteCodeHandler.
func NewInviteCodeHandler(repo *auth.InviteCodeRepository) *InviteCodeHandler {
	return &InviteCodeHandler{repo: repo}
}

// --- Request/Response types ---

type createInviteCodeRequest struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	MaxUses int    `json:"max_uses"`
}

type updateInviteCodeRequest struct {
	Label   *string `json:"label"`
	MaxUses *int    `json:"max_uses"`
	Enabled *bool   `json:"enabled"`
}

type topUpInviteCodeRequest struct {
	AdditionalUses int `json:"additional_uses"`
}

type inviteCodeResponse struct {
	ID        int       `json:"id"`
	Code      string    `json:"code"`
	Label     string    `json:"label"`
	MaxUses   int       `json:"max_uses"`
	UseCount  int       `json:"use_count"`
	CreatedBy int       `json:"created_by"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toInviteCodeResponse(ic *models.InviteCode) inviteCodeResponse {
	return inviteCodeResponse{
		ID:        ic.ID,
		Code:      ic.Code,
		Label:     ic.Label,
		MaxUses:   ic.MaxUses,
		UseCount:  ic.UseCount,
		CreatedBy: ic.CreatedBy,
		Enabled:   ic.Enabled,
		CreatedAt: ic.CreatedAt,
		UpdatedAt: ic.UpdatedAt,
	}
}

// HandleListInviteCodes handles GET /admin/invite-codes.
func (h *InviteCodeHandler) HandleListInviteCodes(w http.ResponseWriter, r *http.Request) {
	codes, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list invite codes")
		return
	}

	resp := make([]inviteCodeResponse, 0, len(codes))
	for _, ic := range codes {
		resp = append(resp, toInviteCodeResponse(ic))
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleCreateInviteCode handles POST /admin/invite-codes.
func (h *InviteCodeHandler) HandleCreateInviteCode(w http.ResponseWriter, r *http.Request) {
	var req createInviteCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
		return
	}

	if req.MaxUses <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "max_uses must be greater than 0")
		return
	}

	claims := apimw.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	ic, err := h.repo.Create(r.Context(), models.CreateInviteCodeInput{
		Code:      req.Code,
		Label:     req.Label,
		MaxUses:   req.MaxUses,
		CreatedBy: claims.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create invite code")
		return
	}

	writeJSON(w, http.StatusCreated, toInviteCodeResponse(ic))
}

// HandleUpdateInviteCode handles PUT /admin/invite-codes/{id}.
func (h *InviteCodeHandler) HandleUpdateInviteCode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid invite code ID")
		return
	}

	var req updateInviteCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
		return
	}

	if err := h.repo.Update(r.Context(), id, models.UpdateInviteCodeInput{
		Label:   req.Label,
		MaxUses: req.MaxUses,
		Enabled: req.Enabled,
	}); err != nil {
		if errors.Is(err, auth.ErrInviteCodeNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Invite code not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update invite code")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTopUpInviteCode handles POST /admin/invite-codes/{id}/top-up.
func (h *InviteCodeHandler) HandleTopUpInviteCode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid invite code ID")
		return
	}

	var req topUpInviteCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
		return
	}

	ic, err := h.repo.TopUp(r.Context(), id, req.AdditionalUses)
	if err != nil {
		if errors.Is(err, auth.ErrInviteCodeInvalid) {
			writeError(w, http.StatusBadRequest, "bad_request", "additional_uses must be greater than 0")
			return
		}
		if errors.Is(err, auth.ErrInviteCodeNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Invite code not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to top up invite code")
		return
	}

	writeJSON(w, http.StatusOK, toInviteCodeResponse(ic))
}

// HandleDeleteInviteCode handles DELETE /admin/invite-codes/{id}.
func (h *InviteCodeHandler) HandleDeleteInviteCode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid invite code ID")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, auth.ErrInviteCodeNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Invite code not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete invite code")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
