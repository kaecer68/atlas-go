package adapters

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
)

// miniMaxChatter abstracts the Chat method of *clients.MiniMaxClient for
// testability.
type miniMaxChatter interface {
	Chat(ctx context.Context, model string, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error)
}

// MiniMaxAdapter wraps a MiniMax M3 client and exposes it as an
// llm.ProviderImpl. It bridges the MiniMax M3 API into the capability-based
// routing system.
//
// The adapter always uses the MiniMax-M3 model. UseAnthropicFormat is
// configured on the underlying client, not on the adapter.
type MiniMaxAdapter struct {
	client miniMaxChatter
}

// NewMiniMaxAdapter creates a MiniMaxAdapter wrapping the given client.
func NewMiniMaxAdapter(client *clients.MiniMaxClient) *MiniMaxAdapter {
	return &MiniMaxAdapter{client: client}
}

// Supports implements llm.ProviderImpl. MiniMax M3 is a general-purpose
// model and supports all capabilities.
func (a *MiniMaxAdapter) Supports(_ llm.Capability) bool {
	return true
}

// Call implements llm.ProviderImpl. It unmarshals req.Payload (expected as
// []byte JSON) into a []clients.Message slice, builds ChatOptions, delegates
// to the underlying client with model "MiniMax-M3", and converts the
// *ChatResponse into an llm.Response.
func (a *MiniMaxAdapter) Call(ctx context.Context, req llm.Request) (llm.Response, error) {
	payloadBytes, ok := req.Payload.([]byte)
	if !ok {
		return llm.Response{}, fmt.Errorf("minimax: expected []byte payload, got %T", req.Payload)
	}

	var payload struct {
		Messages []clients.Message `json:"messages"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return llm.Response{}, fmt.Errorf("minimax: invalid payload: %w", err)
	}
	if len(payload.Messages) == 0 {
		return llm.Response{}, fmt.Errorf("minimax: no messages in payload")
	}

	opts := buildChatOptions(req.Options, req.DataClass)

	resp, err := a.client.Chat(ctx, clients.DefaultModelMiniMaxM3, payload.Messages, opts)
	if err != nil {
		return llm.Response{}, err
	}

	return llm.Response{
		Output:   resp.Content,
		Provider: llm.ProviderMiniMax,
		Usage:    resp.Usage,
	}, nil
}

// Health implements llm.ProviderImpl.
func (a *MiniMaxAdapter) Health() llm.HealthStatus {
	return llm.HealthStatus{Provider: llm.ProviderMiniMax, Healthy: true}
}
