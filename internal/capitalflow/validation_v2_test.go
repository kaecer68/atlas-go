package capitalflow

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// weekdayCalendar returns n weekday dates starting from start.
func weekdayCalendar(start string, n int) []string {
	d0, err := time.Parse("2006-01-02", start)
	if err != nil {
		panic(err)
	}
	out := make([]string, 0, n)
	for d := d0; len(out) < n; d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			out = append(out, d.Format("2006-01-02"))
		}
	}
	return out
}

// genHCF01V2Inputs builds a deterministic synthetic panel. levelP(i) is the
// probability that the spot level on day i is +100 (else −100); draws come
// from a fixed-seed RNG so reruns reproduce exactly. TAIEX walks by rAt(i);
// OI is negative on even days (ΔOI<0 → v2a active) and positive on odd days.
func genHCF01V2Inputs(n int, levelP func(i int) float64, rAt func(i int) int) HCF01V2Inputs {
	dates := weekdayCalendar("2025-01-02", n+2)
	oi := map[string]float64{}
	spot := map[string]float64{}
	taiex := map[string]float64{}
	px := 20000.0
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < n+2; i++ {
		dt := dates[i]
		px += float64(rAt(i)) * 50
		taiex[dt] = px
		if i%2 == 0 {
			oi[dt] = -1000.0 - float64(i%7) // ΔOI<0 on even days
		} else {
			oi[dt] = 1000.0 + float64(i%7)
		}
		if rng.Float64() < levelP(i) {
			spot[dt] = 100
		} else {
			spot[dt] = -100
		}
	}
	return HCF01V2Inputs{FuturesOI: oi, SpotNet: spot, TAIEX: taiex, Dates: dates[:n+2], HolmAlpha: HCF01V2Alpha}
}

// TestValidateHCF01V2A_PassBothLayers: OI<0 days carry real directional
// content beyond sign(spot_t)/sign(ret_t) and beat the no-OI control by a
// wide margin → PASS.
func TestValidateHCF01V2A_PassBothLayers(t *testing.T) {
	// y_t = sign(spot_{t+1}): on ΔOI<0 (even) days the next level is + w.p.
	// .85 (odd levels) → real OI content; on ΔOI≥0 (odd) days + w.p. .2
	// (even levels). ret is +80% on g=1 days, 50/50 on g=0.
	in := genHCF01V2Inputs(400,
		func(i int) float64 {
			if i%2 == 0 {
				return 0.20
			}
			return 0.85
		},
		func(i int) int {
			if i%2 == 0 {
				if i%10 < 8 {
					return 1
				}
				return -1
			}
			if i%4 < 2 {
				return 1
			}
			return -1
		},
	)
	res := ValidateHCF01V2A(in)
	t.Logf("v2a PASS-case: status=%s verdict=%s", res.Status, res.Verdict)
	for _, k := range []string{"active_hit_rate", "control_c_hit_rate", "increment_pp", "increment_p", "stat_layer_p", "beta_signal"} {
		t.Logf("  %s=%.4f", k, res.Metrics[k])
	}
	if res.Status != ValidationPass {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
	if res.Metrics["increment_pp"] < HCF01V2MinIncrementPP {
		t.Fatalf("increment %.3f should clear the %.2f gate", res.Metrics["increment_pp"], HCF01V2MinIncrementPP)
	}
}

// TestValidateHCF01V2A_PassNoIncrement: statistically significant signal
// (buy-rate tilts with ΔOI<0) but the active-day hit equals the no-OI
// control → PASS_NO_INCREMENT, the pre-registered anti-persistence class.
func TestValidateHCF01V2A_PassNoIncrement(t *testing.T) {
	// A pure base-rate tilt: next-day level + w.p. .60 after ΔOI<0 days vs
	// .40 after ΔOI≥0 days (β_signal strongly >0), but ret and spot levels
	// are 50/50 and independent of the tilt → v2a's directional prediction
	// and the control C hit the same ≈.5 → no economic increment.
	in := genHCF01V2Inputs(400,
		func(i int) float64 {
			if i%2 == 0 {
				return 0.40
			}
			return 0.60
		},
		func(i int) int {
			if i%4 < 2 {
				return 1
			}
			return -1
		},
	)
	res := ValidateHCF01V2A(in)
	t.Logf("v2a PNI-case: status=%s verdict=%s increment=%.4f p=%.4f stat_p=%.4f",
		res.Status, res.Verdict, res.Metrics["increment_pp"], res.Metrics["increment_p"], res.Metrics["stat_layer_p"])
	if res.Status != ValidationPassNoIncrement {
		t.Fatalf("status = %s, want PASS_NO_INCREMENT", res.Status)
	}
	if res.Metrics["increment_pp"] >= HCF01V2MinIncrementPP && res.Metrics["increment_p"] <= res.Thresholds["alpha_holm"] {
		t.Fatalf("increment %.3f (p=%.4f) should fail the economic layer for PASS_NO_INCREMENT",
			res.Metrics["increment_pp"], res.Metrics["increment_p"])
	}
	// The triggered abandonment line must accept PASS_NO_INCREMENT.
	if ok, _ := EvaluateHCF01AbandonmentLine(res.Status, ValidationFail, ValidationFail); !ok {
		t.Fatal("PASS_NO_INCREMENT must count toward the abandonment line")
	}
}

// TestValidateHCF01V2A_Fail: spot next-day sign independent of ΔOI, spot
// and ret → no statistical layer signal.
func TestValidateHCF01V2A_Fail(t *testing.T) {
	in := genHCF01V2Inputs(400,
		func(i int) float64 { return 0.5 },
		func(i int) int {
			if i%3 != 0 {
				return 1
			}
			return -1
		},
	)
	res := ValidateHCF01V2A(in)
	t.Logf("v2a FAIL-case: status=%s verdict=%s stat_p=%.4f beta=%.4f", res.Status, res.Verdict, res.Metrics["stat_layer_p"], res.Metrics["beta_signal"])
	if res.Status != ValidationFail {
		t.Fatalf("status = %s, want FAIL", res.Status)
	}
}

// TestValidateHCF01V2A_InsufficientData: below the 252-day floor.
func TestValidateHCF01V2A_InsufficientData(t *testing.T) {
	in := genHCF01V2Inputs(100,
		func(i int) float64 { return 0.5 },
		func(i int) int { return 1 },
	)
	res := ValidateHCF01V2A(in)
	if res.Status != ValidationInsufficientData {
		t.Fatalf("status = %s, want INSUFFICIENT_DATA", res.Status)
	}
}

// TestValidateHCF01V2A_RolloverExcluded: rollover window days are dropped
// from the evaluable sample and disclosed in the notes.
func TestValidateHCF01V2A_RolloverExcluded(t *testing.T) {
	in := genHCF01V2Inputs(400,
		func(i int) float64 { return 0.5 },
		func(i int) int { return 1 }, // monotonically rising TAIEX
	)
	res := ValidateHCF01V2A(in)
	found := false
	for _, n := range res.Notes {
		if strings.Contains(n, "rollover 剔除") && !strings.Contains(n, "rollover 剔除 0") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected non-zero rollover exclusion disclosure, notes: %v", res.Notes)
	}
}

// TestValidateHCF01V2APrime_Pass: sell-side quadrant with a real
// ΔOI<0 increment over the same-cell control.
func TestValidateHCF01V2APrime_Pass(t *testing.T) {
	// Strict downtrend keeps ret<0 on every day; odd levels are − w.p. .76
	// (signal-cell next-day hit), even levels − w.p. .44 (control-cell
	// next-day hit), drawn independently per day.
	in := genHCF01V2Inputs(200,
		func(i int) float64 {
			if i%2 == 0 {
				return 0.56
			}
			return 0.24
		},
		func(i int) int { return -1 }, // strict downtrend: spot_t<0 & ret_t<0
	)
	res := ValidateHCF01V2APrime(in)
	t.Logf("v2a' PASS-case: status=%s verdict=%s signal_hit=%.3f control_hit=%.3f p=%.4f",
		res.Status, res.Verdict, res.Metrics["signal_hit"], res.Metrics["control_hit"], res.Metrics["diff_p"])
	if res.Status != ValidationPass {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
	if res.Metrics["signal_ci95_lo"] <= 0 || res.Metrics["control_ci95_hi"] >= 1 {
		t.Fatal("Wilson CIs must be disclosed and sane")
	}
}

// TestValidateHCF01V2APrime_Insufficient: cells below n≥30.
func TestValidateHCF01V2APrime_Insufficient(t *testing.T) {
	in := genHCF01V2Inputs(30,
		func(i int) float64 { return 0 },
		func(i int) int { return -1 },
	)
	res := ValidateHCF01V2APrime(in)
	if res.Status != ValidationInsufficientData {
		t.Fatalf("status = %s, want INSUFFICIENT_DATA", res.Status)
	}
}

// TestValidateHCF01V2B_WarmupAndFail: warmup consumes the pre-registered
// OI/TAIEX window (never the evaluation window); a residual series with no
// directional content FAILs.
func TestValidateHCF01V2B_WarmupAndFail(t *testing.T) {
	dates := weekdayCalendar("2024-07-01", 600)
	oi := map[string]float64{}
	taiex := map[string]float64{}
	spot := map[string]float64{}
	px := 20000.0
	for i, dt := range dates {
		px += 50
		if i%2 == 0 {
			px += 25
		} else {
			px -= 25
		}
		taiex[dt] = px
		// OI tracks TAIEX (hedge structure) + a smooth deterministic cycle
		// whose Δ sign carries no spot information.
		oi[dt] = 3*px + 10000*(float64(i%30)/30.0-0.5)
		if i%4 < 2 {
			spot[dt] = 100
		} else {
			spot[dt] = -100
		}
	}
	in := HCF01V2Inputs{FuturesOI: oi, SpotNet: spot, TAIEX: taiex, Dates: dates, HolmAlpha: HCF01V2Alpha}
	res := ValidateHCF01V2B(in)
	t.Logf("v2b FAIL-case: status=%s verdict=%s oos_hit=%.3f train_rho=%.3f", res.Status, res.Verdict, res.Metrics["oos_hit_rate"], res.Metrics["train_abs_rho"])
	if res.Status != ValidationFail {
		t.Fatalf("status = %s, want FAIL (no directional content)", res.Status)
	}
	warmupNote := false
	for _, n := range res.Notes {
		if strings.Contains(n, "126 日消耗 OI/TAIEX 窗") && strings.Contains(n, HCF01V2BWarmupStartDate) {
			warmupNote = true
		}
	}
	if !warmupNote {
		t.Fatalf("warmup disclosure missing from notes: %v", res.Notes)
	}
	// Warmup accounting: evaluation window = full series minus warmup days;
	// the residual sample must be well below the raw 600-day input.
	if res.SampleCount > 600-HCF01V2BWarmupDays {
		t.Fatalf("SampleCount %d must exclude the %d warmup days", res.SampleCount, HCF01V2BWarmupDays)
	}
}

// TestValidateHCF01V2B_Insufficient: too few days after warmup.
func TestValidateHCF01V2B_Insufficient(t *testing.T) {
	dates := weekdayCalendar("2024-07-01", 100)
	oi, taiex := map[string]float64{}, map[string]float64{}
	for i, dt := range dates {
		taiex[dt] = float64(20000 + i)
		oi[dt] = float64(-1000 + i)
	}
	in := HCF01V2Inputs{FuturesOI: oi, SpotNet: map[string]float64{}, TAIEX: taiex, Dates: dates, HolmAlpha: HCF01V2Alpha}
	res := ValidateHCF01V2B(in)
	if res.Status != ValidationInsufficientData {
		t.Fatalf("status = %s, want INSUFFICIENT_DATA", res.Status)
	}
}

// TestHolmSplit: pre-registered family {v2a, v2a′, v2b} → α/3, α/2, α/1.
func TestHolmSplit(t *testing.T) {
	if got := HolmAlphaForRank(0, HCF01V2HolmFamilySize); got != 0.05/3 {
		t.Fatalf("rank0 alpha = %v", got)
	}
	if got := HolmAlphaForRank(1, HCF01V2HolmFamilySize); got != 0.05/2 {
		t.Fatalf("rank1 alpha = %v", got)
	}
	if got := HolmAlphaForRank(2, HCF01V2HolmFamilySize); got != 0.05 {
		t.Fatalf("rank2 alpha = %v", got)
	}
	pass := HolmPass([]float64{0.001, 0.02, 0.5}, 3)
	if !pass[0] || !pass[1] || pass[2] {
		t.Fatalf("Holm step-down wrong: %v", pass)
	}
	pass = HolmPass([]float64{0.017, 0.018, 0.019}, 3) // smallest fails α/3 → all rejected
	if pass[0] || pass[1] || pass[2] {
		t.Fatalf("Holm step-down must stop after first failure: %v", pass)
	}
}

// TestEvaluateHCF01AbandonmentLine (arbitration §2.5).
func TestEvaluateHCF01AbandonmentLine(t *testing.T) {
	cases := []struct {
		v2a, prime, v2b string
		want            bool
	}{
		{ValidationFail, ValidationFail, ValidationFail, true},
		{ValidationPassNoIncrement, ValidationFail, ValidationPassNoIncrement, true},
		{ValidationPassNoIncrement, ValidationPassNoIncrement, ValidationFail, true},
		{ValidationPass, ValidationFail, ValidationFail, false}, // any genuine PASS keeps the chain alive
		{ValidationFail, ValidationFail, ValidationPass, false}, // v2b PASS (above baseline A) survives
		{ValidationInsufficientData, ValidationFail, ValidationFail, false},
		{ValidationFail, ValidationInsufficientData, ValidationFail, false},
	}
	for i, c := range cases {
		got, note := EvaluateHCF01AbandonmentLine(c.v2a, c.prime, c.v2b)
		if got != c.want {
			t.Errorf("case %d (%s/%s/%s) = %v, want %v (%s)", i, c.v2a, c.prime, c.v2b, got, c.want, note)
		}
	}
	if ok, note := EvaluateHCF01AbandonmentLine(ValidationFail, ValidationFail, ValidationFail); !ok || !strings.Contains(note, "DEMOTED") {
		t.Fatalf("triggered line must carry the DEMOTED marker, got %q", note)
	}
}

// TestWilsonCI: known-value sanity.
func TestWilsonCI(t *testing.T) {
	hits := make([]int, 100)
	for i := range hits {
		if i < 60 {
			hits[i] = 1
		}
	}
	lo, hi := wilsonCI(hits)
	if lo <= 0.50 || hi >= 0.70 {
		t.Fatalf("Wilson 60%%/n=100 CI = [%.3f, %.3f], want inside (0.50, 0.70)", lo, hi)
	}
}
