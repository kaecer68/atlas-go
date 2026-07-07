package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBriefingTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name: "mcp_quickstart",
		Description: autoDescOr("mcp_quickstart",
			"一站式市場速覽：回傳最新宏觀快照、當前推薦策略、壓力指數與資金流向摘要、今日事件。外部 AI 首次接入時調用此 tool 即可取得完整操作脈絡，無需多次調用。"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPQuickstart)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name: "daily_report",
		Description: autoDescOr("daily_report",
			"回傳最新每日市場報告 JSON，含全球資金總開關、台股七大資金勢力分解與共振、事件日曆、策略訊號與風險提示。適合 LLM agent 每日晨報摘要生成。"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDailyReport)
}

func (s *server) handleMCPQuickstart(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var macro, regHistory, stress, events, strategies map[string]any

	// Fetch all in sequence (atlas HTTP API is single-threaded per call)
	_ = s.cli.Get(ctx, "/api/macro/snapshot/latest", nil, &macro)
	_ = s.cli.Get(ctx, "/api/narrative/stress-index/current", nil, &stress)
	_ = s.cli.Get(ctx, "/api/events/calendar", nil, &events)
	_ = s.cli.Get(ctx, "/api/strategies/active", nil, &strategies)
	_ = s.cli.Get(ctx, "/api/regime/history?days=5", nil, &regHistory)

	return nil, map[string]any{
		"macro_snapshot":          macro,
		"stress_index":            stress,
		"events":                  events,
		"active_strategies":       strategies,
		"recent_regime_5_days":    regHistory,
		"next_steps": "使用 strategy_ranker 取得策略排名、capital_flow 查看資金流向細節、event_flow_prediction 取得未來 5 日資金預測",
	}, nil
}

func (s *server) handleDailyReport(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.cli.Get(ctx, "/api/reports/latest", nil, &out); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
