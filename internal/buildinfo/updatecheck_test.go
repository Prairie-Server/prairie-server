package buildinfo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdateCheckerEnrichUpdateAvailable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v1.4.0",
			"html_url": "https://github.com/Prairie-Server/prairie-server/releases/tag/v1.4.0",
		})
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "https://example.com/releases")
	checker.ttl = time.Minute

	got := checker.Enrich(context.Background(), Info{
		Display:   "1.0.0",
		Available: true,
		Version:   "1.0.0",
	})
	if got.UpdateStatus != UpdateStatusUpdateAvailable {
		t.Fatalf("UpdateStatus = %q, want %q", got.UpdateStatus, UpdateStatusUpdateAvailable)
	}
	if got.LatestVersion != "1.4.0" {
		t.Fatalf("LatestVersion = %q, want 1.4.0", got.LatestVersion)
	}
	if got.ChangelogURL == "" {
		t.Fatal("expected ChangelogURL")
	}
}

func TestUpdateCheckerEnrichUnknownWithoutVersion(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v1.4.0",
			"html_url": "https://github.com/Prairie-Server/prairie-server/releases/tag/v1.4.0",
		})
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "https://example.com/releases")
	got := checker.Enrich(context.Background(), Info{
		Display:   "b4c5aae1",
		Available: true,
		Revision:  "b4c5aae18aa653725ac697b29a05eac797576008",
	})
	if got.UpdateStatus != UpdateStatusUnknown {
		t.Fatalf("UpdateStatus = %q, want %q", got.UpdateStatus, UpdateStatusUnknown)
	}
	if got.LatestVersion != "1.4.0" {
		t.Fatalf("LatestVersion = %q, want 1.4.0", got.LatestVersion)
	}
}

func TestUpdateCheckerEnrich404(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "https://example.com/releases")
	got := checker.Enrich(context.Background(), Info{Version: "1.0.0", Available: true})
	if got.UpdateStatus != UpdateStatusUnknown {
		t.Fatalf("UpdateStatus = %q, want %q", got.UpdateStatus, UpdateStatusUnknown)
	}
	if got.ChangelogURL != "https://example.com/releases" {
		t.Fatalf("ChangelogURL = %q", got.ChangelogURL)
	}
}

func TestCompareSemver(t *testing.T) {
	t.Parallel()
	if compareSemver("1.4.0", "1.0.0") <= 0 {
		t.Fatal("expected 1.4.0 > 1.0.0")
	}
	if compareSemver("v1.0.0", "1.0.0") != 0 {
		t.Fatal("expected equal")
	}
	if compareSemver("1.0.0", "1.4.0") >= 0 {
		t.Fatal("expected 1.0.0 < 1.4.0")
	}
	if compareSemver("not-a-version", "1.0.0") != 0 {
		t.Fatal("unparseable should compare equal")
	}
	if compareSemver("1.2", "1.2.0") != 0 {
		t.Fatal("missing patch should treat as 0")
	}
	if compareSemver("1", "1.0.0") != 0 {
		t.Fatal("single segment should be unparseable → equal")
	}
}

func TestUpdateCheckerEnrichNilChecker(t *testing.T) {
	t.Parallel()
	var checker *UpdateChecker
	got := checker.Enrich(context.Background(), Info{Version: "1.0.0"})
	if got.UpdateStatus != UpdateStatusUnknown {
		t.Fatalf("UpdateStatus = %q", got.UpdateStatus)
	}
	if got.ChangelogURL != DefaultChangelogURL {
		t.Fatalf("ChangelogURL = %q", got.ChangelogURL)
	}
}

func TestUpdateCheckerEnrichUpToDate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v1.4.0",
			"html_url": "https://github.com/Prairie-Server/prairie-server/releases/tag/v1.4.0",
		})
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "")
	got := checker.Enrich(context.Background(), Info{Version: "1.4.0", Available: true})
	if got.UpdateStatus != UpdateStatusUpToDate {
		t.Fatalf("UpdateStatus = %q, want %q", got.UpdateStatus, UpdateStatusUpToDate)
	}
}

func TestUpdateCheckerEnrichUsesNameAndFallbackChangelog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name": "1.5.0",
		})
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "https://example.com/releases")
	got := checker.Enrich(context.Background(), Info{Version: "1.0.0", Available: true})
	if got.LatestVersion != "1.5.0" {
		t.Fatalf("LatestVersion = %q", got.LatestVersion)
	}
	if got.ChangelogURL != "https://example.com/releases" {
		t.Fatalf("ChangelogURL = %q", got.ChangelogURL)
	}
}

func TestUpdateCheckerFetchNonOKAndBadJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "https://example.com/releases")
	got := checker.Enrich(context.Background(), Info{Version: "1.0.0"})
	if got.UpdateStatus != UpdateStatusUnknown {
		t.Fatalf("UpdateStatus = %q", got.UpdateStatus)
	}

	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	t.Cleanup(badJSON.Close)

	checker2 := NewUpdateChecker(badJSON.URL, "https://example.com/releases")
	got2 := checker2.Enrich(context.Background(), Info{Version: "1.0.0"})
	if got2.UpdateStatus != UpdateStatusUnknown {
		t.Fatalf("UpdateStatus = %q", got2.UpdateStatus)
	}
}

func TestUpdateCheckerUsesCache(t *testing.T) {
	t.Parallel()

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v1.0.0",
			"html_url": "https://example.com/r",
		})
	}))
	t.Cleanup(server.Close)

	checker := NewUpdateChecker(server.URL, "https://example.com/releases")
	checker.ttl = time.Hour
	_ = checker.Enrich(context.Background(), Info{Version: "1.0.0"})
	_ = checker.Enrich(context.Background(), Info{Version: "1.0.0"})
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

func TestBuildInfoWithVersionDisplay(t *testing.T) {
	t.Parallel()
	got := buildInfo("b4c5aae18aa653725ac697b29a05eac797576008", true, "2026-04-05T22:24:40Z", "v1.2.3")
	if got.Display != "1.2.3+dirty" {
		t.Fatalf("Display = %q", got.Display)
	}
	if got.Version != "1.2.3" {
		t.Fatalf("Version = %q", got.Version)
	}

	unavailable := buildInfo("", false, "", "v9.9.9")
	if unavailable.Available {
		t.Fatal("expected unavailable")
	}
	if unavailable.Version != "9.9.9" {
		t.Fatalf("Version = %q", unavailable.Version)
	}
}

func TestParseSemverRejectsNonNumeric(t *testing.T) {
	t.Parallel()
	if _, ok := parseSemver("1.x.0"); ok {
		t.Fatal("expected reject")
	}
	if _, ok := parseSemver("1.2.3-rc.1"); !ok {
		t.Fatal("prerelease core should parse")
	}
}
