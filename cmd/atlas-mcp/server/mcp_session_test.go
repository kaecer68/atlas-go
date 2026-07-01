package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// testImpl is a minimal MCP implementation for in-memory tests.
var testImpl = &mcp.Implementation{Name: "atlas-mcp-test", Version: "v0.0.1"}

// newTestAuditWriter creates a temporary audit log writer for tests.
func newTestAuditWriter(t *testing.T) *AuditWriter {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// newTestServerWithClient wires an in-memory MCP client/server pair and
// returns the atlas server wrapper, the connected client, the server session,
// and a cleanup function. Use this helper when a test needs to mutate client
// state such as roots.
func newTestServerWithClient(t *testing.T, clientOpts *mcp.ClientOptions, cfg Config) (*server, *mcp.Client, *mcp.ServerSession, func()) {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	mcpSrv := mcp.NewServer(testImpl, nil)
	ss, err := mcpSrv.Connect(ctx, st, nil)
	require.NoError(t, err)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, clientOpts)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)

	audit := newTestAuditWriter(t)
	cfg.AuditLogPath = audit.path
	s := &server{
		cfg:   cfg,
		audit: audit,
		cli:   newHTTPClient(cfg),
	}

	cleanup := func() {
		_ = cs.Close()
		_ = ss.Close()
		_ = audit.Close()
	}
	return s, client, ss, cleanup
}

// newTestServerWithSession wires an in-memory MCP client/server pair and
// returns the atlas server wrapper, the server session, and a cleanup function.
func newTestServerWithSession(t *testing.T, clientOpts *mcp.ClientOptions, cfg Config) (*server, *mcp.ServerSession, func()) {
	s, _, ss, done := newTestServerWithClient(t, clientOpts, cfg)
	return s, ss, done
}
