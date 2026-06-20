package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// riskSurfaceExtractionSystemPrompt instructs the LLM to extract 3-5 risk
// surfaces from a knowledge gap description, including coverage gaps and
// sector/style blind spots.
const riskSurfaceExtractionSystemPrompt = `你是一個專業的風險分析師，請從以下知識缺口描述中提取 3-5 個關鍵風險表面，包括覆蓋缺口、板塊盲點、風格缺失等，並以中文說明。`

// riskSurfaceExtractionUserPromptFmt is the user prompt template. The
// serialized KnowledgeGap JSON is interpolated.
const riskSurfaceExtractionUserPromptFmt = `知識缺口資料：

%s`

// RiskSurfaceExtractionHandler extracts risk surfaces from a
// spawning.KnowledgeGap via the llm.Router.
type RiskSurfaceExtractionHandler struct {
	router llm.Router
}

// NewRiskSurfaceExtractionHandler creates a handler backed by the given Router.
func NewRiskSurfaceExtractionHandler(router llm.Router) *RiskSurfaceExtractionHandler {
	return &RiskSurfaceExtractionHandler{router: router}
}

// Handle executes the risk surface extraction capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassInternal when unset.
//  3. Dispatches through the Router with CapabilityRiskSurfaceExtraction.
//  4. Parses the response into a RiskSurfaceExtractionResponse (JSON-first,
//     then raw string fallback as EnrichedDescription).
func (h *RiskSurfaceExtractionHandler) Handle(
	ctx context.Context,
	input schemas.RiskSurfaceExtractionInput,
) (schemas.RiskSurfaceExtractionResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityRiskSurfaceExtraction,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 600},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.RiskSurfaceExtractionResponse{}, nil
		}
		return schemas.RiskSurfaceExtractionResponse{}, err
	}

	return parseRiskSurfaceExtractionResponse(resp.Output)
}

// parseRiskSurfaceExtractionResponse converts the Router's Response.Output
// into a RiskSurfaceExtractionResponse. JSON-first, raw string fallback as
// EnrichedDescription.
func parseRiskSurfaceExtractionResponse(output string) (schemas.RiskSurfaceExtractionResponse, error) {
	if output == "" {
		return schemas.RiskSurfaceExtractionResponse{}, nil
	}

	var parsed schemas.RiskSurfaceExtractionResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.RiskSurfaceExtractionResponse{
		EnrichedDescription: output,
	}, nil
}
