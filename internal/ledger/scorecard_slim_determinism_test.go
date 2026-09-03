package ledger

import (
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestBuildScorecards_DeterministicAcrossRuns guards the B4 fix (#1780 Phase
// 1 review): RollingSharpeTrend and MaxDrawdown are computed from the
// per-window daily-mean slice, which was previously built by ranging over the
// windows MAP (random iteration order) — the same input could yield a
// different trend/drawdown on every run. Window keys are now sorted first, so
// repeated builds must be bit-identical (modulo LastUpdatedAt).
func TestBuildScorecards_DeterministicAcrossRuns(t *testing.T) {
	outcomes := scorecardDeterminismFixture()
	first := BuildScorecards(outcomes)
	zeroUpdated(first)
	for i := 0; i < 50; i++ {
		got := BuildScorecards(outcomes)
		zeroUpdated(got)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run #%d diverged from first build (map-iteration nondeterminism):\nfirst=%+v\ngot  =%+v", i, first, got)
		}
	}
}

// TestBuildScorecards_EqualSharpeTiebreakDeterministic guards the B4 tie
// fix: the scorecard sort previously returned 0 for equal SharpeLike, so the
// output order (and the observatory's top-limit cut) depended on the random
// byAgent map iteration. The AgentID tiebreak must pin the order.
func TestBuildScorecards_EqualSharpeTiebreakDeterministic(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Identical return series → identical SharpeLike for both agents.
	series := []float64{0.01, 0.02, 0.015, -0.01, 0.03, 0.005, 0.02, 0.01, -0.005, 0.025}
	var outcomes []domain.RecommendationOutcome
	for _, agent := range []string{"zeta-agent", "alpha-agent"} {
		for i, r := range series {
			outcomes = append(outcomes, domain.RecommendationOutcome{
				AgentID:       agent,
				Skill:         "tie",
				Window:        "2026-06-01",
				ForwardReturn: r,
				Hit:           r > 0,
				RecordedAt:    now.Add(time.Duration(i) * time.Minute),
			})
		}
	}
	scs := BuildScorecards(outcomes)
	if len(scs) != 2 {
		t.Fatalf("expected 2 scorecards, got %d", len(scs))
	}
	if scs[0].SharpeLike != scs[1].SharpeLike {
		t.Fatalf("fixture bug: expected equal SharpeLike, got %v vs %v", scs[0].SharpeLike, scs[1].SharpeLike)
	}
	if scs[0].AgentID != "alpha-agent" || scs[1].AgentID != "zeta-agent" {
		t.Errorf("tie order not deterministic by AgentID: %s, %s", scs[0].AgentID, scs[1].AgentID)
	}
	for i := 0; i < 20; i++ {
		again := BuildScorecards(outcomes)
		if again[0].AgentID != "alpha-agent" || again[1].AgentID != "zeta-agent" {
			t.Fatalf("run #%d: tie order flipped", i)
		}
	}
}

func zeroUpdated(scs []domain.Scorecard) {
	for i := range scs {
		scs[i].LastUpdatedAt = time.Time{}
	}
}

func scorecardDeterminismFixture() []domain.RecommendationOutcome {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var outcomes []domain.RecommendationOutcome
	// Per-window means rise with the window index so a random daily order
	// would (with overwhelming probability) change RollingSharpeTrend /
	// MaxDrawdown — exactly the B4 nondeterminism being guarded.
	windowReturns := map[string][]float64{
		"2026-06-01": {0.001, 0.002, 0.0015},
		"2026-06-02": {0.005, 0.006, 0.0055},
		"2026-06-03": {0.01, 0.012, 0.011},
		"2026-06-04": {0.02, 0.018, 0.019},
		"2026-06-05": {0.03, 0.028, 0.029},
	}
	day := 0
	for w, rets := range windowReturns {
		for i, r := range rets {
			outcomes = append(outcomes, domain.RecommendationOutcome{
				AgentID:       "det-agent",
				Skill:         "determinism",
				Window:        w,
				ForwardReturn: r,
				Hit:           r > 0,
				RecordedAt:    now.Add(time.Duration(day*24)*time.Hour + time.Duration(i)*time.Minute),
			})
		}
		day++
	}
	return outcomes
}
