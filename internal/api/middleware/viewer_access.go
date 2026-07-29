package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/prairie-server/prairie-server/internal/access"
	"github.com/prairie-server/prairie-server/internal/auth"
)

// ViewerResolver resolves the effective viewer access scope for a request.
type ViewerResolver interface {
	Resolve(ctx context.Context, input access.ResolveInput) (access.Scope, error)
}

// ViewerAccessMiddleware resolves and stores viewer access scope in context.
type ViewerAccessMiddleware struct {
	resolver ViewerResolver
}

// NewViewerAccessMiddleware creates a middleware from a scope resolver.
func NewViewerAccessMiddleware(resolver ViewerResolver) *ViewerAccessMiddleware {
	return &ViewerAccessMiddleware{resolver: resolver}
}

// RequireViewerAccess resolves viewer scope from auth + profile headers.
func (m *ViewerAccessMiddleware) RequireViewerAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stream-token delivery carries no identity by design, so there are no
		// claims to resolve a viewer scope from. The scope was already enforced
		// when the session was created (/playback/start runs full auth and
		// viewer access); this request only fetches bytes for that session, and
		// the signature naming it is the authorization. Without this skip the
		// nil-claims branch below would 401 every playlist and segment fetch.
		if IsStreamTokenAuthorized(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}

		claims := GetClaims(r.Context())
		if claims == nil {
			writeUnauthorized(w, "Authentication required")
			return
		}

		profileID := r.Header.Get("X-Profile-Id")
		input := access.ResolveInput{
			UserID:              claims.UserID,
			SessionID:           claims.SessionID,
			ProfileID:           profileID,
			ProfileToken:        r.Header.Get("X-Profile-Token"),
			SkipPINVerification: claims.TokenType == auth.TokenTypeAPIKey,
		}

		scope, err := m.resolver.Resolve(r.Context(), input)
		if err != nil {
			switch {
			case errors.Is(err, access.ErrProfileUnverified):
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(errorResponse{
					Error:   "profile_unverified",
					Message: "Profile verification required",
				})
				return
			case errors.Is(err, access.ErrProfileNotFound):
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(errorResponse{
					Error:   "not_found",
					Message: "Profile not found",
				})
				return
			default:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(errorResponse{
					Error:   "internal_error",
					Message: "Failed to resolve viewer access",
				})
				return
			}
		}

		ctx := access.SetScope(r.Context(), scope)
		if profileID != "" {
			ctx = SetProfileID(ctx, profileID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
