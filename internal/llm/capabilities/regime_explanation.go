package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// regimeExplanationSystemPrompt instructs the LLM to explain the market
// regime implied by a narrative event, producing a concise Chinese headline.
const regimeExplanationSystemPrompt = `你是一個專業的宏觀市場分析師，請根據以下市場事件解釋當前的市場狀態（regime），用一則簡潔的中文標題總結市場走勢。`

// regimeExplanationUserPromptFmt is the user prompt template. The serialized
// NarrativeEvent JSON is interpolated.
const regimeExplanationUserPromptFmt = `市場事件資料：

%s`

// RegimeExplanationHandler explains a narrative.NarrativeEvent's market
// regime in Chinese via the llm.Router.
type RegimeExplanationHandler struct {
	router llm.Router
}

// NewRegimeExplanationHandler creates a handler backed by the given Router.
func NewRegimeExplanationHandler(router llm.Router) *RegimeExplanationHandler {
	return &RegimeExplanationHandler{router: router}
}

// Handle executes the regime explanation capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassPublic when unset.
//  3. Dispatches through the Router with CapabilityRegimeExplanation.
//  4. Parses the response into a RegimeExplanationResponse (JSON-first,
//     then raw string fallback as Headline).
func (h *RegimeExplanationHandler) Handle(
	ctx context.Context,
	input schemas.RegimeExplanationInput,
) (schemas.RegimeExplanationResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassNonRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityRegimeExplanation,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 300},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.RegimeExplanationResponse{}, nil
		}
		return schemas.RegimeExplanationResponse{}, err
	}

	return parseRegimeExplanationResponse(resp.Output)
}

// parseRegimeExplanationResponse converts the Router's Response.Output into
// a RegimeExplanationResponse. JSON-first, raw string fallback as Headline.
func parseRegimeExplanationResponse(output string) (schemas.RegimeExplanationResponse, error) {
	if output == "" {
		return schemas.RegimeExplanationResponse{}, nil
	}

	var parsed schemas.RegimeExplanationResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.RegimeExplanationResponse{
		Headline: output,
	}, nil
}
