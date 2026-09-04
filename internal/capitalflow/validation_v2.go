package capitalflow

// H-CF-01 v2 family — pre-registered revised judgments (PR-2 of the
// 2026-09-04 v2 dispatch; authoritative texts:
// .omo/plans/2026-09-04-hcf01-arbitration.md §2.2–2.6,
// .omo/plans/2026-09-04-hcf01-refine-r1.md,
// docs/specs/capital-flow-seven-dimension-spec.md §10.1).
//
// THIS PR PRE-REGISTERS THRESHOLDS AND JUDGMENT STRUCTURES ONLY. It does NOT
// run a judgment and does NOT produce a -r3 report: executing the judgments
// (v2a revised + v2a′ + v2b, Holm-corrected as one family) is a separate
// PR-3 dispatch, and nobody may judge v2a with the retired v1 thresholds
// (hit≥55% vs H0=0.5) — that would certify a persistence proxy and short-
// circuit the abandonment line.
//
// IRON RULES (unchanged from validation.go, plus the v2 revisions):
//   - All thresholds are compile-time constants here; never CLI-configurable.
//   - The null is NOT 0.5: the disclosed anchors are spot persistence
//     (BaselineA ≈ 58.7%) and price momentum (BaselineB ≈ 58.4%); the
//     economic layer additionally requires a ≥5pp increment over the no-OI
//     control (BaselineC).
//   - A version that is statistically significant but shows no OI increment
//     is PASS_NO_INCREMENT — recorded, and it COUNTS toward the abandonment
//     line (arbitration §2.5).

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// ValidationPassNoIncrement — statistically significant but no economic
// increment over the no-OI control ("現貨延續效果，非 OI 資訊"). Counts
// toward the H-CF-01 abandonment line.
const ValidationPassNoIncrement = "PASS_NO_INCREMENT" //nolint:gosec // status label, not a credential

// ---------------------------------------------------------------------------
// Pre-registered v2 thresholds (arbitration §2.2–2.6 — write-once)
// ---------------------------------------------------------------------------

const (
	// HCF01V2Alpha — family-wise error rate for the {v2a revised, v2a′,
	// v2b} judgment family. Individual judgments receive the Holm-split
	// alpha (see HolmAlphaForRank); the two layers of v2a are a conjunctive
	// condition and do NOT further split α.
	HCF01V2Alpha = 0.05

	// HCF01V2HolmFamilySize — v2a revised + v2a′ + v2b. v2c is descriptive
	// (no PASS gate) and v2d is not started: neither joins the family.
	HCF01V2HolmFamilySize = 3

	// Block-bootstrap geometry (arbitration S9: block=10 primary; 5/20 as
	// disclosed sensitivity). Deterministic seed shared with validation.go.
	HCF01V2BootstrapBlockLen   = 10
	HCF01V2BootstrapIters      = 10000
	HCF01V2LogitBlockLen       = 10
	HCF01V2LogitBootstrapIters = 2000

	// HCF01V2MinIncrementPP — economic-significance layer: the signal's hit
	// rate must EXCEED the no-OI control by at least 5 percentage points
	// (glm53 increment gate, arbitration D1).
	HCF01V2MinIncrementPP = 0.05

	// HCF01V2QuadrantMinN — v2a′ requires n ≥ 30 in BOTH the signal cell
	// and the same-cell control (arbitration §2.3; current 59/39).
	HCF01V2QuadrantMinN = 30

	// HCF01V2SanityFloorQuadrantHit — per-quadrant ≥52% is a SANITY FLOOR,
	// not evidence (arbitration S10); per-quadrant p-values are disclosed,
	// never gated.
	HCF01V2SanityFloorQuadrantHit = 0.52

	// v2b (arbitration §2.4 — R1 procedure unchanged, warmup made explicit):
	// expanding OLS OI~TAIEX residuals, k locked at 1, v1 gates kept for
	// version comparability.
	HCF01V2BWarmupDays      = 126
	HCF01V2BWarmupStartDate = "2024-07-01" // warmup consumes the OI/TAIEX window, NOT the paired window

	// Disclosed anchors measured on the current window by two independent
	// reviews (v4pro & glm53, cross-checked). NOT gates by themselves; the
	// incremental content is what the layers above test.
	HCF01V2BaselineAPersistence = 0.587 // sign(spot_{t+1}) = sign(spot_t)
	HCF01V2BaselineBMomentum    = 0.584 // sign(spot_{t+1}) = sign(ret_t)
)

// HolmAlphaForRank returns the Holm-split alpha for the judgment with the
// given rank (0 = smallest p-value in the family): α/(m−rank). With the
// pre-registered family size 3: ranks 0/1/2 → 0.05/3, 0.05/2, 0.05/1.
func HolmAlphaForRank(rank, familySize int) float64 {
	if familySize <= 0 || rank < 0 || rank >= familySize {
		return HCF01V2Alpha
	}
	return HCF01V2Alpha / float64(familySize-rank)
}

// HolmPass applies the Holm step-down procedure to family p-values and
// returns the per-judgment pass decisions (same order as the input).
// Judgments that do not reach their layer's alpha are rejected in order.
func HolmPass(ps []float64, familySize int) []bool {
	out := make([]bool, len(ps))
	if len(ps) == 0 {
		return out
	}
	type ranked struct {
		idx int
		p   float64
	}
	order := make([]ranked, len(ps))
	for i, p := range ps {
		order[i] = ranked{idx: i, p: p}
	}
	sort.Slice(order, func(a, b int) bool { return order[a].p < order[b].p })
	rejected := true
	for r, it := range order {
		pass := rejected && it.p <= HolmAlphaForRank(r, familySize)
		out[it.idx] = pass
		rejected = pass // step-down: stop rejecting after the first failure
	}
	return out
}

// HCF01V2Inputs is the shared input bundle for the v2 family judgments.
type HCF01V2Inputs struct {
	// FuturesOI — foreign TX OI net (contracts), FinMind single source
	// data/state/taifex_oi/ (macro channel BLOCKED; see oi_alignment.go).
	FuturesOI map[string]float64
	// SpotNet — T86 foreign investor spot net (億股, share counts).
	SpotNet map[string]float64
	// TAIEX — TAIEX close by date (price proxy ret_t = close_t − close_{t−1}).
	TAIEX map[string]float64
	// Dates — the trading calendar (sorted internally).
	Dates []string
	// HolmAlpha — the Holm-adjusted alpha assigned to this judgment by the
	// family ranking (HolmAlphaForRank). Callers that have not ranked the
	// family yet pass HCF01V2Alpha and must NOT finalize a report.
	HolmAlpha float64
}

// hcf01V2Day is one evaluable calendar day shared by the v2 procedures.
type hcf01V2Day struct {
	date     string
	doi      float64 // ΔOI_t (0 when !doiOK)
	doiOK    bool
	ret      float64 // TAIEX return t (close_t − close_{t−1})
	retOK    bool
	spot     float64 // spot_t
	spotOK   bool
	spotNext float64 // spot_{t+1} (next trading day present in SpotNet)
	nextOK   bool
	rollover bool
}

// buildHCF01V2Days assembles the per-day panel over the sorted calendar:
// ΔOI needs OI at t−1 (previous OI-bearing date), ret needs TAIEX at t−1
// (previous calendar date), spot needs SpotNet at t and the next trading
// date. Rollover windows are marked with the weekend calendar (hygiene
// marker; the holiday-shift rule lives in rollover.go).
func buildHCF01V2Days(in HCF01V2Inputs) []hcf01V2Day {
	sorted := append([]string(nil), in.Dates...)
	sort.Strings(sorted)
	days := make([]hcf01V2Day, 0, len(sorted))
	var prevOI float64
	haveOI := false
	var prevTaiex float64
	haveTaiex := false
	for i, d := range sorted {
		day := hcf01V2Day{date: d}
		if oi, ok := in.FuturesOI[d]; ok {
			if haveOI {
				day.doi, day.doiOK = oi-prevOI, true
			}
			prevOI, haveOI = oi, true
		} else {
			haveOI = false
		}
		if c, ok := in.TAIEX[d]; ok {
			if haveTaiex {
				day.ret, day.retOK = c-prevTaiex, true
			}
			prevTaiex, haveTaiex = c, true
		} else {
			haveTaiex = false
		}
		if s, ok := in.SpotNet[d]; ok {
			day.spot, day.spotOK = s, true
		}
		if i+1 < len(sorted) {
			if s, ok := in.SpotNet[sorted[i+1]]; ok {
				day.spotNext, day.nextOK = s, true
			}
		}
		if t, err := time.Parse("2006-01-02", d); err == nil {
			day.rollover = IsRolloverWindow(t, DefaultWeekdayHoliday)
		}
		days = append(days, day)
	}
	return days
}

// ---------------------------------------------------------------------------
// v2a revised — dual-layer judgment (arbitration §2.2)
// ---------------------------------------------------------------------------

// ValidateHCF01V2A runs the pre-registered REVISED v2a judgment.
//
// Signal (unchanged, gaps closed): ΔOI_t<0 & ret_t>0 → predict next-day
// foreign spot BUY; ΔOI_t<0 & ret_t<0 → predict SELL; ΔOI_t≥0 or ret_t==0 →
// abstain. Rollover windows excluded; ret_t==0 is an explicit abstain.
//
// Layer 1 (statistical, v4pro regression form): on the sample of evaluable
// non-abstain-candidate days, logit P(spot_{t+1} buy) ~ sign(spot_t) +
// sign(ret_t) + 1[ΔOI_t<0]; the ΔOI-indicator coefficient must be >0 with a
// circular moving-block bootstrap (block=10) one-tail p ≤ HolmAlpha. (The
// indicator varies on this sample; on the active-day subsample alone it
// would be collinear with the intercept.)
//
// Layer 2 (economic, glm53 increment gate): the v2a active-day hit rate
// must exceed the no-OI control C — active when sign(spot_t)==sign(ret_t),
// same direction as spot_t continued — by ≥5pp, with the difference tested
// by a cell-wise block-bootstrap (block=10) at p ≤ HolmAlpha; the paired
// common-day difference is disclosed alongside.
//
// Verdicts: PASS (both layers), PASS_NO_INCREMENT (layer 1 only; counts
// toward the abandonment line), FAIL, INSUFFICIENT_DATA.
func ValidateHCF01V2A(in HCF01V2Inputs) HypothesisResult {
	started := time.Now().UTC()
	alpha := in.HolmAlpha
	if alpha <= 0 {
		alpha = HCF01V2Alpha
	}
	res := HypothesisResult{
		ID:        "H-CF-01-v2a",
		StartedAt: started,
		Metrics:   map[string]float64{},
		Thresholds: map[string]float64{
			"alpha_holm":             alpha,
			"min_increment_pp":       HCF01V2MinIncrementPP,
			"bootstrap_block_len":    HCF01V2BootstrapBlockLen,
			"quadrant_sanity_floor":  HCF01V2SanityFloorQuadrantHit,
			"baseline_a_persistence": HCF01V2BaselineAPersistence,
			"baseline_b_momentum":    HCF01V2BaselineBMomentum,
		},
	}

	days := buildHCF01V2Days(in)

	// Evaluable regression sample: ΔOI known, ret_t≠0, spot_t & spot_{t+1}
	// known and non-zero; rollover excluded (pre-registered).
	type regRow struct {
		y      float64
		spot   int // sign(spot_t)
		ret    int // sign(ret_t)
		signal float64
		active bool // v2a fires (ΔOI<0); prediction = sign(ret)
		ctrl   bool // no-OI control C fires (sign(spot)==sign(ret))
		hitA   int  // on days where the respective method fires
		hitC   int
	}
	var rows []regRow
	drops := map[string]int{}
	for _, d := range days {
		if d.rollover {
			drops["rollover_window"]++
			continue
		}
		if !d.doiOK || !d.retOK || d.ret == 0 {
			drops["abstain_ret_zero_or_missing"]++
			continue
		}
		if !d.spotOK || !d.nextOK || d.spot == 0 || d.spotNext == 0 {
			drops["spot_missing_or_tie"]++
			continue
		}
		row := regRow{
			y:      boolToFloat(d.spotNext > 0),
			spot:   signF(d.spot),
			ret:    signF(d.ret),
			signal: boolToFloat(d.doi < 0),
			active: d.doi < 0,
			ctrl:   signF(d.spot) == signF(d.ret),
		}
		if row.active {
			row.hitA = hitOf(row.ret > 0, d.spotNext > 0)
		}
		if row.ctrl {
			row.hitC = hitOf(signF(d.spot) > 0, d.spotNext > 0)
		}
		rows = append(rows, row)
	}
	res.SampleCount = len(rows)
	res.Notes = append(res.Notes,
		fmt.Sprintf("精確 n：迴歸樣本 %d（rollover 剔除 %d、abstain/缺值剔除 %d）；資料源聲明：FinMind taifex_oi 為唯一 OI 源。",
			len(rows), drops["rollover_window"], drops["abstain_ret_zero_or_missing"]+drops["spot_missing_or_tie"]))
	if len(rows) < ValidationMinSampleDays {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("evaluable days %d < %d", len(rows), ValidationMinSampleDays)
		return res
	}

	// --- Layer 1: logit with controls + block bootstrap on β_signal ---
	xs := make([][]float64, len(rows))
	ys := make([]float64, len(rows))
	for i, r := range rows {
		xs[i] = []float64{1, float64(r.spot), float64(r.ret), r.signal}
		ys[i] = r.y
	}
	coef, err := logitFit(xs, ys)
	if err != nil {
		res.Status = ValidationFail
		res.Verdict = fmt.Sprintf("logit fit failed: %v", err)
		return res
	}
	betaSignal := coef[3]
	res.Metrics["beta_signal"] = betaSignal
	pStat := logitBlockBootstrapP(xs, ys, 3, HCF01V2LogitBlockLen, HCF01V2LogitBootstrapIters)
	res.Metrics["stat_layer_p"] = pStat
	statPass := betaSignal > 0 && pStat <= alpha

	// --- Active-day hit + per-quadrant disclosure (sanity floor, not gate) ---
	var activeHits []int
	nQ := map[string]int{}
	hQ := map[string]int{}
	pQ := map[string]float64{}
	for _, r := range rows {
		if !r.active {
			continue
		}
		activeHits = append(activeHits, r.hitA)
		q := "sell_side"
		if r.ret > 0 {
			q = "buy_side"
		}
		nQ[q]++
		hQ[q] += r.hitA
		pQ[q] = binomialOneTailP(hQ[q], nQ[q])
	}
	hitA := meanOf(activeHits)
	res.Metrics["active_n"] = float64(len(activeHits))
	res.Metrics["active_hit_rate"] = hitA
	for _, q := range []string{"buy_side", "sell_side"} {
		if nQ[q] > 0 {
			res.Metrics["quad_"+q+"_n"] = float64(nQ[q])
			res.Metrics["quad_"+q+"_hit"] = float64(hQ[q]) / float64(nQ[q])
			res.Metrics["quad_"+q+"_p_disclosure_only"] = pQ[q]
		}
	}
	// Rolling-60 disclosure (descriptive).
	if len(activeHits) >= HCF02RollingWindow {
		minV, maxV := math.Inf(1), math.Inf(-1)
		for i := HCF02RollingWindow; i <= len(activeHits); i++ {
			v := meanOf(activeHits[i-HCF02RollingWindow : i])
			minV = math.Min(minV, v)
			maxV = math.Max(maxV, v)
			if i == len(activeHits) {
				res.Metrics["rolling60_last"] = v
			}
		}
		res.Metrics["rolling60_min"] = minV
		res.Metrics["rolling60_max"] = maxV
	}

	// --- Layer 2: increment over no-OI control C ---
	var ctrlHits []int
	var commonDiff []int
	for _, r := range rows {
		if r.ctrl {
			ctrlHits = append(ctrlHits, r.hitC)
		}
		if r.active && r.ctrl {
			commonDiff = append(commonDiff, r.hitA-r.hitC)
		}
	}
	hitC := meanOf(ctrlHits)
	res.Metrics["control_c_n"] = float64(len(ctrlHits))
	res.Metrics["control_c_hit_rate"] = hitC
	increment := hitA - hitC
	res.Metrics["increment_pp"] = increment
	// Gate test: the pre-registered comparison is the two methods' OWN
	// active-day hit rates (glm53 §2 form: 61.6% vs 60.9%), tested with a
	// cell-wise circular block-bootstrap difference (block=10). The paired
	// difference over commonly active days is disclosed as a robustness
	// note (the McNemar alternative of the pre-registration).
	pInc := twoProportionBlockBootstrapP(activeHits, ctrlHits, HCF01V2BootstrapBlockLen, HCF01V2BootstrapIters)
	res.Metrics["increment_p"] = pInc
	if len(commonDiff) >= HCF01V2QuadrantMinN {
		res.Metrics["paired_common_n"] = float64(len(commonDiff))
		res.Metrics["paired_diff_mean"] = meanOf(commonDiff)
		res.Metrics["paired_diff_p_disclosure"] = pairedBlockBootstrapDiffP(commonDiff, HCF01V2BootstrapBlockLen, HCF01V2BootstrapIters)
	}
	incPass := increment >= HCF01V2MinIncrementPP && pInc <= alpha

	switch {
	case statPass && incPass:
		res.Status = ValidationPass
		res.Verdict = fmt.Sprintf("both layers passed: β_signal=%.3f (p=%.4f≤%.4f), active hit=%.1f%% vs control C %.1f%% (Δ=%.1fpp, p=%.4f)",
			betaSignal, pStat, alpha, hitA*100, hitC*100, increment*100, pInc)
	case statPass:
		res.Status = ValidationPassNoIncrement
		res.Verdict = fmt.Sprintf("statistical layer passed (β_signal=%.3f, p=%.4f) but increment gate failed (hit %.1f%% vs control %.1f%%, Δ=%.1fpp<%.0fpp): 現貨延續效果，非 OI 資訊；計入放棄線",
			betaSignal, pStat, hitA*100, hitC*100, increment*100, HCF01V2MinIncrementPP*100)
	default:
		res.Status = ValidationFail
		res.Verdict = fmt.Sprintf("statistical layer failed: β_signal=%.3f (p=%.4f>%.4f); active hit=%.1f%%, control C=%.1f%%",
			betaSignal, pStat, alpha, hitA*100, hitC*100)
	}
	res.StartedAt = started
	return res
}

// ---------------------------------------------------------------------------
// v2a′ — sell-side quadrant increment (arbitration §2.3)
// ---------------------------------------------------------------------------

// ValidateHCF01V2APrime runs the pre-registered v2a′ judgment: the ONLY
// place the current window shows an OI increment. Signal cell = spot_t<0 &
// ret_t<0 & ΔOI_t<0 → predict next-day spot SELL; control = the SAME cell
// with ΔOI_t≥0. Gate: hit(signal) − hit(control) ≥ 5pp AND cell-wise
// circular block-bootstrap (block=10) one-tail p ≤ HolmAlpha AND n ≥ 30 in
// BOTH cells; Wilson 95% CIs disclosed (small samples — wide by design).
// This is the last falsification chance of the H-CF-01 causal chain: PASS
// does NOT flip eligible, it records "賣邊有條件有效，待新資料樣本外驗證 +
// 30 日線上觀察".
func ValidateHCF01V2APrime(in HCF01V2Inputs) HypothesisResult {
	started := time.Now().UTC()
	alpha := in.HolmAlpha
	if alpha <= 0 {
		alpha = HCF01V2Alpha
	}
	res := HypothesisResult{
		ID:        "H-CF-01-v2a-prime",
		StartedAt: started,
		Metrics:   map[string]float64{},
		Thresholds: map[string]float64{
			"alpha_holm":          alpha,
			"min_increment_pp":    HCF01V2MinIncrementPP,
			"min_cell_n":          HCF01V2QuadrantMinN,
			"bootstrap_block_len": HCF01V2BootstrapBlockLen,
		},
	}
	days := buildHCF01V2Days(in)
	var hitSig, hitCtl []int
	drops := map[string]int{}
	for _, d := range days {
		if d.rollover {
			drops["rollover_window"]++
			continue
		}
		if !d.doiOK || !d.retOK || d.ret == 0 {
			drops["abstain_ret_zero_or_missing"]++
			continue
		}
		if !d.spotOK || !d.nextOK || d.spot == 0 || d.spotNext == 0 {
			drops["spot_missing_or_tie"]++
			continue
		}
		if !(d.spot < 0 && d.ret < 0) {
			continue // outside the sell-side quadrant
		}
		hit := hitOf(false, d.spotNext > 0) // predict SELL: hit when spotNext<0
		if d.doi < 0 {
			hitSig = append(hitSig, hit)
		} else {
			hitCtl = append(hitCtl, hit)
		}
	}
	res.Notes = append(res.Notes,
		fmt.Sprintf("精確 n：訊號格 %d、控制格 %d（rollover 剔除 %d、abstain/缺值剔除 %d）；95%% CI 以 Wilson 區間揭露。",
			len(hitSig), len(hitCtl), drops["rollover_window"], drops["abstain_ret_zero_or_missing"]+drops["spot_missing_or_tie"]))
	res.SampleCount = len(hitSig) + len(hitCtl)
	if len(hitSig) < HCF01V2QuadrantMinN || len(hitCtl) < HCF01V2QuadrantMinN {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("cell n insufficient: signal=%d, control=%d (both need ≥%d)", len(hitSig), len(hitCtl), HCF01V2QuadrantMinN)
		return res
	}
	hitA := meanOf(hitSig)
	hitB := meanOf(hitCtl)
	loA, hiA := wilsonCI(hitSig)
	loB, hiB := wilsonCI(hitCtl)
	res.Metrics["signal_n"] = float64(len(hitSig))
	res.Metrics["signal_hit"] = hitA
	res.Metrics["signal_ci95_lo"], res.Metrics["signal_ci95_hi"] = loA, hiA
	res.Metrics["control_n"] = float64(len(hitCtl))
	res.Metrics["control_hit"] = hitB
	res.Metrics["control_ci95_lo"], res.Metrics["control_ci95_hi"] = loB, hiB
	increment := hitA - hitB
	res.Metrics["increment_pp"] = increment
	p := twoProportionBlockBootstrapP(hitSig, hitCtl, HCF01V2BootstrapBlockLen, HCF01V2BootstrapIters)
	res.Metrics["diff_p"] = p

	switch {
	case increment >= HCF01V2MinIncrementPP && p <= alpha:
		res.Status = ValidationPass
		res.Verdict = fmt.Sprintf("sell-side quadrant PASS: %.1f%% (n=%d, CI %.1f–%.1f%%) vs control %.1f%% (n=%d) → Δ=%.1fpp, p=%.4f (不翻 eligible；待新資料樣本外驗證+30日觀察)",
			hitA*100, len(hitSig), loA*100, hiA*100, hitB*100, len(hitCtl), increment*100, p)
	case p <= alpha:
		res.Status = ValidationPassNoIncrement
		res.Verdict = fmt.Sprintf("diff significant (p=%.4f) but increment %.1fpp < %.0fpp：計入放棄線", p, increment*100, HCF01V2MinIncrementPP*100)
	default:
		res.Status = ValidationFail
		res.Verdict = fmt.Sprintf("sell-side quadrant FAIL: %.1f%% vs %.1f%% (Δ=%.1fpp, p=%.4f>%.4f)", hitA*100, hitB*100, increment*100, p, alpha)
	}
	res.StartedAt = started
	return res
}

// ---------------------------------------------------------------------------
// v2b — hedge-residual procedure, unchanged variables + explicit warmup
// (arbitration §2.4 / D2)
// ---------------------------------------------------------------------------

// ValidateHCF01V2B runs the pre-registered v2b judgment: expanding-window
// OLS of OI level on TAIEX level (R1 variables UNCHANGED — changing the
// regressor pre-judgment would add forbidden post-hoc degrees of freedom),
// residual Δ as the directional expression, k locked at 1.
//
// Warmup (now explicit, closing the n≥252 ambiguity): the first
// HCF01V2BWarmupDays OI/TAIEX days FROM HCF01V2BWarmupStartDate are consumed
// by the expanding estimator and never enter the evaluation window; the
// evaluation window is the full paired window.
//
// Gates (v1 gates kept deliberately for version comparability): train
// |ρ|≥0.10, OOS hit≥55% with binomial p≤0.05, 3-fold sign consistency.
// ADDITIONALLY (arbitration §2.4): an OOS hit in [55%, BaselineA=58.7%) is
// PASS_NO_INCREMENT — a persistence-grade result must not be recorded as a
// bare PASS. Baseline A is disclosed alongside. Expected FAIL (honest
// expanding rerun ≈51.7%); the v4pro limitations (better hedge regressor
// untested; level-vs-level spurious-regression risk) must accompany the
// report.
func ValidateHCF01V2B(in HCF01V2Inputs) HypothesisResult {
	started := time.Now().UTC()
	res := HypothesisResult{
		ID:        "H-CF-01-v2b",
		StartedAt: started,
		Metrics:   map[string]float64{},
		Thresholds: map[string]float64{
			"min_train_abs_rho":      HCF01MinTrainAbsRho,
			"min_oos_hit_rate":       HCF01MinOOSHitRate,
			"max_binomial_p":         HCF01MaxBinomialP,
			"warmup_days":            HCF01V2BWarmupDays,
			"baseline_a_persistence": HCF01V2BaselineAPersistence,
		},
	}

	sorted := append([]string(nil), in.Dates...)
	sort.Strings(sorted)

	// Full OI∩TAIEX series, in calendar order.
	type pt struct {
		date string
		oi   float64
		px   float64
	}
	var series []pt
	for _, d := range sorted {
		oi, okOI := in.FuturesOI[d]
		px, okPx := in.TAIEX[d]
		if okOI && okPx {
			series = append(series, pt{date: d, oi: oi, px: px})
		}
	}

	// Warmup: consume the first HCF01V2BWarmupDays series points from the
	// pre-registered window start; evaluation starts after them.
	warmupSeen := 0
	start := 0
	for i, p := range series {
		if p.date < HCF01V2BWarmupStartDate {
			continue
		}
		warmupSeen++
		if warmupSeen == HCF01V2BWarmupDays {
			start = i + 1
			break
		}
	}
	evalPts := series[start:]
	res.Notes = append(res.Notes,
		fmt.Sprintf("warmup 明文：%d 日消耗 OI/TAIEX 窗（自 %s 起），不消耗配對窗；評估窗=配對窗全段（本窗 %d 日）。",
			HCF01V2BWarmupDays, HCF01V2BWarmupStartDate, len(evalPts)))
	if len(evalPts) < ValidationMinSampleDays {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("evaluation days %d < %d after warmup", len(evalPts), ValidationMinSampleDays)
		return res
	}

	// Expanding OLS residual: resid_t estimated from points BEFORE t only.
	resid := make([]float64, len(series))
	for t := range series {
		if t < start {
			resid[t] = math.NaN()
			continue
		}
		var sx, sy, sxx, sxy float64
		for j := 0; j < t; j++ {
			sx += series[j].px
			sy += series[j].oi
			sxx += series[j].px * series[j].px
			sxy += series[j].px * series[j].oi
		}
		den := float64(t)*sxx - sx*sx
		if math.Abs(den) < 1e-9 {
			resid[t] = math.NaN()
			continue
		}
		b := (float64(t)*sxy - sx*sy) / den
		a := (sy - b*sx) / float64(t)
		resid[t] = series[t].oi - (a + b*series[t].px)
	}

	// Signal day t (in series coordinates, t ≥ start): Δresid_t vs
	// sign(spot_{t+1}); k locked = 1; rollover excluded.
	type evd struct {
		dresid float64
		target float64 // sign(spot_{t+1})
		hit    int
	}
	var evals []evd
	for t := start; t+1 < len(series); t++ {
		if math.IsNaN(resid[t]) || math.IsNaN(resid[t-1]) {
			continue
		}
		sp, ok := in.SpotNet[series[t+1].date]
		if !ok || sp == 0 {
			continue
		}
		tt, err := time.Parse("2006-01-02", series[t].date)
		if err != nil || IsRolloverWindow(tt, DefaultWeekdayHoliday) {
			continue
		}
		dr := resid[t] - resid[t-1]
		if dr == 0 {
			continue
		}
		evals = append(evals, evd{dresid: dr, target: float64(signF(sp)), hit: hitOf(dr > 0, sp > 0)})
	}
	res.SampleCount = len(evals)
	if len(evals) < ValidationMinSampleDays {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("evaluable days %d < %d", len(evals), ValidationMinSampleDays)
		return res
	}

	// Train ρ (first 2/3), OOS hit (last 1/3), 3-fold sign consistency.
	trainN := (len(evals) * 2) / 3
	var txs, tys []float64
	for i, e := range evals {
		if i < trainN {
			txs = append(txs, e.dresid)
			tys = append(tys, e.target)
		}
	}
	trainRho := pearson(txs, tys)
	res.Metrics["train_abs_rho"] = math.Abs(trainRho)

	var oosHits []int
	for i, e := range evals {
		if i >= trainN {
			oosHits = append(oosHits, e.hit)
		}
	}
	oosRate := meanOf(oosHits)
	res.Metrics["oos_n"] = float64(len(oosHits))
	res.Metrics["oos_hit_rate"] = oosRate
	bp := binomialOneTailP(sumOf(oosHits), len(oosHits))
	res.Metrics["binomial_p"] = bp
	res.Metrics["bootstrap_p_block10"] = blockBootstrapHitPLen(oosHits, HCF01V2BootstrapBlockLen, HCF01V2BootstrapIters)

	foldLen := (len(evals) + ValidationFolds - 1) / ValidationFolds
	signOK := true
	for f := 0; f < ValidationFolds; f++ {
		lo, hi := f*foldLen, (f+1)*foldLen
		if hi > len(evals) {
			hi = len(evals)
		}
		if lo >= hi {
			break
		}
		var fx, fy []float64
		for _, e := range evals[lo:hi] {
			fx = append(fx, e.dresid)
			fy = append(fy, e.target)
		}
		fr := pearson(fx, fy)
		res.Metrics[fmt.Sprintf("fold%d_rho", f+1)] = fr
		if math.IsNaN(fr) || signF(fr) != signF(trainRho) || signF(fr) == 0 {
			signOK = false
		}
	}

	gatesOK := math.Abs(trainRho) >= HCF01MinTrainAbsRho &&
		oosRate >= HCF01MinOOSHitRate && bp <= HCF01MaxBinomialP && signOK
	persistenceGrade := oosRate >= HCF01MinOOSHitRate && oosRate < HCF01V2BaselineAPersistence

	switch {
	case gatesOK && !persistenceGrade:
		res.Status = ValidationPass
		res.Verdict = fmt.Sprintf("v2b PASS: train|rho|=%.3f, OOS hit=%.1f%% (n=%d, p=%.4f), fold signs stable (baseline A=%.1f%%)",
			math.Abs(trainRho), oosRate*100, len(oosHits), bp, HCF01V2BaselineAPersistence*100)
	case persistenceGrade:
		res.Status = ValidationPassNoIncrement
		res.Verdict = fmt.Sprintf("OOS hit=%.1f%% in [55%%, baseline A %.1f%%): persistence-grade, 計入放棄線 (train|rho|=%.3f, p=%.4f)",
			oosRate*100, HCF01V2BaselineAPersistence*100, math.Abs(trainRho), bp)
	default:
		res.Status = ValidationFail
		res.Verdict = fmt.Sprintf("v2b FAIL: train|rho|=%.3f (≥0.10? %t), OOS hit=%.1f%% (≥55%%? %t), p=%.4f, fold signs stable? %t (baseline A=%.1f%%)",
			math.Abs(trainRho), math.Abs(trainRho) >= HCF01MinTrainAbsRho, oosRate*100, oosRate >= HCF01MinOOSHitRate, bp, signOK, HCF01V2BaselineAPersistence*100)
	}
	res.Notes = append(res.Notes,
		"限制註記（v4pro）：對沖變數選擇（累積現貨部位）未試；OI~TAIEX 為水位-水位趨勢迴歸，殘差語意接近協整殘差，偽迴歸風險需在報告揭露。")
	res.StartedAt = started
	return res
}

// ---------------------------------------------------------------------------
// Abandonment line (arbitration §2.5 — replaces R1 §C.2)
// ---------------------------------------------------------------------------

// HCF01DemotedNote is the spec §10 marker template written when the
// abandonment line triggers; <date> is replaced by the judgment date.
const HCF01DemotedNote = "DEMOTED(date=%s, versions tried=[v1, v2a, v2a′, v2b], note=\"對沖變數改良未試\")"

// EvaluateHCF01AbandonmentLine implements the pre-registered demotion rule:
// if v2a revised ∈ {FAIL, PASS_NO_INCREMENT} AND v2a′ ∈ {FAIL,
// PASS_NO_INCREMENT} AND v2b ∈ {FAIL, PASS_NO_INCREMENT} (each ≠
// INSUFFICIENT_DATA), the "期貨 OI → 現貨方向" causal chain is demoted
// inactive: OI stays only as hedge-state context for the 7-dimension radar
// narrative, never a direction signal. Any single genuine PASS keeps the
// chain alive. v2c (descriptive) never counts.
func EvaluateHCF01AbandonmentLine(statusV2A, statusV2APrime, statusV2B string) (triggered bool, note string) {
	fails := func(s string) bool { return s == ValidationFail || s == ValidationPassNoIncrement }
	insufficient := func(s string) bool { return s == ValidationInsufficientData }
	if insufficient(statusV2A) || insufficient(statusV2APrime) || insufficient(statusV2B) {
		return false, "INSUFFICIENT_DATA does not count toward the abandonment line (資料問題≠假設死亡)"
	}
	if fails(statusV2A) && fails(statusV2APrime) && fails(statusV2B) {
		return true, fmt.Sprintf(HCF01DemotedNote, time.Now().UTC().Format("2006-01-02"))
	}
	return false, "abandonment line not triggered: " + statusV2A + "/" + statusV2APrime + "/" + statusV2B
}

// ---------------------------------------------------------------------------
// Statistics helpers (v2 family)
// ---------------------------------------------------------------------------

// logitFit fits P(y=1) = sigmoid(Xβ) by Newton–Raphson / IRLS. xs rows
// include the intercept column. A tiny ridge (1e-8 on the diagonal,
// intercept excluded) guards separation; max 100 iterations, converged when
// the max absolute coefficient step < 1e-10. Returns the coefficient vector.
func logitFit(xs [][]float64, ys []float64) ([]float64, error) {
	if len(xs) == 0 || len(xs) != len(ys) || len(xs[0]) == 0 {
		return nil, fmt.Errorf("empty or mismatched design matrix")
	}
	k := len(xs[0])
	n := float64(len(xs))
	beta := make([]float64, k)
	for it := 0; it < 100; it++ {
		// Gradient and Hessian of the negative log-likelihood.
		grad := make([]float64, k)
		hess := make([][]float64, k)
		for r := range hess {
			hess[r] = make([]float64, k)
		}
		for i, row := range xs {
			eta := 0.0
			for j := 0; j < k; j++ {
				eta += beta[j] * row[j]
			}
			p := sigmoid(eta)
			d := ys[i] - p
			w := p * (1 - p)
			for j := 0; j < k; j++ {
				grad[j] += row[j] * d
				for l := 0; l < k; l++ {
					hess[j][l] -= row[j] * row[l] * w
				}
			}
		}
		// Ridge for numerical stability (never on the intercept).
		for j := 1; j < k; j++ {
			hess[j][j] -= 1e-8 * n
		}
		step, err := solveSymmetric(hess, grad)
		if err != nil {
			return nil, err
		}
		maxStep := 0.0
		for j := 0; j < k; j++ {
			beta[j] -= step[j]
			if a := math.Abs(step[j]); a > maxStep {
				maxStep = a
			}
		}
		if maxStep < 1e-10 {
			return beta, nil
		}
	}
	return beta, nil
}

// solveSymmetric solves H x = g for symmetric H via Gaussian elimination
// with partial pivoting (k is tiny: ≤5 columns here).
func solveSymmetric(h [][]float64, g []float64) ([]float64, error) {
	k := len(g)
	m := make([][]float64, k)
	for r := range m {
		m[r] = append(append([]float64(nil), h[r]...), g[r])
	}
	for c := 0; c < k; c++ {
		piv := c
		for r := c + 1; r < k; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[piv][c]) {
				piv = r
			}
		}
		if math.Abs(m[piv][c]) < 1e-12 {
			return nil, fmt.Errorf("singular Hessian")
		}
		m[c], m[piv] = m[piv], m[c]
		for r := 0; r < k; r++ {
			if r == c {
				continue
			}
			f := m[r][c] / m[c][c]
			for l := c; l <= k; l++ {
				m[r][l] -= f * m[c][l]
			}
		}
	}
	out := make([]float64, k)
	for r := 0; r < k; r++ {
		out[r] = m[r][k] / m[r][r]
	}
	return out, nil
}

// logitBlockBootstrapP returns a one-tail p-value for H0: β_signal ≤ 0 using
// a circular moving-block bootstrap over the regression rows (rows are days;
// blocks preserve the serial dependence of spot persistence). The design is
// resampled by day-blocks, the logit refit, and p = fraction of bootstrap
// β_signal ≤ 0 (requires observed β > 0, else 1). Deterministic seed.
func logitBlockBootstrapP(xs [][]float64, ys []float64, signalCol, blockLen, iters int) float64 {
	n := len(xs)
	if n == 0 || iters <= 0 || signalCol < 0 || signalCol >= len(xs[0]) {
		return 1
	}
	beta, err := logitFit(xs, ys)
	if err != nil || beta[signalCol] <= 0 {
		return 1
	}
	rng := rand.New(rand.NewSource(blockBootstrapSeed + 1))
	nBlocks := (n + blockLen - 1) / blockLen
	extremes := 0
	for it := 0; it < iters; it++ {
		bxs := make([][]float64, 0, n)
		bys := make([]float64, 0, n)
		for b := 0; b < nBlocks && len(bxs) < n; b++ {
			start := rng.Intn(n)
			for j := 0; j < blockLen && len(bxs) < n; j++ {
				idx := (start + j) % n
				bxs = append(bxs, xs[idx])
				bys = append(bys, ys[idx])
			}
		}
		bb, err := logitFit(bxs, bys)
		if err != nil {
			continue
		}
		if bb[signalCol] <= 0 {
			extremes++
		}
	}
	return float64(extremes) / float64(iters)
}

// pairedBlockBootstrapDiffP tests H0: mean(diff) ≤ 0 over per-day paired
// hit differences (hitA−hitC ∈ {−1,0,1}) on days where BOTH methods fire,
// using a circular moving-block bootstrap (block=10). Deterministic seed.
func pairedBlockBootstrapDiffP(diff []int, blockLen, iters int) float64 {
	n := len(diff)
	if n == 0 || iters <= 0 {
		return 1
	}
	observed := meanOf(diff)
	if observed <= 0 {
		return 1
	}
	rng := rand.New(rand.NewSource(blockBootstrapSeed + 2))
	nBlocks := (n + blockLen - 1) / blockLen
	extremes := 0
	for it := 0; it < iters; it++ {
		count := 0
		for b := 0; b < nBlocks; b++ {
			start := rng.Intn(n)
			for j := 0; j < blockLen && b*blockLen+j < n; j++ {
				count += diff[(start+j)%n]
			}
		}
		if float64(count)/float64(n) <= 0 {
			extremes++
		}
	}
	return float64(extremes) / float64(iters)
}

// twoProportionBlockBootstrapP tests H0: hitRate(A) ≤ hitRate(B) for two
// disjoint cell hit series (each ordered by calendar date), resampling each
// cell with its own circular moving-block bootstrap (block=10) and taking
// the difference. Deterministic seed.
func twoProportionBlockBootstrapP(hitA, hitB []int, blockLen, iters int) float64 {
	if len(hitA) == 0 || len(hitB) == 0 || iters <= 0 {
		return 1
	}
	observed := meanOf(hitA) - meanOf(hitB)
	if observed <= 0 {
		return 1
	}
	rng := rand.New(rand.NewSource(blockBootstrapSeed + 3))
	nBlocksA := (len(hitA) + blockLen - 1) / blockLen
	nBlocksB := (len(hitB) + blockLen - 1) / blockLen
	extremes := 0
	for it := 0; it < iters; it++ {
		sampleMean := func(xs []int, nBlocks int) float64 {
			count, total := 0, 0
			for b := 0; b < nBlocks; b++ {
				start := rng.Intn(len(xs))
				for j := 0; j < blockLen && b*blockLen+j < len(xs); j++ {
					count += xs[(start+j)%len(xs)]
					total++
				}
			}
			if total == 0 {
				return 0
			}
			return float64(count) / float64(total)
		}
		d := sampleMean(hitA, nBlocksA) - sampleMean(hitB, nBlocksB)
		if d <= 0 {
			extremes++
		}
	}
	return float64(extremes) / float64(iters)
}

// blockBootstrapHitPLen is the parameterized version of
// blockBootstrapHitP: one-tail p for "hit rate > 0.5" with an explicit
// block length (v2 family uses block=10; v1 keeps block=5).
func blockBootstrapHitPLen(hits []int, blockLen, iters int) float64 {
	n := len(hits)
	if n == 0 || iters <= 0 || blockLen <= 0 {
		return 1
	}
	if meanOf(hits) <= 0.5 {
		return 1
	}
	rng := rand.New(rand.NewSource(blockBootstrapSeed))
	observed := sumOf(hits)
	_ = observed
	extremes := 0
	nBlocks := (n + blockLen - 1) / blockLen
	for it := 0; it < iters; it++ {
		count := 0
		for b := 0; b < nBlocks; b++ {
			start := rng.Intn(n)
			for j := 0; j < blockLen && b*blockLen+j < n; j++ {
				count += hits[(start+j)%n]
			}
		}
		if float64(count)/float64(n) <= 0.5 {
			extremes++
		}
	}
	return float64(extremes) / float64(iters)
}

func sumOf(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func hitOf(predUp, actualUp bool) int {
	if predUp == actualUp {
		return 1
	}
	return 0
}

func meanOf(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0
	for _, x := range xs {
		s += x
	}
	return float64(s) / float64(len(xs))
}

// wilsonCI returns the Wilson 95% score interval for a binary hit series.
func wilsonCI(hits []int) (lo, hi float64) {
	n := len(hits)
	if n == 0 {
		return 0, 1
	}
	k := 0
	for _, h := range hits {
		k += h
	}
	p := float64(k) / float64(n)
	z := 1.959963984540054 // two-sided 95%
	z2 := z * z
	den := 1 + z2/float64(n)
	center := (p + z2/(2*float64(n))) / den
	half := z * math.Sqrt(p*(1-p)/float64(n)+z2/(4*float64(n)*float64(n))) / den
	return math.Max(0, center-half), math.Min(1, center+half)
}
