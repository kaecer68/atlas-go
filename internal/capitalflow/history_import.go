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
	// (3 per imported date: foreign / institutional / dealer, +1 when
	// a government reading exists for the date).
	ImportedSamples int
	// GovImportedDates is how many dates contributed a ForceGovernment
	// sample (subset of ImportedDates).
	GovImportedDates int
	// FuturesImportedDates is how many dates contributed a ForceFutures
	// sample from real TAIFEX institutional OI snapshots (via
	// BuildHistorySamplesExt; 0 when -oi was not given).
	FuturesImportedDates int
	// TSMADRImportedDates is how many dates contributed a ForceTSMADR
	// sample from real Yahoo-derived TSM ADR macro snapshots (via
	// BuildHistorySamplesExt; 0 when -macro was not given).
	TSMADRImportedDates int
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

// LoadGovernmentFlow reads every government_flow reading JSON under dir
// (data/state/government_flow/YYYYMMDD.json) and returns them keyed by
// normalized YYYY-MM-DD trading date with TotalNet converted to 億元
// (÷1e8, matching the snapshot GovernmentNet semantics used by
// scoreGovernment). Skipped: *_insurance.json and *_brokers.json (the
// suffix check excludes them), and any non-reading files.
func LoadGovernmentFlow(dir string) (map[string]float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// No government_flow readings yet — the government dimension
			// simply stays out of the import (T86 trio still imported).
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("history_import: read gov dir %s: %w", dir, err)
	}
	out := make(map[string]float64, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") ||
			strings.HasSuffix(name, "_insurance.json") ||
			strings.HasSuffix(name, "_brokers.json") {
			continue
		}
		// Only YYYYMMDD.json (8 digits before .json).
		base := strings.TrimSuffix(name, ".json")
		if len(base) != 8 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("history_import: read %s: %w", name, err)
		}
		var raw struct {
			TotalNet int64  `json:"total_net"`
			Date     string `json:"date"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("history_import: parse %s: %w", name, err)
		}
		if raw.Date == "" {
			continue
		}
		date, err := normalizeT86Date(raw.Date)
		if err != nil {
			return nil, fmt.Errorf("history_import: %s: %w", name, err)
		}
		out[date] = float64(raw.TotalNet) / 1e8
	}
	return out, nil
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

// LoadFuturesOI reads every TAIFEX institutional OI day snapshot under
// dir (data/state/taifex_oi/YYYY-MM-DD.json, produced by
// cmd/backfill-taifex-oi-finmind or the taifex_institutional adapter)
// and returns the TX contract foreign open-interest net (口數) keyed by
// normalized trading date. Files without a TX contract are skipped.
// A missing directory yields an empty map (dimension stays out of the
// import) — never a fabricated value.
func LoadFuturesOI(dir string) (map[string]float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("history_import: read futures OI dir %s: %w", dir, err)
	}
	out := make(map[string]float64, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("history_import: read %s: %w", e.Name(), err)
		}
		var raw struct {
			Date      string `json:"date"`
			Contracts map[string]struct {
				Foreign struct {
					OINet int64 `json:"oi_net"`
				} `json:"foreign"`
			} `json:"contracts"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("history_import: parse %s: %w", e.Name(), err)
		}
		if raw.Date == "" {
			continue
		}
		date, err := normalizeT86Date(raw.Date)
		if err != nil {
			return nil, fmt.Errorf("history_import: %s: %w", e.Name(), err)
		}
		tx, ok := raw.Contracts["TX"]
		if !ok {
			continue
		}
		out[date] = float64(tx.Foreign.OINet)
	}
	return out, nil
}

// LoadMacroTSMADR reads the dated macro snapshots under dir
// (data/state/macro/YYYY-MM-DD.json) and returns the TSM ADR daily
// change percent keyed by trading date. latest.json / previous.json
// and zero-valued points are skipped: the TSMADR dimension's raw
// value is exactly snap.TSMADR.ChangePct (see scoreTSMADR).
func LoadMacroTSMADR(dir string) (map[string]float64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("history_import: read macro dir %s: %w", dir, err)
	}
	out := make(map[string]float64, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "latest.json" || name == "previous.json" {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if err := validateTradingDate(date); err != nil {
			continue // not a dated snapshot
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("history_import: read %s: %w", name, err)
		}
		var raw struct {
			TSMADR struct {
				ChangePct float64 `json:"change_pct"`
			} `json:"tsm_adr"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("history_import: parse %s: %w", name, err)
		}
		if raw.TSMADR.ChangePct == 0 {
			continue
		}
		out[date] = raw.TSMADR.ChangePct
	}
	return out, nil
}

// BuildHistorySamples extends the CAL-1 batch with the two dimensions
// whose real feeds landed: ForceFutures (TAIFEX institutional OI
// snapshots under oiDir, TX foreign oi_net in 口數) and ForceTSMADR
// (Yahoo-derived TSM ADR daily change percent from dated macro
// snapshots under macroDir). Unlike the T86 trio, these dimensions are
// NOT filtered by fromDate: their source snapshots are real readings
// across their whole range (the fromDate placeholder exclusion only
// applies to the early synthetic T86 files). Dates outside the replay
// trading calendar are still skipped. Retail remains a real-source
// gap (TWSE margin + 當沖 history) and is reported via NeedsRealSource.
func BuildHistorySamplesExt(replayPath, t86Dir, govDir, fromDate, oiDir, macroDir string) ([]RollingSample, ImportReport, error) {
	samples, rep, err := BuildHistorySamples(replayPath, t86Dir, govDir, fromDate)
	if err != nil {
		return nil, rep, err
	}
	oi, err := LoadFuturesOI(oiDir)
	if err != nil {
		return nil, rep, err
	}
	adr, err := LoadMacroTSMADR(macroDir)
	if err != nil {
		return nil, rep, err
	}
	replayDates, err := LoadReplayTradingDates(replayPath)
	if err != nil {
		return nil, rep, err
	}

	futDates := make(map[string]struct{})
	adrDates := make(map[string]struct{})
	first, last := "", ""
	for _, d := range replayDates {
		if v, ok := oi[d]; ok {
			samples = append(samples, RollingSample{
				TradingDate: d,
				Dimension:   ForceFutures,
				RawValue:    v,
				Unit:        "contracts",
				SourceID:    SourceFinMindFutOI,
			})
			futDates[d] = struct{}{}
			rep.FuturesImportedDates++
		}
		if v, ok := adr[d]; ok {
			samples = append(samples, RollingSample{
				TradingDate: d,
				Dimension:   ForceTSMADR,
				RawValue:    v,
				Unit:        "percent",
				SourceID:    SourceYahoo,
			})
			adrDates[d] = struct{}{}
			rep.TSMADRImportedDates++
		}
		if _, ok := futDates[d]; ok || first == "" {
			if first == "" {
				first = d
			}
			last = d
		}
	}
	rep.ImportedSamples = len(samples)

	// Drop the dimensions that now have real sources from NeedsRealSource.
	remaining := rep.NeedsRealSource[:0]
	for _, dim := range rep.NeedsRealSource {
		switch dim {
		case ForceFutures:
			if len(futDates) == 0 {
				remaining = append(remaining, dim)
			}
		case ForceTSMADR:
			if len(adrDates) == 0 {
				remaining = append(remaining, dim)
			}
		default:
			remaining = append(remaining, dim)
		}
	}
	rep.NeedsRealSource = remaining

	if first != "" {
		// Ext range = union coverage of the appended dimensions (may be
		// wider than the T86-only range).
		if rep.DateRange[0] == "" || first < rep.DateRange[0] {
			rep.DateRange[0] = first
		}
		if last > rep.DateRange[1] {
			rep.DateRange[1] = last
		}
	}
	return samples, rep, nil
}

// BuildHistorySamples builds the import batch for every replay trading
// date (>= fromDate when non-empty, YYYY-MM-DD inclusive) that has a
// real T86 record, emitting the three dimensions with real sources
// (foreign / institutional / dealer). The remaining four dimensions
// are listed in the returned ImportReport.NeedsRealSource and are
// intentionally not fabricated (see file doc).
func BuildHistorySamples(replayPath, t86Dir, govDir, fromDate string) ([]RollingSample, ImportReport, error) {
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
	gov, err := LoadGovernmentFlow(govDir)
	if err != nil {
		return nil, rep, err
	}

	var samples []RollingSample
	first, last := "", ""
	govDates := make(map[string]struct{})
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
		// CAL-1 government extension (2026-08-26): emit ForceGovernment
		// from data/state/government_flow/YYYYMMDD.json (total_net → 億元).
		// Dates with a gov reading contribute a 4th sample; dates without
		// one keep the T86-only trio (government stays partial).
		if govVal, ok := gov[d]; ok {
			samples = append(samples, RollingSample{
				TradingDate: d,
				Dimension:   ForceGovernment,
				RawValue:    govVal,
				Unit:        "hundred_million_shares",
				SourceID:    SourceGovernmentOperator,
			})
			govDates[d] = struct{}{}
			rep.GovImportedDates++
		}
		rep.ImportedDates++
		if first == "" {
			first = d
		}
		last = d
	}
	rep.ImportedSamples = len(samples)

	// NeedsRealSource: dimensions whose real feed never landed. Government
	// is dropped from the list when at least one gov reading was imported.
	rep.NeedsRealSource = []ForceName{ForceFutures, ForceTSMADR, ForceRetail}
	if len(govDates) == 0 {
		rep.NeedsRealSource = append(rep.NeedsRealSource, ForceGovernment)
	}

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
	if r.GovImportedDates > 0 {
		fmt.Fprintf(&b, "; government: %d dates imported (media-curated readings)", r.GovImportedDates)
	}
	if r.FuturesImportedDates > 0 {
		fmt.Fprintf(&b, "; futures OI: %d dates imported (TX foreign oi_net)", r.FuturesImportedDates)
	}
	if r.TSMADRImportedDates > 0 {
		fmt.Fprintf(&b, "; tsm_adr: %d dates imported (Yahoo daily change%%)", r.TSMADRImportedDates)
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
