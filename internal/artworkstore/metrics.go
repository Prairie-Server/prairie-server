package artworkstore

import (
	"path"
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

// recordArtworkRequest counts one request. The key is the *requested* key, so a
// substituted response is still attributed to the rung the client asked for —
// which is the number you want when checking whether clients ask for the rungs
// you generate.
func recordArtworkRequest(key, outcome string, bytesSent int64) {
	imageType := artworkkey.ImageTypeFromPath(key)
	if imageType == "" {
		imageType = "unknown"
	}
	variant := artworkkey.VariantName(key)
	if variant == "" {
		variant = "unknown"
	}
	format := strings.TrimPrefix(path.Ext(key), ".")
	if format == "" {
		format = "unknown"
	}
	artworkRequests.WithLabelValues(imageType, variant, format, outcome).Inc()
	if bytesSent > 0 {
		artworkBytesSent.WithLabelValues(variant).Add(float64(bytesSent))
	}
}
