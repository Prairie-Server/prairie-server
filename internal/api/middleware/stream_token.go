package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/prairie-server/prairie-server/internal/streamtoken"
)

// streamTokenAuthorizedKey marks a request as authorized by a signed stream
// token rather than by a session bearer.
const streamTokenAuthorizedKey contextKey = "stream_token_authorized"

// streamTokenQueryParam is the query parameter carrying the signed stream token.
// Mirrors handlers.streamTokenParam.
const streamTokenQueryParam = "st"

// HLS delivery lives at:
//
//	/api/v1/playback/transcode/{session_id}/master.m3u8
//	/api/v1/playback/transcode/{session_id}/segment/{name}
//
// Matched on the raw path rather than chi URL params on purpose: middleware
// registered on the parent group runs before the inner route is matched, so
// chi.URLParam is still empty here. (That is also why a rejected request logs
// path_pattern=/api/v1/playback/* instead of the real pattern.)
const transcodeDeliveryPrefix = "/playback/transcode/"

// streamTokenDeliverySession returns the session ID a request is asking to be
// delivered, and whether the path is an HLS delivery path at all.
//
// Only the two delivery shapes qualify. Everything else — including the
// mutation routes under /playback — keeps requiring a session bearer, so a
// stream token can never be spent as a general-purpose credential.
func streamTokenDeliverySession(urlPath string) (string, bool) {
	idx := strings.Index(urlPath, transcodeDeliveryPrefix)
	if idx < 0 {
		return "", false
	}
	rest := urlPath[idx+len(transcodeDeliveryPrefix):]
	sessionID, tail, found := strings.Cut(rest, "/")
	if !found || sessionID == "" {
		return "", false
	}
	switch {
	case tail == "master.m3u8":
		return sessionID, true
	case strings.HasPrefix(tail, "segment/") && len(tail) > len("segment/"):
		// A segment name may not climb out of the segment namespace.
		if strings.Contains(strings.TrimPrefix(tail, "segment/"), "/") {
			return "", false
		}
		return sessionID, true
	default:
		return "", false
	}
}

// StreamTokenAuth authorizes HLS delivery requests that carry a valid signed
// stream token, so the URL is self-authorizing as designed.
//
// The player, not the app, fetches the playlist and every segment. On a TV that
// is AVPlay, which cannot attach an Authorization header or refresh an expired
// access token — which is why the server mints "st" (a session-scoped, signed
// descriptor) and threads it through manifests in the first place. Until now
// nothing at the auth layer honored it: RequireAuth ran first and rejected the
// request before the handler ever read the token, so delivery depended on an
// access token pasted into the URL. A stale or empty one produced an endless
// 401 poll that looks exactly like a transcode that never becomes ready.
//
// This middleware only ever *adds* authorization: it marks the request and lets
// RequireAuth and RequireViewerAccess skip it. A request that already passes
// bearer auth is untouched, so no currently-working request changes behavior;
// only requests that would have 401'd can now succeed, and only for the two
// delivery paths, and only with a signature that names their own session.
func (am *AuthMiddleware) StreamTokenAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if secret == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get(streamTokenQueryParam)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			sessionID, ok := streamTokenDeliverySession(r.URL.Path)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := streamtoken.Verify(token, secret)
			// Binding the signature to the session in the path is the whole
			// authorization: a token minted for one session must not fetch
			// another's bytes. Verify already rejects a bad signature or an
			// expired token.
			if err != nil || claims == nil || claims.SessionID == "" || claims.SessionID != sessionID {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), streamTokenAuthorizedKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsStreamTokenAuthorized reports whether a verified, session-bound stream
// token already authorized this request.
//
// Deliberately carries no identity. The stream token's ownership claims
// (uid/pid/mfid) are lookup keys re-resolved against the authority on
// reconstruct and are never trusted on their own, so this grants exactly one
// thing — delivery of the named session's bytes — and never a user or profile.
func IsStreamTokenAuthorized(ctx context.Context) bool {
	authorized, _ := ctx.Value(streamTokenAuthorizedKey).(bool)
	return authorized
}
