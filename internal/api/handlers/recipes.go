package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/sections/recipes"
)

// RecipeHandler exposes recipe-registry metadata over HTTP.
type RecipeHandler struct{}

// HandleList returns all recipe definitions grouped by category.
// GET /api/sections/recipes
func (h *RecipeHandler) HandleList(w http.ResponseWriter, _ *http.Request) {
	groups := map[string][]recipes.RecipeDefinition{}
	for _, rec := range recipes.List() {
		def := rec.Definition()
		if def.Hidden {
			continue
		}
		// Guarantee Presets serializes as `[]` rather than `null` so the UI can
		// iterate without a guard. Recipes that take no preset (e.g. custom_filter)
		// still need a present-but-empty array.
		if def.Presets == nil {
			def.Presets = []recipes.GalleryPreset{}
		}
		key := string(def.Category)
		groups[key] = append(groups[key], def)
	}
	resp := map[string]any{"categories": groups}
	writeJSON(w, http.StatusOK, resp)
}

// CandidateSource provides UI-pickable candidates for a parameterized recipe.
// Recipe families register their own sources via RegisterCandidateSource.
type CandidateSource func(r *http.Request) ([]Candidate, error)

// Candidate is a generic shape: the UI shows DisplayName + optional Subtitle and uses Value as the param.
type Candidate struct {
	Value       string `json:"value"`
	DisplayName string `json:"display_name"`
	Subtitle    string `json:"subtitle,omitempty"`
}

var candidateSources = map[string]CandidateSource{}

// RegisterCandidateSource is called by recipe families that need parameter helpers.
func RegisterCandidateSource(recipeType string, src CandidateSource) {
	candidateSources[recipeType] = src
}

// HandleCandidates is GET /api/sections/recipes/{type}/candidates.
func (h *RecipeHandler) HandleCandidates(w http.ResponseWriter, r *http.Request) {
	typ := chi.URLParam(r, "type")
	src, ok := candidateSources[typ]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown_recipe", "no candidate source for this recipe type")
		return
	}
	cands, err := src(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "candidate_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}
