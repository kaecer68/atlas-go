package marketdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSectorDataProvider_Name(t *testing.T) {
	p := NewSectorDataProvider("/nonexistent")
	if got := p.Name(); got != "sector_data" {
		t.Errorf("Name() = %q, want %q", got, "sector_data")
	}
}

func TestSectorDataProvider_FetchSnapshot_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{
		"ai_revenue_growth": 65.5,
		"cowos_utilization": 92.0,
		"capex_growth": 48.0,
		"semiconductor_index": 4850.0,
		"updated_at": "2026-05-08T00:00:00Z"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "sector_data.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	p := NewSectorDataProvider(tmpDir)
	snap, err := p.FetchSnapshot(t.Context())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.TSMCRevenue.Value != 65.5 {
		t.Errorf("TSMCRevenue.Value = %v, want 65.5", snap.TSMCRevenue.Value)
	}
	if snap.TSMCRevenue.Symbol != "TSMC_AI_REVENUE" {
		t.Errorf("TSMCRevenue.Symbol = %q, want %q", snap.TSMCRevenue.Symbol, "TSMC_AI_REVENUE")
	}
	if snap.CoWoSUtilization.Value != 92.0 {
		t.Errorf("CoWoSUtilization.Value = %v, want 92.0", snap.CoWoSUtilization.Value)
	}
	if snap.CoWoSUtilization.Symbol != "COWOS_UTILIZATION" {
		t.Errorf("CoWoSUtilization.Symbol = %q, want %q", snap.CoWoSUtilization.Symbol, "COWOS_UTILIZATION")
	}
	if snap.CapexGrowth.Value != 48.0 {
		t.Errorf("CapexGrowth.Value = %v, want 48.0", snap.CapexGrowth.Value)
	}
	if snap.CapexGrowth.Symbol != "CAPEX_GROWTH" {
		t.Errorf("CapexGrowth.Symbol = %q, want %q", snap.CapexGrowth.Symbol, "CAPEX_GROWTH")
	}
	if snap.SOXIndex.Value != 4850.0 {
		t.Errorf("SOXIndex.Value = %v, want 4850.0", snap.SOXIndex.Value)
	}
	if snap.SOXIndex.Symbol != "^SOX" {
		t.Errorf("SOXIndex.Symbol = %q, want %q", snap.SOXIndex.Symbol, "^SOX")
	}
	if snap.RecordedAt == 0 {
		t.Error("RecordedAt should be set")
	}
}

func TestSectorDataProvider_FetchSnapshot_MissingFile(t *testing.T) {
	p := NewSectorDataProvider("/definitely/does/not/exist")
	snap, err := p.FetchSnapshot(t.Context())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.TSMCRevenue.Symbol != "" {
		t.Errorf("TSMCRevenue.Symbol = %q, want empty", snap.TSMCRevenue.Symbol)
	}
	if snap.CoWoSUtilization.Symbol != "" {
		t.Errorf("CoWoSUtilization.Symbol = %q, want empty", snap.CoWoSUtilization.Symbol)
	}
	if snap.CapexGrowth.Symbol != "" {
		t.Errorf("CapexGrowth.Symbol = %q, want empty", snap.CapexGrowth.Symbol)
	}
	if snap.SOXIndex.Symbol != "" {
		t.Errorf("SOXIndex.Symbol = %q, want empty", snap.SOXIndex.Symbol)
	}
	if snap.RecordedAt == 0 {
		t.Error("RecordedAt should be set even for missing file")
	}
}

func TestSectorDataProvider_FetchSnapshot_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "sector_data.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	p := NewSectorDataProvider(tmpDir)
	snap, err := p.FetchSnapshot(t.Context())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.TSMCRevenue.Symbol != "" {
		t.Errorf("TSMCRevenue.Symbol = %q, want empty", snap.TSMCRevenue.Symbol)
	}
	if snap.CoWoSUtilization.Symbol != "" {
		t.Errorf("CoWoSUtilization.Symbol = %q, want empty", snap.CoWoSUtilization.Symbol)
	}
	if snap.CapexGrowth.Symbol != "" {
		t.Errorf("CapexGrowth.Symbol = %q, want empty", snap.CapexGrowth.Symbol)
	}
	if snap.SOXIndex.Symbol != "" {
		t.Errorf("SOXIndex.Symbol = %q, want empty", snap.SOXIndex.Symbol)
	}
}
