package envutil

import (
	"os"
	"testing"
)

func TestLookupEnvAliasesBrandPrefixes(t *testing.T) {
	const (
		prairie = "PRAIRIE_ENVUTIL_ALIAS_TEST"
		silo    = "SILO_ENVUTIL_ALIAS_TEST"
	)
	for _, tc := range []struct {
		name          string
		prairie, silo *string
		want          string
		wantSet       bool
	}{
		{name: "neither", want: "", wantSet: false},
		{name: "silo only", silo: new("legacy"), want: "legacy", wantSet: true},
		{name: "prairie only", prairie: new("current"), want: "current", wantSet: true},
		{name: "prairie wins", prairie: new("current"), silo: new("legacy"), want: "current", wantSet: true},
		{name: "empty prairie does not mask silo", prairie: new(""), silo: new("legacy"), want: "legacy", wantSet: true},
		{name: "empty prairie alone is set", prairie: new(""), want: "", wantSet: true},
		{name: "empty silo alone is set", silo: new(""), want: "", wantSet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unset(t, prairie)
			unset(t, silo)
			if tc.prairie != nil {
				t.Setenv(prairie, *tc.prairie)
			}
			if tc.silo != nil {
				t.Setenv(silo, *tc.silo)
			}
			// Either spelling names the same setting.
			for _, name := range []string{silo, prairie} {
				got, set := LookupEnv(name)
				if got != tc.want || set != tc.wantSet {
					t.Fatalf("LookupEnv(%s) = %q, %v; want %q, %v", name, got, set, tc.want, tc.wantSet)
				}
				if got := Getenv(name); got != tc.want {
					t.Fatalf("Getenv(%s) = %q, want %q", name, got, tc.want)
				}
			}
		})
	}
}

func TestLookupEnvLeavesOtherNamesAlone(t *testing.T) {
	t.Setenv("ENVUTIL_ALIAS_PLAIN", "plain")
	t.Setenv("PRAIRIE_ENVUTIL_ALIAS_PLAIN", "branded")
	if got := Getenv("ENVUTIL_ALIAS_PLAIN"); got != "plain" {
		t.Fatalf("unbranded name = %q, want plain", got)
	}
	for _, name := range []string{"PRAIRIE_", "SILO_"} {
		if _, set := LookupEnv(name); set {
			t.Fatalf("bare prefix %q resolved as set", name)
		}
	}
}

func TestParsersReadThroughTheAlias(t *testing.T) {
	unset(t, testEnv)
	t.Setenv("PRAIRIE_ENVUTIL_TEST_FLAG", "on")
	if !Bool(testEnv) || !IsSet(testEnv) || !BoolDefault(testEnv, false) {
		t.Fatal("PRAIRIE_ spelling not honored for a SILO_ flag")
	}
	if got := FirstNonEmpty("SILO_ENVUTIL_TEST_MISSING", testEnv); got != "on" {
		t.Fatalf("FirstNonEmpty = %q, want on", got)
	}
}

// unset clears name for the test, restoring it afterwards.
func unset(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}
