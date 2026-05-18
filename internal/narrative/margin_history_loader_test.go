package narrative

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMarginHistory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"20260514_margin.json": `{"date":"20260514","margin_balance":5000,"change_pct":1.2}`,
		"20260513_margin.json": `{"date":"20260513","margin_balance":4900,"change_pct":-0.5}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	history, err := LoadMarginHistory(dir)
	if err != nil {
		t.Fatalf("LoadMarginHistory failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(history))
	}
	if history[0].Date != "20260513" || history[1].Date != "20260514" {
		t.Fatalf("expected sorted dates, got %+v", history)
	}
}

func TestComputeRollingPercentile(t *testing.T) {
	history := []MarginHistoryEntry{{MarginBalance: 1}, {MarginBalance: 2}, {MarginBalance: 3}, {MarginBalance: 4}, {MarginBalance: 5}}
	percentile, ok := ComputeRollingPercentile(history, 3, 5)
	if !ok {
		t.Fatalf("expected percentile to be available")
	}
	if percentile < 49 || percentile > 51 {
		t.Fatalf("expected percentile around 50, got %.2f", percentile)
	}
}

func TestComputeRollingAcceleration(t *testing.T) {
	history := []MarginHistoryEntry{{MarginBalance: 10}, {MarginBalance: 12}, {MarginBalance: 15}, {MarginBalance: 19}, {MarginBalance: 24}, {MarginBalance: 30}}
	accel, ok := ComputeRollingAcceleration(history, 5)
	if !ok {
		t.Fatalf("expected acceleration to be available")
	}
	if accel < 3.9 || accel > 4.1 {
		t.Fatalf("expected acceleration around 4, got %.2f", accel)
	}
}

func TestMarginHistoryInsufficientData(t *testing.T) {
	history := []MarginHistoryEntry{{MarginBalance: 1}, {MarginBalance: 2}}
	if _, ok := ComputeRollingPercentile(history, 2, 5); ok {
		t.Fatalf("expected percentile to be unavailable with sparse history")
	}
	if _, ok := ComputeRollingAcceleration(history, 5); ok {
		t.Fatalf("expected acceleration to be unavailable with sparse history")
	}
}
