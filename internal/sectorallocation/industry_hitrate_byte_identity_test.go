package sectorallocation_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ---- gate-off byte-identity proof ----------------------------------------
//
// testdata/production_path_off_baseline.golden.json is the ProjectedTarget JSON
// produced by the PRE-CHANGE revision for the scenario below (same engine
// wiring, uniform 1/20 strategic prior, no driver deltas). It was generated at
// the pre-change revision and reproduces byte-for-byte (sha256
// cf7cc2c81a6d10369849123fcf5f5b01c090f7ba59ffd652e8d1c0e42838c462) at every
// origin/main revision it was checked against: 2a4c9a67, 3c250f91, 0c402539
// and 2504c5ed (the base this branch is merged up to).
//
// The uniform prior is deliberate: the Projector normalizes by summing a Go map,
// so a non-uniform target sums in a random order and the last bits of the
// normalized weights legitimately differ between runs at the base revision too
// (measured: 5 distinct JSON renders in 50 runs with the production prior).
// Uniform weights make the sum order-insensitive, which turns this test into a
// real byte-for-byte comparison instead of a flaky ULP comparison.

const baselineGoldenPath = "testdata/production_path_off_baseline.golden.json"

// uniformPriorForTest returns a strategic prior with an identical weight on
// every canonical L1 sector (the determinism anchor described above).
func uniformPriorForTest(t *testing.T) *sectorallocation.StrategicSectorPrior {
	t.Helper()
	l1 := industry.L1Sectors()
	if len(l1) != 20 {
		t.Fatalf("canonical L1 universe = %d sectors, want 20", len(l1))
	}
	w := 1.0 / float64(len(l1))
	weights := make(map[industry.SectorID]float64, len(l1))
	for _, id := range l1 {
		weights[id] = w
	}
	return &sectorallocation.StrategicSectorPrior{Weights: weights}
}

// runProductionPath executes the production projection entry point
// (ComputeProjectedTarget) with the fixed scenario and returns the compact JSON
// of the result.
func runProductionPath(t *testing.T, gateEnabled bool, provider sectorallocation.IndustryHitRateProvider) []byte {
	t.Helper()
	withHitRateGate(t, gateEnabled)
	sectorallocation.ResetIndustryHitRateProvider()
	t.Cleanup(sectorallocation.ResetIndustryHitRateProvider)
	sectorallocation.RegisterIndustryHitRateProvider(provider)

	engine := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		uniformPriorForTest(t),
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	target, err := engine.ComputeProjectedTarget(context.Background(), sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"})
	if err != nil {
		t.Fatalf("ComputeProjectedTarget: %v", err)
	}
	if len(target.Target) != 20 {
		t.Fatalf("target keys = %d, want 20", len(target.Target))
	}
	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("marshal target: %v", err)
	}
	return data
}

func baselineGolden(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(baselineGoldenPath)
	if err != nil {
		t.Fatalf("read baseline golden (must be committed): %v", err)
	}
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
		data = data[:len(data)-1]
	}
	return data
}

func assertMatchesBaseline(t *testing.T, got []byte) {
	t.Helper()
	want := baselineGolden(t)
	if string(got) != string(want) {
		t.Fatalf("gate-off output is not byte-identical to the pre-change baseline\n got: %s\nwant: %s", got, want)
	}
}

// TestProductionPath_GateOff_MatchesPreChangeBaseline is the primary
// off-path guard: default config, nothing registered, output must equal the
// snapshot taken from the pre-change revision byte-for-byte.
func TestProductionPath_GateOff_MatchesPreChangeBaseline(t *testing.T) {
	assertMatchesBaseline(t, runProductionPath(t, false, nil))
}

// TestProductionPath_GateOff_WithProviderRegistered_MatchesBaseline proves the
// provider registration alone cannot change the production path while the gate
// is off (the registry is populated unconditionally by the composition root).
func TestProductionPath_GateOff_WithProviderRegistered_MatchesBaseline(t *testing.T) {
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		eligibleRow("semiconductor", "buy", 0.95, 400),
		eligibleRow("financials", "avoid", 0.90, 300),
		eligibleRow("shipping", "buy", 0.05, 250),
	}}
	assertMatchesBaseline(t, runProductionPath(t, false, provider))
	if provider.calls != 0 {
		t.Fatalf("gate off must not consult the provider, calls = %d", provider.calls)
	}
}

// TestProductionPath_GateOn_FailClosed_MatchesBaseline proves requirement §2.3
// of the task: when the canonical hit-rate has no calibration-eligible row, the
// gate-on path falls back to the pre-change output instead of tilting on thin
// evidence.
func TestProductionPath_GateOn_FailClosed_MatchesBaseline(t *testing.T) {
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		calibratingRow("semiconductor", 0.95, 3),
		calibratingRow("financials", 0.10, 11),
	}}
	assertMatchesBaseline(t, runProductionPath(t, true, provider))
	if provider.calls != 1 {
		t.Fatalf("gate on must consult the provider once, calls = %d", provider.calls)
	}
}

// TestProductionPath_GateOn_Applied_MovesWeights proves the other half: with
// calibration-eligible evidence the applied weights actually move, in the
// direction the hit-rate implies, and the L1 invariants still hold.
func TestProductionPath_GateOn_Applied_MovesWeights(t *testing.T) {
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		eligibleRow("semiconductor", "buy", 0.7, 200), // +0.04
		eligibleRow("shipping", "buy", 0.3, 200),      // -0.04
	}}
	got := runProductionPath(t, true, provider)

	baseline := baselineGolden(t)
	if string(got) == string(baseline) {
		t.Fatal("gate on with calibration-eligible rows must change the projection")
	}

	var parsed struct {
		Target map[string]float64
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal target: %v", err)
	}
	if len(parsed.Target) != 20 {
		t.Fatalf("target keys = %d, want 20", len(parsed.Target))
	}
	sum := 0.0
	for _, v := range parsed.Target {
		sum += v
	}
	if sum < 1-1e-9 || sum > 1+1e-9 {
		t.Fatalf("target sum = %.12f, want 1 +/- 1e-9", sum)
	}
	base := 1.0 / 20.0
	if parsed.Target["semiconductor"] <= base {
		t.Errorf("semiconductor = %v, want above the uniform base %v (positive evidence)", parsed.Target["semiconductor"], base)
	}
	if parsed.Target["shipping"] >= base {
		t.Errorf("shipping = %v, want below the uniform base %v (negative evidence)", parsed.Target["shipping"], base)
	}
	if parsed.Target["steel"] == 0 {
		t.Errorf("steel must keep its baseline share, got %v", parsed.Target["steel"])
	}
}
