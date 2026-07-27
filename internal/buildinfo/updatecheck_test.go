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
}
