package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireProfileAcceptsHeaderAndQuery(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetProfileID(r.Context()); got != "prof-1" {
			t.Fatalf("profile=%q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequireProfile(next)

	t.Run("header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-Profile-Id", "prof-1")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x?profile_id=prof-1", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", rr.Code)
		}
	})
}
