package server

//go:generate go run ../descgen -out ../auto-desc.gen.json -pkgdir .

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
	registerDataUniverseTools(mcpSrv, s)
	registerReportPrismSwarmTools(mcpSrv, s)
	registerAnomalyTools(mcpSrv, s)
	registerSamplingTools(mcpSrv, s)
	registerRootsTools(mcpSrv, s)
	registerElicitationTools(mcpSrv, s)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "regime_get_history",
		Description: autoDescOr("regime_get_history", "Return the market regime history for the last N days. Regimes are RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRegimeGetHistory)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_list_active",
		Description: autoDescOr("strategy_list_active", "List the strategy set currently active in production (per docs/WORKFLOW_MAP.md WA-500)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyListActive)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_judge",
		Description: autoDescOr("experiment_judge", "Trigger LLM judge scoring for a candidate experiment vs the baseline. Side-effect: writes to experiment history."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleExperimentJudge)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_list_unacknowledged",
		Description: autoDescOr("alert_list_unacknowledged", "List all unacknowledged alerts. Use alert_acknowledge / alert_resolve via direct HTTP for state changes (those remain out of Phase 1 scope)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertListUnacknowledged)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_health",
		Description: autoDescOr("system_get_health", "Return overall system health status (per docs/WORKFLOW_MAP.md WA-606)."),
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
	if err := s.withAudit(ctx, "regime_get_history", []string{"days"}, func() error {
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
	if err := s.withAudit(ctx, "strategy_list_active", nil, func() error {
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
	if err := s.withAudit(ctx, "experiment_judge", []string{"experiment_id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/experiment/judge", body, &out.Result)
	}); err != nil {
		return nil, ExperimentJudgeOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAlertListUnacknowledged(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, AlertListUnacknowledgedOutput, error) {
	var out AlertListUnacknowledgedOutput
	if err := s.withAudit(ctx, "alert_list_unacknowledged", nil, func() error {
		return s.cli.Get(ctx, "/api/alerts/unacknowledged", nil, &out.Alerts)
	}); err != nil {
		return nil, AlertListUnacknowledgedOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetHealth(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SystemHealthOutput, error) {
	var raw map[string]any
	var out SystemHealthOutput
	if err := s.withAudit(ctx, "system_get_health", nil, func() error {
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

// withAudit is the standard wrapper for tool handlers: it measures latency,
// enforces per-tenant per-tool rate limits (Phase 3 B), and emits one
// AuditEntry per call regardless of success/failure.
func (s *server) withAudit(ctx context.Context, tool string, argKeys []string, fn func() error) error {
	return s.withAuditExtra(ctx, tool, argKeys, nil, fn)
}

func (s *server) withAuditExtra(ctx context.Context, tool string, argKeys []string, extraFn func() map[string]any, fn func() error) error {
	start := time.Now()

	tenantID := TenantIDFromContext(ctx)
	agentID := AgentIDFromContext(ctx)

	// Rate limit gate. Caller is resolved from context; stdio or missing
	// identity defaults to "anonymous".
	if s.limiter != nil {
		if r := s.limiter.Allow(tool, tenantID); !r.Allowed {
			elapsed := time.Since(start)
			entry := AuditEntry{
				SchemaVersion: 2,
				Tool:          tool,
				TenantID:      tenantID,
				AgentID:       agentID,
				ArgKeys:       argKeys,
				ArgsHash:      CanonicalizeArgsHash(argKeys),
				DurationMS:    elapsed.Milliseconds(),
				LatencyMS:     elapsed.Milliseconds(),
				Transport:     "stdio",
				Status:        "ratelimited",
				Error:         fmt.Sprintf("retry after %s", r.RetryAfter),
			}
			s.observeAuditEntry(&entry, start)
			_ = s.audit.Write(entry)
			return fmt.Errorf("rate limited: %s: retry after %s", tool, r.RetryAfter)
		}
	}

	err := fn()
	elapsed := time.Since(start)
	entry := AuditEntry{
		SchemaVersion: 2,
		Tool:          tool,
		TenantID:      tenantID,
		AgentID:       agentID,
		ArgKeys:       argKeys,
		ArgsHash:      CanonicalizeArgsHash(argKeys),
		DurationMS:    elapsed.Milliseconds(),
		LatencyMS:     elapsed.Milliseconds(),
		Transport:     "stdio",
		Status:        "ok",
	}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	}
	s.observeAuditEntry(&entry, start)
	if extraFn != nil {
		entry.Extra = extraFn()
	}
	if wErr := s.audit.Write(entry); wErr != nil {
		// audit failure must not mask the original error.
		if err == nil {
			return fmt.Errorf("audit: %w", wErr)
		}
	}
	return err
}

func (s *server) observeAuditEntry(entry *AuditEntry, start time.Time) {
	if s.metrics != nil {
		_ = s.metrics.ObserveCall(entry.Tool, entry.Transport, entry.Status, time.Since(start))
	}
	if s.detector != nil {
		s.detector.Observe(entry)
	}
}
