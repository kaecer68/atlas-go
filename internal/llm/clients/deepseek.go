package clients

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// Default DeepSeek model constants.
const (
	DefaultModelV4Pro   = "deepseek-v4-pro"
	DefaultModelV4Flash  = "deepseek-v4-flash"
)

// deepSeekAPIBase is the production API base URL for DeepSeek.
const deepSeekAPIBase = "https://api.deepseek.com"

// DeepSeekClient is an OpenAI-compatible HTTP client for the DeepSeek V4 API.
// It composes a BaseClient for retry, rate-limiting, and circuit-breaking
// infrastructure. Set BaseURL to override the API endpoint (e.g., for testing
// with httptest servers).
type DeepSeekClient struct {
	*BaseClient

	// APIKey is used for Bearer authentication. If empty, the client
	// reads LLM_DEEPSEEK_API_KEY from the environment on the first
	// Chat() call.
	APIKey string

	// DefaultModel is the model used when Chat() is called with model="".
	// Defaults to DefaultModelV4Pro.
	DefaultModel string

	// BaseURL overrides the API base URL. When empty, the production
	// endpoint (deepSeekAPIBase) is used. Set this to an httptest server
	// URL in tests.
	BaseURL string
}

// NewDeepSeekClient creates a DeepSeekClient wired to the given BaseClient.
// Use DefaultModelV4Pro for typical inference and DefaultModelV4Flash for
// latency-sensitive workloads. If apiKey is empty, the client will read
// LLM_DEEPSEEK_API_KEY from the environment at Chat() time.
func NewDeepSeekClient(apiKey string, baseClient *BaseClient) *DeepSeekClient {
	return &DeepSeekClient{
		BaseClient:   baseClient,
		APIKey:       apiKey,
		DefaultModel: DefaultModelV4Pro,
		BaseURL:      deepSeekAPIBase,
	}
}

// Chat sends messages to the DeepSeek API and returns a normalized response.
// If model is "", c.DefaultModel is used. The request body follows the
// OpenAI chat completions schema.
func (c *DeepSeekClient) Chat(ctx context.Context, model string, messages []Message, opts *ChatOptions) (*ChatResponse, error) {
	apiKey := c.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("deepseek: API key not set (caller must pass via NewDeepSeekClient; use config.GetSecret(\"LLM_DEEPSEEK_API_KEY\") in main.go wiring)")
	}

	if model == "" {
		model = c.DefaultModel
	}

	reqBody := deepSeekRequestBody{
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
		return nil, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	url := c.BaseURL + "/v1/chat/completions"
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	resp, body, err := c.DoRequest(ctx, "POST", url, headers, raw)
	if err != nil {
		return nil, fmt.Errorf("deepseek: %w", err)
	}
	_ = resp // resp.Body already consumed by DoRequest; use parsed body

	var parsed deepSeekResponseBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("deepseek: unmarshal response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("deepseek: empty choices in response")
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

// deepSeekRequestBody is the OpenAI-compatible JSON structure sent to DeepSeek.
type deepSeekRequestBody struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// deepSeekResponseBody is the OpenAI-compatible JSON structure received from DeepSeek.
type deepSeekResponseBody struct {
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
