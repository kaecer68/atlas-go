// Command run-stockpicker-backtest runs the stockpicker panel backtest +
// win-rate aggregation job after market close.
//
// Usage (from the repo root):
//
//	run-stockpicker-backtest -workdir . -start 2026-01-01 -end 2026-08-26 -asof 2026-08-26
//
// Flags: -workdir, -start, -end, -asof, -force, -dry-run, -universe, -backend,
// -expect-db, -conditions, -list-conditions. The job:
//
//  1. replays the selected conditions (PR 2a configurable engine; default
//     foreign-3d-net-buy, momentum-20d-positive) over point-in-time price bars
//     (backend-aware quotes)
//     and per-symbol T86 flows (data/state/stock_flows/<symbol>.json), and
//  2. writes SignalOutcome rows into stock_signal_outcomes (idempotent:
//     ON CONFLICT DO NOTHING), then aggregates per (symbol, source) into
//     stock_win_rate and data/state/stock_win_rate.json.
//
// Outcomes and win-rate rows always land in a job-local SQLite artifact
// (data/state/atlas.db), never in the postgres target; quotes are
// backend-aware (sqlite | postgres). When the postgres backend is used the
// job prints current_database + inet_server_port at startup and aborts
// before running migrations unless -expect-db matches the actual database
// (M12 target guard).
//
// Same-day rerun skips unless -force is given. The pipeline itself lives in
// internal/stockpicker.RunDailyUpdate and is shared with the
// stockpicker_daily_update background task (PR 2e); this command is the CLI
// entry point for the same code path.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

// run parses CLI flags and delegates to runWithPanel. panel==nil builds the
// real point-in-time panel from the workdir (backend-aware quotes + flow files).
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
	backendFlag := fs.String("backend", "", "quote backend: sqlite | postgres (default: ATLAS_STORE_BACKEND env, then job-local sqlite)")
	expectDB := fs.String("expect-db", "", "assert the postgres database name before migrations (e.g. atlas); aborts with the actual current_database on mismatch")
	conditionsFlag := fs.String("conditions", "", "comma-separated condition IDs to run (default: foreign-3d-net-buy,momentum-20d-positive)")
	listConditions := fs.Bool("list-conditions", false, "print the registered conditions and exit")
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

	if *listConditions {
		paramsPath := filepath.Join(*workDir, "configs", "parameters.json")
		if _, err := os.Stat(paramsPath); err != nil {
			return fmt.Errorf("parameters config %s: %w", paramsPath, err)
		}
		params, err := config.LoadParametersConfig(paramsPath)
		if err != nil {
			return fmt.Errorf("load parameters: %w", err)
		}
		fmt.Print(listConditionsText(stockpicker.NewDefaultConditionRegistry(&params.Stockpicker.Conditions)))
		return nil
	}

	idem := stockpicker.IdempotencyRange
	if *force {
		idem = stockpicker.IdempotencyNone
	}

	res, err := stockpicker.RunDailyUpdate(ctx, stockpicker.RunDailyOptions{
		WorkDir:     *workDir,
		Backend:     *backendFlag,
		ExpectDB:    *expectDB,
		Idempotency: idem,
		DryRun:      *dryRun,
		Universe:    *universe,
		Conditions:  *conditionsFlag,
		AsOf:        asOf,
		Start:       start,
		End:         end,
		Panel:       panel,
	})
	if err != nil {
		return err
	}
	if res.Skipped {
		log.Printf("skip: %d stockpicker outcomes already exist for %s..%s (use -force to rerun)", res.Existing,
			start.Format("2006-01-02"), end.Format("2006-01-02"))
		return nil
	}
	printCoverage(res.Coverage)
	fmt.Printf("conditions: %s\n", strings.Join(res.Conditions, ", "))

	if *dryRun {
		log.Printf("dry-run: nothing persisted; a real run would write outcomes to job-local SQLite %s (non-prod artifact), not to the postgres target",
			filepath.Join(*workDir, "data", "state", "atlas.db"))
		return nil
	}

	fmt.Printf("recorded %d outcomes into stock_signal_outcomes\n", res.Outcomes)
	fmt.Printf("aggregated %d keys into stock_win_rate (%d eligible) -> %s\n", res.Keys, res.Eligible, res.StatePath)
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
