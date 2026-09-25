package ledger

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Issue #1944 Batch 4, item I24: BuildScorecards aggregated every row,
// including rows carrying the deterministic placeholder forward return
// (IsSynthetic=true, see orchestrator.syntheticPlaceholderReturn). Production
// recorded 25,571 of 45,668 recommendation_outcomes rows (56.0%) as synthetic,
// so the published HitRate / Sharpe / IS-OOS mixed measurement with a
// placeholder distribution. These tests pin the filter and the disclosure.

func syntheticOutcome(agentID, window string, forwardReturn float64) domain.RecommendationOutcome {
	return domain.RecommendationOutcome{
		AgentID:       agentID,
		Skill:         "alpha",
		Window:        window,
		ForwardReturn: forwardReturn,
		Hit:           forwardReturn > 0,
		IsSynthetic:   true,
		RecordedAt:    time.Now(),
	}
}

func realOutcome(agentID, window string, forwardReturn float64) domain.RecommendationOutcome {
	o := syntheticOutcome(agentID, window, forwardReturn)
	o.IsSynthetic = false
	return o
}

// TestBuildScorecards_ExcludesSyntheticRows is the core I24 assertion: only the
// real rows may reach the statistics, and the excluded count must be disclosed.
func TestBuildScorecards_ExcludesSyntheticRows(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		realOutcome("a", "2026-01-01", 0.10),
		realOutcome("a", "2026-01-02", -0.02),
		syntheticOutcome("a", "2026-01-03", 0.50),
		syntheticOutcome("a", "2026-01-04", 0.50),
		syntheticOutcome("a", "2026-01-05", 0.50),
	}

	scorecards := BuildScorecards(outcomes)
	if len(scorecards) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(scorecards))
	}
	sc := scorecards[0]

	if sc.Observations != 2 {
		t.Fatalf("Observations = %d, want 2 (real rows only)", sc.Observations)
	}
	if sc.SyntheticObservations != 3 {
		t.Fatalf("SyntheticObservations = %d, want 3", sc.SyntheticObservations)
	}
	if sc.WindowCount != 2 {
		t.Fatalf("WindowCount = %d, want 2 (synthetic windows must not count)", sc.WindowCount)
	}
	// Real rows: one hit, one miss => 0.5. The three placeholder rows would
	// have pushed this to 0.8.
	if math.Abs(sc.HitRate-0.5) > 1e-9 {
		t.Fatalf("HitRate = %v, want 0.5 (real rows only; polluted value would be 0.8)", sc.HitRate)
	}
	if math.Abs(sc.AverageReturn-0.04) > 1e-9 {
		t.Fatalf("AverageReturn = %v, want 0.04 (mean of 0.10, -0.02)", sc.AverageReturn)
	}
	if math.Abs(sc.SyntheticShare-0.6) > 1e-9 {
		t.Fatalf("SyntheticShare = %v, want 0.6 (3 of 5 rows synthetic)", sc.SyntheticShare)
	}
}

// TestBuildScorecards_AllSyntheticAgentIsOmitted pins the "unknown, not zero"
// choice: an agent with nothing but placeholder rows has no measurable
// performance, so publishing a card full of zeros would itself be a false claim.
func TestBuildScorecards_AllSyntheticAgentIsOmitted(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		realOutcome("measured", "2026-01-01", 0.03),
		syntheticOutcome("placeholder-only", "2026-01-01", 0.20),
		syntheticOutcome("placeholder-only", "2026-01-02", -0.20),
	}

	scorecards := BuildScorecards(outcomes)
	if len(scorecards) != 1 {
		t.Fatalf("expected only the measured agent, got %d scorecards", len(scorecards))
	}
	if scorecards[0].AgentID != "measured" {
		t.Fatalf("AgentID = %q, want %q", scorecards[0].AgentID, "measured")
	}
	for _, sc := range scorecards {
		if sc.AgentID == "placeholder-only" {
			t.Fatalf("agent with zero real rows must not receive a scorecard; got %+v", sc)
		}
	}
}

// TestBuildScorecards_SyntheticRowsCannotChangeRealStatistics is the regression
// guard: appending placeholder rows to a fixed real set must leave every
// statistic bit-identical. Before I24 the extra rows moved HitRate, SharpeLike,
// Sharpe windows and the IS/OOS split.
func TestBuildScorecards_SyntheticRowsCannotChangeRealStatistics(t *testing.T) {
	realOnly := []domain.RecommendationOutcome{
		realOutcome("a", "2026-01-01", 0.02),
		realOutcome("a", "2026-01-02", -0.01),
		realOutcome("a", "2026-01-03", 0.03),
		realOutcome("a", "2026-01-04", -0.02),
		realOutcome("a", "2026-01-05", 0.01),
		realOutcome("a", "2026-01-06", 0.04),
		realOutcome("a", "2026-01-07", -0.03),
		realOutcome("a", "2026-01-08", 0.02),
		realOutcome("a", "2026-01-09", 0.01),
		realOutcome("a", "2026-01-10", -0.01),
		realOutcome("a", "2026-01-11", 0.03),
		realOutcome("a", "2026-01-12", 0.02),
	}

	withSynthetic := append([]domain.RecommendationOutcome{}, realOnly...)
	withSynthetic = append(withSynthetic,
		syntheticOutcome("a", "2026-01-13", 0.60),
		syntheticOutcome("a", "2026-01-14", -0.60),
		syntheticOutcome("a", "2026-01-15", 0.55),
	)

	base := BuildScorecards(realOnly)
	mixed := BuildScorecards(withSynthetic)
	if len(base) != 1 || len(mixed) != 1 {
		t.Fatalf("expected one scorecard on each side, got %d / %d", len(base), len(mixed))
	}

	b, m := base[0], mixed[0]
	if b.Observations != m.Observations || b.WindowCount != m.WindowCount {
		t.Fatalf("counts changed: obs %d→%d windows %d→%d",
			b.Observations, m.Observations, b.WindowCount, m.WindowCount)
	}
	for name, pair := range map[string][2]float64{
		"HitRate":            {b.HitRate, m.HitRate},
		"AverageReturn":      {b.AverageReturn, m.AverageReturn},
		"SharpeLike":         {b.SharpeLike, m.SharpeLike},
		"MaxDrawdown":        {b.MaxDrawdown, m.MaxDrawdown},
		"TStat":              {b.TStat, m.TStat},
		"HitRateTStat":       {b.HitRateTStat, m.HitRateTStat},
		"IsSharpe":           {b.IsSharpe, m.IsSharpe},
		"OosSharpe":          {b.OosSharpe, m.OosSharpe},
		"IsOosRatio":         {b.IsOosRatio, m.IsOosRatio},
		"RollingSharpeTrend": {b.RollingSharpeTrend, m.RollingSharpeTrend},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s moved when synthetic rows were appended: %v → %v", name, pair[0], pair[1])
		}
	}
	if m.SyntheticObservations != 3 {
		t.Fatalf("SyntheticObservations = %d, want 3", m.SyntheticObservations)
	}
}

// TestBuildScorecards_RealOnlyAgentDisclosesZeroShare keeps the outward
// disclosure honest in the clean case: nothing dropped must read as 0.0, not as
// a missing/unknown value.
func TestBuildScorecards_RealOnlyAgentDisclosesZeroShare(t *testing.T) {
	scs := BuildScorecards([]domain.RecommendationOutcome{
		realOutcome("a", "2026-01-01", 0.01),
		realOutcome("a", "2026-01-02", -0.01),
	})
	if len(scs) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(scs))
	}
	if scs[0].SyntheticObservations != 0 || scs[0].SyntheticShare != 0 {
		t.Fatalf("real-only scorecard must disclose 0/0, got %d/%v",
			scs[0].SyntheticObservations, scs[0].SyntheticShare)
	}
}

// TestScorecardSyntheticDisclosureJSONContract pins the outward field names the
// observatory / API surface publishes. Renaming them is a breaking change and
// must be done in the same commit as the consumer update.
func TestScorecardSyntheticDisclosureJSONContract(t *testing.T) {
	scs := BuildScorecards([]domain.RecommendationOutcome{
		realOutcome("a", "2026-01-01", 0.01),
		syntheticOutcome("a", "2026-01-02", 0.30),
	})
	if len(scs) != 1 {
		t.Fatalf("expected 1 scorecard, got %d", len(scs))
	}
	raw, err := json.Marshal(scs[0])
	if err != nil {
		t.Fatalf("marshal scorecard: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal scorecard: %v", err)
	}
	if _, ok := decoded["synthetic_observations"]; !ok {
		t.Fatalf("missing json field synthetic_observations; keys=%v", raw)
	}
	if _, ok := decoded["synthetic_share"]; !ok {
		t.Fatalf("missing json field synthetic_share; keys=%v", raw)
	}
	if _, ok := decoded["observations"]; !ok {
		t.Fatalf("legacy json field observations disappeared; keys=%v", raw)
	}
}
