package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/prairie-server/prairie-server/internal/models"
)

// Sentinel errors for authentication operations.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user account is disabled")
)

// Credentials holds the username and password for authentication.
type Credentials struct {
	Username string
	Password string
}

// AuthProvider is the interface for pluggable authentication backends.
// Implementations validate user credentials and session state.
type AuthProvider interface {
	// Authenticate validates the given credentials and returns the
	// authenticated user. Returns ErrInvalidCredentials if the username
	// does not exist or the password is wrong. Returns ErrUserDisabled
	// if the account is disabled.
	Authenticate(ctx context.Context, credentials Credentials) (*models.User, error)

	// ValidateSession checks whether the given session ID is still valid
	// (exists, not revoked, not expired).
	ValidateSession(ctx context.Context, sessionID string) (bool, error)
}

// LocalProvider authenticates users against the local PostgreSQL database
// using bcrypt password hashing.
type LocalProvider struct {
	users    *UserRepository
	sessions *SessionRepository
}

// NewLocalProvider creates a new LocalProvider backed by the given repositories.
func NewLocalProvider(users *UserRepository, sessions *SessionRepository) *LocalProvider {
	return &LocalProvider{
		users:    users,
		sessions: sessions,
	}
}

// Authenticate validates the username/password pair against the database.
// Returns ErrInvalidCredentials if the user is not found or the password
// does not match. Returns ErrUserDisabled if the user's account is disabled.
func (p *LocalProvider) Authenticate(ctx context.Context, creds Credentials) (*models.User, error) {
	user, err := p.users.GetByUsername(ctx, creds.Username)
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("looking up user: %w", err)
	}
	if !user.LocalPasswordLoginEnabled {
		return nil, ErrInvalidCredentials
	}

	if !CheckPassword(user, creds.Password) {
		return nil, ErrInvalidCredentials
	}

	if !user.Enabled {
		return nil, ErrUserDisabled
	}

	return user, nil
}

// ValidateSession checks whether the session identified by sessionID is
// currently valid (exists, not revoked, not expired).
func (p *LocalProvider) ValidateSession(ctx context.Context, sessionID string) (bool, error) {
	return p.sessions.IsValid(ctx, sessionID)
}
