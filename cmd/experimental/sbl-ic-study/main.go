// Command sbl-ic-study measures the per-stock predictive value of SBL
// (securities lending) features against broad-universe close-to-close forward
// returns.
//
// 背景（issue #2093）：現有 29 個 detector 全是大盤／主題級，per-stock 訊號從未
// 被量測；本工具是 k3 顧問事前註冊規格下的「最小量測」，判準寫死在
// criteriaRows()，不隨結果調整。
//
// PIT 紀律（三條都必須成立才算乾淨）：
//   - 特徵 SBL 只用 date < t 的觀測（t = 形成訊號的交易日）
//   - 特徵 TDCC 再退 -tdcc-lag-days（週更 + 公布延遲）
//   - 特徵 T86 只用 date < t 的觀測；標籤一律是 t 之後的 close-to-close 報酬
//
// 唯讀：本工具只做 SELECT（quotes、symbol_industry）與讀 state 檔。建議把 DSN
// 接上 default_transaction_read_only，讓寫入在資料庫端就不可能發生：
//
//	export DATABASE_URL='postgres://user:pw@host:5432/atlas?options=-c%20default_transaction_read_only%3Don'
//
// Usage:
//
//	go run ./cmd/experimental/sbl-ic-study -dsn "$DATABASE_URL" \
//	  -sbl-dir data/state/sbl -tdcc-dir data/state/tdcc_dispersion \
//	  -flows-dir data/state/stock_flows -out /tmp/sbl-ic-report
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/cmd/experimental/internal/icstudy"
)

// ---------------------------------------------------------------- input panels

type sblPoint struct {
	date                   string
	balance, volume, retrn float64
	borrowBalance          float64
}

type tdccPoint struct {
	date   string
	shares float64
}

type flowPoint struct {
	date       string
	foreignNet float64
}

type quoteBar struct {
	close  float64
	volume float64
}

func loadSBLPanel(dir string) (map[string][]sblPoint, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*_sbl.json"))
	if err != nil {
		return nil, err
	}
	panel := map[string][]sblPoint{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rows []struct {
			Date             string `json:"date"`
			Symbol           string `json:"symbol"`
			SBLShortBalance  int64  `json:"sbl_short_balance"`
			SBLShortVolume   int64  `json:"sbl_short_volume"`
			SBLReturnVolume  int64  `json:"sbl_return_volume"`
			SBLBorrowBalance int64  `json:"sbl_borrow_balance"`
		}
		if json.Unmarshal(raw, &rows) != nil {
			continue
		}
		for _, r := range rows {
			panel[r.Symbol] = append(panel[r.Symbol], sblPoint{
				date:          r.Date,
				balance:       float64(r.SBLShortBalance),
				volume:        float64(r.SBLShortVolume),
				retrn:         float64(r.SBLReturnVolume),
				borrowBalance: float64(r.SBLBorrowBalance),
			})
		}
	}
	for s := range panel {
		sort.Slice(panel[s], func(i, j int) bool { return panel[s][i].date < panel[s][j].date })
	}
	return panel, nil
}

// loadTDCCPanel sums shares_held over the holder tiers. The published file also
// carries a "total" row and a "差異數調整" reconciliation row; including either
// would roughly double the implied share count, so both are excluded.
func loadTDCCPanel(dir string) (map[string][]tdccPoint, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*_dispersion.json"))
	if err != nil {
		return nil, err
	}
	panel := map[string][]tdccPoint{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rows []struct {
			Date       string  `json:"date"`
			Symbol     string  `json:"symbol"`
			Tier       string  `json:"tier"`
			SharesHeld float64 `json:"shares_held"`
		}
		if json.Unmarshal(raw, &rows) != nil {
			continue
		}
		agg := map[string]*tdccPoint{}
		for _, r := range rows {
			if r.Tier == "total" || strings.Contains(r.Tier, "差異") {
				continue
			}
			p, ok := agg[r.Symbol]
			if !ok {
				p = &tdccPoint{date: r.Date}
				agg[r.Symbol] = p
			}
			p.shares += r.SharesHeld
		}
		for sym, p := range agg {
			panel[sym] = append(panel[sym], *p)
		}
	}
	for s := range panel {
		sort.Slice(panel[s], func(i, j int) bool { return panel[s][i].date < panel[s][j].date })
	}
	return panel, nil
}

func loadFlowsPanel(dir string) (map[string][]flowPoint, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	panel := map[string][]flowPoint{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var ff struct {
			Symbol string `json:"symbol"`
			Flows  []struct {
				Date       string  `json:"date"`
				ForeignNet float64 `json:"foreign_net"`
			} `json:"flows"`
		}
		if json.Unmarshal(raw, &ff) != nil || len(ff.Flows) == 0 {
			continue
		}
		sym := ff.Symbol
		if sym == "" {
			sym = strings.TrimSuffix(filepath.Base(f), ".json")
		}
		pts := make([]flowPoint, 0, len(ff.Flows))
		for _, fl := range ff.Flows {
			pts = append(pts, flowPoint{date: fl.Date, foreignNet: fl.ForeignNet})
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i].date < pts[j].date })
		panel[sym] = pts
	}
	return panel, nil
}

// ------------------------------------------------------------------ panel math

// panel is a symbol x calendar matrix with NaN for missing observations.
type panel struct {
	calendar []string
	index    map[string]int
	symbols  []string
	rows     map[string][]quoteBar // symbol -> bars aligned to calendar
}

func newPanel() *panel {
	return &panel{index: map[string]int{}, rows: map[string][]quoteBar{}}
}

func (p *panel) ensure(sym string) []quoteBar {
	bars, ok := p.rows[sym]
	if !ok {
		bars = make([]quoteBar, len(p.calendar))
		for i := range bars {
			bars[i] = quoteBar{close: math.NaN(), volume: math.NaN()}
		}
		p.rows[sym] = bars
		p.symbols = append(p.symbols, sym)
	}
	return bars
}

// valueAt returns the bar of symbol at calendar offset i (NaN outside range).
func (p *panel) valueAt(sym string, i int) quoteBar {
	if i < 0 || i >= len(p.calendar) {
		return quoteBar{close: math.NaN(), volume: math.NaN()}
	}
	return p.rows[sym][i]
}

// shift returns close[t]/close[t-k]-1 using the global calendar, so a gap in
// one symbol produces NaN instead of silently shifting the window.
func (p *panel) trailingReturn(sym string, i, k int) float64 {
	a := p.valueAt(sym, i)
	b := p.valueAt(sym, i-k)
	if math.IsNaN(a.close) || math.IsNaN(b.close) || b.close <= 0 {
		return math.NaN()
	}
	return a.close/b.close - 1
}

func (p *panel) forwardReturn(sym string, i, k int) float64 {
	a := p.valueAt(sym, i)
	b := p.valueAt(sym, i+k)
	if math.IsNaN(a.close) || math.IsNaN(b.close) || a.close <= 0 {
		return math.NaN()
	}
	return b.close/a.close - 1
}

// averageTurnover is the mean close*volume over the k sessions before i.
func (p *panel) averageTurnover(sym string, i, k int) float64 {
	var sum float64
	var n int
	for s := i - k; s < i; s++ {
		b := p.valueAt(sym, s)
		if math.IsNaN(b.close) || math.IsNaN(b.volume) {
			continue
		}
		sum += b.close * b.volume
		n++
	}
	if n == 0 {
		return math.NaN()
	}
	return sum / float64(n)
}

func shiftDays(dateStr string, days int) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

// latestAtOrBefore returns the last point whose date is <= cutoff.
func latestAtOrBefore[T any](pts []T, cutoff string, dateOf func(T) string) (T, bool) {
	var best T
	found := false
	for _, p := range pts {
		if d := dateOf(p); d <= cutoff && (!found || d >= dateOf(best)) {
			best, found = p, true
		}
	}
	return best, found
}

// ------------------------------------------------------------------ study rows

type row struct {
	date     string
	symbol   string
	group    string
	industry string

	chg5   float64 // SBL balance 5-observation change, ended at t-1
	net5   float64 // 5-observation net lending (volume - returns) / balance
	balpct float64 // balance / shares outstanding (TDCC, lagged)

	mom5, mom20 float64
	t86         float64
	turn20      float64

	fwd1, fwd5, fwd20 float64
	sync              float64 // return over the same window as chg5, [t-6, t-1]
}

type featureDef struct {
	name string
	fn   func(row) float64
}

type labelDef struct {
	name string
	fn   func(row) float64
}

var featureDefs = []featureDef{
	{"sbl_bal_chg5", func(r row) float64 { return r.chg5 }},
	{"sbl_net5_norm", func(r row) float64 { return r.net5 }},
	{"sbl_balance_pct", func(r row) float64 { return r.balpct }},
}

var labelDefs = []labelDef{
	{"fwd1", func(r row) float64 { return r.fwd1 }},
	{"fwd5", func(r row) float64 { return r.fwd5 }},
	{"fwd20", func(r row) float64 { return r.fwd20 }},
	{"sync", func(r row) float64 { return r.sync }},
}

func dateOf(r row) string { return r.date }

func buildRows(p *panel, sbl map[string][]sblPoint, tdcc map[string][]tdccPoint, flows map[string][]flowPoint, industry map[string]string, opt options) []row {
	sblDates := map[string]bool{}
	for _, pts := range sbl {
		for _, pt := range pts {
			sblDates[pt.date] = true
		}
	}
	dates := make([]string, 0, len(sblDates))
	for d := range sblDates {
		if _, ok := p.index[d]; ok {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)

	var out []row
	for _, t := range dates {
		i := p.index[t]
		for _, sym := range p.symbols {
			pts := sbl[sym]
			n := sort.Search(len(pts), func(k int) bool { return pts[k].date >= t })
			if n < 6 {
				continue
			}
			cur := pts[n-1]
			if daysBetween(cur.date, t) > 7 {
				continue
			}
			old := pts[n-6]
			// Every numeric cell starts as NaN ("no observation"): a zero
			// default would enter the cross-section as a real value and quietly
			// bias the rank correlation.
			r := row{
				date: t, symbol: sym, industry: industry[sym],
				chg5: math.NaN(), net5: math.NaN(), balpct: math.NaN(),
				mom5: math.NaN(), mom20: math.NaN(), t86: math.NaN(), turn20: math.NaN(),
				fwd1: math.NaN(), fwd5: math.NaN(), fwd20: math.NaN(), sync: math.NaN(),
			}
			if old.balance > 0 {
				r.chg5 = cur.balance/old.balance - 1
			}
			var net float64
			for _, pt := range pts[n-5 : n] {
				net += pt.volume - pt.retrn
			}
			if cur.balance > 0 {
				r.net5 = net / cur.balance
			}
			r.mom5 = p.trailingReturn(sym, i, 5)
			r.mom20 = p.trailingReturn(sym, i, 20)
			r.turn20 = p.averageTurnover(sym, i, 20)
			r.fwd1 = p.forwardReturn(sym, i, 1)
			r.fwd5 = p.forwardReturn(sym, i, 5)
			r.fwd20 = p.forwardReturn(sym, i, 20)
			r.sync = p.trailingReturn(sym, i-1, 5)

			if tp := tdcc[sym]; len(tp) > 0 {
				if snap, ok := latestAtOrBefore(tp, shiftDays(t, -opt.tdccLagDays), func(q tdccPoint) string { return q.date }); ok && snap.shares > 0 {
					r.balpct = cur.balance / snap.shares
				}
			}
			if math.IsNaN(r.balpct) && !math.IsNaN(r.turn20) {
				// Registered fallback when TDCC has no snapshot for the symbol:
				// proxy the share count from turnover (traded value / price).
				if px := p.valueAt(sym, i).close; px > 0 {
					r.balpct = cur.balance / (r.turn20 / px)
				}
			}
			if fp := flows[sym]; len(fp) > 0 {
				k := sort.Search(len(fp), func(j int) bool { return fp[j].date >= t })
				if k >= 5 {
					var sum float64
					for _, q := range fp[k-5 : k] {
						sum += q.foreignNet
					}
					r.t86 = sum
				}
			}
			out = append(out, r)
		}
	}
	assignGroups(out, opt.minSymbols)
	return out
}

func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// assignGroups labels every row with a per-date turnover tercile. Dates whose
// cross-section is thinner than minPerDate stay ungrouped.
func assignGroups(rows []row, minPerDate int) {
	byDate := map[string][]int{}
	for i := range rows {
		if !math.IsNaN(rows[i].turn20) {
			byDate[rows[i].date] = append(byDate[rows[i].date], i)
		}
	}
	for _, idx := range byDate {
		if len(idx) < minPerDate {
			continue
		}
		sort.Slice(idx, func(a, b int) bool { return rows[idx[a]].turn20 < rows[idx[b]].turn20 })
		for k, i := range idx {
			switch {
			case k*3 < len(idx):
				rows[i].group = "small"
			case k*3 < 2*len(idx):
				rows[i].group = "mid"
			default:
				rows[i].group = "large"
			}
		}
	}
}

func sortedDates(rows []row) []string {
	set := map[string]bool{}
	for _, r := range rows {
		set[r.date] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// obsByDate counts rows carrying a finite value of label.
func obsByDate(rows []row, label func(row) float64) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		if !math.IsNaN(label(r)) {
			out[r.date]++
		}
	}
	return out
}

// ------------------------------------------------------------------ IC helpers

// groupAll marks the unfiltered cross-section; every other value selects one
// size tercile.
const groupAll = "all"

func icCell(rows []row, feat, label func(row) float64, group string, minPerDate int) icstudy.Stats {
	sub := rows
	if group != groupAll {
		sub = make([]row, 0, len(rows))
		for _, r := range rows {
			if r.group == group {
				sub = append(sub, r)
			}
		}
	}
	return icstudy.ComputeIC(sub, dateOf, feat, label, minPerDate)
}

func partialCell(rows []row, feat, label func(row) float64, controls []func(row) float64, group string, minPerDate int) icstudy.Stats {
	sub := rows
	if group != groupAll {
		sub = make([]row, 0, len(rows))
		for _, r := range rows {
			if r.group == group {
				sub = append(sub, r)
			}
		}
	}
	return icstudy.ComputePartialIC(sub, dateOf, feat, label, controls, minPerDate)
}

// ranksResidual of the ranked series after removing its within-date industry mean.
func industryNeutralCell(rows []row, feat, label func(row) float64, group string, minPerDate int) icstudy.Stats {
	sub := rows
	if group != groupAll {
		sub = make([]row, 0, len(rows))
		for _, r := range rows {
			if r.group == group {
				sub = append(sub, r)
			}
		}
	}
	return icstudy.ComputeIndustryNeutralIC(sub, dateOf, feat, label, func(r row) string { return r.industry }, minPerDate)
}

// filterDates keeps only rows whose date is in keep.
func filterDates(rows []row, keep map[string]bool) []row {
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		if keep[r.date] {
			out = append(out, r)
		}
	}
	return out
}

// trimExtremeLabels drops the fraction tail of |label| across the whole sample,
// the registered robustness check against a handful of extreme returns.
func trimExtremeLabels(rows []row, label func(row) float64, frac float64) []row {
	var abs []float64
	for _, r := range rows {
		v := label(r)
		if !math.IsNaN(v) {
			abs = append(abs, math.Abs(v))
		}
	}
	if len(abs) == 0 {
		return rows
	}
	sort.Float64s(abs)
	cut := abs[int(float64(len(abs))*(1-frac))]
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		v := label(r)
		if !math.IsNaN(v) && math.Abs(v) >= cut {
			continue
		}
		out = append(out, r)
	}
	return out
}

func signSame(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	return (a > 0) == (b > 0)
}

// ------------------------------------------------------------------ reporting

type robustRow struct {
	Feature   string  `json:"feature"`
	Horizon   string  `json:"horizon"`
	Group     string  `json:"group"`
	NDates    int     `json:"n_dates"`
	MeanIC    float64 `json:"mean_ic"`
	ICIR      float64 `json:"icir"`
	Half1Mean float64 `json:"half1_mean_ic"`
	Half2Mean float64 `json:"half2_mean_ic"`
	HalfSame  bool    `json:"halves_same_sign"`
	Third1    float64 `json:"third1_mean_ic"`
	Third2    float64 `json:"third2_mean_ic"`
	Third3    float64 `json:"third3_mean_ic"`
	TrimMean  float64 `json:"trimmed_mean_ic"`
	TrimICIR  float64 `json:"trimmed_icir"`
}

type quintileRow struct {
	Feature string    `json:"feature"`
	Horizon string    `json:"horizon"`
	Group   string    `json:"group"`
	Buckets []float64 `json:"bucket_mean_forward_ret_pct"`
	Counts  []int     `json:"bucket_n"`
	Spread  float64   `json:"q5_minus_q1_pct"`
}

type criterionRow struct {
	Feature        string  `json:"feature"`
	Horizon        string  `json:"horizon"`
	NDatesOKSmall  bool    `json:"ndates_ge60_small"`
	NDatesOKMid    bool    `json:"ndates_ge60_mid"`
	MeanICOKSmall  bool    `json:"mean_ic_ge_0.03_small"`
	MeanICOKMid    bool    `json:"mean_ic_ge_0.03_mid"`
	ICIRokSmall    bool    `json:"icir_ge_0.3_small"`
	ICIRokMid      bool    `json:"icir_ge_0.3_mid"`
	PosOKSmall     bool    `json:"pos_ge_55_small"`
	PosOKMid       bool    `json:"pos_ge_55_mid"`
	SubPeriodSmall bool    `json:"subperiod_same_sign_small"`
	SubPeriodMid   bool    `json:"subperiod_same_sign_mid"`
	IncrementOK    bool    `json:"increment_survives_momentum_control"`
	NotOnlySync    bool    `json:"not_only_synchronous"`
	SmallN         int     `json:"small_n_dates"`
	SmallMeanIC    float64 `json:"small_mean_ic"`
	SmallICIR      float64 `json:"small_icir"`
	SmallPosPct    float64 `json:"small_positive_pct"`
	SmallHalf1     float64 `json:"small_half1_mean_ic"`
	SmallHalf2     float64 `json:"small_half2_mean_ic"`
	SmallPartial   float64 `json:"small_partial_mom_mean_ic"`
	MidN           int     `json:"mid_n_dates"`
	MidMeanIC      float64 `json:"mid_mean_ic"`
	MidICIR        float64 `json:"mid_icir"`
	MidPosPct      float64 `json:"mid_positive_pct"`
	MidHalf1       float64 `json:"mid_half1_mean_ic"`
	MidHalf2       float64 `json:"mid_half2_mean_ic"`
	MidPartial     float64 `json:"mid_partial_mom_mean_ic"`
	SyncSmall      float64 `json:"sync_small_mean_ic"`
	Verdict        string  `json:"verdict"`
	Failed         string  `json:"failed_conditions,omitempty"`
}

type report struct {
	Generated  string          `json:"generated"`
	Symbols    int             `json:"universe_symbols"`
	Dates      int             `json:"reference_dates"`
	Rows       int             `json:"rows"`
	PanelStats map[string]int  `json:"panel"`
	Primary    []icstudy.Stats `json:"primary_grid"`
	Broad      []icstudy.Stats `json:"broad_era_grid"`
	Controls   []icstudy.Stats `json:"control_grid"`
	Industry   []icstudy.Stats `json:"industry_neutral_grid"`
	Robust     []robustRow     `json:"robustness"`
	Quintiles  []quintileRow   `json:"quintiles_broad_era"`
	Criteria   []criterionRow  `json:"criteria"`
	Notes      []string        `json:"notes"`
}

type options struct {
	dsn         string
	sblDir      string
	tdccDir     string
	flowsDir    string
	tdccLagDays int
	minSymbols  int
	minGroup    int
	broadMin    int
	out         string
	dumpRows    string
	noIndustry  bool
}

// ---------------------------------------------------------------------- input

type quoteRow struct {
	symbol string
	date   string
	close  float64
	volume float64
}

func normalizeSymbol(s string) string {
	s = strings.TrimSuffix(s, ".TWO")
	return strings.TrimSuffix(s, ".TW")
}

func loadQuotes(ctx context.Context, conn *pgx.Conn, pat *regexp.Regexp) ([]quoteRow, error) {
	rows, err := conn.Query(ctx, `SELECT symbol, date, COALESCE("close", 0), COALESCE(volume, 0) FROM quotes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []quoteRow
	for rows.Next() {
		var r quoteRow
		var vol int64
		if err := rows.Scan(&r.symbol, &r.date, &r.close, &vol); err != nil {
			return nil, err
		}
		r.symbol = normalizeSymbol(r.symbol)
		r.volume = float64(vol)
		if r.close <= 0 || r.volume <= 0 || !pat.MatchString(r.symbol) {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadIndustry reads the sector map used only for the industry-neutral check.
// A missing table degrades the study rather than failing it.
func loadIndustry(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, `SELECT symbol, canonical_l1 FROM symbol_industry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var sym, l1 string
		if err := rows.Scan(&sym, &l1); err != nil {
			return nil, err
		}
		out[normalizeSymbol(sym)] = l1
	}
	return out, rows.Err()
}

func buildPanelFromQuotes(quotes []quoteRow) *panel {
	set := map[string]bool{}
	for _, q := range quotes {
		set[q.date] = true
	}
	p := newPanel()
	for d := range set {
		p.calendar = append(p.calendar, d)
	}
	sort.Strings(p.calendar)
	for i, d := range p.calendar {
		p.index[d] = i
	}
	for _, q := range quotes {
		bars := p.ensure(q.symbol)
		i := p.index[q.date]
		bars[i] = quoteBar{close: q.close, volume: q.volume}
	}
	sort.Strings(p.symbols)
	return p
}

// ------------------------------------------------------------------- analysis

func gridCells(rows []row, opt options, ids []string) []icstudy.Stats {
	var out []icstudy.Stats
	for _, id := range ids {
		for _, f := range featureDefs {
			for _, l := range labelDefs {
				for _, g := range []string{groupAll, "small", "mid", "large"} {
					minN := opt.minSymbols
					if g != groupAll {
						minN = opt.minGroup
					}
					st := icCell(rows, f.fn, l.fn, g, minN)
					st.Feature, st.Horizon, st.Group = f.name, l.name, g
					st.Control = id
					out = append(out, st)
				}
			}
		}
	}
	return out
}

func controlCells(rows []row, opt options) []icstudy.Stats {
	momOnly := []func(row) float64{func(r row) float64 { return r.mom5 }, func(r row) float64 { return r.mom20 }}
	withFlow := append(append([]func(row) float64{}, momOnly...), func(r row) float64 { return r.t86 })
	var out []icstudy.Stats
	for _, f := range featureDefs {
		for _, l := range labelDefs {
			for _, g := range []string{groupAll, "small", "mid", "large"} {
				minN := opt.minSymbols
				if g != groupAll {
					minN = opt.minGroup
				}
				named := []struct {
					name  string
					ctrls []func(row) float64
				}{
					{"partial:mom", momOnly},
					{"partial:mom+t86", withFlow},
				}
				for _, nc := range named {
					st := partialCell(rows, f.fn, l.fn, nc.ctrls, g, minN)
					st.Feature, st.Horizon, st.Group, st.Control = f.name, l.name, g, nc.name
					out = append(out, st)
				}
			}
		}
	}
	return out
}

func industryCells(rows []row, opt options) []icstudy.Stats {
	var out []icstudy.Stats
	for _, f := range featureDefs {
		for _, l := range labelDefs {
			for _, g := range []string{groupAll, "small", "mid", "large"} {
				minN := opt.minSymbols
				if g != groupAll {
					minN = opt.minGroup
				}
				st := industryNeutralCell(rows, f.fn, l.fn, g, minN)
				st.Feature, st.Horizon, st.Group, st.Control = f.name, l.name, g, "industry-neutral"
				out = append(out, st)
			}
		}
	}
	return out
}

func dateSubset(dates []string) map[string]bool {
	out := map[string]bool{}
	for _, d := range dates {
		out[d] = true
	}
	return out
}

func broadDates(rows []row, label func(row) float64, broadMin int) []string {
	counts := obsByDate(rows, label)
	var out []string
	for _, d := range sortedDates(rows) {
		if counts[d] >= broadMin {
			out = append(out, d)
		}
	}
	return out
}

func robustRows(rows []row, opt options) []robustRow {
	dates := sortedDates(rows)
	half1 := dateSubset(dates[:len(dates)/2])
	half2 := dateSubset(dates[len(dates)/2:])
	third := len(dates) / 3
	thirds := []map[string]bool{
		dateSubset(dates[:third]),
		dateSubset(dates[third : 2*third]),
		dateSubset(dates[2*third:]),
	}

	var out []robustRow
	for _, f := range featureDefs {
		for _, l := range labelDefs {
			for _, g := range []string{groupAll, "small", "mid", "large"} {
				minN := opt.minSymbols
				if g != groupAll {
					minN = opt.minGroup
				}
				base := icCell(rows, f.fn, l.fn, g, minN)
				if base.N == 0 {
					continue
				}
				h1 := icCell(filterDates(rows, half1), f.fn, l.fn, g, minN)
				h2 := icCell(filterDates(rows, half2), f.fn, l.fn, g, minN)
				trimmed := trimExtremeLabels(rows, l.fn, 0.01)
				tr := icCell(trimmed, f.fn, l.fn, g, minN)
				rowOut := robustRow{
					Feature: f.name, Horizon: l.name, Group: g,
					NDates: base.N, MeanIC: base.MeanIC, ICIR: base.ICIR,
					Half1Mean: h1.MeanIC, Half2Mean: h2.MeanIC,
					HalfSame: signSame(h1.MeanIC, h2.MeanIC),
					TrimMean: tr.MeanIC, TrimICIR: tr.ICIR,
				}
				for i, ts := range thirds {
					v := icCell(filterDates(rows, ts), f.fn, l.fn, g, minN).MeanIC
					switch i {
					case 0:
						rowOut.Third1 = v
					case 1:
						rowOut.Third2 = v
					default:
						rowOut.Third3 = v
					}
				}
				out = append(out, rowOut)
			}
		}
	}
	return out
}

func quintileRows(rows []row, opt options) []quintileRow {
	const buckets = 5
	var out []quintileRow
	for _, f := range featureDefs {
		for _, l := range labelDefs {
			if l.name == "sync" {
				continue
			}
			keep := dateSubset(broadDates(rows, l.fn, opt.broadMin))
			for _, g := range []string{groupAll, "small", "mid", "large"} {
				sums := make([]float64, buckets)
				counts := make([]int, buckets)
				byDate := map[string][]row{}
				for _, r := range rows {
					if !keep[r.date] || math.IsNaN(f.fn(r)) || math.IsNaN(l.fn(r)) {
						continue
					}
					if g != groupAll && r.group != g {
						continue
					}
					byDate[r.date] = append(byDate[r.date], r)
				}
				for _, rs := range byDate {
					if len(rs) < opt.minGroup {
						continue
					}
					sort.Slice(rs, func(a, b int) bool { return f.fn(rs[a]) < f.fn(rs[b]) })
					for k, r := range rs {
						b := k * buckets / len(rs)
						sums[b] += l.fn(r)
						counts[b]++
					}
				}
				qr := quintileRow{Feature: f.name, Horizon: l.name, Group: g}
				for b := 0; b < buckets; b++ {
					if counts[b] == 0 {
						qr.Buckets = append(qr.Buckets, math.NaN())
					} else {
						qr.Buckets = append(qr.Buckets, sums[b]/float64(counts[b])*100)
					}
					qr.Counts = append(qr.Counts, counts[b])
				}
				qr.Spread = qr.Buckets[buckets-1] - qr.Buckets[0]
				out = append(out, qr)
			}
		}
	}
	return out
}

// criteriaRows encodes the pre-registered decision rule of issue #2093. The
// thresholds are deliberately literal: they must not be tuned after seeing the
// numbers, otherwise the measurement proves nothing.
func criteriaRows(rows []row, opt options) []criterionRow {
	dates := sortedDates(rows)
	half1 := dateSubset(dates[:len(dates)/2])
	half2 := dateSubset(dates[len(dates)/2:])
	mom := []func(row) float64{func(r row) float64 { return r.mom5 }, func(r row) float64 { return r.mom20 }}
	syncLabel := func(r row) float64 { return r.sync }

	var out []criterionRow
	for _, f := range featureDefs {
		for _, l := range labelDefs {
			if l.name == "sync" {
				continue
			}
			small := icCell(rows, f.fn, l.fn, "small", opt.minGroup)
			mid := icCell(rows, f.fn, l.fn, "mid", opt.minGroup)
			smallH1 := icCell(filterDates(rows, half1), f.fn, l.fn, "small", opt.minGroup)
			smallH2 := icCell(filterDates(rows, half2), f.fn, l.fn, "small", opt.minGroup)
			midH1 := icCell(filterDates(rows, half1), f.fn, l.fn, "mid", opt.minGroup)
			midH2 := icCell(filterDates(rows, half2), f.fn, l.fn, "mid", opt.minGroup)
			incSmall := partialCell(rows, f.fn, l.fn, mom, "small", opt.minGroup)
			incMid := partialCell(rows, f.fn, l.fn, mom, "mid", opt.minGroup)
			syncSmall := icCell(rows, f.fn, syncLabel, "small", opt.minGroup)
			syncMid := icCell(rows, f.fn, syncLabel, "mid", opt.minGroup)

			c := criterionRow{
				Feature:        f.name,
				Horizon:        l.name,
				NDatesOKSmall:  small.N >= 60,
				NDatesOKMid:    mid.N >= 60,
				MeanICOKSmall:  small.MeanIC >= 0.03,
				MeanICOKMid:    mid.MeanIC >= 0.03,
				ICIRokSmall:    small.ICIR >= 0.3,
				ICIRokMid:      mid.ICIR >= 0.3,
				PosOKSmall:     small.PosPct >= 55,
				PosOKMid:       mid.PosPct >= 55,
				SubPeriodSmall: signSame(smallH1.MeanIC, smallH2.MeanIC),
				SubPeriodMid:   signSame(midH1.MeanIC, midH2.MeanIC),
				IncrementOK:    math.Abs(incSmall.MeanIC) >= 0.02 || math.Abs(incMid.MeanIC) >= 0.02,
				SmallN:         small.N,
				SmallMeanIC:    small.MeanIC,
				SmallICIR:      small.ICIR,
				SmallPosPct:    small.PosPct,
				SmallHalf1:     smallH1.MeanIC,
				SmallHalf2:     smallH2.MeanIC,
				SmallPartial:   incSmall.MeanIC,
				MidN:           mid.N,
				MidMeanIC:      mid.MeanIC,
				MidICIR:        mid.ICIR,
				MidPosPct:      mid.PosPct,
				MidHalf1:       midH1.MeanIC,
				MidHalf2:       midH2.MeanIC,
				MidPartial:     incMid.MeanIC,
				SyncSmall:      syncSmall.MeanIC,
			}
			// Falsifier: only a synchronous relation (forward IC ~ 0 while the
			// same-window contemporaneous IC is material).
			onlySync := math.Abs(small.MeanIC) < 0.02 && (math.Abs(syncSmall.MeanIC) >= 0.03 || math.Abs(syncMid.MeanIC) >= 0.03)
			c.NotOnlySync = !onlySync

			var failed []string
			checks := []struct {
				name string
				ok   bool
			}{
				{"n_dates>=60 (small)", c.NDatesOKSmall},
				{"n_dates>=60 (mid)", c.NDatesOKMid},
				{"mean IC>=0.03 (small)", c.MeanICOKSmall},
				{"mean IC>=0.03 (mid)", c.MeanICOKMid},
				{"ICIR>=0.3 (small)", c.ICIRokSmall},
				{"ICIR>=0.3 (mid)", c.ICIRokMid},
				{"%positive>=55 (small)", c.PosOKSmall},
				{"%positive>=55 (mid)", c.PosOKMid},
				{"sub-period same sign (small)", c.SubPeriodSmall},
				{"sub-period same sign (mid)", c.SubPeriodMid},
				{"increment survives momentum (falsifier)", c.IncrementOK},
				{"not only synchronous (falsifier)", c.NotOnlySync},
			}
			for _, chk := range checks {
				if !chk.ok {
					failed = append(failed, chk.name)
				}
			}
			c.Failed = strings.Join(failed, "; ")
			if len(failed) == 0 {
				c.Verdict = "PASS"
			} else {
				c.Verdict = "FAIL"
			}
			out = append(out, c)
		}
	}
	return out
}

// finite converts a non-finite float to nil so it encodes as null.
func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// MarshalJSON keeps the robustness table encodable when a cell has no estimate.
func (r robustRow) MarshalJSON() ([]byte, error) {
	type shadow struct {
		Feature   string   `json:"feature"`
		Horizon   string   `json:"horizon"`
		Group     string   `json:"group"`
		NDates    int      `json:"n_dates"`
		MeanIC    *float64 `json:"mean_ic"`
		ICIR      *float64 `json:"icir"`
		Half1Mean *float64 `json:"half1_mean_ic"`
		Half2Mean *float64 `json:"half2_mean_ic"`
		HalfSame  bool     `json:"halves_same_sign"`
		Third1    *float64 `json:"third1_mean_ic"`
		Third2    *float64 `json:"third2_mean_ic"`
		Third3    *float64 `json:"third3_mean_ic"`
		TrimMean  *float64 `json:"trimmed_mean_ic"`
		TrimICIR  *float64 `json:"trimmed_icir"`
	}
	return json.Marshal(shadow{
		Feature: r.Feature, Horizon: r.Horizon, Group: r.Group, NDates: r.NDates,
		MeanIC: finite(r.MeanIC), ICIR: finite(r.ICIR),
		Half1Mean: finite(r.Half1Mean), Half2Mean: finite(r.Half2Mean), HalfSame: r.HalfSame,
		Third1: finite(r.Third1), Third2: finite(r.Third2), Third3: finite(r.Third3),
		TrimMean: finite(r.TrimMean), TrimICIR: finite(r.TrimICIR),
	})
}

// MarshalJSON keeps the quintile table encodable when a bucket is empty.
func (q quintileRow) MarshalJSON() ([]byte, error) {
	buckets := make([]*float64, 0, len(q.Buckets))
	for _, v := range q.Buckets {
		buckets = append(buckets, finite(v))
	}
	type shadow struct {
		Feature string     `json:"feature"`
		Horizon string     `json:"horizon"`
		Group   string     `json:"group"`
		Buckets []*float64 `json:"bucket_mean_forward_ret_pct"`
		Counts  []int      `json:"bucket_n"`
		Spread  *float64   `json:"q5_minus_q1_pct"`
	}
	return json.Marshal(shadow{
		Feature: q.Feature,
		Horizon: q.Horizon,
		Group:   q.Group,
		Buckets: buckets,
		Counts:  q.Counts,
		Spread:  finite(q.Spread),
	})
}

// ------------------------------------------------------------------- reports

func fmtPct(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", v)
}

func fmtF(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%+.4f", v)
}

func fmtICIR(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%+.2f", v)
}

func writeStatsTable(b *strings.Builder, title, note string, cells []icstudy.Stats) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if note != "" {
		fmt.Fprintf(b, "%s\n\n", note)
	}
	b.WriteString("| feature | horizon | group | n_dates | obs | mean IC | std | ICIR | %positive |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range cells {
		fmt.Fprintf(b, "| %s | %s | %s | %d | %d | %s | %.4f | %s | %s |\n",
			c.Feature, c.Horizon, c.Group, c.N, c.SumN, fmtF(c.MeanIC), c.StdIC, fmtICIR(c.ICIR), fmtPct(c.PosPct))
	}
	b.WriteString("\n")
}

func writeReport(path string, rep report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# SBL per-stock IC measurement (#2093)\n\n")
	fmt.Fprintf(&b, "Generated: %s\n\n", rep.Generated)
	fmt.Fprintf(&b, "Universe: %d symbols | reference dates: %d | rows: %d\n\n", rep.Symbols, rep.Dates, rep.Rows)
	b.WriteString("Panels: ")
	keys := make([]string, 0, len(rep.PanelStats))
	for k := range rep.PanelStats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%d ", k, rep.PanelStats[k])
	}
	b.WriteString("\n\n")

	writeStatsTable(&b, "1. Primary grid (all reference dates)",
		fmt.Sprintf("Minimum cross-section: %d rows/date for the `all` column, %d rows/date inside a size tercile.",
			rep.PanelStats["min_symbols"], rep.PanelStats["min_group"]), rep.Primary)
	writeStatsTable(&b, "2. Broad-era grid",
		fmt.Sprintf("Only dates whose labeled cross-section reaches %d rows (the pre-2026-06-25 era carries ~114 quoted symbols, large caps only).",
			rep.PanelStats["broad_min"]), rep.Broad)
	writeStatsTable(&b, "3. Partial (control) grid",
		"`partial:mom` removes the ranked 5d/20d past returns; `partial:mom+t86` also removes ranked 5-day foreign net buying — the registered increment test.",
		rep.Controls)

	if len(rep.Industry) > 0 {
		writeStatsTable(&b, "4. Industry-neutral grid",
			"Within-date ranks demeaned by `symbol_industry.canonical_l1` before correlating: the part of the relation that is not a sector tilt.",
			rep.Industry)
	}

	b.WriteString("## 5. Robustness\n\n")
	b.WriteString("| feature | horizon | group | n_dates | mean IC | ICIR | half1 | half2 | same sign | third1 | third2 | third3 | trimmed mean IC | trimmed ICIR |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rep.Robust {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s | %s | %s | %v | %s | %s | %s | %s | %s |\n",
			r.Feature, r.Horizon, r.Group, r.NDates, fmtF(r.MeanIC), fmtICIR(r.ICIR),
			fmtF(r.Half1Mean), fmtF(r.Half2Mean), r.HalfSame,
			fmtF(r.Third1), fmtF(r.Third2), fmtF(r.Third3), fmtF(r.TrimMean), fmtICIR(r.TrimICIR))
	}
	b.WriteString("\n")

	b.WriteString("## 6. Quintile forward returns (broad era)\n\n")
	b.WriteString("| feature | horizon | group | Q1 (low) | Q2 | Q3 | Q4 | Q5 (high) | Q5-Q1 |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, q := range rep.Quintiles {
		cells := make([]string, 0, len(q.Buckets))
		for _, v := range q.Buckets {
			cells = append(cells, fmt.Sprintf("%+.2f%%", v))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %+.2f%% |\n",
			q.Feature, q.Horizon, q.Group, strings.Join(cells, " | "), q.Spread)
	}
	b.WriteString("\n")

	b.WriteString("## 7. Pre-registered criteria (fixed before the run; not tuned afterwards)\n\n")
	b.WriteString("Worth building a detector when, for the small and mid size terciles: `n_dates >= 60`, `mean IC >= 0.03`, `ICIR >= 0.3`, `%positive >= 55`, and the two halves share a sign. Falsifiers: `|mean IC| < 0.02` or `ICIR < 0.2`, unstable sub-period sign, the increment disappearing under a momentum control, or a purely synchronous relation.\n\n")
	b.WriteString("| feature | horizon | verdict | small (n, mean IC, ICIR, %pos, halves) | mid (n, mean IC, ICIR, %pos, halves) | partial:mom small/mid | sync IC small | failed conditions |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, c := range rep.Criteria {
		failed := c.Failed
		if failed == "" {
			failed = "-"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d, %s, %s, %s, %s/%s | %d, %s, %s, %s, %s/%s | %s / %s | %s | %s |\n",
			c.Feature, c.Horizon, c.Verdict,
			c.SmallN, fmtF(c.SmallMeanIC), fmtICIR(c.SmallICIR), fmtPct(c.SmallPosPct), fmtF(c.SmallHalf1), fmtF(c.SmallHalf2),
			c.MidN, fmtF(c.MidMeanIC), fmtICIR(c.MidICIR), fmtPct(c.MidPosPct), fmtF(c.MidHalf1), fmtF(c.MidHalf2),
			fmtF(c.SmallPartial), fmtF(c.MidPartial), fmtF(c.SyncSmall), failed)
	}
	b.WriteString("\n")

	b.WriteString("## 8. Notes and limitations\n\n")
	for _, n := range rep.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	b.WriteString("\n")

	if err := os.WriteFile(path+".md", []byte(b.String()), 0o644); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path+".json", raw, 0o644)
}

// dumpRowsCSV writes the assembled panel so the aggregation can be re-derived
// outside this binary (used by the measurement's cross-check).
func dumpRowsCSV(path string, rows []row) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	if _, err := fmt.Fprintln(w, "date,symbol,group,industry,chg5,net5,balpct,mom5,mom20,t86,turn20,fwd1,fwd5,fwd20,sync"); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			r.date, r.symbol, r.group, r.industry,
			csvF(r.chg5), csvF(r.net5), csvF(r.balpct), csvF(r.mom5), csvF(r.mom20), csvF(r.t86),
			csvF(r.turn20), csvF(r.fwd1), csvF(r.fwd5), csvF(r.fwd20), csvF(r.sync)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func csvF(v float64) string {
	if math.IsNaN(v) {
		return ""
	}
	return strconv.FormatFloat(v, 'g', 17, 64)
}

func main() {
	var opt options
	var symbolPat string
	flag.StringVar(&opt.dsn, "dsn", os.Getenv("DATABASE_URL"), "Postgres DSN; connect read-only")
	flag.StringVar(&opt.sblDir, "sbl-dir", "data/state/sbl", "SBL daily file directory")
	flag.StringVar(&opt.tdccDir, "tdcc-dir", "data/state/tdcc_dispersion", "TDCC weekly dispersion directory")
	flag.StringVar(&opt.flowsDir, "flows-dir", "data/state/stock_flows", "per-symbol T86 flow directory")
	flag.IntVar(&opt.tdccLagDays, "tdcc-lag-days", 5, "TDCC publication lag in calendar days (PIT safety)")
	flag.IntVar(&opt.minSymbols, "min-symbols", 60, "minimum labeled cross-section per date")
	flag.IntVar(&opt.minGroup, "min-group", 20, "minimum rows inside a size tercile per date")
	flag.IntVar(&opt.broadMin, "broad-min", 500, "labeled cross-section size that marks a date as broad-universe")
	flag.StringVar(&symbolPat, "symbol-pattern", `^[0-9]{4}$`, "regexp a symbol must match to enter the universe")
	flag.StringVar(&opt.out, "out", "", "report prefix (writes .md and .json)")
	flag.StringVar(&opt.dumpRows, "dump-rows", "", "optional CSV path for the assembled study rows (audit / independent re-derivation)")
	flag.BoolVar(&opt.noIndustry, "no-industry", false, "skip the industry-neutral grid (no symbol_industry query)")
	flag.Parse()

	if opt.dsn == "" {
		fmt.Fprintln(os.Stderr, "sbl-ic-study: -dsn or DATABASE_URL is required")
		os.Exit(2)
	}
	pat, err := regexp.Compile(symbolPat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl-ic-study: bad -symbol-pattern: %v\n", err)
		os.Exit(2)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, opt.dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl-ic-study: pg connect: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close(ctx) }()

	quotes, err := loadQuotes(ctx, conn, pat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl-ic-study: load quotes: %v\n", err)
		os.Exit(1)
	}
	industry := map[string]string{}
	if !opt.noIndustry {
		industry, err = loadIndustry(ctx, conn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sbl-ic-study: industry map unavailable (%v); industry-neutral grid skipped\n", err)
			industry = map[string]string{}
		}
	}

	sbl, err := loadSBLPanel(opt.sblDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl-ic-study: sbl panel: %v\n", err)
	}
	tdcc, err := loadTDCCPanel(opt.tdccDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl-ic-study: tdcc panel: %v\n", err)
	}
	flows, err := loadFlowsPanel(opt.flowsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl-ic-study: flow panel: %v\n", err)
	}

	p := buildPanelFromQuotes(quotes)
	rows := buildRows(p, sbl, tdcc, flows, industry, opt)
	dates := sortedDates(rows)

	rep := report{
		Generated: time.Now().Format(time.RFC3339),
		Symbols:   len(p.symbols),
		Dates:     len(dates),
		Rows:      len(rows),
		PanelStats: map[string]int{
			"quotes": len(quotes), "sbl_symbols": len(sbl), "tdcc_symbols": len(tdcc),
			"flow_symbols": len(flows), "industry_symbols": len(industry),
			"min_symbols": opt.minSymbols, "min_group": opt.minGroup, "broad_min": opt.broadMin,
			"calendar": len(p.calendar),
		},
		Notes: []string{
			"Features use only observations dated before the reference date t (SBL date < t; TDCC additionally lagged by -tdcc-lag-days; T86 date < t). Labels are close-to-close forward returns after t, so feature and label windows never overlap.",
			"Adjacent reference dates share most of their forward windows. The reported ICIR is mean/std of the per-date IC series and is NOT corrected for that overlap; read it as a ranking device, not as a t-statistic.",
			"Size terciles are formed per date from the trailing 20-session average traded value, so the same symbol can move between terciles over time.",
		},
	}
	if len(industry) == 0 {
		rep.Notes = append(rep.Notes, "symbol_industry was unavailable, so no industry-neutral check ran (the sector-tilt explanation stays open).")
	}
	missing, borrowRows, borrowTotal := 0, 0, 0
	for _, r := range rows {
		if math.IsNaN(r.balpct) {
			missing++
		}
	}
	for _, pts := range sbl {
		for _, pt := range pts {
			borrowTotal++
			if pt.borrowBalance != 0 {
				borrowRows++
			}
		}
	}
	legacyETF := 0
	for _, sym := range p.symbols {
		if strings.HasPrefix(sym, "00") {
			legacyETF++
		}
	}
	rep.Notes = append(rep.Notes,
		fmt.Sprintf("Universe = symbol-pattern %q (%d symbols, of which %d are legacy 4-digit ETF codes such as 0050/0056). 5-6 digit ETF/ETN codes are excluded by the pattern; pass -symbol-pattern to widen or narrow it.",
			symbolPat, len(p.symbols), legacyETF),
		fmt.Sprintf("%d of %d rows (%.1f%%) could not use a TDCC share count and fell back to a turnover-derived share estimate for sbl_balance_pct.",
			missing, len(rows), float64(missing)/float64(len(rows))*100),
		"TDCC share counts sum the holder tiers and deliberately drop the published `total` and `差異數調整` rows; keeping either roughly doubles the implied share count (2330: 25.93bn vs 51.86bn shares).",
		fmt.Sprintf("sbl_borrow_balance is non-zero in %d of %d SBL observations, so the supply-side (借券餘額) hypotheses stay unmeasurable with the current ingestion — consistent with the known ingestion gap.", borrowRows, borrowTotal))
	for _, l := range labelDefs {
		if bd := broadDates(rows, l.fn, opt.broadMin); len(bd) > 0 && bd[0] > dates[0] {
			rep.Notes = append(rep.Notes, fmt.Sprintf("%s: the first reference date whose labeled cross-section reaches %d symbols is %s; earlier dates are a large-cap-only sample (~114 fugle-quoted names).",
				l.name, opt.broadMin, bd[0]))
		}
	}

	fmt.Printf("quotes rows=%d symbols=%d | sbl=%d tdcc=%d flows=%d industry=%d\n",
		len(quotes), len(p.symbols), len(sbl), len(tdcc), len(flows), len(industry))
	fmt.Printf("study rows=%d over %d reference dates (%s .. %s)\n", len(rows), len(dates), dates[0], dates[len(dates)-1])

	if opt.dumpRows != "" {
		if err := dumpRowsCSV(opt.dumpRows, rows); err != nil {
			fmt.Fprintf(os.Stderr, "sbl-ic-study: dump rows: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("rows dumped: %s\n", opt.dumpRows)
	}

	rep.Primary = gridCells(rows, opt, []string{"full-universe"})
	rep.Controls = controlCells(rows, opt)
	if len(industry) > 0 {
		rep.Industry = industryCells(rows, opt)
	}
	rep.Robust = robustRows(rows, opt)
	rep.Quintiles = quintileRows(rows, opt)
	rep.Criteria = criteriaRows(rows, opt)

	// Broad era is defined per horizon, so run the grid once per horizon slice.
	for _, l := range labelDefs {
		keep := dateSubset(broadDates(rows, l.fn, opt.broadMin))
		sub := filterDates(rows, keep)
		thick := len(keep)
		cells := gridCells(sub, opt, []string{fmt.Sprintf("broad:%s", l.name)})
		for _, c := range cells {
			if c.Horizon == l.name {
				rep.Broad = append(rep.Broad, c)
			}
		}
		rep.Notes = append(rep.Notes, fmt.Sprintf("broad era for %s: %d of %d reference dates reach >= %d labeled rows.", l.name, thick, len(dates), opt.broadMin))
	}

	for _, c := range rep.Criteria {
		fmt.Printf("CRITERIA %-18s %-6s %s\n", c.Feature, c.Horizon, c.Verdict)
	}

	if opt.out != "" {
		if err := writeReport(opt.out, rep); err != nil {
			fmt.Fprintf(os.Stderr, "sbl-ic-study: write report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("report written: %s.md / %s.json\n", opt.out, opt.out)
	}
}
