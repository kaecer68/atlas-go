package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const promptDailyMarket = `You are the atlas-mcp market briefing agent. To produce a daily
briefing, call these tools in order:

1. macro_get_snapshot_latest — current macro snapshot (regime, capital flow)
2. narrative_get_bundle — recent narrative events and chains
3. system_get_metrics — system health and circuit-breaker state
4. risk_get_metrics — current risk posture (regime risk, VaR, drawdown)

Then write a 3-paragraph briefing:
  (1) macro context,
  (2) narrative highlights,
  (3) risk + system status.
Keep total under 400 words.`

const promptRiskCheck = `You are the atlas-mcp risk-check agent. To assess portfolio risk,
call these tools:

1. risk_get_metrics — current risk posture
2. risk_get_correlation_matrix — cross-strategy concentration
3. risk_get_drawdown — current and historical drawdown
4. risk_get_calibration — risk model calibration quality

Output: 4-section assessment:
  (1) current posture,
  (2) concentration risks,
  (3) drawdown exposure,
  (4) model confidence.

Flag any risk_get_metrics returns that show "DEFENSIVE" or
"SUSPENDED" mode — those require escalation.`

const promptRegimeInterpretationTmpl = `You are the atlas-mcp regime interpretation agent. The current regime is: %s

Interpret this regime in the context of the atlas-go investment framework:
- RISK_ON:         Favorable conditions; consider overweight growth strategies
- RISK_OFF:        Defensive posture; reduce exposure, tighten stops
- NEUTRAL:         Mixed signals; maintain current positioning
- TRANSITIONAL:    Regime shift in progress; favor lower-beta strategies

To support your interpretation, call narrative_get_bundle and
macro_get_snapshot_latest.

Provide 2-paragraph output:
  (1) regime interpretation,
  (2) recommended action.`

// registerPrompts attaches 3 reusable prompt templates that the agent can
// invoke by name. Prompts are static text (no HTTP calls) — they describe
// HOW the agent should use the available tools to answer a question.
func registerPrompts(mcpSrv *mcp.Server) {
	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "daily_market_briefing",
		Description: "Generate a daily market briefing by calling the relevant tools in sequence.",
	}, handleDailyMarketBriefing)

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "risk_check",
		Description: "Run a portfolio risk assessment using the risk_* tools.",
	}, handleRiskCheck)

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "regime_interpretation",
		Description: "Interpret a current regime signal and suggest positioning implications.",
		Arguments: []*mcp.PromptArgument{
			{Name: "regime", Description: "Current regime: RISK_ON | RISK_OFF | NEUTRAL | TRANSITIONAL", Required: true},
		},
	}, handleRegimeInterpretation)
}

func handleDailyMarketBriefing(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Daily market briefing",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: promptDailyMarket}},
		},
	}, nil
}

func handleRiskCheck(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Risk check",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: promptRiskCheck}},
		},
	}, nil
}

func handleRegimeInterpretation(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("regime_interpretation: required argument 'regime' is missing")
	}
	regime, ok := req.Params.Arguments["regime"]
	if !ok || regime == "" {
		return nil, fmt.Errorf("regime_interpretation: required argument 'regime' is missing")
	}
	body := fmt.Sprintf(promptRegimeInterpretationTmpl, regime)
	return &mcp.GetPromptResult{
		Description: "Regime interpretation for " + regime,
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: body}},
		},
	}, nil
}
