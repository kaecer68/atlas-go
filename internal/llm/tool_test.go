package llm

import (
	"context"
	"encoding/json"
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
