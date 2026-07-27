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
