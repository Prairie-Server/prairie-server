package models

import "time"

// APIKey represents a row in the api_keys table.
type APIKey struct {
	ID     int64
	UserID int
	Label  string
	// Key is the full plaintext credential, only populated for key-creation
	// responses. It is never stored at rest once api_key_hash backfill is
	// enabled.
	Key string
	// KeyPrefix is a non-secret identifier for UI display (and cannot be used
	// as a credential).
	KeyPrefix  string
	RateTier   string
	CreatedAt  time.Time
	LastUsedAt *time.Time // nil if never used
}

// APIKeyWithUser extends APIKey with the owning user's username.
type APIKeyWithUser struct {
	APIKey
	Username string
}
