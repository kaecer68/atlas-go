package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
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
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_call_stats",
		Description: "Return call statistics for the recent window: total calls, error count, p50 latency, and per-tool breakdown. window_minutes defaults to 60 and is capped at 1440 (24h).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetCallStats)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_session_topology",
		Description: "Return the agent_id to tool call matrix for the recent window. window_minutes defaults to 60 and is capped at 1440 (24h). Empty agent_id falls back to 'anonymous'.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetSessionTopology)
}

// CallStatsInput is the request schema for mcp_get_call_stats.
type CallStatsInput struct {
	WindowMinutes int `json:"window_minutes" jsonschema:"query window in minutes; default 60, max 1440"`
}

// TopologyInput is the request schema for mcp_get_session_topology.
type TopologyInput struct {
	WindowMinutes int `json:"window_minutes" jsonschema:"query window in minutes; default 60, max 1440"`
}

func (s *server) handleMCPGetCallStats(ctx context.Context, _ *mcp.CallToolRequest, in CallStatsInput) (*mcp.CallToolResult, CallStats, error) {
	window := time.Duration(in.WindowMinutes) * time.Minute
	if window <= 0 {
		window = 60 * time.Minute
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
	window := time.Duration(in.WindowMinutes) * time.Minute
	if window <= 0 {
		window = 60 * time.Minute
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

// readAuditEntriesV2 reads the audit log and returns parsed AuditEntryV2
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
