package adapters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
)

// stubMiniMaxChatter implements miniMaxChatter for testing.
type stubMiniMaxChatter struct {
	chatFn func(ctx context.Context, model string, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error)
}

func (s *stubMiniMaxChatter) Chat(ctx context.Context, model string, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error) {
	return s.chatFn(ctx, model, messages, opts)
}

// TestMiniMaxAdapter_ConvertsPayloadToMessages verifies that req.Payload
// ([]byte JSON) is correctly unmarshaled and the messages are passed to the
// underlying client with model "MiniMax-M3".
func TestMiniMaxAdapter_ConvertsPayloadToMessages(t *testing.T) {
	var capturedMessages []clients.Message
	var capturedModel string

	stub := &stubMiniMaxChatter{
		chatFn: func(_ context.Context, model string, messages []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			capturedMessages = messages
			capturedModel = model
			return &clients.ChatResponse{Content: "ok", Model: model}, nil
		},
	}
	adapter := &MiniMaxAdapter{client: stub}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "Hello MiniMax"},
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
	if capturedModel != clients.DefaultModelMiniMaxM3 {
		t.Errorf("model = %q, want %q", capturedModel, clients.DefaultModelMiniMaxM3)
	}
	if len(capturedMessages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(capturedMessages))
	}
	if capturedMessages[0].Role != "user" || capturedMessages[0].Content != "Hello MiniMax" {
		t.Errorf("messages[0] = {%q, %q}, want {user, Hello MiniMax}", capturedMessages[0].Role, capturedMessages[0].Content)
	}
}

// TestMiniMaxAdapter_ConvertsResponseToLLMType verifies that a ChatResponse
// is correctly converted to an llm.Response with matching fields and
// ProviderMiniMax.
func TestMiniMaxAdapter_ConvertsResponseToLLMType(t *testing.T) {
	expectedUsage := llm.Usage{
		InputTokens:  200,
		OutputTokens: 80,
		TotalTokens:  280,
		CostUSD:      0.002,
	}
	stub := &stubMiniMaxChatter{
		chatFn: func(_ context.Context, _ string, _ []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			return &clients.ChatResponse{
				Content:      "MiniMax response",
				Model:        clients.DefaultModelMiniMaxM3,
				Usage:        expectedUsage,
				FinishReason: "stop",
			}, nil
		},
	}
	adapter := &MiniMaxAdapter{client: stub}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
	})
	req := llm.Request{Payload: payload}

	resp, err := adapter.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Output != "MiniMax response" {
		t.Errorf("Call().Output = %q, want %q", resp.Output, "MiniMax response")
	}
	if resp.Provider != llm.ProviderMiniMax {
		t.Errorf("Call().Provider = %q, want %q", resp.Provider, llm.ProviderMiniMax)
	}
	if resp.Usage != expectedUsage {
		t.Errorf("Call().Usage = %+v, want %+v", resp.Usage, expectedUsage)
	}
}
