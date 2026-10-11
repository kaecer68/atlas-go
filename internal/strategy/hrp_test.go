package strategy

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// altSeries 建確定性交替序列：step 為符號翻轉週期（1=逐筆，2=兩筆一翻…）。
func altSeries(n int, amp float64, step int) []float64 {
	out := make([]float64, n)
	for i := range out {
		sign := 1.0
		if (i/step)%2 == 1 {
			sign = -1.0
		}
		out[i] = amp * sign
	}
	return out
}

// TestHRPWeights_UncorrelatedEqualsInverseVariance 對角協方差下 HRP = 逆變異數加權。
// A σ≈0.01、B σ≈0.02、C σ≈0.03 且兩兩零相關 → (0.7347, 0.1837, 0.0816)。
func TestHRPWeights_UncorrelatedEqualsInverseVariance(t *testing.T) {
	ids := []string{"A", "B", "C"}
	returns := map[string][]float64{
		"A": altSeries(60, 0.01, 1),
		"B": altSeries(60, 0.02, 2),
		"C": altSeries(60, 0.03, 3),
	}
	got := HRPWeights(ids, returns)
	want := map[string]float64{"A": 0.7347, "B": 0.1837, "C": 0.0816}
	var total float64
	for _, id := range ids {
		total += got[id]
		if math.Abs(got[id]-want[id]) > 0.01 {
			t.Errorf("HRP %s = %.4f, want ≈%.4f (inverse-variance)", id, got[id], want[id])
		}
	}
	if math.Abs(total-1.0) > 1e-9 {
		t.Errorf("Σw = %v, want 1", total)
	}
}

// TestQuasiDiagonalOrder_GroupsIdenticalPair 全同對距離為 0、最先合併、
// 在葉序中相鄰；權重合法（Σ=1、全正）。注意 HRP 不保證奇數群下 twins 等權
// （三元二分按位置對半），只保證相鄰同群。
func TestQuasiDiagonalOrder_GroupsIdenticalPair(t *testing.T) {
	shared := altSeries(60, 0.01, 1)
	ids := []string{"A", "B", "C"}
	returns := map[string][]float64{"A": shared, "B": shared, "C": altSeries(60, 0.03, 3)}
	order := QuasiDiagonalOrder(CorrelationDistance(CorrelationMatrix(ids, returns)))
	pos := map[int]int{}
	for p, o := range order {
		pos[o] = p
	}
	if math.Abs(float64(pos[0]-pos[1])) != 1 {
		t.Fatalf("identical pair not adjacent in quasi-diagonal order %v", order)
	}
	got := HRPWeights(ids, returns)
	var total float64
	for _, id := range ids {
		total += got[id]
		if got[id] <= 0 {
			t.Fatalf("weight %s = %v, want positive: %v", id, got[id], got)
		}
	}
	if math.Abs(total-1.0) > 1e-9 {
		t.Fatalf("Σw = %v, want 1", total)
	}
}

// TestHRPWeights_TwoIdenticalSplitEvenly 兩全同資產各半。
func TestHRPWeights_TwoIdenticalSplitEvenly(t *testing.T) {
	s := altSeries(60, 0.01, 1)
	got := HRPWeights([]string{"A", "B"}, map[string][]float64{"A": s, "B": s})
	if got["A"] != 0.5 || got["B"] != 0.5 {
		t.Fatalf("two identical = %v, want {A:0.5 B:0.5}", got)
	}
}

// TestHRPWeights_EdgeCases 空→空；單一→100%。
func TestHRPWeights_EdgeCases(t *testing.T) {
	if got := HRPWeights(nil, nil); len(got) != 0 {
		t.Fatalf("empty = %v, want empty", got)
	}
	got := HRPWeights([]string{"A"}, map[string][]float64{"A": altSeries(60, 0.01, 1)})
	if len(got) != 1 || got["A"] != 1.0 {
		t.Fatalf("single = %v, want {A:1}", got)
	}
}

// TestAllocateHRP_CapsAndValidate opt-in 路徑守同樣 caps 並通過 Validate。
func TestAllocateHRP_CapsAndValidate(t *testing.T) {
	reg := syntheticRegistry([]*Strategy{
		makeTestStrategy("A", domain.RegimeRiskOn),
		makeTestStrategy("B", domain.RegimeRiskOn),
		makeTestStrategy("C", domain.RegimeRiskOn),
	})
	sa := NewStrategyAllocator(reg)
	series := map[string][]float64{
		"A": altSeries(60, 0.01, 1),
		"B": altSeries(60, 0.02, 2),
		"C": altSeries(60, 0.03, 3),
	}
	for d := 0; d < 60; d++ {
		sa.UpdateShadowReturns(map[string]float64{
			"A": series["A"][d], "B": series["B"][d], "C": series["C"][d],
		})
	}
	mix := sa.AllocateHRP(domain.RegimeRiskOn)
	if !mix.Validate() {
		t.Fatalf("HRP mix failed validation: %v", mix)
	}
	for id, w := range mix {
		if w > 0.50+1e-9 || w < 0.05-1e-9 {
			t.Fatalf("weight %s=%.4f violates caps [0.05, 0.50]", id, w)
		}
	}
}

// TestAllocateHRP_FallbackSingleStrategy 單候選時與 Allocate 一致（= {id:1}）。
func TestAllocateHRP_FallbackSingleStrategy(t *testing.T) {
	reg := syntheticRegistry([]*Strategy{makeTestStrategy("solo", domain.RegimeRiskOn)})
	sa := NewStrategyAllocator(reg)
	hrp := sa.AllocateHRP(domain.RegimeRiskOn)
	rp := sa.Allocate(domain.RegimeRiskOn, 20)
	if len(hrp) != 1 || hrp["solo"] != 1.0 {
		t.Fatalf("single-strategy HRP = %v, want {solo:1}", hrp)
	}
	if len(rp) != 1 || rp["solo"] != 1.0 {
		t.Fatalf("single-strategy RP = %v, want {solo:1}", rp)
	}
}
