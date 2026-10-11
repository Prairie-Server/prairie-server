package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prairie-server/prairie-server/internal/sections/recipes"
)

func TestHandleCandidatesUnknownTypeReturns404(t *testing.T) {
	h := &RecipeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/sections/recipes/missing/candidates", nil)
	req = withProfileRouteParam(req, "type", "missing")
	rec := httptest.NewRecorder()

	h.HandleCandidates(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHandleListRecipesGroupsByCategory(t *testing.T) {
	h := &RecipeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/sections/recipes", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Categories map[string][]recipes.RecipeDefinition `json:"categories"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Categories) == 0 {
		t.Fatal("expected categories")
	}
	if _, ok := resp.Categories[string(recipes.CategoryLibraryStaples)]; !ok {
		t.Errorf("missing library_staples category in response")
	}
}

func TestHandleListExcludesHiddenRecipes(t *testing.T) {
	h := &RecipeHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/sections/recipes", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Categories map[string][]recipes.RecipeDefinition `json:"categories"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for cat, defs := range resp.Categories {
		for _, def := range defs {
			if def.Type == "genre" {
				t.Errorf("category %q: hidden recipe type %q should not appear in catalog list", cat, def.Type)
			}
			if def.Hidden {
				t.Errorf("category %q: recipe %q has Hidden=true but was returned in catalog list", cat, def.Type)
			}
		}
	}

	// custom_filter should still appear (it is not hidden)
	var foundCustomFilter bool
	for _, defs := range resp.Categories {
		for _, def := range defs {
			if def.Type == "custom_filter" {
				foundCustomFilter = true
			}
		}
	}
	if !foundCustomFilter {
		t.Error("custom_filter recipe should appear in catalog list")
	}
}
