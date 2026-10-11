package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/models"
)

// Sentinel errors for API key operations.
var (
	ErrAPIKeyNotFound = errors.New("api key not found")
)

const (
	// apiKeySelectColumns includes only non-secret fields persisted in DB.
	apiKeySelectColumns = `id, user_id, label, api_key_prefix, rate_tier, scopes, created_at, last_used_at`
	// apiKeyAuthSelectColumns includes the minimum needed for auth middleware.
	apiKeyAuthSelectColumns = `id, user_id, rate_tier, scopes`

	apiKeyPrefixDisplayLen = 10 // includes "sa_" prefix
)

// APIKeyRepository provides CRUD operations for the api_keys table.
type APIKeyRepository struct {
	pool    *pgxpool.Pool
	hashKey []byte // nil disables hash storage/lookup and falls back to plaintext
}

// NewAPIKeyRepository creates a new APIKeyRepository backed by the given pool.
func NewAPIKeyRepository(pool *pgxpool.Pool, hashKey []byte) *APIKeyRepository {
	return &APIKeyRepository{pool: pool, hashKey: hashKey}
}

// scanAPIKey scans a single row into a *models.APIKey.
func scanAPIKey(row pgx.Row) (*models.APIKey, error) {
	var k models.APIKey
	err := row.Scan(
		&k.ID,
		&k.UserID,
		&k.Label,
		&k.KeyPrefix,
		&k.RateTier,
		&k.Scopes,
		&k.CreatedAt,
		&k.LastUsedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("scanning api key: %w", err)
	}
	return &k, nil
}

// ValidTextKey reports whether a client-supplied lookup key (an API key or a
// session token) can be sent to Postgres as a text parameter: valid UTF-8
// with no NUL byte. Postgres rejects any other value with an encoding error
// (SQLSTATE 22021), which a caller would mistake for an unreachable store; a
// key that fails this check cannot match a stored one.
func ValidTextKey(key string) bool {
	return utf8.ValidString(key) && !strings.ContainsRune(key, 0)
}

// generateAPIKey creates a cryptographically random API key with the "sa_" prefix.
func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return "sa_" + hex.EncodeToString(b), nil
}

// Create generates a new API key for the given user and returns the full
// record. scopes may be nil or empty for an unscoped key (full access as the
// owning user); callers should validate scopes with NormalizeAPIKeyScopes
// first.
func (r *APIKeyRepository) Create(ctx context.Context, userID int, label string, scopes []string) (*models.APIKey, error) {
	key, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = []string{}
	}

	prefix := apiKeyPrefix(key)

	// Prefer deterministic equality-hash storage when enabled, so plaintext
	// key material can be cleared. If hashing is disabled (e.g., mis-wired
	// proxy-only nodes), fall back to storing plaintext so auth keeps working.
	var apiKeyValue interface{} = nil
	var hashValue []byte
	if len(r.hashKey) == 0 {
		apiKeyValue = key
	} else {
		hashValue, err = r.hashAPIKey(key)
		if err != nil {
			return nil, err
		}
	}

	query := `INSERT INTO api_keys (user_id, label, api_key, api_key_hash, api_key_prefix, scopes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + apiKeySelectColumns

	row := r.pool.QueryRow(ctx, query, userID, label, apiKeyValue, hashValue, prefix, scopes)
	out, scanErr := scanAPIKey(row)
	if scanErr != nil {
		return nil, scanErr
	}
	out.Key = key
	return out, nil
}

// GetByKey looks up an API key by its full key string (including "sa_" prefix).
// A key Postgres cannot take as text matches no key: ErrAPIKeyNotFound, not
// the query error that would read as a store outage.
func (r *APIKeyRepository) GetByKey(ctx context.Context, key string) (*models.APIKey, error) {
	if !ValidTextKey(key) {
		return nil, ErrAPIKeyNotFound
	}
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE api_key = $1`
	return scanAPIKey(r.pool.QueryRow(ctx, query, key))
}

// Delete removes an API key owned by the given user.
func (r *APIKeyRepository) Delete(ctx context.Context, id int64, userID int) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM api_keys WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// DeleteByAdmin removes any API key by its ID regardless of owner.
func (r *APIKeyRepository) DeleteByAdmin(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM api_keys WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// GetByID looks up an API key by its ID.
func (r *APIKeyRepository) GetByID(ctx context.Context, id int64) (*models.APIKey, error) {
	query := `SELECT ` + apiKeySelectColumns + ` FROM api_keys WHERE id = $1`
	return scanAPIKey(r.pool.QueryRow(ctx, query, id))
}

// UpdateTier updates the rate tier for an API key.
func (r *APIKeyRepository) UpdateTier(ctx context.Context, id int64, tier string) error {
	result, err := r.pool.Exec(ctx, `UPDATE api_keys SET rate_tier = $1 WHERE id = $2`, tier, id)
	if err != nil {
		return fmt.Errorf("update api key tier: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// UpdateLastUsed sets last_used_at to now. Intended to be called asynchronously.
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, "UPDATE api_keys SET last_used_at = NOW() WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("updating api key last_used_at: %w", err)
	}
	return nil
}

// BackfillPlaintextToHash converts legacy plaintext api_keys rows into the
// deterministic api_key_hash + api_key_prefix representation and clears the
// plaintext api_key value.
//
// Best-effort: rows that fail to update are left untouched and the backfill
// continues. Callers should treat the returned count as a lower bound.
func (r *APIKeyRepository) BackfillPlaintextToHash(ctx context.Context, batchSize int) (int, error) {
	if len(r.hashKey) == 0 {
		// Hashing disabled: nothing we can safely do.
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	total := 0
	for {
		rows, err := r.pool.Query(
			ctx,
			`SELECT id, api_key
			FROM api_keys
			WHERE api_key IS NOT NULL
			  AND (api_key_hash IS NULL OR api_key_prefix IS NULL)
			LIMIT $1`,
			batchSize,
		)
		if err != nil {
			return total, fmt.Errorf("api key backfill: select: %w", err)
		}

		var any bool
		for rows.Next() {
			any = true
			var id int64
			var apiKey string
			if err := rows.Scan(&id, &apiKey); err != nil {
				continue
			}

			hash, hErr := r.hashAPIKey(apiKey)
			if hErr != nil {
				continue
			}

			prefix := apiKeyPrefix(apiKey)
			tag, uErr := r.pool.Exec(
				ctx,
				`UPDATE api_keys
				 SET api_key_hash = $1,
				     api_key_prefix = $2,
				     api_key = NULL
				 WHERE id = $3 AND api_key IS NOT NULL`,
				hash, prefix, id,
			)
			if uErr != nil {
				continue
			}
			if tag.RowsAffected() > 0 {
				total++
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return total, fmt.Errorf("api key backfill: iterate: %w", err)
		}
		rows.Close()
		if !any {
			return total, nil
		}
	}
}
