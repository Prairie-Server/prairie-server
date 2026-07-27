package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// profileKey is the context key for storing the profile ID.
const profileKey contextKey = "profile_id"

// RequireProfile is an HTTP middleware that resolves the active profile ID and
// stores it in the request context. It accepts (in order):
//  1. X-Profile-Id header
//  2. profile_id query parameter (for native media players that cannot set headers)
//
// Returns 400 if neither is present.
func RequireProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profileID := extractProfileID(r)
		if profileID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{
				Error:   "bad_request",
				Message: "X-Profile-Id header is required",
			})
			return
		}

		ctx := context.WithValue(r.Context(), profileKey, profileID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractProfileID returns the caller profile from the header or query string.
func extractProfileID(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Profile-Id")); id != "" {
		return id
	}
	if id := strings.TrimSpace(r.URL.Query().Get("profile_id")); id != "" {
		return id
	}
	return ""
}

// GetProfileID retrieves the profile ID from the context. Returns an empty
// string if no profile ID is present.
func GetProfileID(ctx context.Context) string {
	id, _ := ctx.Value(profileKey).(string)
	return id
}

// SetProfileID stores a profile ID in the context. This is useful for
// testing handlers that depend on a profile without going through the
// full middleware chain.
func SetProfileID(ctx context.Context, profileID string) context.Context {
	return context.WithValue(ctx, profileKey, profileID)
}
