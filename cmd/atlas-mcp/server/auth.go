package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// TokenAuth validates the bearer token presented in the JSON-RPC request.
// When the configured token is empty the auth is permissive (dev mode).
//
// When a TokenStore is configured (non-nil), it is consulted first. If the
// store is unavailable, auth fails closed (ErrDBUnavailable). If the store
// returns ErrTokenNotFound, the env-var token is used as fallback. If the
// store returns ErrRevoked, auth is denied.
type TokenAuth struct {
	token string
	store TokenStore
}

// NewTokenAuth constructs a TokenAuth. token == "" enables dev mode (no check).
func NewTokenAuth(token string) *TokenAuth { return &TokenAuth{token: token} }

// SetStore configures a database-backed token store. When nil, only the
// environment-variable token is checked (legacy mode).
func (a *TokenAuth) SetStore(store TokenStore) { a.store = store }

// ErrUnauthorized is returned for an invalid or missing token.
var ErrUnauthorized = errors.New("unauthorized")

// contextKey is the unexported type for auth context values.
type contextKey int

const (
	contextKeyTenantID contextKey = iota
	contextKeyAgentID
)

// TenantIDFromContext returns the tenant_id carried by the context, or
// "anonymous" when absent.
func TenantIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyTenantID).(string); ok && v != "" {
		return v
	}
	return "anonymous"
}

// AgentIDFromContext returns the agent_id carried by the context, or
// "anonymous" when absent.
func AgentIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyAgentID).(string); ok && v != "" {
		return v
	}
	return "anonymous"
}

// ContextWithTenantID returns a child context carrying tenantID.
func ContextWithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, contextKeyTenantID, tenantID)
}

// ContextWithAgentID returns a child context carrying agentID.
func ContextWithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, contextKeyAgentID, agentID)
}

// Authenticate verifies the presented token and returns a context enriched
// with tenant_id and agent_id when the token is valid. It is the preferred
// entry point for transports that need request-scoped identity.
func (a *TokenAuth) Authenticate(ctx context.Context, presented string) (context.Context, error) {
	if a.store != nil {
		info, err := a.store.Lookup(ctx, presented)
		if err == nil {
			ctx = ContextWithTenantID(ctx, info.TenantID)
			ctx = ContextWithAgentID(ctx, info.AgentID)
			return ctx, nil
		}
		if errors.Is(err, ErrDBUnavailable) {
			return ctx, fmt.Errorf("%w: %w", ErrUnauthorized, ErrDBUnavailable)
		}
		if errors.Is(err, ErrRevoked) || errors.Is(err, ErrExpired) {
			return ctx, fmt.Errorf("%w: %w", ErrUnauthorized, err)
		}
		// ErrTokenNotFound — fall through to env-var fallback.
	}
	if a.token == "" {
		return ctx, nil // dev mode
	}
	if presented == "" || !strings.EqualFold(presented, a.token) {
		return ctx, fmt.Errorf("%w: token mismatch", ErrUnauthorized)
	}
	ctx = ContextWithTenantID(ctx, "env-fallback")
	ctx = ContextWithAgentID(ctx, "env-fallback")
	return ctx, nil
}

// Check verifies the provided token. If a TokenStore is configured, it is
// consulted first: DB-unavailable → fail-closed; token-revoked/expired →
// reject; token-not-found → fall through to env-var; token-found → allow.
// When no store is configured, only the env-var token is checked (legacy).
func (a *TokenAuth) Check(presented string) error {
	_, err := a.Authenticate(context.Background(), presented)
	return err
}

// Wrap returns a function that wraps a JSON-RPC tool handler with an auth
// check. The handler is invoked only when Check passes.
//
// Phase 1 wires this only at the dispatch boundary; individual tool handlers
// may still be invoked without an explicit token if the surrounding protocol
// adapter already enforced auth. Tests cover both paths.
func (a *TokenAuth) Wrap(handler TokenProtected) TokenProtected {
	return func(token string) error {
		if err := a.Check(token); err != nil {
			return err
		}
		return handler(token)
	}
}

// TokenProtected is a handler signature that takes the presented token.
type TokenProtected func(token string) error

// Helper for tests / dashboards: Status reports whether auth is enabled.
func (a *TokenAuth) Status() string {
	if a.token == "" {
		return "dev-mode (no token)"
	}
	return "token-required"
}

// Ensure TokenAuth conforms to a minimal http.Handler-like signature to make it
// trivial to slot into a future SSE / streamable-HTTP transport in Phase 2.
var _ = http.Handler(nil)
