package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func readOverlayConfig(t *testing.T, handler *SettingsHandler) overlayConfigResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.HandleGetOverlayConfig(
		recorder,
		httptest.NewRequest(http.MethodGet, "/settings/overlay-config", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response overlayConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func TestGetOverlayConfigDefaults(t *testing.T) {
	response := readOverlayConfig(t, NewSettingsHandler(nil))
	if !response.Enabled {
		t.Fatal("overlays enabled = false, want true")
	}
	if response.Defaults != "" {
		t.Fatalf("defaults = %q, want empty", response.Defaults)
	}
}

func TestGetOverlayConfigReflectsAdminDefaults(t *testing.T) {
	handler := NewSettingsHandler(nil)
	handler.SetServerSettings(&fakeServerSettingsStore{values: map[string]string{
		"overlays.enabled":       "false",
		"defaults.card_overlays": `{"preset":"classic"}`,
	}})

	response := readOverlayConfig(t, handler)
	if response.Enabled {
		t.Fatal("overlays enabled = true, want false")
	}
	if response.Defaults != `{"preset":"classic"}` {
		t.Fatalf("defaults = %q", response.Defaults)
	}
}
