package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewV2Entry_ComputesArgsHash(t *testing.T) {
	entry := NewV2Entry("test-agent", "regime_get_history", []string{"days", "threshold"}, 42)

	if entry.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", entry.SchemaVersion)
	}
	if entry.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want test-agent", entry.AgentID)
	}
	if entry.Transport != "stdio" {
		t.Errorf("Transport = %q, want stdio", entry.Transport)
	}
	if entry.ArgsHash == "" {
		t.Error("ArgsHash should not be empty when argKeys provided")
	}
	if entry.LatencyMS != 42 {
		t.Errorf("LatencyMS = %d, want 42", entry.LatencyMS)
	}
	if entry.Status != "ok" {
		t.Errorf("Status = %q, want ok", entry.Status)
	}
}

func TestNewV2Entry_EmptyArgKeys(t *testing.T) {
	entry := NewV2Entry("agent-1", "tool-x", nil, 0)

	if entry.ArgsHash != "" {
		t.Errorf("ArgsHash = %q, want empty for nil argKeys", entry.ArgsHash)
	}
}

func TestAuditV2Schema_WritesSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.log")
	w, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer w.Close()

	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	entry := AuditEntry{
		Tool:       "regime_get_history",
		Status:     "ok",
		DurationMS: 42,
		AgentID:    "claude-agent-1",
		TenantID:   "tenant-a",
	}
	if wErr := w.Write(entry); wErr != nil {
		t.Fatalf("write: %v", wErr)
	}

	raw, _ := os.ReadFile(path)
	line := strings.TrimRight(string(raw), "\n")

	var decoded map[string]any
	if jErr := json.Unmarshal([]byte(line), &decoded); jErr != nil {
		t.Fatalf("decode: %v (%s)", jErr, line)
	}

	if sv, ok := decoded["schema_version"]; !ok {
		t.Fatal("expected schema_version field, not found")
	} else if sv != float64(2) {
		t.Fatalf("expected schema_version=2, got %v", sv)
	}
	if v, ok := decoded["args_hash"]; ok && v != "" {
		t.Fatalf("expected empty or absent args_hash when no argKeys, got %v", v)
	}
	if v, ok := decoded["transport"]; !ok {
		t.Fatal("expected transport field")
	} else if v != "stdio" {
		t.Fatalf("expected transport=stdio, got %v", v)
	}
	if v, ok := decoded["agent_id"]; !ok || v != "claude-agent-1" {
		t.Fatalf("expected agent_id=claude-agent-1, got %v", v)
	}
}

func TestAuditV2Schema_ArgsHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hash.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	w.now = func() time.Time { return time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC) }

	entry := AuditEntry{
		Tool:       "experiment_judge",
		Status:     "ok",
		DurationMS: 10,
		ArgKeys:    []string{"experiment_id", "threshold"},
	}
	if wErr := w.Write(entry); wErr != nil {
		t.Fatalf("write: %v", wErr)
	}

	raw, _ := os.ReadFile(path)
	var decoded map[string]any
	json.Unmarshal([]byte(strings.TrimRight(string(raw), "\n")), &decoded)

	hash, ok := decoded["args_hash"].(string)
	if !ok || len(hash) != 64 {
		t.Fatalf("expected 64-char hex sha256, got %q", hash)
	}
	if hash != "423d46c7ee84988cf94aaff1a466f80d540b138ad3028cbd709c38d13f50d255" {
		t.Fatalf("expected deterministic hash, got %q", hash)
	}
}

func TestCanonicalizeArgsHash_Empty(t *testing.T) {
	if h := CanonicalizeArgsHash(nil); h != "" {
		t.Fatalf("expected empty hash for nil input, got %q", h)
	}
	if h := CanonicalizeArgsHash([]string{}); h != "" {
		t.Fatalf("expected empty hash for empty slice, got %q", h)
	}
}

func TestCanonicalizeArgsHash_Deterministic(t *testing.T) {
	a := CanonicalizeArgsHash([]string{"a", "b", "c"})
	b := CanonicalizeArgsHash([]string{"a", "b", "c"})
	if a != b || a == "" {
		t.Fatalf("expected same non-empty hash, got a=%q b=%q", a, b)
	}
}

func TestAuditV2Schema_BackwardCompatibleRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.log")

	v1JSON := `{"ts":"2026-06-30T00:00:00Z","tool":"strategy_list_active","status":"ok","duration_ms":15,"tenant_id":"old-tenant","agent_id":"old-agent"}` + "\n"
	if err := os.WriteFile(path, []byte(v1JSON), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, rErr := ReadAuditEntries(path, 0, time.Now())
	if rErr != nil {
		t.Fatalf("read: %v", rErr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Tool != "strategy_list_active" {
		t.Fatalf("tool mismatch: %q", e.Tool)
	}
	if e.SchemaVersion != 1 {
		t.Fatalf("expected SchemaVersion=1 for v1 entry, got %d", e.SchemaVersion)
	}
	if e.ArgsHash != "" {
		t.Fatalf("expected empty args_hash for v1 backfill, got %q", e.ArgsHash)
	}
}

func TestReadAuditEntries_EmptyFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create: %v", err)
	}
	entries, err := ReadAuditEntries(path, 0, time.Now())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadAuditEntries_FileNotFoundReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.log")
	entries, err := ReadAuditEntries(path, 0, time.Now())
	if err == nil {
		t.Fatalf("expected error, got entries=%v", entries)
	}
	if !strings.Contains(err.Error(), "audit") || !strings.Contains(err.Error(), "open") {
		t.Fatalf("expected audit open error, got: %v", err)
	}
}

func TestReadAuditEntries_RetentionFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retention.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	w.now = func() time.Time { return now.Add(-31 * 24 * time.Hour) }
	w.Write(AuditEntry{Tool: "old", Status: "ok", DurationMS: 1})
	w.now = func() time.Time { return now }
	w.Write(AuditEntry{Tool: "new", Status: "ok", DurationMS: 1})

	entries, err := ReadAuditEntries(path, 30, now)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Tool != "new" {
		t.Fatalf("expected new, got %q", entries[0].Tool)
	}
}

func TestReadAuditEntries_NoRetentionWhenZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noret.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	w.now = func() time.Time { return now.Add(-60 * 24 * time.Hour) }
	w.Write(AuditEntry{Tool: "old", Status: "ok", DurationMS: 1})
	w.now = func() time.Time { return now }
	w.Write(AuditEntry{Tool: "new", Status: "ok", DurationMS: 1})

	entries, err := ReadAuditEntries(path, 0, now)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestAuditV2Schema_ExistingV1TestsStillPass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compat.log")
	w, _ := NewAuditWriter(path)
	defer w.Close()

	for i := 0; i < 3; i++ {
		if wErr := w.Write(AuditEntry{Tool: "t", Status: "ok", DurationMS: int64(i)}); wErr != nil {
			t.Fatalf("write %d: %v", i, wErr)
		}
	}

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var e AuditEntry
		if jErr := json.Unmarshal([]byte(line), &e); jErr != nil {
			t.Fatalf("line %d: %v", i, jErr)
		}
		if e.Tool != "t" || e.Status != "ok" {
			t.Fatalf("line %d: unexpected %+v", i, e)
		}
		if e.SchemaVersion != 2 {
			t.Fatalf("line %d: expected schema_version=2, got %d", i, e.SchemaVersion)
		}
	}
}
