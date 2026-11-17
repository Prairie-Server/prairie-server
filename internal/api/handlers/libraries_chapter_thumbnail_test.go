package handlers

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestChapterThumbnailsSupportedFollowsStoreReadiness(t *testing.T) {
	for _, ready := range []bool{true, false} {
		h := &LibraryHandler{ChapterThumbnailStoreReady: ready}
		resp := h.toLibraryResponseWithPoster(t.Context(), &models.MediaFolder{ID: 1, Name: "Movies", Type: "movies"})
		if resp.ChapterThumbnailsSupported != ready {
			t.Errorf("ChapterThumbnailsSupported = %v, want %v", resp.ChapterThumbnailsSupported, ready)
		}
	}
}
