// Command run-stockpicker-backtest runs the PR 1c stockpicker panel
// backtest + win-rate aggregation job after market close.
//
// Usage (from the repo root):
//
//	run-stockpicker-backtest -workdir . -start 2026-01-01 -end 2026-08-26 -asof 2026-08-26
//
// Flags: -workdir, -start, -end, -asof, -force, -dry-run (and optional
// -universe to limit the scan). The job:
//
//  1. replays the PR 1c demo conditions (foreign-3d-net-buy,
//     momentum-20d-positive) over point-in-time price bars (SQLite quotes)
//     and per-symbol T86 flows (data/state/stock_flows/<symbol>.json), and
//  2. writes SignalOutcome rows into stock_signal_outcomes (idempotent:
//     ON CONFLICT DO NOTHING), then aggregates per (symbol, source) into
//     stock_win_rate and data/state/stock_win_rate.json.
//
// Same-day rerun skips unless -force is given. No runtime scheduler is wired
// in this PR; the job is CLI-only (scheduling lands in a later PR).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// aggregationWindow is the rolling window label written into stock_win_rate.
const aggregationWindow = "120d"

// confidenceLevel is the Wilson interval confidence used for summaries.
const confidenceLevel = 0.95

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

// run parses CLI flags and delegates to runWithPanel. panel==nil builds the
// real point-in-time panel from the workdir (SQLite quotes + flow files).
func run(args []string) error {
	return runWithPanel(args, nil)
}

// runWithPanel is the testable core. A nil panel builds the real one.
func runWithPanel(args []string, panel stockpicker.PanelSource) error {
	fs := flag.NewFlagSet("run-stockpicker-backtest", flag.ContinueOnError)
	workDir := fs.String("workdir", ".", "atlas work directory (repo root)")
	startStr := fs.String("start", "", "backtest start date YYYY-MM-DD (default: as-of minus 120 days)")
	endStr := fs.String("end", "", "backtest end date YYYY-MM-DD (default: as-of)")
	asofStr := fs.String("asof", "", "point-in-time data cutoff YYYY-MM-DD (default: today UTC); any bar after this fails the run")
	force := fs.Bool("force", false, "rerun even when outcomes already exist for the date range")
	dryRun := fs.Bool("dry-run", false, "compute coverage and print it without persisting")
	universe := fs.String("universe", "", "comma-separated symbols to scan (default: all symbols present in quotes)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	asOf, err := parseDate(*asofStr, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("parse asof: %w", err)
	}
	start, err := parseDate(*startStr, asOf.AddDate(0, 0, -120))
	if err != nil {
		return fmt.Errorf("parse start: %w", err)
	}
	end, err := parseDate(*endStr, asOf)
	if err != nil {
		return fmt.Errorf("parse end: %w", err)
	}
	if end.Before(start) {
		return fmt.Errorf("end %s before start %s", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	// Parameters from the worktree's parameters.json (P0-3/P0-6: cost and
	// min_samples come from config, never hard-coded).
	paramsPath := filepath.Join(*workDir, "configs", "parameters.json")
	if _, err := os.Stat(paramsPath); err != nil {
		return fmt.Errorf("parameters config %s: %w", paramsPath, err)
	}
	params, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return fmt.Errorf("load parameters: %w", err)
	}
	costRate := params.Stockpicker.Costs.RoundTripPct.Value
	minSamples := params.Stockpicker.Calibration.MinSamples.Value

	dbPath := filepath.Join(*workDir, "data", "state", "atlas.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		return fmt.Errorf("open ledger db %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()
	if err := ledger.InitSchema(db); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	outStore := stockpicker.NewSignalOutcomeStore(db)
	winStore := stockpicker.NewWinRateStore(db)

	// Idempotency: same-day rerun skips (autobacktest snapshot_exists_skip
	// pattern). -force bypasses the check.
	if !*force && !*dryRun {
		n, err := countExistingOutcomes(ctx, db, start, end)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if n > 0 {
			log.Printf("skip: %d stockpicker outcomes already exist for %s..%s (use -force to rerun)", n,
				start.Format("2006-01-02"), end.Format("2006-01-02"))
			return nil
		}
	}

	if panel == nil {
		panel, err = newRealPanel(ctx, db, *workDir)
		if err != nil {
			return fmt.Errorf("build panel: %w", err)
		}
	}
	symbols := panelSymbols(panel, *universe)

	cfg := stockpicker.BacktestConfig{
		Universe:    symbols,
		Start:       start,
		End:         end,
		AsOf:        asOf,
		ForwardDays: 5,
		CostRate:    costRate,
		Source:      "stockpicker",
	}
	outcomes, err := stockpicker.RunBacktest(ctx, cfg, panel)
	if err != nil {
		return fmt.Errorf("run backtest: %w", err)
	}

	coverage := stockpicker.BuildCoverage(cfg, outcomes)
	printCoverage(coverage)
	fmt.Printf("conditions: %s\n", stockpicker.DescribeConditions())

	if *dryRun {
		return nil
	}

	if err := outStore.RecordOutcomes(ctx, outcomes); err != nil {
		return fmt.Errorf("record outcomes: %w", err)
	}
	fmt.Printf("recorded %d outcomes into stock_signal_outcomes\n", len(outcomes))

	summaries, err := stockpicker.AggregateFromStore(ctx, outStore, winStore, aggregationWindow, costRate, minSamples, confidenceLevel, asOf)
	if err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}
	statePath := filepath.Join(*workDir, "data", "state", "stock_win_rate.json")
	if err := stockpicker.WriteStateJSON(statePath, summaries, asOf); err != nil {
		return fmt.Errorf("write state json: %w", err)
	}
	eligible := 0
	for _, s := range summaries {
		if s.CalibrationStatus == stockpicker.CalibrationEligible {
			eligible++
		}
	}
	fmt.Printf("aggregated %d keys into stock_win_rate (%d eligible) -> %s\n", len(summaries), eligible, statePath)
	return nil
}

// parseDate parses YYYY-MM-DD; empty falls back to def (UTC, date only).
func parseDate(s string, def time.Time) (time.Time, error) {
	if s == "" {
		return time.Date(def.Year(), def.Month(), def.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD)", s)
	}
	return d, nil
}

// countExistingOutcomes counts stockpicker rows already recorded in the range.
func countExistingOutcomes(ctx context.Context, db *sql.DB, start, end time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stock_signal_outcomes
		WHERE source LIKE 'stockpicker-%'
		  AND trigger_date >= ? AND trigger_date <= ?`,
		start.Format("2006-01-02"), end.Format("2006-01-02"),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count outcomes: %w", err)
	}
	return n, nil
}

// printCoverage prints the per-condition sample counts (PR verification input).
func printCoverage(rep stockpicker.CoverageReport) {
	fmt.Printf("coverage: asof=%s universe=%d triggers=%s..%s total_outcomes=%d\n",
		rep.AsOf, rep.UniverseSize, rep.Start, rep.End, rep.TotalOutcomes)
	for src, n := range rep.BySource {
		fmt.Printf("  %-40s %d\n", src, n)
	}
}
