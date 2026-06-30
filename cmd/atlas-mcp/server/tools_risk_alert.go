package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRiskAlertTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_metrics",
		Description: "Aggregate risk metrics (current regime risk, VaR estimate, drawdown, exposure).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetMetrics)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_correlation_matrix",
		Description: "Cross-strategy correlation matrix (risk concentration indicator).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetCorrelationMatrix)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_drawdown",
		Description: "Current drawdown, peak drawdown, recovery stats.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetDrawdown)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_calibration",
		Description: "Risk model calibration metrics (predicted vs realized VaR).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetCalibration)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_commentary",
		Description: "Latest narrative risk commentary (auto-generated from the risk engine).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetCommentary)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_list",
		Description: "All alerts (with optional filters). Companion to alert_list_unacknowledged (Phase 1) which is unack-only.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertList)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_get_stats",
		Description: "Alert statistics (counts by severity, by source, ack latency).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertGetStats)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_get_rules",
		Description: "Configured alert rules (severity, threshold, channels).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertGetRules)
}

type riskAlertBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleRiskGetMetrics(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("risk_get_metrics", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/risk", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetCorrelationMatrix(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("risk_get_correlation_matrix", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/correlation-matrix", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetDrawdown(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("risk_get_drawdown", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/drawdown", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetCalibration(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("risk_get_calibration", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/risk-calibration", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetCommentary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("risk_get_commentary", nil, func() error {
		return s.cli.Get(ctx, "/api/risk/commentary", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertList(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("alert_list", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertGetStats(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("alert_get_stats", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/stats", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertGetRules(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit("alert_get_rules", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/rules", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}
