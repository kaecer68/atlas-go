//go:build canary

// Package server contains the MCP canary test suite.
//
// Run against a live atlas-go instance:
//
//	ATLAS_BASE_URL=http://127.0.0.1:18080 go test -run TestCanary -tags=canary -count=1 -v ./cmd/atlas-mcp/server/
//
// The canary exercises every read-only MCP tool's upstream route,
// verifying HTTP 200, non-empty response, and basic data sanity.
// Destructive tools (experiment_judge, control_pause_agent, etc.)
// are skipped to avoid side effects.
package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// canaryTest maps an MCP tool name to its upstream HTTP endpoint.
type canaryTest struct {
	Path   string   // upstream GET path
	Method string   // HTTP method (default "GET")
	Keys   []string // top-level JSON keys that must exist
}

// skipCanary lists tools that are destructive, require special auth
// state, or require a parameter/real data that a bare GET canary cannot
// supply. The canary cannot safely exercise these.
var skipCanary = map[string]bool{
	"experiment_judge":               true,
	"experiment_promote":             true,
	"experiment_revert":              true,
	"mcp_anomaly_ack":                true,
	"control_pause_agent":            true,
	"control_resume_agent":           true,
	"control_sector_ban":             true,
	"control_reject_recommendation":  true,
	"control_approve_recommendation": true,
	"sample_createMessage":           true,
	"elicitation_elicit":             true,
	// Require a parameter (experiment_id) or live session data that a
	// bare GET cannot provide — not route gaps.
	"experiment_diff":             true,
	"universe_get_session_detail": true,
	// Destructive / write side effects.
	"alert_acknowledge": true,
	"alert_resolve":     true,
	"alert_silence":     true,
	"alert_scan":        true,
	// Local-only (embedded snapshot / file access) — no upstream route.
	"audit_state":         true,
	"mcp_roots_read_file": true,
	// Require a parameter (symbol / period / task id) a bare GET cannot supply.
	"stock_get_monthly_revenue": true,
	"strategy_for_period":       true,
	"task_get":                  true,
	"task_get_events":           true,
}

// requiresKey lists tools whose upstream routes refuse to serve when the
// server has no ATLAS_API_KEY configured (production mode → 503 "server
// misconfigured: ATLAS_API_KEY required in production"). CI runs with an
// empty .env (no secrets), so these are skipped there; the canary still
// exercises them when a key is present.
var requiresKey = map[string]bool{
	"mcp_get_call_stats":       true,
	"mcp_get_session_topology": true,
	"mcp_get_tenant_usage":     true,
	"mcp_get_top_slow_tools":   true,
	"mcp_roots_list":           true,
	"mcp_quickstart":           true,
	"mcp_anomaly_get_recent":   true,
}

// requiresLLMKey lists tools that need an LLM annotator client; without
// LLM_ANNOTATOR_API_KEY the upstream route returns 503 ("no KimiClient
// wired"). Skipped in CI, exercised when a key is present.
var requiresLLMKey = map[string]bool{
	"llm_get_cost": true,
}

// tolerateEnvFailures / matchesAnyMarker / truncate / classifyCanaryResponse
// and the warmup-grace window live in canary_tolerance_test.go (no build tag)
// so the offline unit tests run in normal `go test` (ci-full). The warmup-503
// class (capital_flow_summary, get_recommendations, ...) is covered by the
// unified warmup-grace window — no per-tool markers needed.

// canaryRoutes maps MCP tool names to upstream routes.
// Keep in sync with tool registration files (tools.go, tools_*.go).
var canaryRoutes = map[string]canaryTest{
	"regime_get_history":                {Path: "/api/regime/history?days=7"},
	"strategy_list_active":              {Path: "/api/strategies/active"},
	"alert_list_unacknowledged":         {Path: "/api/alerts/unacknowledged"},
	"system_get_health":                 {Path: "/api/dashboard/system-health"},
	"macro_get_snapshot_latest":         {Path: "/api/macro/snapshot/latest"},
	"macro_get_snapshot_history":        {Path: "/api/macro/snapshot/timeline"},
	"macro_get_capital_flow_latest":     {Path: "/api/macro/capital-flow/latest"},
	"macro_get_stress_index_current":    {Path: "/api/narrative/stress-index/current"},
	"macro_get_stress_index_history":    {Path: "/api/narrative/stress-index/history?days=7"},
	"macro_get_ingest_status":           {Path: "/api/dashboard/macro-data-health"},
	"crossmarket_get_us_indices":        {Path: "/api/dashboard/us-indices"},
	"crossmarket_get_correlation":       {Path: "/api/cross-market/correlation"},
	"crossmarket_get_status":            {Path: "/api/cross-market/status"},
	"narrative_get_bundle":              {Path: "/api/narrative/bundle"},
	"narrative_get_events":              {Path: "/api/narrative/events"},
	"narrative_get_chains":              {Path: "/api/narrative/chains"},
	"narrative_get_models":              {Path: "/api/narrative/models"},
	"narrative_get_model_inventory":     {Path: "/api/narrative/models/inventory"},
	"narrative_get_templates":           {Path: "/api/narrative/templates"},
	"narrative_get_seasonal":            {Path: "/api/narrative/seasonal"},
	"narrative_stress_index_thresholds": {Path: "/api/narrative/stress-index/thresholds"},
	"event_calendar":                    {Path: "/api/events/calendar"},
	"event_flow_prediction":             {Path: "/api/events/prediction"},
	"template_detector_status":          {Path: "/api/detector/scan/status"},
	"risk_get_metrics":                  {Path: "/api/dashboard/risk"},
	"risk_get_calibration":              {Path: "/api/dashboard/risk-calibration"},
	"risk_get_commentary":               {Path: "/api/risk/commentary"},
	"risk_get_correlation_matrix":       {Path: "/api/dashboard/correlation-matrix"},
	"risk_get_drawdown":                 {Path: "/api/dashboard/drawdown"},
	"risk_exposure":                     {Path: "/api/dashboard/risk-exposure"},
	"alert_get_rules":                   {Path: "/api/alerts/rules"},
	"alert_get_stats":                   {Path: "/api/alerts/stats"},
	"alert_list":                        {Path: "/api/alerts"},
	"capital_flow_daily":                {Path: "/api/capital-flow/daily"},
	"capital_flow_summary":              {Path: "/api/capital-flow/summary"},
	"explain_market_move":               {Path: "/api/market/explain"},
	"stock_get_quote":                   {Path: "/api/stock/quote?symbol=2330"},
	"stock_get_fundamentals":            {Path: "/api/stock/fundamentals?symbol=2330"},
	"stock_get_technical":               {Path: "/api/stock/technical?symbol=2330&days=10"},
	"channel_health":                    {Path: "/api/dashboard/channel-health"},
	"control_get_audit_log":             {Path: "/api/control/audit-log"},
	"control_get_active_overrides":      {Path: "/api/control/active-overrides"},
	"stock_get_chips":                   {Path: "/api/stock/chips?symbol=2330"},
	"get_recommendations":               {Path: "/api/recommendations"},
	"strategy_ranker":                   {Path: "/api/strategy-ranker/rank"},
	"strategy_get":                      {Path: "/api/strategies/foreign-3day-inflow"},
	"strategy_get_summary":              {Path: "/api/strategies/foreign-3day-inflow/summary"},
	"strategy_get_attribution":          {Path: "/api/strategies/foreign-3day-inflow/attribution"},
	"strategy_get_layers":               {Path: "/api/strategies/layers"},
	"experiment_diff":                   {Path: "/api/experiment/diff"},
	"experiment_history":                {Path: "/api/experiment/history"},
	"synergy_get_darwinian_status":      {Path: "/api/synergy/darwinian/status"},
	"synergy_get_darwinian_trend":       {Path: "/api/synergy/darwinian/trend"},
	"synergy_get_l2_4_schedule":         {Path: "/api/synergy/l2-4-schedule"},
	"system_get_circuit_breaker":        {Path: "/api/dashboard/circuit-breaker"},
	"system_get_data_pipeline":          {Path: "/api/dashboard/data-pipeline"},
	"system_get_metrics":                {Path: "/api/dashboard/metrics"},
	"system_get_metrics_trend":          {Path: "/api/dashboard/metrics/trend?minutes=30"},
	"system_get_thresholds":             {Path: "/api/dashboard/metrics/thresholds"},
	"system_get_maturity":               {Path: "/api/dashboard/maturity"},
	"llm_get_cost":                      {Path: "/api/llm_annotator/cost"},
	"llm_get_health":                    {Path: "/api/llm/health"},
	"data_get_channels":                 {Path: "/api/dashboard/data-channels"},
	"data_get_channel_detail":           {Path: "/api/dashboard/data-channels/fugle"},
	"data_get_field_contract":           {Path: "/api/field-contract"},
	"data_get_quality":                  {Path: "/api/dashboard/data-quality"},
	"universe_get_sessions":             {Path: "/api/dashboard/sessions"},
	"universe_get_session_detail":       {Path: "/api/dashboard/sessions/latest"},
	"universe_get_universe_overlap":     {Path: "/api/dashboard/universe-overlap"},
	"report_get_daily_summary":          {Path: "/api/dashboard/daily-summary"},
	"report_get_performance":            {Path: "/api/dashboard/performance-report"},
	"report_get_tax_snapshot":           {Path: "/api/dashboard/tax-snapshot"},
	"report_get_export_link":            {Path: "/api/dashboard/performance-report/export"},
	"backtest_status":                   {Path: "/api/backtest/status"},
	"backtest_signals":                  {Path: "/api/backtest/signals"},
	"mcp_anomaly_get_recent":            {Path: "/api/mcp/anomalies/recent"},
	"sector_allocation_plan":            {Path: "/api/dashboard/sector-allocation-plan"},
	"industry_sector_list":              {Path: "/api/industry/sectors"},
	"industry_sector_lookup":            {Path: "/api/industry/sector-lookup?symbol=2330"},
	"scheduler_get_status":              {Path: "/api/scheduler/status"},
	"task_list":                         {Path: "/api/tasks"},
	"trace_get_sim_latest":              {Path: "/api/traces/sim-latest"},
	"trace_get_agent_observatory":       {Path: "/api/dashboard/agent-observatory"},
	"trace_get_decision_chain":          {Path: "/api/dashboard/decision-chain?symbol=2330"},
	"trace_get_reasoning":               {Path: "/api/dashboard/reasoning-trace?session_id=latest"},
	"parameters_get":                    {Path: "/api/parameters"},
	"parameters_get_metadata":           {Path: "/api/parameters/metadata"},
	"parameters_get_categories":         {Path: "/api/parameters/categories"},
	"parameters_get_snapshots":          {Path: "/api/parameters/snapshots"},
	"parameters_get_audit_log":          {Path: "/api/parameters/audit-log"},
	"mcp_get_call_stats":                {Path: "/api/mcp/call-stats"},
	"mcp_get_session_topology":          {Path: "/api/mcp/session-topology"},
	"mcp_get_tenant_usage":              {Path: "/api/mcp/tenant-usage"},
	"mcp_get_top_slow_tools":            {Path: "/api/mcp/top-slow-tools"},
	"mcp_roots_list":                    {Path: "/api/mcp/roots"},
	"detector_registry_list":            {Path: "/api/detector/registry/list"},
	"daily_report":                      {Path: "/api/reports/latest"},
	"mcp_quickstart":                    {Path: "/api/mcp/quickstart"},
}

// TestCanary_Runtime exercises every read-only MCP tool's upstream
// HTTP route against a live atlas-go instance.
func TestCanary_Runtime(t *testing.T) {
	baseURL := os.Getenv("ATLAS_BASE_URL")
	if baseURL == "" {
		t.Skip("ATLAS_BASE_URL not set — skipping runtime canary (requires live atlas-go instance)")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	ctx := context.Background()
	results := make(map[string]string, len(canaryRoutes))

	// Warmup-grace window: the container reports healthy while background
	// RunWarmup is still running, so a fresh service can legitimately 503
	// (or briefly refuse connections) right after startup. The unified grace
	// window tolerates those for warmupGraceDuration; 503s after that FAIL
	// as real regressions (see canary_tolerance_test.go).
	graceUntil := time.Now().Add(warmupGraceDuration())
	t.Logf("canary warmup-grace window: %s (until %s)", warmupGraceDuration(), graceUntil.Format(time.RFC3339))

	for name, ct := range canaryRoutes {
		if skipCanary[name] {
			results[name] = "skipped (destructive)"
			continue
		}
		if requiresKey[name] && os.Getenv("ATLAS_API_KEY") == "" {
			results[name] = "skipped (requires ATLAS_API_KEY)"
			continue
		}
		if requiresLLMKey[name] && os.Getenv("LLM_ANNOTATOR_API_KEY") == "" {
			results[name] = "skipped (requires LLM_ANNOTATOR_API_KEY)"
			continue
		}

		url := strings.TrimRight(baseURL, "/") + ct.Path
		method := ct.Method
		if method == "" {
			method = "GET"
		}

		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			results[name] = fmt.Sprintf("FAIL: request build error: %v", err)
			continue
		}
		if key := os.Getenv("ATLAS_API_KEY"); key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}

		resp, err := client.Do(req)
		if err != nil {
			results[name] = classifyCanaryResponse(0, "", name, err.Error(), time.Now(), graceUntil)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		code := resp.StatusCode
		body := strings.TrimSpace(string(bodyBytes))
		results[name] = classifyCanaryResponse(code, body, name, "", time.Now(), graceUntil)
	}

	failures, warnings, skipped, passed := 0, 0, 0, 0
	for tool, status := range results {
		switch {
		case strings.HasPrefix(status, "FAIL"):
			t.Errorf("%-50s %s", tool, status)
			failures++
		case strings.HasPrefix(status, "WARN"):
			t.Logf("%-50s %s", tool, status)
			warnings++
		case strings.HasPrefix(status, "skipped"):
			skipped++
		default:
			passed++
		}
	}
	t.Logf("canary: %d passed | %d warnings | %d skipped | %d FAILED | %d total",
		passed, warnings, skipped, failures, len(results))
	if failures > 0 {
		t.Fatalf("canary failed: %d tools returned errors", failures)
	}
}
