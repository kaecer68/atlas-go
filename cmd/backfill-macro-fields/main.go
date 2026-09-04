// Command backfill-macro-fields merges side-car state files into the
// per-date macro snapshot files (data/state/macro/YYYY-MM-DD.json):
//
//   - source capital_flow: data/state/capital_flow/<date>.json files
//     (TWSE T86 三大法人, produced by TWSECapitalFlowProvider /
//     cmd/fetch-historical-capital-flow) → foreign_investor_net,
//     foreign_dealer_net, domestic_fund_net, dealer_net, dealer_self_net,
//     dealer_hedging_net.
//   - source taifex_oi: data/state/taifex_oi/<date>.json files (FinMind
//     TaiwanFuturesInstitutionalInvestors, produced by
//     cmd/backfill-taifex-oi-finmind) → foreign_futures_oi_net from the
//     TX contract foreign oi_net.
//
// Purpose: the period detector v2 (internal/portfolio/period_detector.go)
// reads ForeignInvestorNet / ForeignFuturesOI out of the macro snapshots
// via SnapshotToPeriodIndicators + EnrichFromDir. The pre-live snapshots
// (2024-07-01..2026-04-24) were created by backfill-macro-history and only
// carry taiex/tsm_adr, so the detector honestly degraded to fallback
// consolidation for the whole window. This command replays the same
// applyCapitalFlow / applyTaifexInstitutional merge the live gateway
// performs (same symbols), with the macrobackfill merge discipline:
// an existing non-zero value in a snapshot is NEVER overwritten; only
// missing points are filled (zero-valued source readings are skipped —
// a missing holiday/T86 file is indistinguishable from a true zero).
//
// Date tolerance: capital_flow file names come in both YYYYMMDD.json and
// YYYY-MM-DD.json shapes (the fetch tool has used both); source JSON date
// fields also accept both shapes. Snapshots are keyed by the TW trading
// date of the source record. Only existing snapshot files are touched.
//
// Usage:
//
//	backfill-macro-fields -workdir . -source capital_flow -dry-run
//	backfill-macro-fields -workdir . -source taifex_oi -start 2024-07-01
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const logFileName = "backfill_log.jsonl"

// capitalFlowFile mirrors internal/marketdata.TWSECapitalFlow (which the
// storage files use) and cmd/fetch-historical-capital-flow.CapitalFlowData.
// Both shapes share the lowercase snake_case tags, so one struct covers both.
type capitalFlowFile struct {
	Date               string  `json:"date"`
	ForeignInvestorNet float64 `json:"foreign_investor_net"`
	ForeignDealerNet   float64 `json:"foreign_dealer_net"`
	DomesticFundNet    float64 `json:"domestic_fund_net"`
	DealerNet          float64 `json:"dealer_net"`
	DealerSelfNet      float64 `json:"dealer_self_net"`
	DealerHedgingNet   float64 `json:"dealer_hedging_net"`
	TotalNet           float64 `json:"total_net"`
}

// taifexOIFile mirrors the per-date artifact of cmd/backfill-taifex-oi-finmind.
type taifexOIFile struct {
	Date      string `json:"date"`
	Contracts map[string]struct {
		Date    string `json:"date"`
		Foreign struct {
			OINet float64 `json:"oi_net"`
		} `json:"foreign"`
	} `json:"contracts"`
}

// fieldMerge is one snapshot field produced by a source file.
type fieldMerge struct {
	field  string  // snapshot key, e.g. "foreign_investor_net"
	symbol string  // symbol recorded in the MacroDataPoint, e.g. "TAIWAN_FOREIGN"
	value  float64 // zero = no trustworthy reading, point is not written
}

type logEntry struct {
	Date         string  `json:"date"`
	Field        string  `json:"field"`
	Symbol       string  `json:"symbol"`
	Value        float64 `json:"value"`
	Action       string  `json:"action"` // filled | skipped_existing
	Source       string  `json:"source"`
	BackfilledAt string  `json:"backfilled_at"`
}

// runStats reports the outcome of one backfill run.
type runStats struct {
	sources           int
	snapshotsMerged   int
	fieldsFilled      int
	skippedExisting   int
	skippedZeroSource int
	missingSnapshot   int
	errors            int
}

var (
	dashedDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	plainDate  = regexp.MustCompile(`^\d{8}$`)
)

func main() {
	if err := runFromOSArgs(); err != nil {
		var hard *hardError
		if errors.As(err, &hard) {
			log.Fatalf("backfill-macro-fields: %v", err)
		}
		log.Fatalf("backfill-macro-fields: %v", err)
	}
}

// hardError marks a run that must exit non-zero (data errors), versus usage
// errors which Fatalf regardless.
type hardError struct{ err error }

func (e *hardError) Error() string { return e.err.Error() }

func runFromOSArgs() error {
	var (
		workDir = flag.String("workdir", ".", "atlas work directory (repo root)")
		source  = flag.String("source", "", "side-car source: capital_flow | taifex_oi")
		srcDir  = flag.String("dir", "", "source directory under workdir (default: data/state/<source>)")
		start   = flag.String("start", "2024-07-01", "start date YYYY-MM-DD (inclusive, snapshot date)")
		end     = flag.String("end", time.Now().Format("2006-01-02"), "end date YYYY-MM-DD (inclusive, snapshot date)")
		dryRun  = flag.Bool("dry-run", false, "report what would be merged without writing")
	)
	flag.Parse()
	stats, err := run(*workDir, *source, *srcDir, *start, *end, *dryRun)
	if err != nil {
		return err
	}
	if stats != nil {
		log.Printf("backfill-macro-fields done: sources=%d snapshots_merged=%d fields_filled=%d skipped_existing=%d skipped_zero_source=%d missing_snapshot=%d errors=%d",
			stats.sources, stats.snapshotsMerged, stats.fieldsFilled, stats.skippedExisting, stats.skippedZeroSource, stats.missingSnapshot, stats.errors)
	}
	return nil
}

// run executes one merge pass. Kept exported-shaped for tests: returns run
// statistics and a non-nil error when hard data errors occurred.
func run(workDir, source, srcDir, startStr, endStr string, dryRun bool) (*runStats, error) {
	switch source {
	case "capital_flow", "taifex_oi":
	default:
		return nil, fmt.Errorf("-source must be capital_flow or taifex_oi (got %q)", source)
	}
	if srcDir == "" {
		srcDir = "data/state/" + source
	}
	startT, err := time.ParseInLocation("2006-01-02", startStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("parse -start: %w", err)
	}
	endT, err := time.ParseInLocation("2006-01-02", endStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("parse -end: %w", err)
	}
	if endT.Before(startT) {
		return nil, fmt.Errorf("-end %s before -start %s", endStr, startStr)
	}

	srcRoot := filepath.Join(workDir, srcDir)
	outRoot := filepath.Join(workDir, "data", "state", "macro")
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return nil, hardWrap(fmt.Errorf("read %s: %w", srcRoot, err))
	}

	var srcDates []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "_metadata.json" {
			continue
		}
		date, ok := normalizeDate(strings.TrimSuffix(name, ".json"))
		if !ok {
			continue
		}
		if date < startT.Format("2006-01-02") || date > endT.Format("2006-01-02") {
			continue
		}
		srcDates = append(srcDates, date)
	}
	sort.Strings(srcDates)
	log.Printf("macro-fields-backfill: source=%s dir=%s %s..%s files=%d dry=%v",
		source, srcRoot, startStr, endStr, len(srcDates), dryRun)

	if !dryRun {
		if err := os.MkdirAll(outRoot, 0o755); err != nil {
			return nil, hardWrap(err)
		}
	}
	var logf *os.File
	if !dryRun {
		logf, err = os.OpenFile(filepath.Join(outRoot, logFileName),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, hardWrap(err)
		}
		defer func() { _ = logf.Close() }()
	}
	now := time.Now().UTC().Format(time.RFC3339)

	stats := &runStats{sources: len(srcDates)}
	for _, date := range srcDates {
		srcPath := sourcePath(srcRoot, date)
		raw, err := os.ReadFile(srcPath)
		if err != nil {
			log.Printf("  %s: read source: %v", date, err)
			stats.errors++
			continue
		}
		var merges []fieldMerge
		switch source {
		case "capital_flow":
			var f capitalFlowFile
			if err := json.Unmarshal(raw, &f); err != nil {
				log.Printf("  %s: unmarshal %s: %v", date, srcPath, err)
				stats.errors++
				continue
			}
			merges = capitalFlowMerges(&f)
		case "taifex_oi":
			var f taifexOIFile
			if err := json.Unmarshal(raw, &f); err != nil {
				log.Printf("  %s: unmarshal %s: %v", date, srcPath, err)
				stats.errors++
				continue
			}
			merges = taifexOIMerges(&f)
		}
		if len(merges) == 0 {
			stats.skippedZeroSource++
			continue
		}
		snapPath := filepath.Join(outRoot, date+".json")
		if _, err := os.Stat(snapPath); err != nil {
			// macrobackfill rule: only existing snapshot files are touched.
			stats.missingSnapshot++
			continue
		}
		snap := map[string]json.RawMessage{}
		if data, err := os.ReadFile(snapPath); err == nil {
			if err := json.Unmarshal(data, &snap); err != nil {
				log.Printf("  %s: parse snapshot: %v", date, err)
				stats.errors++
				continue
			}
		}
		dirty := false
		for _, m := range merges {
			if m.value == 0 {
				stats.skippedZeroSource++
				continue
			}
			if raw, ok := snap[m.field]; ok {
				var existing struct {
					Value float64 `json:"value"`
				}
				if err := json.Unmarshal(raw, &existing); err == nil && existing.Value != 0 {
					stats.skippedExisting++
					writeLog(logf, logEntry{
						Date: date, Field: m.field, Symbol: m.symbol, Value: m.value,
						Action: "skipped_existing", Source: source, BackfilledAt: now,
					})
					continue
				}
			}
			if !dryRun {
				data, err := json.Marshal(map[string]interface{}{
					"symbol":     m.symbol,
					"value":      m.value,
					"change_pct": 0,
					"timestamp":  timestampFor(date),
				})
				if err != nil {
					stats.errors++
					continue
				}
				snap[m.field] = data
			}
			stats.fieldsFilled++
			dirty = true
			writeLog(logf, logEntry{
				Date: date, Field: m.field, Symbol: m.symbol, Value: m.value,
				Action: "filled", Source: source, BackfilledAt: now,
			})
		}
		if dirty && !dryRun {
			out, err := json.MarshalIndent(snap, "", "  ")
			if err != nil {
				stats.errors++
				continue
			}
			if err := os.WriteFile(snapPath, append(out, '\n'), 0o644); err != nil {
				log.Printf("  %s: write snapshot: %v", date, err)
				stats.errors++
				continue
			}
			stats.snapshotsMerged++
		} else if dirty {
			stats.snapshotsMerged++
		}
	}
	log.Printf("  done: sources=%d snapshots_merged=%d fields_filled=%d skipped_existing=%d skipped_zero_source=%d no_snapshot=%d errors=%d",
		stats.sources, stats.snapshotsMerged, stats.fieldsFilled, stats.skippedExisting, stats.skippedZeroSource, stats.missingSnapshot, stats.errors)
	if stats.errors > 0 {
		return stats, hardWrap(fmt.Errorf("%d hard errors", stats.errors))
	}
	return stats, nil
}

func hardWrap(err error) error { return &hardError{err: err} }

// sourcePath resolves the source file for a normalized snapshot date,
// trying YYYYMMDD.json and YYYY-MM-DD.json names.
func sourcePath(root, date string) string {
	compact := strings.ReplaceAll(date, "-", "")
	for _, cand := range []string{compact + ".json", date + ".json"} {
		p := filepath.Join(root, cand)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(root, compact+".json")
}

// normalizeDate maps a file base name (or JSON date string) to YYYY-MM-DD.
func normalizeDate(s string) (string, bool) {
	if dashedDate.MatchString(s) {
		return s, true
	}
	if plainDate.MatchString(s) {
		if t, err := time.ParseInLocation("20060102", s, time.Local); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// timestampFor returns the TW-midnight Unix timestamp for a snapshot date
// (same convention applyCapitalFlow uses for the point timestamp).
func timestampFor(date string) int64 {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// capitalFlowMerges mirrors gateway_adapter.applyCapitalFlow: every field
// present in the file maps to its snapshot point with the same live symbol.
func capitalFlowMerges(f *capitalFlowFile) []fieldMerge {
	return []fieldMerge{
		{field: "foreign_investor_net", symbol: "TAIWAN_FOREIGN", value: f.ForeignInvestorNet},
		{field: "foreign_dealer_net", symbol: "TAIWAN_FOREIGN_DEALER", value: f.ForeignDealerNet},
		{field: "domestic_fund_net", symbol: "TAIWAN_DOMESTIC", value: f.DomesticFundNet},
		{field: "dealer_net", symbol: "TAIWAN_DEALER", value: f.DealerNet},
		{field: "dealer_self_net", symbol: "TAIWAN_DEALER_SELF", value: f.DealerSelfNet},
		{field: "dealer_hedging_net", symbol: "TAIWAN_DEALER_HEDGING", value: f.DealerHedgingNet},
	}
}

// taifexOIMerges mirrors gateway_adapter.applyTaifexInstitutional: the TX
// contract foreign oi_net becomes foreign_futures_oi_net (TX_FOREIGN_OI_NET).
func taifexOIMerges(f *taifexOIFile) []fieldMerge {
	tx, ok := f.Contracts["TX"]
	if !ok {
		return nil
	}
	return []fieldMerge{
		{field: "foreign_futures_oi_net", symbol: "TX_FOREIGN_OI_NET", value: tx.Foreign.OINet},
	}
}

func writeLog(f *os.File, e logEntry) {
	if f == nil {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
}
