package stockpicker

import (
	"context"
	"testing"
)

func sampleWinRateSummary() StockWinRateSummary {
	return StockWinRateSummary{
		Symbol:            "2330",
		Source:            "stockpicker-momentum",
		Window:            "120d",
		Observations:      40,
		Hits:              24,
		WinRate:           0.6,
		WilsonLower:       0.44,
		WilsonUpper:       0.74,
		Confidence:        0.95,
		CalibrationStatus: WinRateEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  0.012,
		UpdatedAt:         "2026-08-27T12:00:00Z",
	}
}

func TestWinRateStore_SaveAndLoad(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewWinRateStore(db)
	if err != nil {
		t.Fatalf("NewWinRateStore: %v", err)
	}
	ctx := context.Background()

	in := sampleWinRateSummary()
	if err := store.SaveWinRate(ctx, in); err != nil {
		t.Fatalf("SaveWinRate: %v", err)
	}

	got, found, err := store.LoadWinRate(ctx, "2330", "stockpicker-momentum", "120d")
	if err != nil {
		t.Fatalf("LoadWinRate: %v", err)
	}
	if !found {
		t.Fatal("LoadWinRate: found = false, want true")
	}
	if got != in {
		t.Fatalf("round trip mismatch: got = %+v, want = %+v", got, in)
	}

	// 未寫入的 key → found=false、nil error。
	missing, found, err := store.LoadWinRate(ctx, "2330", "stockpicker-momentum", "60d")
	if err != nil {
		t.Fatalf("LoadWinRate(missing): %v", err)
	}
	if found {
		t.Fatalf("LoadWinRate(missing): found = true, want false (got %+v)", missing)
	}
}

func TestWinRateStore_UpsertUpdates(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewWinRateStore(db)
	if err != nil {
		t.Fatalf("NewWinRateStore: %v", err)
	}
	ctx := context.Background()

	first := sampleWinRateSummary()
	if err := store.SaveWinRate(ctx, first); err != nil {
		t.Fatalf("first SaveWinRate: %v", err)
	}

	second := first
	second.Observations = 45
	second.Hits = 30
	second.WinRate = 30.0 / 45.0
	second.CalibrationStatus = WinRateDegraded
	second.UpdatedAt = "2026-08-27T13:00:00Z"
	if err := store.SaveWinRate(ctx, second); err != nil {
		t.Fatalf("second SaveWinRate (upsert): %v", err)
	}

	got, found, err := store.LoadWinRate(ctx, first.Symbol, first.Source, first.Window)
	if err != nil {
		t.Fatalf("LoadWinRate: %v", err)
	}
	if !found {
		t.Fatal("LoadWinRate: found = false, want true")
	}
	if got.Observations != 45 || got.Hits != 30 || got.WinRate != 30.0/45.0 {
		t.Fatalf("upsert did not update numeric fields: got %+v", got)
	}
	if got.CalibrationStatus != WinRateDegraded || got.UpdatedAt != "2026-08-27T13:00:00Z" {
		t.Fatalf("upsert did not update status/updated_at: got %+v", got)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM stock_win_rate WHERE symbol = ? AND source = ? AND window = ?`,
		first.Symbol, first.Source, first.Window,
	).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("upsert produced %d rows, want 1", n)
	}
}

func TestCalibrationStatusRoundTrip(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewWinRateStore(db)
	if err != nil {
		t.Fatalf("NewWinRateStore: %v", err)
	}
	ctx := context.Background()

	for _, status := range []string{WinRateCalibrating, WinRateEligible, WinRateDegraded} {
		in := sampleWinRateSummary()
		in.Source = "source-" + status
		in.CalibrationStatus = status

		if err := store.SaveWinRate(ctx, in); err != nil {
			t.Fatalf("SaveWinRate(%s): %v", status, err)
		}

		got, found, err := store.LoadWinRate(ctx, in.Symbol, in.Source, in.Window)
		if err != nil {
			t.Fatalf("LoadWinRate(%s): %v", status, err)
		}
		if !found {
			t.Fatalf("LoadWinRate(%s): found = false, want true", status)
		}
		if got.CalibrationStatus != status {
			t.Fatalf("calibration status round trip = %q, want %q", got.CalibrationStatus, status)
		}
	}
}

func TestWinRateStore_RejectsInvalidCalibrationStatus(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewWinRateStore(db)
	if err != nil {
		t.Fatalf("NewWinRateStore: %v", err)
	}

	in := sampleWinRateSummary()
	in.CalibrationStatus = "not-a-status"
	if err := store.SaveWinRate(context.Background(), in); err == nil {
		t.Fatal("SaveWinRate with invalid calibration_status: want error, got nil")
	}
}

// TestAggregateWinRate_SkipsConsistencyCheck 驗證 §5-1 解法：跨 symbol / 跨
// source 的混合 outcomes 可以直接聚合，不再因 SignalWinRate 的一致性檢查
// 而報 mixed symbols/sources 錯誤。
func TestAggregateWinRate_SkipsConsistencyCheck(t *testing.T) {
	const (
		costRate   = 0.00585
		minSamples = 30
		confidence = 0.95
	)
	outcomes := []SignalOutcome{
		{Symbol: "2330", Source: "stockpicker-momentum", ForwardReturn: 0.02},
		{Symbol: "2317", Source: "research-agent-1", ForwardReturn: -0.01},
		{Symbol: "2330", Source: "research-agent-1", ForwardReturn: 0.01},
	}

	got := aggregateWinRateWithoutConsistency(outcomes, costRate, minSamples, confidence)

	if got.Observations != 3 {
		t.Fatalf("Observations = %d, want 3", got.Observations)
	}
	// 命中：0.02 > 0.00585、0.01 > 0.00585，共 2 筆；-0.01 不命中。
	if got.Hits != 2 {
		t.Fatalf("Hits = %d, want 2", got.Hits)
	}
	if got.WinRate != 2.0/3.0 {
		t.Fatalf("WinRate = %v, want %v", got.WinRate, 2.0/3.0)
	}
	if got.Symbol != "" || got.Source != "" {
		t.Fatalf("cross aggregate must not fill Symbol/Source, got %q/%q", got.Symbol, got.Source)
	}
	if got.CalibrationStatus != CalibrationCalibrating {
		t.Fatalf("CalibrationStatus = %q, want %q (3 < minSamples)", got.CalibrationStatus, CalibrationCalibrating)
	}
}
