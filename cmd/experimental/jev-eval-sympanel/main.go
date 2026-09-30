// Command jev-eval-sympanel exports the Stage 2 (symbol / capital-flow layer)
// prediction panel consumed by scripts/jev_eval's `stock_layer` spec.
//
// Why a separate command (not a subcommand of jev-eval-panel)
//
//	jev-eval-panel is the canonical *industry (L1)* panel (issue #1968): it reads
//	marketdata.SectorIndexReader and keys rows by (date, industry_id). This command
//	keys rows by (date, symbol) and reads per-symbol daily closes, so sharing one
//	binary would mix two readers, two row keys and two contracts. The property that
//	matters — "one command regenerates the panel" — is preserved.
//
// Ground truth is RECOMPUTED, never copied
//
//	forward.ret / forward.hit and backward.backward_hit come from the price series
//	using the SAME caliber as Stage 1: Hit = stockpicker.NetHit(return, costRate)
//	(see cmd/experimental/jev-eval-panel/main.go:409-410,420). Copying labels out
//	of stock_signal_outcomes would risk a different hit definition and label
//	contamination, so it is deliberately not done.
//
// Two precision notes that the report MUST carry
//
//  1. `pit` and `baselines` carry the SAME two platform signals (momentum_20d,
//     net_buy_3d). Therefore a verdict of the form "Jev AUC > momentum AUC" does
//     NOT mean "Jev discovered momentum by itself" — it means "under the same
//     information set, does Jev's judgment discriminate better than the
//     rule-based signals?" An A/B with a baseline-free state is a separate run
//     (not covered here).
//  2. "Capital flow" here is proxied ONLY by foreign net buy (T86, 3 sessions);
//     investment trusts, margin balances and other forces are NOT included.
//     This keeps the comparison honest against the platform's own condition
//     `foreign-3d-net-buy`, but it must not be read as a complete money-flow model.
//
// Leakage discipline
//
//	`pit` contains t and earlier only. `forward`/`backward` are never part of the
//	state: the Python side (spec.CaseView) drops gt/baselines from the state, so
//	the two layers are independent.
//
// Inputs (JSONL; the operator exports them read-only from production)
//
//	-quotes     {"date":"YYYY-MM-DD","symbol":"2330.TW","close":123.4}
//	-industry   {"symbol":"2330.TW","industry_id":"semiconductor","industry_name_zh":"半導體"}
//	-netbuy     {"date":"YYYY-MM-DD","symbol":"2330.TW","net_buy":123456}   (optional)
//
// Output (JSONL, one row per sampled date × symbol) matches the contract that
// scripts/jev_eval/specs/stock_layer.py expects.
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
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

const (
	defaultForwardDays = 5
	defaultMinHistory  = 60
	defaultCostRate    = 0.00585
)

type quoteRow struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Close  float64 `json:"close"`
}

type industryRow struct {
	Symbol         string `json:"symbol"`
	IndustryID     string `json:"industry_id"`
	IndustryNameZH string `json:"industry_name_zh,omitempty"`
}

type netBuyRow struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	NetBuy float64 `json:"net_buy"`
}

type PitFeatures struct {
	TrailingReturn5DPct    float64 `json:"ret_5d_pct"`
	TrailingReturn20DPct   float64 `json:"ret_20d_pct"`
	TrailingReturn60DPct   float64 `json:"ret_60d_pct"`
	RealizedVol20DPct      float64 `json:"vol_20d_pct"`
	DistanceFromHigh60DPct float64 `json:"dist_high_60d_pct"`
	DailyReturnPct         float64 `json:"daily_return_pct"`
	HistoryDays            int     `json:"history_days"`
	Momentum20DPct         float64 `json:"momentum_20d_pct"`
	// NetBuy3DSum 是指標：沒有 -netbuy 輸入時**完全省略**（避免 0 被誤讀成「淨買為 0」）
	NetBuy3DSum *float64 `json:"net_buy_3d_sum,omitempty"`
}

type BackwardTruth struct {
	BackwardDate string  `json:"backward_date"`
	BackwardRet  float64 `json:"backward_ret"`
	BackwardHit  bool    `json:"backward_hit"`
}

type ForwardTruth struct {
	ForwardDate string  `json:"forward_date"`
	Ret         float64 `json:"ret"`
	NetReturn   float64 `json:"net_return"`
	Hit         bool    `json:"hit"`
	Sessions    int     `json:"sessions"`
}

type SymPanelRow struct {
	Date       string             `json:"date"`
	Symbol     string             `json:"symbol"`
	IndustryID string             `json:"industry_id"`
	HoldDays   int                `json:"hold_days"`
	CostRate   float64            `json:"cost_rate"`
	PIT        PitFeatures        `json:"pit"`
	Backward   BackwardTruth      `json:"backward"`
	Forward    ForwardTruth       `json:"forward"`
	Baselines  map[string]float64 `json:"baselines"`
}

type seriesPoint struct {
	Date  string
	Close float64
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "jev-eval-sympanel: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("jev-eval-sympanel", flag.ContinueOnError)
	var (
		quotesPath  = fs.String("quotes", "", "quotes JSONL (required)")
		industryP   = fs.String("industry", "", "symbol->industry map JSONL (required)")
		netBuyPath  = fs.String("netbuy", "", "foreign net buy JSONL (optional; enables net_buy_3d)")
		start       = fs.String("start", "", "panel window start (YYYY-MM-DD)")
		end         = fs.String("end", "", "panel window end (YYYY-MM-DD)")
		forwardDays = fs.Int("forward-days", defaultForwardDays, "fixed holding period in trading sessions")
		minHistory  = fs.Int("min-history", defaultMinHistory, "trailing sessions required before a row is emitted")
		costRate    = fs.Float64("cost-rate", defaultCostRate, "round-trip cost rate")
		symbolsFile = fs.String("symbols-file", "", "optional newline list of symbols to export")
		maxDates    = fs.Int("max-dates", 0, "optional cap on the number of emitted dates")
		out         = fs.String("out", "", "output panel JSONL path (required)")
		quiet       = fs.Bool("quiet", false, "suppress the human summary")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *quotesPath == "" || *industryP == "" || *out == "" {
		return fmt.Errorf("-quotes, -industry and -out are required")
	}
	if *forwardDays <= 0 {
		return fmt.Errorf("-forward-days must be positive")
	}

	quotes, err := readQuotes(*quotesPath)
	if err != nil {
		return err
	}
	industry, err := readIndustry(*industryP)
	if err != nil {
		return err
	}
	netBuy := map[string]map[string]float64{}
	if *netBuyPath != "" {
		netBuy, err = readNetBuy(*netBuyPath)
		if err != nil {
			return err
		}
	}
	allow := map[string]bool{}
	if *symbolsFile != "" {
		list, err := readSymbolList(*symbolsFile)
		if err != nil {
			return err
		}
		for _, s := range list {
			allow[s] = true
		}
	}

	bySymbol := map[string][]seriesPoint{}
	for _, q := range quotes {
		if *start != "" && q.Date < *start {
			continue
		}
		if *end != "" && q.Date > *end {
			continue
		}
		if len(allow) > 0 && !allow[q.Symbol] {
			continue
		}
		bySymbol[q.Symbol] = append(bySymbol[q.Symbol], seriesPoint{Date: q.Date, Close: q.Close})
	}
	symbols := make([]string, 0, len(bySymbol))
	for s := range bySymbol {
		sort.Slice(bySymbol[s], func(i, j int) bool { return bySymbol[s][i].Date < bySymbol[s][j].Date })
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	rows := make([]SymPanelRow, 0, 4096)
	for _, sym := range symbols {
		pts := bySymbol[sym]
		ind := industry[sym]
		for i := *minHistory; i < len(pts)-*forwardDays; i++ {
			hist := pts[:i+1]
			row, ok := buildRow(sym, ind, hist, pts, i, *forwardDays, *costRate, netBuy[sym])
			if !ok {
				continue
			}
			rows = append(rows, row)
		}
	}
	if *maxDates > 0 {
		dates := map[string]bool{}
		for _, r := range rows {
			dates[r.Date] = true
		}
		ordered := make([]string, 0, len(dates))
		for d := range dates {
			ordered = append(ordered, d)
		}
		sort.Strings(ordered)
		keep := map[string]bool{}
		for _, d := range ordered[:min(*maxDates, len(ordered))] {
			keep[d] = true
		}
		filtered := rows[:0]
		for _, r := range rows {
			if keep[r.Date] {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].Symbol < rows[j].Symbol
	})

	if err := writeJSONL(*out, rows); err != nil {
		return err
	}
	if !*quiet {
		dates := map[string]bool{}
		for _, r := range rows {
			dates[r.Date] = true
		}
		// errcheck：對 io.Writer 的寫入錯誤要顯式忽略（本行只是人類摘要，失敗不影響面板輸出）
		_, _ = fmt.Fprintf(stdout, "rows=%d symbols=%d dates=%d hold_days=%d cost_rate=%.5f net_buy=%v\n",
			len(rows), len(symbols), len(dates), *forwardDays, *costRate, *netBuyPath != "")
	}
	return nil
}

func buildRow(sym string, ind industryRow, hist []seriesPoint, all []seriesPoint, i, holdDays int, cost float64, nb map[string]float64) (SymPanelRow, bool) {
	if i < 1 || i+holdDays >= len(all) {
		return SymPanelRow{}, false
	}
	closes := make([]float64, len(hist))
	dates := make([]string, len(hist))
	for k, p := range hist {
		closes[k] = p.Close
		dates[k] = p.Date
	}
	ret := dailyReturns(closes)
	pit := computePIT(closes, ret)
	startIdx := i - holdDays
	if startIdx < 0 {
		return SymPanelRow{}, false
	}
	backRet := closes[i]/closes[startIdx] - 1
	fwdRet := all[i+holdDays].Close/closes[i] - 1
	base := map[string]float64{}
	pit.Momentum20DPct = pit.TrailingReturn20DPct
	base["momentum_20d"] = pit.Momentum20DPct
	if nb != nil {
		sum := 0.0
		n := 0
		for k := i; k >= 0 && n < 3; k-- {
			sum += nb[dates[k]]
			n++
		}
		pit.NetBuy3DSum = &sum
		base["net_buy_3d"] = sum
	}
	return SymPanelRow{
		Date:       dates[i],
		Symbol:     sym,
		IndustryID: ind.IndustryID,
		HoldDays:   holdDays,
		CostRate:   cost,
		PIT:        pit,
		Backward: BackwardTruth{
			BackwardDate: dates[startIdx],
			BackwardRet:  backRet,
			BackwardHit:  stockpicker.NetHit(backRet, cost),
		},
		Forward: ForwardTruth{
			ForwardDate: all[i+holdDays].Date,
			Ret:         fwdRet,
			NetReturn:   fwdRet - cost,
			Hit:         stockpicker.NetHit(fwdRet, cost),
			Sessions:    holdDays,
		},
		Baselines: base,
	}, true
}

func dailyReturns(closes []float64) []float64 {
	out := make([]float64, 0, len(closes))
	for i := 1; i < len(closes); i++ {
		if closes[i-1] > 0 {
			out = append(out, closes[i]/closes[i-1]-1)
		}
	}
	return out
}

func compoundTail(closes []float64, n int) float64 {
	if len(closes) < 2 {
		return 0
	}
	start := len(closes) - 1 - n
	if start < 0 {
		start = 0
	}
	if closes[start] <= 0 {
		return 0
	}
	return (closes[len(closes)-1]/closes[start] - 1) * 100
}

func sampleStdPct(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	mean := 0.0
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	v := 0.0
	for _, x := range xs {
		d := x - mean
		v += d * d
	}
	return math.Sqrt(v/float64(len(xs)-1)) * 100
}

func computePIT(closes []float64, ret []float64) PitFeatures {
	pit := PitFeatures{
		TrailingReturn5DPct:  compoundTail(closes, 5),
		TrailingReturn20DPct: compoundTail(closes, 20),
		TrailingReturn60DPct: compoundTail(closes, 60),
		HistoryDays:          len(closes),
	}
	tail20 := ret
	if len(tail20) > 20 {
		tail20 = tail20[len(tail20)-20:]
	}
	pit.RealizedVol20DPct = sampleStdPct(tail20)
	if len(ret) > 0 {
		pit.DailyReturnPct = ret[len(ret)-1] * 100
	}
	hi := closes[0]
	for _, c := range closes {
		if c > hi {
			hi = c
		}
	}
	if hi > 0 {
		pit.DistanceFromHigh60DPct = (closes[len(closes)-1]/hi - 1) * 100
	}
	return pit
}

func readQuotes(path string) ([]quoteRow, error) {
	out := []quoteRow{}
	err := eachLine(path, func(line []byte) error {
		var r quoteRow
		if err := json.Unmarshal(line, &r); err != nil {
			return nil
		}
		if r.Symbol != "" && r.Date != "" {
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

func readIndustry(path string) (map[string]industryRow, error) {
	out := map[string]industryRow{}
	err := eachLine(path, func(line []byte) error {
		var r industryRow
		if err := json.Unmarshal(line, &r); err != nil {
			return nil
		}
		if r.Symbol != "" {
			out[r.Symbol] = r
		}
		return nil
	})
	return out, err
}

func readNetBuy(path string) (map[string]map[string]float64, error) {
	out := map[string]map[string]float64{}
	err := eachLine(path, func(line []byte) error {
		var r netBuyRow
		if err := json.Unmarshal(line, &r); err != nil {
			return nil
		}
		if r.Symbol == "" || r.Date == "" {
			return nil
		}
		if out[r.Symbol] == nil {
			out[r.Symbol] = map[string]float64{}
		}
		out[r.Symbol][r.Date] = r.NetBuy
		return nil
	})
	return out, err
}

func readSymbolList(path string) ([]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func eachLine(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := fn([]byte(line)); err != nil {
			return err
		}
	}
	return sc.Err()
}

func writeJSONL(path string, rows []SymPanelRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return w.Flush()
}

var _ = time.Now
