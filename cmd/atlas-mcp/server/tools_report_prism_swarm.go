package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerReportPrismSwarmTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_daily_summary",
		Description: autoDescOr("report_get_daily_summary", "Daily summary report (text or structured) suitable for an LLM agent's morning briefing."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetDailySummary)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_performance",
		Description: autoDescOr("report_get_performance", "Performance report (period-configurable) with attribution and risk-adjusted metrics."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetPerformance)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_tax_snapshot",
		Description: autoDescOr("report_get_tax_snapshot", "Tax-relevant snapshot (realized gains, dividend totals, foreign tax)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetTaxSnapshot)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_export_link",
		Description: autoDescOr("report_get_export_link", "Signed export link for a report variant (expires after a short TTL)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetExportLink)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "prism_get_training_results",
		Description: autoDescOr("prism_get_training_results", "Latest PRISM cohort training results (config + metrics per cohort)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handlePrismGetTrainingResults)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "swarm_get_status",
		Description: autoDescOr("swarm_get_status", "MiroFish swarm simulator status (running, paused, fish count)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSwarmGetStatus)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "swarm_get_consensus",
		Description: autoDescOr("swarm_get_consensus", "Latest swarm consensus (fish majority vote + divergence)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSwarmGetConsensus)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "swarm_get_anomalies",
		Description: autoDescOr("swarm_get_anomalies", "Anomalies detected by the swarm over the most recent window."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSwarmGetAnomalies)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "swarm_get_scenarios",
		Description: autoDescOr("swarm_get_scenarios", "Active scenarios the swarm is monitoring (config + current state)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSwarmGetScenarios)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "swarm_get_strategies",
		Description: autoDescOr("swarm_get_strategies", "Strategy swarm composition (how many fish per strategy + voting weights)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSwarmGetStrategies)
}

type reportPrismSwarmBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleReportGetDailySummary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("report_get_daily_summary", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/daily-summary", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleReportGetPerformance(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("report_get_performance", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/performance-report", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleReportGetTaxSnapshot(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("report_get_tax_snapshot", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/tax-snapshot", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleReportGetExportLink(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("report_get_export_link", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/performance-report/export", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handlePrismGetTrainingResults(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("prism_get_training_results", nil, func() error {
		return s.cli.Get(ctx, "/api/prism/training-results", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSwarmGetStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("swarm_get_status", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/swarm-status", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSwarmGetConsensus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("swarm_get_consensus", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/swarm-consensus", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSwarmGetAnomalies(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("swarm_get_anomalies", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/swarm-anomalies", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSwarmGetScenarios(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("swarm_get_scenarios", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/swarm-scenarios", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSwarmGetStrategies(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismSwarmBaseOutput, error) {
	var out reportPrismSwarmBaseOutput
	if err := s.withAudit("swarm_get_strategies", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/swarm-strategies", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismSwarmBaseOutput{}, err
	}
	return nil, out, nil
}
