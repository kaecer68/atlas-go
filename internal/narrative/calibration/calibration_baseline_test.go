package calibration

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// baselineSnapshot creates a snapshot with specified DXY change and value.
func baselineSnapshot(dxyChange float64, dxyValue float64) marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: dxyChange, Value: dxyValue},
		US10Y:              marketdata.MacroDataPoint{Value: 2.5},
		VIX:                marketdata.MacroDataPoint{Value: 15},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -1},
		JPY:                marketdata.MacroDataPoint{ChangePct: 0},
		Oil:                marketdata.MacroDataPoint{ChangePct: 0},
		Gold:               marketdata.MacroDataPoint{ChangePct: 0},
		RecordedAt:         time.Now().Unix(),
	}
}

func TestComputeBaselines_With30Records(t *testing.T) {
	records := make([]CalibrationRecord, 30)
	for i := range records {
		records[i] = CalibrationRecord{
			Date:       time.Now().AddDate(0, 0, -30+i),
			Snapshot:   baselineSnapshot(float64(i%5)*0.1, 100+float64(i%5)),
			ForeignNet: float64(i % 3),
		}
	}

	cfg := ComputeBaselines(records, nil)

	if cfg.Window != 30 {
		t.Fatalf("expected window 30, got %d", cfg.Window)
	}

	bl := cfg.GetBaseline("dxy")
	if bl == nil {
		t.Fatal("expected dxy baseline, got nil")
	}
	if bl.Count != 30 {
		t.Fatalf("expected count 30, got %d", bl.Count)
	}
	if bl.Mean == 0 && bl.StdDev == 0 {
		t.Fatal("expected non-zero mean or stddev for dxy baseline")
	}

	// Verify all 8 factors computed.
	factors := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	for _, f := range factors {
		if cfg.GetBaseline(f) == nil {
			t.Fatalf("expected baseline for factor %s", f)
		}
	}
}

func TestComputeBaselines_WindowSmallerThanData(t *testing.T) {
	records := make([]CalibrationRecord, 50)
	for i := range records {
		records[i] = CalibrationRecord{
			Snapshot:   baselineSnapshot(float64(i)*0.1, 100+float64(i)),
			ForeignNet: float64(i),
		}
	}

	cfg := ComputeBaselines(records, &BaselineConfig{Window: 20})
	bl := cfg.GetBaseline("dxy")
	if bl == nil {
		t.Fatal("expected dxy baseline")
	}
	// Only last 20 records should be used.
	if bl.Count != 20 {
		t.Fatalf("expected count 20 with window=20, got %d", bl.Count)
	}
}

func TestComputeBaselines_EmptyRecords(t *testing.T) {
	cfg := ComputeBaselines(nil, nil)
	if len(cfg.Baselines) != 0 {
		t.Fatalf("expected empty baselines, got %d", len(cfg.Baselines))
	}
	if cfg.Window != 30 {
		t.Fatalf("expected default window 30, got %d", cfg.Window)
	}
}

func TestComputeLevelSignal_ZeroBaseline(t *testing.T) {
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 0, StdDev: 0, Count: 10},
		},
		Window: 30,
	}
	snap := baselineSnapshot(5.0, 105)
	signal := ComputeLevelSignal("dxy", snap, 0, cfg)
	if signal != 0 {
		t.Fatalf("expected 0 signal when stddev=0, got %.4f", signal)
	}
}

func TestComputeLevelSignal_NilBaseline(t *testing.T) {
	snap := baselineSnapshot(5.0, 105)
	signal := ComputeLevelSignal("dxy", snap, 0, nil)
	if signal != 0 {
		t.Fatalf("expected 0 signal when cfg nil, got %.4f", signal)
	}
}

func TestComputeLevelSignal_MissingFactor(t *testing.T) {
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{},
		Window:    30,
	}
	snap := baselineSnapshot(5.0, 105)
	signal := ComputeLevelSignal("dxy", snap, 0, cfg)
	if signal != 0 {
		t.Fatalf("expected 0 signal for missing factor, got %.4f", signal)
	}
}

func TestComputeLevelSignal_NormalDeviation(t *testing.T) {
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 1.0, StdDev: 0.5, Count: 30},
		},
		Window: 30,
	}
	// DXY change_pct = 2.0 → level signal = (2.0 - 1.0) / 0.5 = 2.0
	snap := baselineSnapshot(2.0, 105)
	signal := ComputeLevelSignal("dxy", snap, 0, cfg)
	if math.Abs(signal-2.0) > 1e-9 {
		t.Fatalf("expected level signal 2.0, got %.4f", signal)
	}
}

func TestComputeHybridSignal_PicksMaxAbsolute(t *testing.T) {
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 1.0, StdDev: 0.5, Count: 30},
		},
		Window: 30,
	}

	// Case 1: level signal > change signal.
	// change = 0.5, level = (0.5 - 1.0) / 0.5 = -1.0 → |level| > |change| → return -1.0
	snap := baselineSnapshot(0.5, 105)
	signal := ComputeHybridSignal("dxy", snap, 0, cfg)
	if math.Abs(signal-(-1.0)) > 1e-9 {
		t.Fatalf("expected hybrid signal -1.0 (level dominates), got %.4f", signal)
	}

	// Case 2: change signal > level signal.
	// change = 3.0, level = (3.0 - 1.0) / 0.5 = 4.0 → |level| > |change| → return 4.0
	snap2 := baselineSnapshot(3.0, 105)
	signal2 := ComputeHybridSignal("dxy", snap2, 0, cfg)
	if math.Abs(signal2-4.0) > 1e-9 {
		t.Fatalf("expected hybrid signal 4.0 (level dominates), got %.4f", signal2)
	}

	// Case 3: change signal dominates.
	// change = 5.0, level = (5.0 - 1.0) / 0.5 = 8.0 → |level| > |change| → return 8.0
	snap3 := baselineSnapshot(5.0, 105)
	signal3 := ComputeHybridSignal("dxy", snap3, 0, cfg)
	if math.Abs(signal3-8.0) > 1e-9 {
		t.Fatalf("expected hybrid signal 8.0, got %.4f", signal3)
	}
}

func TestComputeHybridSignal_FallbackWhenNoBaseline(t *testing.T) {
	snap := baselineSnapshot(2.0, 105)
	// No baseline → level = 0, change = 2.0 → hybrid should return 2.0.
	signal := ComputeHybridSignal("dxy", snap, 0, nil)
	if math.Abs(signal-2.0) > 1e-9 {
		t.Fatalf("expected hybrid to fallback to change=2.0, got %.4f", signal)
	}
}

func TestBaselineConfig_Accessors(t *testing.T) {
	// Nil config.
	var nilCfg *BaselineConfig
	if nilCfg.GetBaseline("dxy") != nil {
		t.Fatal("expected nil baseline from nil config")
	}
	if nilCfg.WindowSize() != 30 {
		t.Fatalf("expected default window 30, got %d", nilCfg.WindowSize())
	}

	// Config with no baselines map.
	emptyCfg := &BaselineConfig{Window: 20}
	if emptyCfg.GetBaseline("dxy") != nil {
		t.Fatal("expected nil baseline from empty baselines map")
	}
	if emptyCfg.WindowSize() != 20 {
		t.Fatalf("expected window 20, got %d", emptyCfg.WindowSize())
	}

	// Zero window defaults to 30.
	zeroWindow := &BaselineConfig{Window: 0}
	if zeroWindow.WindowSize() != 30 {
		t.Fatalf("expected default window 30 for zero, got %d", zeroWindow.WindowSize())
	}

	// Normal access.
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 1.0, StdDev: 0.5, Count: 30},
		},
		Window: 25,
	}
	bl := cfg.GetBaseline("dxy")
	if bl == nil || bl.Mean != 1.0 {
		t.Fatal("expected to retrieve dxy baseline")
	}
	if cfg.WindowSize() != 25 {
		t.Fatalf("expected window 25, got %d", cfg.WindowSize())
	}
}

func TestMeanStdDev_Basic(t *testing.T) {
	t.Parallel()

	// mean of {2, 4, 6} = 4.0
	vals := []float64{2, 4, 6}
	m := mean(vals)
	if math.Abs(m-4.0) > 1e-9 {
		t.Fatalf("expected mean 4.0, got %.4f", m)
	}

	// empty → 0
	if m := mean(nil); m != 0 {
		t.Fatalf("expected mean 0 for nil, got %.4f", m)
	}
	if m := mean([]float64{}); m != 0 {
		t.Fatalf("expected mean 0 for empty, got %.4f", m)
	}

	// single element → stddev = 0
	if sd := stdDev([]float64{5}); sd != 0 {
		t.Fatalf("expected 0 for single element, got %.4f", sd)
	}

	// std dev of {2,4,4,4,5,5,7,9} = 2.0
	sdVals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	if sd := stdDev(sdVals); math.Abs(sd-2.0) > 0.01 {
		t.Fatalf("expected ~2.0, got %.4f", sd)
	}
}

func TestComputeBaselines_ZeroValues(t *testing.T) {
	t.Parallel()

	// All records have DXY ChangePct=0 and Value=0 (invalid data).
	// factorSignal("dxy", ...) returns ChangePct which is 0 for all.
	// The baseline mean and stddev should still compute (all-zeros → mean=0, stddev=0).
	records := make([]CalibrationRecord, 10)
	for i := range records {
		records[i] = CalibrationRecord{
			Date: time.Now().AddDate(0, 0, -10+i),
			Snapshot: marketdata.MacroDataSnapshot{
				DXY:  marketdata.MacroDataPoint{ChangePct: 0, Value: 0},
				JPY:  marketdata.MacroDataPoint{ChangePct: 0, Value: 0},
				Oil:  marketdata.MacroDataPoint{ChangePct: 0, Value: 0},
				Gold: marketdata.MacroDataPoint{ChangePct: 0, Value: 0},
			},
		}
	}

	cfg := ComputeBaselines(records, nil)
	bl := cfg.GetBaseline("dxy")
	if bl == nil {
		t.Fatal("expected dxy baseline even with all-zero records")
	}
	// Mean and stddev should be 0 for all-zero input.
	if bl.Mean != 0 {
		t.Fatalf("expected mean 0 for all-zero records, got %.4f", bl.Mean)
	}
	if bl.StdDev != 0 {
		t.Fatalf("expected stddev 0 for all-zero records, got %.4f", bl.StdDev)
	}
	// Level signal should be 0 when stddev is 0 (degenerate baseline).
	snap := marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 0, Value: 0}}
	sig := ComputeLevelSignal("dxy", snap, 0, cfg)
	if sig != 0 {
		t.Fatalf("expected 0 level signal for degenerate baseline, got %.4f", sig)
	}
}

func TestComputeHybridSignal_ChangeWins(t *testing.T) {
	t.Parallel()

	// Baseline: mean=1.0, stddev=0.5
	// If changeSignal=5.0, levelSignal=(5.0-1.0)/0.5=8.0 → |level|>|change| → level wins.
	// To make change win, we need a change signal where |change| > |level|.
	// With mean=1.0, stddev=0.5: level = (change - 1.0) / 0.5 = 2*(change-1.0)
	// For change=1.1: level = 2*0.1 = 0.2 → |change|=1.1 > |level|=0.2 → change wins.
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 1.0, StdDev: 0.5, Count: 30},
		},
		Window: 30,
	}

	snap := baselineSnapshot(1.1, 105)
	signal := ComputeHybridSignal("dxy", snap, 0, cfg)
	// change=1.1, level=(1.1-1.0)/0.5=0.2 → change wins → should return 1.1
	if math.Abs(signal-1.1) > 1e-9 {
		t.Fatalf("expected hybrid signal 1.1 (change dominates), got %.4f", signal)
	}
}

func TestComputeHybridSignal_LevelWins(t *testing.T) {
	t.Parallel()

	// Baseline: mean=1.0, stddev=0.5
	// change=0.1, level=(0.1-1.0)/0.5=-1.8 → |level|=1.8 > |change|=0.1 → level wins.
	cfg := &BaselineConfig{
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 1.0, StdDev: 0.5, Count: 30},
		},
		Window: 30,
	}

	snap := baselineSnapshot(0.1, 105)
	signal := ComputeHybridSignal("dxy", snap, 0, cfg)
	// change=0.1, level=(0.1-1.0)/0.5=-1.8 → |level|>|change| → return -1.8
	expected := -1.8
	if math.Abs(signal-expected) > 1e-9 {
		t.Fatalf("expected hybrid signal %.4f (level dominates), got %.4f", expected, signal)
	}
}

func TestSignalStrategy_Values(t *testing.T) {
	t.Parallel()

	if SignalChange != 0 {
		t.Fatalf("expected SignalChange=0, got %d", SignalChange)
	}
	if SignalLevel != 1 {
		t.Fatalf("expected SignalLevel=1, got %d", SignalLevel)
	}
	if SignalHybrid != 2 {
		t.Fatalf("expected SignalHybrid=2, got %d", SignalHybrid)
	}
}

func TestSaveAndLoadBaselines_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &BaselineConfig{
		Window: 60,
		Baselines: map[string]*FactorBaseline{
			"dxy":   {Factor: "dxy", Mean: 0.1, StdDev: 0.5, Count: 60},
			"jpy":   {Factor: "jpy", Mean: 0.0, StdDev: 0.7, Count: 60},
			"us10y": {Factor: "us10y", Mean: 4.5, StdDev: 0.3, Count: 60},
		},
	}

	if err := SaveBaselines(dir, cfg); err != nil {
		t.Fatalf("SaveBaselines failed: %v", err)
	}

	loaded, err := LoadBaselines(dir)
	if err != nil {
		t.Fatalf("LoadBaselines failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadBaselines returned nil without error")
	}
	if loaded.Window != 60 {
		t.Errorf("expected Window=60, got %d", loaded.Window)
	}
	if len(loaded.Baselines) != 3 {
		t.Errorf("expected 3 baselines, got %d", len(loaded.Baselines))
	}
	if bl, ok := loaded.Baselines["dxy"]; !ok || bl.Mean != 0.1 {
		t.Errorf("expected dxy baseline with Mean=0.1, got %+v", bl)
	}
}

func TestLoadBaselines_MissingFileReturnsNil(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	loaded, err := LoadBaselines(dir)
	if err != nil {
		t.Fatalf("LoadBaselines on empty dir should not error, got: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil config for missing file, got %+v", loaded)
	}
}

func TestSaveBaselines_NilConfig(t *testing.T) {
	t.Parallel()

	if err := SaveBaselines(t.TempDir(), nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestSaveBaselines_EmptyWorkDir(t *testing.T) {
	t.Parallel()

	cfg := &BaselineConfig{Window: 30}
	if err := SaveBaselines("", cfg); err == nil {
		t.Fatal("expected error for empty workDir")
	}
}

func TestLoadBaselines_EmptyWorkDir(t *testing.T) {
	t.Parallel()

	_, err := LoadBaselines("")
	if err == nil {
		t.Fatal("expected error for empty workDir")
	}
}

func TestFactorSignal_JPYUsesRawRate(t *testing.T) {
	t.Parallel()

	// JPY carry trade unwinds are state-driven: extreme USD/JPY rate level
	// relative to history signals pressure, not single-day moves.
	// factorSignal must return the raw rate, not ChangePct.
	flatSnap := marketdata.MacroDataSnapshot{
		JPY: marketdata.MacroDataPoint{Symbol: "USDJPY=X", Value: 160.0, ChangePct: 0.0},
	}
	if got := factorSignal("jpy", flatSnap, 0); got != 160.0 {
		t.Fatalf("flat day: expected JPY signal=160.0 (raw rate), got %v (would zero out level signal)", got)
	}

	movingSnap := marketdata.MacroDataSnapshot{
		JPY: marketdata.MacroDataPoint{Symbol: "USDJPY=X", Value: 152.0, ChangePct: 0.5},
	}
	if got := factorSignal("jpy", movingSnap, 0); got != 152.0 {
		t.Fatalf("moving day: expected JPY signal=152.0 (raw rate), got %v", got)
	}
}

func TestComputeLevelSignal_JPYOnFlatDay(t *testing.T) {
	t.Parallel()

	// Baseline computed from historical JPY rates (mean=150, stddev=3).
	bl := &BaselineConfig{
		Window: 30,
		Baselines: map[string]*FactorBaseline{
			"jpy": {Factor: "jpy", Mean: 150.0, StdDev: 3.0, Count: 30},
		},
	}

	// Flat day: ChangePct=0, but USD/JPY at 162 (4 sigma above mean).
	// Old code would have levelDev=0 here (since JPY.ChangePct=0 and
	// historical mean of ChangePct≈0). New code: (162-150)/3 = 4.0
	// → 4 sigma deviation → strong stress signal.
	flatSnap := marketdata.MacroDataSnapshot{
		JPY: marketdata.MacroDataPoint{Symbol: "USDJPY=X", Value: 162.0, ChangePct: 0.0},
	}
	levelDev := ComputeLevelSignal("jpy", flatSnap, 0, bl)
	if math.Abs(levelDev-4.0) > 1e-9 {
		t.Fatalf("flat day with USDJPY=162 vs baseline mean=150 stddev=3: expected levelDev=4.0, got %v", levelDev)
	}

	// Also test the opposite direction (JPY strengthening → carry unwind).
	strongSnap := marketdata.MacroDataSnapshot{
		JPY: marketdata.MacroDataPoint{Symbol: "USDJPY=X", Value: 138.0, ChangePct: 0.0},
	}
	levelDevStrong := ComputeLevelSignal("jpy", strongSnap, 0, bl)
	if math.Abs(levelDevStrong-(-4.0)) > 1e-9 {
		t.Fatalf("strong JPY (USDJPY=138) vs baseline: expected levelDev=-4.0, got %v", levelDevStrong)
	}
}

func TestExtractFactorValues_JPYCapturesRateVariation(t *testing.T) {
	t.Parallel()

	// Build records with varying USD/JPY rates but flat ChangePct.
	// extractFactorValues should return the rate sequence, enabling
	// ComputeBaselines to produce a non-zero stddev for level signals.
	records := []CalibrationRecord{
		{Snapshot: marketdata.MacroDataSnapshot{JPY: marketdata.MacroDataPoint{Value: 148.0, ChangePct: 0.0}}},
		{Snapshot: marketdata.MacroDataSnapshot{JPY: marketdata.MacroDataPoint{Value: 150.0, ChangePct: 0.0}}},
		{Snapshot: marketdata.MacroDataSnapshot{JPY: marketdata.MacroDataPoint{Value: 152.0, ChangePct: 0.0}}},
		{Snapshot: marketdata.MacroDataSnapshot{JPY: marketdata.MacroDataPoint{Value: 154.0, ChangePct: 0.0}}},
		{Snapshot: marketdata.MacroDataSnapshot{JPY: marketdata.MacroDataPoint{Value: 156.0, ChangePct: 0.0}}},
	}

	cfg := &BaselineConfig{Window: 60}
	bls := ComputeBaselines(records, cfg)
	jpyBl, ok := bls.Baselines["jpy"]
	if !ok {
		t.Fatal("expected jpy baseline")
	}
	if jpyBl.Mean != 152.0 {
		t.Errorf("expected JPY baseline mean=152.0, got %v", jpyBl.Mean)
	}
	expectedStddev := math.Sqrt((math.Pow(148.0-jpyBl.Mean, 2) +
		math.Pow(150.0-jpyBl.Mean, 2) +
		math.Pow(152.0-jpyBl.Mean, 2) +
		math.Pow(154.0-jpyBl.Mean, 2) +
		math.Pow(156.0-jpyBl.Mean, 2)) / 5.0)
	if math.Abs(jpyBl.StdDev-expectedStddev) > 1e-9 {
		t.Errorf("expected JPY baseline stddev=%v, got %v", expectedStddev, jpyBl.StdDev)
	}
}
