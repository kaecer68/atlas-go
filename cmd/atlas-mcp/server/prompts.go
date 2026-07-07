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

const promptTaiwanQuickLook = `You are the atlas-mcp Taiwan morning briefing agent. To produce a 台股今日快覽 (Taiwan Market Quick Look), call these tools in order:

1. mcp_quickstart — 一站式取得宏觀快照、策略、壓力指數、事件
2. capital_flow_summary — 資金流向摘要與共振方向
3. event_calendar — 今日與近期事件

然後用繁體中文輸出 3 段：
  (1) 總經環境（Risk-On/Off、美元、美債、VIX），
  (2) 資金流向（外資/投信/公股/散戶 多空方向與共振），
  (3) 今日關注重點（事件提醒、策略建議）。
控制在 400 字以內。`

const promptStrategyAdvice = `You are the atlas-mcp strategy advisor. To produce strategy advice, call these tools:

1. strategy_list_active — 取得當前生產環境中的活躍策略清單
2. strategy_get_summary — 針對感興趣的策略取得摘要（命中率、Sharpe、回撤、盤勢行為）
3. risk_get_commentary — 風險評論
4. regime_get_history — 近期盤勢歷史

Output in 繁體中文:
  (1) 當前盤勢適合的策略（依活躍清單與其摘要），
  (2) 各策略的適用條件與風險，
  (3) 建議的資金配置比例。
針對散戶投資人，避免過度專業術語。`

const promptStockHealthCheck = `You are the atlas-mcp stock health inspector. User provides a stock symbol (e.g., "2330"). Call these tools:

1. trace_get_decision_chain — 決策鏈追蹤
2. universe_get_sessions — 近期模擬 session
3. strategy_get — 該策略詳細資訊

Then output in 繁體中文:
  (1) 基本面狀態（來自 decision_chain），
  (2) 技術面與籌碼面（來自 strategy detail），
  (3) 綜合評分與建議（強力買進/買進/觀望/減碼）。
若數據不足，明確告知使用者哪些資料缺失。`

// registerPrompts attaches 6 reusable prompt templates that the agent can
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

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "taiwan_quick_look",
		Description: "台股今日快覽：呼叫 macro_snapshot + capital_flow + event_calendar，用繁體中文產出 3 段晨報。",
	}, handleTaiwanQuickLook)

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "strategy_advice",
		Description: "策略建議：呼叫 strategy_ranker + risk_commentary + regime_history，產出繁體中文策略建議。",
	}, handleStrategyAdvice)

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "stock_health_check",
		Description: "持股健檢：輸入股票代號，呼叫 trace + universe + strategy，產出繁體中文健檢報告。",
		Arguments: []*mcp.PromptArgument{
			{Name: "symbol", Description: "台股股票代號，例如 2330", Required: true},
		},
	}, handleStockHealthCheck)
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

func handleTaiwanQuickLook(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "台股今日快覽",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: promptTaiwanQuickLook}},
		},
	}, nil
}

func handleStrategyAdvice(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "策略建議",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: promptStrategyAdvice}},
		},
	}, nil
}

func handleStockHealthCheck(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("stock_health_check: required argument 'symbol' is missing")
	}
	symbol, ok := req.Params.Arguments["symbol"]
	if !ok || symbol == "" {
		return nil, fmt.Errorf("stock_health_check: required argument 'symbol' is missing")
	}
	body := fmt.Sprintf("股票代號：%s\n\n", symbol) + promptStockHealthCheck
	return &mcp.GetPromptResult{
		Description: "持股健檢 for " + symbol,
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: body}},
		},
	}, nil
}
