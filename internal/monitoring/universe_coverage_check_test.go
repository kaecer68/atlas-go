package monitoring

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// This file pins the judgement of the in-process universe coverage alert
// (monitoring.AssessUniverseCoverage, wired in cmd/atlas/main.go).
//
// The defect it exists for (2026-09-27 audit, gap C): the task alerted only when
// `symbols_built > 0 && coveragePct < 90`, so a 39.7h-old snapshot produced no
// alert at all. The age half did not exist, and the coverage half could not
// compensate: the task passed `symbols_built` (1,599) over
// TotalClassifiedSymbols(tree) (27 representative stocks) ⇒ ≈5922%, so `< 90`
// was unsatisfiable for any artifact reporting the pipeline's universe.
//
// That denominator is FIXED (issue #1944 item I29): it is now the pipeline
// POPULATION (monitoring.UniversePopulationSize), which is what turns the ratio
// into the question the alert asks. The fixtures below therefore set
// `PopulationSymbols` to population-scale numbers; the contract that the OLD
// representative-stock denominator can no longer produce a percentage (and no
// longer produces an absurd one) is pinned in
// TestUniverseCoverageCheck_PopulationDenominatorIsThePipelines.
//
// The population denominator makes the age tests below hold the age half down,
// plus the boundaries where the check must stay silent.
//
// Boundary rows are not filler. Each one is a world in which a plausible
// "improvement" of the check produces a false page:
//   - comparing the age against a fixed window instead of the trading calendar
//     fires on every holiday closure (the 2026-09-25 中秋 + 09-28 教師節 gap);
//   - judging before the grace period elapses fires while a healthy run is still
//     writing its artifact;
//   - turning an unreadable calendar or a missing mtime into a finding fires on
//     something the check cannot know.

func TestAssessUniverseCoverage(t *testing.T) {
	// A Tuesday session whose run was expected at 06:00Z; the coverage check runs
	// sixteen hours later (06:00 Asia/Taipei the next day, i.e. 22:00Z), which is
	// the real cadence in cmd/atlas/main.go.
	lastExpected := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	fresh := lastExpected.Add(2 * time.Minute)
	stale := lastExpected.Add(-48 * time.Hour)
	now := lastExpected.Add(16 * time.Hour)

	tests := []struct {
		name     string
		in       CoverageInput
		wantKind []string
		// mutationDiffers is true when deleting the age branch (see the
		// self-proof at the end of this test) must CHANGE the finding set. It is
		// the executable statement "this row is what the age check is for".
		mutationDiffers bool
	}{
		{
			name: "fresh artifact at full coverage: nothing to report",
			in: CoverageInput{
				SnapshotSymbols: 1599, PopulationSymbols: 1600,
				SnapshotMTime: fresh, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind: nil,
		},
		{
			name: "stale artifact at full coverage: the 2026-09-27 production shape",
			in: CoverageInput{
				SnapshotSymbols: 1599, PopulationSymbols: 1600,
				SnapshotMTime: stale, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind:        []string{CoverageFindingStaleArtifact},
			mutationDiffers: true,
		},
		{
			name: "stale artifact with low coverage: both findings, coverage first",
			in: CoverageInput{
				SnapshotSymbols: 100, PopulationSymbols: 1600,
				SnapshotMTime: stale, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind:        []string{CoverageFindingLowCoverage, CoverageFindingStaleArtifact},
			mutationDiffers: true,
		},
		{
			name: "fresh artifact with low coverage: the historical condition still fires",
			in: CoverageInput{
				SnapshotSymbols: 100, PopulationSymbols: 1600,
				SnapshotMTime: fresh, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind: []string{CoverageFindingLowCoverage},
		},
		{
			name: "coverage exactly at the threshold is not below it",
			in: CoverageInput{
				SnapshotSymbols: 1440, PopulationSymbols: 1600, // 90.0%
				SnapshotMTime: fresh, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind: nil,
		},
		{
			name: "just after the trigger, inside the grace: never race the run",
			in: CoverageInput{
				SnapshotSymbols: 1599, PopulationSymbols: 1600,
				SnapshotMTime: stale, Now: lastExpected.Add(time.Hour), LastExpectedRun: lastExpected,
			},
			wantKind: nil,
		},
		{
			name: "artifact within the mtime tolerance of the trigger counts as this run's",
			in: CoverageInput{
				SnapshotSymbols: 1599, PopulationSymbols: 1600,
				SnapshotMTime: lastExpected.Add(-time.Minute), Now: now, LastExpectedRun: lastExpected,
			},
			wantKind: nil,
		},
		{
			name: "unknown mtime (artifact missing) is not judged by this check",
			in: CoverageInput{
				SnapshotSymbols: 0, PopulationSymbols: 1600,
				SnapshotMTime: time.Time{}, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind: nil,
		},
		{
			name: "unreadable calendar is not turned into an age finding",
			in: CoverageInput{
				SnapshotSymbols: 1599, PopulationSymbols: 1600,
				SnapshotMTime: stale, Now: now, LastExpectedRun: time.Time{},
			},
			wantKind: nil,
		},
		{
			name: "unknown denominator: no coverage finding, age still judged",
			in: CoverageInput{
				SnapshotSymbols: 1599, PopulationSymbols: 0,
				SnapshotMTime: stale, Now: now, LastExpectedRun: lastExpected,
			},
			wantKind:        []string{CoverageFindingStaleArtifact},
			mutationDiffers: true,
		},
	}

	// withoutAgeCheck is AssessUniverseCoverage with the age branch deleted,
	// expressed through the function's own contract (both age inputs unknown ⇒
	// ArtifactStaleForCoverage is false). It duplicates no logic, so it cannot
	// drift from the real implementation the way a copy would.
	withoutAgeCheck := func(in CoverageInput) []CoverageFinding {
		in.SnapshotMTime = time.Time{}
		in.LastExpectedRun = time.Time{}
		return AssessUniverseCoverage(in)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AssessUniverseCoverage(tc.in)
			var kinds []string
			for _, f := range got {
				kinds = append(kinds, f.Kind)
				if f.Message == "" {
					t.Errorf("finding %q carries no message", f.Kind)
				}
				if f.Details["kind"] != f.Kind {
					t.Errorf("finding %q: metadata kind = %v, want %q", f.Kind, f.Details["kind"], f.Kind)
				}
			}
			if strings.Join(kinds, ",") != strings.Join(tc.wantKind, ",") {
				t.Fatalf("findings = %v, want %v", kinds, tc.wantKind)
			}

			// The message has to be actionable on its own: an operator reading the
			// alert must learn how old the artifact is and which run it should
			// have come from. A finding that only says "stale" would send them
			// back to the filesystem to reconstruct both numbers.
			for _, f := range got {
				if f.Kind != CoverageFindingStaleArtifact {
					continue
				}
				if !strings.Contains(f.Message, "old") ||
					!strings.Contains(f.Message, tc.in.LastExpectedRun.UTC().Format(time.RFC3339)) {
					t.Errorf("stale finding must carry the age and the last expected run, got: %s", f.Message)
				}
				if _, ok := f.Details["artifact_age_hours"]; !ok {
					t.Errorf("stale finding must carry artifact_age_hours, got: %v", f.Details)
				}
			}

			// Mutation self-proof: for the rows that carry a stale artifact, the
			// mutated implementation (age branch gone) must produce a DIFFERENT
			// finding set. If it ever matches, this table has stopped guarding the
			// half of the check that the 2026-09-27 audit added.
			mutated := withoutAgeCheck(tc.in)
			if tc.mutationDiffers && len(mutated) == len(got) {
				t.Errorf("mutation not caught: deleting the age branch left %d finding(s), same as the real check", len(mutated))
			}
			if !tc.mutationDiffers && len(mutated) != len(got) {
				t.Errorf("mutation changed a row the age branch does not own: %d -> %d findings", len(got), len(mutated))
			}
		})
	}
}

// TestArtifactStaleForCoverage_UnknownInputsStaySilent pins the two inputs whose
// zero value must never be read as "stale": they are the difference between "I
// cannot judge" and "the artifact is old".
func TestArtifactStaleForCoverage_UnknownInputsStaySilent(t *testing.T) {
	now := time.Date(2026, 9, 29, 22, 0, 0, 0, time.UTC)
	last := now.Add(-16 * time.Hour)
	old := now.Add(-720 * time.Hour)

	if ArtifactStaleForCoverage(time.Time{}, last, now) {
		t.Error("an unknown mtime was judged stale")
	}
	if ArtifactStaleForCoverage(old, time.Time{}, now) {
		t.Error("an unknown last-expected run was judged stale")
	}
	if !ArtifactStaleForCoverage(old, last, now) {
		t.Error("a 720h-old artifact was not judged stale")
	}
}

// TestPreviousUniverseRun_AcrossHolidayClosure is the regression that keeps the
// new age check from firing on a closure: the 2026-09-27 production artifact was
// 39.7h old at a moment when the calendar expected NO run in between (09-25 中秋,
// 09-26/27 weekend, 09-28 教師節), so it was correct and must stay silent. A
// window-based age check ("older than 24h") fails exactly here.
func TestPreviousUniverseRun_AcrossHolidayClosure(t *testing.T) {
	// The check runs at 06:00 Asia/Taipei; express that instant in UTC (22:00Z
	// the previous day) because that is how the fixtures in the verifier's shell
	// test are anchored.
	now := time.Date(2026, 9, 28, 22, 0, 0, 0, time.UTC)

	// Calendar premises, asserted rather than assumed: 09-28 and 09-25 are
	// closures, 09-24 is a session.
	for _, premise := range []struct {
		day         time.Time
		wantTrading bool
		why         string
	}{
		{time.Date(2026, 9, 25, 0, 0, 0, 0, universeLocation()), false, "中秋節 (休市)"},
		{time.Date(2026, 9, 28, 0, 0, 0, 0, universeLocation()), false, "教師節 (休市)"},
		{time.Date(2026, 9, 24, 0, 0, 0, 0, universeLocation()), true, "ordinary Thursday session"},
	} {
		requireCalendarPremise(t, premise.day, premise.wantTrading)
	}

	lastExpected := PreviousUniverseRun(now)
	want := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)
	if !lastExpected.Equal(want) {
		t.Fatalf("PreviousUniverseRun(%s) = %s, want %s (the closure must not shift it)", now, lastExpected, want)
	}

	// The real artifact of that moment: mtime 2026-09-25T17:01:40Z,
	// symbols_built 1599, symbols_ranked 150 (recorded in
	// docs/operations/universe-run-truth-model.md §5).
	mtime := time.Date(2026, 9, 25, 17, 1, 40, 0, time.UTC)
	if ArtifactStaleForCoverage(mtime, lastExpected, now) {
		t.Fatalf("the 2026-09-27 production artifact (mtime %s) was judged stale against %s: "+
			"the age check would have fired during a holiday closure", mtime, lastExpected)
	}
	findings := AssessUniverseCoverage(CoverageInput{
		SnapshotSymbols: 1599, PopulationSymbols: 1600,
		SnapshotMTime: mtime, Now: now, LastExpectedRun: lastExpected,
	})
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want none (fixture coverage 1599/1600 is above the threshold, and a holiday closure must not be judged stale)", findings)
	}

	// One session later the same artifact IS stale: this is the transition the
	// alert is for, and it is what makes the silence above a judgement rather
	// than a blind spot.
	later := time.Date(2026, 9, 29, 22, 0, 0, 0, time.UTC)
	if !ArtifactStaleForCoverage(mtime, PreviousUniverseRun(later), later) {
		t.Fatal("the same artifact was not judged stale one session later: the check cannot fire")
	}
}

// TestPreviousUniverseRun_IsTheMirrorOfNextUniverseRun keeps the two calendar
// halves from drifting apart: for any instant in a normal week, "the previous
// expected run" is strictly before "the next expected run", and neither is zero.
func TestPreviousUniverseRun_IsTheMirrorOfNextUniverseRun(t *testing.T) {
	now := time.Date(2026, 9, 29, 22, 0, 0, 0, time.UTC)
	prev := PreviousUniverseRun(now)
	next := NextUniverseRun(now)
	if prev.IsZero() || next.IsZero() {
		t.Fatalf("calendar returned a zero instant: prev=%s next=%s", prev, next)
	}
	if !prev.Before(now) || next.Before(now) {
		t.Fatalf("prev=%s must be before now=%s and next=%s must not be", prev, now, next)
	}
	if !prev.Before(next) {
		t.Fatalf("prev=%s is not before next=%s", prev, next)
	}
}

// TestUniverseCoverageCheck_DenominatorCannotFireWithPipelineUniverse pins the
// arithmetic behind the corrected narration of gap C (2026-09-27; see
// universe_coverage_check.go).
//
// The alert's coverage finding is `symbols_built > 0 && coveragePct < 90` over
// TotalSymbols = TotalClassifiedSymbols(tree). The tree supplies REPRESENTATIVE
// stocks, not the pipeline's universe, so with the production reading
// (symbols_built = 1,599; docs/operations/universe-run-truth-model.md §5) the
// percentage is ~5922% and the finding is unsatisfiable. This test exists so the
// corrected comments cannot drift back into the "99.9% coverage was fine"
// reading: if someone makes the denominator the pipeline's universe (issue #1944
// item I29), the pinned numbers below flip red and the narration must be updated
// in the same commit.
// TestUniverseCoverageCheck_PopulationDenominatorIsThePipelines pins the I29 fix
// (issue #1944) from three sides. "The denominator changed" is the kind of change
// that looks right on the happy path and wrong everywhere else, so each side is
// a separate assertion:
//
//  1. the OLD denominator — the classification tree's representative stocks —
//     can no longer be rendered as a percentage at all. It used to produce
//     ≈5922%, which is why the coverage half could never fire;
//  2. the population denominator makes the threshold REACHABLE: a run that
//     builds 87.5% of the population now fires, which is the entire point;
//  3. UniversePopulationSize really reads the pipeline's population (the
//     substrate's list) and not the representative table — otherwise (2) would
//     be satisfied by accident, with both numbers happening to be equal.
func TestUniverseCoverageCheck_PopulationDenominatorIsThePipelines(t *testing.T) {
	t.Run("the representative-stock table is not a percentage", func(t *testing.T) {
		const wantRepresentativeStocks = 27
		representative := TotalClassifiedSymbols(AdaptClassificationTree(industry.DefaultClassification()))
		if representative != wantRepresentativeStocks {
			t.Fatalf("TotalClassifiedSymbols(DefaultClassification()) = %d, want %d: if this changed deliberately (issue #1944 item I29), update the doc block in universe_coverage_check.go, the comment in cmd/atlas/main.go and this test together", representative, wantRepresentativeStocks)
		}
		const productionSymbolsBuilt = 1599 // 2026-09-25 artifact, truth-model §5

		findings := AssessUniverseCoverage(CoverageInput{
			SnapshotSymbols:   productionSymbolsBuilt,
			PopulationSymbols: representative,
			// Ages deliberately omitted: this must be judged on the ratio shape
			// alone, not because the age half is silent.
		})
		if len(findings) != 1 {
			t.Fatalf("findings = %v, want exactly the unusable-population finding", findings)
		}
		if findings[0].Kind != CoverageFindingPopulationUnusable {
			t.Fatalf("kind = %q, want %q (the representative table is smaller than the build, which is a contradiction, not a coverage reading)", findings[0].Kind, CoverageFindingPopulationUnusable)
		}
		if _, ok := findings[0].Details["coverage_pct"]; ok {
			t.Error("no coverage ratio may be attached to a denominator that is smaller than the numerator")
		}
		if strings.Contains(findings[0].Message, "5922") {
			t.Errorf("message still renders the absurd percentage: %q", findings[0].Message)
		}
	})

	t.Run("the population denominator makes the threshold reachable", func(t *testing.T) {
		const population = 1599 // production population (2026-09-25 artifact)

		// Baseline shape: the run built the whole population ⇒ silent. This is the
		// measured state and the reason the fix adds no paging source by itself.
		if got := AssessUniverseCoverage(CoverageInput{SnapshotSymbols: population, PopulationSymbols: population}); len(got) != 0 {
			t.Fatalf("findings = %v, want none when the run built the whole population", got)
		}

		// A real shortfall must now fire — the pre-I29 check could not fire for any
		// numerator the pipeline can produce.
		short := AssessUniverseCoverage(CoverageInput{SnapshotSymbols: 1400, PopulationSymbols: population})
		if len(short) != 1 || short[0].Kind != CoverageFindingLowCoverage {
			t.Fatalf("findings = %v, want one %s finding (1400/1599 = 87.5%%)", short, CoverageFindingLowCoverage)
		}
		pct, ok := short[0].Details["coverage_pct"].(float64)
		if !ok {
			t.Fatalf("coverage_pct missing from details: %v", short[0].Details)
		}
		if want := 1400.0 / 1599.0 * 100; pct != want {
			t.Errorf("coverage_pct = %v, want %v", pct, want)
		}
		if pct >= CoverageLowThreshold {
			t.Errorf("fixture arithmetic: %.2f%% is not below CoverageLowThreshold %.1f", pct, CoverageLowThreshold)
		}
	})

	t.Run("UniversePopulationSize reads the substrate, not the representative table", func(t *testing.T) {
		tree := AdaptClassificationTree(industry.DefaultClassification())

		// 30 distinct symbols in the first-party substrate: a population-scale list
		// (the production one is 1,599, its exact size is not what this test is
		// about).
		entries := make(map[string]industry.SectorID, 30)
		for i := range 30 {
			entries[fmt.Sprintf("23%02d", i)] = "semiconductor"
		}
		substrate := newFakeSubstrate(entries)

		got := UniversePopulationSize(tree, substrate)
		if got != len(entries) {
			t.Fatalf("UniversePopulationSize = %d, want %d (the substrate's population)", got, len(entries))
		}
		representative := TotalClassifiedSymbols(tree)
		if got == representative {
			t.Fatalf("the population and the representative table are both %d: this fixture cannot tell the two readings apart", got)
		}
		// Without a substrate the same function falls back to the tree+mapper
		// reading, i.e. the representative stocks — a real population, just a
		// smaller one. Pinned so the precedence itself is a test, not a comment.
		if fallback := UniversePopulationSize(tree, nil); fallback != representative {
			t.Errorf("UniversePopulationSize(tree, nil) = %d, want the tree+mapper fallback %d", fallback, representative)
		}
	})
}
