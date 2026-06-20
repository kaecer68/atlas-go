package clients

import (
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// Message represents a single chat message in an OpenAI-compatible
// conversation. Role must be one of "system", "user", or "assistant".
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatOptions provides fine-grained control over a Chat() call.
// Zero-value fields use provider-specific defaults.
type ChatOptions struct {
	// Temperature controls the randomness of the output.
	// If nil, the provider's default is used.
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens caps the number of output tokens.
	// If nil, the provider's default is used.
	MaxTokens *int `json:"max_tokens,omitempty"`

	// DataClass classifies the sensitivity of the payload.
	// Providers may reject requests with incompatible DataClass values.
	DataClass llm.DataClass `json:"data_class,omitempty"`
}

// ChatResponse is the normalized result of a Chat() call. It abstracts
// away provider-specific response shapes into a uniform format.
type ChatResponse struct {
	// Content is the model's textual response.
	Content string `json:"content"`

	// Model is the specific model that produced this response.
	Model string `json:"model"`

	// Usage reports token consumption for this call.
	Usage llm.Usage `json:"usage"`

	// FinishReason indicates why the model stopped generating tokens
	// (e.g., "stop", "length", "content_filter"). Empty if unknown.
	FinishReason string `json:"finish_reason,omitempty"`
}

// ErrIncompatibleDataClass is returned when a provider rejects a request
// because the payload's DataClass is incompatible with the provider's
// data governance constraints.
var ErrIncompatibleDataClass = errors.New("data class incompatible with provider")
