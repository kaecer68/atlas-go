package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// Default MiniMax model constant.
const DefaultModelMiniMaxM3 = "MiniMax-M3"

// Inline thinking markers used by MiniMax M3 (see stripInlineThinking).
const (
	thinkOpenTag  = "<think>"
	thinkCloseTag = "</think>"
)

// miniMaxAPIBase is the production API base URL for MiniMax.
// China: api.minimaxi.com (extra 'i'), International: api.minimax.io
const miniMaxAPIBase = "https://api.minimaxi.com"

// MiniMaxChatBaseV1 is the OpenAI-compatible v1 base URL (no path suffix).
// Use it for clients that build their own request URL, e.g.
// internal/llm_annotator.KimiClient (which appends "/chat/completions").
//
// MiniMax is the only usable upstream for annotator-style traffic in this
// deployment: the legacy "kimi" coding-plan keys are CLI-only and return
// HTTP 401/403 against api.kimi.com (verified 2026-09-11).
const MiniMaxChatBaseV1 = miniMaxAPIBase + "/v1"

// miniMaxEndpoint is the relative path for the OpenAI-compatible endpoint.
const miniMaxEndpointOpenAI = "/v1/chat/completions"

// miniMaxEndpointAnthropic is the Anthropic-compatible endpoint path.
const miniMaxEndpointAnthropic = "/v1/anthropic/chat/completions"

// MiniMaxClient is an OpenAI-compatible HTTP client for the MiniMax M3 API.
// It composes a BaseClient for retry, rate-limiting, and circuit-breaking.
//
// Data residency note: MiniMax is hosted under Chinese national security law,
// and so are the DeepSeek (Hangzhou) and Kimi/Moonshot (Beijing) upstreams.
// The Router therefore applies no per-provider data-sovereignty gate
// (ADR-012); if a deployment must keep data away from third-party
// infrastructure, that control has to cover all providers or move to
// self-hosting.
type MiniMaxClient struct {
	*BaseClient

	// APIKey is used for Bearer authentication. If empty, read from
	// LLM_MINIMAX_API_KEY at Chat() time.
	APIKey string

	// DefaultModel is used when Chat() receives model="".
	DefaultModel string

	// UseAnthropicFormat switches the endpoint to the Anthropic-compatible
	// path. When true, Chat() sends requests to /v1/anthropic/chat/completions
	// instead of /v1/chat/completions. The request/response body shape
	// remains OpenAI-compatible in both modes.
	UseAnthropicFormat bool

	// BaseURL overrides the API base URL for testing.
	BaseURL string
}

// NewMiniMaxClient creates a MiniMaxClient wired to the given BaseClient.
// If apiKey is empty, the client will read LLM_MINIMAX_API_KEY from the
// environment on the first Chat() call.
//
// A nil baseClient is accepted and replaced with a default BaseClient so that
// callers (e.g., cmd/atlas wiring, cmd/lint-pr) are not required to build the
// shared HTTP infrastructure manually.
func NewMiniMaxClient(apiKey string, baseClient *BaseClient) *MiniMaxClient {
	if baseClient == nil {
		baseClient = NewBaseClient(llm.ProviderMiniMax, BaseClientConfig{})
	}
	return &MiniMaxClient{
		BaseClient:   baseClient,
		APIKey:       apiKey,
		DefaultModel: DefaultModelMiniMaxM3,
		BaseURL:      miniMaxAPIBase,
	}
}

// Chat sends messages to the MiniMax API and returns a normalized response.
// If model is "", c.DefaultModel is used. When UseAnthropicFormat is true,
// the Anthropic-compatible endpoint is used instead of the default OpenAI
// path.
func (c *MiniMaxClient) Chat(ctx context.Context, model string, messages []Message, opts *ChatOptions) (*ChatResponse, error) {
	apiKey := c.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("minimax: API key not set (caller must pass via NewMiniMaxClient; use config.GetSecret(\"LLM_MINIMAX_API_KEY\") in main.go wiring)")
	}

	if model == "" {
		model = c.DefaultModel
	}

	reqBody := miniMaxRequestBody{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	if opts != nil {
		if opts.Temperature != nil {
			reqBody.Temperature = opts.Temperature
		}
		if opts.MaxTokens != nil {
			reqBody.MaxTokens = opts.MaxTokens
		}
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("minimax: marshal request: %w", err)
	}

	path := miniMaxEndpointOpenAI
	if c.UseAnthropicFormat {
		path = miniMaxEndpointAnthropic
	}
	url := c.BaseURL + path

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	resp, body, err := c.DoRequest(ctx, "POST", url, headers, raw)
	if err != nil {
		return nil, fmt.Errorf("minimax: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var parsed miniMaxResponseBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("minimax: unmarshal response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("minimax: empty choices in response")
	}

	choice := parsed.Choices[0]
	return &ChatResponse{
		Content:      stripInlineThinking(choice.Message.Content),
		Model:        parsed.Model,
		FinishReason: choice.FinishReason,
		Usage: llm.Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}, nil
}

// stripInlineThinking removes MiniMax M3's inline reasoning block from a
// response. The CN OpenAI-compatible endpoint returns native thinking inside
// message.content, wrapped in <think>...</think> and followed by the actual
// answer (verified 2026-09-11 against api.minimaxi.com: `"<think>The user
// asked…</think>\n\n好"`). Without this, JSON-first capability parsers fail
// and fall back to raw-string output, so users would see reasoning text.
//
// Unbalanced/truncated thinking (an opening tag with no closing tag) means the
// model never got to its answer: the result is empty, which the Router treats
// as a provider failure and falls through to the next chain member.
func stripInlineThinking(content string) string {
	s := strings.TrimSpace(content)
	for {
		if !strings.HasPrefix(s, thinkOpenTag) {
			return s
		}
		// Find the end of the thinking block. Use the last closing tag so a
		// nested/duplicated marker inside the reasoning text cannot truncate
		// the real answer.
		end := strings.LastIndex(s, thinkCloseTag)
		if end < 0 {
			return ""
		}
		s = strings.TrimSpace(s[end+len(thinkCloseTag):])
	}
}

// miniMaxRequestBody mirrors the OpenAI chat completions request shape.
type miniMaxRequestBody struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// miniMaxResponseBody mirrors the OpenAI chat completions response shape.
type miniMaxResponseBody struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}
