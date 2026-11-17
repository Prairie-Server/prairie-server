package titleutil

import "testing"

func TestIsDefaultSortTitle(t *testing.T) {
	tests := []struct {
		sortTitle string
		want      bool
	}{
		{"Office, The", true},
		{"Beautiful Mind, A", true},
		{"Inconvenient Truth, An", true},
		{"Office", false},
		{", The", false},
		{"   , An", false},
		{"Office, the", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsDefaultSortTitle(tt.sortTitle); got != tt.want {
			t.Errorf("IsDefaultSortTitle(%q) = %v, want %v", tt.sortTitle, got, tt.want)
		}
	}
}

func TestIsDefaultSortTitleMatchesDerived(t *testing.T) {
	for _, title := range []string{"The Office", "A Beautiful Mind", "An Inconvenient Truth"} {
		if derived := DeriveDefaultSortTitle(title); !IsDefaultSortTitle(derived) {
			t.Errorf("IsDefaultSortTitle(DeriveDefaultSortTitle(%q) = %q) = false", title, derived)
		}
	}
}
