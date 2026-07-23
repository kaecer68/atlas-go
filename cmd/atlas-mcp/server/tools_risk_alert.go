package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRiskAlertTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_metrics",
		Description: autoDescOr("risk_get_metrics", "Aggregate risk metrics (current regime risk, VaR estimate, drawdown, exposure) HTTP: GET /api/dashboard/risk. Alternative: risk_get_drawdown, risk_get_correlation_matrix."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetMetrics)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_correlation_matrix",
		Description: autoDescOr("risk_get_correlation_matrix", "Cross-strategy correlation matrix (risk concentration indicator) HTTP: GET /api/dashboard/correlation-matrix. Alternative: risk_get_metrics, crossmarket_get_correlation."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetCorrelationMatrix)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_drawdown",
		Description: autoDescOr("risk_get_drawdown", "Current drawdown, peak drawdown, recovery stats.  HTTP: GET /api/dashboard/drawdown. Alternative: risk_get_metrics, risk_exposure."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetDrawdown)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_calibration",
		Description: autoDescOr("risk_get_calibration", "Risk model calibration metrics (predicted vs realized VaR) HTTP: GET /api/dashboard/risk-calibration. Alternative: risk_get_metrics, risk_get_commentary."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetCalibration)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_get_commentary",
		Description: autoDescOr("risk_get_commentary", "Latest narrative risk commentary (auto-generated from the risk engine) HTTP: GET /api/risk/commentary. Alternative: risk_get_metrics, explain_market_move."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskGetCommentary)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_list",
		Description: autoDescOr("alert_list", "All alerts (with optional filters). Companion to alert_list_unacknowledged (Phase 1) which is unack-only."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertList)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_get_stats",
		Description: autoDescOr("alert_get_stats", "Alert statistics (counts by severity, by source, ack latency)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertGetStats)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_get_rules",
		Description: autoDescOr("alert_get_rules", "Configured alert rules (severity, threshold, channels)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertGetRules)
}

type riskAlertBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleRiskGetMetrics(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	var fetchErr error
	if err := s.withAudit(ctx, "risk_get_metrics", nil, func() error {
		fetchErr = s.cli.Get(ctx, "/api/dashboard/risk", nil, &out.Result)
		return fetchErr
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	if out.Result != nil {
		injectDataQuality(*out.Result, "risk_get_metrics", fetchErr)
	}
	return nil, out, nil
}

func (s *server) handleRiskGetCorrelationMatrix(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "risk_get_correlation_matrix", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/correlation-matrix", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetDrawdown(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "risk_get_drawdown", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/drawdown", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetCalibration(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "risk_get_calibration", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/risk-calibration", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskGetCommentary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "risk_get_commentary", nil, func() error {
		return s.cli.Get(ctx, "/api/risk/commentary", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertList(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "alert_list", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertGetStats(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "alert_get_stats", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/stats", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertGetRules(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, riskAlertBaseOutput, error) {
	var out riskAlertBaseOutput
	if err := s.withAudit(ctx, "alert_get_rules", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/rules", nil, &out.Result)
	}); err != nil {
		return nil, riskAlertBaseOutput{}, err
	}
	return nil, out, nil
}
