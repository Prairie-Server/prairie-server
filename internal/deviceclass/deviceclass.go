// Package deviceclass carries the requesting client's device class on the
// request context.
//
// It exists so artwork URL resolution can pick a variant sized for the screen
// that will actually render it. Clients cannot make that choice themselves:
// artwork URLs are HMAC-signed over the exact object key, so a client that
// rewrote the variant segment would invalidate the signature. The smart-TV
// client already refuses to try (see isSignedArtworkURL in its artworkUrl.ts),
// which means its own width constants have no effect against a signing server
// and every TV has been fetching the desktop rung. The decision has to be made
// where the URL is minted.
package deviceclass

import (
	"context"
	"net/http"
	"strings"
)

// Class is the kind of screen a response is being rendered on.
type Class string

const (
	// Unknown is any client that does not identify itself. Treated as desktop:
	// the pre-existing behaviour, so nothing regresses by default.
	Unknown Class = ""
	// TV is a living-room client — a big screen viewed from far away, on a slow
	// SoC with a small memory budget for decoded images.
	TV Class = "tv"
)

// PlatformHeader is the header the first-party clients send.
const PlatformHeader = "X-Prairie-Device-Platform"

type contextKey string

const deviceClassKey contextKey = "device_class"

// tvPlatforms are the platform values that mean "television".
//
// Kept as an explicit set rather than substring matching so a future
// "tvos-companion" or a user-agent containing "tv" cannot silently opt a
// desktop-sized screen into television artwork.
var tvPlatforms = map[string]Class{
	"smarttv":   TV,
	"tizen":     TV,
	"webos":     TV,
	"androidtv": TV,
	"tvos":      TV,
}

// FromPlatform maps a platform identifier to a class.
func FromPlatform(platform string) Class {
	if class, ok := tvPlatforms[strings.ToLower(strings.TrimSpace(platform))]; ok {
		return class
	}
	return Unknown
}

// Middleware stores the requesting client's device class on the context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		class := FromPlatform(r.Header.Get(PlatformHeader))
		if class == Unknown {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(SetContext(r.Context(), class)))
	})
}

// FromContext returns the device class for this request, or Unknown.
func FromContext(ctx context.Context) Class {
	if ctx == nil {
		return Unknown
	}
	class, _ := ctx.Value(deviceClassKey).(Class)
	return class
}

// IsTV reports whether this request came from a television client.
func IsTV(ctx context.Context) bool {
	return FromContext(ctx) == TV
}

// SetContext stores a device class on the context. Useful for tests and for
// non-HTTP entry points that know their caller.
func SetContext(ctx context.Context, class Class) context.Context {
	return context.WithValue(ctx, deviceClassKey, class)
}
