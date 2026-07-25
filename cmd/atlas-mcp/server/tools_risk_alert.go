package server

import (
	"context"
	"errors"

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

	// Phase 2 (Route C): write-capable alert lifecycle tools.
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_scan",
		Description: autoDescOr("alert_scan", "Scan for all unacknowledged alerts (startup rescan). Returns active alert counts and blocker status."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertScan)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_acknowledge",
		Description: autoDescOr("alert_acknowledge", "Acknowledge an alert by id. Side-effect: persists acknowledged status to alert store."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleAlertAcknowledge)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_resolve",
		Description: autoDescOr("alert_resolve", "Resolve an alert by id. Side-effect: persists resolved status to alert store."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleAlertResolve)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_silence",
		Description: autoDescOr("alert_silence", "Silence all non-resolved alerts matching a rule for a duration. Side-effect: persists silenced status."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleAlertSilence)
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

// --- Phase 2 (Route C) alert lifecycle handlers ---

// AlertScanOutput is the output schema for alert_scan.
type AlertScanOutput struct {
	Result map[string]any `json:"result"`
}

// AlertAcknowledgeInput is the input schema for alert_acknowledge.
type AlertAcknowledgeInput struct {
	AlertID string `json:"id" jsonschema:"the alert id to acknowledge"`
	User    string `json:"user,omitempty" jsonschema:"who is acknowledging (optional)"`
}

// AlertResolveInput is the input schema for alert_resolve.
type AlertResolveInput struct {
	AlertID string `json:"id" jsonschema:"the alert id to resolve"`
	User    string `json:"user,omitempty" jsonschema:"who is resolving (optional)"`
}

// AlertSilenceInput is the input schema for alert_silence.
type AlertSilenceInput struct {
	Rule        string `json:"rule" jsonschema:"the alert rule to silence"`
	DurationMin int    `json:"duration_minutes" jsonschema:"how long to silence (minutes)"`
	Reason      string `json:"reason,omitempty" jsonschema:"why silencing (optional)"`
}

// AlertAcknowledgeOutput is the output schema for alert_acknowledge.
type AlertAcknowledgeOutput struct {
	Acknowledged bool `json:"acknowledged"`
}

// AlertResolveOutput is the output schema for alert_resolve.
type AlertResolveOutput struct {
	Resolved bool `json:"resolved"`
}

// AlertSilenceOutput is the output schema for alert_silence.
type AlertSilenceOutput struct {
	Rule          string `json:"rule"`
	SilencedUntil string `json:"silenced_until"`
	Reason        string `json:"reason,omitempty"`
	SilencedCount int    `json:"silenced_count"`
}

func (s *server) handleAlertScan(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, AlertScanOutput, error) {
	var out AlertScanOutput
	if err := s.withAudit(ctx, "alert_scan", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/unacknowledged", nil, &out.Result)
	}); err != nil {
		return nil, AlertScanOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertAcknowledge(ctx context.Context, _ *mcp.CallToolRequest, in AlertAcknowledgeInput) (*mcp.CallToolResult, AlertAcknowledgeOutput, error) {
	if in.AlertID == "" {
		return nil, AlertAcknowledgeOutput{}, errors.New("alert_acknowledge: id is required")
	}
	body := map[string]string{"id": in.AlertID}
	if in.User != "" {
		body["user"] = in.User
	}
	var out AlertAcknowledgeOutput
	if err := s.withAudit(ctx, "alert_acknowledge", []string{"id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/alerts/acknowledge", body, &out)
	}); err != nil {
		return nil, AlertAcknowledgeOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertResolve(ctx context.Context, _ *mcp.CallToolRequest, in AlertResolveInput) (*mcp.CallToolResult, AlertResolveOutput, error) {
	if in.AlertID == "" {
		return nil, AlertResolveOutput{}, errors.New("alert_resolve: id is required")
	}
	body := map[string]string{"id": in.AlertID}
	if in.User != "" {
		body["user"] = in.User
	}
	var out AlertResolveOutput
	if err := s.withAudit(ctx, "alert_resolve", []string{"id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/alerts/resolve", body, &out)
	}); err != nil {
		return nil, AlertResolveOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertSilence(ctx context.Context, _ *mcp.CallToolRequest, in AlertSilenceInput) (*mcp.CallToolResult, AlertSilenceOutput, error) {
	if in.Rule == "" {
		return nil, AlertSilenceOutput{}, errors.New("alert_silence: rule is required")
	}
	if in.DurationMin <= 0 {
		return nil, AlertSilenceOutput{}, errors.New("alert_silence: duration_minutes must be > 0")
	}
	body := map[string]any{
		"rule":             in.Rule,
		"duration_minutes": in.DurationMin,
	}
	if in.Reason != "" {
		body["reason"] = in.Reason
	}
	var out AlertSilenceOutput
	if err := s.withAudit(ctx, "alert_silence", []string{"rule"}, func() error {
		return s.cli.PostJSON(ctx, "/api/alerts/silence", body, &out)
	}); err != nil {
		return nil, AlertSilenceOutput{}, err
	}
	return nil, out, nil
}
