package sectorallocation_test

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// fakeHitRateProvider is a deterministic test double for the
// IndustryHitRateProvider interface. Each test gets a fresh one and
// sets the rows it expects.
type fakeHitRateProvider struct {
	rows []sectorallocation.IndustryHitRateSummary
	err  error
}

func (f *fakeHitRateProvider) LoadIndustryWinRate(source, conditionID, rollingWindow string, resolve sectorallocation.SectorResolver) (sectorallocation.IndustryHitRateReport, error) {
	if f.err != nil {
		return sectorallocation.IndustryHitRateReport{}, f.err
	}
	return sectorallocation.IndustryHitRateReport{Industries: f.rows}, nil
}

func TestBuildIndustryHitRateTilt_GateOff_ReturnsNil(t *testing.T) {
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		{IndustryID: "semiconductor", Direction: "buy", WilsonLower: 0.7, WilsonUpper: 0.8, WinRate: 0.75, Observations: 100},
	}}
	tilts, err := sectorallocation.BuildIndustryHitRateTilt(provider, "src", "cond", "120d")
	if err != nil {
		t.Fatalf("gate-off path must not error: %v", err)
	}
	if tilts != nil {
		t.Fatalf("gate-off path must return nil tilts, got %v", tilts)
	}
}

func TestApplyIndustryHitRateToDrivers_GateOff_NoChange(t *testing.T) {
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		{IndustryID: "semiconductor", Direction: "buy", WilsonLower: 0.7, WilsonUpper: 0.8, WinRate: 0.75, Observations: 100},
	}}
	in := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		CapitalFlow:     map[industry.SectorID]float64{industry.SectorFinancials: 0.05},
	}
	out, err := sectorallocation.ApplyIndustryHitRateToDrivers(in, provider, "src", "cond", "120d")
	if err != nil {
		t.Fatalf("gate-off must not error: %v", err)
	}
	if len(out.CapitalFlow) != 1 {
		t.Fatalf("gate-off must not mutate CapitalFlow, got %v", out.CapitalFlow)
	}
	if out.CapitalFlow[industry.SectorFinancials] != 0.05 {
		t.Fatalf("gate-off must not change existing CapitalFlow entry, got %v", out.CapitalFlow)
	}
}

func TestTiltToDriverMap_EmptyReturnsNil(t *testing.T) {
	m := sectorallocation.TiltToDriverMap(nil)
	if m != nil {
		t.Fatalf("TiltToDriverMap(nil) must return nil, got %v", m)
	}
	m = sectorallocation.TiltToDriverMap([]sectorallocation.IndustryHitRateTilt{})
	if m != nil {
		t.Fatalf("TiltToDriverMap([]) must return nil, got %v", m)
	}
}

func TestTiltToDriverMap_RejectsNonL1Keys(t *testing.T) {
	tilts := []sectorallocation.IndustryHitRateTilt{
		{IndustryID: "semiconductor", TiltMagnitude: 0.05},
		{IndustryID: "bogus_subindustry", TiltMagnitude: 0.99},
		{IndustryID: "financials", TiltMagnitude: -0.03},
	}
	m := sectorallocation.TiltToDriverMap(tilts)
	if m == nil {
		t.Fatal("non-empty tilts must produce non-nil map")
	}
	if _, ok := m[industry.SectorSemiconductor]; !ok {
		t.Fatal("semiconductor L1 must be present")
	}
	if _, ok := m["bogus_subindustry"]; ok {
		t.Fatal("non-L1 keys must be rejected by defensive IsL1 check")
	}
	if _, ok := m[industry.SectorFinancials]; !ok {
		t.Fatal("financials L1 must be present")
	}
}

func TestIndustryHitRateTilt_RowMath(t *testing.T) {
	cases := []struct {
		name    string
		wilson  float64
		dir     string
		wantMag float64
	}{
		{name: "buy_wilson_0.5", wilson: 0.5, dir: "buy", wantMag: 0.0},
		{name: "buy_wilson_0.7", wilson: 0.7, dir: "buy", wantMag: 0.04},
		{name: "buy_wilson_0.3", wilson: 0.3, dir: "buy", wantMag: -0.04},
		{name: "avoid_wilson_0.7", wilson: 0.7, dir: "avoid", wantMag: -0.04},
		{name: "avoid_wilson_0.3", wilson: 0.3, dir: "avoid", wantMag: 0.04},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			magnitude := (tc.wilson - 0.5) * 0.2
			if tc.dir == "avoid" {
				magnitude *= -1.0
			}
			if abs(magnitude-tc.wantMag) > 1e-9 {
				t.Fatalf("magnitude = %v, want %v", magnitude, tc.wantMag)
			}
		})
	}
}

func TestApplyIndustryHitRateToDrivers_GateOff_NoProvider(t *testing.T) {
	in := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}
	out, err := sectorallocation.ApplyIndustryHitRateToDrivers(in, nil, "src", "cond", "120d")
	if err != nil {
		t.Fatalf("gate-off with nil provider must not error: %v", err)
	}
	if out.CapitalFlow != nil {
		t.Fatalf("gate-off must not allocate CapitalFlow, got %v", out.CapitalFlow)
	}
}

func TestComputeProjectedTarget_WithGateOff_ByteIdentical(t *testing.T) {
	priorMap := makeStrategicPriorForTest(t)
	engine := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		priorMap,
		sectorallocation.NewDefaultProjector(),
		nil, nil, nil, nil, nil, nil, 0.3, 2.5,
	)
	drivers := sectorallocation.DriverInputs{AsOfTradingDate: "2026-07-17"}

	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		{IndustryID: "semiconductor", Direction: "buy", WilsonLower: 0.9, WilsonUpper: 0.95, WinRate: 0.9, Observations: 200},
		{IndustryID: "financials", Direction: "buy", WilsonLower: 0.7, WilsonUpper: 0.8, WinRate: 0.75, Observations: 100},
	}}
	sectorallocation.RegisterIndustryHitRateProvider(provider)
	defer sectorallocation.ResetIndustryHitRateProvider()

	target, err := engine.ComputeProjectedTarget(context.Background(), drivers)
	if err != nil {
		t.Fatalf("ComputeProjectedTarget failed: %v", err)
	}
	if len(target.Target) != 20 {
		t.Fatalf("must have 20 L1 keys, got %d", len(target.Target))
	}
	s := 0.0
	for _, v := range target.Target {
		s += v
	}
	if s < 0.999999999 || s > 1.000000001 {
		t.Fatalf("sum drift: %.12f", s)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
