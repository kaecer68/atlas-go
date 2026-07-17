package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerReportPrismTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_daily_summary",
		Description: autoDescOr("report_get_daily_summary", "Daily summary report (text or structured) suitable for an LLM agent's morning briefing."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetDailySummary)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_performance",
		Description: autoDescOr("report_get_performance", "Performance report (period-configurable) with attribution and risk-adjusted metrics."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetPerformance)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_tax_snapshot",
		Description: autoDescOr("report_get_tax_snapshot", "Tax-relevant snapshot (realized gains, dividend totals, foreign tax)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetTaxSnapshot)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_export_link",
		Description: autoDescOr("report_get_export_link", "Signed export link for a report variant (expires after a short TTL)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleReportGetExportLink)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "prism_get_training_results",
		Description: autoDescOr("prism_get_training_results", "Latest PRISM cohort training results (config + metrics per cohort)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handlePrismGetTrainingResults)
}

type reportPrismBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleReportGetDailySummary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismBaseOutput, error) {
	var out reportPrismBaseOutput
	if err := s.withAudit(ctx, "report_get_daily_summary", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/daily-summary", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleReportGetPerformance(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismBaseOutput, error) {
	var out reportPrismBaseOutput
	if err := s.withAudit(ctx, "report_get_performance", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/performance-report", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleReportGetTaxSnapshot(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismBaseOutput, error) {
	var out reportPrismBaseOutput
	if err := s.withAudit(ctx, "report_get_tax_snapshot", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/tax-snapshot", nil, &out.Result)
	}); err != nil {
		return nil, reportPrismBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleReportGetExportLink(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reportPrismBaseOutput, error) {
	var out reportPrismBaseOutput
	if err := s.withAudit(ctx, "report_get_export_link", nil, func() error {
		md, err := s.cli.GetRaw(ctx, "/api/dashboard/performance-report/export", nil)
		if err != nil {
			return err
		}
		result := map[string]any{"markdown": string(md)}
		out.Result = &result
		return nil
	}); err != nil {
		return nil, reportPrismBaseOutput{}, err
	}
	return nil, out, nil
}

// prismTrainingResultsOutput decodes the JSON array returned by
// GET /api/prism/training-results. Items stay as map[string]any to keep
// MCP schema decoupled from the cohort result type.
type prismTrainingResultsOutput struct {
	Results []map[string]any `json:"results"`
}

func (s *server) handlePrismGetTrainingResults(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, prismTrainingResultsOutput, error) {
	var out prismTrainingResultsOutput
	if err := s.withAudit(ctx, "prism_get_training_results", nil, func() error {
		return s.cli.Get(ctx, "/api/prism/training-results", nil, &out.Results)
	}); err != nil {
		return nil, prismTrainingResultsOutput{}, err
	}
	return nil, out, nil
}
