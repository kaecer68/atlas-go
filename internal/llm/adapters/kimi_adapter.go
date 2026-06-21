package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
)

// ErrKimiRestricted is returned by KimiAdapter.Call when the requested
// Capability is not in Kimi K2.7's allowed set per ADR-009 §6.1.
var ErrKimiRestricted = errors.New("kimi: capability restricted by ADR-009")

// kimiChatter abstracts the Chat method of *clients.KimiClient for
// testability. Note: Kimi K2.7 does not accept a model parameter; the model
// is always "kimi-for-coding".
type kimiChatter interface {
	Chat(ctx context.Context, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error)
}

// kimiAllowedCaps is the set of Capability values that Kimi K2.7 is
// authorized to handle under ADR-009 §6.1. K2.7 is a code-specialized model;
// all non-code capabilities must be routed elsewhere.
var kimiAllowedCaps = map[llm.Capability]bool{
	llm.CapabilityCodeReviewAnnotation: true,
	llm.CapabilityPromptLint:           true,
}

// KimiAdapter wraps a Kimi K2.7 client and exposes it as an llm.ProviderImpl.
// It bridges the Kimi K2.7 (kimi-for-coding) API into the capability-based
// routing system.
//
// K2.7 guard: ADR-009 restricts to code_review_annotation and prompt_lint
// per §6.1. All other capabilities are rejected by both Supports and Call.
type KimiAdapter struct {
	client kimiChatter
}

// NewKimiAdapter creates a KimiAdapter wrapping the given client.
func NewKimiAdapter(client *clients.KimiClient) *KimiAdapter {
	return &KimiAdapter{client: client}
}

// Supports implements llm.ProviderImpl. Per ADR-009 §6.1, Kimi K2.7 is
// restricted to code_review_annotation and prompt_lint.
func (a *KimiAdapter) Supports(cap llm.Capability) bool {
	return kimiAllowedCaps[cap]
}

// Call implements llm.ProviderImpl. It enforces the ADR-009 capability
// guard, unmarshals req.Payload (expected as []byte JSON) into a
// []clients.Message slice, builds ChatOptions, delegates to the underlying
// KimiClient.Chat (always kimi-for-coding, no model override), and converts
// the *ChatResponse into an llm.Response.
func (a *KimiAdapter) Call(ctx context.Context, req llm.Request) (llm.Response, error) {
	if !kimiAllowedCaps[req.Capability] {
		return llm.Response{}, fmt.Errorf("%w: %q not in allowed set {code_review_annotation, prompt_lint}", ErrKimiRestricted, req.Capability)
	}

	payloadBytes, ok := req.Payload.([]byte)
	if !ok {
		return llm.Response{}, fmt.Errorf("kimi: expected []byte payload, got %T", req.Payload)
	}

	var payload struct {
		Messages []clients.Message `json:"messages"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return llm.Response{}, fmt.Errorf("kimi: invalid payload: %w", err)
	}
	if len(payload.Messages) == 0 {
		return llm.Response{}, fmt.Errorf("kimi: no messages in payload")
	}

	opts := buildChatOptions(req.Options, req.DataClass)

	resp, err := a.client.Chat(ctx, payload.Messages, opts)
	if err != nil {
		return llm.Response{}, err
	}

	return llm.Response{
		Output:   resp.Content,
		Provider: llm.ProviderKimi,
		Usage:    resp.Usage,
	}, nil
}

// Health implements llm.ProviderImpl.
func (a *KimiAdapter) Health() llm.HealthStatus {
	return llm.HealthStatus{Provider: llm.ProviderKimi, Healthy: true}
}
