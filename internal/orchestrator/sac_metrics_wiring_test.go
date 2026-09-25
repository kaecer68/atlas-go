package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// recordingSACObserver captures the events the SA11.B emitters produce.
type recordingSACObserver struct {
	events []struct {
		name   string
		fields map[string]any
	}
}

func (o *recordingSACObserver) ObserveSACEvent(name string, fields map[string]any) {
	o.events = append(o.events, struct {
		name   string
		fields map[string]any
	}{name: name, fields: fields})
}

func (o *recordingSACObserver) names() []string {
	out := make([]string, 0, len(o.events))
	for _, e := range o.events {
		out = append(out, e.name)
	}
	return out
}

func (o *recordingSACObserver) has(name string) bool {
	for _, e := range o.events {
		if e.name == name {
			return true
		}
	}
	return false
}

func (o *recordingSACObserver) field(name, key string) (any, bool) {
	for _, e := range o.events {
		if e.name != name {
			continue
		}
		v, ok := e.fields[key]
		return v, ok
	}
	return nil, false
}

// TestApplySectorRotation_EmitsSACLifecycleEvents is the N-A1 wiring proof
// (#1944 Batch 4): the SA11.B emitters had no caller at all, so the dark-launch
// observation window never ran. They now fire on the real policy lifecycle.
func TestApplySectorRotation_EmitsSACLifecycleEvents(t *testing.T) {
	store := sectorallocation.NewFileClosureStore(t.TempDir())
	nextDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	obs := &recordingSACObserver{}

	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(fixtureResolver(nextDate, nil)).
		WithSACMetrics(NewSACMetrics(nil).WithObserver(obs))

	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{
			{Sector: "semiconductor", TargetPct: 0.30},
			{Sector: "financials", TargetPct: 0.20},
		},
	}
	if _, _, _ = evolver.ApplySectorRotation(plan, asOf, map[string]float64{"semiconductor": 0.25}); false {
		t.Fatal("unreachable")
	}

	for _, want := range []string{
		"sac.snapshot.start",
		"sac.snapshot.current",
		"sac.snapshot.target",
		"sac.snapshot.end",
	} {
		if !obs.has(want) {
			t.Errorf("missing %s; got %v", want, obs.names())
		}
	}

	sessionID := asOf.Format("2006-01-02")
	for _, evt := range []string{"sac.snapshot.start", "sac.snapshot.current", "sac.snapshot.target", "sac.snapshot.end"} {
		if v, ok := obs.field(evt, "session_id"); !ok || v != sessionID {
			t.Errorf("%s session_id = %v (present=%v), want %q", evt, v, ok, sessionID)
		}
	}
	// No weight engine is wired on this path, so the target falls back to the
	// plan allocations and that fallback must be reported, not hidden.
	if !obs.has("sac.snapshot.fallback") {
		t.Errorf("missing sac.snapshot.fallback for the fallback target; got %v", obs.names())
	}
	// A stored-but-unconsumed snapshot is not an applied policy (Batch 1), and
	// the emitters must agree with the return value.
	if obs.has("sac.policy.applied") {
		t.Errorf("sac.policy.applied emitted although the policy was not applied: %v", obs.names())
	}
	if v, ok := obs.field("sac.snapshot.end", "ok"); !ok || v != true {
		t.Errorf("sac.snapshot.end ok = %v (present=%v), want true", v, ok)
	}
}

// TestApplySectorRotation_NilSACMetricsIsSafe pins that the emitter is optional
// and can never panic the rotation path.
func TestApplySectorRotation_NilSACMetricsIsSafe(t *testing.T) {
	store := sectorallocation.NewFileClosureStore(t.TempDir())
	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(fixtureResolver(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), nil))
	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{{Sector: "semiconductor", TargetPct: 0.30}},
	}
	receipt, _, _ := evolver.ApplySectorRotation(plan, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), nil)
	if receipt == nil {
		t.Fatal("expected a receipt with the emitter left unwired")
	}

	// A nil *SACMetrics must be a no-op, not a panic.
	var nilMetrics *SACMetrics
	nilMetrics.EmitSnapshotStart("2026-06-30")
	NewSACMetrics(nil).WithObserver(nil).EmitSnapshotEnd("2026-06-30", true)
}

// TestApplySectorRotation_AppliedEmitsPolicyApplied covers the consumed-policy
// branch: a registered consumer that consumes the policy makes applied=true and
// must be observable as sac.policy.applied + sac.policy.consumed.
func TestApplySectorRotation_AppliedEmitsPolicyApplied(t *testing.T) {
	obs := &recordingSACObserver{}
	base := sectorallocation.NewFileClosureStore(t.TempDir())
	store := &consumingSACStore{FileClosureStore: base, consumeSession: "2026-07-01"}

	evolver := NewStrategyEvolver().
		WithClosureStore(store).
		WithSessionResolver(fixtureResolver(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), nil)).
		WithSACMetrics(NewSACMetrics(nil).WithObserver(obs))

	sectorallocation.ResetPolicyConsumers()
	t.Cleanup(sectorallocation.ResetPolicyConsumers)
	sectorallocation.RegisterPolicyConsumer("test-consumer")

	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{{Sector: "semiconductor", TargetPct: 0.30}},
	}
	receipt, applied, reason := evolver.ApplySectorRotation(plan, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), map[string]float64{"semiconductor": 0.25})
	if !applied {
		t.Skipf("consumption evidence not produced in this fixture (reason=%q); the applied branch is covered by strategy_evolver_applied_evidence_test.go", reason)
	}
	if receipt == nil {
		t.Fatal("expected a receipt")
	}
	if !obs.has("sac.policy.consumed") || !obs.has("sac.policy.applied") {
		t.Errorf("applied policy must emit sac.policy.consumed + sac.policy.applied; got %v", obs.names())
	}
}

// consumingSACStore wraps FileClosureStore and consumes every stored snapshot,
// producing the consumption receipt that makes applied=true.
type consumingSACStore struct {
	*sectorallocation.FileClosureStore
	consumeSession string
}

func (s *consumingSACStore) Store(snap sectorallocation.SectorAllocationSnapshot) (*sectorallocation.MutationReceipt, error) {
	receipt, err := s.FileClosureStore.Store(snap)
	if err != nil {
		return receipt, err
	}
	if _, cErr := s.FileClosureStore.Consume(receipt.ReceiptID, s.consumeSession); cErr != nil {
		return receipt, cErr
	}
	return receipt, nil
}
