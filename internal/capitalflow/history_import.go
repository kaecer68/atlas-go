package capitalflow

// CAL-1 — rolling-store history import builder.
//
// The rolling window that backs the Z-score reference (BK-15 /
// spec §8.5) only accumulates one trading day per Service.Refresh
// call, so a freshly-migrated production store stays in
// "calibrating" with sample_count=1 and a flat Z-score history for
// weeks. This file builds the historical batch that
// (File|Memory)RollingSampleStore.ImportHistory loads in one atomic
// write, from data that already exists in the repo:
//
//   - the real TWSE T86 三大法人買賣超日報 snapshots under
//     data/state/capital_flow/ (foreign / institutional / dealer
//     readings in 億元, i.e. hundred_million_shares), and
//   - the replay trading calendar (data/replay/tw_extended_90days.csv)
//     which defines which dates are valid trading days.
//
// Dimension → source mapping (docs/specs/capital-flow-seven-dimension-spec.md §5):
//
//	ForceForeign       ← foreign_investor_net   (TWSE-T86, REAL)
//	ForceInstitutional ← domestic_fund_net      (TWSE-T86, REAL)
//	ForceDealer        ← dealer_net             (TWSE-T86, REAL)
//	ForceFutures       — needs TAIFEX institutional OI (口數); not in replay/T86.
//	ForceTSMADR        — needs Yahoo-derived TSM ADR premium; not in replay/T86.
//	ForceGovernment    — needs operator-imported 官股行庫 readings; not in replay/T86.
//	ForceRetail        — needs TWSE margin + 當沖 data; not in replay/T86.
//
// The four non-T86 dimensions are deliberately NOT approximated from
// replay close prices: the replay CSV carries only OHLCV, so any
// fabricated value would poison the rolling reference and every
// downstream Z-score. They are reported via ImportReport.NeedsRealSource
// until their real feeds land.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// T86Record is one day of real TWSE T86 三大法人買賣超 readings
// (units: 億元 / hundred_million_shares). Loaded from the snapshot
// JSON files under data/state/capital_flow/.
type T86Record struct {
	TradingDate      string
	ForeignNet       float64
	InstitutionalNet float64
	DealerNet        float64
}

// ImportReport summarizes what a history import actually did so the
// operator can audit dimension coverage and spot missing sources.
type ImportReport struct {
	// ImportedDates is the number of trading dates written.
	ImportedDates int
	// ImportedSamples is the total number of RollingSample entries
	// (3 per imported date: foreign / institutional / dealer).
	ImportedSamples int
	// SkippedDatesNoT86 is the count of replay trading dates inside
	// the window that had no T86 snapshot (real source missing).
	SkippedDatesNoT86 int
	// NeedsRealSource lists dimensions that were NOT imported because
	// no real source file exists yet. Their samples must come from
	// the real feeds once available; do not fabricate them.
	NeedsRealSource []ForceName
	// DateRange is the first..last imported date (empty when nothing
	// was imported).
	DateRange [2]string
}

// LoadT86CapitalFlow reads every T86 snapshot JSON under dir
// (data/state/capital_flow/*.json) and returns them keyed by
// normalized YYYY-MM-DD trading date. Non-T86 files (e.g.
// _metadata.json) are skipped silently.
func LoadT86CapitalFlow(dir string) (map[string]T86Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("history_import: read t86 dir %s: %w", dir, err)
	}
	out := make(map[string]T86Record, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("history_import: read %s: %w", e.Name(), err)
		}
		var raw struct {
			Date             string  `json:"date"`
			ForeignNet       float64 `json:"foreign_investor_net"`
			InstitutionalNet float64 `json:"domestic_fund_net"`
			DealerNet        float64 `json:"dealer_net"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("history_import: parse %s: %w", e.Name(), err)
		}
		if raw.Date == "" {
			// _metadata.json and other non-snapshot files.
			continue
		}
		date, err := normalizeT86Date(raw.Date)
		if err != nil {
			return nil, fmt.Errorf("history_import: %s: %w", e.Name(), err)
		}
		out[date] = T86Record{
			TradingDate:      date,
			ForeignNet:       raw.ForeignNet,
			InstitutionalNet: raw.InstitutionalNet,
			DealerNet:        raw.DealerNet,
		}
	}
	return out, nil
}

// normalizeT86Date maps the two date shapes found in the T86 snapshot
// files ("2026-08-14" and "20260814") to YYYY-MM-DD.
func normalizeT86Date(s string) (string, error) {
	if len(s) == 8 {
		if _, err := time.Parse("20060102", s); err == nil {
			return s[0:4] + "-" + s[4:6] + "-" + s[6:8], nil
		}
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", fmt.Errorf("unrecognized t86 date %q", s)
	}
	return s, nil
}

// LoadReplayTradingDates reads the replay CSV at path (columns
// Date,Code,Name,TradeVolume,Open,High,Low,Close) and returns the
// sorted, unique trading dates (YYYY-MM-DD). A malformed date row is
// a hard error — silently skipping one would silently shrink the
// import window.
func LoadReplayTradingDates(replayPath string) ([]string, error) {
	f, err := os.Open(replayPath)
	if err != nil {
		return nil, fmt.Errorf("history_import: open replay %s: %w", replayPath, err)
	}
	defer func() { _ = f.Close() }()

	seen := make(map[string]struct{})
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if lineNo == 1 {
			// header row: Date,Code,Name,TradeVolume,Open,High,Low,Close
			continue
		}
		cols := strings.Split(line, ",")
		if len(cols) == 0 {
			continue
		}
		date := strings.TrimSpace(cols[0])
		if err := validateTradingDate(date); err != nil {
			return nil, fmt.Errorf("history_import: replay line %d: bad date %q: %w", lineNo, date, err)
		}
		seen[date] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("history_import: read replay %s: %w", replayPath, err)
	}
	dates := make([]string, 0, len(seen))
	for d := range seen {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	return dates, nil
}

// BuildHistorySamples builds the import batch for every replay trading
// date (>= fromDate when non-empty, YYYY-MM-DD inclusive) that has a
// real T86 record, emitting the three dimensions with real sources
// (foreign / institutional / dealer). The remaining four dimensions
// are listed in the returned ImportReport.NeedsRealSource and are
// intentionally not fabricated (see file doc).
func BuildHistorySamples(replayPath, t86Dir, fromDate string) ([]RollingSample, ImportReport, error) {
	var rep ImportReport
	if fromDate != "" {
		if err := validateTradingDate(fromDate); err != nil {
			return nil, rep, fmt.Errorf("history_import: invalid from_date %q: %w", fromDate, err)
		}
	}
	replayDates, err := LoadReplayTradingDates(replayPath)
	if err != nil {
		return nil, rep, err
	}
	t86, err := LoadT86CapitalFlow(t86Dir)
	if err != nil {
		return nil, rep, err
	}
	rep.NeedsRealSource = []ForceName{ForceFutures, ForceTSMADR, ForceGovernment, ForceRetail}

	var samples []RollingSample
	first, last := "", ""
	for _, d := range replayDates {
		if fromDate != "" && d < fromDate {
			continue
		}
		rec, ok := t86[d]
		if !ok {
			rep.SkippedDatesNoT86++
			continue
		}
		samples = append(samples,
			RollingSample{TradingDate: d, Dimension: ForceForeign, RawValue: rec.ForeignNet, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
			RollingSample{TradingDate: d, Dimension: ForceInstitutional, RawValue: rec.InstitutionalNet, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
			RollingSample{TradingDate: d, Dimension: ForceDealer, RawValue: rec.DealerNet, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
		)
		rep.ImportedDates++
		if first == "" {
			first = d
		}
		last = d
	}
	rep.ImportedSamples = len(samples)
	if first != "" {
		rep.DateRange = [2]string{first, last}
	}
	return samples, rep, nil
}

// String renders the report as one human-readable line for the CLI
// and Makefile output.
func (r ImportReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "imported %d dates / %d samples", r.ImportedDates, r.ImportedSamples)
	if r.ImportedDates > 0 {
		fmt.Fprintf(&b, " (%s .. %s)", r.DateRange[0], r.DateRange[1])
	}
	if r.SkippedDatesNoT86 > 0 {
		fmt.Fprintf(&b, "; %d replay dates skipped (no T86 snapshot)", r.SkippedDatesNoT86)
	}
	if len(r.NeedsRealSource) > 0 {
		names := make([]string, len(r.NeedsRealSource))
		for i, d := range r.NeedsRealSource {
			names[i] = string(d)
		}
		fmt.Fprintf(&b, "; dimensions awaiting real sources: %s", strings.Join(names, ", "))
	}
	return b.String()
}
