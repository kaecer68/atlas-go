package charter

import (
	"math"
	"math/rand"
	"sort"

	"gonum.org/v1/gonum/stat/distuv"

	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// ─── Paired t-test ────────────────────────────────────────────────────────

// TTestResult is the outcome of a paired two-sided Student's t-test on
// same-day return differences (feature − baseline).
type TTestResult struct {
	T           float64 `json:"t"`
	DF          int     `json:"df"`
	P           float64 `json:"p"`
	MeanDiff    float64 `json:"mean_daily_return_diff"`
	Significant bool    `json:"significant"` // p < 0.05 (α = 0.05)
}

// PairedTTest tests H0: mean(feature − baseline) = 0 on paired observations
// (the two arms replay the same days deterministically, so each day's return
// pair is a dependent sample). Two-sided with α = 0.05.
//
// Degenerate guards: n < 2 returns a zero result; a constant non-zero
// difference yields t = ±Inf with p = 0 (statistically conclusive);
// identical series yields t = 0, p = 1.
func PairedTTest(baseline, feature []float64) TTestResult {
	n := min(len(feature), len(baseline))
	if n < 2 {
		return TTestResult{DF: max(n-1, 0)}
	}
	var sum, sumSq float64
	for i := 0; i < n; i++ {
		d := feature[i] - baseline[i]
		sum += d
		sumSq += d * d
	}
	mean := sum / float64(n)
	variance := (sumSq - float64(n)*mean*mean) / float64(n-1)
	if variance < 0 {
		variance = 0 // float64 rounding can produce tiny negatives
	}
	sd := math.Sqrt(variance)
	df := n - 1

	var t float64
	switch {
	case sd == 0 && mean == 0:
		t = 0
	case sd == 0:
		t = math.Inf(1) // constant non-zero difference → decisive
	default:
		t = mean / (sd / math.Sqrt(float64(n)))
	}
	p := 2 * distuv.StudentsT{Nu: float64(df), Sigma: 1}.Survival(math.Abs(t))
	if math.IsNaN(p) {
		p = 1
	}
	return TTestResult{
		T:           t,
		DF:          df,
		P:           p,
		MeanDiff:    mean,
		Significant: p < 0.05,
	}
}

// ─── BCa bootstrap ────────────────────────────────────────────────────────

// BootstrapResult is the BCa 95% confidence interval for the observed
// difference of a nonlinear metric between the two arms.
type BootstrapResult struct {
	Observed    float64 `json:"observed_diff"`
	CI95Low     float64 `json:"ci95_low"`
	CI95High    float64 `json:"ci95_high"`
	Resamples   int     `json:"resamples"`
	Significant bool    `json:"significant"` // CI excludes 0
	// Degenerate marks bootstrap distributions that cannot support a CI:
	// the resampled statistic never reproduces the observed value (typical
	// for path-dependent stats like MaxDrawdown, where resampling destroys
	// the temporal ordering). Such CIs are reported but MUST NOT be read as
	// significance evidence.
	Degenerate bool `json:"degenerate"`
}

// BCaBootstrap computes a bias-corrected-and-accelerated 100*(1-alpha)%
// confidence interval for stat(baseline, feature) using paired resampling:
// the same day indices are drawn for both arms, preserving the paired
// structure of the deterministic replay. The statistic returns a scalar
// where positive means "feature better than baseline" (by the metric's
// convention — see SharpeDiff / MaxDrawdownDiff).
//
// resamples = 10_000 for the C3 harness. The RNG is seeded for reproducible
// CIs. The acceleration term is estimated by the delete-one jackknife.
func BCaBootstrap(baseline, feature []float64, stat func(baseline, feature []float64) float64, resamples int, alpha float64) BootstrapResult {
	n := min(len(feature), len(baseline))
	baseline = baseline[:n]
	feature = feature[:n]

	observed := stat(baseline, feature)

	// Paired bootstrap distribution.
	rng := rand.New(rand.NewSource(20260822)) // deterministic seed
	dist := make([]float64, resamples)
	bs, fs := make([]float64, n), make([]float64, n)
	below := 0
	for b := range resamples {
		for i := 0; i < n; i++ {
			k := rng.Intn(n)
			bs[i], fs[i] = baseline[k], feature[k]
		}
		v := stat(bs, fs)
		dist[b] = v
		if v < observed {
			below++
		}
	}
	sort.Float64s(dist)

	// Bias correction: z0 = Φ⁻¹(proportion of bootstrap estimates < observed).
	prop := float64(below) / float64(resamples)
	prop = math.Max(math.Min(prop, 1-1e-12), 1e-12) // keep Φ⁻¹ finite
	z0 := normalQuantile(prop)

	// Acceleration via delete-one jackknife.
	acc := jackknifeAcceleration(baseline, feature, stat)

	norm := distuv.Normal{Mu: 0, Sigma: 1}
	zCrit := norm.Quantile(alpha / 2)
	a1 := norm.CDF(z0 + (z0+zCrit)/(1-acc*(z0+zCrit)))
	a2 := norm.CDF(z0 + (z0-zCrit)/(1-acc*(z0-zCrit)))

	lo := empiricalQuantile(dist, a1)
	hi := empiricalQuantile(dist, a2)
	if lo > hi {
		lo, hi = hi, lo // degenerate distributions can invert the BCa endpoints
	}
	// A valid CI brackets the observed statistic. When it does not, the
	// bootstrap distribution is degenerate (e.g. resampling destroyed the
	// path structure of a MaxDrawdown) and significance must not be claimed.
	degenerate := observed < lo || observed > hi
	return BootstrapResult{
		Observed:    observed,
		CI95Low:     lo,
		CI95High:    hi,
		Resamples:   resamples,
		Significant: !degenerate && (lo > 0 || hi < 0),
		Degenerate:  degenerate,
	}
}

// jackknifeAcceleration estimates the BCa acceleration parameter a from the
// delete-one jackknife of the statistic:
//
//	a = Σᵢ(θ̄(·) − θ̂(i))³ / (6 · [Σᵢ(θ̄(·) − θ̂(i))²]^(3/2))
func jackknifeAcceleration(baseline, feature []float64, stat func(baseline, feature []float64) float64) float64 {
	n := len(baseline)
	if n < 2 {
		return 0
	}
	bs, fs := make([]float64, n-1), make([]float64, n-1)
	jack := make([]float64, n)
	var sum float64
	for i := range n {
		k := 0
		for j := range n {
			if j == i {
				continue
			}
			bs[k], fs[k] = baseline[j], feature[j]
			k++
		}
		jack[i] = stat(bs, fs)
		sum += jack[i]
	}
	mean := sum / float64(n)
	var num, den float64
	for i := range n {
		d := mean - jack[i]
		num += d * d * d
		den += d * d
	}
	if den == 0 {
		return 0
	}
	return num / (6 * math.Pow(den, 1.5))
}

// normalQuantile is Φ⁻¹ via gonum's standard normal.
func normalQuantile(p float64) float64 {
	return distuv.Normal{Mu: 0, Sigma: 1}.Quantile(p)
}

// empiricalQuantile returns the type-7 quantile (linear interpolation) of a
// sorted sample — the same convention as R's quantile(..., type=7).
func empiricalQuantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// ─── C3 metric statistics (nonlinear) ─────────────────────────────────────

// SharpeDiff returns Sharpe(feature) − Sharpe(baseline) on daily return
// series (annualized by √252). Positive → feature arm has higher Sharpe.
func SharpeDiff(baselineReturns, featureReturns []float64) float64 {
	return sharpe(featureReturns) - sharpe(baselineReturns)
}

func sharpe(returns []float64) float64 {
	return portfolio.ComputeSharpe(returns, portfolio.SharpeConfig{Frequency: portfolio.FrequencyPerDay})
}

// MaxDrawdown returns the maximum peak-to-trough decline of an equity curve
// as a positive fraction (0.25 = 25% drawdown). Empty or flat curves yield 0.
func MaxDrawdown(equity []float64) float64 {
	if len(equity) == 0 {
		return 0
	}
	peak := equity[0]
	maxDD := 0.0
	for _, v := range equity {
		if v > peak {
			peak = v
		}
		if peak > 0 {
			if dd := (peak - v) / peak; dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

// MaxDrawdownDiff returns MaxDrawdown(baseline) − MaxDrawdown(feature).
// Positive → feature arm has a shallower (better) drawdown.
func MaxDrawdownDiff(baselineEquity, featureEquity []float64) float64 {
	return MaxDrawdown(baselineEquity) - MaxDrawdown(featureEquity)
}
