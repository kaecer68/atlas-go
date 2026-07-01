package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSpike_SamplingAPISurface verifies that ServerSession.CreateMessage exists
// and can be called through the go-mcp-sdk v1.6.1 API.
func TestSpike_SamplingAPISurface(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.1.0"}, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "sampled"},
				Model:   "test-model",
				Role:    "assistant",
			}, nil
		},
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	if _, err := client.Connect(ctx, ct, nil); err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := callSampling(ctx, ss)
	if err != nil {
		t.Fatalf("callSampling: %v", err)
	}
	if res == nil {
		t.Fatal("callSampling returned nil result")
	}
	text, ok := res.Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", res.Content)
	}
	if text.Text != "sampled" {
		t.Fatalf("unexpected text %q", text.Text)
	}
}

// TestSpike_RootsAPISurface verifies that ServerSession.ListRoots and
// ServerOptions.RootsListChangedHandler exist and can be wired.
func TestSpike_RootsAPISurface(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	changed := make(chan struct{}, 1)
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.1.0"}, &mcp.ServerOptions{
		RootsListChangedHandler: func(_ context.Context, _ *mcp.RootsListChangedRequest) {
			select {
			case changed <- struct{}{}:
			default:
			}
		},
	})
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
	client.AddRoots(&mcp.Root{URI: "file:///tmp/atlas-root", Name: "atlas-workspace"})

	if _, err := client.Connect(ctx, ct, nil); err != nil {
		t.Fatalf("client connect: %v", err)
	}

	roots, err := callRoots(ctx, ss)
	if err != nil {
		t.Fatalf("callRoots: %v", err)
	}
	if roots == nil {
		t.Fatal("callRoots returned nil result")
	}
	if len(roots.Roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots.Roots))
	}
	if roots.Roots[0].URI != "file:///tmp/atlas-root" {
		t.Fatalf("unexpected root URI %q", roots.Roots[0].URI)
	}

	client.AddRoots(&mcp.Root{URI: "file:///tmp/atlas-root-2"})
	select {
	case <-changed:
		// expected
	case <-ctx.Done():
		t.Fatal("timed out waiting for roots list changed notification")
	}
}

// TestSpike_ElicitationAPISurface verifies that ServerSession.Elicit and
// ClientOptions.ElicitationHandler exist and can be wired.
func TestSpike_ElicitationAPISurface(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.1.0"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"answer": "42"},
			}, nil
		},
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	if _, err := client.Connect(ctx, ct, nil); err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := callElicitation(ctx, ss)
	if err != nil {
		t.Fatalf("callElicitation: %v", err)
	}
	if res == nil {
		t.Fatal("callElicitation returned nil result")
	}
	if res.Action != "accept" {
		t.Fatalf("unexpected action %q", res.Action)
	}
	if got := res.Content["answer"]; got != "42" {
		t.Fatalf("unexpected content answer %v", got)
	}
}
