package server

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAuditV2_ParseV1Backwards(t *testing.T) {
	line := []byte(`{"ts":"2026-07-01T09:00:00.000Z","tool":"regime_get_history","status":"ok","duration_ms":42}`)

	e, err := ParseAuditEntry(line)
	if err != nil {
		t.Fatalf("ParseAuditEntry: %v", err)
	}
	if e.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", e.SchemaVersion)
	}
	if e.AgentID != "" {
		t.Errorf("agent_id = %q, want empty for v1 backward compatibility", e.AgentID)
	}
	if e.LatencyMS != 42 {
		t.Errorf("latency_ms = %d, want 42 (mapped from v1 duration_ms)", e.LatencyMS)
	}
}

func TestAuditV2_ParseV2AllFields(t *testing.T) {
	argsHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	line := []byte(`{"schema_version":2,"ts":"2026-07-01T09:00:00.000Z","session_id":"sess-1","agent_id":"agent-1","tool":"regime_get_history","args_hash":"` + argsHash + `","status":"ok","latency_ms":42,"transport":"stdio"}`)

	e, err := ParseAuditEntry(line)
	if err != nil {
		t.Fatalf("ParseAuditEntry: %v", err)
	}
	if e.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", e.SchemaVersion)
	}
	if e.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", e.SessionID)
	}
	if e.AgentID != "agent-1" {
		t.Errorf("agent_id = %q, want agent-1", e.AgentID)
	}
	if e.Tool != "regime_get_history" {
		t.Errorf("tool = %q, want regime_get_history", e.Tool)
	}
	if e.ArgsHash != argsHash {
		t.Errorf("args_hash = %q, want %q", e.ArgsHash, argsHash)
	}
	if e.LatencyMS != 42 {
		t.Errorf("latency_ms = %d, want 42", e.LatencyMS)
	}
	if e.Transport != "stdio" {
		t.Errorf("transport = %q, want stdio", e.Transport)
	}
}

func TestAuditV2_AggregateCallStats(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	entries := []AuditEntryV2{
		{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), Tool: "t1", Status: "ok", LatencyMS: 10},
		{TS: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), Tool: "t1", Status: "ok", LatencyMS: 20},
		{TS: now.Add(-3 * time.Minute).Format(time.RFC3339Nano), Tool: "t2", Status: "ok", LatencyMS: 30},
		{TS: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), Tool: "t2", Status: "error", LatencyMS: 40},
		{TS: now.Add(-1 * time.Minute).Format(time.RFC3339Nano), Tool: "t1", Status: "error", LatencyMS: 50},
	}

	stats := AggregateCallStats(entries, 60*time.Minute, now)

	if stats.TotalCalls != 5 {
		t.Errorf("total_calls = %d, want 5", stats.TotalCalls)
	}
	if stats.ErrorCount != 2 {
		t.Errorf("error_count = %d, want 2", stats.ErrorCount)
	}
	if stats.P50LatencyMS != 30 {
		t.Errorf("p50_latency_ms = %v, want 30", stats.P50LatencyMS)
	}
	if stats.PerTool["t1"].Count != 3 {
		t.Errorf("per_tool[t1].count = %d, want 3", stats.PerTool["t1"].Count)
	}
	if stats.PerTool["t1"].ErrorCount != 1 {
		t.Errorf("per_tool[t1].error_count = %d, want 1", stats.PerTool["t1"].ErrorCount)
	}
	if stats.PerTool["t1"].P50LatencyMS != 20 {
		t.Errorf("per_tool[t1].p50_latency_ms = %v, want 20", stats.PerTool["t1"].P50LatencyMS)
	}
	if stats.PerTool["t2"].Count != 2 {
		t.Errorf("per_tool[t2].count = %d, want 2", stats.PerTool["t2"].Count)
	}
	if stats.PerTool["t2"].ErrorCount != 1 {
		t.Errorf("per_tool[t2].error_count = %d, want 1", stats.PerTool["t2"].ErrorCount)
	}
	if stats.PerTool["t2"].P50LatencyMS != 35 {
		t.Errorf("per_tool[t2].p50_latency_ms = %v, want 35", stats.PerTool["t2"].P50LatencyMS)
	}
}

func TestAuditV2_SessionTopology(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	entries := []AuditEntryV2{
		{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), AgentID: "a1", Tool: "t1", Status: "ok", LatencyMS: 10},
		{TS: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), AgentID: "a1", Tool: "t2", Status: "ok", LatencyMS: 20},
		{TS: now.Add(-3 * time.Minute).Format(time.RFC3339Nano), AgentID: "a2", Tool: "t1", Status: "ok", LatencyMS: 30},
		{TS: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), AgentID: "a2", Tool: "t2", Status: "ok", LatencyMS: 40},
	}

	topo := BuildSessionTopology(entries, 60*time.Minute, now)

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

func TestAuditV2_30DayRetentionFilter(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	entries := []AuditEntryV2{
		{TS: now.Add(-35 * 24 * time.Hour).Format(time.RFC3339Nano), Tool: "old", Status: "ok", LatencyMS: 1},
		{TS: now.Add(-1 * 24 * time.Hour).Format(time.RFC3339Nano), Tool: "new", Status: "ok", LatencyMS: 2},
	}

	stats := AggregateCallStats(entries, 30*24*time.Hour, now)

	if stats.TotalCalls != 1 {
		t.Errorf("total_calls = %d, want 1 (35-day entry excluded)", stats.TotalCalls)
	}
	if stats.PerTool["old"].Count != 0 {
		t.Errorf("per_tool[old].count = %d, want 0", stats.PerTool["old"].Count)
	}
	if stats.PerTool["new"].Count != 1 {
		t.Errorf("per_tool[new].count = %d, want 1", stats.PerTool["new"].Count)
	}
}

func TestAuditV2_V1V2Interop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interop.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	now := time.Now().UTC()
	// Write a v1-style entry (no schema_version, duration_ms only).
	_ = w.Write(AuditEntry{
		TS:         now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		Tool:       "v1_tool",
		Status:     "ok",
		DurationMS: 15,
	})
	// Write a v2-style entry (schema_version explicitly set, latency_ms).
	_ = w.Write(AuditEntry{
		SchemaVersion: 2,
		TS:            now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
		SessionID:     "sess-interop",
		AgentID:       "agent-interop",
		Tool:          "v2_tool",
		ArgsHash:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Status:        "ok",
		LatencyMS:     25,
		Transport:     "stdio",
	})

	entries, err := readAuditEntriesV2(w)
	if err != nil {
		t.Fatalf("readAuditEntriesV2: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	stats := AggregateCallStats(entries, 60*time.Minute, now)
	if stats.TotalCalls != 2 {
		t.Errorf("total_calls = %d, want 2", stats.TotalCalls)
	}
	if stats.PerTool["v1_tool"].Count != 1 {
		t.Errorf("per_tool[v1_tool].count = %d, want 1", stats.PerTool["v1_tool"].Count)
	}
	if stats.PerTool["v2_tool"].Count != 1 {
		t.Errorf("per_tool[v2_tool].count = %d, want 1", stats.PerTool["v2_tool"].Count)
	}
}
