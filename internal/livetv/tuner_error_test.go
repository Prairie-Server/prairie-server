package livetv

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseHDHomeRunError(t *testing.T) {
	cases := []struct {
		header  string
		code    string
		message string
	}{
		{"807 No Video Data", "807", "No Video Data"},
		{"  807   No Video Data  ", "807", "No Video Data"},
		{"802 Resource Locked", "802", "Resource Locked"},
		{"807", "807", ""},
		// A header with no numeric code is all message, so the text still reaches
		// the user instead of being dropped as unparseable.
		{"Something Went Wrong", "", "Something Went Wrong"},
		{"oops", "", "oops"},
		{"", "", ""},
	}
	for _, tc := range cases {
		code, message := parseHDHomeRunError(tc.header)
		if code != tc.code || message != tc.message {
			t.Errorf("parseHDHomeRunError(%q) = (%q, %q), want (%q, %q)",
				tc.header, code, message, tc.code, tc.message)
		}
	}
}

// The whole point is that a viewer sees a reason instead of an ffmpeg exit code,
// and that the API layer can answer with the right status.
func TestTunerRefusalErrorMapping(t *testing.T) {
	// 807 is the reception failure we actually hit on channel 2.1.
	err := tunerRefusalError(http.StatusServiceUnavailable, "807 No Video Data")
	if !errors.Is(err, ErrNoSignal) {
		t.Errorf("807 = %v, want ErrNoSignal", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ErrNoSignal must stay answerable as not-found, got %v", err)
	}

	// Contention maps to the tuner sentinel the API already answers 409 for.
	for _, code := range []string{"802 Resource Locked", "803 Resource Locked"} {
		if err := tunerRefusalError(http.StatusServiceUnavailable, code); !errors.Is(err, ErrNoTuner) {
			t.Errorf("%q = %v, want ErrNoTuner", code, err)
		}
	}

	// An unmapped code still surfaces the tuner's own wording rather than being
	// flattened into a generic failure.
	err = tunerRefusalError(http.StatusServiceUnavailable, "899 Something New")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "Something New") {
		t.Errorf("unmapped code lost its message: %v", err)
	}

	// 503 with no reason header is how older firmware reports contention.
	if err := tunerRefusalError(http.StatusServiceUnavailable, ""); !errors.Is(err, ErrNoTuner) {
		t.Errorf("bare 503 = %v, want ErrNoTuner", err)
	}

	// Any other status still produces something actionable.
	if err := tunerRefusalError(http.StatusForbidden, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("403 = %v, want ErrNotFound", err)
	}
}

// A willing tuner must return nil so the caller keeps its original error --
// misreporting a healthy tuner would hide the real cause.
func TestDescribeTunerRefusalIgnoresHealthyTuner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := DescribeTunerRefusal(t.Context(), srv.Client(), srv.URL); err != nil {
		t.Errorf("healthy tuner reported a refusal: %v", err)
	}
}

func TestDescribeTunerRefusalReadsTheHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdhomerunErrorHeader, "807 No Video Data")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := DescribeTunerRefusal(t.Context(), srv.Client(), srv.URL)
	if !errors.Is(err, ErrNoSignal) {
		t.Fatalf("DescribeTunerRefusal = %v, want ErrNoSignal", err)
	}
	// The message has to be readable, not just typed.
	if !strings.Contains(err.Error(), "no signal") {
		t.Errorf("error is not viewer-readable: %v", err)
	}
}

// An unreachable tuner is itself the explanation, and must not be reported as a
// healthy one.
func TestDescribeTunerRefusalOnUnreachableTuner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	if err := DescribeTunerRefusal(t.Context(), srv.Client(), url); err == nil {
		t.Error("unreachable tuner reported no refusal")
	}
}

func TestDescribeTunerRefusalIgnoresEmptyURL(t *testing.T) {
	if err := DescribeTunerRefusal(t.Context(), nil, "   "); err != nil {
		t.Errorf("empty URL = %v, want nil", err)
	}
}
