package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScratchpad_NewScratchpad(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)
	if s == nil {
		t.Fatal("NewScratchpad returned nil")
	}
	if s.sessionID != "session-20250101-daily" {
		t.Errorf("sessionID = %q, want session-20250101-daily", s.sessionID)
	}
	tracesDir := filepath.Join(dir, "traces")
	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		t.Error("traces directory was not created")
	}
}

func TestScratchpad_Record(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)

	trace := ReasoningTrace{Component: "test-agent", Phase: "screening", IsFallback: false}
	s.Record(trace)

	if got := len(s.Traces()); got != 1 {
		t.Fatalf("Traces() len = %d, want 1", got)
	}
	if got := s.Traces()[0].Component; got != "test-agent" {
		t.Errorf("Component = %q, want test-agent", got)
	}
}

func TestScratchpad_MarkAllAsFallback(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)

	s.Record(ReasoningTrace{Component: "a", IsFallback: false})
	s.Record(ReasoningTrace{Component: "b", IsFallback: false})
	s.Record(ReasoningTrace{Component: "c", IsFallback: false})

	s.MarkAllAsFallback()

	for i, trace := range s.Traces() {
		if !trace.IsFallback {
			t.Errorf("trace %d: IsFallback should be true", i)
		}
	}
}

func TestScratchpad_MarkAllAsFallback_EmptyTraces(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)
	s.MarkAllAsFallback() // no panic on empty
	if got := len(s.Traces()); got != 0 {
		t.Errorf("Traces() len = %d, want 0", got)
	}
}

func TestScratchpad_Traces_ReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)

	s.Record(ReasoningTrace{Component: "original", IsFallback: false})
	copied := s.Traces()
	copied[0].Component = "modified"

	if s.Traces()[0].Component != "original" {
		t.Error("mutating returned slice should not affect internal traces")
	}
}

func TestScratchpad_ExportJSONL(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)

	s.Record(ReasoningTrace{Component: "a", Phase: "screening"})
	s.Record(ReasoningTrace{Component: "b", Phase: "screening"})

	path, err := s.ExportJSONL()
	if err != nil {
		t.Fatalf("ExportJSONL error: %v", err)
	}
	if path == "" {
		t.Fatal("ExportJSONL path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read exported file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("exported file is empty")
	}
}

func TestScratchpad_LoadScratchpad_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadScratchpad("nonexistent", dir)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestScratchpad_LoadScratchpad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	s := NewScratchpad("session-20250101-daily", dir)
	s.Record(ReasoningTrace{Component: "loaded", Phase: "screening", IsFallback: false})

	loaded, err := LoadScratchpad("session-20250101-daily", dir)
	if err != nil {
		t.Fatalf("LoadScratchpad error: %v", err)
	}
	if got := len(loaded.Traces()); got != 1 {
		t.Fatalf("Traces() len = %d, want 1", got)
	}
	if loaded.Traces()[0].Component != "loaded" {
		t.Errorf("Component = %q, want loaded", loaded.Traces()[0].Component)
	}
}

func TestScratchpad_LoadScratchpad_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	os.MkdirAll(tracesDir, 0o755)
	path := filepath.Join(tracesDir, "session-20250101-daily.jsonl")
	// Write one valid line and one invalid line.
	content := []byte("{\"agent_id\":\"valid\"}\nnot json at all\n{\"agent_id\":\"also_valid\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadScratchpad("session-20250101-daily", dir)
	if err != nil {
		t.Fatalf("LoadScratchpad error: %v", err)
	}
	if got := len(loaded.Traces()); got != 2 {
		t.Fatalf("Traces() len = %d, want 2 (malformed line skipped)", got)
	}
}

func TestScratchpad_LoadScratchpad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	os.MkdirAll(tracesDir, 0o755)
	path := filepath.Join(tracesDir, "session-20250101-daily.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadScratchpad("session-20250101-daily", dir)
	if err != nil {
		t.Fatalf("LoadScratchpad error: %v", err)
	}
	if got := len(loaded.Traces()); got != 0 {
		t.Fatalf("Traces() len = %d, want 0", got)
	}
}
