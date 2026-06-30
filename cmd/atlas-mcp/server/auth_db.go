package server

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// TokenInfo is the deserialized row from atlas_mcp_tokens.
type TokenInfo struct {
	TokenID         uuid.UUID  `json:"token_id"`
	TokenHash       string     `json:"-"`
	TenantID        string     `json:"tenant_id"`
	AgentID         string     `json:"agent_id"`
	Scopes          []string   `json:"scopes"`
	RateLimitPerMin *int       `json:"rate_limit_per_min,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

// TokenRegistration is the input for registering a new MCP token.
type TokenRegistration struct {
	TenantID        string     `json:"tenant_id"`
	AgentID         string     `json:"agent_id"`
	Scopes          []string   `json:"scopes,omitempty"`
	RateLimitPerMin *int       `json:"rate_limit_per_min,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

// ErrRevoked is returned when a valid but revoked token is presented.
var ErrRevoked = errors.New("token revoked")

// ErrTokenNotFound is returned when the token is not in the store.
var ErrTokenNotFound = errors.New("token not found")

// ErrExpired is returned when a token has passed its expires_at time.
var ErrExpired = errors.New("token expired")

// ErrDBUnavailable is returned when the database is unreachable.
var ErrDBUnavailable = errors.New("database unavailable")

// TokenStore manages MCP bearer tokens.
type TokenStore interface {
	// Lookup finds a token by its raw bearer value and returns the associated
	// TokenInfo. Returns ErrTokenNotFound if unknown, ErrRevoked if revoked,
	// ErrExpired if expired, and ErrDBUnavailable if the database is unreachable.
	Lookup(ctx context.Context, rawToken string) (*TokenInfo, error)

	// Register creates a new token. Returns the generated TokenInfo (which
	// includes the raw token only in the response — the store only persists
	// the SHA-256 hash).
	Register(ctx context.Context, reg TokenRegistration) (*TokenInfo, string, error)

	// Revoke marks a token as revoked by its ID.
	Revoke(ctx context.Context, id uuid.UUID) error

	// Rotate generates a new raw token for the given ID, invalidating the old
	// one. Returns the new TokenInfo and raw token.
	Rotate(ctx context.Context, id uuid.UUID) (*TokenInfo, string, error)

	// List returns all non-revoked tokens (token_hash is redacted).
	List(ctx context.Context) ([]TokenInfo, error)
}
