package titleutil

import "testing"

func TestIsProvisionalEpisodeTitle(t *testing.T) {
	for title, want := range map[string]bool{"TBA": true, " tbd ": true, "Pilot": false, "": false} {
		if got := IsProvisionalEpisodeTitle(title); got != want {
			t.Errorf("IsProvisionalEpisodeTitle(%q) = %v, want %v", title, got, want)
		}
	}
}

func TestIsNumberedEpisodeTitle(t *testing.T) {
	for _, tt := range []struct {
		title  string
		number int
		want   bool
	}{
		{"Episode 3", 3, true},
		{"Avsnitt 3", 3, true},
		{"3. Bölüm", 3, true},
		{"Épisode 3", 3, true},
		{"第 3 集", 3, true},
		{"\u202bقسمت 3", 3, true},
		{"Episode 4", 3, false},
		{"Day 3", 3, false},
		{"Part 3", 3, false},
		{"Chapter 3: Fire", 3, false},
		{"第３話", 3, false},
		{"Mob and Reigen", 3, false},
	} {
		if got := IsNumberedEpisodeTitle(tt.title, tt.number); got != tt.want {
			t.Errorf("IsNumberedEpisodeTitle(%q, %d) = %v, want %v", tt.title, tt.number, got, tt.want)
		}
	}
}
