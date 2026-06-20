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

// scenarioSimulationSystemPrompt instructs the LLM to explain a prism
// training result in natural Chinese, covering hit rate, Sharpe ratio,
// max drawdown, and regime context.
const scenarioSimulationSystemPrompt = `你是一個專業的量化回測分析師，請用中文解釋以下訓練結果的含義，包括勝率、夏普比率、最大回撤、以及在不同市場環境下的表現差異。`

// scenarioSimulationUserPromptFmt is the user prompt template. The serialized
// TrainingResult JSON and regime type are interpolated.
const scenarioSimulationUserPromptFmt = `訓練結果：

%s

市場環境：%s`

// ScenarioSimulationHandler explains a prism.TrainingResult in natural
// Chinese via the llm.Router. It wraps the prism cohort insight flow.
type ScenarioSimulationHandler struct {
	router llm.Router
}

// NewScenarioSimulationHandler creates a handler backed by the given Router.
func NewScenarioSimulationHandler(router llm.Router) *ScenarioSimulationHandler {
	return &ScenarioSimulationHandler{router: router}
}

// Handle executes the scenario simulation capability. It:
//  1. Serializes the input to JSON for the Router payload.
//  2. Defaults DataClass to DataClassInternal when unset.
//  3. Dispatches through the Router with CapabilityScenarioSimulation.
//  4. Parses the response into a ScenarioSimulationResponse (JSON-first,
//     then raw string fallback as Insight).
func (h *ScenarioSimulationHandler) Handle(
	ctx context.Context,
	input schemas.ScenarioSimulationInput,
) (schemas.ScenarioSimulationResponse, error) {
	dc := input.DataClass
	if dc == llm.DataClassUnmarked {
		dc = llm.DataClassRegulated
	}

	resultJSON, _ := json.Marshal(input.Result)
	userPrompt := fmt.Sprintf(scenarioSimulationUserPromptFmt, string(resultJSON), input.Regime)
	messages := []clients.Message{
		{Role: "system", Content: scenarioSimulationSystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	payload, _ := json.Marshal(map[string]any{
		"messages": messages,
	})
	req := llm.Request{
		Capability: llm.CapabilityScenarioSimulation,
		Payload:    payload,
		DataClass:  dc,
		Options:    llm.Options{MaxTokens: 600},
	}

	resp, err := h.router.Call(ctx, req)
	if err != nil {
		if errors.Is(err, llm.ErrAllProvidersFailed) {
			return schemas.ScenarioSimulationResponse{}, nil
		}
		return schemas.ScenarioSimulationResponse{}, err
	}

	return parseScenarioSimulationResponse(resp.Output)
}

// parseScenarioSimulationResponse converts the Router's Response.Output into
// a ScenarioSimulationResponse. JSON-first, raw string fallback as Insight.
func parseScenarioSimulationResponse(output string) (schemas.ScenarioSimulationResponse, error) {
	if output == "" {
		return schemas.ScenarioSimulationResponse{}, nil
	}

	var parsed schemas.ScenarioSimulationResponse
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		return parsed, nil
	}

	return schemas.ScenarioSimulationResponse{
		Insight: output,
	}, nil
}
