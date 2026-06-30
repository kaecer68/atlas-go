package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools attaches the five Phase 1 core tools to mcpSrv. Each handler
// invokes atlas-go via the shared httpClient and writes one AuditEntry.
//
// The tool descriptions follow docs/AGENT_TOOLS.md §"高頻工具 Top 15" so a
// reading agent recognizes them. JSON schemas are derived automatically from
// the Input structs (per OFFICIAL go-sdk convention with `jsonschema` tags).
func registerTools(mcpSrv *mcp.Server, s *server) {
	registerMacroTools(mcpSrv, s)
	registerCrossmarketTools(mcpSrv, s)
	registerNarrativeTools(mcpSrv, s)
	registerRiskAlertTools(mcpSrv, s)
	registerStrategyTools(mcpSrv, s)
	registerExperimentTools(mcpSrv, s)
	registerSynergyTools(mcpSrv, s)
	registerControlTools(mcpSrv, s)
	registerSchedulerTaskTools(mcpSrv, s)
	registerSystemTools(mcpSrv, s)
	registerLLMTraceTools(mcpSrv, s)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "regime_get_history",
		Description: "Return the market regime history for the last N days. Regimes are RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRegimeGetHistory)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_list_active",
		Description: "List the strategy set currently active in production (per docs/WORKFLOW_MAP.md WA-500).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyListActive)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_judge",
		Description: "Trigger LLM judge scoring for a candidate experiment vs the baseline. Side-effect: writes to experiment history.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleExperimentJudge)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_list_unacknowledged",
		Description: "List all unacknowledged alerts. Use alert_acknowledge / alert_resolve via direct HTTP for state changes (those remain out of Phase 1 scope).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertListUnacknowledged)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_health",
		Description: "Return overall system health status (per docs/WORKFLOW_MAP.md WA-606).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetHealth)
}

func boolPtr(b bool) *bool { return &b }

// --- Input / Output schemas ---------------------------------------------------

type RegimeGetHistoryInput struct {
	Days int `json:"days" jsonschema:"how many days back; default 30, max 365"`
}

type RegimePoint struct {
	Date   string `json:"date"`
	Regime string `json:"regime"`
	Score  int    `json:"score"`
}

type RegimeGetHistoryOutput struct {
	Regimes []RegimePoint `json:"regimes"`
}

type StrategyListActiveOutput struct {
	Strategies []map[string]any `json:"strategies"`
}

type ExperimentJudgeInput struct {
	ExperimentID string `json:"experiment_id" jsonschema:"the experiment id to judge"`
}

type ExperimentJudgeOutput struct {
	Result map[string]any `json:"result"`
}

type AlertListUnacknowledgedOutput struct {
	Alerts []map[string]any `json:"alerts"`
}

type SystemHealthOutput struct {
	Status string         `json:"status"`
	Info   map[string]any `json:"info,omitempty"`
}

// --- Handlers ----------------------------------------------------------------

func (s *server) handleRegimeGetHistory(ctx context.Context, _ *mcp.CallToolRequest, in RegimeGetHistoryInput) (*mcp.CallToolResult, RegimeGetHistoryOutput, error) {
	if in.Days <= 0 {
		in.Days = 30
	}
	if in.Days > 365 {
		in.Days = 365
	}
	q := map[string]string{"days": fmt.Sprintf("%d", in.Days)}
	var out RegimeGetHistoryOutput
	if err := s.withAudit("regime_get_history", []string{"days"}, func() error {
		var raw []RegimePoint
		if err := s.cli.Get(ctx, "/api/dashboard/regime-history", urlValues(q), &raw); err != nil {
			return err
		}
		out.Regimes = raw
		return nil
	}); err != nil {
		return nil, RegimeGetHistoryOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStrategyListActive(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, StrategyListActiveOutput, error) {
	var out StrategyListActiveOutput
	if err := s.withAudit("strategy_list_active", nil, func() error {
		if err := s.cli.Get(ctx, "/api/strategies/active", nil, &out.Strategies); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, StrategyListActiveOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleExperimentJudge(ctx context.Context, _ *mcp.CallToolRequest, in ExperimentJudgeInput) (*mcp.CallToolResult, ExperimentJudgeOutput, error) {
	if in.ExperimentID == "" {
		return nil, ExperimentJudgeOutput{}, errors.New("experiment_judge: experiment_id is required")
	}
	var out ExperimentJudgeOutput
	body := map[string]string{"experiment_id": in.ExperimentID}
	if err := s.withAudit("experiment_judge", []string{"experiment_id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/experiment/judge", body, &out.Result)
	}); err != nil {
		return nil, ExperimentJudgeOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertListUnacknowledged(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, AlertListUnacknowledgedOutput, error) {
	var out AlertListUnacknowledgedOutput
	if err := s.withAudit("alert_list_unacknowledged", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/unacknowledged", nil, &out.Alerts)
	}); err != nil {
		return nil, AlertListUnacknowledgedOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetHealth(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SystemHealthOutput, error) {
	var raw map[string]any
	var out SystemHealthOutput
	if err := s.withAudit("system_get_health", nil, func() error {
		if err := s.cli.Get(ctx, "/api/dashboard/system-health", nil, &raw); err != nil {
			return err
		}
		out.Status, _ = raw["status"].(string)
		delete(raw, "status")
		if len(raw) > 0 {
			out.Info = raw
		}
		return nil
	}); err != nil {
		return nil, SystemHealthOutput{}, err
	}
	return nil, out, nil
}

// withAudit is the standard wrapper for tool handlers: it measures latency and
// emits one AuditEntry per call, regardless of success/failure.
func (s *server) withAudit(tool string, argKeys []string, fn func() error) error {
	start := time.Now()
	err := fn()
	entry := AuditEntry{
		Tool:       tool,
		ArgKeys:    argKeys,
		DurationMS: time.Since(start).Milliseconds(),
		Status:     "ok",
	}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	}
	if wErr := s.audit.Write(entry); wErr != nil {
		// audit failure must not mask the original error.
		if err == nil {
			return fmt.Errorf("audit: %w", wErr)
		}
	}
	return err
}
