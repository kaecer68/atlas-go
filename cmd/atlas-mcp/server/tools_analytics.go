package server

import (
	"context"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- CallStats -----------------------------------------------------------

type CallStats struct {
	TotalCalls      int                   `json:"total_calls"`
	P50LatencyMS    int64                 `json:"p50_latency_ms"`
	GlobalErrorRate float64               `json:"global_error_rate"`
	PerTool         map[string]*ToolStats `json:"per_tool"`
}

type ToolStats struct {
	Count        int     `json:"count"`
	P50LatencyMS int64   `json:"p50_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

func percentile64(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	fidx := (float64(len(sorted)-1)) * p / 100.0
	lo := int(fidx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[lo]
	}
	frac := fidx - float64(lo)
	val := float64(sorted[lo])*(1-frac) + float64(sorted[hi])*frac
	return int64(val)
}

// AggregateCallStats computes per-tool and global statistics from audit
// entries within the time window (windowMinutes before now). windowMinutes
// <= 0 means no time filtering.
func AggregateCallStats(entries []AuditEntry, windowMinutes int, now time.Time) *CallStats {
	stats := &CallStats{PerTool: make(map[string]*ToolStats)}
	cutoff := now.Add(-time.Duration(windowMinutes) * time.Minute)

	var latencies []int64
	for _, e := range entries {
		if windowMinutes > 0 {
			t, err := time.Parse(time.RFC3339Nano, e.TS)
			if err != nil {
				t, err = time.Parse(time.RFC3339, e.TS)
			}
			if err == nil && t.Before(cutoff) {
				continue
			}
		}
		lat := e.LatencyMS
		if lat == 0 {
			lat = e.DurationMS
		}
		latencies = append(latencies, lat)
		ts, ok := stats.PerTool[e.Tool]
		if !ok {
			ts = &ToolStats{}
			stats.PerTool[e.Tool] = ts
		}
		ts.Count++
		stats.TotalCalls++
	}

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		stats.P50LatencyMS = percentile64(latencies, 50)
	}

	var globalErrCount int
	for tool, ts := range stats.PerTool {
		var toolLats []int64
		var errCount int
		for _, e := range entries {
			if windowMinutes > 0 {
				t, err := time.Parse(time.RFC3339Nano, e.TS)
				if err != nil {
					t, err = time.Parse(time.RFC3339, e.TS)
				}
				if err == nil && t.Before(cutoff) {
					continue
				}
			}
			if e.Tool != tool {
				continue
			}
			lat := e.LatencyMS
			if lat == 0 {
				lat = e.DurationMS
			}
			toolLats = append(toolLats, lat)
			if e.Status == "error" {
				errCount++
				globalErrCount++
			}
		}
		if len(toolLats) > 0 {
			sort.Slice(toolLats, func(i, j int) bool { return toolLats[i] < toolLats[j] })
			ts.P50LatencyMS = percentile64(toolLats, 50)
		}
		if ts.Count > 0 {
			ts.ErrorRate = float64(errCount) / float64(ts.Count) * 100.0
		}
	}
	if stats.TotalCalls > 0 {
		stats.GlobalErrorRate = float64(globalErrCount) / float64(stats.TotalCalls) * 100.0
	}
	return stats
}

// --- TopologyResult ------------------------------------------------------

type EdgeStats struct {
	CallCount    int     `json:"call_count"`
	P50LatencyMS int64   `json:"p50_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

type TopologyResult struct {
	AgentCount int                              `json:"agent_count"`
	ToolCount  int                              `json:"tool_count"`
	Matrix     map[string]map[string]*EdgeStats `json:"matrix"`
}

// BuildSessionTopology builds an agent-to-tool call matrix from audit
// entries within the time window.
func BuildSessionTopology(entries []AuditEntry, windowMinutes int, now time.Time) *TopologyResult {
	topo := &TopologyResult{Matrix: make(map[string]map[string]*EdgeStats)}
	cutoff := now.Add(-time.Duration(windowMinutes) * time.Minute)

	for _, e := range entries {
		if windowMinutes > 0 {
			t, err := time.Parse(time.RFC3339Nano, e.TS)
			if err != nil {
				t, err = time.Parse(time.RFC3339, e.TS)
			}
			if err == nil && t.Before(cutoff) {
				continue
			}
		}
		agentID := e.AgentID
		if agentID == "" {
			agentID = "anonymous"
		}
		if _, ok := topo.Matrix[agentID]; !ok {
			topo.Matrix[agentID] = make(map[string]*EdgeStats)
		}
		if _, ok := topo.Matrix[agentID][e.Tool]; !ok {
			topo.Matrix[agentID][e.Tool] = &EdgeStats{}
		}
		topo.Matrix[agentID][e.Tool].CallCount++
	}

	for agent, tools := range topo.Matrix {
		for tool, edge := range tools {
			var lats []int64
			var errCount int
			for _, e := range entries {
				if windowMinutes > 0 {
					t, err := time.Parse(time.RFC3339Nano, e.TS)
					if err != nil {
						t, err = time.Parse(time.RFC3339, e.TS)
					}
					if err == nil && t.Before(cutoff) {
						continue
					}
				}
				aID := e.AgentID
				if aID == "" {
					aID = "anonymous"
				}
				if aID != agent || e.Tool != tool {
					continue
				}
				lat := e.LatencyMS
				if lat == 0 {
					lat = e.DurationMS
				}
				lats = append(lats, lat)
				if e.Status == "error" {
					errCount++
				}
			}
			if len(lats) > 0 {
				sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
				edge.P50LatencyMS = percentile64(lats, 50)
			}
			if edge.CallCount > 0 {
				edge.ErrorRate = float64(errCount) / float64(edge.CallCount) * 100.0
			}
		}
	}

	agentSet := make(map[string]struct{})
	toolSet := make(map[string]struct{})
	for _, e := range entries {
		if windowMinutes > 0 {
			t, err := time.Parse(time.RFC3339Nano, e.TS)
			if err != nil {
				t, err = time.Parse(time.RFC3339, e.TS)
			}
			if err == nil && t.Before(cutoff) {
				continue
			}
		}
		aID := e.AgentID
		if aID == "" {
			aID = "anonymous"
		}
		agentSet[aID] = struct{}{}
		toolSet[e.Tool] = struct{}{}
	}
	topo.AgentCount = len(agentSet)
	topo.ToolCount = len(toolSet)
	return topo
}

// --- Tool registration + handlers ---------------------------------------

func registerAnalyticsTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_call_stats",
		Description: "Return per-tool and global call statistics (count, p50 latency, error rate) for the last N minutes.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetCallStats)
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_get_session_topology",
		Description: "Return the agent-to-tool call matrix (edges: call_count, p50 latency, error_rate) for the last N minutes.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMCPGetSessionTopology)
}

type CallStatsInput struct {
	WindowMinutes int `json:"window_minutes" jsonschema:"number of minutes to aggregate; 0 = all available data"`
}

type CallStatsOutput struct {
	Stats *CallStats `json:"stats"`
}

func (s *server) handleMCPGetCallStats(ctx context.Context, _ *mcp.CallToolRequest, in CallStatsInput) (*mcp.CallToolResult, CallStatsOutput, error) {
	if in.WindowMinutes < 0 {
		in.WindowMinutes = 0
	}
	entries, err := ReadAuditEntries(s.cfg.AuditLogPath, 0, time.Now())
	if err != nil {
		return nil, CallStatsOutput{}, err
	}
	stats := AggregateCallStats(entries, in.WindowMinutes, time.Now())
	return nil, CallStatsOutput{Stats: stats}, nil
}

type TopologyInput struct {
	WindowMinutes int `json:"window_minutes" jsonschema:"number of minutes to analyze; 0 = all available data"`
}

type TopologyOutput struct {
	Topology *TopologyResult `json:"topology"`
}

func (s *server) handleMCPGetSessionTopology(ctx context.Context, _ *mcp.CallToolRequest, in TopologyInput) (*mcp.CallToolResult, TopologyOutput, error) {
	if in.WindowMinutes < 0 {
		in.WindowMinutes = 0
	}
	entries, err := ReadAuditEntries(s.cfg.AuditLogPath, 0, time.Now())
	if err != nil {
		return nil, TopologyOutput{}, err
	}
	topo := BuildSessionTopology(entries, in.WindowMinutes, time.Now())
	return nil, TopologyOutput{Topology: topo}, nil
}
