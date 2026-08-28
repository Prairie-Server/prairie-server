package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/config"
)

// A server without object storage (S3) configured previously dropped the
// entire "/subtitles" route tree — including the AI capability probe and its
// intended graceful-negative fallback (WriteSubtitleAIDisabledStatus) — because
// both lived inside the subtitleSearchHandler-only block, which requires S3 for
// downloaded-subtitle storage. That left GET /api/v1/subtitles/ai/status
// returning a bare 404 instead of the "not configured" response the web
// player's subtitle menu expects, which broke opening/using subtitle track
// selection and settings in the player for any server without S3 wired up.
//
// The AI status probe must be reachable independently of subtitleSearchHandler,
// matching its "/api/v1/metadata/ai/status" sibling (which never had this bug).
func TestSubtitleAIStatusRouteRegisteredWithoutObjectStorage(t *testing.T) {
	// A pool that never actually connects is enough: NewRouter only checks
	// deps.DB != nil for this wiring, and pgxpool connections are lazy.
	pool, err := pgxpool.New(context.Background(), "postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	// DB present, S3Public intentionally left nil (no object storage
	// configured) — this used to make subtitleSearchHandler nil and, as a
	// side effect, take the whole /subtitles route tree down with it.
	r := NewRouter(Dependencies{
		DB:         pool,
		AppContext: context.Background(),
		Config:     &config.Config{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subtitles/ai/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// A generic 404 means the route was never registered at all (the bug).
	// 401 means the route matched and only auth is missing, i.e. the same
	// behavior as the unaffected "/api/v1/metadata/ai/status" sibling below.
	if rec.Code == http.StatusNotFound {
		t.Fatalf(
			"GET /api/v1/subtitles/ai/status = 404 without object storage configured; "+
				"want the route registered (401 without auth) like /api/v1/metadata/ai/status; body=%s",
			rec.Body.String(),
		)
	}

	siblingReq := httptest.NewRequest(http.MethodGet, "/api/v1/metadata/ai/status", nil)
	siblingRec := httptest.NewRecorder()
	r.ServeHTTP(siblingRec, siblingReq)

	if rec.Code != siblingRec.Code {
		t.Fatalf(
			"GET /api/v1/subtitles/ai/status = %d, want it to match its always-registered sibling "+
				"/api/v1/metadata/ai/status = %d",
			rec.Code, siblingRec.Code,
		)
	}
}
