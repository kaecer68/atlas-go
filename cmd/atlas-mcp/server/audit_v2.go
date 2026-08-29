package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// AuditEntryV2 is the normalized read model for audit log entries. It supports
// both v1 (duration_ms) and v2 (latency_ms) on-disk schemas and is used by the
// in-memory aggregation tools (mcp_get_call_stats / mcp_get_session_topology).
type AuditEntryV2 struct {
	SchemaVersion int    `json:"schema_version,omitempty"`
	TS            string `json:"ts"`
	SessionID     string `json:"session_id,omitempty"`
	TenantID      string `json:"tenant_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	Tool          string `json:"tool"`
	ArgsHash      string `json:"args_hash,omitempty"`
	Status        string `json:"status"`
	LatencyMS     int64  `json:"latency_ms"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	Transport     string `json:"transport,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ParseAuditEntry detects v1/v2 schema from a single JSONL line and returns a
// normalized AuditEntryV2. v1 entries (no schema_version) are backfilled with
// SchemaVersion=1 and LatencyMS is derived from DurationMS when absent.
func ParseAuditEntry(line []byte) (AuditEntryV2, error) {
	var e AuditEntryV2
	if err := json.Unmarshal(line, &e); err != nil {
		return AuditEntryV2{}, fmt.Errorf("parse audit entry: %w", err)
	}
	if e.SchemaVersion == 0 {
		e.SchemaVersion = 1
	}
	if e.LatencyMS == 0 && e.DurationMS != 0 {
		e.LatencyMS = e.DurationMS
	}
	return e, nil
}

// HashArgs returns the SHA-256 hex digest of raw argument bytes. The caller
// decides the canonical serialization; this helper only guarantees a 64-char
// lowercase hex string.
func HashArgs(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// CallStats aggregates call counts, error counts and p50 latency per tool over
// a recent time window.
type CallStats struct {
	Window       time.Duration            `json:"window"`
	TotalCalls   int                      `json:"total_calls"`
	ErrorCount   int                      `json:"error_count"`
	P50LatencyMS float64                  `json:"p50_latency_ms"`
	PerTool      map[string]ToolCallStats `json:"per_tool"`
}

// ToolCallStats is the per-tool slice of CallStats.
type ToolCallStats struct {
	Count        int     `json:"count"`
	ErrorCount   int     `json:"error_count"`
	P50LatencyMS float64 `json:"p50_latency_ms"`
}

// AggregateCallStats aggregates entries within [now-window, now]. Entries
// without a parseable timestamp are included (conservative: better to count an
// entry than drop it silently). The window is caller-defined; callers that need
// the 30-day retention ceiling enforce it before invoking this function.
func AggregateCallStats(entries []AuditEntryV2, window time.Duration, now time.Time) CallStats {
	stats := CallStats{
		Window:  window,
		PerTool: make(map[string]ToolCallStats),
	}
	cutoff := now.Add(-window)

	var allLatencies []int64
	toolLatencies := make(map[string][]int64)

	for _, e := range entries {
		ts := parseAuditTS(e.TS)
		if !ts.IsZero() && ts.Before(cutoff) {
			continue
		}

		stats.TotalCalls++
		if e.Status != "ok" {
			stats.ErrorCount++
		}

		lat := e.LatencyMS
		if lat == 0 && e.DurationMS != 0 {
			lat = e.DurationMS
		}
		allLatencies = append(allLatencies, lat)
		toolLatencies[e.Tool] = append(toolLatencies[e.Tool], lat)

		tcs := stats.PerTool[e.Tool]
		tcs.Count++
		if e.Status != "ok" {
			tcs.ErrorCount++
		}
		stats.PerTool[e.Tool] = tcs
	}

	stats.P50LatencyMS = percentile50(allLatencies)
	for tool, lats := range toolLatencies {
		tcs := stats.PerTool[tool]
		tcs.P50LatencyMS = percentile50(lats)
		stats.PerTool[tool] = tcs
	}
	return stats
}

// SessionTopology is the agent_id ↔ tool call matrix for behavior audit.
type SessionTopology struct {
	Window     time.Duration             `json:"window"`
	AgentCount int                       `json:"agent_count"`
	ToolCount  int                       `json:"tool_count"`
	Matrix     map[string]map[string]int `json:"matrix"` // matrix[agent_id][tool] = count
}

// BuildSessionTopology aggregates entries into an agent_id × tool matrix for
// the requested window. Empty agent_id falls back to "anonymous" so the matrix
// never has a panic-causing empty key.
func BuildSessionTopology(entries []AuditEntryV2, window time.Duration, now time.Time) SessionTopology {
	topo := SessionTopology{
		Window: window,
		Matrix: make(map[string]map[string]int),
	}
	cutoff := now.Add(-window)
	tools := make(map[string]struct{})

	for _, e := range entries {
		ts := parseAuditTS(e.TS)
		if !ts.IsZero() && ts.Before(cutoff) {
			continue
		}

		agentID := e.AgentID
		if agentID == "" {
			agentID = "anonymous"
		}
		if topo.Matrix[agentID] == nil {
			topo.Matrix[agentID] = make(map[string]int)
		}
		topo.Matrix[agentID][e.Tool]++
		tools[e.Tool] = struct{}{}
	}

	topo.AgentCount = len(topo.Matrix)
	topo.ToolCount = len(tools)
	return topo
}

// parseAuditTS parses RFC3339Nano or RFC3339 timestamps used by audit entries.
func parseAuditTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// percentile50 returns the 50th percentile (median) of latencies in
// milliseconds. Returns 0 for an empty slice.
func percentile50(latencies []int64) float64 {
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]int64, len(latencies))
	copy(sorted, latencies)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2.0
}
