package capitalflow

// Synthetic-series tests for the Phase 1 validators. The three
// outcomes PASS / FAIL / INSUFFICIENT_DATA are locked here so the
// pre-registered verdict logic cannot drift silently.

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// dateSeq returns n consecutive calendar dates starting 2026-01-01.
// The validators are calendar-agnostic (they only need a sorted,
// next-day-lookup-able sequence), so tests use consecutive days.
func dateSeq(n int) []string {
	out := make([]string, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out[i] = base.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

func TestBinomialOneTailP(t *testing.T) {
	// P(X>=8 | n=10, p=0.5) = 56/1024 ≈ 0.0547.
	if got := binomialOneTailP(8, 10); math.Abs(got-56.0/1024.0) > 1e-9 {
		t.Fatalf("binomialOneTailP(8,10)=%v, want %v", got, 56.0/1024.0)
	}
	// P(X>=10 | n=10) = 1/1024.
	if got := binomialOneTailP(10, 10); math.Abs(got-1.0/1024.0) > 1e-9 {
		t.Fatalf("binomialOneTailP(10,10)=%v, want %v", got, 1.0/1024.0)
	}
	// At or below chance the one-tail p is ≥ 0.5.
	if got := binomialOneTailP(5, 10); got < 0.5 {
		t.Fatalf("binomialOneTailP(5,10)=%v, want >= 0.5", got)
	}
	if got := binomialOneTailP(0, 10); got != 1 {
		t.Fatalf("binomialOneTailP(0,10)=%v, want 1", got)
	}
}

func TestBlockBootstrapHitP(t *testing.T) {
	// A perfectly consistent hit series gets a tiny bootstrap p; a
	// coin-flip series gets ~1.
	perfect := make([]int, 200)
	for i := range perfect {
		perfect[i] = 1
	}
	if p := blockBootstrapHitP(perfect, 2000); p > 0.01 {
		t.Fatalf("blockBootstrapHitP(perfect)=%v, want <= 0.01", p)
	}
	coin := make([]int, 200)
	for i := range coin {
		if i%2 == 0 {
			coin[i] = 1
		}
	}
	if p := blockBootstrapHitP(coin, 2000); p < 0.5 {
		t.Fatalf("blockBootstrapHitP(coin)=%v, want >= 0.5", p)
	}
}

func TestPearson(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	if r := pearson(xs, []float64{2, 4, 6, 8, 10}); math.Abs(r-1) > 1e-9 {
		t.Fatalf("pearson perfect=%v, want 1", r)
	}
	if r := pearson(xs, []float64{10, 8, 6, 4, 2}); math.Abs(r+1) > 1e-9 {
		t.Fatalf("pearson inverse=%v, want -1", r)
	}
	if !math.IsNaN(pearson(xs, []float64{1, 2, 3})) {
		t.Fatalf("pearson length mismatch should be NaN")
	}
	if r := pearson(xs, []float64{1, 1, 1, 1, 1}); r != 0 {
		t.Fatalf("pearson constant series=%v, want 0", r)
	}
}

// --- H-CF-01 -----------------------------------------------------------

// buildH01Series builds an OI/spot world where ΔOI_t perfectly leads
// spot_{t+1} when signFactor=+1 (100% direction hits at lag 1) or
// anti-leads when signFactor=-1 (0% hits).
func buildH01Series(n int, signFactor int) (map[string]float64, map[string]float64, []string) {
	dates := dateSeq(n)
	oi := make(map[string]float64, n)
	spot := make(map[string]float64, n)
	// ΔOI alternates +1/−1 (oi alternates around 0 with step 2).
	prev := 0.0
	for i, d := range dates {
		oiVal := float64(i%2)*2 - 1 // -1, +1, -1, ...
		oi[d] = oiVal
		if i >= 1 {
			doi := oiVal - prev
			// spot at the NEXT day reacts to today's ΔOI (lag-1 lead).
			if i+1 < n {
				spot[dates[i+1]] = float64(signFactor * signF(doi) * 5)
			}
		}
		prev = oiVal
	}
	return oi, spot, dates
}

func TestHypothesis01InsufficientData(t *testing.T) {
	oi, spot, dates := buildH01Series(100, 1) // ~98 pairs < 252
	res := ValidateHypothesis01(oi, spot, dates)
	if res.Status != ValidationInsufficientData {
		t.Fatalf("status=%s, want INSUFFICIENT_DATA (n=%d)", res.Status, res.SampleCount)
	}
	if res.SampleCount >= ValidationMinSampleDays {
		t.Fatalf("sample_count=%d should be < %d", res.SampleCount, ValidationMinSampleDays)
	}
}

func TestHypothesis01Pass(t *testing.T) {
	oi, spot, dates := buildH01Series(310, 1)
	res := ValidateHypothesis01(oi, spot, dates)
	if res.Status != ValidationPass {
		t.Fatalf("status=%s verdict=%s metrics=%v", res.Status, res.Verdict, res.Metrics)
	}
	if res.Metrics["k_star"] != 1 {
		t.Fatalf("k_star=%v, want 1 (perfect lag-1 leader; tie broken to smaller k)", res.Metrics["k_star"])
	}
	if res.Metrics["oos_hit_rate"] != 1 {
		t.Fatalf("oos_hit_rate=%v, want 1", res.Metrics["oos_hit_rate"])
	}
	if res.Metrics["binomial_p"] > HCF01MaxBinomialP {
		t.Fatalf("binomial_p=%v should be <= %v", res.Metrics["binomial_p"], HCF01MaxBinomialP)
	}
}

func TestHypothesis01Fail(t *testing.T) {
	oi, spot, dates := buildH01Series(310, -1) // anti-leading: 0% OOS hits
	res := ValidateHypothesis01(oi, spot, dates)
	if res.Status != ValidationFail {
		t.Fatalf("status=%s verdict=%s", res.Status, res.Verdict)
	}
}

// --- H-CF-02 -----------------------------------------------------------

func buildH02Series(n int, signFactor int) (map[string]float64, map[string]float64, []string) {
	dates := dateSeq(n)
	taiex := make(map[string]float64, n)
	adr := make(map[string]float64, n)
	for i, d := range dates {
		// TAIEX alternates 100 → 102 → 100 → …, so the next-day
		// direction alternates +1/−1.
		taiex[d] = 101 + float64(i%2)*2 - 1
	}
	for i, d := range dates {
		if i+1 >= n {
			break
		}
		mov := taiex[dates[i+1]] - taiex[d]
		adr[d] = float64(signFactor * signF(mov) * 2)
	}
	return adr, taiex, dates
}

func TestHypothesis02InsufficientData(t *testing.T) {
	adr, taiex, dates := buildH02Series(100, 1)
	res := ValidateHypothesis02(adr, taiex, dates)
	if res.Status != ValidationInsufficientData {
		t.Fatalf("status=%s, want INSUFFICIENT_DATA (n=%d)", res.Status, res.SampleCount)
	}
}

func TestHypothesis02Pass(t *testing.T) {
	adr, taiex, dates := buildH02Series(300, 1)
	res := ValidateHypothesis02(adr, taiex, dates)
	if res.Status != ValidationPass {
		t.Fatalf("status=%s verdict=%s metrics=%v", res.Status, res.Verdict, res.Metrics)
	}
	if res.Metrics["hit_rate"] != 1 {
		t.Fatalf("hit_rate=%v, want 1", res.Metrics["hit_rate"])
	}
	if res.Metrics["binomial_p"] > HCF02MaxBinomialP {
		t.Fatalf("binomial_p=%v should be <= %v", res.Metrics["binomial_p"], HCF02MaxBinomialP)
	}
}

func TestHypothesis02Fail(t *testing.T) {
	adr, taiex, dates := buildH02Series(300, -1)
	res := ValidateHypothesis02(adr, taiex, dates)
	if res.Status != ValidationFail {
		t.Fatalf("status=%s verdict=%s", res.Status, res.Verdict)
	}
	if res.Metrics["majority_class_hit_rate"] <= 0 {
		t.Fatalf("majority_class_hit_rate=%v should be reported", res.Metrics["majority_class_hit_rate"])
	}
}

// --- Pre-registered threshold constants --------------------------------

// TestPreRegisteredThresholdsLocked pins the compile-time decision
// thresholds. Changing any of these invalidates the pre-registration
// (plan §3.1) and requires a plan revision — this test fails loudly if
// someone edits them casually.
func TestPreRegisteredThresholdsLocked(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"ValidationMinSampleDays", ValidationMinSampleDays, 252},
		{"HCF01MinTrainAbsRho", HCF01MinTrainAbsRho, 0.10},
		{"HCF01MinOOSHitRate", HCF01MinOOSHitRate, 0.55},
		{"HCF01MaxBinomialP", HCF01MaxBinomialP, 0.05},
		{"HCF02MinHitRate", HCF02MinHitRate, 0.55},
		{"HCF02MaxBinomialP", HCF02MaxBinomialP, 0.05},
		{"HCF02MinFoldHitRate", HCF02MinFoldHitRate, 0.50},
		{"ValidationFolds", ValidationFolds, 3},
		{"BlockBootstrapBlockLen", BlockBootstrapBlockLen, 5},
		{"HCF05WarmupDays", HCF05WarmupDays, 126},
		{"EWNeutralBand", EWNeutralBand, 0.1},
		{"HCF05HitRateTolerancePP", HCF05HitRateTolerancePP, 1.0},
		{"HCF05BrierTolerance", HCF05BrierTolerance, 0.02},
		{"HCF05MaxDDTolerancePP", HCF05MaxDDTolerancePP, 0.5},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, pre-registered value is %v (plan §3.1)", c.name, c.got, c.want)
		}
	}
}

func TestSigmoid(t *testing.T) {
	if got := sigmoid(0); math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("sigmoid(0)=%v, want 0.5", got)
	}
	if got := sigmoid(4); math.Abs(got-0.982013797) > 1e-8 {
		t.Fatalf("sigmoid(4)=%v", got)
	}
	if fmt.Sprintf("%.2f", sigmoid(-100)) != "0.00" {
		t.Fatalf("sigmoid(-100) should underflow to ~0")
	}
}
