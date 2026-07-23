package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBriefingTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name: "mcp_quickstart",
		Description: autoDescOr("mcp_quickstart",
			"一站式市場速覽 (MCP aggregated: macro + capital + regime + events)。回傳最新宏觀快照、當前推薦策略、壓力指數與資金流向摘要、今日事件。首次接入調用即可取得完整操作脈絡。"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPQuickstart)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name: "daily_report",
		Description: autoDescOr("daily_report",
			"回傳最新每日市場報告 JSON (HTTP: GET /api/reports/latest)，含全球資金總開關、台股七維錢潮雷達、事件日曆、策略訊號與風險提示。適合 LLM agent 每日晨報摘要生成。"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDailyReport)
}

func (s *server) handleMCPQuickstart(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "mcp_quickstart", nil, func() error {
		// Each section is fetched independently: a partial failure must be
		// visible to the calling agent (degraded marker + error), not
		// silently returned as an empty/null block.
		degraded := []string{}
		fetch := func(name, path string) any {
			var m map[string]any
			if err := s.cli.Get(ctx, path, nil, &m); err != nil {
				degraded = append(degraded, name)
				return map[string]any{"degraded": true, "error": err.Error()}
			}
			return m
		}
		out = map[string]any{
			"macro_snapshot":       fetch("macro_snapshot", "/api/macro/snapshot/latest"),
			"stress_index":         fetch("stress_index", "/api/narrative/stress-index/current"),
			"events":               fetch("events", "/api/events/calendar"),
			"active_strategies":    fetch("active_strategies", "/api/strategies/active"),
			"recent_regime_5_days": fetch("recent_regime_5_days", "/api/regime/history?days=5"),
			"next_steps":           "使用 strategy_list_active 取得活躍策略、capital_flow_summary 查看資金流向摘要、event_flow_prediction 取得未來 5 日資金預測",
		}
		if len(degraded) > 0 {
			out["degraded_sections"] = degraded
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (s *server) handleDailyReport(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "daily_report", nil, func() error {
		return s.cli.Get(ctx, "/api/reports/latest", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
