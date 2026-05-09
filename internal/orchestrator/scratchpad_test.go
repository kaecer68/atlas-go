package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScratchpad_RecordAndLoad(t *testing.T) {
	dir := t.TempDir()

	trace := ReasoningTrace{
		SessionID:  "session-001",
		Timestamp:  time.Now().UTC(),
		Phase:      PhaseAgentRecommendation,
		Step:       1,
		Component:  "SectorExecutor",
		Action:     "recommend",
		Reasoning:  "Strong momentum in semiconductor sector",
		Confidence: 0.85,
		IsFallback: false,
	}

	sp := NewScratchpad("session-001", dir)
	sp.Record(trace)

	_, err := sp.ExportJSONL()
	if err != nil {
		t.Fatalf("ExportJSONL failed: %v", err)
	}

	loaded, err := LoadScratchpad("session-001", dir)
	if err != nil {
		t.Fatalf("LoadScratchpad failed: %v", err)
	}

	traces := loaded.Traces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	if traces[0].SessionID != trace.SessionID {
		t.Errorf("SessionID mismatch: got %s, want %s", traces[0].SessionID, trace.SessionID)
	}
	if traces[0].Reasoning != trace.Reasoning {
		t.Errorf("Reasoning mismatch: got %s, want %s", traces[0].Reasoning, trace.Reasoning)
	}
	if traces[0].Confidence != trace.Confidence {
		t.Errorf("Confidence mismatch: got %f, want %f", traces[0].Confidence, trace.Confidence)
	}
}

func TestScratchpad_ExportJSONL_MultipleTraces(t *testing.T) {
	dir := t.TempDir()

	sp := NewScratchpad("session-002", dir)

	trace1 := ReasoningTrace{
		SessionID:  "session-002",
		Timestamp:  time.Now().UTC(),
		Phase:      PhaseRegimeDetection,
		Step:       1,
		Component:  "Macro",
		Action:     "detect",
		Reasoning:  "Risk-on environment",
		Confidence: 0.9,
	}
	trace2 := ReasoningTrace{
		SessionID:  "session-002",
		Timestamp:  time.Now().UTC(),
		Phase:      PhaseControlFilter,
		Step:       2,
		Component:  "CIO",
		Action:     "filter",
		Reasoning:  "Applying sector limits",
		Confidence: 0.75,
	}

	sp.Record(trace1)
	sp.Record(trace2)

	filePath, err := sp.ExportJSONL()
	if err != nil {
		t.Fatalf("ExportJSONL failed: %v", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer file.Close()

	lineCount := 0
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var trace ReasoningTrace
		if err := decoder.Decode(&trace); err != nil {
			t.Fatalf("decode line %d: %v", lineCount+1, err)
		}
		lineCount++
	}

	if lineCount != 2 {
		t.Errorf("expected 2 JSONL lines, got %d", lineCount)
	}
}

func TestScratchpad_LoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	os.MkdirAll(tracesDir, 0755)

	filePath := filepath.Join(tracesDir, "session-003.jsonl")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create test file: %v", err)
	}

	validTrace := ReasoningTrace{
		SessionID:  "session-003",
		Timestamp:  time.Now().UTC(),
		Phase:      PhasePortfolioBuild,
		Step:       1,
		Component:  "Portfolio",
		Action:     "build",
		Reasoning:  "Constructing portfolio",
		Confidence: 0.8,
	}
	enc := json.NewEncoder(file)
	enc.Encode(validTrace)

	file.WriteString("this is not valid json\n")

	validTrace2 := ReasoningTrace{
		SessionID:  "session-003",
		Timestamp:  time.Now().UTC(),
		Phase:      PhaseAgentRecommendation,
		Step:       2,
		Component:  "Sector",
		Action:     "recommend",
		Reasoning:  "Adding positions",
		Confidence: 0.7,
	}
	enc.Encode(validTrace2)
	file.Close()

	loaded, err := LoadScratchpad("session-003", dir)
	if err != nil {
		t.Fatalf("LoadScratchpad should not fail on corrupted file: %v", err)
	}

	traces := loaded.Traces()
	if len(traces) != 2 {
		t.Errorf("expected 2 valid traces after skipping bad line, got %d", len(traces))
	}
}

func TestScratchpad_LoadNonExistent(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadScratchpad("non-existent-session", dir)
	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}
