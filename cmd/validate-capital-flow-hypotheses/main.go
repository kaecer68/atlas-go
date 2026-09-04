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
// macro snapshots with TAIEX/TSM ADR, and the rolling sample store)
// through the shared runner in internal/capitalflow
// (RunHypothesisValidation — the exact procedure the scheduled
// cf_hypothesis_validation task replays), prints a
// PASS/FAIL/INSUFFICIENT_DATA table, and optionally writes a versioned
// validation report (JSON via -out, human-readable Markdown via
// -out-md; conventional location
// data/reports/cf-hypotheses-<date>.{json,md}). It never writes state
// or config; flipping automation eligibility is a separate human PR.
//
// -v2 (H-CF-01 v2 judgment family, PR-3 wiring; arbitration report
// .omo/plans/2026-09-04-hcf01-arbitration.md §2.2–2.6): replaces the
// v1 H-CF-01 judgment with the three pre-registered v2 judgments — v2a
// revised + v2a′ + v2b — run in ONE batch with the same-batch Holm
// correction, the abandonment line, the exact n with the per-day drop
// list, and the verbatim single-OI-source declaration
// (internal/capitalflow.RunHCF01V2Family). This is a MANUAL-ONLY
// governance one-shot: the scheduled cf_hypothesis_validation rerun
// never sets it. In v2 mode the report NEVER recommends eligibility
// (arbitration §4: a v2-family PASS does not flip eligible — a PASS
// only earns the 30-day online observation window and a separate human
// config PR). The -r3 report filename is the caller's choice: pass it
// straight through -out/-out-md, e.g. -out
// data/reports/cf-hypotheses-2026-09-04-r3.json (never overwriting the
// -r2 files). Sample assembly stays inside internal/capitalflow
// (buildHCF01V2Days); the CLI only hands over raw data maps.
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
//	     -out-md data/reports/cf-hypotheses-2026-09-04.md] \
//	    [-v2] [-dry-run]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

func main() {
	flags := registerValidationFlags(flag.CommandLine)
	_ = flag.CommandLine.Parse(os.Args[1:])

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var report capitalflow.ValidationReport
	var err error
	if *flags.v2 {
		report, err = runV2FamilyValidation(*flags.workdir, *flags.start, *flags.end)
	} else {
		report, err = capitalflow.RunHypothesisValidation(ctx, capitalflow.ValidationInputs{
			WorkDir:   *flags.workdir,
			StartDate: *flags.start,
			EndDate:   *flags.end,
		})
	}
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

	if *flags.dryRun {
		if *flags.out != "" || *flags.outMD != "" {
			log.Printf("dry-run: report files NOT written (-out %q -out-md %q)", *flags.out, *flags.outMD)
		}
		return
	}
	if *flags.out != "" {
		if err := capitalflow.WriteValidationReportJSON(*flags.out, report); err != nil {
			log.Fatalf("validate-capital-flow-hypotheses: write JSON report: %v", err)
		}
		log.Printf("JSON report written: %s", *flags.out)
	}
	if *flags.outMD != "" {
		if err := capitalflow.WriteValidationReportMarkdown(*flags.outMD, report); err != nil {
			log.Fatalf("validate-capital-flow-hypotheses: write Markdown report: %v", err)
		}
		log.Printf("Markdown report written: %s", *flags.outMD)
	}
}

// validationFlags holds the parsed CLI flag pointers.
type validationFlags struct {
	workdir *string
	start   *string
	end     *string
	out     *string
	outMD   *string
	dryRun  *bool
	v2      *bool
}

// registerValidationFlags declares the CLI flags on fs. Split out of
// main so the flag contract (including -v2) is testable.
func registerValidationFlags(fs *flag.FlagSet) validationFlags {
	return validationFlags{
		workdir: fs.String("workdir", ".", "repo working directory holding data/ (offline inputs)"),
		start:   fs.String("start", "", "restrict to trading dates >= YYYY-MM-DD (inclusive)"),
		end:     fs.String("end", "", "restrict to trading dates <= YYYY-MM-DD (inclusive)"),
		out:     fs.String("out", "", "write the validation report JSON to this path (empty = stdout only); in -v2 mode pass the -r3 filename here, e.g. data/reports/cf-hypotheses-2026-09-04-r3.json"),
		outMD:   fs.String("out-md", "", "write the human-readable Markdown report to this path (empty = skip)"),
		dryRun:  fs.Bool("dry-run", false, "run all validations read-only but write no report file"),
		v2:      fs.Bool("v2", false, "run the H-CF-01 v2 judgment family (v2a revised + v2a-prime + v2b, same-batch Holm correction + abandonment line) instead of the v1 H-CF-01 judgment; manual-only governance one-shot; the report never recommends eligibility"),
	}
}

// runV2FamilyValidation runs the H-CF-01 v2 judgment family (PR-3
// wiring) on the offline inputs under workdir. It reuses the shared
// exported loaders from internal/capitalflow and hands the RAW maps to
// RunHCF01V2Family — sample assembly stays inside internal/capitalflow
// (buildHCF01V2Days), never in the CLI. The returned report carries
// the three v2 judgments and NEVER recommends eligibility (arbitration
// §4: a v2-family PASS does not flip eligible on its own).
func runV2FamilyValidation(workdir, start, end string) (capitalflow.ValidationReport, error) {
	if err := capitalflow.ValidateValidationDateArg(start); err != nil {
		return capitalflow.ValidationReport{}, fmt.Errorf("start date: %w", err)
	}
	if err := capitalflow.ValidateValidationDateArg(end); err != nil {
		return capitalflow.ValidationReport{}, fmt.Errorf("end date: %w", err)
	}
	dates, err := capitalflow.LoadReplayTradingDates(filepath.Join(workdir, capitalflow.ValidationDefaultReplayPath))
	if err != nil {
		return capitalflow.ValidationReport{}, fmt.Errorf("load trading calendar: %w", err)
	}
	if start != "" || end != "" {
		filtered := make([]string, 0, len(dates))
		for _, d := range dates {
			if start != "" && d < start {
				continue
			}
			if end != "" && d > end {
				continue
			}
			filtered = append(filtered, d)
		}
		dates = filtered
	}
	oi, err := capitalflow.LoadValidationFuturesOI(filepath.Join(workdir, capitalflow.ValidationDefaultOIDir))
	if err != nil {
		return capitalflow.ValidationReport{}, err
	}
	spot, err := capitalflow.LoadValidationForeignSpot(filepath.Join(workdir, capitalflow.ValidationDefaultT86Dir))
	if err != nil {
		return capitalflow.ValidationReport{}, err
	}
	macro, err := capitalflow.LoadValidationMacroSnapshots(filepath.Join(workdir, capitalflow.ValidationDefaultMacroDir))
	if err != nil {
		return capitalflow.ValidationReport{}, err
	}
	taiex := make(map[string]float64, len(macro))
	for d, row := range macro {
		if v, ok := row.Taiex(); ok {
			taiex[d] = v
		}
	}

	family := capitalflow.RunHCF01V2Family(capitalflow.HCF01V2Inputs{
		FuturesOI: oi,
		SpotNet:   spot,
		TAIEX:     taiex,
		Dates:     dates,
	})
	log.Printf("v2 family: exact n = v2a %d / v2a-prime %d / v2b %d; drop-list days %d",
		family.ExactN["H-CF-01-v2a"], family.ExactN["H-CF-01-v2a-prime"],
		family.ExactN["H-CF-01-v2b"], len(family.DropList))

	report := capitalflow.BuildValidationReport(workdir, map[string]int{
		"calendar_days": len(dates),
		"oi_days":       len(oi),
		"t86_spot_days": len(spot),
		"macro_days":    len(macro),
		"taiex_days":    len(taiex),
	}, []capitalflow.HypothesisResult{family.V2A, family.V2APrime, family.V2B})
	// Governance guard (arbitration §4): the v2 family judgment NEVER
	// recommends eligibility — the flag is only ever forced DOWN to
	// false; flipping eligible stays a separate human config PR.
	if report.EligibleRecommendation {
		log.Printf("v2 mode: eligible_recommendation suppressed (a v2-family PASS never flips eligible on its own)")
	}
	report.EligibleRecommendation = false
	report.OperatorNotes = append(report.OperatorNotes,
		"v2 判決模式（-v2）：v2 家族結果永不自動建議 eligible（仲裁報告 §4；PASS 僅換得 30 日線上觀察期），eligible 翻轉仍須另一個人工 config PR。")
	if family.AbandonmentTriggered {
		report.OperatorNotes = append(report.OperatorNotes,
			"放棄線（§2.5）已觸發："+family.AbandonmentNote)
	}
	return report, nil
}
