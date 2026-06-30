package main

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/cmd/atlas-mcp/server"
)

// TestPackageCompiles is a build-time smoke that the package imports cleanly
// and the public surface (server.Config, server.Run) is reachable.
func TestPackageCompiles(t *testing.T) {
	// Bare type and function references keep the test stable across implementations.
	var _ = server.Config{}
	var _ = server.Run
	_, cancel := context.WithCancel(context.Background())
	cancel()
}
