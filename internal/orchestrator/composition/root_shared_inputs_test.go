package composition

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// Issue #1944 Batch 3 I12/I13: the composition root used to build its own
// config-seeded industry.CycleTracker for the cycle driver and to answer a
// hardcoded 0.0 for the macro driver, even though the monitoring dashboard
// already runs a data-fed tracker and a DynamicEnvModulator. These tests pin the
// shared-input wiring: when the dashboard installs its adapters, the engine built
// by buildWeightEngine must consume THOSE instances.

func TestBuildWeightEngine_UsesSharedSectorInputs(t *testing.T) {
	cfg := config.Config{}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	shared := industry.NewCycleTracker()
	shared.UpdatePosition("semiconductor", industry.IndustryMetrics{RevenueGrowthYoY: 0.40})
	cycleAdapter := sectorallocation.NewCycleAdapter(shared)

	want, err := cycleAdapter.GetCycleMultiplier(context.Background(), "semiconductor")
	if err != nil {
		t.Fatalf("shared cycle adapter: %v", err)
	}
	if want == 1.0 {
		t.Fatalf("test fixture is not discriminating: shared tracker yields neutral multiplier")
	}

	root.WithSharedSectorInputs(SharedSectorInputs{Cycle: cycleAdapter})
	if got := root.SharedSectorInputs().Cycle; got == nil {
		t.Fatal("SharedSectorInputs().Cycle is nil after WithSharedSectorInputs")
	}

	eng := root.buildWeightEngine()
	if eng == nil {
		t.Fatal("buildWeightEngine returned nil")
	}
	sw, err := eng.ComputeWeight(context.Background(), "semiconductor", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight: %v", err)
	}
	got, ok := findLogEntry(sw.AdjustmentLog, "cycle")
	if !ok {
		t.Fatalf("no cycle entry in adjustment log: %v", sw.AdjustmentLog)
	}
	// The engine records log entries with %.4f precision, so compare at that
	// resolution (1e-4) rather than bit-exactly.
	if diff := got - want; diff > 1e-4 || diff < -1e-4 {
		t.Fatalf("cycle multiplier = %v, want the shared tracker's %v (engine kept its own tracker?)", got, want)
	}
}

func TestBuildWeightEngine_WithoutSharedInputsKeepsFallback(t *testing.T) {
	cfg := config.Config{}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	if got := root.SharedSectorInputs(); got.Cycle != nil || got.Seasonal != nil || got.Linkage != nil || got.Macro != nil {
		t.Fatalf("fresh root must have no shared inputs, got %+v", got)
	}
	if root.buildWeightEngine() == nil {
		t.Fatal("buildWeightEngine must still work without shared inputs (test/CLI fallback)")
	}
	if SectorFactorDriverWired {
		t.Fatal("SectorFactorDriverWired flipped to true — update the factor adapter and docs/reference/inert-registry.md")
	}
}
