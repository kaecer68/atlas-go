package capabilities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// confidenceCommentarySystemPrompt instructs the LLM to analyze a risk
// decision and produce a concise confidence-level explanation in
// Traditional Chinese.
const confidenceCommentarySystemPrompt = `你是 Atlas 交易系統的風險信心分析師。根據風險決策的結構化資料，以繁體中文產出一段簡潔的解釋，說明為什麼該決策的信心水準合理或不合理。限制在 3-4 句話以內。`

// confidenceCommentaryUserPromptFmt is the user prompt template. The
// serialized decision JSON is interpolated.
const confidenceCommentaryUserPromptFmt = `風險決策資料：

%s`

// ConfidenceCommentaryHandler explains confidence levels in a risk
// decision via the llm.Router.
type ConfidenceCommentaryHandler struct {
	router llm.Router
}

// NewConfidenceCommentaryHandler creates a handler backed by the given
// Router.
func NewConfidenceCommentaryHandler(r llm.Router) *ConfidenceCommentaryHandler {
	return &ConfidenceCommentaryHandler{router: r}
}

// Handle executes the confidence commentary capability. It:
//  1. Serializes the decision to JSON for the Router payload.
//  2. Dispatches through the Router with CapabilityConfidenceCommentary.
//  3. Parses the response into a ConfidenceCommentaryResponse (JSON-first,
//     then raw string fallback as Commentary).
func (h *ConfidenceCommentaryHandler) Handle(
	ctx context.Context,
	input schemas.ConfidenceCommentaryInput,
) (schemas.ConfidenceCommentaryResponse, error) {
	decisionJSON, _ := json.Marshal(input.Decision)
	userPrompt := fmt.Sprintf(confidenceCommentaryUserPromptFmt, string(decisionJSON))
	messages := []clients.Message{
		{Role: "system", Content: confidenceCommentarySystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	payload, _ := json.Marshal(map[string]any{
		"messages": messages,
	})
	req := llm.Request{
		Capability: llm.CapabilityConfidenceCommentary,
		Payload:    payload,
		DataClass:  input.DataClass,
		Options:    llm.Options{MaxTokens: 400},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.ConfidenceCommentaryResponse{}, nil
		}
		return schemas.ConfidenceCommentaryResponse{}, err
	}

	return parseConfidenceCommentaryResponse(resp.Output)
}

// parseConfidenceCommentaryResponse converts the Router's Response.Output
// into a ConfidenceCommentaryResponse. JSON-first, raw string fallback as
// Commentary.
func parseConfidenceCommentaryResponse(output string) (schemas.ConfidenceCommentaryResponse, error) {
	if output == "" {
		return schemas.ConfidenceCommentaryResponse{}, nil
	}

	var parsed schemas.ConfidenceCommentaryResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.ConfidenceCommentaryResponse{
		Commentary: output,
	}, nil
}
