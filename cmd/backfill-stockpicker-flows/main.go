// Command backfill-stockpicker-flows backfills per-symbol TWSE T86
// institutional investor flows into data/state/stock_flows/<symbol>.json.
//
// The files are the flow source for the stockpicker panel backtest
// (internal/stockpicker/real_panel.go) and for the stockpicker win-rate flow
// gate (internal/orchestrator/stockpicker_winrate_executor.go):
// {"symbol":"2330","flows":[{"date":"2026-01-05","foreign_net":1500},...]}.
// foreign_net is the foreign-investor net buy/sell in thousands of shares
// (provider convention: TWSE raw share counts / 1e3), matching the PR 1c
// fixture format.
//
// The run logic lives in internal/stockflows so this CLI and the scheduled
// daily refresh (internal/scheduler stockpicker_flows_update, issue #1945)
// share one implementation. Use the CLI for the initial backfill and for
// repairing a gap wider than the scheduler's window.
//
// Usage:
//
//	backfill-stockpicker-flows -workdir . -start 2026-01-01 -end 2026-08-31
//	backfill-stockpicker-flows -dry-run -start 2012-05-02   # preview dates
//	backfill-stockpicker-flows -symbols 2330,2317 -sleep 5s  # filter + throttle
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/stockflows"
)

const defaultStart = "2026-01-01"

// config carries the CLI flags.
type config struct {
	workDir string
	start   time.Time
	end     time.Time
	dryRun  bool
	symbols map[string]bool // nil = all symbols
	minRows int
	sleep   time.Duration
	// provider overrides the production TWSE provider (tests inject a stub
	// server); nil creates the production provider.
	provider *marketdata.TWSECapitalFlowProvider
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("backfill-stockpicker-flows: %v", err)
	}
	res, err := stockflows.Run(context.Background(), stockflows.Config{
		WorkDir:  cfg.workDir,
		Start:    cfg.start,
		End:      cfg.end,
		DryRun:   cfg.dryRun,
		Symbols:  cfg.symbols,
		MinRows:  cfg.minRows,
		Sleep:    cfg.sleep,
		Provider: cfg.provider,
	})
	printSummary(res, err)
	if err != nil {
		log.Fatalf("backfill-stockpicker-flows: %v", err)
	}
}

func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("backfill-stockpicker-flows", flag.ContinueOnError)
	var (
		workDir  = fs.String("workdir", ".", "atlas work directory (repo root); reads/writes data/state/stock_flows/")
		startStr = fs.String("start", defaultStart, "backfill start date YYYY-MM-DD (inclusive)")
		endStr   = fs.String("end", "", "backfill end date YYYY-MM-DD (inclusive; default: today)")
		dryRun   = fs.Bool("dry-run", false, "print the date list without fetching or writing")
		symbols  = fs.String("symbols", "", "comma-separated symbol filter (empty = all)")
		minRows  = fs.Int("min-rows", stockflows.DefaultMinRows, "minimum rows per trading day; fewer fails the run")
		sleep    = fs.Duration("sleep", stockflows.DefaultSleep, "pause between days (TWSE-friendly throttling)")
	)
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	start, err := time.ParseInLocation("2006-01-02", *startStr, time.Local)
	if err != nil {
		return config{}, fmt.Errorf("parse -start %q: %w", *startStr, err)
	}
	end := time.Now()
	if *endStr != "" {
		end, err = time.ParseInLocation("2006-01-02", *endStr, time.Local)
		if err != nil {
			return config{}, fmt.Errorf("parse -end %q: %w", *endStr, err)
		}
	}
	if end.Before(start) {
		return config{}, fmt.Errorf("-end %s is before -start %s", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}
	if *minRows <= 0 {
		return config{}, fmt.Errorf("-min-rows must be > 0, got %d", *minRows)
	}
	if *sleep < 0 {
		return config{}, fmt.Errorf("-sleep must be >= 0, got %v", *sleep)
	}

	cfg := config{workDir: *workDir, start: start, end: end, dryRun: *dryRun, minRows: *minRows, sleep: *sleep}
	if *symbols != "" {
		cfg.symbols = map[string]bool{}
		for s := range strings.SplitSeq(*symbols, ",") {
			if s = strings.TrimSpace(s); s != "" {
				cfg.symbols[s] = true
			}
		}
	}
	return cfg, nil
}

// printSummary reports the per-day rows table, aggregate counts, and the run
// outcome. It runs on every exit path (success and failure).
func printSummary(res stockflows.Result, runErr error) {
	var ok, skip, fail int
	for _, r := range res.Days {
		switch r.Status {
		case stockflows.StatusOK:
			ok++
		case stockflows.StatusSkip:
			skip++
		case stockflows.StatusFail:
			fail++
		}
	}
	fmt.Printf("per-day rows:\n")
	for _, r := range res.Days {
		fmt.Printf("  %s rows=%d %s\n", r.Date, r.Rows, r.Status)
	}
	fmt.Printf("summary: days=%d ok=%d skip=%d fail=%d files_written=%d flow_points=%d\n",
		len(res.Days), ok, skip, fail, len(res.Written), res.FlowPoints)
	if runErr != nil {
		fmt.Printf("result: FAILED — %v\n", runErr)
	} else {
		fmt.Printf("result: OK\n")
	}
}
