package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
)

// stubDeepSeekChatter implements deepSeekChatter for testing.
type stubDeepSeekChatter struct {
	chatFn func(ctx context.Context, model string, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error)
}

func (s *stubDeepSeekChatter) Chat(ctx context.Context, model string, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error) {
	return s.chatFn(ctx, model, messages, opts)
}

// TestDeepSeekAdapter_ConvertsPayloadToMessages verifies that req.Payload
// ([]byte JSON) is correctly unmarshaled and the messages are passed to the
// underlying client.
func TestDeepSeekAdapter_ConvertsPayloadToMessages(t *testing.T) {
	var capturedMessages []clients.Message
	var capturedModel string

	stub := &stubDeepSeekChatter{
		chatFn: func(_ context.Context, model string, messages []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			capturedMessages = messages
			capturedModel = model
			return &clients.ChatResponse{Content: "ok", Model: model}, nil
		},
	}
	adapter := &DeepSeekAdapter{
		client: stub,
		Model:  "deepseek-v4-pro",
	}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there"},
		},
	})
	req := llm.Request{Payload: payload}

	resp, err := adapter.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Output != "ok" {
		t.Errorf("Call().Output = %q, want %q", resp.Output, "ok")
	}
	if capturedModel != "deepseek-v4-pro" {
		t.Errorf("model = %q, want %q", capturedModel, "deepseek-v4-pro")
	}
	if len(capturedMessages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(capturedMessages))
	}
	if capturedMessages[0].Role != "user" || capturedMessages[0].Content != "Hello" {
		t.Errorf("messages[0] = {%q, %q}, want {user, Hello}", capturedMessages[0].Role, capturedMessages[0].Content)
	}
}

// TestDeepSeekAdapter_ConvertsResponseToLLMType verifies that a ChatResponse
// is correctly converted to an llm.Response with matching Output, Usage, and
// Provider fields.
func TestDeepSeekAdapter_ConvertsResponseToLLMType(t *testing.T) {
	expectedUsage := llm.Usage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		CostUSD:      0.001,
	}
	stub := &stubDeepSeekChatter{
		chatFn: func(_ context.Context, _ string, _ []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			return &clients.ChatResponse{
				Content:      "DeepSeek response",
				Model:        "deepseek-v4-pro",
				Usage:        expectedUsage,
				FinishReason: "stop",
			}, nil
		},
	}
	adapter := &DeepSeekAdapter{
		client: stub,
		Model:  "deepseek-v4-pro",
	}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
	})
	req := llm.Request{Payload: payload}

	resp, err := adapter.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Output != "DeepSeek response" {
		t.Errorf("Call().Output = %q, want %q", resp.Output, "DeepSeek response")
	}
	if resp.Provider != llm.ProviderDeepSeek {
		t.Errorf("Call().Provider = %q, want %q", resp.Provider, llm.ProviderDeepSeek)
	}
	if resp.Usage != expectedUsage {
		t.Errorf("Call().Usage = %+v, want %+v", resp.Usage, expectedUsage)
	}
}

// TestDeepSeekAdapter_InvalidPayload verifies that non-[]byte payloads
// produce an error.
func TestDeepSeekAdapter_InvalidPayload(t *testing.T) {
	adapter := NewDeepSeekAdapter(nil, "deepseek-v4-pro")

	req := llm.Request{Payload: "not bytes"}
	_, err := adapter.Call(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for non-[]byte payload, got nil")
	}
}

// TestDeepSeekAdapter_EmptyMessages verifies that a payload with an empty
// messages array produces an error.
func TestDeepSeekAdapter_EmptyMessages(t *testing.T) {
	adapter := NewDeepSeekAdapter(nil, "deepseek-v4-pro")

	payload, _ := json.Marshal(map[string]any{"messages": []any{}})
	req := llm.Request{Payload: payload}
	_, err := adapter.Call(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty messages, got nil")
	}
}

// TestDeepSeekAdapter_ClientError verifies that client errors are propagated.
func TestDeepSeekAdapter_ClientError(t *testing.T) {
	stub := &stubDeepSeekChatter{
		chatFn: func(_ context.Context, _ string, _ []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			return nil, fmt.Errorf("upstream failure")
		},
	}
	adapter := &DeepSeekAdapter{client: stub, Model: "deepseek-v4-pro"}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
	})
	req := llm.Request{Payload: payload}

	_, err := adapter.Call(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from client, got nil")
	}
}
