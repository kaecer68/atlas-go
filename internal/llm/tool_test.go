package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTool_RoundTrip_Fields(t *testing.T) {
	tool := Tool{
		Name:        "get_weather",
		Description: "Look up the weather for a city",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"temp":72,"condition":"sunny"}`), nil
		},
	}

	if tool.Name != "get_weather" {
		t.Errorf("Name = %q, want get_weather", tool.Name)
	}
	if tool.Description == "" {
		t.Error("Description should not be empty")
	}
	if len(tool.InputSchema) == 0 {
		t.Error("InputSchema should not be empty")
	}
	if tool.Handler == nil {
		t.Error("Handler should not be nil")
	}
}

func TestTool_Handler_InvokableWithJSONArgs(t *testing.T) {
	called := false
	tool := Tool{
		Name:        "echo",
		Description: "Echo back the input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			called = true
			return args, nil
		},
	}

	got, err := tool.Handler(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	if !called {
		t.Error("Handler was not called")
	}
	if string(got) != `{"msg":"hi"}` {
		t.Errorf("Handler returned %q, want {\"msg\":\"hi\"}", string(got))
	}
}

func TestRequest_ToolsAndToolChoice(t *testing.T) {
	tool := Tool{
		Name:        "lookup",
		Description: "Look up data",
		InputSchema: json.RawMessage(`{}`),
	}
	req := Request{
		Capability: CapabilityFailureAttribution,
		Tools:      []Tool{tool},
		ToolChoice: "auto",
	}

	if len(req.Tools) != 1 {
		t.Errorf("Tools len = %d, want 1", len(req.Tools))
	}
	if req.ToolChoice != "auto" {
		t.Errorf("ToolChoice = %q, want auto", req.ToolChoice)
	}
}

func TestResponse_ToolCalls(t *testing.T) {
	resp := Response{
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "get_weather", Arguments: json.RawMessage(`{"city":"Taipei"}`)},
		},
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" {
		t.Errorf("ID = %q, want call_1", resp.ToolCalls[0].ID)
	}
}

// TestRequest_Validate_ToolChoice covers the 5 ToolChoice validation
// cases introduced by Issue #711 #11. Provider adapters will call
// Request.Validate() before dispatch (PR5a) and trust the input on
// nil return.
func TestRequest_Validate_ToolChoice(t *testing.T) {
	makeTools := func() []Tool {
		return []Tool{
			{Name: "get_weather", Description: "Look up weather"},
			{Name: "get_stock_price", Description: "Look up stock price"},
		}
	}

	tests := []struct {
		name       string
		toolChoice string
		tools      []Tool
		wantErr    bool
		errSubstr  string
	}{
		{
			name:       "empty (provider default)",
			toolChoice: "",
			tools:      makeTools(),
			wantErr:    false,
		},
		{
			name:       "reserved keyword: none",
			toolChoice: "none",
			tools:      makeTools(),
			wantErr:    false,
		},
		{
			name:       "reserved keyword: auto",
			toolChoice: "auto",
			tools:      makeTools(),
			wantErr:    false,
		},
		{
			name:       "reserved keyword: required",
			toolChoice: "required",
			tools:      makeTools(),
			wantErr:    false,
		},
		{
			name:       "matching tool name",
			toolChoice: "get_stock_price",
			tools:      makeTools(),
			wantErr:    false,
		},
		{
			name:       "non-matching tool name",
			toolChoice: "get_nonexistent",
			tools:      makeTools(),
			wantErr:    true,
			errSubstr:  "get_nonexistent",
		},
		{
			name:       "garbage string with no tools",
			toolChoice: "definitely-not-a-tool",
			tools:      nil,
			wantErr:    true,
			errSubstr:  "definitely-not-a-tool",
		},
		{
			name:       "garbage string with empty tools slice",
			toolChoice: "anything",
			tools:      []Tool{},
			wantErr:    true,
			errSubstr:  "anything",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{Tools: tc.tools, ToolChoice: tc.toolChoice}
			err := req.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error containing %q", tc.errSubstr)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err.Error(), tc.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			}
		})
	}
}
