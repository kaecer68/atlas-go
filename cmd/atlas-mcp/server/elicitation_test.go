package server

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func elicitClientOpts(handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ClientOptions {
	return &mcp.ClientOptions{
		ElicitationHandler: handler,
		Capabilities: &mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{
				Form: &mcp.FormElicitationCapabilities{},
				URL:  &mcp.URLElicitationCapabilities{},
			},
		},
	}
}

func TestMCPElicitUser_Form_Success(t *testing.T) {
	clientOpts := elicitClientOpts(func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		require.Equal(t, "form", req.Params.Mode)
		require.Equal(t, "Enter values", req.Params.Message)
		require.NotNil(t, req.Params.RequestedSchema)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"name": "alice"}}, nil
	})

	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, out, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Message: "Enter values",
		Schema:  map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
	})
	require.NoError(t, err)
	require.Equal(t, "accept", out.Action)
	require.Equal(t, map[string]any{"name": "alice"}, out.Content)
}

func TestMCPElicitUser_URL_Success(t *testing.T) {
	clientOpts := elicitClientOpts(func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		require.Equal(t, "url", req.Params.Mode)
		require.Equal(t, "https://example.com/confirm", req.Params.URL)
		require.Equal(t, "confirm-1", req.Params.ElicitationID)
		return &mcp.ElicitResult{Action: "accept"}, nil
	})

	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, out, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Message:       "Please confirm",
		URL:           "https://example.com/confirm",
		ElicitationID: "confirm-1",
	})
	require.NoError(t, err)
	require.Equal(t, "accept", out.Action)
}

func TestMCPElicitUser_ClientLacksCapability_Error(t *testing.T) {
	clientOpts := &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}
	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, _, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Message: "test",
		Schema:  map[string]any{"type": "object"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support elicitation")
}

func TestMCPElicitUser_EmptyMessage_Error(t *testing.T) {
	clientOpts := elicitClientOpts(func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("should not be called")
	})
	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, _, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Schema: map[string]any{"type": "object"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "message is required")
}

func TestMCPElicitUser_FormMissingSchema_Error(t *testing.T) {
	clientOpts := elicitClientOpts(func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("should not be called")
	})
	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, _, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Message: "test",
		Mode:    "form",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema")
}

func TestMCPElicitUser_URLMissingID_Error(t *testing.T) {
	clientOpts := elicitClientOpts(func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("should not be called")
	})
	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, _, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Message: "test",
		Mode:    "url",
		URL:     "https://example.com/confirm",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "elicitation_id")
}

func TestMCPElicitUser_HandlerReturnsError(t *testing.T) {
	clientOpts := elicitClientOpts(func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("client refused")
	})
	s, _, ss, done := newTestServerWithClient(t, clientOpts, Config{ElicitationEnabled: true})
	defer done()

	_, _, err := s.handleMCPElicitUser(context.Background(), &mcp.CallToolRequest{Session: ss}, MCPElicitUserInput{
		Message: "test",
		Schema:  map[string]any{"type": "object"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "client refused")
}
