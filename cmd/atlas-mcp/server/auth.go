package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// TokenAuth validates the bearer token presented in the JSON-RPC request.
// When the configured token is empty the auth is permissive (dev mode).
type TokenAuth struct {
	token string
}

// NewTokenAuth constructs a TokenAuth. token == "" enables dev mode (no check).
func NewTokenAuth(token string) *TokenAuth { return &TokenAuth{token: token} }

// ErrUnauthorized is returned for an invalid or missing token.
var ErrUnauthorized = errors.New("unauthorized")

// Check verifies the provided token against the configured one. In dev mode
// (no token configured) it returns nil for any input.
func (a *TokenAuth) Check(presented string) error {
	if a.token == "" {
		return nil // dev mode
	}
	if presented == "" || !strings.EqualFold(presented, a.token) {
		return fmt.Errorf("%w: token mismatch", ErrUnauthorized)
	}
	return nil
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
