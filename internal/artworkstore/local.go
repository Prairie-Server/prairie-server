// Package artworkstore stores and serves cached artwork objects.
//
// When public S3 is configured, the S3 client remains the artwork backend.
// Otherwise LocalStore writes WebP/AVIF/PNG variants under a configurable
// filesystem root and serves them at /artwork/... with short-lived HMAC URLs.
package artworkstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
)

const (
	// DefaultLocalRoot is the on-disk artwork cache when S3 public storage is off.
	DefaultLocalRoot = "/var/lib/prairie/artwork"

	localBucketName     = "local-artwork"
	artworkURLPrefix    = "/artwork/"
	signatureDomain     = "prairie:artwork:v1"
	defaultPresignTTL   = 4 * time.Hour
	signatureQueryParam = "sig"
	expiresQueryParam   = "expires"
)

// ErrNotFound is returned when a local artwork object does not exist.
var ErrNotFound = errors.New("artworkstore: object not found")

// LocalStore is a filesystem ObjectPutter used when public S3 is unconfigured.
type LocalStore struct {
	root          string
	secret        []byte
	ttl           time.Duration
	publicBaseURL string
}

// LocalConfig configures a LocalStore.
type LocalConfig struct {
	Root      string
	URLSecret string
	URLTTL    time.Duration
	// PublicBaseURL, when set (e.g. PRAIRIE_PUBLIC_URL), makes PresignGetURL
	// return absolute URLs so native clients can fetch artwork off-origin.
	PublicBaseURL string
}

// NewLocalStore creates a LocalStore rooted at cfg.Root (default DefaultLocalRoot).
// URLSecret signs read URLs; when empty, PresignGetURL returns unsigned paths
// (suitable only for trusted networks / tests).
func NewLocalStore(cfg LocalConfig) (*LocalStore, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		root = DefaultLocalRoot
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("artworkstore: create root %s: %w", root, err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("artworkstore: resolve root %s: %w", root, err)
	}
	ttl := cfg.URLTTL
	if ttl <= 0 {
		ttl = defaultPresignTTL
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	return &LocalStore{
		root:          abs,
		secret:        []byte(strings.TrimSpace(cfg.URLSecret)),
		ttl:           ttl,
		publicBaseURL: base,
	}, nil
}

// Root returns the absolute filesystem root.
func (s *LocalStore) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// Bucket returns a synthetic bucket name so existing callers that pass
// Bucket() into Put/Delete/Exists keep working unchanged.
func (s *LocalStore) Bucket() string {
	return localBucketName
}

// PutObject writes data to root/key atomically.
func (s *LocalStore) PutObject(ctx context.Context, _ string, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("artworkstore: mkdir for %s: %w", key, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".artwork-*.tmp")
	if err != nil {
		return fmt.Errorf("artworkstore: create temp for %s: %w", key, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("artworkstore: write %s: %w", key, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fmt.Errorf("artworkstore: chmod %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("artworkstore: close temp %s: %w", key, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("artworkstore: rename %s: %w", key, err)
	}
	return nil
}

// GetObject reads the object at key.
func (s *LocalStore) GetObject(ctx context.Context, _ string, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("artworkstore: read %s: %w", key, err)
	}
	return data, nil
}

// ObjectExists reports whether key exists under the local root.
func (s *LocalStore) ObjectExists(ctx context.Context, _ string, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("artworkstore: stat %s: %w", key, err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	return true, nil
}

// ObjectMatches checks that an immutable object exists with the expected length
// and SHA-256 content. Missing or mismatched objects return false so callers
// can safely rewrite them.
func (s *LocalStore) ObjectMatches(ctx context.Context, _ string, key string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("artworkstore: stat %s: %w", key, err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
		return false, nil
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("artworkstore: read %s: %w", key, err)
	}
	sum := sha256.Sum256(existing)
	want := sha256.Sum256(data)
	return subtle.ConstantTimeCompare(sum[:], want[:]) == 1, nil
}

// DeleteObject removes a single key. Missing objects are ignored.
func (s *LocalStore) DeleteObject(ctx context.Context, _ string, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("artworkstore: delete %s: %w", key, err)
	}
	return nil
}

// DeleteObjects deletes the given keys. Returns how many were removed.
func (s *LocalStore) DeleteObjects(ctx context.Context, bucket string, keys []string) (int, error) {
	deleted := 0
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		path, err := s.objectPath(key)
		if err != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return deleted, fmt.Errorf("artworkstore: delete %s: %w", key, err)
		}
		deleted++
	}
	return deleted, nil
}

// DeletePrefix deletes all regular files under prefix. Returns count deleted.
func (s *LocalStore) DeletePrefix(ctx context.Context, _ string, prefix string) (int, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return 0, fmt.Errorf("artworkstore: refuse to delete empty prefix")
	}
	keys, err := s.ListObjects(ctx, "", prefix)
	if err != nil {
		return 0, err
	}
	return s.DeleteObjects(ctx, "", keys)
}

// ListObjects returns object keys with the given prefix.
func (s *LocalStore) ListObjects(ctx context.Context, _ string, prefix string) ([]string, error) {
	prefix = strings.TrimLeft(strings.TrimSpace(prefix), "/")
	root := s.root
	var keys []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("artworkstore: list prefix %s: %w", prefix, err)
	}
	return keys, nil
}

// EffectivePresignTTL returns the TTL used for signed read URLs.
func (s *LocalStore) EffectivePresignTTL(requested time.Duration) time.Duration {
	if requested <= 0 {
		return s.ttl
	}
	return requested
}

// PresignGetURL returns a /artwork/... URL for the object key. When a signing
// secret is configured the URL includes expires+sig query params that the
// HTTP handler verifies. bucket is ignored (kept for interface parity).
func (s *LocalStore) PresignGetURL(_ context.Context, _ string, key string, expiry time.Duration) (string, error) {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return "", fmt.Errorf("artworkstore: empty object key")
	}
	if _, err := s.objectPath(key); err != nil {
		return "", err
	}
	path := artworkURLPrefix + key
	if len(s.secret) == 0 {
		if s.publicBaseURL != "" {
			return s.publicBaseURL + path, nil
		}
		return path, nil
	}
	if expiry <= 0 {
		expiry = s.ttl
	}
	expires := time.Now().Add(expiry).Unix()
	sig := s.sign(key, expires)
	signed := path + "?" + expiresQueryParam + "=" + strconv.FormatInt(expires, 10) +
		"&" + signatureQueryParam + "=" + url.QueryEscape(sig)
	if s.publicBaseURL != "" {
		return s.publicBaseURL + signed, nil
	}
	return signed, nil
}

// Handler returns an http.Handler that serves objects from the local root at
// /artwork/{key}. Signed URLs are required when a secret is configured.
func (s *LocalStore) Handler() http.Handler {
	return http.HandlerFunc(s.ServeHTTP)
}

// ServeHTTP serves a single artwork object.
func (s *LocalStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, artworkURLPrefix)
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "..") {
		http.NotFound(w, r)
		return
	}
	// Decode percent-encoding once; objectPath rejects traversal.
	if decoded, err := url.PathUnescape(key); err == nil {
		key = decoded
	}
	if len(s.secret) > 0 {
		expiresStr := r.URL.Query().Get(expiresQueryParam)
		sig := r.URL.Query().Get(signatureQueryParam)
		expires, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil || expires < time.Now().Unix() {
			http.Error(w, "url expired", http.StatusForbidden)
			return
		}
		expected := s.sign(key, expires)
		if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
	}

	f, info, substituted, err := s.openWithRungFallback(key)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Content type comes from the requested key, which is correct either way: a
	// substituted rung differs only in width, never in format.
	if ct := ContentTypeFromKey(key); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if substituted {
		// A wider rung is standing in for one that has not been generated yet.
		// This response must NOT be cached as content-addressed: the real object
		// will appear at this exact URL once the backfill reaches it, and an
		// immutable year-long entry would hide it for that long — on TV clients
		// that is effectively forever. Keep it short so caches re-ask soon.
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		// Revisioned object keys are content-addressed; allow long-lived browser cache.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = io.Copy(w, f)
}

// openWithRungFallback opens the object for key, falling back to wider rungs of
// the same artwork when the requested one is absent. Reports whether the file it
// returns is a substitute.
//
// Adding a width to the ladder does not create it for artwork cached earlier,
// and the backfill that fills it in takes as long as the catalogue is large.
// Without this, a client asking for the new rung gets a 404 for that entire
// period; the client cannot tell "not generated yet" from "gone", and probing
// costs a wasted round-trip per image on exactly the low-powered devices the
// narrow rung exists for.
//
// Substituting stays within the same artwork directory, revision and extension,
// so it can only ever serve another rung of the very same image — a client could
// request it directly with its own signed URL, so no access is widened.
func (s *LocalStore) openWithRungFallback(key string) (*os.File, fs.FileInfo, bool, error) {
	f, info, err := s.openObject(key)
	if err == nil {
		return f, info, false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, false, err
	}
	for _, wider := range artworkkey.WiderVariantKeys(key) {
		f, info, widerErr := s.openObject(wider)
		if widerErr == nil {
			return f, info, true, nil
		}
		if !errors.Is(widerErr, fs.ErrNotExist) {
			return nil, nil, false, widerErr
		}
	}
	return nil, nil, false, err
}

// openObject opens one object by key, treating anything that is not a regular
// file as absent.
func (s *LocalStore) openObject(key string) (*os.File, fs.FileInfo, error) {
	objectPath, err := s.objectPath(key)
	if err != nil {
		return nil, nil, fs.ErrNotExist
	}
	f, err := os.Open(objectPath)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fs.ErrNotExist
	}
	return f, info, nil
}

func (s *LocalStore) sign(key string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(signatureDomain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.FormatInt(expires, 10)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *LocalStore) objectPath(key string) (string, error) {
	if s == nil || s.root == "" {
		return "", fmt.Errorf("artworkstore: store not configured")
	}
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return "", fmt.Errorf("artworkstore: empty key")
	}
	if strings.Contains(key, "\x00") {
		return "", fmt.Errorf("artworkstore: invalid key")
	}
	cleaned := filepath.Clean(filepath.FromSlash(key))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("artworkstore: invalid key %q", key)
	}
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("artworkstore: absolute key rejected")
	}
	full := filepath.Join(s.root, cleaned)
	// Ensure the resolved path stays under root even if Clean left oddities.
	rel, err := filepath.Rel(s.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("artworkstore: key escapes root")
	}
	return full, nil
}

// ContentTypeFromKey returns a MIME type based on the file extension.
func ContentTypeFromKey(key string) string {
	switch {
	case strings.HasSuffix(key, ".jpg"), strings.HasSuffix(key, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(key, ".png"):
		return "image/png"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	case strings.HasSuffix(key, ".avif"):
		return "image/avif"
	case strings.HasSuffix(key, ".gif"):
		return "image/gif"
	case strings.HasSuffix(key, ".svg"):
		return "image/svg+xml"
	default:
		return ""
	}
}

// StorageIdentity fingerprints a local artwork root for reconcile triggers.
func StorageIdentity(root string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		root = DefaultLocalRoot
	}
	return "local|" + root
}
