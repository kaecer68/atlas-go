package stockpicker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func sampleOutcomes() []SignalOutcome {
	base := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-01-05", ForwardReturn: 0.02, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"},
		{Symbol: "2330", TriggerDate: "2026-01-06", ForwardReturn: 0.01, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"},
		{Symbol: "2330", TriggerDate: "2026-01-07", ForwardReturn: -0.03, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"},
		{Symbol: "2330", TriggerDate: "2026-01-05", ForwardReturn: 0.015, CostRate: 0.00585, Source: "stockpicker-foreign-3d-net-buy"},
	}
	for i := range base {
		base[i].NetForwardReturn = base[i].ForwardReturn - base[i].CostRate
		base[i].Hit = NetHit(base[i].ForwardReturn, base[i].CostRate)
	}
	return base
}

// TestAggregate_WritesSummary: outcomes → group → summary → persist → load
// round-trip with correct math (observations/hits/win rate from NetHit).
func TestAggregate_WritesSummary(t *testing.T) {
	db := openStockpickerTestDB(t)
	ctx := context.Background()
	outStore := NewSignalOutcomeStore(db)
	winStore := NewWinRateStore(db)

	if err := outStore.RecordOutcomes(ctx, sampleOutcomes()); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	asOf := mustDate(t, "2026-01-31")
	summaries, err := AggregateFromStore(ctx, outStore, winStore, "120d", 0.00585, 30, 0.95, asOf)
	if err != nil {
		t.Fatalf("AggregateFromStore: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries (one per source), got %d", len(summaries))
	}

	var mom StockWinRateSummary
	for _, s := range summaries {
		if s.Source == "stockpicker-momentum-20d-positive" {
			mom = s
		}
	}
	// momentum group: 3 observations, hits = 2 (0.02>0.00585 ✓, 0.01>0.00585 ✓, -0.03 ✗)
	if mom.Symbol != "2330" || mom.Source != "stockpicker-momentum-20d-positive" {
		t.Fatalf("unexpected summary key: %+v", mom)
	}
	if mom.Observations != 3 || mom.Hits != 2 {
		t.Fatalf("momentum summary obs=%d hits=%d, want 3/2", mom.Observations, mom.Hits)
	}
	if mom.Window != "120d" || mom.CalibrationStatus != CalibrationCalibrating {
		t.Fatalf("momentum summary window=%q status=%q, want 120d/calibrating (3 < 30)", mom.Window, mom.CalibrationStatus)
	}
	if want := 2.0 / 3.0; mom.WinRate != want {
		t.Fatalf("momentum WinRate = %v, want %v", mom.WinRate, want)
	}
	if mom.NetCostRate != 0.00585 {
		t.Fatalf("NetCostRate = %v, want 0.00585", mom.NetCostRate)
	}

	// Persisted round-trip.
	got, found, err := winStore.LoadWinRate(ctx, "2330", "stockpicker-momentum-20d-positive", "120d")
	if err != nil || !found {
		t.Fatalf("LoadWinRate found=%v err=%v", found, err)
	}
	if got.Observations != 3 || got.Hits != 2 {
		t.Fatalf("persisted summary obs=%d hits=%d, want 3/2", got.Observations, got.Hits)
	}
}

// TestAggregate_UpsertUpdates: aggregating again with more outcomes updates
// the same key instead of duplicating it.
func TestAggregate_UpsertUpdates(t *testing.T) {
	db := openStockpickerTestDB(t)
	ctx := context.Background()
	outStore := NewSignalOutcomeStore(db)
	winStore := NewWinRateStore(db)

	if err := outStore.RecordOutcomes(ctx, sampleOutcomes()); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}
	asOf := mustDate(t, "2026-01-31")
	if _, err := AggregateFromStore(ctx, outStore, winStore, "120d", 0.00585, 30, 0.95, asOf); err != nil {
		t.Fatalf("first aggregate: %v", err)
	}

	// Second run appends one more momentum outcome then re-aggregates.
	extra := SignalOutcome{Symbol: "2330", TriggerDate: "2026-01-08", ForwardReturn: 0.05, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"}
	extra.NetForwardReturn = extra.ForwardReturn - extra.CostRate
	extra.Hit = NetHit(extra.ForwardReturn, extra.CostRate)
	if err := outStore.RecordOutcomes(ctx, []SignalOutcome{extra}); err != nil {
		t.Fatalf("RecordOutcomes extra: %v", err)
	}
	summaries, err := AggregateFromStore(ctx, outStore, winStore, "120d", 0.00585, 30, 0.95, asOf)
	if err != nil {
		t.Fatalf("second aggregate: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected still 2 keys after upsert, got %d", len(summaries))
	}
	got, _, err := winStore.LoadWinRate(ctx, "2330", "stockpicker-momentum-20d-positive", "120d")
	if err != nil {
		t.Fatalf("LoadWinRate: %v", err)
	}
	if got.Observations != 4 || got.Hits != 3 {
		t.Fatalf("after upsert obs=%d hits=%d, want 4/3 (0.05 also hits)", got.Observations, got.Hits)
	}

	// And no duplicate rows: count distinct via a fresh load.
	all, err := LoadOutcomesAsOf(ctx, db, "", "", "", asOf)
	if err != nil {
		t.Fatalf("LoadOutcomesAsOf: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 outcome rows, got %d", len(all))
	}
}

// TestAggregate_AsOfWindow: LoadOutcomesAsOf with a 120d window relative to a
// fixed as-of date excludes older rows deterministically (P0-5).
func TestAggregate_AsOfWindow(t *testing.T) {
	db := openStockpickerTestDB(t)
	ctx := context.Background()
	outStore := NewSignalOutcomeStore(db)

	old := SignalOutcome{Symbol: "2330", TriggerDate: "2025-01-05", ForwardReturn: 0.02, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"}
	recent := SignalOutcome{Symbol: "2330", TriggerDate: "2026-01-05", ForwardReturn: 0.02, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"}
	if err := outStore.RecordOutcomes(ctx, []SignalOutcome{old, recent}); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	asOf := mustDate(t, "2026-01-31")
	windowed, err := LoadOutcomesAsOf(ctx, db, "", "", "120d", asOf)
	if err != nil {
		t.Fatalf("LoadOutcomesAsOf: %v", err)
	}
	if len(windowed) != 1 || windowed[0].TriggerDate != "2026-01-05" {
		t.Fatalf("120d window (as-of 2026-01-31) should keep only the 2026-01-05 row, got %+v", windowed)
	}

	all, err := LoadOutcomesAsOf(ctx, db, "", "", "", asOf)
	if err != nil {
		t.Fatalf("LoadOutcomesAsOf(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("empty window should keep all rows, got %d", len(all))
	}
}

// TestAggregate_WriteStateJSON verifies the state snapshot shape and the
// eligible-key count.
func TestAggregate_WriteStateJSON(t *testing.T) {
	summaries := []StockWinRateSummary{
		{Symbol: "2330", Source: "stockpicker-momentum-20d-positive", Window: "120d", Observations: 40, Hits: 24, WinRate: 0.6, CalibrationStatus: CalibrationEligible},
		{Symbol: "2330", Source: "stockpicker-foreign-3d-net-buy", Window: "120d", Observations: 3, Hits: 2, WinRate: 0.67, CalibrationStatus: CalibrationCalibrating},
	}
	path := filepath.Join(t.TempDir(), "state", "stock_win_rate.json")
	if err := WriteStateJSON(path, summaries, mustDate(t, "2026-08-27")); err != nil {
		t.Fatalf("WriteStateJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state json: %v", err)
	}
	var snap StateJSON
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal state json: %v", err)
	}
	if snap.AsOf != "2026-08-27" {
		t.Fatalf("AsOf = %q, want 2026-08-27", snap.AsOf)
	}
	if snap.Window != "120d" {
		t.Fatalf("Window = %q, want 120d", snap.Window)
	}
	if snap.Eligible != 1 {
		t.Fatalf("Eligible = %d, want 1", snap.Eligible)
	}
	if len(snap.Summaries) != 2 {
		t.Fatalf("Summaries = %d, want 2", len(snap.Summaries))
	}
}

// TestCalibrationStatus_FromParams verifies min_samples comes from
// configs/parameters.json (stockpicker.calibration.min_samples = 30), not a
// hard-coded literal (contract P0-6).
func TestCalibrationStatus_FromParams(t *testing.T) {
	cfg, err := loadParametersForTest(t)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	minSamples := cfg.Stockpicker.Calibration.MinSamples.Value
	if minSamples != 30 {
		t.Fatalf("parameters.json stockpicker.calibration.min_samples = %d, want 30", minSamples)
	}
	if got := CalibrationStatusFor(30, minSamples); got != CalibrationEligible {
		t.Fatalf("CalibrationStatusFor(30, %d) = %q, want eligible", minSamples, got)
	}
	if got := CalibrationStatusFor(29, minSamples); got != CalibrationCalibrating {
		t.Fatalf("CalibrationStatusFor(29, %d) = %q, want calibrating", minSamples, got)
	}
	if cfg.Stockpicker.Costs.RoundTripPct.Value != 0.00585 {
		t.Fatalf("stockpicker.costs.round_trip_pct = %v, want 0.00585", cfg.Stockpicker.Costs.RoundTripPct.Value)
	}
}

// TestCostRateFromParams pins the round-trip cost source for NetHit (P0-3).
func TestCostRateFromParams(t *testing.T) {
	cfg, err := loadParametersForTest(t)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	cost := cfg.Stockpicker.Costs.RoundTripPct.Value
	// gross 0.006 > cost 0.00585 → hit; gross 0.005 < cost → no hit.
	if !NetHit(0.006, cost) {
		t.Fatalf("NetHit(0.006, %v) = false, want true", cost)
	}
	if NetHit(0.005, cost) {
		t.Fatalf("NetHit(0.005, %v) = true, want false", cost)
	}
}

// loadParametersForTest loads configs/parameters.json relative to the package.
func loadParametersForTest(t *testing.T) (*config.ParametersConfig, error) {
	t.Helper()
	return config.LoadParametersConfig(filepath.Join("..", "..", "configs", "parameters.json"))
}
