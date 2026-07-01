// Package main is a temporary SDK spike for go-mcp-sdk v1.6.1 sampling/roots/elicitation.
// It will be removed in T2.2-T2.4 when production handlers replace it.
package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	fmt.Println("atlas-mcp-server-sdk-spike: compile-time API surface verification")
}

// callSampling exercises ServerSession.CreateMessage (sampling primitive).
func callSampling(ctx context.Context, ss *mcp.ServerSession) (*mcp.CreateMessageResult, error) {
	return ss.CreateMessage(ctx, &mcp.CreateMessageParams{
		Messages: []*mcp.SamplingMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: "hello"},
			},
		},
		MaxTokens: 64,
	})
}

// callRoots exercises ServerSession.ListRoots (roots primitive).
func callRoots(ctx context.Context, ss *mcp.ServerSession) (*mcp.ListRootsResult, error) {
	return ss.ListRoots(ctx, &mcp.ListRootsParams{})
}

// callElicitation exercises ServerSession.Elicit (elicitation primitive).
func callElicitation(ctx context.Context, ss *mcp.ServerSession) (*mcp.ElicitResult, error) {
	return ss.Elicit(ctx, &mcp.ElicitParams{
		Mode:    "form",
		Message: "Please confirm",
	})
}
