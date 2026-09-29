package s3client

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/envutil"
)

// TestAWSStyleSmoke exercises the same SDK client used by Silo against an
// opt-in endpoint. It is intentionally skipped in normal test runs; QA can use
// it for AWS or a TLS-enabled, virtual-host-style compatible endpoint.
func TestAWSStyleSmoke(t *testing.T) {
	endpoint := envutil.Getenv("SILO_AWS_STYLE_SMOKE_ENDPOINT")
	if endpoint == "" {
		t.Skip("SILO_AWS_STYLE_SMOKE_ENDPOINT is not set")
	}
	bucket := envutil.Getenv("SILO_AWS_STYLE_SMOKE_BUCKET")
	accessKey := envutil.Getenv("SILO_AWS_STYLE_SMOKE_ACCESS_KEY")
	secretKey := envutil.Getenv("SILO_AWS_STYLE_SMOKE_SECRET_KEY")
	region := envutil.Getenv("SILO_AWS_STYLE_SMOKE_REGION")
	if bucket == "" || accessKey == "" || secretKey == "" {
		t.Fatal("bucket and credentials are required")
	}

	client := NewClient(BucketConfig{
		Role:      "qa-aws-style",
		Endpoint:  endpoint,
		Region:    region,
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
		PathStyle: false,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	key := "qa/aws-style-smoke.txt"
	want := []byte("silo aws-style smoke")
	putErr := client.PutObject(ctx, bucket, key, want)
	if envutil.Getenv("SILO_AWS_STYLE_SMOKE_EXPECT_PUT_DENIED") == "true" {
		if putErr == nil {
			t.Fatal("IAM smoke expected PutObject to be denied")
		}
		return
	}
	if putErr != nil {
		t.Fatalf("virtual-host-style put: %v", putErr)
	}
	got, err := client.GetObject(ctx, bucket, key)
	if err != nil || string(got) != string(want) {
		t.Fatalf("virtual-host-style TLS get: %v %q", err, got)
	}
	keys, err := client.ListObjects(ctx, bucket, "qa/")
	if err != nil || len(keys) != 1 || keys[0] != key {
		t.Fatalf("virtual-host-style TLS list: %v %#v", err, keys)
	}
	url, err := client.PresignGetURL(ctx, bucket, key, time.Minute)
	if err != nil || !strings.Contains(url, bucket+".") {
		t.Fatalf("virtual-host-style presign: %v %q", err, url)
	}
}
