package httpheaders

import (
	"net/http"
	"strings"
)

const (
	prairiePrefix = "X-Prairie-"
	siloPrefix    = "X-Silo-"

	HeaderDeviceID           = prairiePrefix + "Device-Id"
	HeaderDeviceName         = prairiePrefix + "Device-Name"
	HeaderDevicePlatform     = prairiePrefix + "Device-Platform"
	HeaderClient             = prairiePrefix + "Client"
	HeaderClientVersion      = prairiePrefix + "Client-Version"
	HeaderStreamToken        = prairiePrefix + "Stream-Token"
	HeaderEbookConversion    = prairiePrefix + "Ebook-Conversion"
	HeaderUserID             = prairiePrefix + "User-Id"
	HeaderUserRole           = prairiePrefix + "User-Role"
	HeaderUserName           = prairiePrefix + "User-Name"
	HeaderProfileName        = prairiePrefix + "Profile-Name"
	HeaderProfilePrimary     = prairiePrefix + "Profile-Primary"
	HeaderTheme              = prairiePrefix + "Theme"
	HeaderRestartRequired    = prairiePrefix + "Restart-Required"
	HeaderJellyfinWebVersion = prairiePrefix + "Jellyfin-Web-Version"
	HeaderEvent              = prairiePrefix + "Event"
	HeaderWebhookID          = prairiePrefix + "Webhook-Id"
	HeaderDeliveryID         = prairiePrefix + "Delivery-Id"
	HeaderChannelID          = prairiePrefix + "Channel-Id"
	HeaderTimestamp          = prairiePrefix + "Timestamp"
	HeaderSignature          = prairiePrefix + "Signature"

	// HeaderImageFormats is a comma-separated raster preference list from the
	// client (e.g. "avif,webp,png"), ordered best-first. Clients probe decode
	// support once per install and send this on API requests so the server can
	// pick siblings without per-image trial-and-error fallbacks.
	HeaderImageFormats = prairiePrefix + "Image-Formats"
)

// LegacyName returns the corresponding pre-rebrand X-Silo-* header name.
func LegacyName(name string) string {
	if strings.HasPrefix(name, prairiePrefix) {
		return siloPrefix + strings.TrimPrefix(name, prairiePrefix)
	}
	return name
}

// Get prefers X-Prairie-* and falls back to the equivalent X-Silo-* header.
func Get(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if value := headers.Get(name); value != "" {
		return value
	}
	legacy := LegacyName(name)
	if legacy == name {
		return ""
	}
	return headers.Get(legacy)
}

// RequestValue prefers X-Prairie-* and falls back to the equivalent X-Silo-* header.
func RequestValue(r *http.Request, name string) string {
	if r == nil {
		return ""
	}
	return Get(r.Header, name)
}

// Set emits the Prairie header name.
func Set(headers http.Header, name, value string) {
	headers.Set(name, value)
}

// SetMap emits the Prairie header name into a string header map.
func SetMap(headers map[string]string, name, value string) {
	headers[name] = value
}
