package buildinfo

import "testing"

func TestCurrentSmoke(t *testing.T) {
	info := Current()
	if info.Display == "" {
		t.Fatal("Display should never be empty")
	}
	// In tests, VCS metadata may or may not be present; just exercise the path.
	_ = info.Available
	_ = info.Revision
	_ = info.Dirty
	_ = info.VCSTime
}
