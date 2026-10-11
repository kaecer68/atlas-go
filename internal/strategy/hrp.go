package strategy

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ─── Hierarchical Risk Parity（López de Prado 2016）────────────────────
//
// 與 computeRiskParity（naive 1/σ、無視相關性）並存的 opt-in 路徑：
// 高相關策略只跟同類競爭權重，樣本外更穩。預設 Allocate 不動（= 回退開關）。
//
// 輸入為策略日報酬序列（既有 shadowReturns，無新數據源）。全部純函數，
// 除了 AllocateHRP 的 registry 讀取；確定性（tie-break 永遠取小 index）。

// minHRPOverlap 是相關估計的最小重疊樣本數（house「樣本足」語言：<5 不可信）。
const minHRPOverlap = 5

// CorrelationMatrix 回傳 ids 順序的 Pearson 相關係數矩陣。序列以尾端對齊
// 取 pairwise-complete 重疊（shadow returns 是時序 append，尾端對齊最新）；
// 重疊 <2 或零方差時該對係數為 0（對角線恆為 1）。
func CorrelationMatrix(ids []string, returns map[string][]float64) [][]float64 {
	n := len(ids)
	corr := make([][]float64, n)
	for i := range corr {
		corr[i] = make([]float64, n)
		corr[i][i] = 1
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			c := pairwiseCorrelation(returns[ids[i]], returns[ids[j]])
			corr[i][j] = c
			corr[j][i] = c
		}
	}
	return corr
}

// pairwiseCorrelation 以尾端對齊算 Pearson 相關；重疊 <2 或任一零方差回 0。
func pairwiseCorrelation(a, b []float64) float64 {
	m := min(len(a), len(b))
	if m < 2 {
		return 0
	}
	a = a[len(a)-m:]
	b = b[len(b)-m:]
	var sa, sb float64
	for k := 0; k < m; k++ {
		sa += a[k]
		sb += b[k]
	}
	ma, mb := sa/float64(m), sb/float64(m)
	var cov, va, vb float64
	for k := 0; k < m; k++ {
		da, db := a[k]-ma, b[k]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va < 1e-15 || vb < 1e-15 {
		return 0
	}
	c := cov / math.Sqrt(va*vb)
	return min(max(c, -1), 1)
}

// CorrelationDistance 將相關矩陣轉成距離矩陣 D = √((1-ρ)/2)（LP §3）。
func CorrelationDistance(corr [][]float64) [][]float64 {
	n := len(corr)
	dist := make([][]float64, n)
	for i := range dist {
		dist[i] = make([]float64, n)
		for j := range dist[i] {
			c := corr[i][j]
			if i == j {
				dist[i][j] = 0
				continue
			}
			dist[i][j] = math.Sqrt(max(1-c, 0) / 2)
		}
	}
	return dist
}

// QuasiDiagonalOrder 以 single-linkage 凝聚聚類回傳葉序（quasi-diagonalization）。
// 確定性：距離 tie 永遠取 index 最小的對。
func QuasiDiagonalOrder(dist [][]float64) []int {
	n := len(dist)
	if n <= 2 {
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		return order
	}
	// clusters 保存每群的葉序；d 為群間 single-linkage 距離。
	clusters := make([][]int, n)
	for i := range clusters {
		clusters[i] = []int{i}
	}
	active := make([]bool, n)
	for i := range active {
		active[i] = true
	}
	clusterDist := func(a, b int) float64 {
		best := math.Inf(1)
		for _, x := range clusters[a] {
			for _, y := range clusters[b] {
				if dist[x][y] < best {
					best = dist[x][y]
				}
			}
		}
		return best
	}
	remaining := n
	for remaining > 1 {
		bi, bj := -1, -1
		best := math.Inf(1)
		for i := 0; i < n; i++ {
			if !active[i] {
				continue
			}
			for j := i + 1; j < n; j++ {
				if !active[j] {
					continue
				}
				if d := clusterDist(i, j); d < best {
					best, bi, bj = d, i, j
				}
			}
		}
		clusters[bi] = append(clusters[bi], clusters[bj]...)
		sort.Ints(clusters[bi])
		active[bj] = false
		remaining--
	}
	for i := 0; i < n; i++ {
		if active[i] {
			return clusters[i]
		}
	}
	return nil
}

// clusterVariance 回傳一群的 IVP 權重變異數 w'Σw（w ∝ 1/σ²）。
func clusterVariance(cov [][]float64, members []int) float64 {
	var invSum float64
	inv := make([]float64, len(members))
	for k, m := range members {
		v := cov[m][m]
		if v < 1e-15 {
			inv[k] = 0
			continue
		}
		inv[k] = 1 / v
		invSum += inv[k]
	}
	if invSum < 1e-15 {
		return 0
	}
	var cVar float64
	for k, m := range members {
		for l, n := range members {
			cVar += (inv[k] / invSum) * (inv[l] / invSum) * cov[m][n]
		}
	}
	return cVar
}

// CovarianceMatrix 回傳樣本共變異數矩陣（pairwise 尾端對齊，與相關一致）。
func CovarianceMatrix(ids []string, returns map[string][]float64) [][]float64 {
	n := len(ids)
	cov := make([][]float64, n)
	aligned := make([][]float64, n)
	for i, id := range ids {
		aligned[i] = returns[id]
	}
	for i := range cov {
		cov[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			v := pairwiseCovariance(aligned[i], aligned[j])
			cov[i][j] = v
			cov[j][i] = v
		}
	}
	return cov
}

// pairwiseCovariance 以尾端對齊算樣本共變異數（n-1）；重疊 <2 回 0。
func pairwiseCovariance(a, b []float64) float64 {
	m := min(len(a), len(b))
	if m < 2 {
		return 0
	}
	a = a[len(a)-m:]
	b = b[len(b)-m:]
	var sa, sb float64
	for k := 0; k < m; k++ {
		sa += a[k]
		sb += b[k]
	}
	ma, mb := sa/float64(m), sb/float64(m)
	var cov float64
	for k := 0; k < m; k++ {
		cov += (a[k] - ma) * (b[k] - mb)
	}
	return cov / float64(m-1)
}

// HRPWeights 回傳 HRP 權重（Σw=1，無 caps；caps 由呼叫端套用）。
// 空輸入回空 map；單一回 100%；全零方差回等權（無資訊可分）。
func HRPWeights(ids []string, returns map[string][]float64) map[string]float64 {
	out := make(map[string]float64, len(ids))
	if len(ids) == 0 {
		return out
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	if len(sorted) == 1 {
		out[sorted[0]] = 1.0
		return out
	}
	cov := CovarianceMatrix(sorted, returns)
	var totalVar float64
	for i := range sorted {
		totalVar += cov[i][i]
	}
	if totalVar < 1e-15 {
		w := 1.0 / float64(len(sorted))
		for _, id := range sorted {
			out[id] = w
		}
		return out
	}
	order := QuasiDiagonalOrder(CorrelationDistance(CorrelationMatrix(sorted, returns)))
	weights := make(map[int]float64, len(sorted))
	for _, o := range order {
		weights[o] = 1.0
	}
	bisect(order, cov, weights)
	for i, id := range sorted {
		out[id] = weights[i]
	}
	return out
}

// bisect 遞迴二分：按群變異數反比拆權重（LP §4）。
func bisect(order []int, cov [][]float64, weights map[int]float64) {
	if len(order) < 2 {
		return
	}
	mid := len(order) / 2
	left, right := order[:mid], order[mid:]
	vLeft := clusterVariance(cov, left)
	vRight := clusterVariance(cov, right)
	denom := vLeft + vRight
	var alphaLeft float64
	if denom < 1e-15 {
		alphaLeft = 0.5
	} else {
		alphaLeft = 1 - vLeft/denom
	}
	for _, m := range left {
		weights[m] *= alphaLeft
	}
	for _, m := range right {
		weights[m] *= 1 - alphaLeft
	}
	bisect(left, cov, weights)
	bisect(right, cov, weights)
}

// riskParityLocked 是不取鎖的 risk-parity 回退（呼叫端須已持有讀鎖）。
func (sa *StrategyAllocator) riskParityLocked(candidates []*Strategy) StrategyMix {
	vols := make(map[string]float64, len(candidates))
	for _, c := range candidates {
		vols[c.ID] = sa.estimateVolatility(c.ID)
	}
	return sa.computeRiskParity(candidates, vols)
}

// AllocateHRP 以 HRP 計權重的 opt-in 分配路徑：同 regime 過濾、同 caps、
// 同 fallback（候選 <2 或任一序列 <minHRPOverlap 筆時回退 risk parity）。
// 預設 Allocate 一字不動（= 回退開關）。
func (sa *StrategyAllocator) AllocateHRP(regime domain.Regime) StrategyMix {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	candidates := sa.registry.ListByRegime(regime)
	if len(candidates) == 0 {
		all := sa.registry.List()
		if len(all) == 0 {
			return StrategyMix{}
		}
		candidates = all
	}
	if len(candidates) < 2 {
		return sa.riskParityLocked(candidates)
	}
	returns := make(map[string][]float64, len(candidates))
	for _, s := range candidates {
		r := sa.shadowReturns[s.ID]
		if len(r) < minHRPOverlap {
			return sa.riskParityLocked(candidates)
		}
		returns[s.ID] = r
	}
	ids := make([]string, 0, len(candidates))
	for _, s := range candidates {
		ids = append(ids, s.ID)
	}
	raw := HRPWeights(ids, returns)
	mix := make(StrategyMix, len(ids))
	for _, id := range ids {
		mix[id] = raw[id]
	}
	return sa.applyCaps(mix, candidates)
}
