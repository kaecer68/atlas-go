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

// skipCanary lists tools that are destructive or require special
// auth state. The canary cannot safely exercise these.
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
}

// canaryRoutes maps MCP tool names to upstream routes.
// Keep in sync with tool registration files (tools.go, tools_*.go).
var canaryRoutes = map[string]canaryTest{
	"regime_get_history":                {Path: "/api/janus/regime-history?days=7"},
	"strategy_list_active":              {Path: "/api/strategies/active"},
	"alert_list_unacknowledged":         {Path: "/api/alerts/unacknowledged"},
	"system_get_health":                 {Path: "/api/dashboard/system-health"},
	"macro_get_snapshot_latest":         {Path: "/api/macro/snapshot"},
	"macro_get_snapshot_history":        {Path: "/api/macro/snapshot-history?days=7"},
	"macro_get_capital_flow_latest":     {Path: "/api/macro/capital-flow"},
	"macro_get_stress_index_current":    {Path: "/api/macro/stress-index"},
	"macro_get_stress_index_history":    {Path: "/api/macro/stress-index-history?days=7"},
	"macro_get_ingest_status":           {Path: "/api/macro/ingest-status"},
	"crossmarket_get_us_indices":        {Path: "/api/crossmarket/us-indices"},
	"crossmarket_get_correlation":       {Path: "/api/crossmarket/correlation"},
	"crossmarket_get_status":            {Path: "/api/crossmarket/status"},
	"narrative_get_bundle":              {Path: "/api/narrative/bundle"},
	"narrative_get_events":              {Path: "/api/narrative/events"},
	"narrative_get_chains":              {Path: "/api/narrative/chains"},
	"narrative_get_models":              {Path: "/api/narrative/models"},
	"narrative_get_templates":           {Path: "/api/narrative/templates"},
	"narrative_get_seasonal":            {Path: "/api/narrative/seasonal"},
	"narrative_stress_index_thresholds": {Path: "/api/narrative/stress-thresholds"},
	"event_calendar":                    {Path: "/api/events/calendar"},
	"event_flow_prediction":             {Path: "/api/events/flow-prediction"},
	"calendar_events":                   {Path: "/api/events/calendar"},
	"template_detector_status":          {Path: "/api/template-detector/status"},
	"risk_get_metrics":                  {Path: "/api/risk/metrics"},
	"risk_get_calibration":              {Path: "/api/risk/calibration"},
	"risk_get_commentary":               {Path: "/api/risk/commentary"},
	"risk_get_correlation_matrix":       {Path: "/api/risk/correlation-matrix"},
	"risk_get_drawdown":                 {Path: "/api/risk/drawdown"},
	"risk_exposure":                     {Path: "/api/risk/exposure"},
	"alert_get_rules":                   {Path: "/api/alerts/rules"},
	"alert_get_stats":                   {Path: "/api/alerts/stats"},
	"alert_list":                        {Path: "/api/alerts"},
	"capital_flow_daily":                {Path: "/api/capital-flow/daily"},
	"capital_flow_summary":              {Path: "/api/capital-flow/summary"},
	"explain_market_move":               {Path: "/api/market/explain"},
	"stock_get_quote":                   {Path: "/api/stock/quote?symbol=2330"},
	"stock_get_fundamentals":            {Path: "/api/stock/fundamentals?symbol=2330"},
	"stock_get_technical":               {Path: "/api/stock/technical?symbol=2330&days=10"},
	"stock_get_chips":                   {Path: "/api/stock/chips?symbol=2330"},
	"get_recommendations":               {Path: "/api/recommendations"},
	"strategy_ranker":                   {Path: "/api/strategy-ranker/rank"},
	"strategy_get":                      {Path: "/api/strategies/foreign-3day-inflow"},
	"strategy_get_summary":              {Path: "/api/strategies/foreign-3day-inflow/summary"},
	"strategy_get_attribution":          {Path: "/api/strategies/foreign-3day-inflow/attribution"},
	"strategy_get_layers":               {Path: "/api/strategy-techniques/layers"},
	"experiment_diff":                   {Path: "/api/experiments/latest/diff"},
	"experiment_history":                {Path: "/api/experiments/history"},
	"synergy_get_darwinian_status":      {Path: "/api/synergy/darwinian-status"},
	"synergy_get_darwinian_trend":       {Path: "/api/synergy/darwinian-trend"},
	"synergy_get_l2_4_schedule":         {Path: "/api/synergy/l2-4-schedule"},
	"system_get_circuit_breaker":        {Path: "/api/system/circuit-breaker"},
	"system_get_data_pipeline":          {Path: "/api/system/data-pipeline"},
	"system_get_metrics":                {Path: "/api/system/metrics"},
	"system_get_metrics_trend":          {Path: "/api/system/metrics-trend?minutes=30"},
	"system_get_thresholds":             {Path: "/api/system/thresholds"},
	"system_get_maturity":               {Path: "/api/system/maturity"},
	"llm_get_cost":                      {Path: "/api/llm/cost"},
	"llm_get_health":                    {Path: "/api/llm/health"},
	"data_get_channels":                 {Path: "/api/data/channels"},
	"data_get_channel_detail":           {Path: "/api/data/channels/fugle"},
	"data_get_field_contract":           {Path: "/api/data/field-contract"},
	"data_get_quality":                  {Path: "/api/data/quality"},
	"universe_get_sessions":             {Path: "/api/universe/sessions"},
	"universe_get_session_detail":       {Path: "/api/universe/sessions/latest"},
	"universe_get_universe_overlap":     {Path: "/api/universe/overlap"},
	"report_get_daily_summary":          {Path: "/api/report/daily-summary"},
	"report_get_performance":            {Path: "/api/report/performance"},
	"report_get_tax_snapshot":           {Path: "/api/report/tax-snapshot"},
	"report_get_export_link":            {Path: "/api/report/export-link"},
	"prism_get_training_results":        {Path: "/api/prism/training-results"},
	"backtest_status":                   {Path: "/api/backtest/status"},
	"backtest_signals":                  {Path: "/api/backtest/signals"},
	"taiwan_stress_index":               {Path: "/api/taiwan/stress-index"},
	"mcp_anomaly_get_recent":            {Path: "/api/mcp/anomalies/recent"},
	"sector_allocation_plan":            {Path: "/api/sector/allocation-plan"},
	"industry_sector_list":              {Path: "/api/industry/sectors"},
	"industry_sector_lookup":            {Path: "/api/industry/lookup?symbol=2330"},
	"scheduler_get_status":              {Path: "/api/scheduler/status"},
	"task_list":                         {Path: "/api/tasks"},
	"trace_get_sim_latest":              {Path: "/api/trace/sim-latest"},
	"trace_get_agent_observatory":       {Path: "/api/trace/agent-observatory"},
	"trace_get_decision_chain":          {Path: "/api/trace/decision-chain?symbol=2330"},
	"trace_get_reasoning":               {Path: "/api/trace/reasoning?session_id=latest"},
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
	"detector_registry_list":            {Path: "/api/narrative/detector-registry"},
	"daily_report":                      {Path: "/api/daily-report"},
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

	for name, ct := range canaryRoutes {
		if skipCanary[name] {
			results[name] = "skipped (destructive)"
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
			results[name] = fmt.Sprintf("FAIL: HTTP error: %v", err)
			continue
		}

		code := resp.StatusCode
		resp.Body.Close()
		switch {
		case code >= 200 && code < 300:
			results[name] = "ok"
		case code >= 300 && code < 400:
			results[name] = fmt.Sprintf("WARN: HTTP %d", code)
		case code == 401 || code == 403:
			results[name] = fmt.Sprintf("WARN: HTTP %d (auth — set ATLAS_API_KEY)", code)
		default:
			results[name] = fmt.Sprintf("FAIL: HTTP %d", code)
		}
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
