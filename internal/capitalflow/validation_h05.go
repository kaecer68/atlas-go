package capitalflow

// H-CF-05 — layered (E07 4-layer vote) vs equal-weight (7-dim mean Z)
// walk-forward comparison (plan §3.1 B3 revision; spec §10).
//
// Both models are static (no parameter re-estimation) and are replayed
// date by date from the rolling sample store with strictly-prior
// history, honoring spec §8.4 (Z from prior samples only).
//
// Pre-registered model definitions (do not change after seeing
// results):
//
//   - Layered = E07 4-layer vote. Each layer's vote v_i ∈ {+1,−1,0}:
//     Direction=="bullish" → +1, "bearish" → −1, anything else → 0.
//     An unavailable layer is skipped (contributes 0); no neutral
//     fill-in, no borrowing from other layers. vote_sum = Σ v_i;
//     direction = sign(vote_sum); vote_sum == 0 is an abstain.
//     If H-CF-02 has not passed, the cross_market layer stays
//     unavailable and the model naturally degrades to 3 votes —
//     that is a pre-defined rule, not a post-hoc patch.
//   - Layered vote→probability: p(up) = sigmoid(vote_sum); abstain
//     days use p = 0.5. This mapping is written into the report's
//     method/threshold fields and must not change afterwards.
//   - Equal-weight = sign(mean(z_1..z_7)) over available dimensions,
//     with a ±EWNeutralBand neutral band (→ abstain). The
//     equal-weight model has no calibrated confidence output, so
//     its Brier probability is the honest three-state mapping
//     p ∈ {0, 0.5, 1} from its −1/0/+1 direction call.
//   - Hit-rate denominator = non-abstain days only; abstain days are
//     reported separately (abstain_days). Brier scores ALL
//     evaluation days (abstain days score p = 0.5). Position =
//     sign of the model's direction; abstain days hold no position.
//   - Walk-forward: expanding window, evaluation starts after
//     HCF05WarmupDays evaluation dates; fewer than
//     ValidationMinSampleDays evaluation days → INSUFFICIENT_DATA.
//   - PASS iff all three "no-degradation" conditions hold:
//     hit_layered ≥ hit_ew − HCF05HitRateTolerancePP,
//     brier_layered ≤ brier_ew + HCF05BrierTolerance,
//     maxdd_layered ≤ maxdd_ew + HCF05MaxDDTolerancePP.
//     Any strictly better metric → PASS(improved); any tolerance
//     exceeded → FAIL. Base rate and majority-class hit rate are
//     always reported for human gate reading.

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	// HCF05WarmupDays — expanding-window warmup before scoring starts.
	HCF05WarmupDays = 126
	// EWNeutralBand — |mean Z| below this abstains (plan §3.1).
	EWNeutralBand = 0.1
	// Tolerances (plan §3.1, pre-registered).
	HCF05HitRateTolerancePP = 1.0 // percentage points
	HCF05BrierTolerance     = 0.02
	HCF05MaxDDTolerancePP   = 0.5 // percentage points
)

// hcf05Day is one replayed evaluation day.
type hcf05Day struct {
	date      string
	direction int // next-day TAIEX direction (+1 up / -1 down); ties were skipped

	layeredVote  int     // vote_sum ∈ [-4, 4]
	layeredP     float64 // sigmoid(vote_sum), 0.5 when abstain
	layeredPos   int     // sign(vote_sum)
	layerAbstain bool

	ewMeanZ float64
	ewP     float64 // three-state {0, 0.5, 1}
	ewPos   int
	ewABst  bool
}

// ValidateHypothesis05 replays the rolling store and judges H-CF-05.
//
// Inputs:
//
//	samples — per-dimension rolling samples (all seven dimensions),
//	          e.g. read back from the persisted rolling store.
//	taiex   — date-keyed TAIEX close values.
//	dates   — sorted trading calendar covering the sample dates
//	          (next-trading-date lookup for the target direction).
func ValidateHypothesis05(samples map[ForceName][]RollingSample, taiex map[string]float64, dates []string) HypothesisResult {
	started := time.Now().UTC()
	res := HypothesisResult{
		ID:        "H-CF-05",
		StartedAt: started,
		Metrics:   map[string]float64{},
		Thresholds: map[string]float64{
			"min_sample_days":         ValidationMinSampleDays,
			"warmup_days":             HCF05WarmupDays,
			"ew_neutral_band":         EWNeutralBand,
			"hit_rate_tolerance_pp":   HCF05HitRateTolerancePP,
			"brier_tolerance":         HCF05BrierTolerance,
			"maxdd_tolerance_pp":      HCF05MaxDDTolerancePP,
			"layered_probability_map": 1, // sigmoid(vote_sum), abstain→0.5 (documented; see method notes)
		},
		Notes: []string{
			"分層模型機率映射 p=sigmoid(vote_sum)（abstain→0.5）；平權模型無信心輸出，Brier 以三態 p∈{0,0.5,1} 誠實計分。",
			"命中率分母只算非 abstain 日；Brier 對所有評估日計分（abstain 以 p=0.5）。缺層直接跳過（投 0），不補中性、不借層。",
		},
	}

	sorted := append([]string(nil), dates...)
	sort.Strings(sorted)
	// Index samples by date per dimension.
	byDate := map[ForceName]map[string]float64{}
	forceDates := map[string]struct{}{}
	for dim, rows := range samples {
		m := map[string]float64{}
		for _, r := range rows {
			m[r.TradingDate] = r.RawValue
			forceDates[r.TradingDate] = struct{}{}
		}
		byDate[dim] = m
	}
	// Only calendar dates that carry at least one dimension reading
	// can be evaluation days.
	evalCalendar := make([]string, 0, len(forceDates))
	for _, d := range sorted {
		if _, ok := forceDates[d]; ok {
			evalCalendar = append(evalCalendar, d)
		}
	}

	dims := []ForceName{ForceForeign, ForceFutures, ForceTSMADR, ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail}
	var days []hcf05Day
	for i := 0; i+1 < len(evalCalendar); i++ {
		d, dn := evalCalendar[i], evalCalendar[i+1]
		c0, ok0 := taiex[d]
		c1, ok1 := taiex[dn]
		if !ok0 || !ok1 {
			continue
		}
		dir := signF(c1 - c0)
		if dir == 0 {
			continue // tie days excluded
		}

		// Rebuild each dimension's Z from strictly-prior history.
		forces := make([]ForceScore, 0, len(dims))
		meanZ := 0.0
		zCount := 0
		for _, dim := range dims {
			m := byDate[dim]
			raw, has := m[d]
			if !has {
				forces = append(forces, ForceScore{Force: dim, DataAvailable: false})
				continue
			}
			var hist []RollingSample
			for _, r := range samples[dim] {
				if r.TradingDate < d {
					hist = append(hist, r)
				}
			}
			z := zScoreFromSamples(hist, raw)
			trend := trendFor(z)
			fs := ForceScore{
				Force:           dim,
				RawValue:        raw,
				ZScore:          z,
				Trend:           trend,
				DataAvailable:   true,
				SampleCount:     len(hist),
				DimensionRole:   ComputeForceProvenance(dim).DimensionRole,
				AsOfTradingDate: d,
			}
			if fs.SampleCount >= 30 {
				fs.CalibrationStatus = CalibrationEligible
			} else {
				fs.CalibrationStatus = CalibrationCalibrating
			}
			forces = append(forces, fs)
			meanZ += z
			zCount++
		}

		// Layered vote via the E07 assessment (production layer rules).
		assess := ComputeCapitalFlowAssessment(forces)
		layers := []*DirectionalAssessment{
			&assess.Institutional, &assess.Behavioral, &assess.ForeignPosition, &assess.CrossMarket,
		}
		voteSum := 0
		for _, la := range layers {
			if la == nil || !la.Available {
				continue // skip unavailable layer (vote 0)
			}
			switch la.Direction {
			case "bullish":
				voteSum++
			case "bearish":
				voteSum--
			}
		}
		day := hcf05Day{date: d, direction: dir, layeredVote: voteSum}
		if voteSum == 0 {
			day.layerAbstain = true
			day.layeredP = 0.5
		} else {
			day.layeredP = sigmoid(float64(voteSum))
			day.layeredPos = signF(float64(voteSum))
		}

		// Equal-weight model over available dimensions.
		day.ewMeanZ = 0
		if zCount > 0 {
			day.ewMeanZ = meanZ / float64(zCount)
		}
		if zCount == 0 || math.Abs(day.ewMeanZ) < EWNeutralBand {
			day.ewABst = true
			day.ewP = 0.5
		} else {
			day.ewPos = signF(day.ewMeanZ)
			day.ewP = float64(day.ewPos) // honest three-state {0,0.5,1} → {−1→0, +1→1}
			if day.ewP < 0 {
				day.ewP = 0
			}
		}
		days = append(days, day)
	}

	// Walk-forward: skip the warmup.
	if len(days) <= HCF05WarmupDays {
		res.SampleCount = len(days)
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("insufficient evaluation days: %d (≤ warmup %d)", len(days), HCF05WarmupDays)
		return res
	}
	eval := days[HCF05WarmupDays:]
	res.SampleCount = len(eval)
	if len(eval) < ValidationMinSampleDays {
		res.Status = ValidationInsufficientData
		res.Verdict = fmt.Sprintf("insufficient evaluation days: %d < %d", len(eval), ValidationMinSampleDays)
		return res
	}

	upDays := 0
	for _, d := range eval {
		if d.direction == 1 {
			upDays++
		}
	}
	baseRate := float64(upDays) / float64(len(eval))
	majorityCount := upDays
	if upDays*2 < len(eval) {
		majorityCount = len(eval) - upDays
	}
	res.Metrics["taiex_up_base_rate"] = baseRate
	res.Metrics["majority_class_hit_rate"] = float64(majorityCount) / float64(len(eval))

	hitL, nL, abstL := hcf05Score(eval, true)
	hitE, nE, abstE := hcf05Score(eval, false)
	brierL := hcf05Brier(eval, true)
	brierE := hcf05Brier(eval, false)
	ddL := hcf05MaxDD(eval, true)
	ddE := hcf05MaxDD(eval, false)

	res.Metrics["hit_layered"] = hitL
	res.Metrics["hit_ew"] = hitE
	res.Metrics["non_abstain_layered"] = float64(nL)
	res.Metrics["non_abstain_ew"] = float64(nE)
	res.Metrics["abstain_days_layered"] = float64(abstL)
	res.Metrics["abstain_days_ew"] = float64(abstE)
	res.Metrics["brier_layered"] = brierL
	res.Metrics["brier_ew"] = brierE
	res.Metrics["maxdd_layered_pp"] = ddL
	res.Metrics["maxdd_ew_pp"] = ddE

	noDegrade := hitL >= hitE-HCF05HitRateTolerancePP/100 &&
		brierL <= brierE+HCF05BrierTolerance &&
		ddL <= ddE+HCF05MaxDDTolerancePP/100
	improved := hitL > hitE || brierL < brierE || ddL < ddE
	switch {
	case noDegrade && improved:
		res.Status = ValidationPassImproved
	case noDegrade:
		res.Status = ValidationPass
	default:
		res.Status = ValidationFail
	}
	res.Verdict = fmt.Sprintf("hit L=%.1f%% vs EW=%.1f%% (tol −%.0fpp); Brier L=%.4f vs EW=%.4f (tol +%.2f); maxDD L=%.2fpp vs EW=%.2fpp (tol +%.1fpp) → %s",
		hitL*100, hitE*100, HCF05HitRateTolerancePP, brierL, brierE, HCF05BrierTolerance, ddL, ddE, HCF05MaxDDTolerancePP, res.Status)
	res.StartedAt = started
	return res
}

// hcf05Score returns (hit rate, non-abstain n, abstain days) for the
// layered (layered=true) or equal-weight model.
func hcf05Score(days []hcf05Day, layered bool) (float64, int, int) {
	hits, n, abstain := 0, 0, 0
	for _, d := range days {
		pos, isAbstain := d.layeredPos, d.layerAbstain
		if !layered {
			pos, isAbstain = d.ewPos, d.ewABst
		}
		if isAbstain {
			abstain++
			continue
		}
		n++
		if pos == d.direction {
			hits++
		}
	}
	if n == 0 {
		return 0, 0, abstain
	}
	return float64(hits) / float64(n), n, abstain
}

// hcf05Brier scores every evaluation day (abstain → p=0.5).
func hcf05Brier(days []hcf05Day, layered bool) float64 {
	if len(days) == 0 {
		return 0
	}
	var sum float64
	for _, d := range days {
		p := d.layeredP
		if !layered {
			p = d.ewP
		}
		y := 0.0
		if d.direction == 1 {
			y = 1
		}
		sum += (p - y) * (p - y)
	}
	return sum / float64(len(days))
}

// hcf05MaxDD returns the max drawdown (in percentage points) of the
// cumulative sum of daily position × direction (1 point per correct
// unit-long-short day, −1 per wrong day; abstain days hold nothing).
func hcf05MaxDD(days []hcf05Day, layered bool) float64 {
	cum, peak, maxDD := 0.0, 0.0, 0.0
	for _, d := range days {
		pos := d.layeredPos
		if !layered {
			pos = d.ewPos
		}
		cum += float64(pos * d.direction)
		if cum > peak {
			peak = cum
		}
		if peak-cum > maxDD {
			maxDD = peak - cum
		}
	}
	return maxDD * 100
}
