// Command jev-eval-panel exports the canonical industry (L1) prediction panel
// consumed by the Jev shadow-evaluation framework (Stage 1 / E0, issue #1966).
//
// Why this exists
//
//	The question "does Jev (TypeSafe System One) add predictive value for the
//	industry layer?" needs a ground truth that is (a) regenerable by one
//	command and (b) expressed in a caliber the repo already owns — not a new
//	one. Both constraints are satisfied by exporting the panel in Go:
//
//	  - inputs are the canonical industry return series read through
//	    marketdata.SectorIndexReader (the same reader the period detector and
//	    the sector-rotation flag consume), and
//	  - the hit definition / aggregation are the canonical ones
//	    (stockpicker.NetHit → IndustryWinRate: Wilson 95% CI + min_samples
//	    gate + coverage), per docs/specs/industry-hitrate-metric-spec.md §1.
//
// Output (JSONL, one row per trading date × canonical L1 industry)
//
//	Each row carries two structurally separate blocks:
//	  pit      — features knowable at that date's close (trailing returns,
//	             realized volatility, distance from the 60-day high). Computed
//	             from returns with date <= t only.
//	  forward  — the ground truth: the cost-adjusted forward return over the
//	             fixed holding period (5 trading sessions) and its hit flag.
//
//	The evaluator must never feed `forward` into the model state; the split is
//	enforced on the Python side by the CaseView type (scripts/jev_eval).
//
// Flags:
//
//	-dir           sector_index directory (default data/state/sector_index)
//	-start/-end    panel window (YYYY-MM-DD); rows need min-history before start
//	-forward-days  fixed holding period in trading sessions (default 5)
//	-min-history   trailing sessions required before a row is emitted (default 60)
//	-params        parameters JSON for the canonical cost rate + min_samples
//	-cost-rate     override cost rate (<0 = read from -params)
//	-min-samples   override calibration gate (<0 = read from -params)
//	-confidence    Wilson CI confidence (default 0.95)
//	-source        outcome source label of the exported GT rows
//	-out           panel JSONL path (required)
//	-summary-out   canonical aggregate JSON path (optional)
//	-quiet         suppress the human summary on stdout
//
// Read-only: it never writes config, state or production data other than the
// two output files it is asked for.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// seriesPoint is one daily observation of an industry return series.
type seriesPoint struct {
	date string
	ret  float64
}

// defaultSource labels the exported GT rows. It is a distinct source on
// purpose: the canonical industry hit-rate read surface never leaks a
// non-production source, and a reader can tell an evaluation panel from a
// production condition.
const defaultSource = "jev-eval-industry-l1"

// PitFeatures are the state inputs the evaluator may use: only values derived
// from returns dated <= t.
type PitFeatures struct {
	// TrailingReturn5DPct / 20D / 60D are compounded returns over the last
	// 5 / 20 / 60 sessions ending at t (percent).
	TrailingReturn5DPct  float64 `json:"ret_5d_pct"`
	TrailingReturn20DPct float64 `json:"ret_20d_pct"`
	TrailingReturn60DPct float64 `json:"ret_60d_pct"`
	// RealizedVol20DPct is the sample standard deviation of the daily returns
	// over the last 20 sessions (percent).
	RealizedVol20DPct float64 `json:"vol_20d_pct"`
	// DistanceFromHigh60DPct is how far below the trailing 60-session high the
	// reconstructed level sits at t (percent, <= 0).
	DistanceFromHigh60DPct float64 `json:"dist_high_60d_pct"`
	// DailyReturnPct is the return of the session ending at t (percent).
	DailyReturnPct float64 `json:"daily_return_pct"`
	// HistoryDays counts the sessions available up to and including t.
	HistoryDays int `json:"history_days"`
}

// BackwardTruth describes the window immediately BEFORE the case date. It is a
// state fact (knowable at t), exported because the memorisation probe needs a
// ground truth the model can only answer from prior knowledge: the probe asks
// about this window with a state that deliberately omits every price feature.
type BackwardTruth struct {
	BackwardDate      string  `json:"backward_date"`
	BackwardReturn    float64 `json:"backward_return"`
	BackwardNetReturn float64 `json:"backward_net_return"`
	BackwardHit       bool    `json:"backward_hit"`
}

// ForwardTruth is the ground truth block. It is never part of the model state.
type ForwardTruth struct {
	ForwardDate             string  `json:"forward_date"`
	ForwardReturn           float64 `json:"forward_return"`
	ForwardNetReturn        float64 `json:"forward_net_return"`
	Hit                     bool    `json:"hit"`
	ForwardSessions         int     `json:"forward_sessions"`
	ForwardSpanCalendarDays int     `json:"forward_span_calendar_days"`
}

// PanelRow is one exported (date, canonical L1 industry) observation.
type PanelRow struct {
	Date           string        `json:"date"`
	IndustryID     string        `json:"industry_id"`
	IndustryNameZH string        `json:"industry_name_zh,omitempty"`
	HoldDays       int           `json:"hold_days"`
	CostRate       float64       `json:"cost_rate"`
	PIT            PitFeatures   `json:"pit"`
	Backward       BackwardTruth `json:"backward"`
	Forward        ForwardTruth  `json:"forward"`
}

// panelSummary is the canonical aggregate for the exported GT rows.
type panelSummary struct {
	Source         string                               `json:"source"`
	HoldDays       int                                  `json:"hold_days"`
	CostRate       float64                              `json:"cost_rate"`
	MinSamples     int                                  `json:"min_samples"`
	Confidence     float64                              `json:"confidence"`
	Caliber        string                               `json:"caliber"`
	DateStart      string                               `json:"date_start"`
	DateEnd        string                               `json:"date_end"`
	Industries     []stockpicker.IndustryWinRateSummary `json:"industries"`
	Coverage       stockpicker.IndustryCoverage         `json:"coverage"`
	IndustriesSeen []string                             `json:"industries_seen"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "jev-eval-panel: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("jev-eval-panel", flag.ContinueOnError)
	dir := fs.String("dir", "data/state/sector_index", "sector_index directory")
	startStr := fs.String("start", "", "panel start date YYYY-MM-DD (required)")
	endStr := fs.String("end", "", "panel end date YYYY-MM-DD (required)")
	forwardDays := fs.Int("forward-days", stockpicker.DefaultForwardDays, "fixed holding period in trading sessions")
	minHistory := fs.Int("min-history", 60, "trailing sessions required before a row is emitted")
	paramsPath := fs.String("params", "configs/parameters.json", "parameters JSON for cost rate + min_samples")
	costRate := fs.Float64("cost-rate", -1, "round-trip cost rate override (<0 = read from -params)")
	minSamples := fs.Int("min-samples", -1, "calibration min-samples override (<0 = read from -params)")
	confidence := fs.Float64("confidence", 0.95, "Wilson CI confidence")
	source := fs.String("source", defaultSource, "outcome source label of the exported GT rows")
	outPath := fs.String("out", "", "panel JSONL path (required)")
	summaryPath := fs.String("summary-out", "", "canonical aggregate JSON path (optional)")
	quiet := fs.Bool("quiet", false, "suppress the human summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outPath == "" {
		return fmt.Errorf("-out is required")
	}
	if *startStr == "" || *endStr == "" {
		return fmt.Errorf("-start and -end are required")
	}
	start, err := time.Parse("2006-01-02", *startStr)
	if err != nil {
		return fmt.Errorf("parse -start: %w", err)
	}
	end, err := time.Parse("2006-01-02", *endStr)
	if err != nil {
		return fmt.Errorf("parse -end: %w", err)
	}
	if end.Before(start) {
		return fmt.Errorf("-end %s before -start %s", *endStr, *startStr)
	}
	if *forwardDays <= 0 {
		return fmt.Errorf("-forward-days must be positive, got %d", *forwardDays)
	}
	if *minHistory < 0 {
		return fmt.Errorf("-min-history must be non-negative, got %d", *minHistory)
	}

	rate, gates, err := resolveCaliber(*paramsPath, *costRate, *minSamples)
	if err != nil {
		return err
	}

	reader := marketdata.NewSectorIndexReader(*dir)
	allDates, err := reader.AvailableDates()
	if err != nil {
		return fmt.Errorf("read sector index dates: %w", err)
	}
	windowDates := make([]string, 0, len(allDates))
	for _, d := range allDates {
		if d >= *startStr && d <= *endStr {
			windowDates = append(windowDates, d)
		}
	}
	if len(windowDates) == 0 {
		return fmt.Errorf("no sector index dates in [%s, %s] under %s", *startStr, *endStr, *dir)
	}

	// One read for the whole window: the reader is authoritative for the
	// canonical industry id normalisation (native 18-industry files win over
	// the legacy 8-industry ones).
	returns, err := reader.ReadRange(start, end)
	if err != nil {
		return fmt.Errorf("read sector index range: %w", err)
	}
	// Also load the sessions before the window so the first in-window rows
	// have real trailing history instead of a truncated one.
	historyStart := start.AddDate(0, 0, -(*minHistory*3 + 30))
	history, err := reader.ReadRange(historyStart, end)
	if err != nil {
		return fmt.Errorf("read sector index history: %w", err)
	}
	for date, byIndustry := range history {
		if _, ok := returns[date]; ok {
			continue
		}
		returns[date] = byIndustry
	}

	seriesDates := make([]string, 0, len(returns))
	for d := range returns {
		seriesDates = append(seriesDates, d)
	}
	sort.Strings(seriesDates)

	industryIDs := make([]string, 0, len(industry.L1Sectors()))
	for _, id := range industry.L1Sectors() {
		industryIDs = append(industryIDs, string(id))
	}
	sort.Strings(industryIDs)

	rows, outcomes, seen := buildPanel(seriesDates, windowDates, industryIDs, returns, *forwardDays, *minHistory, rate, *source)
	if len(rows) == 0 {
		return fmt.Errorf("panel is empty: no (date, industry) row had both %d trailing sessions and %d forward sessions", *minHistory, *forwardDays)
	}

	if err := writeJSONL(*outPath, rows); err != nil {
		return err
	}

	// Canonical aggregate: the exported rows are already at canonical L1
	// grain, so the resolver is the identity (one "symbol" per row = the L1
	// id itself). The hit/Wilson/min_samples math is the shared canonical one.
	identity := func(symbol string) (string, bool) { return symbol, true }
	report, err := stockpicker.IndustryWinRate(*source, outcomes, identity, rate, gates.minSamples, *confidence)
	if err != nil {
		return fmt.Errorf("canonical industry aggregate: %w", err)
	}
	summary := panelSummary{
		Source:         *source,
		HoldDays:       *forwardDays,
		CostRate:       rate,
		MinSamples:     gates.minSamples,
		Confidence:     *confidence,
		Caliber:        "docs/specs/industry-hitrate-metric-spec.md §1 (hit = forward_return - cost_rate > 0, Wilson 95% CI, min_samples gate)",
		DateStart:      rows[0].Date,
		DateEnd:        rows[len(rows)-1].Date,
		Industries:     report.Industries,
		Coverage:       report.Coverage,
		IndustriesSeen: seen,
	}
	if *summaryPath != "" {
		if err := writeJSON(*summaryPath, summary); err != nil {
			return err
		}
	}

	if !*quiet {
		printSummary(stdout, summary, len(rows))
	}
	return nil
}

type caliberGates struct {
	minSamples int
}

// resolveCaliber reads the canonical cost rate and min_samples from the
// parameters file unless they were overridden on the command line. Reading
// them (instead of hard-coding 0.00585 / 30) is what keeps this panel from
// silently drifting away from the caliber spec.
func resolveCaliber(paramsPath string, costRate float64, minSamples int) (float64, caliberGates, error) {
	if costRate >= 0 && minSamples >= 0 {
		return costRate, caliberGates{minSamples: minSamples}, nil
	}
	params, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return 0, caliberGates{}, fmt.Errorf("load %s (needed for cost rate / min_samples): %w", paramsPath, err)
	}
	if costRate < 0 {
		costRate = params.Stockpicker.Costs.RoundTripPct.Value
	}
	if minSamples < 0 {
		minSamples = params.Stockpicker.Calibration.MinSamples.Value
	}
	return costRate, caliberGates{minSamples: minSamples}, nil
}

// buildPanel computes PIT features and forward truth for every
// (in-window date × industry) that has enough history and forward data.
// It cannot fail: a row without trailing history or forward truth is simply
// not emitted, and the caller treats an empty panel as its own error.
func buildPanel(seriesDates, windowDates, industryIDs []string, returns map[string]map[string]float64,
	forwardDays, minHistory int, costRate float64, source string,
) ([]PanelRow, []stockpicker.SignalOutcome, []string) {
	// Position of each date in the full series so "next N sessions" is exact.
	pos := make(map[string]int, len(seriesDates))
	for i, d := range seriesDates {
		pos[d] = i
	}

	rows := make([]PanelRow, 0, len(windowDates)*len(industryIDs))
	outcomes := make([]stockpicker.SignalOutcome, 0, len(windowDates)*len(industryIDs))
	seen := make([]string, 0, len(industryIDs))

	// Per-industry series of (date, returnPct), ascending.
	byIndustry := make(map[string][]seriesPoint, len(industryIDs))
	for _, d := range seriesDates {
		for id, ret := range returns[d] {
			byIndustry[id] = append(byIndustry[id], seriesPoint{date: d, ret: ret})
		}
	}
	for _, id := range industryIDs {
		sort.Slice(byIndustry[id], func(i, j int) bool { return byIndustry[id][i].date < byIndustry[id][j].date })
		if len(byIndustry[id]) > 0 {
			seen = append(seen, id)
		}
	}

	for _, id := range industryIDs {
		s := byIndustry[id]
		if len(s) == 0 {
			continue
		}
		index := make(map[string]int, len(s))
		for i, p := range s {
			index[p.date] = i
		}
		for _, date := range windowDates {
			i, ok := index[date]
			if !ok || i < minHistory-1 || i < forwardDays {
				continue
			}
			// Forward truth: compound over the next forwardDays sessions of
			// THIS industry's series. Fewer than forwardDays available ⇒ the
			// row has no truth yet and is skipped.
			if i+forwardDays >= len(s) {
				continue
			}
			fwd := 1.0
			for k := 1; k <= forwardDays; k++ {
				fwd *= 1 + s[i+k].ret/100
			}
			fwdReturn := fwd - 1
			fwdDate := s[i+forwardDays].date
			if fwdDate <= date {
				continue
			}

			hist := s[:i+1]
			if len(hist) < minHistory {
				continue
			}
			feat := computeFeatures(hist)

			// Backward window: the H sessions ending at t (exactly the
			// trailing-5-session window). It is a state fact, exported for the
			// memorisation probe, whose ground truth can only be answered from
			// prior knowledge because its state omits every price feature.
			back := 1.0
			for k := 0; k < forwardDays; k++ {
				back *= 1 + s[i-k].ret/100
			}
			backReturn := back - 1
			backDate := s[i-forwardDays].date

			rows = append(rows, PanelRow{
				Date:           date,
				IndustryID:     id,
				IndustryNameZH: zhName(id),
				HoldDays:       forwardDays,
				CostRate:       costRate,
				PIT:            feat,
				Backward: BackwardTruth{
					BackwardDate:      backDate,
					BackwardReturn:    backReturn,
					BackwardNetReturn: backReturn - costRate,
					BackwardHit:       stockpicker.NetHit(backReturn, costRate),
				},
				Forward: ForwardTruth{
					ForwardDate:             fwdDate,
					ForwardReturn:           fwdReturn,
					ForwardNetReturn:        fwdReturn - costRate,
					Hit:                     stockpicker.NetHit(fwdReturn, costRate),
					ForwardSessions:         forwardDays,
					ForwardSpanCalendarDays: calendarDays(date, fwdDate),
				},
			})
			outcomes = append(outcomes, stockpicker.SignalOutcome{
				Symbol:           id,
				TriggerDate:      date,
				ForwardReturn:    fwdReturn,
				NetForwardReturn: fwdReturn - costRate,
				Hit:              stockpicker.NetHit(fwdReturn, costRate),
				CostRate:         costRate,
				Source:           source,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].IndustryID < rows[j].IndustryID
	})
	return rows, outcomes, seen
}

// computeFeatures derives every PIT feature from hist (returns dated <= date).
func computeFeatures(hist []seriesPoint) PitFeatures {
	ret := make([]float64, len(hist))
	for i, p := range hist {
		ret[i] = p.ret
	}
	f := PitFeatures{HistoryDays: len(ret), DailyReturnPct: ret[len(ret)-1]}
	f.TrailingReturn5DPct = compoundTail(ret, 5)
	f.TrailingReturn20DPct = compoundTail(ret, 20)
	f.TrailingReturn60DPct = compoundTail(ret, 60)
	f.RealizedVol20DPct = sampleStd(tail(ret, 20))
	f.DistanceFromHigh60DPct = distanceFromHigh(tail(ret, 60))
	return f
}

// compoundTail compounds the last n returns; when fewer than n exist it
// compounds whatever is there (the caller already enforces min-history).
func compoundTail(ret []float64, n int) float64 {
	w := tail(ret, n)
	v := 1.0
	for _, r := range w {
		v *= 1 + r/100
	}
	return (v - 1) * 100
}

func tail(ret []float64, n int) []float64 {
	if n <= 0 || len(ret) == 0 {
		return nil
	}
	if len(ret) <= n {
		return ret
	}
	return ret[len(ret)-n:]
}

// sampleStd is the sample standard deviation (n-1). Fewer than 2 points → 0.
func sampleStd(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

// distanceFromHigh rebuilds a level series from returns and reports how far
// below its running high it ends (percent, <= 0).
func distanceFromHigh(ret []float64) float64 {
	if len(ret) == 0 {
		return 0
	}
	level, high := 1.0, 1.0
	for _, r := range ret {
		level *= 1 + r/100
		if level > high {
			high = level
		}
	}
	if high <= 0 {
		return 0
	}
	return (level/high - 1) * 100
}

func zhName(id string) string {
	if s, ok := industry.SectorIDFromString(id); ok {
		if name, ok := industry.DisplayZHTw[s]; ok {
			return name
		}
	}
	return ""
}

func calendarDays(a, b string) int {
	da, err1 := time.Parse("2006-01-02", a)
	db, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(db.Sub(da).Hours() / 24)
}

func writeJSONL(path string, rows []PanelRow) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	// Read-only resource release: the project convention drops Close errors on
	// read paths, and the explicit flush below reports a failed write first.
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("encode %s: %w", path, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush %s: %w", path, err)
	}
	return f.Close()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func printSummary(w io.Writer, s panelSummary, rows int) {
	// stdout summaries are best-effort: a broken pipe must not mask the real
	// exit status of the export, so write errors are dropped explicitly.
	emit := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	emit("jev-eval-panel: %d rows, %d industries, %s..%s\n", rows, len(s.IndustriesSeen), s.DateStart, s.DateEnd)
	emit("caliber: hold=%d sessions, cost=%.5f, min_samples=%d, confidence=%.2f\n", s.HoldDays, s.CostRate, s.MinSamples, s.Confidence)
	emit("%-22s %6s %6s %8s %8s %8s %-14s\n", "industry", "obs", "hits", "win_rate", "wilson_lo", "wilson_hi", "calibration")
	for _, r := range s.Industries {
		emit("%-22s %6d %6d %8.4f %8.4f %8.4f %-14s\n",
			r.IndustryID, r.Observations, r.Hits, r.WinRate, r.WilsonLower, r.WilsonUpper, r.CalibrationStatus)
	}
	emit("coverage: %d/%d observations (%.2f%%), %d/%d industries\n",
		s.Coverage.MappedObservations, s.Coverage.TotalObservations, s.Coverage.CoveragePct,
		s.Coverage.MappedSymbols, s.Coverage.TotalSymbols)
}
