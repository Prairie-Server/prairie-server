package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/streamtoken"
)

const testStreamSecret = "test-stream-secret"

func signStreamToken(t *testing.T, sessionID string, ttl time.Duration) string {
	t.Helper()
	token, err := streamtoken.Sign(streamtoken.Claims{SessionID: sessionID}, testStreamSecret, ttl)
	if err != nil {
		t.Fatalf("signing stream token: %v", err)
	}
	return token
}

// serveWithStreamTokenAuth runs the middleware and reports whether the request
// came out marked as stream-token authorized.
func serveWithStreamTokenAuth(t *testing.T, secret, target string) bool {
	t.Helper()
	var authorized bool
	handler := (&AuthMiddleware{}).StreamTokenAuth(secret)(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			authorized = IsStreamTokenAuthorized(r.Context())
		},
	))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	return authorized
}

func TestStreamTokenAuthAuthorizesDeliveryPaths(t *testing.T) {
	const session = "f1e5e994-29eb-4b2c-8a8f-c75c005baf12"
	token := signStreamToken(t, session, time.Hour)

	for _, target := range []string{
		"/api/v1/playback/transcode/" + session + "/master.m3u8?st=" + token,
		// The player follows the master's variant URI itself, so this fetch
		// carries the stream token and no bearer. It must authorize like the
		// master does.
		"/api/v1/playback/transcode/" + session + "/media.m3u8?st=" + token,
		"/api/v1/playback/transcode/" + session + "/segment/seg-00001.m4s?st=" + token,
	} {
		if !serveWithStreamTokenAuth(t, testStreamSecret, target) {
			t.Errorf("%s was not authorized by a valid session-bound stream token", target)
		}
	}
}

// The token must only ever buy delivery of the session it names. A token for
// session A fetching session B's bytes is the one thing binding prevents.
func TestStreamTokenAuthRejectsCrossSessionToken(t *testing.T) {
	token := signStreamToken(t, "session-a", time.Hour)
	target := "/api/v1/playback/transcode/session-b/master.m3u8?st=" + token
	if serveWithStreamTokenAuth(t, testStreamSecret, target) {
		t.Fatal("a token minted for session-a authorized delivery of session-b")
	}
}

func TestStreamTokenAuthRejectsCrossSessionTokenOnMediaPlaylist(t *testing.T) {
	token := signStreamToken(t, "session-a", time.Hour)
	if serveWithStreamTokenAuth(t, testStreamSecret,
		"/api/v1/playback/transcode/session-b/media.m3u8?st="+token) {
		t.Fatal("a token for session-a authorized session-b's media playlist")
	}
}

func TestStreamTokenAuthRejectsBadSignatureAndExpiry(t *testing.T) {
	const session = "session-1"
	base := "/api/v1/playback/transcode/" + session + "/master.m3u8?st="

	if serveWithStreamTokenAuth(t, testStreamSecret, base+"not-a-jwt") {
		t.Error("a malformed token authorized delivery")
	}
	// Signed with a different secret: right shape, wrong signer.
	foreign, err := streamtoken.Sign(streamtoken.Claims{SessionID: session}, "other-secret", time.Hour)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	if serveWithStreamTokenAuth(t, testStreamSecret, base+foreign) {
		t.Error("a token signed with a foreign secret authorized delivery")
	}
	if serveWithStreamTokenAuth(t, testStreamSecret, base+signStreamToken(t, session, -time.Minute)) {
		t.Error("an expired token authorized delivery")
	}
}

// A stream token is not a general-purpose credential: it must not unlock any
// route other than the two delivery shapes, or the bypass becomes an escalation.
func TestStreamTokenAuthOnlyAppliesToDeliveryPaths(t *testing.T) {
	const session = "session-1"
	token := signStreamToken(t, session, time.Hour)

	for _, target := range []string{
		"/api/v1/admin/users?st=" + token,
		"/api/v1/playback/start?st=" + token,
		"/api/v1/playback/" + session + "/progress?st=" + token,
		"/api/v1/playback/transcode/" + session + "?st=" + token,
		"/api/v1/playback/transcode/" + session + "/other.m3u8?st=" + token,
		"/api/v1/playback/transcode/" + session + "/segment/?st=" + token,
		// Must not climb out of the segment namespace.
		"/api/v1/playback/transcode/" + session + "/segment/../../admin?st=" + token,
	} {
		if serveWithStreamTokenAuth(t, testStreamSecret, target) {
			t.Errorf("%s was authorized by a stream token; only HLS delivery paths may be", target)
		}
	}
}

func TestStreamTokenAuthInertWithoutTokenOrSecret(t *testing.T) {
	const session = "session-1"
	token := signStreamToken(t, session, time.Hour)
	delivery := "/api/v1/playback/transcode/" + session + "/master.m3u8"

	if serveWithStreamTokenAuth(t, testStreamSecret, delivery) {
		t.Error("a delivery request with no stream token was authorized")
	}
	// No configured secret must not mean "verify nothing".
	if serveWithStreamTokenAuth(t, "", delivery+"?st="+token) {
		t.Error("stream token accepted while no signing secret is configured")
	}
}

// The parameter name is a wire contract shared by the handler that appends it,
// this middleware that authorizes on it, and the manifest rewriter that threads
// it into segment URIs. Pin the value so a rename cannot silently stop the
// player's URLs from authorizing.
func TestStreamTokenQueryParamIsTheSharedContract(t *testing.T) {
	if streamtoken.QueryParam != "st" {
		t.Fatalf("streamtoken.QueryParam = %q, want \"st\" — existing clients and manifests carry this name", streamtoken.QueryParam)
	}
	const session = "session-1"
	target := "/api/v1/playback/transcode/" + session + "/master.m3u8?" +
		streamtoken.QueryParam + "=" + signStreamToken(t, session, time.Hour)
	if !serveWithStreamTokenAuth(t, testStreamSecret, target) {
		t.Error("a token presented under streamtoken.QueryParam was not honored")
	}
}

func TestStreamTokenDeliverySessionParsing(t *testing.T) {
	for target, want := range map[string]string{
		"/api/v1/playback/transcode/abc/master.m3u8":       "abc",
		"/api/v1/playback/transcode/abc/media.m3u8":        "abc",
		"/api/v1/playback/transcode/abc/segment/seg-1.m4s": "abc",
		"/playback/transcode/abc/master.m3u8":              "abc",
		"/api/v1/playback/transcode//master.m3u8":          "",
		"/api/v1/playback/transcode/abc":                   "",
		"/api/v1/playback/transcode/abc/":                  "",
		"/api/v1/playback/transcode/abc/segment":           "",
		"/api/v1/playback/transcode/abc/segment/a/b":       "",
		"/api/v1/playback/transcode/abc/master.m3u8/extra": "",
		// Progressive (direct-play / remux) delivery. Previously excluded because
		// this middleware was HLS-only, which 401'd every native-player fetch of
		// the stream_url the server itself had signed.
		"/api/v1/stream/abc": "abc",
		// chi trims the mount prefix for sub-routers, so the middleware sees this
		// shape too. Requiring the API prefix here 401'd every real request.
		"/stream/abc": "abc",
		// Sub-resources stay out: a stream token authorizes the session's media
		// bytes and nothing wider.
		"/api/v1/stream/abc/subtitles/2": "",
		// Unanchored matching would let any route containing "/stream/" through.
		"/api/v1/playback/transcode/abc/stream/evil": "",
	} {
		got, ok := streamTokenDeliverySession(target)
		if want == "" {
			if ok {
				t.Errorf("streamTokenDeliverySession(%q) matched, want no match (got %q)", target, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("streamTokenDeliverySession(%q) = (%q, %v), want (%q, true)", target, got, ok, want)
		}
	}
}

// RequireAuth must honor the marker, since that is what actually unblocks the
// player, and must still reject an unmarked request with no bearer.
func TestRequireAuthHonorsStreamTokenMarker(t *testing.T) {
	am := &AuthMiddleware{}
	const session = "session-1"
	target := "/api/v1/playback/transcode/" + session + "/master.m3u8?st=" + signStreamToken(t, session, time.Hour)

	var reached bool
	chain := am.StreamTokenAuth(testStreamSecret)(am.RequireAuth(http.HandlerFunc(
		func(_ http.ResponseWriter, _ *http.Request) { reached = true },
	)))

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if !reached || rec.Code != http.StatusOK {
		t.Fatalf("stream-token delivery blocked by RequireAuth: status=%d reached=%v", rec.Code, reached)
	}

	// Same path, no token: still needs a bearer.
	reached = false
	rec = httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/playback/transcode/"+session+"/master.m3u8", nil))
	if reached || rec.Code != http.StatusUnauthorized {
		t.Fatalf("tokenless delivery reached the handler: status=%d reached=%v", rec.Code, reached)
	}
}

func TestRequireViewerAccessHonorsStreamTokenMarker(t *testing.T) {
	// A nil resolver would panic if the skip did not short-circuit first, which
	// also proves no viewer scope is resolved on this path.
	m := &ViewerAccessMiddleware{}
	const session = "session-1"
	target := "/api/v1/playback/transcode/" + session + "/master.m3u8?st=" + signStreamToken(t, session, time.Hour)

	var reached bool
	chain := (&AuthMiddleware{}).StreamTokenAuth(testStreamSecret)(m.RequireViewerAccess(http.HandlerFunc(
		func(_ http.ResponseWriter, _ *http.Request) { reached = true },
	)))

	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if !reached || rec.Code != http.StatusOK {
		t.Fatalf("stream-token delivery blocked by viewer access: status=%d reached=%v", rec.Code, reached)
	}
}

// The progressive endpoint had a token minted for it all along --
// playbackStreamURL appends one to every non-transcode stream_url -- but nothing
// honored it here, so a native player following that URL got a 401 on every
// attempt and surfaced it as a connection failure.
func TestStreamTokenDeliverySessionAcceptsProgressiveStream(t *testing.T) {
	sessionID, ok := streamTokenDeliverySession("/api/v1/stream/abc-123")
	if !ok || sessionID != "abc-123" {
		t.Fatalf("streamTokenDeliverySession(progressive) = (%q, %v), want (abc-123, true)", sessionID, ok)
	}
}

// A stream token authorizes the session's media bytes and nothing wider. The
// sub-resources are excluded on purpose, so widening that is a deliberate
// decision rather than a side effect of prefix matching.
func TestStreamTokenDeliverySessionRejectsProgressiveSubResources(t *testing.T) {
	for _, path := range []string{
		"/api/v1/stream/abc-123/subtitles/2",
		"/api/v1/stream/abc-123/subtitles/2/fonts",
		"/api/v1/stream/",
		"/api/v1/stream/abc-123/",
	} {
		if sessionID, ok := streamTokenDeliverySession(path); ok {
			t.Errorf("streamTokenDeliverySession(%q) = (%q, true), want no session", path, sessionID)
		}
	}
}

// Anchoring matters: an unanchored search for "/stream/" would also match a
// nested segment of some other route and quietly widen what a token authorizes.
func TestStreamTokenDeliverySessionRejectsNestedStreamSegment(t *testing.T) {
	for _, path := range []string{
		"/api/v1/playback/transcode/sess-1/stream/evil",
		"/api/v2/stream/abc-123",
		"/internal/stream/abc-123",
	} {
		if sessionID, ok := streamTokenDeliverySession(path); ok {
			t.Errorf("streamTokenDeliverySession(%q) = (%q, true), want no session", path, sessionID)
		}
	}
}
