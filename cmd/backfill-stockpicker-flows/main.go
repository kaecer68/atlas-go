// Command backfill-stockpicker-flows backfills per-symbol TWSE T86
// institutional investor flows into data/state/stock_flows/<symbol>.json.
//
// The files are the flow source for run-stockpicker-backtest's real panel:
// {"symbol":"2330","flows":[{"date":"2026-01-05","foreign_net":1500},...]}.
// foreign_net is the foreign-investor net buy/sell in thousands of shares
// (provider convention: TWSE raw share counts / 1e3), matching the PR 1c
// fixture format.
//
// The CLI walks every weekday in [start,end] (P1: Taiwan public holidays are
// simplified to weekends) and fetches the whole market in one T86 request
// per day. Each day's rows are merged into the per-symbol files keyed by
// date, so reruns are idempotent.
//
// Usage:
//
//	backfill-stockpicker-flows -workdir . -start 2026-01-01 -end 2026-08-31
//	backfill-stockpicker-flows -dry-run -start 2012-05-02   # preview dates
//	backfill-stockpicker-flows -symbols 2330,2317 -sleep 5s  # filter + throttle
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	defaultStart   = "2026-01-01"
	defaultMinRows = 500
	defaultSleep   = 3 * time.Second
)

// config carries the CLI flags plus an optional injected provider.
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

// dayResult records one date's backfill outcome for the end-of-run summary.
type dayResult struct {
	date   string // YYYY-MM-DD
	rows   int
	status string // ok / skip / fail
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("backfill-stockpicker-flows: %v", err)
	}
	if err := run(context.Background(), cfg); err != nil {
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
		minRows  = fs.Int("min-rows", defaultMinRows, "minimum rows per trading day; fewer fails the run")
		sleep    = fs.Duration("sleep", defaultSleep, "pause between days (TWSE-friendly throttling)")
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
		for _, s := range strings.Split(*symbols, ",") {
			if s = strings.TrimSpace(s); s != "" {
				cfg.symbols[s] = true
			}
		}
	}
	return cfg, nil
}

// run executes the backfill. It returns an error (main exits non-zero) on
// the first hard failure: a fetch error that is not ErrNoData, a weekday
// with fewer than minRows rows, a file write failure, or a verify failure.
func run(ctx context.Context, cfg config) (err error) {
	days := weekdays(cfg.start, cfg.end)
	if cfg.dryRun {
		for _, d := range days {
			fmt.Println(d.Format("2006-01-02"))
		}
		fmt.Printf("dry-run: %d weekdays in %s..%s; no API calls, no files written\n",
			len(days), cfg.start.Format("2006-01-02"), cfg.end.Format("2006-01-02"))
		return nil
	}

	provider := cfg.provider
	if provider == nil {
		provider = marketdata.NewTWSECapitalFlowProvider("")
	}
	flowsDir := filepath.Join(cfg.workDir, "data", "state", "stock_flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", flowsDir, err)
	}

	results := make([]dayResult, 0, len(days))
	written := map[string]bool{} // symbols whose file exists after the run
	var flowPoints int
	defer func() { printSummary(results, written, flowPoints, err) }()

	//nolint:gosec // G706: values are operator-supplied CLI flags on an
	// admin-run one-shot backfill tool; logging them for diagnostics is
	// intentional (same justification as cmd/atlas-mcp).
	log.Printf("backfill-stockpicker-flows: window %s..%s workdir=%s min-rows=%d sleep=%v symbols=%s",
		cfg.start.Format("2006-01-02"), cfg.end.Format("2006-01-02"), cfg.workDir, cfg.minRows, cfg.sleep, symbolDesc(cfg))

	for _, d := range days {
		iso := d.Format("2006-01-02")
		flows, ferr := provider.FetchDateFlows(ctx, d.Format("20060102"))
		if ferr != nil {
			if errors.Is(ferr, marketdata.ErrNoData) {
				// TWSE answered with no data: holiday / not yet published.
				// Skip, not a failure (anti-fake-success still fails on
				// weekdays whose data parses to < minRows rows).
				results = append(results, dayResult{date: iso, rows: 0, status: "skip"})
				log.Printf("%s rows=0 skip (non-trading day)", iso)
				continue
			}
			results = append(results, dayResult{date: iso, rows: 0, status: "fail"})
			return fmt.Errorf("fetch %s: %w", iso, ferr)
		}
		if len(flows) < cfg.minRows {
			results = append(results, dayResult{date: iso, rows: len(flows), status: "fail"})
			return fmt.Errorf("date %s: %d rows < min-rows %d (possible fake success)", iso, len(flows), cfg.minRows)
		}

		by := groupFlows(flows, cfg.symbols)
		dayAdded := 0
		for _, sym := range sortedMapKeys(by) {
			added, _, merr := mergeSymbolFile(flowsDir, sym, by[sym])
			if merr != nil {
				results = append(results, dayResult{date: iso, rows: len(flows), status: "fail"})
				return fmt.Errorf("date %s: %w", iso, merr)
			}
			dayAdded += added
			written[sym] = true
		}
		results = append(results, dayResult{date: iso, rows: len(flows), status: "ok"})
		log.Printf("%s rows=%d symbols=%d new_points=%d", iso, len(flows), len(by), dayAdded)

		if d != days[len(days)-1] && cfg.sleep > 0 {
			select {
			case <-time.After(cfg.sleep):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	if err := verifyFiles(flowsDir, written); err != nil {
		return err
	}
	if flowPoints, err = countFlowPoints(flowsDir); err != nil {
		return err
	}
	return nil
}

// printSummary reports the per-day rows table, aggregate counts, and the
// run outcome. It runs on every exit path of run (via defer).
func printSummary(results []dayResult, written map[string]bool, flowPoints int, runErr error) {
	var ok, skip, fail int
	for _, r := range results {
		switch r.status {
		case "ok":
			ok++
		case "skip":
			skip++
		case "fail":
			fail++
		}
	}
	fmt.Printf("per-day rows:\n")
	for _, r := range results {
		fmt.Printf("  %s rows=%d %s\n", r.date, r.rows, r.status)
	}
	fmt.Printf("summary: days=%d ok=%d skip=%d fail=%d files_written=%d flow_points=%d\n",
		len(results), ok, skip, fail, len(written), flowPoints)
	if runErr != nil {
		fmt.Printf("result: FAILED — %v\n", runErr)
	} else {
		fmt.Printf("result: OK\n")
	}
}

// weekdays returns every weekday (Mon–Fri) in [start, end]. P1: Taiwan
// public holidays are simplified to weekends only; a weekday holiday then
// surfaces as a no-data skip during the fetch.
func weekdays(start, end time.Time) []time.Time {
	var days []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		days = append(days, d)
	}
	return days
}

// groupFlows buckets per-symbol flows, honoring the optional symbol filter.
func groupFlows(flows []marketdata.SymbolFlow, filter map[string]bool) map[string][]marketdata.SymbolFlow {
	by := make(map[string][]marketdata.SymbolFlow)
	for _, f := range flows {
		if filter != nil && !filter[f.Symbol] {
			continue
		}
		by[f.Symbol] = append(by[f.Symbol], f)
	}
	return by
}

// sortedMapKeys returns the keys of m in ascending order.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func symbolDesc(cfg config) string {
	if cfg.symbols == nil {
		return "all"
	}
	return strings.Join(sortedMapKeys(cfg.symbols), ",")
}
