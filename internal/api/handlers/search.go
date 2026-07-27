package handlers

import (
	"net/http"
	"strings"

	"github.com/prairie-server/prairie-server/internal/catalog"
)

// SearchHandler handles the search endpoint.
type SearchHandler struct {
	itemRepo *catalog.ItemRepository
	itemsH   *ItemsHandler // reuse toItemListResponse
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(
	itemRepo *catalog.ItemRepository,
	itemsH *ItemsHandler,
) *SearchHandler {
	return &SearchHandler{
		itemRepo: itemRepo,
		itemsH:   itemsH,
	}
}

// HandleSearch handles GET /search?q=...&limit=...&offset=...
func (h *SearchHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	writeDeprecatedReadHeaders(w, "/api/v1/catalog")
	if h == nil || h.itemsH == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Catalog is not configured")
		return
	}
	NewCatalogHandler(h.itemsH.catalogResolver, h.itemsH).HandleLegacySearch(w, r)
}

func parseSearchTypes(rawValues []string) []string {
	if len(rawValues) == 0 {
		return nil
	}

	seen := map[string]bool{}
	result := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		for part := range strings.SplitSeq(raw, ",") {
			normalized := strings.ToLower(strings.TrimSpace(part))
			switch normalized {
			case itemTypeMovie, itemTypeSeries, itemTypeSeason, itemTypeEpisode:
			default:
				continue
			}
			if seen[normalized] {
				continue
			}
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	return result
}
