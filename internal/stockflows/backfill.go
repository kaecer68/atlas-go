// Package stockflows maintains the per-symbol T86 institutional-flow store
// (data/state/stock_flows/<symbol>.json) that backs two stockpicker consumers:
// the flow condition panel (internal/stockpicker) and the win-rate flow gate
// (internal/orchestrator StockpickerWinrateExecutor).
//
// Issue #1945: the store was written by cmd/backfill-stockpicker-flows only —
// a manual one-shot CLI that nothing scheduled and no ops doc referenced. It
// stopped at 2026-08-27 and nothing noticed, because both consumers read the
// newest stored point without asking how old it was. This package is the
// single implementation shared by the CLI (manual backfill / gap repair) and
// the scheduler task stockpicker_flows_update (daily incremental refresh), so
// the store can no longer depend on an unregistered tool to stay current.
//
// The CLI walks every weekday in [start,end] (P1: Taiwan public holidays are
// simplified to weekends) and fetches the whole market in one T86 request per
// day. Each day's rows are merged into the per-symbol files keyed by date, so
// reruns are idempotent.
package stockflows

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	// DefaultMinRows is the minimum rows a trading day's T86 response must
	// carry; fewer fails the run (anti-fake-success).
	DefaultMinRows = 500
	// DefaultSleep paces TWSE requests between days.
	DefaultSleep = 3 * time.Second
	// DefaultLookbackDays seeds the refresh window when the store is missing
	// or empty: fetch the last N calendar days instead of guessing a far
	// past start.
	DefaultLookbackDays = 7
)

// DayStatus values reported per processed weekday.
const (
	StatusOK   = "ok"
	StatusSkip = "skip"
	StatusFail = "fail"
)

// DayResult records one date's backfill outcome for the end-of-run summary.
type DayResult struct {
	Date   string // YYYY-MM-DD
	Rows   int
	Status string // ok / skip / fail
}

// Result reports what a run did, including on the error paths (the caller
// prints it).
type Result struct {
	Days       []DayResult
	Written    map[string]bool // symbols whose file exists after the run
	FlowPoints int             // total flow points across the store after the run
}

// Config configures one backfill/refresh run.
type Config struct {
	WorkDir string    // atlas work directory (repo root); the store lives under it
	Start   time.Time // first weekday (inclusive)
	End     time.Time // last weekday (inclusive)
	// Symbols restricts the run to a symbol set (nil = every symbol in the
	// T86 response).
	Symbols map[string]bool
	// MinRows is the per-day minimum row count; <= 0 → DefaultMinRows.
	MinRows int
	// Sleep paces requests between days; < 0 → DefaultSleep (0 = no pause).
	Sleep time.Duration
	// DryRun prints the date list without fetching or writing.
	DryRun bool
	// Provider overrides the production TWSE provider (tests inject a stub
	// server); nil creates the production provider.
	Provider *marketdata.TWSECapitalFlowProvider
	// Logf receives per-day progress lines (nil → log.Printf).
	Logf func(format string, args ...any)
	// Outf receives human-facing lines such as the dry-run listing and the
	// verification sample (nil → fmt.Printf).
	Outf func(format string, args ...any)
}

func (c Config) minRows() int {
	if c.MinRows > 0 {
		return c.MinRows
	}
	return DefaultMinRows
}

func (c Config) sleep() time.Duration {
	if c.Sleep < 0 {
		return DefaultSleep
	}
	return c.Sleep
}

func (c Config) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
		return
	}
	//nolint:gosec // G706: operator-supplied CLI flags / scheduler config on
	// an admin tool; logging them for diagnostics is intentional.
	log.Printf(format, args...)
}

func (c Config) outf(format string, args ...any) {
	if c.Outf != nil {
		c.Outf(format, args...)
		return
	}
	fmt.Printf(format, args...)
}

// Run executes the backfill. It returns an error (the CLI exits non-zero) on
// the first hard failure: a fetch error that is not ErrNoData, a weekday with
// fewer than MinRows rows, a file write failure, a verification failure, or a
// window in which no trading day produced any row. Result is populated on
// every path so the caller can print a summary either way.
func Run(ctx context.Context, cfg Config) (res Result, err error) {
	res.Written = map[string]bool{}
	if cfg.WorkDir == "" {
		return res, errors.New("stockflows: workdir is empty")
	}
	if cfg.End.Before(cfg.Start) {
		return res, fmt.Errorf("stockflows: end %s is before start %s",
			cfg.End.Format("2006-01-02"), cfg.Start.Format("2006-01-02"))
	}
	minRows := cfg.minRows()
	days := Weekdays(cfg.Start, cfg.End)
	if cfg.DryRun {
		for _, d := range days {
			cfg.outf("%s\n", d.Format("2006-01-02"))
		}
		cfg.outf("dry-run: %d weekdays in %s..%s; no API calls, no files written\n",
			len(days), cfg.Start.Format("2006-01-02"), cfg.End.Format("2006-01-02"))
		return res, nil
	}

	provider := cfg.Provider
	if provider == nil {
		provider = marketdata.NewTWSECapitalFlowProvider("")
	}
	flowsDir := Dir(cfg.WorkDir)
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		return res, fmt.Errorf("mkdir %s: %w", flowsDir, err)
	}

	res.Days = make([]DayResult, 0, len(days))
	cfg.logf("stockflows: window %s..%s workdir=%s min-rows=%d sleep=%v symbols=%s",
		cfg.Start.Format("2006-01-02"), cfg.End.Format("2006-01-02"), cfg.WorkDir, minRows, cfg.sleep(), symbolDesc(cfg.Symbols))

	for _, d := range days {
		iso := d.Format("2006-01-02")
		flows, ferr := provider.FetchDateFlows(ctx, d.Format("20060102"))
		if ferr != nil {
			if errors.Is(ferr, marketdata.ErrNoData) {
				// TWSE answered with no data: holiday / not yet published.
				// Skip, not a failure (anti-fake-success still fails on
				// weekdays whose data parses to < minRows rows).
				res.Days = append(res.Days, DayResult{Date: iso, Status: StatusSkip})
				cfg.logf("%s rows=0 skip (non-trading day)", iso)
				continue
			}
			res.Days = append(res.Days, DayResult{Date: iso, Status: StatusFail})
			return res, fmt.Errorf("fetch %s: %w", iso, ferr)
		}
		if len(flows) < minRows {
			res.Days = append(res.Days, DayResult{Date: iso, Rows: len(flows), Status: StatusFail})
			return res, fmt.Errorf("date %s: %d rows < min-rows %d (possible fake success)", iso, len(flows), minRows)
		}

		by := GroupFlows(flows, cfg.Symbols)
		dayAdded := 0
		for _, sym := range SortedKeys(by) {
			added, _, merr := MergeSymbolFile(flowsDir, sym, by[sym])
			if merr != nil {
				res.Days = append(res.Days, DayResult{Date: iso, Rows: len(flows), Status: StatusFail})
				return res, fmt.Errorf("date %s: %w", iso, merr)
			}
			dayAdded += added
			res.Written[sym] = true
		}
		res.Days = append(res.Days, DayResult{Date: iso, Rows: len(flows), Status: StatusOK})
		cfg.logf("%s rows=%d symbols=%d new_points=%d", iso, len(flows), len(by), dayAdded)

		if d != days[len(days)-1] && cfg.sleep() > 0 {
			select {
			case <-time.After(cfg.sleep()):
			case <-ctx.Done():
				return res, ctx.Err()
			}
		}
	}

	// Aggregated fake-success gate (PR review P0): if not a single trading day
	// produced rows (e.g. every day was ErrNoData from an invalid range, a
	// systematic stat != OK, or a start before T86's earliest date), the run
	// must fail loudly instead of reporting "OK" with 0 files written.
	if len(res.Written) == 0 {
		return res, fmt.Errorf("no trading day produced data: %d day(s) processed, 0 files written (possible fake success)",
			len(res.Days))
	}

	if err := VerifyFiles(flowsDir, res.Written, cfg.outf); err != nil {
		return res, err
	}
	if res.FlowPoints, err = CountFlowPoints(flowsDir); err != nil {
		return res, err
	}
	return res, nil
}

// Weekdays returns every weekday (Mon–Fri) in [start, end]. P1: Taiwan
// public holidays are simplified to weekends only; a weekday holiday then
// surfaces as a no-data skip during the fetch.
func Weekdays(start, end time.Time) []time.Time {
	var days []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		days = append(days, d)
	}
	return days
}

// GroupFlows buckets per-symbol flows, honoring the optional symbol filter.
func GroupFlows(flows []marketdata.SymbolFlow, filter map[string]bool) map[string][]marketdata.SymbolFlow {
	by := make(map[string][]marketdata.SymbolFlow)
	for _, f := range flows {
		if filter != nil && !filter[f.Symbol] {
			continue
		}
		by[f.Symbol] = append(by[f.Symbol], f)
	}
	return by
}

// symbolDesc renders the optional symbol filter for the run log.
func symbolDesc(filter map[string]bool) string {
	if filter == nil {
		return "all"
	}
	return strings.Join(SortedKeys(filter), ",")
}
