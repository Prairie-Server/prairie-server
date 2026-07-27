package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/buildinfo"
)

func TestSystemBuildInfoResponse(t *testing.T) {
	t.Parallel()

	handler := &SystemHandler{
		buildInfo: buildinfo.Info{
			Display:   "b4c5aae1+dirty",
			Revision:  "b4c5aae18aa653725ac697b29a05eac797576008",
			Dirty:     true,
			VCSTime:   "2026-04-05T22:24:40Z",
			Available: true,
		},
	}

	router := chi.NewRouter()
	router.Get("/admin/system/build", handler.HandleBuildInfo)

	req := httptest.NewRequest(http.MethodGet, "/admin/system/build", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got buildinfo.Info
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	want := handler.buildInfo
	want.UpdateStatus = buildinfo.UpdateStatusUnknown
	want.ChangelogURL = "https://github.com/Prairie-Server/prairie-server/releases"
	if got != want {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestSystemBuildInfoUnavailableResponseShape(t *testing.T) {
	t.Parallel()

	handler := &SystemHandler{
		buildInfo: buildinfo.Info{
			Display:   "unavailable",
			Revision:  "",
			Dirty:     false,
			VCSTime:   "",
			Available: false,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/system/build", nil)
	rec := httptest.NewRecorder()
	handler.HandleBuildInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding raw response: %v", err)
	}

	expected := map[string]any{
		"display":       "unavailable",
		"revision":      "",
		"dirty":         false,
		"vcs_time":      "",
		"available":     false,
		"update_status": "unknown",
		"changelog_url": "https://github.com/Prairie-Server/prairie-server/releases",
	}

	for key, want := range expected {
		if got, ok := raw[key]; !ok || got != want {
			t.Fatalf("response[%q] = %#v (present=%v), want %#v", key, got, ok, want)
		}
	}
}

func TestSystemBuildInfoEnrichesUpdateStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v1.4.0",
			"html_url": "https://github.com/Prairie-Server/prairie-server/releases/tag/v1.4.0",
		})
	}))
	t.Cleanup(server.Close)

	handler := &SystemHandler{
		buildInfo: buildinfo.Info{
			Display:   "1.0.0",
			Revision:  "abc123",
			Available: true,
			Version:   "1.0.0",
		},
		updateChecker: buildinfo.NewUpdateChecker(server.URL, "https://example.com/releases"),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/system/build", nil)
	rec := httptest.NewRecorder()
	handler.HandleBuildInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got buildinfo.Info
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.UpdateStatus != buildinfo.UpdateStatusUpdateAvailable {
		t.Fatalf("UpdateStatus = %q, want %q", got.UpdateStatus, buildinfo.UpdateStatusUpdateAvailable)
	}
	if got.LatestVersion != "1.4.0" {
		t.Fatalf("LatestVersion = %q, want 1.4.0", got.LatestVersion)
	}
	if got.ChangelogURL == "" {
		t.Fatal("expected ChangelogURL")
	}
}
