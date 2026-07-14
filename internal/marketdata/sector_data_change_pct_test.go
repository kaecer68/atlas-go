package marketdata

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestSectorDataProvider_ChangePctFromCache 驗證 PR-B Bug#5 fix：
// sector_data JSON 只有當期值，ChangePct 從 in-memory cache 上次 fetch 計算。
// - 第一次 fetch：cache 為 0 → ChangePct=0（cold start）
// - 第二次 fetch：cache = 上次的 value → ChangePct = (new-old)/old*100
func TestSectorDataProvider_ChangePctFromCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sector_data.json")

	if err := os.WriteFile(path, []byte(`{
		"ai_revenue_growth": 100,
		"cowos_utilization": 80,
		"capex_growth": 50,
		"semiconductor_index": 5000,
		"updated_at": "2026-07-12T00:00:00Z"
	}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p := NewSectorDataProvider(dir)

	// First fetch: cold start, all ChangePct=0
	snap1, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if snap1.TSMCRevenue.ChangePct != 0 {
		t.Errorf("cold start: TSMC.ChangePct = %v, want 0", snap1.TSMCRevenue.ChangePct)
	}
	if snap1.SOXIndex.ChangePct != 0 {
		t.Errorf("cold start: SOX.ChangePct = %v, want 0", snap1.SOXIndex.ChangePct)
	}

	// Mutate file to simulate next fetch with new values
	if err := os.WriteFile(path, []byte(`{
		"ai_revenue_growth": 110,
		"cowos_utilization": 88,
		"capex_growth": 55,
		"semiconductor_index": 5250,
		"updated_at": "2026-07-13T00:00:00Z"
	}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Second fetch: ChangePct derived from previous values
	snap2, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	// AI: (110-100)/100*100 = 10
	if got, want := snap2.TSMCRevenue.ChangePct, 10.0; got != want {
		t.Errorf("TSMC.ChangePct = %v, want %v", got, want)
	}
	// CoWoS: (88-80)/80*100 = 10
	if got, want := snap2.CoWoSUtilization.ChangePct, 10.0; got != want {
		t.Errorf("CoWoS.ChangePct = %v, want %v", got, want)
	}
	// Capex: (55-50)/50*100 = 10
	if got, want := snap2.CapexGrowth.ChangePct, 10.0; got != want {
		t.Errorf("Capex.ChangePct = %v, want %v", got, want)
	}
	// SOX: (5250-5000)/5000*100 = 5
	if got, want := snap2.SOXIndex.ChangePct, 5.0; got != want {
		t.Errorf("SOX.ChangePct = %v, want %v", got, want)
	}
}

func TestPctChange_ZeroPreviousReturnsZero(t *testing.T) {
	if got := pctChange(100, 0); got != 0 {
		t.Errorf("pctChange(100, 0) = %v, want 0", got)
	}
	if got := pctChange(0, 0); got != 0 {
		t.Errorf("pctChange(0, 0) = %v, want 0", got)
	}
}

func TestPctChange_NegativeChange(t *testing.T) {
	// Price drops from 100 to 90 → -10%
	if got := pctChange(90, 100); got != -10.0 {
		t.Errorf("pctChange(90, 100) = %v, want -10", got)
	}
}
