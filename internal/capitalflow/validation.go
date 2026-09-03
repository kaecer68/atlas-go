package capitalflow

// Phase 1 hypothesis validation (plan .omo/plans/2026-09-04-capital-flow-model-plan.md §3).
//
// This file implements the offline validators for H-CF-01 (foreign
// futures OI leads foreign spot 1-3 days) and H-CF-02 (TSM ADR
// information content for next-day TAIEX direction). H-CF-05 lives in
// validation_h05.go.
//
// IRON RULES (pre-registered — do not relax after seeing results):
//
//   - All decision thresholds are compile-time constants in this
//     file. They are never read from flags or config, so tuning
//     parameters to force a PASS is impossible in this tool.
//   - Fewer than ValidationMinSampleDays paired observations means
//     INSUFFICIENT_DATA. That is a first-class outcome, not a
//     failure; a small-sample PASS/FAIL verdict is forbidden.
//   - Every result records the thresholds it was judged against so a
//     report can be audited after the fact.

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// ---------------------------------------------------------------------------
// Pre-registered thresholds (plan §3.1 — write-once, never configurable)
// ---------------------------------------------------------------------------

const (
	// ValidationMinSampleDays is the minimum number of paired
	// observations before any hypothesis may be judged. Below it the
	// validator must report INSUFFICIENT_DATA (spec §10 minimum data
	// column; plan §3.1).
	ValidationMinSampleDays = 252

	// H-CF-01 thresholds.
	// HCF01MinTrainAbsRho — training-window |rho_k*| screening floor.
	// The real gate is OOS hit rate; this only filters noise-level
	// correlations (plan §3.1 H-CF-01 note).
	HCF01MinTrainAbsRho = 0.10
	// HCF01MinOOSHitRate — out-of-sample direction hit rate floor.
	HCF01MinOOSHitRate = 0.55
	// HCF01MaxBinomialP — one-tail binomial p-value ceiling (H0 = 0.5).
	HCF01MaxBinomialP = 0.05

	// H-CF-02 thresholds.
	HCF02MinHitRate     = 0.55
	HCF02MaxBinomialP   = 0.05
	HCF02MinFoldHitRate = 0.50 // every fold must beat chance
	HCF02RollingWindow  = 60   // rolling hit-rate window (descriptive)

	// Shared walk-forward geometry (plan §3.1: walk-forward is always
	// 3 folds; splits are fixed).
	ValidationFolds = 3

	// Block-bootstrap settings for the autocorrelation caveat
	// (plan §3.1 statistics notes). Block length 5 trading days;
	// deterministic seed so reruns reproduce the same p-value.
	BlockBootstrapBlockLen = 5
	blockBootstrapIters    = 10000
	blockBootstrapSeed     = 42
)

// Validation status values (JSON strings are fixed contract; the
// report schema documents them).
const (
	ValidationPass             = "PASS"
	ValidationPassImproved     = "PASS(improved)"
	ValidationFail             = "FAIL"
	ValidationInsufficientData = "INSUFFICIENT_DATA"
)

// HypothesisResult is one hypothesis's judged outcome. Thresholds
// echoes the exact pre-registered constants used for the verdict so
// reports stay auditable after the fact.
type HypothesisResult struct {
	ID          string             `json:"id"`
	Status      string             `json:"status"`
	Verdict     string             `json:"verdict"`
	SampleCount int                `json:"sample_count"`
	Metrics     map[string]float64 `json:"metrics,omitempty"`
	Thresholds  map[string]float64 `json:"thresholds,omitempty"`
	Notes       []string           `json:"notes,omitempty"`
	StartedAt   time.Time          `json:"started_at"`
}

// ---------------------------------------------------------------------------
// Statistics helpers
// ---------------------------------------------------------------------------

// pearson returns the Pearson correlation of xs and ys. Both slices
// must have equal length and at least 2 elements; otherwise it
// returns math.NaN().
func pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return math.NaN()
	}
	var sx, sy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
	}
	mx, my := sx/float64(len(xs)), sy/float64(len(ys))
	var sxx, syy, sxy float64
	for i := range xs {
		dx, dy := xs[i]-mx, ys[i]-my
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

// binomialOneTailP returns P(X >= hits) under H0: p=0.5, n trials,
// computed exactly via log-gamma so large n stays stable.
func binomialOneTailP(hits, n int) float64 {
	if n <= 0 || hits <= 0 {
		return 1
	}
	if hits > n {
		return 0
	}
	logPHead := float64(n) * math.Log(0.5) // log of 0.5^n
	// P(X >= hits) = sum_{i=hits..n} C(n,i) 0.5^n. Work in log space
	// and accumulate with the log-sum-exp trick to avoid overflow.
	logTerms := make([]float64, 0, n-hits+1)
	for i := hits; i <= n; i++ {
		lc := logChoose(n, i)
		logTerms = append(logTerms, lc+logPHead)
	}
	return logSumExpTail(logTerms)
}

// logChoose returns log(C(n,k)) via lgamma.
func logChoose(n, k int) float64 {
	return lgamma(float64(n)+1) - lgamma(float64(k)+1) - lgamma(float64(n-k)+1)
}

func lgamma(x float64) float64 {
	lv, _ := math.Lgamma(x)
	return lv
}

// logSumExpTail sums exp(logTerms) stably (they are already in
// descending order by construction of the binomial tail).
func logSumExpTail(logTerms []float64) float64 {
	if len(logTerms) == 0 {
		return 0
	}
	maxT := logTerms[0]
	for _, t := range logTerms[1:] {
		if t > maxT {
			maxT = t
		}
	}
	if math.IsInf(maxT, -1) {
		return 0
	}
	var sum float64
	for _, t := range logTerms {
		sum += math.Exp(t - maxT)
	}
	return math.Exp(maxT + math.Log(sum))
}

// blockBootstrapHitP estimates a one-tail p-value for
// "hit rate > 0.5" using a circular moving-block bootstrap over the
// per-day hit indicator series (1 = hit, 0 = miss). Daily direction
// hits are autocorrelated, so the i.i.d. binomial p is optimistic;
// this is the disclosed robustness check (plan §3.1). The gate still
// uses the binomial p.
func blockBootstrapHitP(hits []int, iters int) float64 {
	n := len(hits)
	if n == 0 || iters <= 0 {
		return 1
	}
	rng := rand.New(rand.NewSource(blockBootstrapSeed))
	observed := 0
	for _, h := range hits {
		observed += h
	}
	obsRate := float64(observed) / float64(n)
	if obsRate <= 0.5 {
		return 1
	}
	extremes := 0
	nBlocks := (n + BlockBootstrapBlockLen - 1) / BlockBootstrapBlockLen
	for it := 0; it < iters; it++ {
		count := 0
		for b := 0; b < nBlocks; b++ {
			start := rng.Intn(n)
			for j := 0; j < BlockBootstrapBlockLen && b*BlockBootstrapBlockLen+j < n; j++ {
				count += hits[(start+j)%n]
			}
		}
		if float64(count)/float64(n) <= 0.5 {
			extremes++
		}
	}
	return float64(extremes) / float64(iters)
}

func sigmoid(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

func signF(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// dateRange returns (min, max, count) of a date-keyed series for
// coverage notes in validation reports.
func dateRange(m map[string]float64) (string, string, int) {
	minD, maxD := "", ""
	for d := range m {
		if minD == "" || d < minD {
			minD = d
		}
		if maxD == "" || d > maxD {
			maxD = d
		}
	}
	return minD, maxD, len(m)
}

// ---------------------------------------------------------------------------
// H-CF-01: foreign futures OI leads foreign spot net by 1-3 days
// ---------------------------------------------------------------------------

// ValidateHypothesis01 runs the pre-registered H-CF-01 procedure.
//
// Inputs are date-keyed (YYYY-MM-DD) daily series:
//
//	futuresOI — foreign futures OI net (contracts), from TAIFEX
//	institutional data (e.g. data/state/taifex_oi/*.json).
//	spotNet   — foreign investor spot net (hundred_million_shares),
//		from T86 snapshots (e.g. data/state/capital_flow/).
//
// Method (plan §3.1, pre-registered):
//  1. Build ΔOI_t = OI_t − OI_{t−1} over the sorted union calendar.
//  2. Base pairs = t with ΔOI_t and spot at the next trading date.
//     < ValidationMinSampleDays pairs → INSUFFICIENT_DATA.
//  3. Training window = first 2/3 of pairs; pick k* = argmax_k
//     |ρ_k| ONCE (tie → smaller k); lock k*.
//  4. Validation window = last 1/3; direction hit rate at lag k*
//     (ties excluded), one-tail binomial p vs 0.5.
//  5. PASS iff |ρ_k*| ≥ 0.10 AND hit ≥ 0.55 AND p ≤ 0.05 AND the
//     sign of ρ_k* is identical in all 3 contiguous folds.
func ValidateHypothesis01(futuresOI, spotNet map[string]float64, dates []string) HypothesisResult {
	started := time.Now().UTC()
	res := HypothesisResult{
		ID:        "H-CF-01",
		StartedAt: started,
		Metrics:   map[string]float64{},
		Thresholds: map[string]float64{
			"min_train_abs_rho":   HCF01MinTrainAbsRho,
			"min_oos_hit_rate":    HCF01MinOOSHitRate,
			"max_binomial_p":      HCF01MaxBinomialP,
			"min_sample_days":     ValidationMinSampleDays,
			"folds":               ValidationFolds,
			"bootstrap_block_len": BlockBootstrapBlockLen,
		},
	}
	sorted := append([]string(nil), dates...)
	sort.Strings(sorted)

	// Coverage notes: disjoint source ranges are the most common
	// reason for zero pairs (e.g. OI backfill 2021 vs T86 from 2026-06).
	oiMin, oiMax, oiN := dateRange(futuresOI)
	spotMin, _, spotN := dateRange(spotNet)
	res.Notes = append(res.Notes,
		fmt.Sprintf("資料涵蓋: 期貨 OI %d 日 (%s..%s)、外資現貨 %d 日 (%s..)；配對僅在兩源重疊的交易日成立。", oiN, oiMin, oiMax, spotN, spotMin))

	// ΔOI keyed by date index in the sorted calendar.
	doi := make([]float64, len(sorted)) // ΔOI at sorted[i]
	doiOK := make([]bool, len(sorted))
	prevOI := math.NaN()
	for i, d := range sorted {
		oi, ok := futuresOI[d]
		if !ok {
			prevOI = math.NaN()
			continue
		}
		if !math.IsNaN(prevOI) {
			doi[i] = oi - prevOI
			doiOK[i] = true
		}
		prevOI = oi
	}
	// Spot next-date index lookup.
	spotIdx := make([]float64, len(sorted))
	for i, d := range sorted {
		spotIdx[i] = spotNet[d] // 0 when absent; presence tracked separately
	}
	hasSpot := make([]bool, len(sorted))
	for i, d := range sorted {
		_, hasSpot[i] = spotNet[d]
	}

	// Base pair index list: ΔOI at i, spot at i+1.
	type pair struct{ idx int }
	var base []pair
	for i := 0; i+1 < len(sorted); i++ {
		if doiOK[i] && hasSpot[i+1] {
			base = append(base, pair{idx: i})
		}
	}
	res.SampleCount = len(base)
	if len(base) < ValidationMinSampleDays {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("insufficient paired ΔOI/spot days: %d < %d", len(base), ValidationMinSampleDays)
		res.Notes = append(res.Notes,
			"現貨歷史缺口需先回填（cmd/fetch-historical-capital-flow 先例）；不足 252 一律 INSUFFICIENT_DATA，不得以少樣本硬判。")
		res.StartedAt = started
		return res
	}

	// Lagged targets per k (1..3): value = spot at idx+k, present flag.
	type lagged struct {
		x    []float64 // ΔOI
		y    []float64 // spot_{t+k}
		idxs []int
	}
	lags := map[int]*lagged{1: {}, 2: {}, 3: {}}
	for _, p := range base {
		for k := 1; k <= 3; k++ {
			j := p.idx + k
			if j < len(sorted) && hasSpot[j] {
				l := lags[k]
				l.x = append(l.x, doi[p.idx])
				l.y = append(l.y, spotIdx[j])
				l.idxs = append(l.idxs, p.idx)
			}
		}
	}

	// Training window = first 2/3 of base pairs (by calendar order).
	trainN := (len(base) * 2) / 3
	// rankOf maps a calendar index to its base-pair position so lag
	// subsets (which may skip pairs missing the spot reading at i+k)
	// split on the same 2/3 boundary as the base series.
	rankOf := make(map[int]int, len(base))
	for r, p := range base {
		rankOf[p.idx] = r
	}
	rho := map[int]float64{}
	for k := 1; k <= 3; k++ {
		l := lags[k]
		var xs, ys []float64
		for pos, idx := range l.idxs {
			if rankOf[idx] < trainN {
				xs = append(xs, l.x[pos])
				ys = append(ys, l.y[pos])
			}
		}
		rho[k] = pearson(xs, ys)
		res.Metrics[fmt.Sprintf("rho_lag%d", k)] = rho[k]
	}
	// k* = argmax |rho_k| over lags with a computable (non-NaN) rho;
	// tie → smaller k (pre-registered).
	kStar := 0
	best := -1.0
	for k := 1; k <= 3; k++ {
		if math.IsNaN(rho[k]) {
			continue
		}
		if a := math.Abs(rho[k]); a > best+1e-12 {
			best = a
			kStar = k
		}
	}
	if kStar == 0 {
		res.Status = ValidationFail
		res.Verdict = "no lag with computable training correlation"
		return res
	}
	res.Metrics["k_star"] = float64(kStar)
	res.Metrics["train_abs_rho"] = math.Abs(rho[kStar])

	// OOS window = last 1/3 of base pairs; direction hits at lag k*.
	l := lags[kStar]
	var hits []int
	for pos, idx := range l.idxs {
		if rankOf[idx] < trainN {
			continue
		}
		dx, dy := l.x[pos], l.y[pos]
		if dx == 0 || dy == 0 {
			continue // ties excluded (pre-registered)
		}
		if signF(dx) == signF(dy) {
			hits = append(hits, 1)
		} else {
			hits = append(hits, 0)
		}
	}
	oosN := len(hits)
	oosHits := 0
	for _, h := range hits {
		oosHits += h
	}
	oosRate := 0.0
	if oosN > 0 {
		oosRate = float64(oosHits) / float64(oosN)
	}
	res.Metrics["oos_n"] = float64(oosN)
	res.Metrics["oos_hits"] = float64(oosHits)
	res.Metrics["oos_hit_rate"] = oosRate
	bp := binomialOneTailP(oosHits, oosN)
	res.Metrics["binomial_p"] = bp
	res.Metrics["bootstrap_p_block5"] = blockBootstrapHitP(hits, blockBootstrapIters)

	// Fold sign stability: 3 contiguous equal folds over the full
	// paired series at lag k*; rho sign must agree everywhere.
	foldRho := make([]float64, 0, ValidationFolds)
	signOK := true
	foldLen := (len(base) + ValidationFolds - 1) / ValidationFolds
	for f := 0; f < ValidationFolds; f++ {
		lo, hi := f*foldLen, (f+1)*foldLen
		if lo >= len(base) {
			break
		}
		if hi > len(base) {
			hi = len(base)
		}
		var xs, ys []float64
		for pos, idx := range l.idxs {
			if rank := rankOf[idx]; rank < lo || rank >= hi {
				continue
			}
			xs = append(xs, l.x[pos])
			ys = append(ys, l.y[pos])
		}
		fr := pearson(xs, ys)
		foldRho = append(foldRho, fr)
		res.Metrics[fmt.Sprintf("fold%d_rho", f+1)] = fr
	}
	for _, fr := range foldRho {
		if math.IsNaN(fr) || signF(fr) != signF(rho[kStar]) || signF(fr) == 0 {
			signOK = false
		}
	}

	pass := math.Abs(rho[kStar]) >= HCF01MinTrainAbsRho &&
		oosRate >= HCF01MinOOSHitRate &&
		bp <= HCF01MaxBinomialP &&
		signOK
	if pass {
		res.Status = ValidationPass
		res.Verdict = fmt.Sprintf("lag k*=%d: train |rho|=%.3f, OOS hit=%.1f%% (n=%d, p=%.4f), fold signs stable",
			kStar, math.Abs(rho[kStar]), oosRate*100, oosN, bp)
	} else {
		res.Status = ValidationFail
		res.Verdict = fmt.Sprintf("lag k*=%d: train |rho|=%.3f (>=0.10? %t), OOS hit=%.1f%% (>=55%%? %t), binomial p=%.4f (<=0.05? %t), fold signs stable? %t",
			kStar, math.Abs(rho[kStar]), math.Abs(rho[kStar]) >= HCF01MinTrainAbsRho,
			oosRate*100, oosRate >= HCF01MinOOSHitRate, bp, bp <= HCF01MaxBinomialP, signOK)
	}
	res.StartedAt = started
	return res
}

// ---------------------------------------------------------------------------
// H-CF-02: TSM ADR information content for next-day TAIEX direction
// ---------------------------------------------------------------------------

// ValidateHypothesis02 runs the pre-registered H-CF-02 procedure.
//
// Inputs (date-keyed YYYY-MM-DD):
//
//	adrChange — TSM ADR daily change in percent (e.g. from macro
//	            snapshots' tsm_adr.change_pct).
//	taiex     — TAIEX close (e.g. macro snapshots' taiex.value).
//	dates     — sorted trading calendar (next-trading-date lookup).
//
// PASS iff overall hit rate ≥ 0.55 AND one-tail binomial p ≤ 0.05
// AND every 1/3 fold hit rate ≥ 0.50. Ties (zero ADR change or
// zero TAIEX move) are excluded. Regime stratification is
// descriptive-only per the plan and requires period_history from
// the PG store; this offline validator reports a note instead of a
// gate.
func ValidateHypothesis02(adrChange, taiex map[string]float64, dates []string) HypothesisResult {
	started := time.Now().UTC()
	res := HypothesisResult{
		ID:        "H-CF-02",
		StartedAt: started,
		Metrics:   map[string]float64{},
		Thresholds: map[string]float64{
			"min_hit_rate":      HCF02MinHitRate,
			"max_binomial_p":    HCF02MaxBinomialP,
			"min_fold_hit_rate": HCF02MinFoldHitRate,
			"min_sample_days":   ValidationMinSampleDays,
			"folds":             ValidationFolds,
			"rolling_window":    HCF02RollingWindow,
		},
	}
	sorted := append([]string(nil), dates...)
	sort.Strings(sorted)

	adrMin, adrMax, adrN := dateRange(adrChange)
	taiexMin, taiexMax, taiexN := dateRange(taiex)
	res.Notes = append(res.Notes,
		fmt.Sprintf("資料涵蓋: TSM ADR %d 日 (%s..%s)、TAIEX %d 日 (%s..%s)。", adrN, adrMin, adrMax, taiexN, taiexMin, taiexMax))

	// Pair each calendar index i (ADR at sorted[i], TAIEX at i and
	// i+1).
	var hits []int
	var dirs []int
	for i := 0; i+1 < len(sorted); i++ {
		d, dn := sorted[i], sorted[i+1]
		a, okA := adrChange[d]
		c0, ok0 := taiex[d]
		c1, ok1 := taiex[dn]
		if !okA || !ok0 || !ok1 {
			continue
		}
		mov := c1 - c0
		if a == 0 || mov == 0 {
			continue // ties excluded
		}
		dir := signF(mov)
		dirs = append(dirs, dir)
		if signF(a) == dir {
			hits = append(hits, 1)
		} else {
			hits = append(hits, 0)
		}
	}
	res.SampleCount = len(hits)
	if len(hits) < ValidationMinSampleDays {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("insufficient ADR/TAIEX pairs: %d < %d", len(hits), ValidationMinSampleDays)
		res.Notes = append(res.Notes,
			"ADR 歷史僅覆蓋近期（~108 天）；<252 天直接 INSUFFICIENT_DATA 是合法結論，不得以少樣本硬判。")
		res.StartedAt = started
		return res
	}

	overallHits := 0
	for _, h := range hits {
		overallHits += h
	}
	rate := float64(overallHits) / float64(len(hits))
	res.Metrics["hit_rate"] = rate
	res.Metrics["binomial_p"] = binomialOneTailP(overallHits, len(hits))
	res.Metrics["bootstrap_p_block5"] = blockBootstrapHitP(hits, blockBootstrapIters)

	// 3 contiguous folds.
	foldLen := (len(hits) + ValidationFolds - 1) / ValidationFolds
	allFoldOK := true
	for f := 0; f < ValidationFolds; f++ {
		lo, hi := f*foldLen, (f+1)*foldLen
		if lo >= len(hits) {
			break
		}
		if hi > len(hits) {
			hi = len(hits)
		}
		fh := 0
		for _, h := range hits[lo:hi] {
			fh += h
		}
		fr := float64(fh) / float64(hi-lo)
		res.Metrics[fmt.Sprintf("fold%d_hit_rate", f+1)] = fr
		if fr < HCF02MinFoldHitRate {
			allFoldOK = false
		}
	}

	// Rolling 60-day hit series (descriptive only).
	if len(hits) >= HCF02RollingWindow {
		minV, maxV := math.Inf(1), math.Inf(-1)
		for i := HCF02RollingWindow; i <= len(hits); i++ {
			s := 0
			for _, h := range hits[i-HCF02RollingWindow : i] {
				s += h
			}
			v := float64(s) / float64(HCF02RollingWindow)
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
			if i == len(hits) {
				res.Metrics["rolling60_last"] = v
			}
		}
		res.Metrics["rolling60_min"] = minV
		res.Metrics["rolling60_max"] = maxV
	}

	// Descriptive: up-day base rate and majority-class hit rate.
	upDays := 0
	for _, d := range dirs {
		if d == 1 {
			upDays++
		}
	}
	baseRate := float64(upDays) / float64(len(dirs))
	majority := 0
	if baseRate >= 0.5 {
		majority = 1
	}
	majorityHits := 0
	for _, d := range dirs {
		if d == majority {
			majorityHits++
		}
	}
	res.Metrics["taiex_up_base_rate"] = baseRate
	res.Metrics["majority_class_hit_rate"] = float64(majorityHits) / float64(len(dirs))

	pass := rate >= HCF02MinHitRate && res.Metrics["binomial_p"] <= HCF02MaxBinomialP && allFoldOK
	if pass {
		res.Status = ValidationPass
		res.Verdict = fmt.Sprintf("hit=%.1f%% (n=%d, p=%.4f), all folds ≥50%%", rate*100, len(hits), res.Metrics["binomial_p"])
	} else {
		res.Status = ValidationFail
		res.Verdict = fmt.Sprintf("hit=%.1f%% (>=55%%? %t), binomial p=%.4f (<=0.05? %t), all folds ≥50%%? %t",
			rate*100, rate >= HCF02MinHitRate, res.Metrics["binomial_p"], res.Metrics["binomial_p"] <= HCF02MaxBinomialP, allFoldOK)
	}
	res.Notes = append(res.Notes,
		"regime 分層（3 態）為描述性輸出、不作 gate；離線工具不讀 PG period_history，此欄位留待線上驗證補。")
	res.StartedAt = started
	return res
}
