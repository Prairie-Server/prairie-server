package secret

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
)

const (
	// apiKeyHashHKDFInfo is the domain-separation label used to derive the
	// deterministic HMAC key for equality-looked-up API keys.
	apiKeyHashHKDFInfo = "silo/api-keys-hash/v1"

	apiKeyHashKeyLen = 32
)

// DeriveHMACKey derives a deterministic HMAC key from the master SECRET_KEY.
// The derived key is intended for equality lookup / blind-index hashing.
func DeriveHMACKey(masterKey []byte, hkdfInfo string, outLen int) ([]byte, error) {
	if len(masterKey) < MinMasterKeyLen {
		return nil, fmt.Errorf("secret: master key must be at least %d bytes, got %d", MinMasterKeyLen, len(masterKey))
	}
	if outLen <= 0 {
		return nil, fmt.Errorf("secret: outLen must be positive, got %d", outLen)
	}
	return hkdf.Key(sha256.New, masterKey, nil /* salt */, hkdfInfo, outLen)
}

// DeriveAPIKeyHashKey derives the deterministic HMAC key used to hash API
// keys stored in the api_keys table.
func DeriveAPIKeyHashKey(masterKey []byte) ([]byte, error) {
	return DeriveHMACKey(masterKey, apiKeyHashHKDFInfo, apiKeyHashKeyLen)
}
