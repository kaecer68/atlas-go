package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// N-A5 (issue #1944 Batch 3, new finding): ApplySectorRotation — the only
// production caller of WeightEngine.ComputeProjectedTarget — supplies no per-driver
// delta maps. The engine applies each provider only to keys already present in the
// matching map (sectorallocation.collect*Deltas), so no driver adapter is ever
// invoked and the persisted target is the strategic prior re-normalised.
//
// This test pins that fact from both directions:
//   - recording adapters must record zero calls,
//   - the persisted Target must equal the strategic prior.
//
// Supplying driver deltas is a deliberate behaviour change: when someone does it,
// this test fails and forces them to flip SectorDriverDeltasSupplied and update
// docs/reference/inert-registry.md (same convention as Batch 1/2 bug-pin tests).

type recordingDriverAdapters struct {
	calls map[string]int
}

func newRecordingDriverAdapters() *recordingDriverAdapters {
	return &recordingDriverAdapters{calls: map[string]int{}}
}

func (r *recordingDriverAdapters) note(name string) float64 {
	r.calls[name]++
	return 1.0
}

func (r *recordingDriverAdapters) GetCycleMultiplier(context.Context, string) (float64, error) {
	return r.note("cycle"), nil
}
func (r *recordingDriverAdapters) GetSeasonalMultiplier(context.Context, string, time.Time) (float64, error) {
	return r.note("seasonal"), nil
}
func (r *recordingDriverAdapters) GetLinkageMultiplier(context.Context, string) (float64, error) {
	return r.note("linkage"), nil
}
func (r *recordingDriverAdapters) GetNarrativeMultiplier(context.Context, string) (float64, error) {
	return r.note("narrative"), nil
}
func (r *recordingDriverAdapters) GetMacroTilt(context.Context, string, string, string) (float64, error) {
	return r.note("macro") - 1.0, nil
}
func (r *recordingDriverAdapters) GetFactorTilt(context.Context, string) (float64, error) {
	return r.note("factor") - 1.0, nil
}

func TestApplySectorRotation_SuppliesNoDriverDeltas(t *testing.T) {
	if SectorDriverDeltasSupplied {
		t.Fatal("SectorDriverDeltasSupplied flipped to true — update this pin test and docs/reference/inert-registry.md")
	}

	prior := sectorallocation.LoadStrategicPriorFromConfigForTest()
	if prior == nil || len(prior.Weights) != 20 {
		t.Fatalf("strategic prior fixture unusable: %+v", prior)
	}

	rec := newRecordingDriverAdapters()
	eng := sectorallocation.NewDefaultEngineWithProjector(
		sectorallocation.NewEngineTestConfig(),
		prior,
		sectorallocation.NewDefaultProjector(),
		rec, // cycle
		rec, // seasonal
		rec, // linkage
		rec, // narrative
		rec, // macro
		rec, // factor
		0.3,
		2.5,
	)

	dir := t.TempDir()
	store := sectorallocation.NewFileClosureStore(dir)
	nextDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(fixtureResolver(nextDate, nil)).
		WithSectorWeightEngine(eng)

	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{
			{Sector: "semiconductor", TargetPct: 0.35},
		},
	}
	if _, applied, reason := evolver.ApplySectorRotation(plan, asOf, nil); applied {
		t.Fatalf("expected applied=false without consumption evidence (reason=%q)", reason)
	}

	if len(rec.calls) != 0 {
		t.Fatalf("driver providers were invoked from ApplySectorRotation: %v — the path now supplies deltas; update SectorDriverDeltasSupplied and the registry", rec.calls)
	}

	snap, err := store.Latest()
	if err != nil {
		t.Fatalf("latest snapshot: %v", err)
	}
	if snap == nil {
		t.Fatal("no snapshot persisted")
	}
	if snap.TargetNote != "" {
		t.Fatalf("unexpected target note (projection degraded?): %q", snap.TargetNote)
	}
	if len(snap.Target) != 20 {
		t.Fatalf("target has %d sectors, want 20", len(snap.Target))
	}
	for _, id := range sortedSectorIDsForTest(prior.Weights) {
		want := prior.Weights[id]
		got := snap.Target[id]
		if diff := got - want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("target[%s] = %v, want strategic prior %v (projection is prior-only while no driver deltas are supplied)", id, got, want)
		}
	}
}

func sortedSectorIDsForTest(m map[industry.SectorID]float64) []industry.SectorID {
	out := make([]industry.SectorID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
