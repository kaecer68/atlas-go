package capabilities

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
)

// performanceForensicsSystemPrompt instructs the LLM to analyze a risk
// snapshot and identify performance anomalies (VaR, CVaR, max drawdown)
// with calibration context in Chinese.
const performanceForensicsSystemPrompt = `你是一個專業的風險績效分析師，請分析以下風險快照，找出績效異常點（如 VaR 超標、最大回撤過大），並用中文提供校準建議。`

// performanceForensicsUserPromptFmt is the user prompt template. The
// serialized RiskSnapshot JSON is interpolated.
const performanceForensicsUserPromptFmt = `風險快照資料：

%s`

// PerformanceForensicsHandler explains performance anomalies in a
// domain.RiskSnapshot via the llm.Router.
type PerformanceForensicsHandler struct {
	router llm.Router
}

// NewPerformanceForensicsHandler creates a handler backed by the given Router.
func NewPerformanceForensicsHandler(router llm.Router) *PerformanceForensicsHandler {
	return &PerformanceForensicsHandler{router: router}
}

// Handle executes the performance forensics capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassRegulated when unset (VaR/ES data is
//     sensitive).
//  3. Dispatches through the Router with CapabilityPerformanceForensics.
//  4. Parses the response into a PerformanceForensicsResponse (JSON-first,
//     then raw string fallback as Commentary).
func (h *PerformanceForensicsHandler) Handle(
	ctx context.Context,
	input schemas.PerformanceForensicsInput,
) (schemas.PerformanceForensicsResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassRegulated
	}

	payload, _ := json.Marshal(input)
	req := llm.Request{
		Capability: llm.CapabilityPerformanceForensics,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 600},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.PerformanceForensicsResponse{}, nil
		}
		return schemas.PerformanceForensicsResponse{}, err
	}

	return parsePerformanceForensicsResponse(resp.Output)
}

// parsePerformanceForensicsResponse converts the Router's Response.Output
// into a PerformanceForensicsResponse. JSON-first, raw string fallback as
// Commentary.
func parsePerformanceForensicsResponse(output string) (schemas.PerformanceForensicsResponse, error) {
	if output == "" {
		return schemas.PerformanceForensicsResponse{}, nil
	}

	var parsed schemas.PerformanceForensicsResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.PerformanceForensicsResponse{
		Commentary: output,
	}, nil
}
