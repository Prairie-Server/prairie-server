package deviceclass

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromPlatform(t *testing.T) {
	for platform, want := range map[string]Class{
		"smarttv":   TV,
		"SmartTV":   TV,
		"  tizen  ": TV,
		"webos":     TV,
		"androidtv": TV,
		"tvos":      TV,
		"web":       Unknown,
		"ios":       Unknown,
		"android":   Unknown,
		"":          Unknown,
	} {
		if got := FromPlatform(platform); got != want {
			t.Errorf("FromPlatform(%q) = %q, want %q", platform, got, want)
		}
	}
}

// Substring matching would opt a desktop screen into television artwork; the
// platform set is exact on purpose.
func TestFromPlatformDoesNotMatchSubstrings(t *testing.T) {
	for _, platform := range []string{"tv-companion", "smarttvish", "nottv", "web-tv-remote"} {
		if got := FromPlatform(platform); got != Unknown {
			t.Errorf("FromPlatform(%q) = %q, want Unknown", platform, got)
		}
	}
}

func TestMiddlewareStoresClass(t *testing.T) {
	var seen Class
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home/sections", nil)
	req.Header.Set(PlatformHeader, "smarttv")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if seen != TV {
		t.Fatalf("device class = %q, want %q", seen, TV)
	}
}

func TestMiddlewareLeavesUnknownClientsAlone(t *testing.T) {
	var isTV bool
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		isTV = IsTV(r.Context())
	}))

	// No header at all: must behave exactly as before this package existed.
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if isTV {
		t.Fatal("a client that sends no platform header must not be treated as a TV")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(PlatformHeader, "web")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if isTV {
		t.Fatal("web must not be treated as a TV")
	}
}

func TestFromContextAndIsTV(t *testing.T) {
	if got := FromContext(context.Background()); got != Unknown {
		t.Fatalf("bare context = %q, want Unknown", got)
	}
	//nolint:staticcheck // deliberately exercising the nil-context guard
	if got := FromContext(nil); got != Unknown {
		t.Fatalf("nil context = %q, want Unknown", got)
	}
	if !IsTV(SetContext(context.Background(), TV)) {
		t.Fatal("IsTV should report true for a TV context")
	}
	if IsTV(SetContext(context.Background(), Unknown)) {
		t.Fatal("IsTV should report false for Unknown")
	}
}
