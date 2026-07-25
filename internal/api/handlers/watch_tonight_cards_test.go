package handlers

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/recommendations"
)

func TestRecommendationCardItemsPreservesRankedOrder(t *testing.T) {
	candidates := []recommendations.ScoredItem{
		{MediaItemID: "multi-genre-match", Score: 0.4},
		{MediaItemID: "excluded", Score: 0.99},
		{MediaItemID: "single-genre-high-score", Score: 0.8},
		{MediaItemID: "single-genre-low-score", Score: 0.2},
	}
	excluded := map[string]struct{}{"excluded": {}}

	cards := recommendationCardItems(candidates, excluded)

	got := make([]string, len(cards))
	for i, card := range cards {
		got[i] = card.scored.MediaItemID
	}
	want := []string{"multi-genre-match", "single-genre-high-score", "single-genre-low-score"}
	if len(got) != len(want) {
		t.Fatalf("got %d cards, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("card order = %#v, want %#v", got, want)
		}
	}
}
