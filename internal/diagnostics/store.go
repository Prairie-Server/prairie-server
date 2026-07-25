package diagnostics

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/prairie-server/prairie-server/internal/s3client"
)

var ErrObjectNotFound = errors.New("diagnostics object not found")

// ObjectStore is the diagnostics-owned object storage surface. It is narrow
// enough for fake-backed ingest/admin tests and wraps the private S3 bucket in
// production.
type ObjectStore interface {
	PutStream(ctx context.Context, bucket, key string, r io.Reader, contentType string) error
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	ListObjects(ctx context.Context, prefix string) ([]string, error)
	PresignGetURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
	Bucket() string
}

type s3ObjectStore struct {
	client *s3client.Client
}

// NewS3ObjectStore adapts the private S3 client to the diagnostics store
// interface. A nil client returns nil so interface-nil checks remain reliable.
func NewS3ObjectStore(client *s3client.Client) ObjectStore {
	if client == nil {
		return nil
	}
	return &s3ObjectStore{client: client}
}

func (s *s3ObjectStore) PutStream(ctx context.Context, bucket, key string, r io.Reader, contentType string) error {
	return normalizeObjectStoreError(s.client.PutObjectStream(ctx, bucket, key, r, contentType))
}

func (s *s3ObjectStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	body, err := s.client.GetObjectStream(ctx, bucket, key)
	if err != nil {
		return nil, normalizeObjectStoreError(err)
	}
	return body, nil
}

func (s *s3ObjectStore) DeleteObject(ctx context.Context, bucket, key string) error {
	return normalizeObjectStoreError(s.client.DeleteObject(ctx, bucket, key))
}

func (s *s3ObjectStore) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	keys, err := s.client.ListObjects(ctx, s.client.Bucket(), prefix)
	if err != nil {
		return nil, normalizeObjectStoreError(err)
	}
	return keys, nil
}

func (s *s3ObjectStore) PresignGetURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	url, err := s.client.PresignGetURL(ctx, bucket, key, expiry)
	if err != nil {
		return "", normalizeObjectStoreError(err)
	}
	return url, nil
}

func (s *s3ObjectStore) EffectivePresignTTL(requested time.Duration) time.Duration {
	return s.client.EffectivePresignTTL(requested)
}

func (s *s3ObjectStore) Bucket() string {
	return s.client.Bucket()
}

func normalizeObjectStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, s3client.ErrNotFound) {
		return ErrObjectNotFound
	}
	return err
}

func IsObjectNotFound(err error) bool {
	return errors.Is(err, ErrObjectNotFound) || errors.Is(err, s3client.ErrNotFound)
}
