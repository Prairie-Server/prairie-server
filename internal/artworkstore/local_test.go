package artworkstore

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalStorePutGetExistsAndMatches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := NewLocalStore(LocalConfig{Root: root, URLSecret: "test-secret"})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	ctx := context.Background()
	key := "tmdb/movies/550/poster/original.abc123.webp"
	data := []byte("webp-bytes")
	if err := store.PutObject(ctx, store.Bucket(), key, data); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	got, err := store.GetObject(ctx, store.Bucket(), key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("GetObject = %q, want %q", got, data)
	}

	exists, err := store.ObjectExists(ctx, store.Bucket(), key)
	if err != nil || !exists {
		t.Fatalf("ObjectExists = %v, %v", exists, err)
	}

	match, err := store.ObjectMatches(ctx, store.Bucket(), key, data)
	if err != nil || !match {
		t.Fatalf("ObjectMatches same = %v, %v", match, err)
	}
	match, err = store.ObjectMatches(ctx, store.Bucket(), key, []byte("other"))
	if err != nil || match {
		t.Fatalf("ObjectMatches different = %v, %v", match, err)
	}

	diskPath := filepath.Join(root, filepath.FromSlash(key))
	if _, err := os.Stat(diskPath); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}
}

func TestLocalStoreRejectsPathTraversal(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	for _, key := range []string{"../escape.webp", "a/../../b.webp", "..", "foo/../../../etc/passwd"} {
		if _, err := store.objectPath(key); err == nil {
			t.Fatalf("objectPath(%q) should fail", key)
		}
	}
	// Leading slash is stripped to a relative key under the artwork root — safe.
	if _, err := store.objectPath("/tmdb/movies/1/poster/original.webp"); err != nil {
		t.Fatalf("leading-slash key should be accepted: %v", err)
	}
}

func TestLocalStoreDeletePrefix(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	keys := []string{
		"local/series/1/abcd1234/poster/original.r1.webp",
		"local/series/1/abcd1234/poster/original.r1.avif",
		"local/series/1/abcd1234/poster/w300.r1.webp",
		"tmdb/movies/1/poster/original.r1.webp",
	}
	for _, key := range keys {
		if err := store.PutObject(ctx, store.Bucket(), key, []byte(key)); err != nil {
			t.Fatalf("PutObject %s: %v", key, err)
		}
	}
	n, err := store.DeletePrefix(ctx, store.Bucket(), "local/series/1/abcd1234/poster/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if n != 3 {
		t.Fatalf("DeletePrefix deleted %d, want 3", n)
	}
	exists, err := store.ObjectExists(ctx, store.Bucket(), "tmdb/movies/1/poster/original.r1.webp")
	if err != nil || !exists {
		t.Fatalf("unrelated object should remain: exists=%v err=%v", exists, err)
	}
}

func TestLocalStorePresignAndServe(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{
		Root:      t.TempDir(),
		URLSecret: "serve-secret",
		URLTTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	key := "tmdb/movies/550/poster/original.rev.webp"
	payload := []byte{0x00, 0x01, 0x02, 0x03}
	if err := store.PutObject(ctx, store.Bucket(), key, payload); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	signed, err := store.PresignGetURL(ctx, store.Bucket(), key, time.Hour)
	if err != nil {
		t.Fatalf("PresignGetURL: %v", err)
	}
	if !strings.HasPrefix(signed, "/artwork/"+key+"?") {
		t.Fatalf("signed URL = %q", signed)
	}
	if !strings.Contains(signed, "sig=") || !strings.Contains(signed, "expires=") {
		t.Fatalf("signed URL missing params: %q", signed)
	}

	req := httptest.NewRequest(http.MethodGet, signed, nil)
	rec := httptest.NewRecorder()
	store.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("Content-Type = %q", got)
	}
	if body, _ := io.ReadAll(rec.Body); string(body) != string(payload) {
		t.Fatalf("body mismatch")
	}

	// Tampered signature.
	u, _ := url.Parse(signed)
	q := u.Query()
	q.Set("sig", "deadbeef")
	u.RawQuery = q.Encode()
	bad := httptest.NewRequest(http.MethodGet, u.String(), nil)
	badRec := httptest.NewRecorder()
	store.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusForbidden {
		t.Fatalf("tampered status = %d", badRec.Code)
	}
}

func TestLocalStorePresignRequiresExistingKeyShape(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s"})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	if _, err := store.PresignGetURL(context.Background(), store.Bucket(), "../x", time.Minute); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestContentTypeFromKey(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"a.webp": "image/webp",
		"a.avif": "image/avif",
		"a.png":  "image/png",
		"a.jpg":  "image/jpeg",
		"a.bin":  "",
	}
	for key, want := range cases {
		if got := ContentTypeFromKey(key); got != want {
			t.Fatalf("ContentTypeFromKey(%q)=%q want %q", key, got, want)
		}
	}
}

func TestStorageIdentity(t *testing.T) {
	t.Parallel()
	if got := StorageIdentity("/var/lib/prairie/artwork"); got != "local|/var/lib/prairie/artwork" {
		t.Fatalf("got %q", got)
	}
}

func TestObjectMatchesHashesContent(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	data := []byte("same-bytes")
	sum := sha256.Sum256(data)
	_ = sum
	ctx := context.Background()
	key := "a/b.webp"
	if err := store.PutObject(ctx, "", key, data); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	ok, err := store.ObjectMatches(ctx, "", key, append([]byte(nil), data...))
	if err != nil || !ok {
		t.Fatalf("match=%v err=%v", ok, err)
	}
}

// serveKey presigns key and serves it, returning the recorder.
func serveKey(t *testing.T, store *LocalStore, key string) *httptest.ResponseRecorder {
	t.Helper()
	signed, err := store.PresignGetURL(context.Background(), store.Bucket(), key, time.Hour)
	if err != nil {
		t.Fatalf("PresignGetURL(%s): %v", key, err)
	}
	rec := httptest.NewRecorder()
	store.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, signed, nil))
	return rec
}

// A rung added to the ladder after artwork was cached does not exist for that
// artwork until the backfill reaches it. Serving the next rung up keeps clients
// working through that window instead of 404ing every image.
func TestLocalStoreServesWiderRungWhenVariantMissing(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	dir := "tmdb/movies/550/poster/"
	w300 := []byte("three-hundred")
	if err := store.PutObject(ctx, store.Bucket(), dir+"w300.rev.webp", w300); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	rec := serveKey(t, store, dir+"w200.rev.webp")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (should fall back to w300)", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Body); string(body) != string(w300) {
		t.Fatalf("body = %q, want the w300 bytes", body)
	}
	// Content type follows the requested key; the substitute differs only in width.
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("Content-Type = %q", got)
	}
	// Critically: not immutable. The real w200 lands at this same URL later, and a
	// year-long entry would hide it.
	if got := rec.Header().Get("Cache-Control"); strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q, must not be immutable for a substituted rung", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=300") {
		t.Fatalf("Cache-Control = %q, want a short max-age", got)
	}
}

func TestLocalStorePrefersNarrowestAvailableRung(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	dir := "tmdb/movies/550/poster/"
	for key, body := range map[string]string{
		"w300.rev.webp":     "three-hundred",
		"w500.rev.webp":     "five-hundred",
		"original.rev.webp": "original",
	} {
		if err := store.PutObject(ctx, store.Bucket(), dir+key, []byte(body)); err != nil {
			t.Fatalf("PutObject(%s): %v", key, err)
		}
	}

	// Sending the original would defeat the ladder; w300 is the nearest rung up.
	rec := serveKey(t, store, dir+"w200.rev.webp")
	if body, _ := io.ReadAll(rec.Body); string(body) != "three-hundred" {
		t.Fatalf("body = %q, want the narrowest available rung (w300)", body)
	}
}

func TestLocalStoreExactRungStaysImmutable(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	key := "tmdb/movies/550/poster/w200.rev.webp"
	if err := store.PutObject(context.Background(), store.Bucket(), key, []byte("two-hundred")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	rec := serveKey(t, store, key)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q, an exact hit must stay content-addressed", got)
	}
}

func TestLocalStoreStill404sWhenNoRungExists(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	// Nothing cached at all: absence is still absence.
	rec := serveKey(t, store, "tmdb/movies/550/poster/w200.rev.webp")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestLocalStoreOriginalDoesNotFallBack(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	dir := "tmdb/movies/550/poster/"
	if err := store.PutObject(context.Background(), store.Bucket(), dir+"w500.rev.webp", []byte("x")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	// "original" is the top of the ladder; a request for it must not be answered
	// with a downscaled rung.
	rec := serveKey(t, store, dir+"original.rev.webp")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a missing original", rec.Code)
	}
}

// A substituted rung must never cross format: a client that asked for AVIF
// cannot decode WebP bytes served under an .avif URL.
func TestLocalStoreFallbackKeepsFormat(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "s", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	dir := "tmdb/movies/550/poster/"
	// Only WebP exists at the wider rung; an AVIF request must not borrow it.
	if err := store.PutObject(context.Background(), store.Bucket(), dir+"w300.rev.webp", []byte("webp")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	rec := serveKey(t, store, dir+"w200.rev.avif")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 rather than WebP bytes under an .avif URL", rec.Code)
	}
}

// serveKeySignedAs presigns signKey, swaps the path to requestKey, and serves
// it — i.e. a client selecting a different rung from the URL it was given.
func serveKeySignedAs(t *testing.T, store *LocalStore, signKey, requestKey string) *httptest.ResponseRecorder {
	t.Helper()
	signed, err := store.PresignGetURL(context.Background(), store.Bucket(), signKey, time.Hour)
	if err != nil {
		t.Fatalf("PresignGetURL(%s): %v", signKey, err)
	}
	swapped := strings.Replace(signed, signKey, requestKey, 1)
	if swapped == signed && signKey != requestKey {
		t.Fatalf("failed to swap %q for %q in %q", signKey, requestKey, signed)
	}
	rec := httptest.NewRecorder()
	store.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, swapped, nil))
	return rec
}

func scopeStore(t *testing.T) *LocalStore {
	t.Helper()
	store, err := NewLocalStore(LocalConfig{Root: t.TempDir(), URLSecret: "scope-secret", URLTTL: time.Hour})
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	return store
}

// One signature covers every display rung of an artwork revision, which is what
// lets a client pick the width that fits the element it is drawing.
func TestSignatureCoversSiblingRungsOfSameRevision(t *testing.T) {
	t.Parallel()
	store := scopeStore(t)
	ctx := context.Background()
	dir := "tmdb/movies/550/poster/"
	for _, name := range []string{"w200.rev1.webp", "w300.rev1.webp", "w500.rev1.webp", "w300.rev1.avif"} {
		if err := store.PutObject(ctx, store.Bucket(), dir+name, []byte(name)); err != nil {
			t.Fatalf("PutObject(%s): %v", name, err)
		}
	}

	for _, requested := range []string{"w200.rev1.webp", "w300.rev1.webp", "w300.rev1.avif"} {
		rec := serveKeySignedAs(t, store, dir+"w500.rev1.webp", dir+requested)
		if rec.Code != http.StatusOK {
			t.Errorf("requesting %s with a w500 signature = %d, want 200", requested, rec.Code)
		}
		if body, _ := io.ReadAll(rec.Body); string(body) != requested {
			t.Errorf("requesting %s served %q", requested, body)
		}
	}
}

// A different revision is different content and must not be reachable.
func TestSignatureDoesNotCrossRevisions(t *testing.T) {
	t.Parallel()
	store := scopeStore(t)
	ctx := context.Background()
	dir := "tmdb/movies/550/poster/"
	if err := store.PutObject(ctx, store.Bucket(), dir+"w300.rev1.webp", []byte("one")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if err := store.PutObject(ctx, store.Bucket(), dir+"w300.rev2.webp", []byte("two")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	rec := serveKeySignedAs(t, store, dir+"w300.rev1.webp", dir+"w300.rev2.webp")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-revision request = %d, want 403", rec.Code)
	}
}

// Nor another item's artwork.
func TestSignatureDoesNotCrossArtwork(t *testing.T) {
	t.Parallel()
	store := scopeStore(t)
	ctx := context.Background()
	mine := "tmdb/movies/550/poster/w300.rev1.webp"
	theirs := "tmdb/movies/999/poster/w300.rev1.webp"
	for _, key := range []string{mine, theirs} {
		if err := store.PutObject(ctx, store.Bucket(), key, []byte("x")); err != nil {
			t.Fatalf("PutObject(%s): %v", key, err)
		}
	}

	if rec := serveKeySignedAs(t, store, mine, theirs); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-artwork request = %d, want 403", rec.Code)
	}
	// Same directory but a different image type is also a different scope.
	still := "tmdb/movies/550/still/w300.rev1.webp"
	if err := store.PutObject(ctx, store.Bucket(), still, []byte("x")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if rec := serveKeySignedAs(t, store, mine, still); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-image-type request = %d, want 403", rec.Code)
	}
}

// The full-size source is scoped to itself: a display rung must not widen to it.
func TestSignatureDoesNotWidenToOriginal(t *testing.T) {
	t.Parallel()
	store := scopeStore(t)
	ctx := context.Background()
	dir := "tmdb/movies/550/poster/"
	for _, name := range []string{"w300.rev1.webp", "original.rev1.webp"} {
		if err := store.PutObject(ctx, store.Bucket(), dir+name, []byte(name)); err != nil {
			t.Fatalf("PutObject(%s): %v", name, err)
		}
	}

	rec := serveKeySignedAs(t, store, dir+"w300.rev1.webp", dir+"original.rev1.webp")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rung->original = %d, want 403", rec.Code)
	}
	// And the reverse: an original URL is not a license to fetch rungs.
	rec = serveKeySignedAs(t, store, dir+"original.rev1.webp", dir+"w300.rev1.webp")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("original->rung = %d, want 403", rec.Code)
	}
	// A caller issued an original URL can still use it.
	if rec := serveKey(t, store, dir+"original.rev1.webp"); rec.Code != http.StatusOK {
		t.Fatalf("original with its own signature = %d, want 200", rec.Code)
	}
}

func TestSignatureScope(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{
		"a/b/poster/w300.rev1.webp":     "a/b/poster/rev1",
		"a/b/poster/w500.rev1.avif":     "a/b/poster/rev1",
		"a/b/poster/original.rev1.webp": "a/b/poster/original.rev1.webp",
		// Legacy unrevisioned keys have one generation per directory.
		"a/b/poster/w300.webp": "a/b/poster/",
	} {
		if got := signatureScope(key); got != want {
			t.Errorf("signatureScope(%q) = %q, want %q", key, got, want)
		}
	}
}
