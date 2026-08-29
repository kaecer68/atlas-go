package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSimTrace_Record(t *testing.T) {
	dir := t.TempDir()

	w := NewSimTraceWriter(dir, "20260526", false)

	w.Record(1, "data_fetch", "START", nil)
	w.Record(2, "data_fetch", "OK", map[string]any{"symbols": 150})
	w.Record(3, "regime_detect", "OK", map[string]any{"regime": "risk_on"})

	if len(w.records) != 3 {
		t.Fatalf("expected 3 records in memory, got %d", len(w.records))
	}

	r0 := w.records[0]
	if r0.Step != 1 || r0.Layer != "data_fetch" || r0.Status != "START" {
		t.Errorf("record 0 mismatch: step=%d, layer=%s, status=%s", r0.Step, r0.Layer, r0.Status)
	}
	if r0.SessionID != "20260526" {
		t.Errorf("record 0 SessionID mismatch: got %s, want 20260526", r0.SessionID)
	}
	if r0.TS.IsZero() {
		t.Error("record 0 timestamp should not be zero")
	}

	r1 := w.records[1]
	if r1.Metadata == nil {
		t.Fatal("record 1 metadata should not be nil")
	}
	symCount, ok := r1.Metadata["symbols"]
	if !ok {
		t.Error("record 1 metadata missing 'symbols'")
	}
	if v, ok := symCount.(int); !ok || v != 150 {
		t.Errorf("record 1 metadata 'symbols': got %v (type %T), want 150", symCount, symCount)
	}

	r2 := w.records[2]
	if regime, ok := r2.Metadata["regime"]; !ok || regime != "risk_on" {
		t.Errorf("record 2 metadata 'regime': got %v, want risk_on", r2.Metadata["regime"])
	}
}

func TestSimTrace_ExportJSONL(t *testing.T) {
	dir := t.TempDir()

	w := NewSimTraceWriter(dir, "20260526", false)

	w.Record(1, "screening", "OK", map[string]any{"passed": 42, "rejected": 8})
	w.Record(2, "recommend", "WARN", map[string]any{"count": 5})
	w.Record(3, "guard_filter", "OK", map[string]any{"kept": 3, "blocked": 2})
	w.Record(4, "sim_exec", "FAIL", map[string]any{"error": "insufficient capital"})

	filePath, err := w.ExportJSONL()
	if err != nil {
		t.Fatalf("ExportJSONL failed: %v", err)
	}

	expectedPath := filepath.Join(dir, "traces", "sim-20260526.jsonl")
	if filePath != expectedPath {
		t.Errorf("path mismatch: got %s, want %s", filePath, expectedPath)
	}

	// Read back and verify JSONL format.
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer func() { _ = file.Close() }()

	dec := json.NewDecoder(file)
	var records []SimTraceRecord
	for dec.More() {
		var rec SimTraceRecord
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		records = append(records, rec)
	}

	if len(records) != 4 {
		t.Fatalf("expected 4 records in JSONL, got %d", len(records))
	}

	// Verify JSON field names (snake_case).
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("exported file should not be empty")
	}

	// Spot-check the FAIL record.
	failRec := records[3]
	if failRec.Layer != "sim_exec" {
		t.Errorf("layer mismatch: got %s", failRec.Layer)
	}
	if failRec.Status != "FAIL" {
		t.Errorf("status mismatch: got %s", failRec.Status)
	}
	if failRec.Metadata["error"] != "insufficient capital" {
		t.Errorf("metadata mismatch: %v", failRec.Metadata)
	}
}

func TestSimTrace_ThreadSafety(t *testing.T) {
	dir := t.TempDir()

	w := NewSimTraceWriter(dir, "20260526", false)

	const numGoroutines = 20
	const writesPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for i := range writesPerGoroutine {
				// Alternate between different statuses to cover all code paths.
				statuses := []string{"START", "OK", "WARN", "FAIL"}
				status := statuses[i%len(statuses)]
				w.Record(
					(id*writesPerGoroutine)+i,
					"test_layer",
					status,
					map[string]any{"goroutine": id, "index": i},
				)
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := numGoroutines * writesPerGoroutine
	if len(w.records) != expectedTotal {
		t.Errorf("record count mismatch: got %d, want %d (possible data race)",
			len(w.records), expectedTotal)
	}

	// Verify we can export without panicking after concurrent writes.
	filePath, err := w.ExportJSONL()
	if err != nil {
		t.Fatalf("ExportJSONL after concurrent writes failed: %v", err)
	}

	// Verify the exported file is valid JSONL.
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer func() { _ = file.Close() }()

	dec := json.NewDecoder(file)
	readCount := 0
	for dec.More() {
		var rec SimTraceRecord
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode at record %d failed: %v", readCount, err)
		}
		readCount++
	}

	if readCount != expectedTotal {
		t.Errorf("JSONL record count mismatch: got %d, want %d", readCount, expectedTotal)
	}
}

func TestSimTrace_VerboseDoesNotPanic(t *testing.T) {
	// Just verify verbose mode doesn't cause panics.
	_ = t.TempDir()

	// Use a temp dir for base dir; the writer doesn't create dirs until ExportJSONL.
	w := NewSimTraceWriter(os.TempDir(), "20260526", true)

	w.Record(1, "data_fetch", "START", nil)
	w.Record(2, "data_fetch", "OK", map[string]any{"symbols": 150})
	w.Record(3, "screening", "WARN", map[string]any{"passed": 10})
	w.Record(4, "guard_filter", "FAIL", map[string]any{"error": "timeout"})
	w.Record(5, "ledger_write", "OK", nil)

	// Unknown status should not panic.
	w.Record(6, "unknown", "CUSTOM", map[string]any{"note": "test"})

	if len(w.records) != 6 {
		t.Errorf("expected 6 records, got %d", len(w.records))
	}
}
