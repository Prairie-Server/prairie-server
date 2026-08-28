package secret

import (
	"bytes"
	"testing"
)

func TestDeriveAPIKeyHashKey(t *testing.T) {
	t.Parallel()

	master := []byte("0123456789abcdef0123456789abcdef")
	key1, err := DeriveAPIKeyHashKey(master)
	if err != nil {
		t.Fatalf("DeriveAPIKeyHashKey: %v", err)
	}
	key2, err := DeriveAPIKeyHashKey(master)
	if err != nil {
		t.Fatalf("DeriveAPIKeyHashKey second: %v", err)
	}
	if !bytes.Equal(key1, key2) {
		t.Fatal("derived API key hash keys should be deterministic")
	}
	if len(key1) != 32 {
		t.Fatalf("key length = %d, want 32", len(key1))
	}

	other, err := DeriveAPIKeyHashKey([]byte("fedcba9876543210fedcba9876543210"))
	if err != nil {
		t.Fatalf("DeriveAPIKeyHashKey other: %v", err)
	}
	if bytes.Equal(key1, other) {
		t.Fatal("different masters must not derive the same hash key")
	}
}

func TestDeriveHMACKeyRejectsShortMaster(t *testing.T) {
	t.Parallel()
	if _, err := DeriveHMACKey([]byte("short"), "info", 32); err == nil {
		t.Fatal("expected error for short master key")
	}
}
