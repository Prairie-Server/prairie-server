package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/models"
)

type LibraryCollectionGroupHandler struct {
	groupRepo *catalog.LibraryCollectionGroupRepository
	collRepo  *catalog.LibraryCollectionRepository
	pool      *pgxpool.Pool
}

func NewLibraryCollectionGroupHandler(
	groupRepo *catalog.LibraryCollectionGroupRepository,
	collRepo *catalog.LibraryCollectionRepository,
	pool *pgxpool.Pool,
) *LibraryCollectionGroupHandler {
	return &LibraryCollectionGroupHandler{groupRepo: groupRepo, collRepo: collRepo, pool: pool}
}

type createGroupRequest struct {
	Name            string  `json:"name"`
	Slug            *string `json:"slug,omitempty"`
	DefaultSortMode *string `json:"default_sort_mode,omitempty"`
}

type updateGroupRequest struct {
	Name            *string `json:"name,omitempty"`
	Slug            *string `json:"slug,omitempty"`
	DefaultSortMode *string `json:"default_sort_mode,omitempty"`
}

type listGroupsResponse struct {
	Groups             []libraryCollectionGroupResponse `json:"groups"`
	UngroupedSortOrder int                              `json:"ungrouped_sort_order"`
}

type collectionGroupReorderRequest struct {
	IDs []string `json:"ids"`
}

func (h *LibraryCollectionGroupHandler) HandleListGroups(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := parseLibraryIDFromRouteParam(w, r, "libraryID")
	if !ok {
		return
	}
	groups, err := h.groupRepo.ListByLibrary(r.Context(), libraryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load groups")
		return
	}
	if groups == nil {
		groups = []models.LibraryCollectionGroup{}
	}

	ungroupedSortOrder := 9999
	if h.groupRepo != nil {
		var err error
		if ungroupedSortOrder, err = h.groupRepo.GetUngroupedSortOrder(r.Context(), libraryID); err != nil {
			ungroupedSortOrder = 9999
		}
	}

	writeJSON(w, http.StatusOK, listGroupsResponse{
		Groups:             toLibraryCollectionGroupResponses(groups),
		UngroupedSortOrder: ungroupedSortOrder,
	})
}

func (h *LibraryCollectionGroupHandler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := parseLibraryIDFromRouteParam(w, r, "libraryID")
	if !ok {
		return
	}
	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name required")
		return
	}
	in := catalog.CreateLibraryCollectionGroupInput{
		LibraryID: libraryID,
		Name:      req.Name,
		Kind:      models.GroupKindRegular,
	}
	if req.Slug != nil {
		in.Slug = *req.Slug
	}
	if req.DefaultSortMode != nil {
		in.DefaultSortMode = models.GroupSortMode(*req.DefaultSortMode)
	}
	g, err := h.groupRepo.Create(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create group")
		return
	}
	writeJSON(w, http.StatusCreated, toLibraryCollectionGroupResponse(*g))
}

func (h *LibraryCollectionGroupHandler) HandleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	var req updateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	in := catalog.UpdateLibraryCollectionGroupInput{Name: req.Name, Slug: req.Slug}
	if req.DefaultSortMode != nil {
		mode := models.GroupSortMode(*req.DefaultSortMode)
		in.DefaultSortMode = &mode
	}
	g, err := h.groupRepo.Update(r.Context(), id, in)
	if err != nil {
		if errors.Is(err, catalog.ErrLibraryCollectionGroupNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Group not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update group")
		return
	}
	writeJSON(w, http.StatusOK, toLibraryCollectionGroupResponse(*g))
}

func (h *LibraryCollectionGroupHandler) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	if err := h.groupRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, catalog.ErrLibraryCollectionGroupNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Group not found")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LibraryCollectionGroupHandler) HandleReorderGroups(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := parseLibraryIDFromRouteParam(w, r, "libraryID")
	if !ok {
		return
	}
	var req collectionGroupReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if err := h.groupRepo.Reorder(r.Context(), libraryID, req.IDs); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LibraryCollectionGroupHandler) HandleReorderCollectionsInGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupID")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "groupID required")
		return
	}

	moveOmitted := r.URL.Query().Get("move_omitted")
	strict := moveOmitted != "" && moveOmitted != "ungrouped"

	var libraryID int
	var targetGroupID *string
	if groupID == "ungrouped" {
		raw := r.URL.Query().Get("library_id")
		if raw == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "library_id query param required for ungrouped reorder")
			return
		}
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid library_id")
			return
		}
		libraryID = id
	} else {
		g, err := h.groupRepo.GetByID(r.Context(), groupID)
		if err != nil {
			if errors.Is(err, catalog.ErrLibraryCollectionGroupNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "Group not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load group")
			return
		}
		libraryID = g.LibraryID
		targetGroupID = &g.ID
	}

	var req collectionGroupReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	err := h.collRepo.MoveAndReorder(r.Context(), catalog.MoveAndReorderInput{
		LibraryID:     libraryID,
		TargetGroupID: targetGroupID,
		OrderedIDs:    req.IDs,
		Strict:        strict,
	})
	var strictErr *catalog.StrictReorderError
	if errors.As(err, &strictErr) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":       "strict_reorder_rejected",
			"missing_ids": strictErr.MissingIDs,
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseLibraryIDFromRouteParam(w http.ResponseWriter, r *http.Request, param string) (int, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "library id required")
		return 0, false
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return 0, false
	}
	return id, true
}
