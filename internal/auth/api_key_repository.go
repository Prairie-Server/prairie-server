package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/auditmutation"
	"github.com/prairie-server/prairie-server/internal/models"
)

// Sentinel errors for API key operations.
var (
	ErrAPIKeyNotFound = errors.New("api key not found")
)

// apiKeyColumns is the row projection. Neither the bearer nor its verifier is
// part of it; key_prefix is the non-secret "sa_" and first 8 hex digits.
const apiKeyColumns = `id, user_id, label, key_prefix, rate_tier, scopes, created_at, last_used_at` //nolint:gosec // SQL column names, not credentials.

// apiKeyBearerFormat is the shape generateAPIKey produces; only such a key has
// a display prefix.
var apiKeyBearerFormat = regexp.MustCompile(`^sa_[0-9a-f]{64}$`)

// APIKeyRepository provides CRUD operations for the api_keys table.
type APIKeyRepository struct {
	pool    *pgxpool.Pool
	hashKey []byte // HMAC key of Prairie's legacy api_key_hash; nil skips the legacy lookup
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

// scanAPIKeys scans multiple rows into a []*models.APIKey slice.
func scanAPIKeys(rows pgx.Rows) ([]*models.APIKey, error) {
	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		err := rows.Scan(
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
			return nil, fmt.Errorf("scanning api key row: %w", err)
		}
		keys = append(keys, &k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api key rows: %w", err)
	}
	return keys, nil
}

func (r *APIKeyRepository) hashAPIKey(key string) ([]byte, error) {
	if len(r.hashKey) == 0 {
		return nil, fmt.Errorf("api key hashing disabled")
	}
	h := hmac.New(sha256.New, r.hashKey)
	_, _ = h.Write([]byte(key))
	return h.Sum(nil), nil
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning api key creation: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	return insertAPIKey(ctx, tx, userID, label, scopes)
}

// CreateAsActor is Create for a key that actorID mints on account userID,
// refused when CheckOwnerTarget refuses actorID acting on that account. Both
// accounts stay share-locked, in id order as ownership transfers lock them,
// until the key is inserted: a promotion or transfer that commits first is
// what the check sees, and a later promotion of the account, or transfer of
// ownership to it, deletes the key with the account's other keys. An unknown account passes the check and fails the
// insert, as it does with Create.
func (r *APIKeyRepository) CreateAsActor(ctx context.Context, actorID, userID int, label string, scopes []string) (*models.APIKey, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning api key creation: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	// The audit row's accounts, an impersonator included, join that id order.
	rows, err := tx.Query(ctx, `SELECT id, role, is_owner FROM users
		WHERE id = ANY($1) ORDER BY id FOR SHARE`, append([]int{actorID, userID}, auditmutation.ActorIDs(ctx)...))
	if err != nil {
		return nil, fmt.Errorf("locking api key accounts: %w", err)
	}
	actor := OwnerActor{ID: actorID}
	var target *models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Role, &user.IsOwner); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning api key account: %w", err)
		}
		if user.ID == actorID {
			actor.IsOwner = user.IsOwner
		}
		if user.ID == userID {
			target = &user
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("locking api key accounts: %w", err)
	}
	if err := CheckOwnerTarget(actor, target); err != nil {
		return nil, err
	}
	return insertAPIKey(ctx, tx, userID, label, scopes)
}

// insertAPIKey inserts the key and its audit row in tx and commits tx.
func insertAPIKey(ctx context.Context, tx pgx.Tx, userID int, label string, scopes []string) (*models.APIKey, error) {
	key, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = []string{}
	}

	query := `INSERT INTO api_keys (user_id, label, key_hash, key_prefix, scopes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + apiKeyColumns

	created, err := scanAPIKey(tx.QueryRow(ctx, query, userID, label, HashLinkToken(key), key[:11], scopes))
	if err != nil {
		return nil, err
	}
	audit, err := recordAPIKeyMutation(ctx, tx, nil, &models.APIKeyMetadata{ID: created.ID, UserID: created.UserID,
		Label: created.Label, RateTier: created.RateTier, Scopes: created.Scopes})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing api key creation: %w", err)
	}
	auditmutation.CommitMutation(ctx, audit)
	// The bearer is disclosed once. No ordinary row projection carries it.
	created.Key = key
	return created, nil
}

// GetByKey looks up an API key by its full key string (including "sa_" prefix).
// A key Postgres cannot take as text matches no key: ErrAPIKeyNotFound, not
// the query error that would read as a store outage.
func (r *APIKeyRepository) GetByKey(ctx context.Context, key string) (*models.APIKey, error) {
	if !ValidTextKey(key) {
		return nil, ErrAPIKeyNotFound
	}
	// These bearers contain 256 random bits. The indexed SHA-256 digest is
	// both the lookup key and verifier, as for password-reset and invitation
	// link tokens. Do not tighten the public format here: legacy credentials
	// remain valid.
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE key_hash = $1`
	k, err := scanAPIKey(r.pool.QueryRow(ctx, query, HashLinkToken(key)))
	if !errors.Is(err, ErrAPIKeyNotFound) || len(r.hashKey) == 0 {
		return k, err
	}
	k, err = r.upgradeLegacyKey(ctx, key)
	if errors.Is(err, ErrAPIKeyNotFound) {
		// A concurrent request may have just upgraded the same legacy key.
		return scanAPIKey(r.pool.QueryRow(ctx, query, HashLinkToken(key)))
	}
	return k, err
}

// upgradeLegacyKey authenticates a key Prairie stored before SHA-256
// verifiers: only as an HMAC in api_key_hash with the bearer cleared, so
// 20261019181724 left its key_hash NULL. A match gets its key_hash and
// key_prefix now, while the bearer is at hand, and the HMAC is retired
// (20261102180000 lets the trigger accept that one fill).
func (r *APIKeyRepository) upgradeLegacyKey(ctx context.Context, key string) (*models.APIKey, error) {
	hmacHash, err := r.hashAPIKey(key)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if apiKeyBearerFormat.MatchString(key) {
		prefix = key[:11]
	}
	query := `UPDATE api_keys SET key_hash = $1, key_prefix = $2, api_key_hash = NULL, api_key_prefix = NULL
		WHERE key_hash IS NULL AND api_key_hash = $3
		RETURNING ` + apiKeyColumns
	return scanAPIKey(r.pool.QueryRow(ctx, query, HashLinkToken(key), prefix, hmacHash))
}

// Delete removes an API key owned by the given user.
func (r *APIKeyRepository) Delete(ctx context.Context, id int64, userID int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning api key deletion: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	// The audit row's account locks come before the key row lock, the order
	// an account deletion takes them in.
	if err := auditmutation.LockActors(ctx, tx); err != nil {
		return err
	}
	deleted, err := scanAPIKeyMetadata(tx.QueryRow(ctx, `DELETE FROM api_keys ak WHERE ak.id = $1 AND ak.user_id = $2 RETURNING `+apiKeyMetadataColumns, id, userID))
	if errors.Is(err, ErrAPIKeyNotFound) {
		return ErrAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}
	audit, err := recordAPIKeyMutation(ctx, tx, deleted, nil)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing api key deletion: %w", err)
	}
	auditmutation.CommitMutation(ctx, audit)
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
	query := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE id = $1`
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
