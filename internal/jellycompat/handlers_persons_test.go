package jellycompat

import "testing"

func TestLooksLikePersonName_AcceptsRealNames(t *testing.T) {
	t.Parallel()
	cases := []string{"Christopher Nolan", "Tom Hanks", "Émilie Dequenne", "O'Brien"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			if !looksLikePersonName(name) {
				t.Errorf("expected %q to look like a person name", name)
			}
		})
	}
}

func TestLooksLikePersonName_RejectsMediaTitles(t *testing.T) {
	t.Parallel()
	cases := []string{"Avatar 2", "Star Wars: A New Hope", "1917"}
	for _, q := range cases {
		q := q
		t.Run(q, func(t *testing.T) {
			if looksLikePersonName(q) {
				t.Errorf("expected %q to NOT look like a person name", q)
			}
		})
	}
}

func TestLooksLikePersonName_EdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		term string
		want bool
	}{
		// Punctuation in names — accept
		{"hyphenated", "Jean-Luc Picard", true},
		{"apostrophe", "O'Brien", true},
		{"period after initial", "Robert J. Oppenheimer", true},

		// Unicode — accept
		{"accents", "Émilie Dequenne", true},
		{"diacritic", "Pelé", true},
		{"cjk", "李娜", true},

		// Length guard
		{"single char", "A", false},
		{"empty", "", false},
		{"whitespace only", "   ", false},

		// Trim leading/trailing whitespace
		{"leading whitespace", "   Tom Hanks", true},
		{"trailing whitespace", "Tom Hanks   ", true},

		// Forms that should fall through to media probe
		{"ampersand", "Day & Night", false},
		{"slash", "Either/Or", false},
		{"digit anywhere", "Apollo 13", false},
		{"colon", "Star Wars: A New Hope", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikePersonName(c.term); got != c.want {
				t.Errorf("looksLikePersonName(%q) = %v; want %v", c.term, got, c.want)
			}
		})
	}
}
