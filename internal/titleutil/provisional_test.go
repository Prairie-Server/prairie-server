package titleutil

import "testing"

func TestIsProvisionalEpisodeTitle(t *testing.T) {
	for title, want := range map[string]bool{"TBA": true, " tbd ": true, "Pilot": false, "": false} {
		if got := IsProvisionalEpisodeTitle(title); got != want {
			t.Errorf("IsProvisionalEpisodeTitle(%q) = %v, want %v", title, got, want)
		}
	}
}
