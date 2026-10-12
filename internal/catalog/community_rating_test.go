package catalog

import "testing"

func TestNewCommunityRating(t *testing.T) {
	if got := NewCommunityRating(5, minCommunityRatings-1); got != nil {
		t.Fatalf("below threshold = %#v, want nil", got)
	}
	got := NewCommunityRating(25.0/6, 6) // 4.1666...
	if got == nil || got.Average != 4.2 || got.Count != 6 {
		t.Fatalf("got %#v, want {4.2 6}", got)
	}
	if got := NewCommunityRating(4, minCommunityRatings); got == nil || got.Average != 4 {
		t.Fatalf("at threshold = %#v, want {4 3}", got)
	}
}
