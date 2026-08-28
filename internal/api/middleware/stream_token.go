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

// HLS delivery lives at:
//
//	/api/v1/playback/transcode/{session_id}/master.m3u8
//	/api/v1/playback/transcode/{session_id}/media.m3u8
//	/api/v1/playback/transcode/{session_id}/segment/{name}
//
// Matched on the raw path rather than chi URL params on purpose: middleware
// registered on the parent group runs before the inner route is matched, so
// chi.URLParam is still empty here. (That is also why a rejected request logs
// path_pattern=/api/v1/playback/* instead of the real pattern.)
const transcodeDeliveryPrefix = "/playback/transcode/"

// Progressive (direct-play / remux) delivery lives at:
//
//	/api/v1/stream/{session_id}
//
// A different prefix entirely, which is why it needs its own case: the token was
// already minted for it (playbackStreamURL appends one to every non-transcode
// stream_url) but nothing here honored it, so a native player following that URL
// got a 401 on every attempt and reported a connection failure.
const progressiveDeliveryPrefix = "/stream/"

// apiVersionPrefix anchors the progressive match so "/stream/" is only honored
// at the top level of the API, never as a nested segment of another route.
const apiVersionPrefix = "/api/v1"

// Live TV HLS delivery lives at:
//
//	/api/v1/livetv/live-hls/{playback_session_id}/index.m3u8
//	/api/v1/livetv/live-hls/{playback_session_id}/seg_00000.ts
//
// Same disease as the progressive path above, one route later. The app embeds
// its own access token in this URL and hands it to the native player, which
// cannot refresh or retry -- so the moment that token goes stale the player
// polls 401 forever while the app's own requests sail through on refresh. A live
// session outlives a short access token by design, which makes a rotating
// bearer the wrong credential for it.
const liveHLSDeliveryPrefix = "/livetv/live-hls/"

// streamTokenDeliverySession returns the session ID a request is asking to be
// delivered, and whether the path is a media delivery path at all.
//
// Only these delivery shapes qualify. Everything else — including the mutation
// routes under /playback — keeps requiring a session bearer, so a stream token
// can never be spent as a general-purpose credential.
//
// media.m3u8 has to be here for the same reason master.m3u8 is: the player
// follows the master's variant URI itself, so that fetch carries the stream
// token and no bearer. Omitting it 401s every variant fetch, which is exactly
// what happened when the master playlist landed without this list being updated.
func streamTokenDeliverySession(urlPath string) (string, bool) {
	if sessionID, ok := transcodeDeliverySession(urlPath); ok {
		return sessionID, true
	}
	if sessionID, ok := liveHLSDeliverySession(urlPath); ok {
		return sessionID, true
	}
	return progressiveDeliverySession(urlPath)
}

// liveHLSDeliverySession matches a Live TV playlist or segment fetch.
//
// The playlist and its segments both have to qualify, for the same reason
// media.m3u8 does above: serveLiveHLSPlaylist copies the request query onto every
// relative media URI, so the player's segment fetches carry the stream token and
// no bearer. Authorizing only the playlist would 401 every segment.
//
// The name is confined to a single path segment so a token for one session cannot
// be spent walking out of its own directory.
func liveHLSDeliverySession(urlPath string) (string, bool) {
	idx := strings.Index(urlPath, liveHLSDeliveryPrefix)
	if idx < 0 {
		return "", false
	}
	rest := urlPath[idx+len(liveHLSDeliveryPrefix):]
	sessionID, name, found := strings.Cut(rest, "/")
	if !found || sessionID == "" || name == "" {
		return "", false
	}
	if strings.Contains(name, "/") {
		return "", false
	}
	return sessionID, true
}

func transcodeDeliverySession(urlPath string) (string, bool) {
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
	case tail == "master.m3u8", tail == "media.m3u8":
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

// progressiveDeliverySession matches the bare progressive stream path.
//
// Only the bare path: the session's own sub-resources (subtitles, fonts) are
// deliberately excluded, because a stream token authorizes delivery of the
// session's media bytes and nothing wider. Widening it later is a decision to
// make on purpose rather than by accident of prefix matching.
//
// The prefix is anchored rather than searched. Searching for "/stream/"
// anywhere would also match /api/v1/playback/transcode/{id}/stream/... and any
// future route containing that segment, quietly extending what a stream token
// authorizes.
func progressiveDeliverySession(urlPath string) (string, bool) {
	idx := strings.Index(urlPath, progressiveDeliveryPrefix)
	if idx < 0 {
		return "", false
	}
	// Must be a top-level /stream/, not a nested segment of some other route.
	//
	// An empty prefix counts: chi trims the mount prefix for sub-routers, so this
	// middleware sees "/stream/{id}" as often as "/api/v1/stream/{id}" -- which is
	// also why the transcode matcher searches instead of anchoring. Requiring the
	// API prefix rejected every real request and 401'd the player.
	if before := urlPath[:idx]; before != "" && !strings.HasSuffix(before, apiVersionPrefix) {
		return "", false
	}
	sessionID := urlPath[idx+len(progressiveDeliveryPrefix):]
	if sessionID == "" || strings.Contains(sessionID, "/") {
		return "", false
	}
	return sessionID, true
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
			token := r.URL.Query().Get(streamtoken.QueryParam)
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
