package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// Default MiniMax model constant.
const DefaultModelMiniMaxM3 = "MiniMax-M3"

// miniMaxAPIBase is the production API base URL for MiniMax.
const miniMaxAPIBase = "https://api.minimax.io"

// miniMaxEndpoint is the relative path for the OpenAI-compatible endpoint.
const miniMaxEndpointOpenAI = "/v1/chat/completions"

// miniMaxEndpointAnthropic is the Anthropic-compatible endpoint path.
const miniMaxEndpointAnthropic = "/v1/anthropic/chat/completions"

// MiniMaxClient is an OpenAI-compatible HTTP client for the MiniMax M3 API.
// It composes a BaseClient for retry, rate-limiting, and circuit-breaking.
//
// WARNING: MiniMax is hosted under Chinese national security law.
// DataClassSecret and DataClassRegulated payloads MUST NOT route to this
// provider. This restriction is enforced at the Router level; the client
// itself does not reject requests based on DataClass — see the KimiClient
// for an example of client-side enforcement.
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
func NewMiniMaxClient(apiKey string, baseClient *BaseClient) *MiniMaxClient {
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
		apiKey = os.Getenv("LLM_MINIMAX_API_KEY")
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
	_ = resp

	var parsed miniMaxResponseBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("minimax: unmarshal response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("minimax: empty choices in response")
	}

	choice := parsed.Choices[0]
	return &ChatResponse{
		Content:      choice.Message.Content,
		Model:        parsed.Model,
		FinishReason: choice.FinishReason,
		Usage: llm.Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		},
	}, nil
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
