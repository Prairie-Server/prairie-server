package artworkstore

import (
	"path"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
)

// Artwork was the one hot path in this server with no telemetry at all: it is
// served from the metrics mux, which skips the API request logger, and the store
// recorded nothing of its own. Answering "is this client actually fetching the
// narrow rung we generated for it?" meant counting files on the volume or
// reading a browser's network tab — which is how a rung selection bug survived
// three releases, and how a discovery bug got misdiagnosed twice in one evening.
var (
	artworkRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamapp_artwork_requests_total",
		Help: "Artwork object requests by image type, width variant, format and outcome.",
	}, []string{"image_type", "variant", "format", "outcome"})

	artworkBytesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamapp_artwork_bytes_sent_total",
		Help: "Artwork response body bytes served, by width variant.",
	}, []string{"variant"})
)

// Outcomes recorded by the artwork handler.
const (
	// outcomeServed is the requested object, served as asked.
	outcomeServed = "served"
	// outcomeSubstituted is a wider rung standing in for one the backfill has
	// not generated yet. A rising rate here means coverage is behind demand.
	outcomeSubstituted  = "substituted"
	outcomeNotFound     = "not_found"
	outcomeExpired      = "expired"
	outcomeBadSignature = "bad_signature"
	outcomeError        = "error"
)

// labelUnknown replaces any label value that is not on an allowlist below.
const labelUnknown = "unknown"

// Every label on these metrics comes from a request path, and the outcomes that
// matter most — not_found, bad_signature, expired — are recorded for requests
// that never passed signature validation. So an unauthenticated caller picks the
// label values. Without an allowlist, GET /artwork/x/<random>/<random>.<random>
// in a loop mints a series per request and grows this process until it dies,
// taking Prometheus cardinality with it.
var (
	knownImageTypes = map[string]bool{
		"poster":   true,
		"backdrop": true,
		"logo":     true,
		"still":    true,
		"profile":  true,
	}

	// Widths any image type has ever generated, as one set rather than per type,
	// so a retired rung still reports under its own name instead of collapsing
	// into unknown — a client still asking for a retired rung is worth seeing.
	knownVariantWidths = map[int]bool{
		200:  true,
		300:  true,
		500:  true,
		1280: true,
		1920: true,
	}

	knownFormats = map[string]bool{
		"webp": true,
		"avif": true,
		"png":  true,
		"jpg":  true,
		"jpeg": true,
	}
)

func normalizedImageType(key string) string {
	imageType := strings.ToLower(artworkkey.ImageTypeFromPath(key))
	if knownImageTypes[imageType] {
		return imageType
	}
	return labelUnknown
}

func normalizedVariant(key string) string {
	variant := strings.ToLower(artworkkey.VariantName(key))
	if variant == artworkkey.OriginalVariant {
		return variant
	}
	if !strings.HasPrefix(variant, "w") {
		return labelUnknown
	}
	width, err := strconv.Atoi(strings.TrimPrefix(variant, "w"))
	if err != nil || !knownVariantWidths[width] {
		return labelUnknown
	}
	return variant
}

func normalizedFormat(key string) string {
	format := strings.ToLower(strings.TrimPrefix(path.Ext(key), "."))
	if knownFormats[format] {
		return format
	}
	return labelUnknown
}

// recordArtworkRequest counts one request. The key is the *requested* key, so a
// substituted response is still attributed to the rung the client asked for —
// which is the number you want when checking whether clients ask for the rungs
// you generate.
func recordArtworkRequest(key, outcome string, bytesSent int64) {
	variant := normalizedVariant(key)
	artworkRequests.WithLabelValues(normalizedImageType(key), variant, normalizedFormat(key), outcome).Inc()
	if bytesSent > 0 {
		artworkBytesSent.WithLabelValues(variant).Add(float64(bytesSent))
	}
}
