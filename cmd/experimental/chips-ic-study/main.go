// chips-ic-study — P0 read-only information-coefficient study for the two
// newly wired chips datasets (G01 TDCC dispersion, G02 SBL balances).
//
// 用户指令 2026-09-01: 能回填來做預測的數據，先驗證預測力再接線。
// Joins the half-year backfilled history against existing recommendation
// outcomes (PG recommendation_outcomes.metadata) and reports:
//
//  1. Quintile-bucketed hit-rate / forward-return per feature
//  2. Cross-sectional Spearman rank IC per date → mean IC / ICIR / %positive
//
// Usage (deployment host, DATABASE_URL set):
//
//	go run ./cmd/experimental/chips-ic-study -out /tmp/chips-ic-report
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type outcome struct {
	symbol        string
	date          string // YYYY-MM-DD (Taipei)
	forwardReturn float64
	hit           bool
	synthetic     bool
}

type sblPoint struct {
	date    string
	balance float64
	volume  float64
	returns float64
}

type tdccSnap struct {
	date      string
	retailPct float64 // 1-999 tier holders / total holders
	bigPct    float64 // sum pct_held for tiers >= 400001 shares
}

func loadSBLPanel(dir string) (map[string][]sblPoint, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*_sbl.json"))
	if err != nil {
		return nil, err
	}
	panel := map[string][]sblPoint{}
	for _, f := range entries {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rows []struct {
			Date            string `json:"date"`
			Symbol          string `json:"symbol"`
			SBLShortBalance int64  `json:"sbl_short_balance"`
			SBLShortVolume  int64  `json:"sbl_short_volume"`
			SBLReturnVolume int64  `json:"sbl_return_volume"`
		}
		if json.Unmarshal(raw, &rows) != nil {
			continue
		}
		for _, r := range rows {
			panel[r.Symbol] = append(panel[r.Symbol], sblPoint{
				date:    r.Date,
				balance: float64(r.SBLShortBalance),
				volume:  float64(r.SBLShortVolume),
				returns: float64(r.SBLReturnVolume),
			})
		}
	}
	for sym := range panel {
		sort.Slice(panel[sym], func(i, j int) bool { return panel[sym][i].date < panel[sym][j].date })
	}
	return panel, nil
}

func tierLowerBound(tier string) int {
	parts := strings.SplitN(tier, "-", 2)
	if len(parts) == 0 {
		return 0
	}
	v, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	return v
}

func loadTDCCPanel(dir string) (map[string][]tdccSnap, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*_tdcc_dispersion.json"))
	if err != nil {
		return nil, err
	}
	panel := map[string][]tdccSnap{}
	for _, f := range entries {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rows []struct {
			Date    string  `json:"date"`
			Symbol  string  `json:"symbol"`
			Tier    string  `json:"tier"`
			Holders int     `json:"holders"`
			PctHeld float64 `json:"pct_held"`
		}
		if json.Unmarshal(raw, &rows) != nil {
			continue
		}
		agg := map[string]*tdccSnap{}
		holderTotals := map[string]int{}
		for _, r := range rows {
			s, ok := agg[r.Symbol]
			if !ok {
				s = &tdccSnap{date: r.Date}
				agg[r.Symbol] = s
			}
			holderTotals[r.Symbol] += r.Holders
			low := tierLowerBound(r.Tier)
			if low <= 999 {
				s.retailPct += float64(r.Holders)
			}
			if low >= 400001 {
				s.bigPct += r.PctHeld
			}
		}
		for sym, s := range agg {
			if holderTotals[sym] > 0 {
				s.retailPct = s.retailPct / float64(holderTotals[sym]) * 100
			}
			panel[sym] = append(panel[sym], *s)
		}
	}
	for sym := range panel {
		sort.Slice(panel[sym], func(i, j int) bool { return panel[sym][i].date < panel[sym][j].date })
	}
	return panel, nil
}

func latestBefore[T any](pts []T, cutoff string, dateOf func(T) string) (T, bool) {
	var best T
	found := false
	for _, p := range pts {
		d := dateOf(p)
		if d <= cutoff {
			if !found || d >= dateOf(best) {
				best, found = p, true
			}
		}
	}
	return best, found
}

type featRow struct {
	symbol        string
	date          string
	forwardReturn float64
	hit           bool

	fSBLChg5    float64
	fSBLNet5    float64
	fSBLCover   float64
	fTDCCRetail float64
	fTDCCBig    float64
}

func shiftDate(dateStr string, days int) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

func computeFeatures(outcomes []outcome, sbl map[string][]sblPoint, tdcc map[string][]tdccSnap, tdccLagDays int) []featRow {
	var out []featRow
	for _, o := range outcomes {
		fr := featRow{symbol: o.symbol, date: o.date, forwardReturn: o.forwardReturn, hit: o.hit}

		if pts, ok := sbl[o.symbol]; ok {
			var hist []sblPoint
			for _, p := range pts {
				if p.date < o.date {
					hist = append(hist, p)
				}
			}
			if n := len(hist); n >= 6 {
				cur := hist[n-1]
				old := hist[n-6]
				if old.balance > 0 {
					fr.fSBLChg5 = cur.balance/old.balance - 1
				}
				net := 0.0
				for _, p := range hist[n-5:] {
					net += p.volume - p.returns
				}
				if cur.balance > 0 {
					fr.fSBLNet5 = net / cur.balance
				}
				if fr.fSBLNet5 < 0 {
					fr.fSBLCover = 1
				}
			}
		}

		if snaps, ok := tdcc[o.symbol]; ok {
			cutoff := shiftDate(o.date, -tdccLagDays)
			cur, ok1 := latestBefore(snaps, cutoff, func(s tdccSnap) string { return s.date })
			if ok1 {
				fr.fTDCCRetail = cur.retailPct
				fr.fTDCCBig = cur.bigPct
				var prev []tdccSnap
				for _, s := range snaps {
					if s.date < cur.date {
						prev = append(prev, s)
					}
				}
				if p, ok2 := latestBefore(prev, cur.date, func(s tdccSnap) string { return s.date }); ok2 {
					if p.retailPct > 0 {
						fr.fTDCCRetail = cur.retailPct - p.retailPct
					}
					fr.fTDCCBig = cur.bigPct - p.bigPct
				}
			}
		}
		out = append(out, fr)
	}
	return out
}

func spearman(xs, ys []float64) float64 {
	n := len(xs)
	if n < 8 {
		return math.NaN()
	}
	rx := rank(xs)
	ry := rank(ys)
	var d2 float64
	for i := 0; i < n; i++ {
		d := rx[i] - ry[i]
		d2 += d * d
	}
	return 1 - 6*d2/float64(n*(n*n-1))
}

func rank(xs []float64) []float64 {
	type pair struct {
		v float64
		i int
	}
	ps := make([]pair, len(xs))
	for i, v := range xs {
		ps[i] = pair{v, i}
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a].v < ps[b].v })
	r := make([]float64, len(xs))
	i := 0
	for i < len(ps) {
		j := i
		for j+1 < len(ps) && ps[j+1].v == ps[i].v {
			j++
		}
		avg := float64(i+j+2) / 2
		for k := i; k <= j; k++ {
			r[ps[k].i] = avg
		}
		i = j + 1
	}
	return r
}

type icStats struct {
	Feature string  `json:"feature"`
	N       int     `json:"n_dates"`
	SumN    int     `json:"total_obs"`
	MeanIC  float64 `json:"mean_ic"`
	StdIC   float64 `json:"std_ic"`
	ICIR    float64 `json:"icir"`
	PosPct  float64 `json:"positive_pct"`
}

func computeIC(rows []featRow, feat func(featRow) float64, minPerDate int) icStats {
	byDate := map[string][]featRow{}
	for _, r := range rows {
		if math.IsNaN(feat(r)) {
			continue
		}
		byDate[r.date] = append(byDate[r.date], r)
	}
	var ics []float64
	total := 0
	for _, rs := range byDate {
		if len(rs) < minPerDate {
			continue
		}
		xs := make([]float64, len(rs))
		ys := make([]float64, len(rs))
		for i, r := range rs {
			xs[i] = feat(r)
			ys[i] = r.forwardReturn
		}
		ic := spearman(xs, ys)
		if !math.IsNaN(ic) {
			ics = append(ics, ic)
			total += len(rs)
		}
	}
	st := icStats{N: len(ics), SumN: total}
	if len(ics) == 0 {
		return st
	}
	var sum float64
	for _, v := range ics {
		sum += v
	}
	st.MeanIC = sum / float64(len(ics))
	var ss float64
	pos := 0
	for _, v := range ics {
		ss += (v - st.MeanIC) * (v - st.MeanIC)
		if v > 0 {
			pos++
		}
	}
	st.StdIC = math.Sqrt(ss / float64(len(ics)))
	if st.StdIC > 0 {
		st.ICIR = st.MeanIC / st.StdIC
	}
	st.PosPct = float64(pos) / float64(len(ics)) * 100
	return st
}

type bucketRow struct {
	Bucket     string  `json:"bucket"`
	N          int     `json:"n"`
	HitRate    float64 `json:"hit_rate_pct"`
	MeanFwdRet float64 `json:"mean_fwd_ret_pct"`
}

func computeBuckets(rows []featRow, feat func(featRow) float64, nBuckets int) []bucketRow {
	type kv struct {
		v float64
		r featRow
	}
	var kvs []kv
	for _, r := range rows {
		v := feat(r)
		if v == 0 {
			continue // no-feature rows would distort buckets
		}
		kvs = append(kvs, kv{v, r})
	}
	sort.Slice(kvs, func(a, b int) bool { return kvs[a].v < kvs[b].v })
	if len(kvs) < nBuckets*5 {
		return nil
	}
	out := make([]bucketRow, nBuckets)
	per := len(kvs) / nBuckets
	for b := 0; b < nBuckets; b++ {
		lo := b * per
		hi := (b + 1) * per
		if b == nBuckets-1 {
			hi = len(kvs)
		}
		var sumRet float64
		hits := 0
		for _, k := range kvs[lo:hi] {
			sumRet += k.r.forwardReturn
			if k.r.hit {
				hits++
			}
		}
		n := hi - lo
		lbl := fmt.Sprintf("Q%d(low)", b+1)
		if b == nBuckets-1 {
			lbl = fmt.Sprintf("Q%d(high)", nBuckets)
		}
		out[b] = bucketRow{Bucket: lbl, N: n, HitRate: float64(hits) / float64(n) * 100, MeanFwdRet: sumRet / float64(n) * 100}
	}
	return out
}

func main() {
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN")
	sblDir := flag.String("sbl-dir", "data/state/sbl", "SBL daily files dir")
	tdccDir := flag.String("tdcc-dir", "data/state/tdcc_dispersion", "TDCC weekly files dir")
	tdccLag := flag.Int("tdcc-lag-days", 5, "TDCC publishing lag (PIT safety) in days")
	out := flag.String("out", "", "report output prefix (writes .json and .md)")
	flag.Parse()

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pg connect: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx, `
		SELECT symbol,
		       to_char(time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') AS d,
		       COALESCE((metadata->>'forward_return')::float8, 0),
		       COALESCE((metadata->>'hit')::bool, false),
		       COALESCE((metadata->>'is_synthetic')::bool, false)
		FROM recommendation_outcomes
		WHERE symbol <> ''`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query outcomes: %v\n", err)
		os.Exit(1)
	}
	var outcomes []outcome
	seen := map[string]bool{}
	for rows.Next() {
		var o outcome
		if err := rows.Scan(&o.symbol, &o.date, &o.forwardReturn, &o.hit, &o.synthetic); err != nil {
			continue
		}
		if o.synthetic {
			continue
		}
		// Panel join key: FinMind symbols carry no exchange suffix.
		o.symbol = strings.TrimSuffix(strings.TrimSuffix(o.symbol, ".TWO"), ".TW")
		key := o.symbol + "|" + o.date + "|" + strconv.FormatFloat(o.forwardReturn, 'f', 6, 64)
		if seen[key] {
			continue
		}
		seen[key] = true
		outcomes = append(outcomes, o)
	}
	rows.Close()
	fmt.Printf("outcomes loaded: %d (synthetic excluded)\n", len(outcomes))

	sbl, err := loadSBLPanel(*sblDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbl panel: %v\n", err)
	}
	tdcc, err := loadTDCCPanel(*tdccDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tdcc panel: %v\n", err)
	}
	fmt.Printf("sbl symbols: %d | tdcc symbols: %d\n", len(sbl), len(tdcc))

	feats := computeFeatures(outcomes, sbl, tdcc, *tdccLag)
	withSBL, withTDCC := 0, 0
	for _, f := range feats {
		if f.fSBLChg5 != 0 || f.fSBLNet5 != 0 {
			withSBL++
		}
		if f.fTDCCRetail != 0 || f.fTDCCBig != 0 {
			withTDCC++
		}
	}
	fmt.Printf("joined rows: %d | with SBL features: %d | with TDCC features: %d\n", len(feats), withSBL, withTDCC)

	features := []struct {
		name string
		fn   func(featRow) float64
	}{
		{"sbl_bal_chg5", func(f featRow) float64 { return f.fSBLChg5 }},
		{"sbl_net5_norm", func(f featRow) float64 { return f.fSBLNet5 }},
		{"tdcc_retail_pct_wow", func(f featRow) float64 { return f.fTDCCRetail }},
		{"tdcc_big_conc_wow", func(f featRow) float64 { return f.fTDCCBig }},
	}
	var allICS []icStats
	var md strings.Builder
	md.WriteString("# Chips IC Study (P0)\n\n")
	fmt.Fprintf(&md, "Generated: %s | outcomes: %d | SBL join: %d | TDCC join: %d\n\n",
		time.Now().Format(time.RFC3339), len(outcomes), withSBL, withTDCC)
	md.WriteString("| feature | dates | obs | mean IC | std | ICIR | positive % |\n|---|---|---|---|---|---|---|\n")
	for _, f := range features {
		st := computeIC(feats, f.fn, 10)
		allICS = append(allICS, st)
		note := ""
		if st.SumN > 0 && st.StdIC == 0 {
			note = " (zero-variance — no signal coverage)"
		}
		fmt.Printf("IC %-22s dates=%-4d obs=%-6d meanIC=%+.4f ICIR=%+.2f pos=%.0f%%%s\n",
			f.name, st.N, st.SumN, st.MeanIC, st.ICIR, st.PosPct, note)
		fmt.Fprintf(&md, "| %s | %d | %d | %+.4f | %.4f | %+.2f | %.0f%% |\n",
			f.name, st.N, st.SumN, st.MeanIC, st.StdIC, st.ICIR, st.PosPct)
	}
	md.WriteString("\n## Quintile buckets (hit rate %% / mean fwd return %%)\n\n")
	for _, f := range features {
		fmt.Fprintf(&md, "### %s\n\n| bucket | n | hit%% | mean fwd ret%% |\n|---|---|---|---|\n", f.name)
		bs := computeBuckets(feats, f.fn, 5)
		for _, b := range bs {
			fmt.Printf("BUCKET %-22s %-9s n=%-5d hit=%.1f%% ret=%+.2f%%\n", f.name, b.Bucket, b.N, b.HitRate, b.MeanFwdRet)
			fmt.Fprintf(&md, "| %s | %d | %.1f | %+.2f |\n", b.Bucket, b.N, b.HitRate, b.MeanFwdRet)
		}
		md.WriteString("\n")
	}

	if *out != "" {
		j, _ := json.MarshalIndent(allICS, "", "  ")
		_ = os.WriteFile(*out+".json", j, 0o644)
		_ = os.WriteFile(*out+".md", []byte(md.String()), 0o644)
		fmt.Printf("report written: %s.json / %s.md\n", *out, *out)
	}
}
