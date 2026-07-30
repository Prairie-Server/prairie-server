package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestHandleCreateLibrary_RejectsChapterThumbnailsWithoutArtworkStore(t *testing.T) {
	handler := &LibraryHandler{}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/libraries",
		strings.NewReader(`{
			"name":"Movies",
			"type":"movies",
			"paths":["/mnt/media/movies"],
			"chapter_thumbnails_enabled":true
		}`),
	)
	rr := httptest.NewRecorder()

	handler.HandleCreateLibrary(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Chapter thumbnails require configured artwork storage") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestHandleUpdateLibrary_RejectsChapterThumbnailsWithoutArtworkStore(t *testing.T) {
	handler := &LibraryHandler{}
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/libraries/42",
		strings.NewReader(`{"chapter_thumbnails_enabled":true}`),
	)
	req = withPlaybackRouteParam(req, "id", "42")
	rr := httptest.NewRecorder()

	handler.HandleUpdateLibrary(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Chapter thumbnails require configured artwork storage") {
		t.Fatalf("unexpected body: %s", body)
	}
}

// The reported flag drives whether the admin UI enables the switch at all, so it
// has to follow the same store the extraction service writes to.
func TestChapterThumbnailsSupportedFollowsStoreReadiness(t *testing.T) {
	for _, ready := range []bool{true, false} {
		h := &LibraryHandler{ChapterThumbnailStoreReady: ready}
		resp := h.toLibraryResponseWithPoster(t.Context(), &models.MediaFolder{ID: 1, Name: "Movies", Type: "movies"})
		if resp.ChapterThumbnailsSupported != ready {
			t.Errorf("ChapterThumbnailsSupported = %v, want %v", resp.ChapterThumbnailsSupported, ready)
		}
	}
}
