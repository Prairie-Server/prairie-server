package artworkstore

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func artworkCounter(t *testing.T, imageType, variant, format, outcome string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
	want := map[string]string{
		"image_type": imageType,
		"variant":    variant,
		"format":     format,
		"outcome":    outcome,
	}
	for _, family := range families {
		if family.GetName() != "streamapp_artwork_requests_total" {
			continue
		}
	sample:
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if want[label.GetName()] != label.GetValue() {
					continue sample
				}
			}
			return metric.GetCounter().GetValue()
		}
	}
	return 0
}

// writeObject puts a file at key inside the store root.
func writeObject(t *testing.T, root, key string, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestArtworkMetricsCountServedAndSubstituted(t *testing.T) {
	root := t.TempDir()
	// Only the wider rung exists, so a w200 request is answered by substitution —
	// exactly the state a catalog is in while a new rung backfills.
	writeObject(t, root, "m/1/poster/w500.9.webp", "wide")
	writeObject(t, root, "m/1/still/w300.9.webp", "still")
	store := &LocalStore{root: root}

	before := artworkCounter(t, "poster", "w500", "webp", outcomeServed)
	beforeSub := artworkCounter(t, "poster", "w200", "webp", outcomeSubstituted)

	rec := httptest.NewRecorder()
	store.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/artwork/m/1/poster/w500.9.webp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("served status = %d, want 200", rec.Code)
	}

	subRec := httptest.NewRecorder()
	store.ServeHTTP(subRec, httptest.NewRequest(http.MethodGet, "/artwork/m/1/poster/w200.9.webp", nil))
	if subRec.Code != http.StatusOK {
		t.Fatalf("substituted status = %d, want 200", subRec.Code)
	}

	if got := artworkCounter(t, "poster", "w500", "webp", outcomeServed); got != before+1 {
		t.Errorf("served counter = %v, want %v", got, before+1)
	}
	// Attributed to the rung the client asked for, not the one that answered:
	// that is the number that says whether clients want the rung we generate.
	if got := artworkCounter(t, "poster", "w200", "webp", outcomeSubstituted); got != beforeSub+1 {
		t.Errorf("substituted counter = %v, want %v", got, beforeSub+1)
	}
}

func TestArtworkMetricsCountNotFound(t *testing.T) {
	store := &LocalStore{root: t.TempDir()}
	before := artworkCounter(t, "logo", "w500", "webp", outcomeNotFound)

	rec := httptest.NewRecorder()
	store.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/artwork/m/2/logo/w500.1.webp", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := artworkCounter(t, "logo", "w500", "webp", outcomeNotFound); got != before+1 {
		t.Errorf("not_found counter = %v, want %v", got, before+1)
	}
}

// A bad-signature spike is the fingerprint of a client rewriting a key outside
// its signing scope, which is how the cast-portrait 403s stayed invisible.
func TestArtworkMetricsCountBadSignatureAndExpiry(t *testing.T) {
	root := t.TempDir()
	writeObject(t, root, "p/1/profile/w200.3.webp", "narrow")
	store := &LocalStore{root: root, secret: []byte("secret")}

	beforeBad := artworkCounter(t, "profile", "w200", "webp", outcomeBadSignature)
	beforeExp := artworkCounter(t, "profile", "w200", "webp", outcomeExpired)

	bad := httptest.NewRecorder()
	store.ServeHTTP(bad, httptest.NewRequest(http.MethodGet,
		"/artwork/p/1/profile/w200.3.webp?expires=99999999999&sig=deadbeef", nil))
	if bad.Code != http.StatusForbidden {
		t.Fatalf("bad signature status = %d, want 403", bad.Code)
	}

	expired := httptest.NewRecorder()
	store.ServeHTTP(expired, httptest.NewRequest(http.MethodGet,
		"/artwork/p/1/profile/w200.3.webp?expires=1&sig=deadbeef", nil))
	if expired.Code != http.StatusForbidden {
		t.Fatalf("expired status = %d, want 403", expired.Code)
	}

	if got := artworkCounter(t, "profile", "w200", "webp", outcomeBadSignature); got != beforeBad+1 {
		t.Errorf("bad_signature counter = %v, want %v", got, beforeBad+1)
	}
	if got := artworkCounter(t, "profile", "w200", "webp", outcomeExpired); got != beforeExp+1 {
		t.Errorf("expired counter = %v, want %v", got, beforeExp+1)
	}
}

// HEAD is how the reconciler probes coverage: counted as a request, never as
// bytes, so probe traffic cannot inflate the bandwidth series.
func TestArtworkMetricsCountHeadWithoutBytes(t *testing.T) {
	root := t.TempDir()
	writeObject(t, root, "m/3/backdrop/w1280.5.webp", "backdrop-bytes")
	store := &LocalStore{root: root}

	before := artworkCounter(t, "backdrop", "w1280", "webp", outcomeServed)

	rec := httptest.NewRecorder()
	store.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/artwork/m/3/backdrop/w1280.5.webp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD returned %d body bytes, want 0", rec.Body.Len())
	}
	if got := artworkCounter(t, "backdrop", "w1280", "webp", outcomeServed); got != before+1 {
		t.Errorf("served counter = %v, want %v", got, before+1)
	}
}
