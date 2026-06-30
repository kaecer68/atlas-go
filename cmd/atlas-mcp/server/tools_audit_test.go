package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPGetCallStats_ReturnsAggregatedMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	_ = w.Write(AuditEntry{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), Tool: "t1", Status: "ok", DurationMS: 10})
	_ = w.Write(AuditEntry{TS: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), Tool: "t1", Status: "ok", DurationMS: 20})
	_ = w.Write(AuditEntry{TS: now.Add(-3 * time.Minute).Format(time.RFC3339Nano), Tool: "t2", Status: "error", DurationMS: 30})
	_ = w.Close()

	s := &server{audit: w}
	_, stats, err := s.handleMCPGetCallStats(context.Background(), nil, CallStatsInput{WindowMinutes: 60})
	if err != nil {
		t.Fatalf("handleMCPGetCallStats: %v", err)
	}
	if stats.TotalCalls != 3 {
		t.Errorf("total_calls = %d, want 3", stats.TotalCalls)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("error_count = %d, want 1", stats.ErrorCount)
	}
	if stats.PerTool["t1"].Count != 2 {
		t.Errorf("per_tool[t1].count = %d, want 2", stats.PerTool["t1"].Count)
	}
	if stats.PerTool["t2"].Count != 1 {
		t.Errorf("per_tool[t2].count = %d, want 1", stats.PerTool["t2"].Count)
	}
}

func TestMCPGetSessionTopology_ReturnsAgentToolMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	_ = w.Write(AuditEntry{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), AgentID: "a1", Tool: "t1", Status: "ok", DurationMS: 10})
	_ = w.Write(AuditEntry{TS: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), AgentID: "a1", Tool: "t2", Status: "ok", DurationMS: 20})
	_ = w.Write(AuditEntry{TS: now.Add(-3 * time.Minute).Format(time.RFC3339Nano), AgentID: "a2", Tool: "t1", Status: "ok", DurationMS: 30})
	_ = w.Write(AuditEntry{TS: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), AgentID: "a2", Tool: "t2", Status: "ok", DurationMS: 40})
	_ = w.Close()

	s := &server{audit: w}
	_, topo, err := s.handleMCPGetSessionTopology(context.Background(), nil, TopologyInput{WindowMinutes: 60})
	if err != nil {
		t.Fatalf("handleMCPGetSessionTopology: %v", err)
	}
	if topo.AgentCount != 2 {
		t.Errorf("agent_count = %d, want 2", topo.AgentCount)
	}
	if topo.ToolCount != 2 {
		t.Errorf("tool_count = %d, want 2", topo.ToolCount)
	}
	if topo.Matrix["a1"]["t1"] != 1 {
		t.Errorf("matrix[a1][t1] = %d, want 1", topo.Matrix["a1"]["t1"])
	}
	if topo.Matrix["a1"]["t2"] != 1 {
		t.Errorf("matrix[a1][t2] = %d, want 1", topo.Matrix["a1"]["t2"])
	}
	if topo.Matrix["a2"]["t1"] != 1 {
		t.Errorf("matrix[a2][t1] = %d, want 1", topo.Matrix["a2"]["t1"])
	}
	if topo.Matrix["a2"]["t2"] != 1 {
		t.Errorf("matrix[a2][t2] = %d, want 1", topo.Matrix["a2"]["t2"])
	}
}
