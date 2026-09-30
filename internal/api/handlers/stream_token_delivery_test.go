package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "github.com/prairie-server/prairie-server/internal/api/middleware"
	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/playback"
	"github.com/prairie-server/prairie-server/internal/streamtoken"
)

// A native TV player fetches the plan's /api/v2/stream/{id}?st=… URL with no
// bearer. The session-bound stream token must authorize those bytes exactly as
// it does the HLS manifest and segments; before this, every direct-play and
// progressive-remux start on Tizen 401'd and fell back to an encode.
func TestHandleStream_StreamTokenAuthorizesAnonymousDelivery(t *testing.T) {
	const secret = "test-stream-token-secret"
	filePath := writePlaybackTestMediaFile(t, "movie.mp4")
	file := &models.MediaFile{ID: 42, ContentID: "movie-1", FilePath: filePath, Duration: 3600}
	sessionMgr := playback.NewSessionManager(0, 0)
	session, err := sessionMgr.StartSession(1, "profile-1", 42, playback.PlayDirect, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	handler := NewStreamHandler(sessionMgr, testPlaybackFileResolver{file: file})
	handler.JWTSecret = secret
	gated := (&apimw.AuthMiddleware{}).StreamTokenAuth(secret)(http.HandlerFunc(handler.HandleStream))

	serve := func(token string) *httptest.ResponseRecorder {
		t.Helper()
		target := "/api/v2/stream/" + session.ID
		if token != "" {
			target += "?st=" + url.QueryEscape(token)
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("session_id", session.ID)
		req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, routeCtx))
		rr := httptest.NewRecorder()
		gated.ServeHTTP(rr, req)
		return rr
	}

	if rr := serve(""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("no credential: status = %d, want 401", rr.Code)
	}

	other, err := streamtoken.Sign(streamtoken.Claims{SessionID: "another-session"}, secret, time.Hour)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if rr := serve(other); rr.Code != http.StatusUnauthorized {
		t.Fatalf("token for another session: status = %d, want 401", rr.Code)
	}

	token, err := streamtoken.Sign(streamtoken.Claims{SessionID: session.ID}, secret, time.Hour)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if rr := serve(token); rr.Code != http.StatusOK {
		t.Fatalf("session-bound token: status = %d, body = %s", rr.Code, rr.Body.String())
	}
}
