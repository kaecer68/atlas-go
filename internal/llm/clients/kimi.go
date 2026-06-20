package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// Default Kimi K2.7 model constant.
const DefaultModelKimiK27 = "kimi-for-coding"

// kimiAPIBase is the production API base URL for Kimi (Moonshot).
const kimiAPIBase = "https://api.kimi.com"

// kimiEndpoint is the relative path for the coding API.
const kimiEndpoint = "/coding/v1/chat/completions"

// KimiClient is an HTTP client for the Kimi K2.7 (kimi-for-coding) API.
// It composes a BaseClient for retry, rate-limiting, and circuit-breaking.
//
// K2.7-specific constraints are enforced client-side:
//   - Thinking mode is FORCED ON (thinking: {type: "enabled"} injected into
//     every request body).
//   - Temperature is LOCKED to 1.0 regardless of ChatOptions.Temperature.
//   - DataClassRegulated and DataClassSecret payloads are rejected with
//     ErrIncompatibleDataClass.
type KimiClient struct {
	*BaseClient

	// APIKey is used for Bearer authentication. If empty, read from
	// LLM_KIMI_API_KEY at Chat() time. Note: this is distinct from
	// LLM_ANNOTATOR_API_KEY used by the existing llm_annotator package.
	APIKey string

	// BaseURL overrides the API base URL for testing.
	BaseURL string
}

// NewKimiClient creates a KimiClient wired to the given BaseClient.
// If apiKey is empty, the client reads LLM_KIMI_API_KEY from the
// environment on the first Chat() call.
func NewKimiClient(apiKey string, baseClient *BaseClient) *KimiClient {
	return &KimiClient{
		BaseClient: baseClient,
		APIKey:     apiKey,
		BaseURL:    kimiAPIBase,
	}
}

// Chat sends messages to the Kimi K2.7 API and returns a normalized response.
// The model is always kimi-for-coding; no model parameter is accepted.
//
// K2.7 guard: if opts != nil and opts.DataClass is DataClassRegulated or
// DataClassSecret, Chat returns ErrIncompatibleDataClass immediately.
// Thinking mode is forced on and temperature is locked to 1.0.
func (c *KimiClient) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*ChatResponse, error) {
	// K2.7 data class guard.
	if opts != nil {
		switch opts.DataClass {
		case llm.DataClassRegulated, llm.DataClassSecret:
			return nil, fmt.Errorf("kimi: %w: K2.7 does not accept regulated or secret data", ErrIncompatibleDataClass)
		}
	}

	apiKey := c.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("LLM_KIMI_API_KEY")
	}

	// K2.7 constraints: thinking forced on, temperature locked to 1.0.
	const kimiForcedTemperature = 1.0
	reqBody := map[string]any{
		"model":       DefaultModelKimiK27,
		"messages":    messages,
		"temperature": kimiForcedTemperature,
		"stream":      false,
		"thinking": map[string]string{
			"type": "enabled",
		},
	}

	if opts != nil {
		if opts.MaxTokens != nil {
			reqBody["max_tokens"] = *opts.MaxTokens
		}
		// Warn if caller provided a temperature (it will be ignored).
		if opts.Temperature != nil && *opts.Temperature != kimiForcedTemperature {
			log.Printf("kimi: K2.7 temperature locked to %.1f; ignoring requested temperature %.2f",
				kimiForcedTemperature, *opts.Temperature)
		}
	}
	// Always log that thinking is forced on for K2.7.
	log.Printf("kimi: K2.7 thinking mode forced on (thinking.type=enabled)")

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("kimi: marshal request: %w", err)
	}

	url := c.BaseURL + kimiEndpoint
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	resp, body, err := c.DoRequest(ctx, "POST", url, headers, raw)
	if err != nil {
		return nil, fmt.Errorf("kimi: %w", err)
	}
	_ = resp

	var parsed kimiResponseBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("kimi: unmarshal response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("kimi: empty choices in response")
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

// kimiResponseBody is the OpenAI-compatible JSON structure received from Kimi.
type kimiResponseBody struct {
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
