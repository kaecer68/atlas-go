package monitoring

import (
	"fmt"
	"math"
	"time"
)

// This file holds the judgement behind the in-process alert
// `universe_coverage_check` (cmd/atlas/main.go).
//
// Why the judgement moved out of the task closure (2026-09-27 audit, gap C)
//
// The task used to alert on exactly one condition:
//
//	symbols_built > 0 && coveragePct < 90
//
// where the numerator came from the snapshot artifact on disk and the
// denominator from the classification tree. Two independent reasons made that
// condition silent on the production shape of 2026-09-27 (an artifact with
// symbols_built=1599 and an mtime 39.7h old — see
// docs/operations/universe-run-truth-model.md §5). Only the second one is the
// gap this file closes:
//
//  1. The percentage was not a coverage ratio, so the coverage half could not
//     fire either. The numerator is the snapshot's `symbols_built` — 1,599, the
//     pipeline's universe — while the denominator WAS TotalClassifiedSymbols(tree):
//     the classification tree's REPRESENTATIVE stocks, 27 against the shipped
//     tree (measured 2026-09-27: 11 of the 12 L1 segments carry representatives,
//     Σ len(seg.RepresentativeStocks) = 27). 1599/27 ≈ 5922%, so `coveragePct < 90`
//     would have needed `symbols_built < 24.3` — a state in which the number means
//     nothing, i.e. the coverage half could never fire.
//
//     FIXED (issue #1944 item I29, 2026-09-29). The denominator is now the
//     PIPELINE POPULATION — how many symbols the pipeline is expected to cover,
//     i.e. len(gatherAllSymbols(tree, mapper, substrate)) via
//     UniversePopulationSize, the same precedence the pipeline itself uses
//     (first-party symbol_industry population when available, tree+mapper
//     otherwise). The ratio therefore answers the question the alert claims to
//     ask — "of the symbols this pipeline can see, how many did the last run
//     actually build?" — and it is satisfiable: a run that builds materially
//     fewer symbols than the population now reports low coverage.
//
//     Consequence of the fix (measured, not assumed): on the production shape of
//     2026-09-29 the population and the build agree (1,599 gathered / 1,599
//     built ⇒ **100.0%**), so the new condition does NOT introduce a new paging
//     source at the baseline. What it can fire on, and what it therefore means,
//     is worth stating plainly rather than implying a market-coverage claim:
//
//       - the numerator is the LAST run's build and the denominator is the
//         population the checker can see NOW, so the ratio tracks POPULATION
//         DRIFT between runs: a run that is materially behind a grown population
//         (>11% growth before the next run) reports low coverage;
//       - a population source that is degraded at check time (substrate not
//         loaded ⇒ tree+mapper fallback, a mapper that lost symbols) makes the
//         denominator SMALLER than the numerator. That shape is no longer
//         rendered as an absurd percentage (pre-I29: 1,599/27 ≈ 5922%); it is
//         reported as CoverageFindingPopulationUnusable, because the population
//         reading itself is what is broken.
//
//     Both are real, actionable shapes. Neither is reachable at the measured
//     baseline, which is why the fix adds no paging source by itself.
//  2. Nothing in the condition mentioned WHEN the artifact was written, so even a
//     correct percentage could not have flagged the stale file.
//
// Two consequences shaped this design:
//
//  1. The age half is NOT a Prometheus rule candidate. It is an in-process
//     check, so no amount of rule writing could have covered it — the fix has to
//     live where the number is computed.
//  2. A check that can only be exercised by running the scheduled task cannot be
//     driven into its failure state on purpose. The judgement is therefore a
//     pure function of (artifact, clock, calendar) and the task keeps only the
//     wiring: what to alert with, not what to conclude.
//
// What this function deliberately does NOT do: judge a MISSING artifact. It
// cannot — there is no mtime to compare — and that shape already has owners
// (the AtlasUniverseSnapshotNotPersisted rule and the verifier's L5, both of
// which read the file directly). Returning no finding here is not a claim that
// an absent artifact is healthy; it is the statement "this check is not the
// detector for that".

// CoverageFinding kind values. They are stable identifiers carried in the alert
// metadata so a consumer can tell the two findings apart without parsing the
// human-readable message.
const (
	// CoverageFindingLowCoverage is the historical condition: the artifact is
	// there and its coverage is below the threshold.
	CoverageFindingLowCoverage = "coverage_below_threshold"
	// CoverageFindingStaleArtifact is the condition this check was missing: the
	// artifact is older than the last run the calendar says should have written
	// it, so whatever coverage it reports describes an older market.
	CoverageFindingStaleArtifact = "artifact_stale"
	// CoverageFindingPopulationUnusable is the "the denominator is not a
	// population" shape: the artifact claims to have built MORE symbols than the
	// population the pipeline can see. Reporting that as a percentage produces a
	// meaningless >100% reading (the pre-I29 behavior, 1,599/27 ≈ 5922%), so this
	// kind reports the contradiction itself and no ratio. It is not a coverage
	// verdict: it says the population source is incomplete (a substrate that did
	// not load, a mapper that lost symbols), which is why it must be visible.
	CoverageFindingPopulationUnusable = "population_smaller_than_built"
)

// CoverageLowThreshold is the coverage percentage below which the check alerts:
// built symbols / pipeline population. It is the threshold the task has always
// used, and it is unchanged by the I29 fix — what changed is that the ratio can
// now actually reach it (see the file header). It lives here so the finding's
// message and the task's wiring cannot drift apart.
const CoverageLowThreshold = 90.0

// CoverageStaleGrace is how long after the last expected run the artifact may
// stay untouched before the age check is allowed to judge it.
//
// Two hours, matching two independent sources that already encode the same fact:
// AtlasUniverseRunOverdue's 2h window and the verifier's
// DEFAULT_GRACE_SECONDS. A universe run takes minutes (2026-09-25 production:
// ~2 min), so a trigger that is two hours old and has left no newer artifact is
// not "a run in progress".
const CoverageStaleGrace = 2 * time.Hour

// CoverageStaleTolerance absorbs filesystem timestamp granularity and clock
// alignment when the artifact's mtime is compared against the instant the run
// was scheduled. It mirrors the verifier's ARTIFACT_STALE_SECONDS.
//
// It is small on purpose: a false "fresh" (reporting a stale artifact as if it
// belonged to the last expected run) is the failure mode that would silence the
// alert this check feeds.
const CoverageStaleTolerance = 2 * time.Minute

// CoverageInput is everything the coverage check is allowed to judge. Every
// field is a measurement taken by the caller; the function adds no I/O, so a
// test can drive any world it likes.
type CoverageInput struct {
	// SnapshotSymbols is snapshot.result.symbols_built: the numerator, read from
	// the canonical artifact. Zero when the artifact is missing or unreadable.
	SnapshotSymbols int
	// PopulationSymbols is the denominator: the number of symbols the pipeline is
	// expected to cover, as reported by UniversePopulationSize(tree, substrate)
	// — the first-party symbol_industry population when the substrate provides
	// one, the tree+mapper fallback otherwise. It is NOT
	// TotalClassifiedSymbols(tree) (that is the classification tree's
	// representative stocks, 27 against the shipped tree, and using it made the
	// ratio meaningless — issue #1944 item I29).
	PopulationSymbols int
	// SnapshotMTime is the modification time of the canonical snapshot artifact.
	// The zero time means "unknown" (missing file) and disables the age check.
	SnapshotMTime time.Time
	// Now is the instant the check runs.
	Now time.Time
	// LastExpectedRun is the last instant at which the universe pipeline was
	// scheduled to run, computed from the Taiwan trading calendar
	// (PreviousUniverseRun). The zero time means "the calendar is unreadable" and
	// disables the age check rather than guessing.
	LastExpectedRun time.Time
}

// CoverageFinding is one reason to alert. An empty slice is the honest "nothing
// to report" — it is never a claim that the universe is healthy in general, only
// that none of the conditions this check owns holds.
type CoverageFinding struct {
	// Kind is one of the CoverageFinding* constants.
	Kind string
	// Message is what the operator reads. It must carry the numbers needed to
	// act: for a stale artifact, how old it is and which run it should have come
	// from.
	Message string
	// Details is the structured metadata attached to the alert.
	Details map[string]any
}

// ArtifactStaleForCoverage reports whether an artifact with modification time
// mtime is older than the last run the calendar expected at lastExpectedRun,
// judged at now.
//
// Both zero-time inputs mean "unknown" and return false: the calendar is parsed
// from internal/taiwanholidays and an unreadable calendar must not be turned
// into an alert (the same rule NextUniverseRun follows when it publishes no
// heartbeat at all).
//
// The two time gates are deliberately different, and both are needed:
//
//   - now must be past lastExpectedRun + CoverageStaleGrace, so the check never
//     races a run that has just started (a healthy pipeline writes its artifact
//     minutes after the trigger);
//   - mtime must be before lastExpectedRun - CoverageStaleTolerance: the artifact
//     has to predate the run that should have replaced it. Comparing against
//     "now minus some window" instead would fire on every holiday closure, which
//     is exactly the false-positive shape the calendar-aware rules were
//     introduced to remove.
func ArtifactStaleForCoverage(mtime, lastExpectedRun, now time.Time) bool {
	if mtime.IsZero() || lastExpectedRun.IsZero() {
		return false
	}
	if !now.After(lastExpectedRun.Add(CoverageStaleGrace)) {
		return false
	}
	return mtime.Before(lastExpectedRun.Add(-CoverageStaleTolerance))
}

// AssessUniverseCoverage turns one reading of the snapshot artifact into the
// findings the coverage task should alert on.
//
// The findings are independent and can both hold at once: an artifact can be
// stale AND report low coverage, and reporting only one of them would hide the
// other. The order is stable (coverage first, then age) so tests and logs are
// comparable.
func AssessUniverseCoverage(in CoverageInput) []CoverageFinding {
	var findings []CoverageFinding

	switch {
	case in.PopulationSymbols > 0 && in.SnapshotSymbols > in.PopulationSymbols:
		// The denominator is not a population at all: the artifact claims more
		// symbols than the pipeline can see. Rendering that as a percentage is the
		// pre-I29 bug (1,599/27 ≈ 5922%), so the contradiction is reported as
		// itself and no ratio is attached.
		findings = append(findings, CoverageFinding{
			Kind: CoverageFindingPopulationUnusable,
			Message: fmt.Sprintf(
				"Universe coverage not measurable: the snapshot reports %d built symbols but the pipeline population "+
					"is only %d — the population reading is incomplete (no coverage ratio is reported, because one "+
					"would exceed 100%%)",
				in.SnapshotSymbols, in.PopulationSymbols),
			Details: map[string]any{
				"kind":               CoverageFindingPopulationUnusable,
				"snapshot_symbols":   in.SnapshotSymbols,
				"population_symbols": in.PopulationSymbols,
			},
		})
	case in.PopulationSymbols > 0 && in.SnapshotSymbols > 0:
		pct := float64(in.SnapshotSymbols) / float64(in.PopulationSymbols) * 100
		if pct < CoverageLowThreshold {
			findings = append(findings, CoverageFinding{
				Kind: CoverageFindingLowCoverage,
				Message: fmt.Sprintf(
					"Universe coverage %.1f%% (%d built of %d expected symbols) — the pipeline built materially "+
						"fewer symbols than its population",
					pct, in.SnapshotSymbols, in.PopulationSymbols),
				Details: map[string]any{
					"kind":               CoverageFindingLowCoverage,
					"snapshot_symbols":   in.SnapshotSymbols,
					"population_symbols": in.PopulationSymbols,
					"coverage_pct":       pct,
					"threshold_pct":      CoverageLowThreshold,
				},
			})
		}
	}

	if ArtifactStaleForCoverage(in.SnapshotMTime, in.LastExpectedRun, in.Now) {
		age := in.Now.Sub(in.SnapshotMTime)
		findings = append(findings, CoverageFinding{
			Kind: CoverageFindingStaleArtifact,
			Message: fmt.Sprintf(
				"Universe snapshot artifact is %.1fh old (mtime %s) — older than the last expected universe run "+
					"(%s); any coverage reading from it (%d/%d) describes that older market, not this one",
				age.Hours(),
				in.SnapshotMTime.UTC().Format(time.RFC3339),
				in.LastExpectedRun.UTC().Format(time.RFC3339),
				in.SnapshotSymbols, in.PopulationSymbols),
			Details: map[string]any{
				"kind":                   CoverageFindingStaleArtifact,
				"artifact_age_hours":     math.Round(age.Hours()*10) / 10,
				"artifact_mtime":         in.SnapshotMTime.UTC().Format(time.RFC3339),
				"last_expected_run":      in.LastExpectedRun.UTC().Format(time.RFC3339),
				"snapshot_symbols":       in.SnapshotSymbols,
				"population_symbols":     in.PopulationSymbols,
				"stale_grace_seconds":    int(CoverageStaleGrace.Seconds()),
				"stale_tolerance_secs":   int(CoverageStaleTolerance.Seconds()),
				"last_expected_run_unix": in.LastExpectedRun.Unix(),
			},
		})
	}

	return findings
}
