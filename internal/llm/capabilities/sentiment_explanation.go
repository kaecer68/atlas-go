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

// sentimentExplanationSystemPrompt instructs the LLM to explain the
// sentiment (bullish/bearish/neutral) implied by a narrative event,
// providing rationale and contributing factors in Chinese.
const sentimentExplanationSystemPrompt = `你是一個專業的市場情緒分析師，請根據以下市場事件解釋其情緒傾向（看漲/看跌/中性），並用中文說明判斷理由和影響因素。`

// sentimentExplanationUserPromptFmt is the user prompt template. The
// serialized NarrativeEvent JSON is interpolated.
const sentimentExplanationUserPromptFmt = `市場事件資料：

%s`

// SentimentExplanationHandler explains the sentiment of a
// narrative.NarrativeEvent in Chinese via the llm.Router.
type SentimentExplanationHandler struct {
	router llm.Router
}

// NewSentimentExplanationHandler creates a handler backed by the given Router.
func NewSentimentExplanationHandler(router llm.Router) *SentimentExplanationHandler {
	return &SentimentExplanationHandler{router: router}
}

// Handle executes the sentiment explanation capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassNonRegulated when unset.
//  3. Dispatches through the Router with CapabilitySentimentExplanation.
//  4. Parses the response into a SentimentExplanationResponse (JSON-first,
//     then raw string fallback as Explanation).
func (h *SentimentExplanationHandler) Handle(
	ctx context.Context,
	input schemas.SentimentExplanationInput,
) (schemas.SentimentExplanationResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassNonRegulated
	}

	eventJSON, _ := json.Marshal(input.Event)
	userPrompt := fmt.Sprintf(sentimentExplanationUserPromptFmt, string(eventJSON))
	messages := []clients.Message{
		{Role: "system", Content: sentimentExplanationSystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	payload, _ := json.Marshal(map[string]any{
		"messages": messages,
	})
	req := llm.Request{
		Capability: llm.CapabilitySentimentExplanation,
		Payload:    payload,
		DataClass:  dc,
		// max_tokens 2048 (was 500): 情緒解釋段落 + JSON 外殼；原值不足。
		// ADR-012 校準：reasoning 模型（M3 / deepseek-flash）最小預算 2048，短輸出型不低於此。
		Options: llm.Options{MaxTokens: 2048},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.SentimentExplanationResponse{}, nil
		}
		return schemas.SentimentExplanationResponse{}, err
	}

	return parseSentimentExplanationResponse(resp.Output)
}

// parseSentimentExplanationResponse converts the Router's Response.Output into
// a SentimentExplanationResponse. JSON-first, raw string fallback as
// Explanation.
func parseSentimentExplanationResponse(output string) (schemas.SentimentExplanationResponse, error) {
	if output == "" {
		return schemas.SentimentExplanationResponse{}, nil
	}

	var parsed schemas.SentimentExplanationResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.SentimentExplanationResponse{
		Explanation: output,
	}, nil
}
