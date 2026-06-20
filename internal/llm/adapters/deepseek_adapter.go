package adapters

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
)

// deepSeekChatter abstracts the Chat method of *clients.DeepSeekClient for
// testability. Both the concrete client and test stubs satisfy this interface.
type deepSeekChatter interface {
	Chat(ctx context.Context, model string, messages []clients.Message, opts *clients.ChatOptions) (*clients.ChatResponse, error)
}

// DeepSeekAdapter wraps a DeepSeek V4 client and exposes it as an
// llm.ProviderImpl. It bridges the DeepSeek V4 API into the capability-based
// routing system.
//
// Model selection: "deepseek-v4-pro" (default) or "deepseek-v4-flash"
// (latency-sensitive). If Model is empty, the underlying client falls back
// to its own DefaultModel.
type DeepSeekAdapter struct {
	client deepSeekChatter
	Model  string
}

// NewDeepSeekAdapter creates a DeepSeekAdapter wrapping the given client.
// If model is "", the adapter passes an empty model to the client, which
// falls back to its own DefaultModel (deepseek-v4-pro).
func NewDeepSeekAdapter(client *clients.DeepSeekClient, model string) *DeepSeekAdapter {
	return &DeepSeekAdapter{client: client, Model: model}
}

// Supports implements llm.ProviderImpl. DeepSeek V4 is a general-purpose
// model and supports all capabilities.
func (a *DeepSeekAdapter) Supports(_ llm.Capability) bool {
	return true
}

// Call implements llm.ProviderImpl. It unmarshals req.Payload (expected as
// []byte JSON) into a []clients.Message slice, builds ChatOptions from
// req.Options, delegates to the underlying client, and converts the
// *ChatResponse into an llm.Response.
func (a *DeepSeekAdapter) Call(ctx context.Context, req llm.Request) (llm.Response, error) {
	payloadBytes, ok := req.Payload.([]byte)
	if !ok {
		return llm.Response{}, fmt.Errorf("deepseek: expected []byte payload, got %T", req.Payload)
	}

	var payload struct {
		Messages []clients.Message `json:"messages"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return llm.Response{}, fmt.Errorf("deepseek: invalid payload: %w", err)
	}
	if len(payload.Messages) == 0 {
		return llm.Response{}, fmt.Errorf("deepseek: no messages in payload")
	}

	opts := buildChatOptions(req.Options, req.DataClass)

	resp, err := a.client.Chat(ctx, a.Model, payload.Messages, opts)
	if err != nil {
		return llm.Response{}, err
	}

	return llm.Response{
		Output:   resp.Content,
		Provider: llm.ProviderDeepSeek,
		Usage:    resp.Usage,
	}, nil
}

// Health implements llm.ProviderImpl. The adapter always reports healthy;
// circuit-breaking is managed by the Router.
func (a *DeepSeekAdapter) Health() llm.HealthStatus {
	return llm.HealthStatus{Provider: llm.ProviderDeepSeek, Healthy: true}
}

// buildChatOptions constructs a *clients.ChatOptions from llm.Options and
// DataClass. Returns nil when no non-zero options are present.
func buildChatOptions(opts llm.Options, dc llm.DataClass) *clients.ChatOptions {
	if opts.MaxTokens == 0 && opts.Temperature == 0 {
		return nil
	}
	co := &clients.ChatOptions{DataClass: dc}
	if opts.MaxTokens > 0 {
		mt := opts.MaxTokens
		co.MaxTokens = &mt
	}
	if opts.Temperature != 0 {
		t := opts.Temperature
		co.Temperature = &t
	}
	return co
}
