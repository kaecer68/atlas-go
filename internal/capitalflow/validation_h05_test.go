package capitalflow

// Synthetic-world tests for H-CF-05 (plan §3.2). Two worlds are
// constructed:
//
//   - h05WorldLayeredBetter: the layered model abstains exactly on
//     the days the equal-weight model is wrong (layer disagreement →
//     vote_sum = 0), so layered strictly improves on hit rate, Brier
//     and drawdown → PASS(improved).
//   - h05WorldEWBetter: the layered model is dragged wrong by its
//     3-vote degradation (no TSM ADR channel) on a minority of days
//     where the equal-weight mean Z still reads the true direction →
//     FAIL.
//
// The Z-scores rely on alternating ±1 "normal" history (mean 0), so
// the trap-day raw values below are calibrated to land on the intended
// side of the trend thresholds.

import (
	"testing"
)

// h05World builds date series + per-dimension rolling samples.
//
//   - n days, direction alternates +1/−1 (TAIEX zigzag 100/102).
//   - normalDays: all dims raw = direction.
//   - downTrap (direction −1): actors +2, futures +2, gov/retail govRaw;
//   - upTrap (direction +1): mirrored.
//   - adrDay raws only when withADR (otherwise the cross_market layer
//     is unavailable — the pre-registered 3-vote degradation).
func h05World(n, trapEvery int, govRaw, futRaw float64, withADR bool) (map[ForceName][]RollingSample, map[string]float64, []string) {
	dates := dateSeq(n)
	taiex := make(map[string]float64, n)
	for i, d := range dates {
		if i%2 == 0 {
			taiex[d] = 100
		} else {
			taiex[d] = 102
		}
	}
	dims := []ForceName{ForceForeign, ForceFutures, ForceTSMADR, ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail}
	samples := make(map[ForceName][]RollingSample, len(dims))
	for _, dim := range dims {
		if dim == ForceTSMADR && !withADR {
			continue
		}
		for i, d := range dates {
			dir := 1 - 2*(i%2) // +1 even, −1 odd
			var raw float64
			switch {
			case i%trapEvery == trapEvery-1 && dir == -1:
				// down-trap: layered reads bullish while market falls
				switch dim {
				case ForceForeign, ForceInstitutional, ForceDealer:
					raw = 2
				case ForceFutures:
					raw = futRaw
				case ForceGovernment, ForceRetail:
					raw = govRaw
				default: // ForceTSMADR
					raw = float64(dir)
				}
			case i%trapEvery == trapEvery-1 && dir == 1:
				// up-trap mirrored
				switch dim {
				case ForceForeign, ForceInstitutional, ForceDealer:
					raw = -2
				case ForceFutures:
					raw = -futRaw
				case ForceGovernment, ForceRetail:
					raw = -govRaw
				default:
					raw = float64(dir)
				}
			default:
				raw = float64(dir)
			}
			samples[dim] = append(samples[dim], RollingSample{
				TradingDate: d, Dimension: dim, RawValue: raw,
				Unit: "test", SourceID: "SRC-TEST",
			})
		}
	}
	return samples, taiex, dates
}

func TestHypothesis05InsufficientData(t *testing.T) {
	samples, taiex, dates := h05World(200, 10, -1, 0, false)
	res := ValidateHypothesis05(samples, taiex, dates)
	if res.Status != ValidationInsufficientData {
		t.Fatalf("status=%s, want INSUFFICIENT_DATA (n=%d)", res.Status, res.SampleCount)
	}
}

func TestHypothesis05LayeredBetterPassImproved(t *testing.T) {
	// 10% down-traps where the layered model abstains (vote 0) while
	// the equal-weight model reads the wrong direction.
	samples, taiex, dates := h05World(400, 10, -1, 0, false)
	res := ValidateHypothesis05(samples, taiex, dates)
	if res.Status != ValidationPassImproved {
		t.Fatalf("status=%s verdict=%s metrics=%v", res.Status, res.Verdict, res.Metrics)
	}
	if res.Metrics["abstain_days_layered"] <= 0 {
		t.Fatalf("expected layered abstain days, metrics=%v", res.Metrics)
	}
	if res.Metrics["hit_layered"] <= res.Metrics["hit_ew"] {
		t.Fatalf("layered hit %v should exceed EW hit %v", res.Metrics["hit_layered"], res.Metrics["hit_ew"])
	}
	if res.Metrics["brier_layered"] >= res.Metrics["brier_ew"] {
		t.Fatalf("layered Brier %v should beat EW Brier %v", res.Metrics["brier_layered"], res.Metrics["brier_ew"])
	}
	if res.Metrics["maxdd_layered_pp"] >= res.Metrics["maxdd_ew_pp"] {
		t.Fatalf("layered maxDD %v should beat EW maxDD %v", res.Metrics["maxdd_layered_pp"], res.Metrics["maxdd_ew_pp"])
	}
}

func TestHypothesis05EWBetterFail(t *testing.T) {
	// 2% traps where the layered model is wrong (3-vote degradation)
	// while the equal-weight mean Z still reads the true direction.
	samples, taiex, dates := h05World(420, 50, -10, 2, false)
	res := ValidateHypothesis05(samples, taiex, dates)
	if res.Status != ValidationFail {
		t.Fatalf("status=%s verdict=%s metrics=%v", res.Status, res.Verdict, res.Metrics)
	}
	if res.Metrics["hit_ew"] <= res.Metrics["hit_layered"] {
		t.Fatalf("EW hit %v should exceed layered hit %v", res.Metrics["hit_ew"], res.Metrics["hit_layered"])
	}
}

func TestHypothesis05WithADRFourVotes(t *testing.T) {
	// With a real TSM ADR channel the cross_market layer joins the
	// vote; on normal days all four layers agree with the direction,
	// so the layered model stays correct — the world must not change
	// verdict class merely because ADR exists.
	samples, taiex, dates := h05World(400, 10, -1, 0, true)
	res := ValidateHypothesis05(samples, taiex, dates)
	if res.Status != ValidationPassImproved && res.Status != ValidationPass {
		t.Fatalf("status=%s verdict=%s metrics=%v", res.Status, res.Verdict, res.Metrics)
	}
}
