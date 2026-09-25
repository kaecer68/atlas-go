package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/orchestrator/composition"
)

// Issue #1944 Batch 3 I12/I13. Before this change DashboardAPI.SetCompositionRoot
// had zero callers and injected the dashboard's legacy WeightEngine into the
// composition root — which StrategyEvolver.ApplySectorRotation cannot use, because
// that engine has no Projector and ComputeProjectedTarget rejects it. The
// simulation path therefore kept its own config-seeded CycleTracker and a
// hardcoded 0.0 macro driver. These tests pin the replacement: the root receives
// the dashboard's data-fed driver adapters.

func newTestDashboardWithIndustryState() (*DashboardAPI, *service.IndustryService) {
	svc := &service.IndustryService{
		CycleTracker:    industry.NewCycleTracker(),
		SeasonalEngine:  industry.NewSeasonalEngine(),
		LinkageAnalyzer: industry.NewLinkageAnalyzer(),
	}
	return &DashboardAPI{industryService: svc}, svc
}

func TestSetCompositionRoot_SharesDashboardIndustryState(t *testing.T) {
	api, svc := newTestDashboardWithIndustryState()
	root, err := composition.NewRoot(config.Config{})
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	// Wire a macro modulator so the macro driver has real dashboard state to
	// derive from (DXY +10% ⇒ exporter penalty ⇒ modulation 0.95 for
	// semiconductor, i.e. a tilt of -0.05, not the previous hardcoded 0.0).
	baseline := marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: 100}}
	current := marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: 110}}
	svc.SeasonalEngine.SetDynamicEnv(industry.NewDynamicEnvModulator(baseline, current))

	api.SetCompositionRoot(root)

	shared := root.SharedSectorInputs()
	if shared.Cycle == nil || shared.Seasonal == nil || shared.Linkage == nil {
		t.Fatalf("cycle/seasonal/linkage adapters not shared: %+v", shared)
	}
	if shared.Macro == nil {
		t.Fatalf("macro adapter not shared: %+v", shared)
	}

	// I13: the shared cycle adapter must read the DASHBOARD's tracker. Updating
	// that tracker after wiring must move the multiplier the root exposes.
	ctx := context.Background()
	before, err := shared.Cycle.GetCycleMultiplier(ctx, "semiconductor")
	if err != nil {
		t.Fatalf("cycle multiplier: %v", err)
	}
	svc.CycleTracker.UpdatePosition("semiconductor", industry.IndustryMetrics{RevenueGrowthYoY: 0.40})
	after, err := shared.Cycle.GetCycleMultiplier(ctx, "semiconductor")
	if err != nil {
		t.Fatalf("cycle multiplier after update: %v", err)
	}
	if after == before {
		t.Fatalf("shared cycle adapter did not observe the dashboard tracker update (before=%v after=%v)", before, after)
	}

	// I12: the macro driver must no longer be the hardcoded 0.0 stub; it must be
	// the dashboard modulator's tilt.
	tilt, err := shared.Macro.GetMacroTilt(ctx, "semiconductor", "", "")
	if err != nil {
		t.Fatalf("macro tilt: %v", err)
	}
	wantTilt := svc.SeasonalEngine.DynamicEnvModulator().SeasonalModulation("semiconductor") - 1.0
	if diff := tilt - wantTilt; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("macro tilt = %v, want dashboard modulator tilt %v", tilt, wantTilt)
	}
	if tilt == 0.0 {
		t.Fatalf("macro tilt is still the hardcoded-neutral value; the dashboard modulator was ignored")
	}
}

func TestSetCompositionRoot_NoModulatorKeepsNeutralMacroDriver(t *testing.T) {
	api, _ := newTestDashboardWithIndustryState()
	root, err := composition.NewRoot(config.Config{})
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	api.SetCompositionRoot(root)

	if got := root.SharedSectorInputs().Macro; got != nil {
		t.Fatalf("macro adapter must stay nil without a dashboard modulator, got %#v", got)
	}
}

func TestSetCompositionRoot_NilInputsAreNoOps(t *testing.T) {
	root, err := composition.NewRoot(config.Config{})
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	// No industry service wired → nothing shared, no panic.
	(&DashboardAPI{}).SetCompositionRoot(root)
	if got := root.SharedSectorInputs(); got.Cycle != nil || got.Macro != nil {
		t.Fatalf("expected no shared inputs, got %+v", got)
	}
	// Nil root → no panic.
	api, _ := newTestDashboardWithIndustryState()
	api.SetCompositionRoot(nil)
	_ = time.Now()
}
