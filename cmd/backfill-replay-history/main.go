// Command backfill-replay-history extends the 44-symbol replay universe
// (data/replay/tw_extended_90days.jsonl) backward to 2024-07-01 using the
// FinMind 2020–2024 OHLCV snapshot (data/replay/finmind_2020_2024.jsonl),
// and optionally merges extra JSONL sources for gap patching.
//
// R6 scope (Phase C): the extended 90-day universe only covers 2024-07-01
// onward for 8 core symbols; most of the other 36 symbols start at
// 2025-10 / 2026-01 / 2026-04, and 1216/2357/3231 are interrupted at
// 2026-04-24. FinMind historical data (2020-01 → 2024-12-31) can close the
// 2024-07-01 → 2024-12-31 window for the 29 symbols it covers.
//
// Behavior:
//   - FinMind rows inside [--start, --end] are converted to replay format
//     (date gains the T00:00:00Z suffix, source="finmind_historical").
//   - Merge is keyed by (symbol, normalized date); existing replay rows
//     always win, so twse_open_data_csv data is never overwritten.
//   - --extra sources are merged the same way (existing rows still win);
//     this is the hook for patching interrupted symbols once a verified
//     TWSE open-data batch is available.
//   - --symbols restricts which symbols the FinMind window applies to
//     (empty = all symbols present in the replay target).
//   - Output is written sorted by (date, symbol); --dry-run reports only.
//
// Provenance: backfill is recorded by the JSONL diff itself (data files are
// gitignored runtime state); no schema mutation is introduced.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// replayBar matches the replay JSONL schema
// ({"date":"2024-07-01T00:00:00Z","symbol":"2454.TW","name":"聯發科",
//
//	"open":1300,"high":1308.52,"low":1288.21,"close":1296.82,
//	"volume":114432867,"source":"twse_open_data_csv"}).
type replayBar struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
	Source string  `json:"source"`
}

const finmindSource = "finmind_historical"

// key normalizes (symbol, date) for dedup: date keeps only the YYYY-MM-DD
// prefix so replay ("2024-07-01T00:00:00Z") and FinMind ("2024-07-01") rows
// collide as expected.
func key(symbol, date string) string {
	if i := strings.IndexByte(date, 'T'); i >= 0 {
		date = date[:i]
	}
	return symbol + "|" + date
}

// loadBars reads a JSONL replay file, skipping blank lines and non-JSON
// trailer lines (e.g. "Total 2335 stock records ...").
func loadBars(path string) ([]replayBar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var bars []replayBar
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var b replayBar
		if err := json.Unmarshal([]byte(line), &b); err != nil {
			// Tolerate non-JSON trailer lines; they are not replay bars.
			continue
		}
		if b.Date == "" || b.Symbol == "" {
			continue
		}
		bars = append(bars, b)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return bars, nil
}

// convertFinMindBar maps one FinMind row into replay format when its date is
// inside [start, end]. name is taken from the replay universe when the symbol
// is already known there (FinMind names are empty after the first row).
func convertFinMindBar(b replayBar, start, end, name string) (replayBar, bool) {
	if b.Date < start || b.Date > end {
		return replayBar{}, false
	}
	b.Date += "T00:00:00Z"
	b.Name = name
	b.Source = finmindSource
	return b, true
}

// mergeBars merges additions into existing rows. Existing rows always win on
// (symbol, date); symbolFilter (when non-empty) restricts which addition
// symbols are accepted. Returns the merged slice plus added/skipped counts.
func mergeBars(existing []replayBar, additions []replayBar, symbolFilter map[string]bool) ([]replayBar, int, int) {
	index := make(map[string]bool, len(existing))
	merged := make([]replayBar, 0, len(existing)+len(additions))
	merged = append(merged, existing...)
	for _, b := range existing {
		index[key(b.Symbol, b.Date)] = true
	}
	added, skipped := 0, 0
	for _, b := range additions {
		if len(symbolFilter) > 0 && !symbolFilter[b.Symbol] {
			continue
		}
		k := key(b.Symbol, b.Date)
		if index[k] {
			skipped++
			continue
		}
		index[k] = true
		merged = append(merged, b)
		added++
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Date != merged[j].Date {
			return merged[i].Date < merged[j].Date
		}
		return merged[i].Symbol < merged[j].Symbol
	})
	return merged, added, skipped
}

// universeNames returns the first observed Name per symbol (used to carry
// Chinese names onto FinMind rows for symbols already in the universe).
func universeNames(bars []replayBar) map[string]string {
	names := make(map[string]string)
	for _, b := range bars {
		if _, ok := names[b.Symbol]; !ok && b.Name != "" {
			names[b.Symbol] = b.Name
		}
	}
	return names
}

type coverage struct {
	first string
	last  string
	count int
}

func summarize(bars []replayBar) map[string]coverage {
	cov := make(map[string]coverage)
	for _, b := range bars {
		d := b.Date
		if i := strings.IndexByte(d, 'T'); i >= 0 {
			d = d[:i]
		}
		c := cov[b.Symbol]
		if c.count == 0 || d < c.first {
			c.first = d
		}
		if d > c.last {
			c.last = d
		}
		c.count++
		cov[b.Symbol] = c
	}
	return cov
}

func writeBars(path string, bars []replayBar) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, b := range bars {
		if err := enc.Encode(b); err != nil {
			return fmt.Errorf("encode %s: %w", path, err)
		}
	}
	return w.Flush()
}

func run(args []string) error {
	fs := flag.NewFlagSet("backfill-replay-history", flag.ContinueOnError)
	replayPath := fs.String("replay", "data/replay/tw_extended_90days.jsonl", "replay universe JSONL (read + default write target)")
	finmindPath := fs.String("finmind", "data/replay/finmind_2020_2024.jsonl", "FinMind historical JSONL source")
	start := fs.String("start", "2024-07-01", "FinMind window start (YYYY-MM-DD, inclusive)")
	end := fs.String("end", "2024-12-31", "FinMind window end (YYYY-MM-DD, inclusive)")
	symbolsFlag := fs.String("symbols", "", "comma-separated symbol filter for the FinMind window (empty = all symbols in replay target)")
	var extra multiFlag
	fs.Var(&extra, "extra", "extra JSONL source to merge (repeatable; existing rows win)")
	outPath := fs.String("out", "", "output path (default: -replay, in-place update)")
	dryRun := fs.Bool("dry-run", false, "report only, do not write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outPath == "" {
		*outPath = *replayPath
	}

	existing, err := loadBars(*replayPath)
	if err != nil {
		return err
	}
	finmind, err := loadBars(*finmindPath)
	if err != nil {
		return err
	}

	names := universeNames(existing)
	before := summarize(existing)

	var symbolFilter map[string]bool
	if *symbolsFlag != "" {
		symbolFilter = make(map[string]bool)
		for _, s := range strings.Split(*symbolsFlag, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				symbolFilter[s] = true
			}
		}
	} else {
		// Default: only symbols already in the replay universe (the 44-target
		// set) may be backfilled — FinMind's other symbols must not leak in.
		symbolFilter = make(map[string]bool, len(before))
		for sym := range before {
			symbolFilter[sym] = true
		}
	}

	converted := 0
	var additions []replayBar
	for _, b := range finmind {
		conv, ok := convertFinMindBar(b, *start, *end, names[b.Symbol])
		if !ok {
			continue
		}
		converted++
		additions = append(additions, conv)
	}

	merged, added, skipped := mergeBars(existing, additions, symbolFilter)

	for _, path := range extra {
		extraBars, err := loadBars(path)
		if err != nil {
			return err
		}
		var n int
		merged, n, _ = mergeBars(merged, extraBars, nil)
		added += n
	}

	after := summarize(merged)

	fmt.Printf("backfill-replay-history [%s → %s]\n", *start, *end)
	fmt.Printf("  replay target : %s (%d rows, %d symbols)\n", *replayPath, len(existing), len(before))
	fmt.Printf("  finmind source : %s (%d rows scanned, %d in window)\n", *finmindPath, len(finmind), converted)
	fmt.Printf("  merge result   : +%d added, %d skipped (existing rows won)\n", added, skipped)
	fmt.Printf("  output         : %s (%d rows, %d symbols) dry-run=%v\n", *outPath, len(merged), len(after), *dryRun)

	fmt.Printf("\n%-10s %-12s %-12s %-8s %-12s %-12s %-8s\n", "symbol", "before_first", "before_last", "before_n", "after_first", "after_last", "after_n")
	for _, sym := range sortedSymbols(before, after) {
		pre, post := before[sym], after[sym]
		marker := ""
		if pre.count == 0 || pre.first != post.first || pre.count != post.count {
			marker = "  <--"
		}
		fmt.Printf("%-10s %-12s %-12s %-8d %-12s %-12s %-8d%s\n",
			sym, pre.first, pre.last, pre.count, post.first, post.last, post.count, marker)
	}

	if *dryRun {
		return nil
	}
	return writeBars(*outPath, merged)
}

func sortedSymbols(before, after map[string]coverage) []string {
	set := make(map[string]bool)
	for s := range before {
		set[s] = true
	}
	for s := range after {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// multiFlag collects repeatable string flags.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("backfill-replay-history: %v", err)
	}
}
