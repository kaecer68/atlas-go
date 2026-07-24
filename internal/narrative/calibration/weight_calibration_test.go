package calibration

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestWeightCalibrationEngine_CalibratesPerfectFactorHighest(t *testing.T) {
	engine := &WeightCalibrationEngine{}
	records := []CalibrationRecord{
		{Snapshot: syntheticSnapshot(0, 0, 0), ForeignNet: -1, Outflow: 1},
		{Snapshot: syntheticSnapshot(0, 0, 0), ForeignNet: -2, Outflow: 2},
		{Snapshot: syntheticSnapshot(0, 0, 0), ForeignNet: 1, Outflow: -1},
		{Snapshot: syntheticSnapshot(0, 0, 0), ForeignNet: -3, Outflow: 3},
	}

	accuracies := engine.ComputeFactorAccuracy(records)
	if accuracies["foreign_flow"] != 1 {
		t.Fatalf("expected foreign_flow accuracy 1, got %.2f", accuracies["foreign_flow"])
	}

	weights := engine.CalibrateWeights(accuracies)
	if weights.ForeignFlow <= weights.DXY || weights.ForeignFlow <= weights.US10Y {
		t.Fatalf("expected foreign_flow to have highest weight, got %+v", weights)
	}
	if sum := weights.DXY + weights.US10Y + weights.ForeignFlow + weights.VIX + weights.JPY + weights.Geopolitical + weights.Oil + weights.Gold; sum < 0.99 || sum > 1.01 {
		t.Fatalf("weights should sum to 1, got %.4f", sum)
	}
	if weights.VIX < 0.05 || weights.JPY < 0.05 || weights.Geopolitical < 0.05 || weights.Oil < 0.05 || weights.Gold < 0.05 {
		t.Fatalf("expected minimum floor on all factors, got %+v", weights)
	}
}

func TestWeightCalibrationEngine_ExportConfigWritesValidFile(t *testing.T) {
	dir := t.TempDir()
	engine := &WeightCalibrationEngine{}
	weights := StressIndexWeights{DXY: 0.15, US10Y: 0.20, ForeignFlow: 0.25, VIX: 0.15, JPY: 0.10, Geopolitical: 0.15}
	if err := engine.ExportConfig(dir, weights, StressIndexScaling{DXY: 5, US10Y: 2, ForeignFlow: 10, VIX: 2.5, JPY: 10, Geopolitical: 1}, StressIndexThresholds{Crisis: 70, High: 50, Alert: 30}); err != nil {
		t.Fatalf("export config failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "configs", "stress_index_weights.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	cfg := LoadWeightsConfig(dir)
	if cfg == nil {
		t.Fatalf("expected config to load, got nil: %s", string(data))
	}
	if !cfg.IsValid() {
		t.Fatalf("expected valid config, got %+v", cfg)
	}
}

func TestWeightCalibrationEngine_ZeroHistoryFallback(t *testing.T) {
	engine := &WeightCalibrationEngine{}
	weights := engine.CalibrateWeights(map[string]float64{})
	if weights.DXY != 0.13 || weights.US10Y != 0.18 || weights.ForeignFlow != 0.22 || weights.VIX != 0.13 || weights.JPY != 0.08 || weights.Geopolitical != 0.13 || weights.Oil != 0.07 || weights.Gold != 0.06 {
		t.Fatalf("expected default weights on zero history, got %+v", weights)
	}
}

func TestWeightCalibrationEngine_LoadHistoricalDataPairsMacroAndFlow(t *testing.T) {
	dir := t.TempDir()
	macroDir := filepath.Join(dir, "data", "state", "macro")
	flowDir := filepath.Join(dir, "data", "state", "capital_flow")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSyntheticPair(t, macroDir, flowDir, "2026-05-17", -2)
	writeSyntheticPair(t, macroDir, flowDir, "2026-05-18", 3)

	recs, err := (&WeightCalibrationEngine{}).LoadHistoricalData(dir, 2)
	if err != nil {
		t.Fatalf("load historical data failed: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0].Date.After(recs[1].Date) {
		t.Fatalf("expected chronological order, got %v then %v", recs[0].Date, recs[1].Date)
	}
	if recs[0].OutflowTarget != recs[0].Outflow {
		t.Fatalf("expected outflow target to match outflow")
	}
}

func syntheticSnapshot(dxy, us10y, vix float64) marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: dxy},
		US10Y:              marketdata.MacroDataPoint{Value: us10y},
		VIX:                marketdata.MacroDataPoint{Value: vix},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -1},
		JPY:                marketdata.MacroDataPoint{ChangePct: 0},
		Oil:                marketdata.MacroDataPoint{ChangePct: 0},
		Gold:               marketdata.MacroDataPoint{ChangePct: 0},
		RecordedAt:         time.Now().Unix(),
	}
}

func writeSyntheticPair(t *testing.T, macroDir, flowDir, date string, foreignNet float64) {
	t.Helper()
	macro := `{"us10y":{"value":1},"dxy":{"change_pct":1},"vix":{"value":10},"jpy":{"change_pct":0},"gold":{"change_pct":0},"oil":{"change_pct":0},"foreign_investor_net":{"value":` + itoaFloat(foreignNet) + `},"recorded_at":1}`
	if err := os.WriteFile(filepath.Join(macroDir, date+".json"), []byte(macro), 0o644); err != nil {
		t.Fatal(err)
	}
	flow := `{"date":"` + date + `","foreign_investor_net":` + itoaFloat(foreignNet) + `,"domestic_fund_net":0,"dealer_net":0,"total_net":` + itoaFloat(foreignNet) + `}`
	if err := os.WriteFile(filepath.Join(flowDir, strings.ReplaceAll(date, "-", "")+".json"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoaFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
