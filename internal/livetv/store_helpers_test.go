package livetv

import "testing"

func TestNonNilStringSlice(t *testing.T) {
	t.Parallel()
	if got := nonNilStringSlice(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil -> empty, got %#v", got)
	}
	in := []string{"News"}
	got := nonNilStringSlice(in)
	if len(got) != 1 || got[0] != "News" {
		t.Fatalf("passthrough = %#v", got)
	}
}
