package narrative

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

func TestTaiwanStressCalculator_Calculate(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         1713000000,
	}
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 30}

	idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, geo)

	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range: %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	if idx.Timestamp != snap.RecordedAt {
		t.Fatalf("timestamp mismatch: got %v, want %v", idx.Timestamp, snap.RecordedAt)
	}

	// Verify component contributions exist.
	expectedKeys := []string{"dxy", "us10y", "foreign_flow", "vix", "geopolitical", "oil", "gold"}
	for _, k := range expectedKeys {
		if _, ok := idx.Components[k]; !ok {
			t.Fatalf("missing component %s", k)
		}
	}
}

type mockGeoProvider struct {
	calls int
	score geopolitical.GeopoliticalRiskScore
}

func (m *mockGeoProvider) Name() string { return "mock" }
func (m *mockGeoProvider) FetchScore(ctx context.Context) (geopolitical.GeopoliticalRiskScore, error) {
	m.calls++
	return m.score, nil
}

func TestTaiwanStressCalculator_CalculateFromSnapshot_CachesResult(t *testing.T) {
	mock := &mockGeoProvider{score: geopolitical.GeopoliticalRiskScore{Intensity: 10}}
	calc := NewTaiwanStressCalculator(mock, "")
	calc.cacheTTL = 100 * time.Millisecond

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: 1},
		US10Y:              marketdata.MacroDataPoint{Value: 1},
		VIX:                marketdata.MacroDataPoint{Value: 10},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0},
		RecordedAt:         1713000000,
	}

	ctx := context.Background()
	if _, err := calc.CalculateFromSnapshot(ctx, snap, marketdata.MacroDataSnapshot{}); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 provider call after first invocation, got %d", mock.calls)
	}

	if _, err := calc.CalculateFromSnapshot(ctx, snap, marketdata.MacroDataSnapshot{}); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected cached result, but provider was called again (calls=%d)", mock.calls)
	}

	time.Sleep(150 * time.Millisecond)
	if _, err := calc.CalculateFromSnapshot(ctx, snap, marketdata.MacroDataSnapshot{}); err != nil {
		t.Fatalf("third call after ttl failed: %v", err)
	}
	if mock.calls != 2 {
		t.Fatalf("expected provider to be called after ttl expired, got %d calls", mock.calls)
	}
}

func TestTaiwanStressCalculator_RegimeThresholds(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")

	tests := []struct {
		name string
		snap marketdata.MacroDataSnapshot
		geo  float64
		want string
	}{
		{
			name: "low",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 8.5}, US10Y: marketdata.MacroDataPoint{Value: 0}, VIX: marketdata.MacroDataPoint{Value: 0}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
			geo:  10,
			want: "low",
		},
		{
			name: "alert",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 45}, VIX: marketdata.MacroDataPoint{Value: 15}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -2}, Oil: marketdata.MacroDataPoint{ChangePct: 5}, Gold: marketdata.MacroDataPoint{ChangePct: 3}},
			geo:  30,
			want: "alert",
		},
		{
			name: "high",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 60}, VIX: marketdata.MacroDataPoint{Value: 35}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -6}, Oil: marketdata.MacroDataPoint{ChangePct: 10}, Gold: marketdata.MacroDataPoint{ChangePct: 5}},
			geo:  60,
			want: "high",
		},
		{
			name: "crisis",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 80}, VIX: marketdata.MacroDataPoint{Value: 80}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -10}},
			geo:  100,
			want: "crisis",
		},
	}

	for _, tt := range tests {
		geo := geopolitical.GeopoliticalRiskScore{Intensity: tt.geo}
		idx := calc.Calculate(tt.snap, marketdata.MacroDataSnapshot{}, geo)
		if idx.Regime != tt.want {
			t.Fatalf("%s: expected regime %q, got %q (score=%.1f)", tt.name, tt.want, idx.Regime, idx.Score)
		}
	}
}

func TestLoadWeightsConfigFromParameters(t *testing.T) {
	cfg := LoadWeightsConfig("")
	if cfg == nil {
		t.Fatal("expected non-nil config from parameters system")
	}
	if !cfg.IsValid() {
		t.Fatal("expected valid config (weights sum to 1.0)")
	}
	expected := DefaultCalibrationWeights()
	if cfg.Weights != expected {
		t.Fatalf("weights mismatch: got %+v, want %+v", cfg.Weights, expected)
	}
}

func TestLoadWeightsConfigFileNotFound(t *testing.T) {
	cfg := LoadWeightsConfig("/nonexistent/dir")
	if cfg == nil {
		t.Fatal("expected non-nil config from parameters system even when workDir doesn't exist")
	}
}

func TestLoadWeightsConfigIntegratesWithCalculator(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")
	snap := marketdata.MacroDataSnapshot{
		DXY:   marketdata.MacroDataPoint{Value: 104, ChangePct: 1.0},
		US10Y: marketdata.MacroDataPoint{Value: 4.5},
		VIX:   marketdata.MacroDataPoint{Value: 20},
	}
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 30}
	idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, geo)
	if idx.Score < 0 {
		t.Fatal("expected positive score with parameters config")
	}
}

func TestGetCurrentStressIndex(t *testing.T) {
	eng := NewNarrativeEngine()

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		Oil:                marketdata.MacroDataPoint{ChangePct: 1.5},
		Gold:               marketdata.MacroDataPoint{ChangePct: 0.8},
		JPY:                marketdata.MacroDataPoint{ChangePct: -0.3},
		RecordedAt:         time.Now().Unix(),
	}
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 30}
	eng.UpdateMacro(snap, geo)

	idx := eng.GetCurrentStressIndex()

	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range [0,100]: got %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	expectedKeys := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	for _, k := range expectedKeys {
		if _, ok := idx.Components[k]; !ok {
			t.Fatalf("missing component %s", k)
		}
	}
	if idx.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestGetStressIndexHistory(t *testing.T) {
	eng := NewNarrativeEngine()
	baseTime := time.Now().Unix()

	for i := range int64(3) {
		snap := marketdata.MacroDataSnapshot{
			DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: float64(i) * 0.5},
			US10Y:              marketdata.MacroDataPoint{Value: 4.5 + float64(i)*0.1},
			VIX:                marketdata.MacroDataPoint{Value: 20 + float64(i)*2},
			ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5 - float64(i)},
			RecordedAt:         baseTime + i,
		}
		eng.UpdateMacro(snap, geopolitical.GeopoliticalRiskScore{Intensity: 30})
		eng.RecordStressIndex(eng.GetCurrentStressIndex())
	}

	t.Run("returns exact limit", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(2)
		if len(hist) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(hist))
		}
	})

	t.Run("returns min of history and limit", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(10)
		if len(hist) != 3 {
			t.Fatalf("expected 3 entries (history has 3), got %d", len(hist))
		}
	})

	t.Run("limit zero defaults to 30", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(0)
		if len(hist) != 3 {
			t.Fatalf("expected 3 entries (default 30, history has 3), got %d", len(hist))
		}
	})

	t.Run("negative limit defaults to 30", func(t *testing.T) {
		hist := eng.GetStressIndexHistory(-1)
		if len(hist) != 3 {
			t.Fatalf("expected 3 entries (default 30 for negative, history has 3), got %d", len(hist))
		}
	})

	t.Run("empty history returns empty slice", func(t *testing.T) {
		eng2 := NewNarrativeEngine()
		hist := eng2.GetStressIndexHistory(10)
		if len(hist) != 0 {
			t.Fatalf("expected 0 entries for empty history, got %d", len(hist))
		}
	})
}

func TestGetStressIndexThresholds(t *testing.T) {
	eng := NewNarrativeEngine()
	th := eng.GetStressIndexThresholds()

	if th.Crisis <= th.High {
		t.Fatalf("expected Crisis > High, got Crisis=%v High=%v", th.Crisis, th.High)
	}
	if th.High <= th.Alert {
		t.Fatalf("expected High > Alert, got High=%v Alert=%v", th.High, th.Alert)
	}
	if th.Alert <= 0 {
		t.Fatalf("expected Alert > 0, got Alert=%v", th.Alert)
	}
	if th.Crisis == 0 || th.High == 0 || th.Alert == 0 {
		t.Fatal("expected non-zero threshold values")
	}

	t.Run("nil stressCalc returns empty struct", func(t *testing.T) {
		eng2 := NewNarrativeEngine()
		eng2.stressCalc = nil
		th := eng2.GetStressIndexThresholds()
		if th.Crisis != 0 || th.High != 0 || th.Alert != 0 {
			t.Fatalf("expected zero thresholds for nil stressCalc, got %+v", th)
		}
	})
}

func TestCalculateFromSnapshotWithStore_FallbackToPersistedGeo(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")

	dir := t.TempDir()
	store := geopolitical.NewGeopoliticalStore(dir)
	if err := store.Save(geopolitical.GeopoliticalRiskScore{
		Region:    "Global",
		Intensity: 30,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("save geo score: %v", err)
	}

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         time.Now().Unix(),
	}
	ctx := context.Background()
	idx, err := calc.CalculateFromSnapshotWithStore(ctx, snap, marketdata.MacroDataSnapshot{}, store)
	if err != nil {
		t.Fatalf("CalculateFromSnapshotWithStore: %v", err)
	}
	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range: %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	if _, ok := idx.Components["geopolitical"]; !ok {
		t.Fatal("expected geopolitical component from persisted store")
	}
}

func TestCalculateFromSnapshotWithStore_NoProviderNoStoreFallsBack(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil, "")
	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         time.Now().Unix(),
	}
	idx, err := calc.CalculateFromSnapshotWithStore(context.Background(), snap, marketdata.MacroDataSnapshot{}, nil)
	if err != nil {
		t.Fatalf("expected partial index when geo provider and store are both nil, got error: %v", err)
	}
	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range: %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	if v, ok := idx.Components["geopolitical"]; !ok {
		t.Fatal("expected geopolitical component (set to 0)")
	} else if v != 0 {
		t.Fatalf("expected geopolitical=0, got %v", v)
	}
}

func TestNewTaiwanStressCalculator_AutoLoadsBaselines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bl := &BaselineConfig{
		Window: 30,
		Baselines: map[string]*FactorBaseline{
			"dxy": {Factor: "dxy", Mean: 0.0, StdDev: 0.5, Count: 30},
		},
	}
	if err := SaveBaselines(dir, bl); err != nil {
		t.Fatalf("SaveBaselines: %v", err)
	}

	calc := NewTaiwanStressCalculator(nil, dir)
	if calc.baselines == nil {
		t.Fatal("expected baselines to be auto-loaded from workDir, got nil")
	}
	if calc.signalStrategy != SignalHybrid {
		t.Errorf("expected SignalHybrid after auto-load, got %d", calc.signalStrategy)
	}
	if !calc.useHybridSignal() {
		t.Error("expected useHybridSignal() to be true after auto-load")
	}
}

func TestNewTaiwanStressCalculator_FallsBackWithoutBaselines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	calc := NewTaiwanStressCalculator(nil, dir)
	if calc.baselines != nil {
		t.Errorf("expected nil baselines when no file exists, got %+v", calc.baselines)
	}
	if calc.useHybridSignal() {
		t.Error("expected useHybridSignal() to be false on first run (no baselines file)")
	}
}

func TestNewTaiwanStressCalculator_EmptyWorkDirNoHybrid(t *testing.T) {
	t.Parallel()

	calc := NewTaiwanStressCalculator(nil, "")
	if calc.baselines != nil {
		t.Error("expected nil baselines for empty workDir")
	}
	if calc.useHybridSignal() {
		t.Error("expected useHybridSignal() to be false for empty workDir")
	}
}

func TestCalculate_US10YHybridSignal_OnFlatDay(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bl := &BaselineConfig{
		Window: 30,
		Baselines: map[string]*FactorBaseline{
			"us10y": {Factor: "us10y", Mean: 4.0, StdDev: 0.3, Count: 30},
		},
	}
	if err := SaveBaselines(dir, bl); err != nil {
		t.Fatalf("SaveBaselines: %v", err)
	}

	calc := NewTaiwanStressCalculator(nil, dir)
	if !calc.useHybridSignal() {
		t.Fatal("expected hybrid signal to be enabled after auto-load")
	}

	// US10Y flat day: ChangePct=0, but value=4.8 deviates from baseline mean 4.0.
	// z-score = (4.8 - 4.0) / 0.3 = 2.67. Hybrid path should pick this up
	// even though ChangePct=0.
	snap := marketdata.MacroDataSnapshot{
		US10Y:      marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.8, ChangePct: 0},
		RecordedAt: 1713000000,
	}
	idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, geopolitical.GeopoliticalRiskScore{})

	us10yComponent, ok := idx.Components["us10y"]
	if !ok {
		t.Fatal("expected us10y component in result")
	}
	if us10yComponent <= 0 {
		t.Fatalf("expected non-zero us10y component on flat day with high level deviation, got %v", us10yComponent)
	}
}

func TestCalculate_US10YHybridSignal_CompareWithLegacy(t *testing.T) {
	t.Parallel()

	// Legacy constructor (empty workDir) uses raw yield * scale.
	legacyCalc := NewTaiwanStressCalculator(nil, "")

	// Hybrid constructor (with baselines) uses z-score for level signal.
	dir := t.TempDir()
	bl := &BaselineConfig{
		Window: 30,
		Baselines: map[string]*FactorBaseline{
			"us10y": {Factor: "us10y", Mean: 4.0, StdDev: 0.3, Count: 30},
		},
	}
	if err := SaveBaselines(dir, bl); err != nil {
		t.Fatalf("SaveBaselines: %v", err)
	}
	hybridCalc := NewTaiwanStressCalculator(nil, dir)

	// Flat day (ChangePct=0): legacy uses raw Value*scale, hybrid uses |level_dev|*scale.
	// Both should give non-zero for US10Y=4.8 vs baseline mean 4.0.
	snap := marketdata.MacroDataSnapshot{
		US10Y:      marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.8, ChangePct: 0},
		RecordedAt: 1713000000,
	}
	legacyIdx := legacyCalc.Calculate(snap, marketdata.MacroDataSnapshot{}, geopolitical.GeopoliticalRiskScore{})
	hybridIdx := hybridCalc.Calculate(snap, marketdata.MacroDataSnapshot{}, geopolitical.GeopoliticalRiskScore{})

	legacyUS10Y := legacyIdx.Components["us10y"]
	hybridUS10Y := hybridIdx.Components["us10y"]

	if legacyUS10Y <= 0 {
		t.Fatalf("legacy US10Y should be non-zero (raw yield 4.8 * scale), got %v", legacyUS10Y)
	}
	if hybridUS10Y <= 0 {
		t.Fatalf("hybrid US10Y should be non-zero (z-score), got %v", hybridUS10Y)
	}
	t.Logf("legacy=%.4f hybrid=%.4f (both non-zero confirms US10Y coverage)", legacyUS10Y, hybridUS10Y)
}

// TestGeoIntensityFromStressComponent covers the component↔GeoIntensity mapping
// (2b): stress component = intensity * 0.13, so 4.29 ≈ intensity 33, 5.2 ≈ 40.
func TestGeoIntensityFromStressComponent(t *testing.T) {
	tests := []struct {
		component float64
		want      float64
	}{
		{0, 0},
		{4.29, 33.0}, // hermes 案例：4.29 → intensity ≈ 33（4 級制升溫邊界內）
		{5.2, 40.0},  // intensity 40 對應的元件值
		{7.8, 60.0},  // intensity 60 對應的元件值（黑天鵝門檻）
	}
	for _, tt := range tests {
		got := GeoIntensityFromStressComponent(tt.component)
		if diff := math.Abs(got - tt.want); diff > 0.01 {
			t.Errorf("GeoIntensityFromStressComponent(%v) = %v, want ~%v", tt.component, got, tt.want)
		}
	}
}
