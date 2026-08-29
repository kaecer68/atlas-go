package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerReportPrismTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_daily_summary",
		Description: autoDescOr("report_get_daily_summary", "Daily summary report (text or structured) suitable for an LLM agent's morning briefing.  HTTP: GET /api/dashboard/daily-summary. Alternative: daily_report, report_get_performance."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleReportGetDailySummary)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_performance",
		Description: autoDescOr("report_get_performance", "Performance report (period-configurable) with attribution and risk-adjusted metrics.  HTTP: GET /api/dashboard/performance-report. Alternative: report_get_daily_summary, report_get_tax_snapshot."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleReportGetPerformance)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_tax_snapshot",
		Description: autoDescOr("report_get_tax_snapshot", "Tax-relevant snapshot (realized gains, dividend totals, foreign tax) HTTP: GET /api/dashboard/tax-snapshot. Alternative: report_get_performance, report_get_export_link."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleReportGetTaxSnapshot)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "report_get_export_link",
		Description: autoDescOr("report_get_export_link", "Signed export link for a report variant (expires after a short TTL) HTTP: GET /api/dashboard/performance-report/export. Alternative: report_get_performance, report_get_tax_snapshot."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleReportGetExportLink)
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
