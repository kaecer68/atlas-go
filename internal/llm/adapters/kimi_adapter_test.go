package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
)

// stubKimiChatter implements kimiChatter for testing.
type stubKimiChatter struct {
	chatFn func(ctx context.Context, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error)
}

func (s *stubKimiChatter) Chat(ctx context.Context, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error) {
	return s.chatFn(ctx, messages, opts)
}

// TestKimiAdapter_ConvertsPayloadToMessages verifies that req.Payload
// ([]byte JSON) is correctly unmarshaled and the messages are passed to the
// underlying client.
func TestKimiAdapter_ConvertsPayloadToMessages(t *testing.T) {
	var capturedMessages []clients.Message

	stub := &stubKimiChatter{
		chatFn: func(_ context.Context, messages []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			capturedMessages = messages
			return &clients.ChatResponse{Content: "ok", Model: "kimi-for-coding"}, nil
		},
	}
	adapter := &KimiAdapter{client: stub}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "Review this code"},
		},
	})
	req := llm.Request{
		Capability: llm.CapabilityCodeReviewAnnotation,
		Payload:    payload,
	}

	resp, err := adapter.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Output != "ok" {
		t.Errorf("Call().Output = %q, want %q", resp.Output, "ok")
	}
	if len(capturedMessages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(capturedMessages))
	}
	if capturedMessages[0].Role != "user" || capturedMessages[0].Content != "Review this code" {
		t.Errorf("messages[0] = {%q, %q}, want {user, Review this code}", capturedMessages[0].Role, capturedMessages[0].Content)
	}
}

// TestKimiAdapter_ConvertsResponseToLLMType verifies that a ChatResponse is
// correctly converted to an llm.Response with matching fields and ProviderKimi.
func TestKimiAdapter_ConvertsResponseToLLMType(t *testing.T) {
	expectedUsage := llm.Usage{
		InputTokens:  300,
		OutputTokens: 120,
		TotalTokens:  420,
		CostUSD:      0.003,
	}
	stub := &stubKimiChatter{
		chatFn: func(_ context.Context, _ []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			return &clients.ChatResponse{
				Content:      "Kimi review result",
				Model:        "kimi-for-coding",
				Usage:        expectedUsage,
				FinishReason: "stop",
			}, nil
		},
	}
	adapter := &KimiAdapter{client: stub}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "lint this"}},
	})
	req := llm.Request{
		Capability: llm.CapabilityPromptLint,
		Payload:    payload,
	}

	resp, err := adapter.Call(context.Background(), req)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	if resp.Output != "Kimi review result" {
		t.Errorf("Call().Output = %q, want %q", resp.Output, "Kimi review result")
	}
	if resp.Provider != llm.ProviderKimi {
		t.Errorf("Call().Provider = %q, want %q", resp.Provider, llm.ProviderKimi)
	}
	if resp.Usage != expectedUsage {
		t.Errorf("Call().Usage = %+v, want %+v", resp.Usage, expectedUsage)
	}
}

// TestKimiAdapter_RejectsUnauthorizedCapability verifies the ADR-009 guard:
// calling with a capability not in the allowed set (e.g.,
// CapabilityRationaleGeneration) returns ErrKimiRestricted.
func TestKimiAdapter_RejectsUnauthorizedCapability(t *testing.T) {
	stub := &stubKimiChatter{
		chatFn: func(_ context.Context, _ []clients.Message, _ *clients.ChatOptions) (*clients.ChatResponse, error) {
			t.Fatal("Chat should not be called for unauthorized capability")
			return nil, nil
		},
	}
	adapter := &KimiAdapter{client: stub}

	payload, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "explain this"}},
	})
	req := llm.Request{
		Capability: llm.CapabilityRationaleGeneration,
		Payload:    payload,
	}

	_, err := adapter.Call(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unauthorized capability, got nil")
	}
	if !errors.Is(err, ErrKimiRestricted) {
		t.Errorf("error = %v, want ErrKimiRestricted", err)
	}
}

// TestKimiAdapter_Supports_AllowedCapabilities verifies that Supports returns
// true for the ADR-009 allowed capabilities.
func TestKimiAdapter_Supports_AllowedCapabilities(t *testing.T) {
	adapter := &KimiAdapter{client: nil}

	if !adapter.Supports(llm.CapabilityCodeReviewAnnotation) {
		t.Error("Supports(CapabilityCodeReviewAnnotation) = false, want true")
	}
	if !adapter.Supports(llm.CapabilityPromptLint) {
		t.Error("Supports(CapabilityPromptLint) = false, want true")
	}
}

// TestKimiAdapter_Supports_RejectedCapabilities verifies that Supports returns
// false for capabilities not in the ADR-009 allowed set.
func TestKimiAdapter_Supports_RejectedCapabilities(t *testing.T) {
	adapter := &KimiAdapter{client: nil}

	rejected := []llm.Capability{
		llm.CapabilityFailureAttribution,
		llm.CapabilityRationaleGeneration,
		llm.CapabilityStrategySummary,
		llm.CapabilityRiskSurfaceExtraction,
		llm.CapabilityRegimeExplanation,
		llm.CapabilityScenarioSimulation,
		llm.CapabilitySentimentExplanation,
		llm.CapabilityPerformanceForensics,
		llm.CapabilityContraAttribution,
	}
	for _, cap := range rejected {
		if adapter.Supports(cap) {
			t.Errorf("Supports(%q) = true, want false (ADR-009)", cap)
		}
	}
}
