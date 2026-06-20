package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// strategySummarySystemPrompt instructs the LLM to produce a 1-2 sentence
// Chinese summary of a trading strategy frame, including its key conditions.
const strategySummarySystemPrompt = `你是一個專業的量化交易策略分析師，請用簡潔的中文總結以下策略框架，包含 1-2 句話的策略描述和關鍵進場條件。`

// strategySummaryUserPromptFmt is the user prompt template. The serialized
// StrategyFrame JSON is interpolated.
const strategySummaryUserPromptFmt = `策略框架資料：

%s`

// StrategySummaryHandler produces a human-readable Chinese summary of a
// strategy_techniques.StrategyFrame via the llm.Router.
type StrategySummaryHandler struct {
	router llm.Router
}

// NewStrategySummaryHandler creates a handler backed by the given Router.
func NewStrategySummaryHandler(router llm.Router) *StrategySummaryHandler {
	return &StrategySummaryHandler{router: router}
}

// Handle executes the strategy summary capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassInternal when unset.
//  3. Dispatches through the Router with CapabilityStrategySummary.
//  4. Parses the response into a StrategySummaryResponse (JSON-first,
//     then raw string fallback as Summary).
func (h *StrategySummaryHandler) Handle(
	ctx context.Context,
	input schemas.StrategySummaryInput,
) (schemas.StrategySummaryResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityStrategySummary,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 300},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.StrategySummaryResponse{}, nil
		}
		return schemas.StrategySummaryResponse{}, err
	}

	return parseStrategySummaryResponse(resp.Output)
}

// parseStrategySummaryResponse converts the Router's Response.Output into a
// StrategySummaryResponse. JSON-first, raw string fallback as Summary.
func parseStrategySummaryResponse(output string) (schemas.StrategySummaryResponse, error) {
	if output == "" {
		return schemas.StrategySummaryResponse{}, nil
	}

	var parsed schemas.StrategySummaryResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.StrategySummaryResponse{
		Summary: output,
	}, nil
}
