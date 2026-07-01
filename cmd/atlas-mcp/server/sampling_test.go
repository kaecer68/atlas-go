package server

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestMCPSampleLLM_DisabledByDefault(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	mcpSrv := mcp.NewServer(testImpl, nil)
	s := &server{cfg: Config{SamplingEnabled: false}}
	registerSamplingTools(mcpSrv, s)

	ss, err := mcpSrv.Connect(ctx, st, nil)
	require.NoError(t, err)
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	defer cs.Close()

	_, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "mcp_sample_llm",
		Arguments: map[string]any{"messages": []any{map[string]any{"role": "user", "content": map[string]any{"type": "text", "text": "hi"}}}, "max_tokens": 10},
	})
	require.Error(t, err)
}

func TestMCPSampleLLM_ClientSupportsSampling_CallsCreateMessage(t *testing.T) {
	var got *mcp.CreateMessageParams
	clientOpts := &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			got = req.Params
			return &mcp.CreateMessageResult{
				Model:   "test-model",
				Role:    "assistant",
				Content: &mcp.TextContent{Text: "hello"},
			}, nil
		},
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{}},
	}

	s, ss, done := newTestServerWithSession(t, clientOpts, Config{SamplingEnabled: true})
	defer done()

	ctx := ContextWithTenantID(context.Background(), "tenant-1")
	req := &mcp.CallToolRequest{Session: ss}
	in := MCPSampleLLMInput{
		Messages:  []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}},
		MaxTokens: 42,
	}

	_, out, err := s.handleMCPSampleLLM(ctx, req, in)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(42), got.MaxTokens)
	require.Len(t, got.Messages, 1)
	require.Equal(t, "test-model", out.Model)
	require.Equal(t, mcp.Role("assistant"), out.Role)
}

func TestMCPSampleLLM_WithTools_CallsCreateMessageWithTools(t *testing.T) {
	var got *mcp.CreateMessageWithToolsParams
	clientOpts := &mcp.ClientOptions{
		CreateMessageWithToolsHandler: func(_ context.Context, req *mcp.CreateMessageWithToolsRequest) (*mcp.CreateMessageWithToolsResult, error) {
			got = req.Params
			return &mcp.CreateMessageWithToolsResult{
				Model:   "test-model",
				Role:    "assistant",
				Content: []mcp.Content{&mcp.TextContent{Text: "with-tools"}},
			}, nil
		},
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{Tools: &mcp.SamplingToolsCapabilities{}}},
	}

	s, ss, done := newTestServerWithSession(t, clientOpts, Config{SamplingEnabled: true})
	defer done()

	req := &mcp.CallToolRequest{Session: ss}
	in := MCPSampleLLMInput{
		Messages: []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}},
		Tools:    []*mcp.Tool{{Name: "calc"}},
	}

	_, _, err := s.handleMCPSampleLLM(context.Background(), req, in)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "calc", got.Tools[0].Name)
}

func TestMCPSampleLLM_ClientLacksSamplingCapability_ReturnsError(t *testing.T) {
	clientOpts := &mcp.ClientOptions{}
	s, ss, done := newTestServerWithSession(t, clientOpts, Config{SamplingEnabled: true})
	defer done()

	req := &mcp.CallToolRequest{Session: ss}
	in := MCPSampleLLMInput{Messages: []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}}}

	_, _, err := s.handleMCPSampleLLM(context.Background(), req, in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sampling")
}

func TestMCPSampleLLM_EmitsAuditEntry(t *testing.T) {
	clientOpts := &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{Model: "m", Content: &mcp.TextContent{Text: "x"}}, nil
		},
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{}},
	}

	s, ss, done := newTestServerWithSession(t, clientOpts, Config{SamplingEnabled: true})
	defer done()

	_, _, err := s.handleMCPSampleLLM(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPSampleLLMInput{
		Messages:  []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}},
		MaxTokens: 10,
	})
	require.NoError(t, err)

	entries, rerr := ReadAuditEntries(s.audit.path, 0, time.Now())
	require.NoError(t, rerr)
	require.Len(t, entries, 1)
	require.Equal(t, "mcp_sample_llm", entries[0].Tool)
	require.Equal(t, "ok", entries[0].Status)
}

func TestMCPSampleLLM_ContextTimeout_ReturnsDeadlineError(t *testing.T) {
	clientOpts := &mcp.ClientOptions{
		CreateMessageHandler: func(ctx context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, context.DeadlineExceeded
			}
		},
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{}},
	}

	s, ss, done := newTestServerWithSession(t, clientOpts, Config{SamplingEnabled: true})
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := s.handleMCPSampleLLM(ctx, &mcp.CallToolRequest{Session: ss}, MCPSampleLLMInput{
		Messages: []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMCPSampleLLM_EmptyMessages_ValidationError(t *testing.T) {
	clientOpts := &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{Sampling: &mcp.SamplingCapabilities{}},
	}

	s, ss, done := newTestServerWithSession(t, clientOpts, Config{SamplingEnabled: true})
	defer done()

	_, _, err := s.handleMCPSampleLLM(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPSampleLLMInput{
		Messages:  []*mcp.SamplingMessage{},
		MaxTokens: 10,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "messages")
}
