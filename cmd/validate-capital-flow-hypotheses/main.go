// Command validate-capital-flow-hypotheses — Phase 1 offline validator
// for the three pre-registered capital-flow hypotheses (plan
// .omo/plans/2026-09-04-capital-flow-model-plan.md §3; spec
// docs/specs/capital-flow-seven-dimension-spec.md §10):
//
//	H-CF-01  foreign futures OI leads foreign spot net 1-3 days
//	H-CF-02  TSM ADR information content for next-day TAIEX direction
//	H-CF-05  layered (E07 4-layer vote) vs equal-weight model
//
// OI SOURCE CONTRACT (H-CF-01 data hygiene, 2026-09-04): the FinMind
// snapshots under data/state/taifex_oi/ are the SINGLE open-interest source
// for every judgment this tool produces. The macro-snapshot channel
// (foreign_futures_oi_net) is BLOCKED as signal input — its date attribution
// carried the previous session's value on 19/33 overlap days (see
// internal/capitalflow/oi_alignment.go); any -r3+ judgment report must state
// this single-source declaration verbatim.
//
// The tool is strictly offline and read-only: it reads local data
// under -workdir (TAIFEX OI snapshots, T86 capital-flow snapshots,
// macro snapshots with TAIEX/TSM ADR, the rolling sample store, and
// the replay trading calendar) through the shared runner in
// internal/capitalflow (RunHypothesisValidation — the exact procedure
// the scheduled cf_hypothesis_validation task replays), prints a
// PASS/FAIL/INSUFFICIENT_DATA table, and optionally writes a versioned
// validation report (JSON via -out, human-readable Markdown via
// -out-md; conventional location
// data/reports/cf-hypotheses-<date>.{json,md}). It never writes state
// or config; flipping automation eligibility is a separate human PR.
//
// All decision thresholds are compile-time constants inside
// internal/capitalflow (pre-registered; never configurable from the
// CLI), so parameters cannot be tuned to force a PASS.
//
// Usage:
//
//	go run ./cmd/validate-capital-flow-hypotheses \
//	    [-workdir .] [-start 2020-01-01] [-end 2026-12-31] \
//	    [-out data/reports/cf-hypotheses-2026-09-04.json \
//	     -out-md data/reports/cf-hypotheses-2026-09-04.md] [-dry-run]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

func main() {
	var (
		workdir   = flag.String("workdir", ".", "repo working directory holding data/ (offline inputs)")
		startDate = flag.String("start", "", "restrict to trading dates >= YYYY-MM-DD (inclusive)")
		endDate   = flag.String("end", "", "restrict to trading dates <= YYYY-MM-DD (inclusive)")
		out       = flag.String("out", "", "write the validation report JSON to this path (empty = stdout only)")
		outMD     = flag.String("out-md", "", "write the human-readable Markdown report to this path (empty = skip)")
		dryRun    = flag.Bool("dry-run", false, "run all validations read-only but write no report file")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	report, err := capitalflow.RunHypothesisValidation(ctx, capitalflow.ValidationInputs{
		WorkDir:   *workdir,
		StartDate: *startDate,
		EndDate:   *endDate,
	})
	if err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: %v", err)
	}
	results := report.Hypotheses
	coverage := report.DataCoverage

	log.Printf("inputs: calendar=%d, oi_days=%d, t86_spot_days=%d, macro_days=%d (taiex=%d, adr=%d)",
		coverage["calendar_days"], coverage["oi_days"], coverage["t86_spot_days"],
		coverage["macro_days"], coverage["taiex_days"], coverage["adr_days"])

	fmt.Println()
	fmt.Println("=== Capital-Flow Hypothesis Validation (pre-registered thresholds) ===")
	for _, r := range results {
		fmt.Printf("\n[%s] %s (n=%d)\n  %s\n", r.ID, r.Status, r.SampleCount, r.Verdict)
		for _, n := range r.Notes {
			fmt.Printf("  note: %s\n", n)
		}
	}
	fmt.Printf("\neligible_recommendation: %t (僅供人工 PR review；CLI 永不寫 config)\n", report.EligibleRecommendation)
	fmt.Println("\nPass/FAIL 对照表:")
	fmt.Printf("%-10s %-18s %s\n", "ID", "STATUS", "KEY METRICS")
	for _, r := range results {
		keys := make([]string, 0, len(r.Metrics))
		for k := range r.Metrics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%.4g", k, r.Metrics[k]))
		}
		fmt.Printf("%-10s %-18s %s\n", r.ID, r.Status, strings.Join(parts, ", "))
	}

	if *dryRun {
		if *out != "" || *outMD != "" {
			log.Printf("dry-run: report files NOT written (-out %q -out-md %q)", *out, *outMD)
		}
		return
	}
	if *out != "" {
		if err := capitalflow.WriteValidationReportJSON(*out, report); err != nil {
			log.Fatalf("validate-capital-flow-hypotheses: write JSON report: %v", err)
		}
		log.Printf("JSON report written: %s", *out)
	}
	if *outMD != "" {
		if err := capitalflow.WriteValidationReportMarkdown(*outMD, report); err != nil {
			log.Fatalf("validate-capital-flow-hypotheses: write Markdown report: %v", err)
		}
		log.Printf("Markdown report written: %s", *outMD)
	}
}
