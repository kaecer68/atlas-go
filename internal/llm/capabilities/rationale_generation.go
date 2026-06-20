package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// rationaleGenerationSystemPrompt instructs the LLM to translate English
// financial rationale into concise Chinese while preserving all numbers and
// key domain terminology.
const rationaleGenerationSystemPrompt = `你是一個專業的金融工程翻譯助手，請將以下英文投資邏輯翻譯為簡潔的中文，保留所有數字和關鍵術語。`

// rationaleGenerationUserPromptFmt is the user prompt template for rationale
// translation. The caller's English text is interpolated into this format.
const rationaleGenerationUserPromptFmt = `請翻譯以下內容：

%s`

// RationaleGenerationHandler translates English investment rationale text into
// Chinese via the llm.Router. It wraps the narrative.TranslateReason flow.
type RationaleGenerationHandler struct {
	router llm.Router
}

// NewRationaleGenerationHandler creates a handler backed by the given Router.
func NewRationaleGenerationHandler(router llm.Router) *RationaleGenerationHandler {
	return &RationaleGenerationHandler{router: router}
}

// Handle executes the rationale generation capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassPublic when unset.
//  3. Dispatches through the Router with CapabilityRationaleGeneration.
//  4. Parses the response into a RationaleGenerationResponse (JSON-first,
//     then raw string fallback).
//  5. On all-providers-failed, returns an empty response.
func (h *RationaleGenerationHandler) Handle(
	ctx context.Context,
	input schemas.RationaleGenerationInput,
) (schemas.RationaleGenerationResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassNonRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityRationaleGeneration,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 500},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.RationaleGenerationResponse{}, nil
		}
		return schemas.RationaleGenerationResponse{}, err
	}

	return parseRationaleGenerationResponse(resp.Output)
}

// parseRationaleGenerationResponse converts the Router's Response.Output
// into a RationaleGenerationResponse. JSON is attempted first; if parsing
// fails the raw output is used as TranslatedText.
func parseRationaleGenerationResponse(output string) (schemas.RationaleGenerationResponse, error) {
	if output == "" {
		return schemas.RationaleGenerationResponse{}, nil
	}

	var parsed schemas.RationaleGenerationResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.RationaleGenerationResponse{
		TranslatedText: output,
	}, nil
}
