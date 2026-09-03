// Command validate-capital-flow-hypotheses — Phase 1 offline validator
// for the three pre-registered capital-flow hypotheses (plan
// .omo/plans/2026-09-04-capital-flow-model-plan.md §3; spec
// docs/specs/capital-flow-seven-dimension-spec.md §10):
//
//	H-CF-01  foreign futures OI leads foreign spot net 1-3 days
//	H-CF-02  TSM ADR information content for next-day TAIEX direction
//	H-CF-05  layered (E07 4-layer vote) vs equal-weight model
//
// The tool is strictly offline and read-only: it reads local data
// under -workdir (TAIFEX OI snapshots, T86 capital-flow snapshots,
// macro snapshots with TAIEX/TSM ADR, the rolling sample store, and
// the replay trading calendar), replays the pre-registered statistical
// procedures, prints a PASS/FAIL/INSUFFICIENT_DATA table, and
// optionally writes a JSON report with -out. It never writes state or
// config; flipping automation eligibility is a separate human PR.
//
// All decision thresholds are compile-time constants inside
// internal/capitalflow (pre-registered; never configurable from the
// CLI), so parameters cannot be tuned to force a PASS.
//
// Usage:
//
//	go run ./cmd/validate-capital-flow-hypotheses \
//	    [-workdir .] [-start 2020-01-01] [-end 2026-12-31] \
//	    [-out /tmp/cf-report.json] [-dry-run]
package main

import (
	"context"
	"encoding/json"
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

const (
	defaultReplayPath  = "data/replay/tw_extended_90days.csv"
	defaultOIDir       = "data/state/taifex_oi"
	defaultT86Dir      = "data/state/capital_flow"
	defaultMacroDir    = "data/state/macro"
	defaultRollingPath = "data/state/capital_flow_rolling.json"
	rollingCapacity    = 252 // must match the server wiring (main.go uses 252)
	sentinelFutureDate = "9999-12-31"
)

// macroRow is the slice of a macro snapshot the validator needs:
// TAIEX close (target series) and TSM ADR daily change in percent.
type macroRow struct {
	taiex    float64
	adrPct   float64
	hasADR   bool
	hasTaiex bool
}

func main() {
	var (
		workdir   = flag.String("workdir", ".", "repo working directory holding data/ (offline inputs)")
		startDate = flag.String("start", "", "restrict to trading dates >= YYYY-MM-DD (inclusive)")
		endDate   = flag.String("end", "", "restrict to trading dates <= YYYY-MM-DD (inclusive)")
		replay    = flag.String("replay", defaultReplayPath, "replay CSV trading calendar (relative to -workdir)")
		out       = flag.String("out", "", "write the results JSON to this path (empty = stdout only)")
		dryRun    = flag.Bool("dry-run", false, "run all validations read-only but write no report file")
	)
	flag.Parse()

	if *startDate != "" || *endDate != "" {
		if err := validateDateArg(*startDate); err != nil {
			log.Fatalf("validate-capital-flow-hypotheses: -start: %v", err)
		}
		if err := validateDateArg(*endDate); err != nil {
			log.Fatalf("validate-capital-flow-hypotheses: -end: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dates, err := loadTradingDates(*workdir, *replay, *startDate, *endDate)
	if err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: %v", err)
	}
	oi, err := loadFuturesOI(filepath.Join(*workdir, defaultOIDir))
	if err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: %v", err)
	}
	spot, err := loadForeignSpot(filepath.Join(*workdir, defaultT86Dir))
	if err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: %v", err)
	}
	macro, err := loadMacroSnapshots(filepath.Join(*workdir, defaultMacroDir))
	if err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: %v", err)
	}
	samples, err := loadRollingSamples(ctx, filepath.Join(*workdir, defaultRollingPath))
	if err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: %v", err)
	}

	taiex := make(map[string]float64, len(macro))
	adr := make(map[string]float64, len(macro))
	for d, row := range macro {
		if row.hasTaiex {
			taiex[d] = row.taiex
		}
		if row.hasADR {
			adr[d] = row.adrPct
		}
	}

	log.Printf("inputs: calendar=%d dates, OI days=%d, T86 spot days=%d, macro days=%d (taiex=%d, adr=%d), rolling dims=%d",
		len(dates), len(oi), len(spot), len(macro), len(taiex), len(adr), len(samples))

	results := []capitalflow.HypothesisResult{
		capitalflow.ValidateHypothesis01(oi, spot, dates),
		capitalflow.ValidateHypothesis02(adr, taiex, dates),
		capitalflow.ValidateHypothesis05(samples, taiex, dates),
	}

	fmt.Println()
	fmt.Println("=== Capital-Flow Hypothesis Validation (pre-registered thresholds) ===")
	for _, r := range results {
		fmt.Printf("\n[%s] %s (n=%d)\n  %s\n", r.ID, r.Status, r.SampleCount, r.Verdict)
		for _, n := range r.Notes {
			fmt.Printf("  note: %s\n", n)
		}
	}
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

	if *out == "" || *dryRun {
		if *dryRun && *out != "" {
			log.Printf("dry-run: report %s NOT written", *out)
		}
		return
	}
	if err := writeResultsJSON(*out, results); err != nil {
		log.Fatalf("validate-capital-flow-hypotheses: write report: %v", err)
	}
	log.Printf("report written: %s", *out)
}

// validateDateArg checks a YYYY-MM-DD CLI date argument.
func validateDateArg(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("bad date %q: %w", s, err)
	}
	return nil
}

// loadTradingDates loads the replay trading calendar and restricts it
// to the optional start/end window. It reuses the production loader so
// a malformed calendar row is a hard error.
func loadTradingDates(workdir, replayPath, start, end string) ([]string, error) {
	dates, err := capitalflow.LoadReplayTradingDates(filepath.Join(workdir, replayPath))
	if err != nil {
		return nil, fmt.Errorf("load trading calendar: %w", err)
	}
	if start == "" && end == "" {
		return dates, nil
	}
	out := make([]string, 0, len(dates))
	for _, d := range dates {
		if start != "" && d < start {
			continue
		}
		if end != "" && d > end {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// loadFuturesOI reads every YYYY-MM-DD.json TAIFEX institutional
// snapshot under dir and returns the TX (大台) foreign open-interest
// net in contracts, keyed by trading date.
func loadFuturesOI(dir string) (map[string]float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("read OI dir: %w", err)
	}
	out := make(map[string]float64, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue // not a dated snapshot
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var raw struct {
			Contracts map[string]struct {
				Foreign struct {
					OINet int64 `json:"oi_net"`
				} `json:"foreign"`
			} `json:"contracts"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		tx, ok := raw.Contracts["TX"]
		if !ok {
			continue
		}
		out[date] = float64(tx.Foreign.OINet)
	}
	return out, nil
}

// loadForeignSpot reuses the production T86 loader: foreign investor
// spot net (hundred_million_shares) keyed by trading date.
func loadForeignSpot(dir string) (map[string]float64, error) {
	recs, err := capitalflow.LoadT86CapitalFlow(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, err
	}
	out := make(map[string]float64, len(recs))
	for d, r := range recs {
		out[d] = r.ForeignNet
	}
	return out, nil
}

// loadMacroSnapshots reads the dated macro snapshots under dir and
// extracts the TAIEX close and TSM ADR change percent. Older files
// without a tsm_adr channel simply contribute no ADR sample.
func loadMacroSnapshots(dir string) (map[string]macroRow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]macroRow{}, nil
		}
		return nil, fmt.Errorf("read macro dir: %w", err)
	}
	out := make(map[string]macroRow, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "latest.json" || name == "previous.json" {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var raw struct {
			Taiex struct {
				Value float64 `json:"value"`
			} `json:"taiex"`
			TSMADR struct {
				ChangePct float64 `json:"change_pct"`
			} `json:"tsm_adr"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		row := macroRow{taiex: raw.Taiex.Value, hasTaiex: raw.Taiex.Value != 0}
		if raw.TSMADR.ChangePct != 0 {
			row.adrPct = raw.TSMADR.ChangePct
			row.hasADR = true
		}
		out[date] = row
	}
	return out, nil
}

// loadRollingSamples reads back the full persisted rolling store (all
// seven dimensions) via the production store contract.
func loadRollingSamples(ctx context.Context, path string) (map[capitalflow.ForceName][]capitalflow.RollingSample, error) {
	store := capitalflow.NewFileRollingSampleStore(path, rollingCapacity)
	dims := []capitalflow.ForceName{
		capitalflow.ForceForeign, capitalflow.ForceFutures, capitalflow.ForceTSMADR,
		capitalflow.ForceInstitutional, capitalflow.ForceDealer,
		capitalflow.ForceGovernment, capitalflow.ForceRetail,
	}
	out := make(map[capitalflow.ForceName][]capitalflow.RollingSample, len(dims))
	for _, dim := range dims {
		rows, err := store.History(ctx, dim, sentinelFutureDate, rollingCapacity)
		if err != nil {
			// A missing store file means the dimension simply has no
			// history; every hypothesis then reports
			// INSUFFICIENT_DATA, which is the honest outcome.
			if os.IsNotExist(err) {
				out[dim] = nil
				continue
			}
			return nil, fmt.Errorf("read rolling store %s: %w", dim, err)
		}
		out[dim] = rows
	}
	return out, nil
}

// writeResultsJSON marshals the hypothesis results as a JSON object.
func writeResultsJSON(path string, results []capitalflow.HypothesisResult) error {
	data, err := json.MarshalIndent(map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"hypotheses":   results,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
