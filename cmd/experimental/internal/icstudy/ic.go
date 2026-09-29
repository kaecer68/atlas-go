// Package icstudy holds the cross-sectional information-coefficient math shared
// by the experimental IC-study commands (chips-ic-study and sbl-ic-study).
//
// Keeping it in one place means the two studies cannot drift apart in how they
// rank, correlate, or aggregate: the numbers they publish are comparable by
// construction.
package icstudy

import (
	"encoding/json"
	"math"
	"sort"
)

// MinObs is the smallest cross-section on which a rank correlation is computed.
// Below it the estimate is dominated by noise, so the helpers return NaN and
// the caller drops the date.
const MinObs = 8

// Rank returns 1-based ranks with ties sharing their mean rank.
func Rank(xs []float64) []float64 {
	type pair struct {
		v float64
		i int
	}
	ps := make([]pair, len(xs))
	for i, v := range xs {
		ps[i] = pair{v: v, i: i}
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

// Pearson returns the Pearson correlation of xs and ys, or NaN when the lengths
// differ or either side has zero variance.
func Pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) == 0 {
		return math.NaN()
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	n := float64(len(xs))
	mx /= n
	my /= n
	var sxy, sxx, syy float64
	for i := range xs {
		dx := xs[i] - mx
		dy := ys[i] - my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx <= 0 || syy <= 0 {
		return math.NaN()
	}
	return sxy / math.Sqrt(sxx*syy)
}

// Spearman returns the rank correlation of xs and ys. It returns NaN when the
// cross-section is shorter than MinObs.
func Spearman(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < MinObs {
		return math.NaN()
	}
	return Pearson(Rank(xs), Rank(ys))
}

// PartialSpearman returns the correlation of xs and ys after both ranked series
// have been residualised on the ranked controls — the usual partial (rank)
// correlation, used to show a signal's increment beyond known factors.
func PartialSpearman(xs, ys []float64, controls [][]float64) float64 {
	if len(xs) != len(ys) || len(xs) < MinObs {
		return math.NaN()
	}
	ranks := make([][]float64, len(controls))
	for i, c := range controls {
		if len(c) != len(xs) {
			return math.NaN()
		}
		ranks[i] = Rank(c)
	}
	ex, ok := residualize(Rank(xs), ranks)
	if !ok {
		return math.NaN()
	}
	ey, ok := residualize(Rank(ys), ranks)
	if !ok {
		return math.NaN()
	}
	return Pearson(ex, ey)
}

// residualize removes from y the OLS fit on [1, controls...]. It reports false
// when the normal equations are singular (collinear controls).
func residualize(y []float64, controls [][]float64) ([]float64, bool) {
	p := len(controls) + 1
	n := len(y)
	// Normal equations X'X beta = X'y for the design [1 | controls...].
	a := make([][]float64, p)
	b := make([]float64, p)
	for i := range a {
		a[i] = make([]float64, p)
	}
	col := func(i int, r int) float64 {
		if i == 0 {
			return 1
		}
		return controls[i-1][r]
	}
	for i := 0; i < p; i++ {
		for j := 0; j < p; j++ {
			var s float64
			for r := 0; r < n; r++ {
				s += col(i, r) * col(j, r)
			}
			a[i][j] = s
		}
		var s float64
		for r := 0; r < n; r++ {
			s += col(i, r) * y[r]
		}
		b[i] = s
	}
	beta, ok := solve(a, b)
	if !ok {
		return nil, false
	}
	out := make([]float64, n)
	for r := 0; r < n; r++ {
		var fit float64
		for i := 0; i < p; i++ {
			fit += beta[i] * col(i, r)
		}
		out[r] = y[r] - fit
	}
	return out, true
}

// solve runs Gaussian elimination with partial pivoting on a square system.
func solve(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n+1)
		copy(m[i], a[i])
		m[i][n] = b[i]
	}
	for c := 0; c < n; c++ {
		piv := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[piv][c]) {
				piv = r
			}
		}
		if math.Abs(m[piv][c]) < 1e-12 {
			return nil, false
		}
		m[c], m[piv] = m[piv], m[c]
		for r := c + 1; r < n; r++ {
			f := m[r][c] / m[c][c]
			for k := c; k <= n; k++ {
				m[r][k] -= f * m[c][k]
			}
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := m[r][n]
		for k := r + 1; k < n; k++ {
			s -= m[r][k] * x[k]
		}
		x[r] = s / m[r][r]
	}
	return x, true
}

// Stats is one aggregated IC cell: the summary of a per-date IC series.
type Stats struct {
	Feature string  `json:"feature"`
	Horizon string  `json:"horizon,omitempty"`
	Group   string  `json:"group,omitempty"`
	Control string  `json:"control,omitempty"`
	N       int     `json:"n_dates"`
	SumN    int     `json:"total_obs"`
	MeanIC  float64 `json:"mean_ic"`
	StdIC   float64 `json:"std_ic"`
	ICIR    float64 `json:"icir"`
	PosPct  float64 `json:"positive_pct"`
}

// MarshalJSON renders non-finite cells as null: a NaN IC means "no estimate"
// rather than a value, and encoding/json refuses NaN outright.
func (s Stats) MarshalJSON() ([]byte, error) {
	type shadow struct {
		Feature string   `json:"feature"`
		Horizon string   `json:"horizon,omitempty"`
		Group   string   `json:"group,omitempty"`
		Control string   `json:"control,omitempty"`
		N       int      `json:"n_dates"`
		SumN    int      `json:"total_obs"`
		MeanIC  *float64 `json:"mean_ic"`
		StdIC   *float64 `json:"std_ic"`
		ICIR    *float64 `json:"icir"`
		PosPct  *float64 `json:"positive_pct"`
	}
	return json.Marshal(shadow{
		Feature: s.Feature, Horizon: s.Horizon, Group: s.Group, Control: s.Control,
		N: s.N, SumN: s.SumN,
		MeanIC: finite(s.MeanIC), StdIC: finite(s.StdIC), ICIR: finite(s.ICIR), PosPct: finite(s.PosPct),
	})
}

// finite converts a non-finite float to nil so it encodes as null.
func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// Aggregate summarises a per-date IC series (population standard deviation, so
// ICIR is mean/std exactly as reported by the earlier chips study).
func Aggregate(ics []float64) Stats {
	st := Stats{N: len(ics)}
	if len(ics) == 0 {
		st.MeanIC, st.StdIC, st.ICIR, st.PosPct = math.NaN(), math.NaN(), math.NaN(), math.NaN()
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

// ICByDate computes the per-date cross-sectional Spearman IC of feat against
// label. Rows whose feature or label is NaN are dropped, and dates with fewer
// than minPerDate surviving rows are skipped.
func ICByDate[T any](rows []T, dateOf func(T) string, feat, label func(T) float64, minPerDate int) map[string]float64 {
	type bucket struct {
		f []float64
		l []float64
	}
	byDate := map[string]*bucket{}
	for _, r := range rows {
		f, l := feat(r), label(r)
		if math.IsNaN(f) || math.IsNaN(l) {
			continue
		}
		b, ok := byDate[dateOf(r)]
		if !ok {
			b = &bucket{}
			byDate[dateOf(r)] = b
		}
		b.f = append(b.f, f)
		b.l = append(b.l, l)
	}
	out := make(map[string]float64, len(byDate))
	for d, b := range byDate {
		if len(b.f) < minPerDate {
			continue
		}
		if ic := Spearman(b.f, b.l); !math.IsNaN(ic) {
			out[d] = ic
		}
	}
	return out
}

// PartialICByDate is ICByDate with the listed controls partialled out of every
// date's cross-section.
func PartialICByDate[T any](rows []T, dateOf func(T) string, feat, label func(T) float64, controls []func(T) float64, minPerDate int) (map[string]float64, map[string]int) {
	type bucket struct {
		f []float64
		l []float64
		c [][]float64
	}
	byDate := map[string]*bucket{}
	for _, r := range rows {
		f, l := feat(r), label(r)
		if math.IsNaN(f) || math.IsNaN(l) {
			continue
		}
		cs := make([]float64, len(controls))
		bad := false
		for i, c := range controls {
			cs[i] = c(r)
			if math.IsNaN(cs[i]) {
				bad = true
			}
		}
		if bad {
			continue
		}
		b, ok := byDate[dateOf(r)]
		if !ok {
			b = &bucket{c: make([][]float64, len(controls))}
			byDate[dateOf(r)] = b
		}
		b.f = append(b.f, f)
		b.l = append(b.l, l)
		for i := range cs {
			b.c[i] = append(b.c[i], cs[i])
		}
	}
	out := make(map[string]float64, len(byDate))
	obs := make(map[string]int, len(byDate))
	for d, b := range byDate {
		obs[d] = len(b.f)
		if len(b.f) < minPerDate {
			continue
		}
		if ic := PartialSpearman(b.f, b.l, b.c); !math.IsNaN(ic) {
			out[d] = ic
		}
	}
	return out, obs
}

// ComputeIC is the one-shot form used by callers that only need the aggregate:
// the per-date IC series reduced to a Stats cell whose SumN counts the
// observations that actually entered the reported dates.
func ComputeIC[T any](rows []T, dateOf func(T) string, feat, label func(T) float64, minPerDate int) Stats {
	type bucket struct {
		f []float64
		l []float64
	}
	byDate := map[string]*bucket{}
	for _, r := range rows {
		f, l := feat(r), label(r)
		if math.IsNaN(f) || math.IsNaN(l) {
			continue
		}
		b, ok := byDate[dateOf(r)]
		if !ok {
			b = &bucket{}
			byDate[dateOf(r)] = b
		}
		b.f = append(b.f, f)
		b.l = append(b.l, l)
	}
	var ics []float64
	total := 0
	for _, b := range byDate {
		if len(b.f) < minPerDate {
			continue
		}
		if ic := Spearman(b.f, b.l); !math.IsNaN(ic) {
			ics = append(ics, ic)
			total += len(b.f)
		}
	}
	st := Aggregate(ics)
	st.SumN = total
	return st
}

// ComputePartialIC is the one-shot controlled form: the per-date partial IC of
// feat against label after partialling the ranked controls out of each date's
// cross-section. Rows with a NaN control are dropped, so the effective sample
// can be smaller than for ComputeIC.
func ComputePartialIC[T any](rows []T, dateOf func(T) string, feat, label func(T) float64, controls []func(T) float64, minPerDate int) Stats {
	byDate, obs := PartialICByDate(rows, dateOf, feat, label, controls, minPerDate)
	vals := make([]float64, 0, len(byDate))
	total := 0
	for d, v := range byDate {
		vals = append(vals, v)
		total += obs[d]
	}
	st := Aggregate(vals)
	st.SumN = total
	return st
}

// ComputeIndustryNeutralIC correlates the within-date industry-demeaned ranks of
// feat and label: the cross-sectional relation that survives after each date's
// sector tilt is removed. Rows without an industry label are dropped.
func ComputeIndustryNeutralIC[T any](rows []T, dateOf func(T) string, feat, label func(T) float64, industryOf func(T) string, minPerDate int) Stats {
	type entry struct {
		f, l float64
		ind  string
	}
	byDate := map[string][]entry{}
	for _, r := range rows {
		ind := industryOf(r)
		if ind == "" {
			continue
		}
		f, l := feat(r), label(r)
		if math.IsNaN(f) || math.IsNaN(l) {
			continue
		}
		byDate[dateOf(r)] = append(byDate[dateOf(r)], entry{f: f, l: l, ind: ind})
	}
	var ics []float64
	total := 0
	for _, es := range byDate {
		if len(es) < minPerDate {
			continue
		}
		fs := make([]float64, len(es))
		ls := make([]float64, len(es))
		inds := make([]string, len(es))
		for i, e := range es {
			fs[i], ls[i], inds[i] = e.f, e.l, e.ind
		}
		rf := demeanByGroup(Rank(fs), inds)
		rl := demeanByGroup(Rank(ls), inds)
		if ic := Pearson(rf, rl); !math.IsNaN(ic) {
			ics = append(ics, ic)
			total += len(es)
		}
	}
	st := Aggregate(ics)
	st.SumN = total
	return st
}

// demeanByGroup subtracts each group's mean from the values, leaving
// between-group differences out of the correlation.
func demeanByGroup(vals []float64, groups []string) []float64 {
	sum := map[string]float64{}
	cnt := map[string]int{}
	for i, g := range groups {
		sum[g] += vals[i]
		cnt[g]++
	}
	out := make([]float64, len(vals))
	for i, g := range groups {
		out[i] = vals[i] - sum[g]/float64(cnt[g])
	}
	return out
}
