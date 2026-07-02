package server

// MCP transport dispatchers — stdio (default, Phase 1) and SSE +
// streamable-HTTP (Phase 4, with Bearer auth middleware). All three
// Serve* functions are constructed atop the same *mcp.Server so tool
// registration, rate limiter, audit, metrics, anomaly detector, and
// the 82–84 RegisteredToolCount assertion remain transport-agnostic.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Transport names accepted by Config.Transport. Stable, lowercase, and
// exported so external callers (main.go, scripts, tests) can reference
// them without typo risk.
const (
	TransportStdio          = "stdio"
	TransportSSE            = "sse"
	TransportStreamableHTTP = "streamable-http"
)

// BearerAuth returns an HTTP middleware that enforces bearer-token auth
// against the supplied TokenAuth. When TokenAuth is in dev mode (empty
// token), the middleware is permissive — useful for local development.
//
// Bearer extraction is intentionally strict: only `Bearer <token>` with
// a literal space is recognized. Any other scheme (Basic, Digest, bare
// token) is rejected as 401. Case-sensitive per RFC 6750 §2.1.
func BearerAuth(auth *TokenAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := extractBearer(r.Header.Get("Authorization"))
			if _, err := auth.Authenticate(r.Context(), presented); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractBearer returns the token portion of an `Authorization: Bearer <tok>`
// header. Returns "" for missing, malformed, or non-Bearer schemes.
func extractBearer(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// ServeStdio runs the MCP server over stdio until ctx is done. This is
// the default transport for backwards compatibility with Claude Desktop,
// Cursor, OpenCode, and any other MCP client that pipes JSON-RPC over
// stdin/stdout.
func ServeStdio(ctx context.Context, srv *mcp.Server) error {
	if srv == nil {
		return fmt.Errorf("server: ServeStdio: nil MCP server")
	}
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// ServeSSE runs the MCP server over the SSE transport (MCP 2024-11-05
// spec). Note: the SSE transport has been deprecated by the MCP spec
// (superseded by streamable-HTTP), but we still ship it for clients that
// have not migrated yet. addr must be non-empty (e.g. "127.0.0.1:9090").
func ServeSSE(ctx context.Context, srv *mcp.Server, addr string, auth *TokenAuth) error {
	if srv == nil {
		return fmt.Errorf("server: ServeSSE: nil MCP server")
	}
	if addr == "" {
		return fmt.Errorf("server: ServeSSE: addr is required (e.g. \"127.0.0.1:9090\")")
	}
	sse := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	return listenHTTP(ctx, addr, BearerAuth(auth)(sse))
}

// ServeStreamableHTTP runs the MCP server over the streamable-HTTP
// transport (MCP 2025-03-26 spec, the current MCP standard). addr must
// be non-empty (e.g. "127.0.0.1:9090"). We always bind 127.0.0.1 by
// convention; remote exposure should sit behind a reverse proxy with
// TLS termination.
func ServeStreamableHTTP(ctx context.Context, srv *mcp.Server, addr string, auth *TokenAuth) error {
	if srv == nil {
		return fmt.Errorf("server: ServeStreamableHTTP: nil MCP server")
	}
	if addr == "" {
		return fmt.Errorf("server: ServeStreamableHTTP: addr is required (e.g. \"127.0.0.1:9090\")")
	}
	httpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	return listenHTTP(ctx, addr, BearerAuth(auth)(httpHandler))
}

// listenHTTP binds an http.Server with a 5s graceful shutdown on ctx
// cancellation. Returns nil when shutdown completes cleanly.
func listenHTTP(ctx context.Context, addr string, handler http.Handler) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: graceful shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
