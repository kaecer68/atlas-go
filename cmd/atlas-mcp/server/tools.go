package server

//go:generate go run ../descgen -out ../auto-desc.gen.json -pkgdir .

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisteredToolCount is incremented by countedAddTool for every successfully
// registered MCP tool. It is used by server.go to assert the tool inventory
// has not drifted at startup.
var RegisteredToolCount int

// countedAddTool is a thin wrapper around mcp.AddTool that tracks the total
// tool count for startup assertions. It MUST be used instead of calling
// mcp.AddTool directly in all tool registration functions.
func countedAddTool[In any, Out any](mcpSrv *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(mcpSrv, tool, handler)
	RegisteredToolCount++
}

// registerTools attaches the five Phase 1 core tools to mcpSrv. Each handler
// invokes atlas-go via the shared HttpClient and writes one AuditEntry.
//
// The tool descriptions follow docs/reference/tool-catalog.md §"高頻工具 Top 15" so a
// reading agent recognizes them. JSON schemas are derived automatically from
// the Input structs (per OFFICIAL go-sdk convention with `jsonschema` tags).
func registerTools(mcpSrv *mcp.Server, s *server) {
	registerMacroTools(mcpSrv, s)
	registerCrossmarketTools(mcpSrv, s)
	registerNarrativeTools(mcpSrv, s)
	registerEventTools(mcpSrv, s)
	registerTemplateDetectorTools(mcpSrv, s)
	registerRiskAlertTools(mcpSrv, s)
	registerStrategyTools(mcpSrv, s)
	registerCapitalFlowTools(mcpSrv, s)
	registerMarketExplainTools(mcpSrv, s)
	registerRecommendationTools(mcpSrv, s)
	registerStockTools(mcpSrv, s)
	registerStrategyRankerTools(mcpSrv, s)
	registerExperimentTools(mcpSrv, s)
	registerSynergyTools(mcpSrv, s)
	registerControlTools(mcpSrv, s)
	registerSchedulerTaskTools(mcpSrv, s)
	registerSystemTools(mcpSrv, s)
	registerLLMTraceTools(mcpSrv, s)
	registerDataUniverseTools(mcpSrv, s)
	registerReportPrismTools(mcpSrv, s)
	registerAnomalyTools(mcpSrv, s)
	registerSamplingTools(mcpSrv, s)
	registerRootsTools(mcpSrv, s)
	registerElicitationTools(mcpSrv, s)
	registerBriefingTools(mcpSrv, s)
	registerParametersTools(mcpSrv, s)
	registerBacktestTools(mcpSrv, s)
	registerIndustryExtTools(mcpSrv, s)
	registerSectorTools(mcpSrv, s)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "regime_get_history",
		Description: autoDescOr("regime_get_history", "Return the market regime history for the last N days. Regimes are RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRegimeGetHistory)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_list_active",
		Description: autoDescOr("strategy_list_active", "List the market technique set (L1-L5 signal detectors) currently active in production. Includes detectors like foreign-3day-inflow, margin-balance-extreme — these are trading signal rules, NOT portfolio allocation strategies. For portfolio strategies (growth, momentum, defensive), use get_recommendations."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyListActive)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_judge",
		Description: autoDescOr("experiment_judge", "Trigger LLM judge scoring for a candidate experiment vs the baseline. Side-effect: writes to experiment history."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleExperimentJudge)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "alert_list_unacknowledged",
		Description: autoDescOr("alert_list_unacknowledged", "List all unacknowledged alerts. Use alert_acknowledge / alert_resolve via direct HTTP for state changes (those remain out of Phase 1 scope)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleAlertListUnacknowledged)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_health",
		Description: autoDescOr("system_get_health", "Return overall system health status (per docs/workflow-map.md WA-606)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetHealth)
}

func boolPtr(b bool) *bool { return &b }

// --- Input / Output schemas ---------------------------------------------------

type RegimeGetHistoryInput struct {
	Days int `json:"days" jsonschema:"how many days back; default 30, max 365"`
}

// RegimePoint represents one session in regime_get_history output.
// Score is a float64 pointer with omitempty: when the handler cannot supply
// a meaningful historical score, the field is omitted — honest "unknown".
// The current composite score is carried separately in the output envelope
// (CurrentRegimeScore), NOT cloned into every historical row.
type RegimePoint struct {
	Date   string   `json:"date"`
	Regime string   `json:"regime"`
	Score  *float64 `json:"score,omitempty"`
}

type RegimeGetHistoryOutput struct {
	Regimes               []RegimePoint `json:"regimes"`
	CurrentRegimeScore    *float64      `json:"current_regime_score,omitempty"`
	CurrentScoreSource    string        `json:"current_score_source,omitempty"`    // "janus_composite" or ""
	CurrentScoreSynthetic bool          `json:"current_score_synthetic,omitempty"` // true when score is macro-derived, not from PRISM training
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
	q := map[string]string{"limit": fmt.Sprintf("%d", in.Days)}
	var out RegimeGetHistoryOutput
	if err := s.withAudit(ctx, "regime_get_history", []string{"days"}, func() error {
		var raw struct {
			Sessions []struct {
				SessionID  string `json:"session_id"`
				Regime     string `json:"regime"`
				RecordedAt string `json:"recorded_at"`
			} `json:"sessions"`
			Current string `json:"current_regime"`
		}
		if err := s.cli.Get(ctx, "/api/dashboard/regime-history", urlValues(q), &raw); err != nil {
			return err
		}
		out.Regimes = make([]RegimePoint, len(raw.Sessions))
		for i, sess := range raw.Sessions {
			out.Regimes[i] = RegimePoint{
				Date:   sess.RecordedAt,
				Regime: sess.Regime,
			}
			// Score intentionally left nil — historical scores are not
			// yet persisted (regime_history table has no score column).
			// When they become available, each row will carry its own
			// historical Score. Until then, consumers should use
			// CurrentRegimeScore for the latest composite snapshot.
		}
		// Current composite score from /api/janus/regime-score, reported
		// once at the output envelope level — NOT cloned into every row.
		if score, isSynthetic, ok := fetchRegimeScore(ctx, s); ok {
			out.CurrentRegimeScore = &score
			out.CurrentScoreSource = "janus_composite"
			out.CurrentScoreSynthetic = isSynthetic
		}
		return nil
	}); err != nil {
		return nil, RegimeGetHistoryOutput{}, err
	}
	return nil, out, nil
}

// fetchRegimeScore queries /api/janus/regime-score and returns the float64
// composite score plus the is_synthetic flag. Returns ok=false when the
// endpoint is unavailable. The score is kept as float64 to preserve precision;
// int truncation (e.g. 0.018 → 0) is a data-integrity bug (#1263).
func fetchRegimeScore(ctx context.Context, s *server) (float64, bool, bool) {
	var raw struct {
		Score       float64 `json:"score"`
		IsSynthetic bool    `json:"is_synthetic"`
	}
	if err := s.cli.Get(ctx, "/api/janus/regime-score", nil, &raw); err != nil {
		return 0, false, false
	}
	return raw.Score, raw.IsSynthetic, true
}

func (s *server) handleStrategyListActive(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, StrategyListActiveOutput, error) {
	var out StrategyListActiveOutput
	if err := s.withAudit(ctx, "strategy_list_active", nil, func() error {
		if err := s.cli.Get(ctx, "/api/strategies/active", nil, &out); err != nil {
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
		var wrapper struct {
			Alerts []map[string]any `json:"alerts"`
		}
		if err := s.cli.Get(ctx, "/api/alerts/unacknowledged", nil, &wrapper); err != nil {
			return err
		}
		out.Alerts = wrapper.Alerts
		return nil
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
		if st, _ := raw["status"].(string); st != "" {
			// Legacy contract: backend provided an explicit status.
			out.Status = st
			delete(raw, "status")
		} else {
			// Production SystemHealthResponse has no status field
			// (internal/monitoring/service/system.go) — derive it.
			out.Status = deriveSystemHealthStatus(raw)
		}
		if len(raw) > 0 {
			out.Info = raw
		}
		return nil
	}); err != nil {
		return nil, SystemHealthOutput{}, err
	}
	return nil, out, nil
}

// deriveSystemHealthStatus computes an overall status from the
// SystemHealthResponse payload: any failed integrity check or non-ok data
// channel degrades the status; otherwise "ok".
func deriveSystemHealthStatus(raw map[string]any) string {
	info, _ := raw["info"].(map[string]any)
	if info == nil {
		info = raw // tolerate a flattened payload
	}
	if v, ok := info["replay_data_path_ok"].(bool); ok && !v {
		return "degraded"
	}
	if v, ok := info["cycle_stale"].(bool); ok && v {
		return "degraded"
	}
	if channels, ok := info["data_channels"].([]any); ok {
		for _, ch := range channels {
			m, _ := ch.(map[string]any)
			if st, _ := m["status"].(string); st != "" && st != "ok" {
				return "degraded"
			}
		}
	}
	return "ok"
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
		// Audit is best-effort: a write failure must never poison
		// the tool result (#1267). Log to stderr so operators can
		// detect a poisoned AuditWriter without blocking callers.
		fmt.Fprintf(os.Stderr, "atlas-mcp: audit write failed for tool %q: %v\n", tool, wErr)
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
