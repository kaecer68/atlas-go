package stockpicker

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// TestGroupAndSummarizeByRegime_Strata 同一 (symbol, source) 按 regime 切分，
// 空 regime 歸 unknown（family_expectancy.go 同口徑）。
func TestGroupAndSummarizeByRegime_Strata(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-08-01", ForwardReturn: 0.03, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-02", ForwardReturn: -0.01, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-03", ForwardReturn: 0.05, Source: "stockpicker-x", Regime: "RISK_OFF"},
		{Symbol: "2330", TriggerDate: "2026-08-04", ForwardReturn: 0.04, Source: "stockpicker-x"},
	}
	got := GroupAndSummarizeByRegime(outcomes, "120d", 0.00585, 1, 0.95)
	if len(got) != 3 {
		t.Fatalf("strata = %d, want 3 (RISK_ON/RISK_OFF/unknown): %+v", len(got), got)
	}
	byRegime := map[string]RegimeWinRateSummary{}
	for _, s := range got {
		byRegime[s.Regime] = s
	}
	on := byRegime["RISK_ON"]
	if on.Observations != 2 || on.Hits != 1 {
		t.Errorf("RISK_ON = %+v, want obs=2 hits=1", on)
	}
	if byRegime["RISK_OFF"].Observations != 1 {
		t.Errorf("RISK_OFF = %+v, want obs=1", byRegime["RISK_OFF"])
	}
	unknown := byRegime[UnknownRegime]
	if unknown.Observations != 1 || unknown.Regime != UnknownRegime {
		t.Errorf("unknown = %+v, want obs=1 regime=unknown", unknown)
	}
	for _, s := range got {
		if s.Window != "120d" || s.Symbol != "2330" || s.Source != "stockpicker-x" {
			t.Errorf("stratum key not carried: %+v", s)
		}
	}
}

// TestSaveLoadRegimeWinRate_RoundTrip upsert 後可讀回；不存在回 found=false。
func TestSaveLoadRegimeWinRate_RoundTrip(t *testing.T) {
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	ctx := context.Background()
	store := NewWinRateStore(db)
	summary := RegimeWinRateSummary{
		Symbol: "2330", Source: "stockpicker-x", Window: "120d", Regime: "RISK_ON",
		Observations: 30, Hits: 20, WinRate: 20.0 / 30.0,
		WilsonLower: 0.48, WilsonUpper: 0.82, Confidence: 0.95,
		CalibrationStatus: CalibrationEligible, NetCostRate: 0.00585,
		AvgForwardReturn: 0.01, UpdatedAt: "2026-10-11T00:00:00Z",
	}
	if err := store.SaveRegimeWinRate(ctx, summary); err != nil {
		t.Fatalf("save: %v", err)
	}
	back, found, err := store.LoadRegimeWinRate(ctx, "2330", "stockpicker-x", "120d", "RISK_ON")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if back.Hits != 20 || back.Regime != "RISK_ON" || back.CalibrationStatus != CalibrationEligible {
		t.Errorf("round trip mismatch: %+v", back)
	}
	if _, found, err := store.LoadRegimeWinRate(ctx, "2330", "stockpicker-x", "120d", "RISK_OFF"); err != nil || found {
		t.Errorf("missing stratum: found=%v err=%v, want false/nil", found, err)
	}
}

// TestAggregateRegimeFromStore_Writes seeding outcomes 後聚合寫入各 stratum。
func TestAggregateRegimeFromStore_Writes(t *testing.T) {
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	ctx := context.Background()
	outStore := NewSignalOutcomeStore(db)
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-08-01", ForwardReturn: 0.03, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-02", ForwardReturn: -0.01, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-03", ForwardReturn: 0.05, Source: "stockpicker-x", Regime: "RISK_OFF"},
	}
	if err := outStore.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("record: %v", err)
	}
	winStore := NewWinRateStore(db)
	got, err := AggregateRegimeFromStore(ctx, outStore, winStore, "120d", 0.00585, 1, 0.95, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("summaries = %d, want 2: %+v", len(got), got)
	}
	back, found, err := winStore.LoadRegimeWinRate(ctx, "2330", "stockpicker-x", "120d", "RISK_ON")
	if err != nil || !found || back.Observations != 2 || back.Hits != 1 {
		t.Errorf("persisted RISK_ON stratum wrong: found=%v err=%v %+v", found, err, back)
	}
}
