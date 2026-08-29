package server

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
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
	_, stats, err := s.handleMCPGetCallStats(context.Background(), nil, CallStatsInput{WindowMinutes: intPtr(60)})
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
	_, topo, err := s.handleMCPGetSessionTopology(context.Background(), nil, TopologyInput{WindowMinutes: intPtr(60)})
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

func TestMCPGetTopSlowTools_ReturnsSortedByLatency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	_ = w.Write(AuditEntry{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), Tool: "fast_tool", Status: "ok", DurationMS: 5})
	_ = w.Write(AuditEntry{TS: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), Tool: "medium_tool", Status: "ok", DurationMS: 50})
	_ = w.Write(AuditEntry{TS: now.Add(-3 * time.Minute).Format(time.RFC3339Nano), Tool: "slow_tool", Status: "ok", DurationMS: 200})
	_ = w.Write(AuditEntry{TS: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), Tool: "slow_tool", Status: "error", DurationMS: 300})
	_ = w.Close()

	s := &server{audit: w}
	_, out, err := s.handleMCPGetTopSlowTools(context.Background(), nil, TopSlowToolsInput{Limit: intPtr(5), WindowMinutes: intPtr(60)})
	if err != nil {
		t.Fatalf("handleMCPGetTopSlowTools: %v", err)
	}
	if len(out.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(out.Tools))
	}
	if out.Tools[0].Tool != "slow_tool" {
		t.Errorf("top tool = %q, want slow_tool", out.Tools[0].Tool)
	}
	if out.Tools[0].P50LatencyMS != 250 {
		t.Errorf("top tool p50 = %f, want 250", out.Tools[0].P50LatencyMS)
	}
	if out.Tools[0].Count != 2 {
		t.Errorf("top tool count = %d, want 2", out.Tools[0].Count)
	}
	if out.Tools[0].ErrorCount != 1 {
		t.Errorf("top tool error_count = %d, want 1", out.Tools[0].ErrorCount)
	}
	if out.Tools[2].Tool != "fast_tool" {
		t.Errorf("bottom tool = %q, want fast_tool", out.Tools[2].Tool)
	}
}

func TestMCPGetTopSlowTools_RespectsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	for i := range 10 {
		_ = w.Write(AuditEntry{TS: now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano), Tool: fmt.Sprintf("t%d", i), Status: "ok", DurationMS: int64(10 * (i + 1))})
	}
	_ = w.Close()

	s := &server{audit: w}
	_, out, err := s.handleMCPGetTopSlowTools(context.Background(), nil, TopSlowToolsInput{Limit: intPtr(3), WindowMinutes: intPtr(60)})
	if err != nil {
		t.Fatalf("handleMCPGetTopSlowTools: %v", err)
	}
	if len(out.Tools) != 3 {
		t.Errorf("got %d tools, want 3", len(out.Tools))
	}
}

func TestMCPGetTenantUsage_ReturnsTenantStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	_ = w.Write(AuditEntry{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), TenantID: "tenant-a", Tool: "t1", Status: "ok", DurationMS: 10})
	_ = w.Write(AuditEntry{TS: now.Add(-4 * time.Minute).Format(time.RFC3339Nano), TenantID: "tenant-a", Tool: "t2", Status: "ok", DurationMS: 20})
	_ = w.Write(AuditEntry{TS: now.Add(-3 * time.Minute).Format(time.RFC3339Nano), TenantID: "tenant-b", Tool: "t1", Status: "error", DurationMS: 30})
	_ = w.Write(AuditEntry{TS: now.Add(-2 * time.Minute).Format(time.RFC3339Nano), TenantID: "tenant-b", Tool: "t1", Status: "ok", DurationMS: 40})
	_ = w.Close()

	s := &server{audit: w}
	_, out, err := s.handleMCPGetTenantUsage(context.Background(), nil, TenantUsageInput{WindowMinutes: intPtr(60)})
	if err != nil {
		t.Fatalf("handleMCPGetTenantUsage: %v", err)
	}
	if len(out.Tenants) != 2 {
		t.Fatalf("got %d tenants, want 2", len(out.Tenants))
	}
	if out.Tenants[0].TenantID != "tenant-b" {
		t.Errorf("top tenant = %q, want tenant-b", out.Tenants[0].TenantID)
	}
	if out.Tenants[0].TotalCalls != 2 {
		t.Errorf("top tenant total_calls = %d, want 2", out.Tenants[0].TotalCalls)
	}
	if out.Tenants[0].ErrorCount != 1 {
		t.Errorf("top tenant error_count = %d, want 1", out.Tenants[0].ErrorCount)
	}
	if out.Tenants[1].TenantID != "tenant-a" {
		t.Errorf("bottom tenant = %q, want tenant-a", out.Tenants[1].TenantID)
	}
	if out.Tenants[1].TotalCalls != 2 {
		t.Errorf("bottom tenant total_calls = %d, want 2", out.Tenants[1].TotalCalls)
	}
}

func TestMCPGetTenantUsage_AnonymousFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	_ = w.Write(AuditEntry{TS: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), Tool: "t1", Status: "ok", DurationMS: 10})
	_ = w.Close()

	s := &server{audit: w}
	_, out, err := s.handleMCPGetTenantUsage(context.Background(), nil, TenantUsageInput{WindowMinutes: intPtr(60)})
	if err != nil {
		t.Fatalf("handleMCPGetTenantUsage: %v", err)
	}
	if len(out.Tenants) != 1 {
		t.Fatalf("got %d tenants, want 1", len(out.Tenants))
	}
	if out.Tenants[0].TenantID != "anonymous" {
		t.Errorf("tenant = %q, want anonymous", out.Tenants[0].TenantID)
	}
	if out.Tenants[0].TotalCalls != 1 {
		t.Errorf("total_calls = %d, want 1", out.Tenants[0].TotalCalls)
	}
}

func TestMCPGetTenantUsage_EmptyAuditReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	_ = w.Close()

	s := &server{audit: w}
	_, out, err := s.handleMCPGetTenantUsage(context.Background(), nil, TenantUsageInput{WindowMinutes: intPtr(60)})
	if err != nil {
		t.Fatalf("handleMCPGetTenantUsage: %v", err)
	}
	if len(out.Tenants) != 0 {
		t.Errorf("got %d tenants, want 0", len(out.Tenants))
	}
}

func TestReadAuditEntriesV2_ConcurrentSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit_concurrent.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new audit writer: %v", err)
	}
	now := time.Now().UTC()
	for i := range 100 {
		_ = w.Write(AuditEntry{
			TS:         now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
			AgentID:    "agent1",
			Tool:       "tool1",
			Status:     "ok",
			DurationMS: int64(i % 50),
		})
	}
	_ = w.Close()

	s := &server{audit: w}

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := readAuditEntriesV2(s.audit)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent read: %v", err)
	}
}
