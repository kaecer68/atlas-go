package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxAuditWindow matches the default audit retention period (30 days). Tools
// reject queries beyond this to prevent unbounded JSONL parsing.
const maxAuditWindow = 30 * 24 * time.Hour

// registerAuditTools registers the behavior-analysis tools. Known limitation:
// because the stdio transport does not yet inject agent_id into the context,
// AgentIDFromContext currently returns "anonymous" for stdio callers. The
// fallback is intentional (no panic) and will be removed once SSE/streamable-
// HTTP transports wire authenticated agent identity.
func registerAuditTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_call_stats",
		Description: autoDescOr("mcp_get_call_stats", "Return call statistics for the recent window: total calls, error count, p50 latency, and per-tool breakdown. window_minutes defaults to 60 and is capped at 1440 (24h)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetCallStats)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_session_topology",
		Description: autoDescOr("mcp_get_session_topology", "Return the agent_id to tool call matrix for the recent window. window_minutes defaults to 60 and is capped at 1440 (24h). Empty agent_id falls back to 'anonymous'."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetSessionTopology)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_top_slow_tools",
		Description: autoDescOr("mcp_get_top_slow_tools", "Return the slowest tools by p50 latency for the recent window. window_minutes defaults to 60 and is capped at 1440 (24h). limit defaults to 5 and is capped at 20."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetTopSlowTools)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_tenant_usage",
		Description: autoDescOr("mcp_get_tenant_usage", "Return per-tenant usage stats for the recent window: total calls, error count, tool count. window_minutes defaults to 60 and is capped at 1440 (24h)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetTenantUsage)
}

// CallStatsInput is the request schema for mcp_get_call_stats.
type CallStatsInput struct {
	WindowMinutes *int `json:"window_minutes,omitempty" jsonschema:"query window in minutes; default 60, max 1440"`
}

// TopologyInput is the request schema for mcp_get_session_topology.
type TopologyInput struct {
	WindowMinutes *int `json:"window_minutes,omitempty" jsonschema:"query window in minutes; default 60, max 1440"`
}

type TopSlowToolsInput struct {
	Limit         *int `json:"limit,omitempty" jsonschema:"number of slowest tools to return; default 5, max 20"`
	WindowMinutes *int `json:"window_minutes,omitempty" jsonschema:"query window in minutes; default 60, max 1440"`
}

type ToolLatencyStats struct {
	Tool         string  `json:"tool"`
	P50LatencyMS float64 `json:"p50_latency_ms"`
	Count        int     `json:"count"`
	ErrorCount   int     `json:"error_count"`
}

type TopSlowToolsOutput struct {
	Window time.Duration      `json:"window"`
	Tools  []ToolLatencyStats `json:"tools"`
}

type TenantUsageInput struct {
	WindowMinutes *int `json:"window_minutes,omitempty" jsonschema:"query window in minutes; default 60, max 1440"`
}

type TenantUsageStats struct {
	TenantID   string `json:"tenant_id"`
	TotalCalls int    `json:"total_calls"`
	ErrorCount int    `json:"error_count"`
	ToolCount  int    `json:"tool_count"`
}

type TenantUsageOutput struct {
	Window  time.Duration      `json:"window"`
	Tenants []TenantUsageStats `json:"tenants"`
}

func (s *server) handleMCPGetCallStats(ctx context.Context, _ *mcp.CallToolRequest, in CallStatsInput) (*mcp.CallToolResult, CallStats, error) {
	window := 60 * time.Minute
	if in.WindowMinutes != nil && *in.WindowMinutes > 0 {
		window = time.Duration(*in.WindowMinutes) * time.Minute
	}
	if window > maxAuditWindow {
		return nil, CallStats{}, fmt.Errorf("mcp_get_call_stats: window too large")
	}

	entries, err := readAuditEntriesV2(s.audit)
	if err != nil {
		return nil, CallStats{}, fmt.Errorf("mcp_get_call_stats: %w", err)
	}

	stats := AggregateCallStats(entries, window, time.Now())
	return nil, stats, nil
}

func (s *server) handleMCPGetSessionTopology(ctx context.Context, _ *mcp.CallToolRequest, in TopologyInput) (*mcp.CallToolResult, SessionTopology, error) {
	window := 60 * time.Minute
	if in.WindowMinutes != nil && *in.WindowMinutes > 0 {
		window = time.Duration(*in.WindowMinutes) * time.Minute
	}
	if window > maxAuditWindow {
		return nil, SessionTopology{}, fmt.Errorf("mcp_get_session_topology: window too large")
	}

	entries, err := readAuditEntriesV2(s.audit)
	if err != nil {
		return nil, SessionTopology{}, fmt.Errorf("mcp_get_session_topology: %w", err)
	}

	topo := BuildSessionTopology(entries, window, time.Now())
	return nil, topo, nil
}

func (s *server) handleMCPGetTopSlowTools(ctx context.Context, _ *mcp.CallToolRequest, in TopSlowToolsInput) (*mcp.CallToolResult, TopSlowToolsOutput, error) {
	window := 60 * time.Minute
	if in.WindowMinutes != nil && *in.WindowMinutes > 0 {
		window = time.Duration(*in.WindowMinutes) * time.Minute
	}
	if window > maxAuditWindow {
		return nil, TopSlowToolsOutput{}, fmt.Errorf("mcp_get_top_slow_tools: window too large")
	}
	limit := 5
	if in.Limit != nil && *in.Limit > 0 {
		limit = *in.Limit
	}
	if limit > 20 {
		limit = 20
	}

	entries, err := readAuditEntriesV2(s.audit)
	if err != nil {
		return nil, TopSlowToolsOutput{}, fmt.Errorf("mcp_get_top_slow_tools: %w", err)
	}

	stats := AggregateCallStats(entries, window, time.Now())

	tools := make([]ToolLatencyStats, 0, len(stats.PerTool))
	for tool, tcs := range stats.PerTool {
		tools = append(tools, ToolLatencyStats{
			Tool:         tool,
			P50LatencyMS: tcs.P50LatencyMS,
			Count:        tcs.Count,
			ErrorCount:   tcs.ErrorCount,
		})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].P50LatencyMS > tools[j].P50LatencyMS
	})
	if len(tools) > limit {
		tools = tools[:limit]
	}

	return nil, TopSlowToolsOutput{Window: window, Tools: tools}, nil
}

func (s *server) handleMCPGetTenantUsage(ctx context.Context, _ *mcp.CallToolRequest, in TenantUsageInput) (*mcp.CallToolResult, TenantUsageOutput, error) {
	window := 60 * time.Minute
	if in.WindowMinutes != nil && *in.WindowMinutes > 0 {
		window = time.Duration(*in.WindowMinutes) * time.Minute
	}
	if window > maxAuditWindow {
		return nil, TenantUsageOutput{}, fmt.Errorf("mcp_get_tenant_usage: window too large")
	}

	entries, err := readAuditEntriesV2(s.audit)
	if err != nil {
		return nil, TenantUsageOutput{}, fmt.Errorf("mcp_get_tenant_usage: %w", err)
	}

	cutoff := time.Now().Add(-window)
	tenantMap := make(map[string]*TenantUsageStats)
	for _, e := range entries {
		ts := parseAuditTS(e.TS)
		if !ts.IsZero() && ts.Before(cutoff) {
			continue
		}
		tid := e.TenantID
		if tid == "" {
			tid = "anonymous"
		}
		tus, ok := tenantMap[tid]
		if !ok {
			tus = &TenantUsageStats{TenantID: tid}
			tenantMap[tid] = tus
		}
		tus.TotalCalls++
		if e.Status != "ok" {
			tus.ErrorCount++
		}
		if e.Tool != "" {
			tus.ToolCount = countDistinctTools(entries, tid, cutoff)
		}
	}

	tenants := make([]TenantUsageStats, 0, len(tenantMap))
	for _, tus := range tenantMap {
		tenants = append(tenants, *tus)
	}
	sort.Slice(tenants, func(i, j int) bool {
		if tenants[i].ErrorCount != tenants[j].ErrorCount {
			return tenants[i].ErrorCount > tenants[j].ErrorCount
		}
		return tenants[i].TotalCalls > tenants[j].TotalCalls
	})

	return nil, TenantUsageOutput{Window: window, Tenants: tenants}, nil
}

func countDistinctTools(entries []AuditEntryV2, tenantID string, cutoff time.Time) int {
	seen := make(map[string]struct{})
	for _, e := range entries {
		ts := parseAuditTS(e.TS)
		if !ts.IsZero() && ts.Before(cutoff) {
			continue
		}
		tid := e.TenantID
		if tid == "" {
			tid = "anonymous"
		}
		if tid != tenantID {
			continue
		}
		if e.Tool != "" {
			seen[e.Tool] = struct{}{}
		}
	}
	return len(seen)
}

// readAuditEntriesV2 reads the audit log at path and returns parsed AuditEntryV2
// values. Malformed lines are skipped (audit log may contain injected test
// lines). A missing file returns an empty slice and no error so the tools
// return zero-count results instead of failing.
// The caller must hold w.mu (read lock) for the duration of the call to
// serialize against concurrent Write and Cleanup operations.
func readAuditEntriesV2(w *AuditWriter) ([]AuditEntryV2, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.Open(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()

	var entries []AuditEntryV2
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		e, pErr := ParseAuditEntry(scanner.Bytes())
		if pErr != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit log: %w", err)
	}
	return entries, nil
}
