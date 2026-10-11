package experiment

import (
	"math"
	"testing"
)

// TestInverseNormalCDF 以已知分位數驗證 Acklam 近似。
func TestInverseNormalCDF(t *testing.T) {
	if got := InverseNormalCDF(0.5); got != 0 {
		t.Fatalf("InverseNormalCDF(0.5) = %v, want 0", got)
	}
	if got := InverseNormalCDF(0.975); math.Abs(got-1.959963984540054) > 1e-6 {
		t.Fatalf("InverseNormalCDF(0.975) = %v, want 1.959964", got)
	}
	if got := InverseNormalCDF(0.025); math.Abs(got+1.959963984540054) > 1e-6 {
		t.Fatalf("InverseNormalCDF(0.025) = %v, want -1.959964", got)
	}
	for _, p := range []float64{0, 1, -0.1, 1.1, math.NaN()} {
		if got := InverseNormalCDF(p); !math.IsNaN(got) {
			t.Fatalf("InverseNormalCDF(%v) = %v, want NaN", p, got)
		}
	}
}

// TestSampleMoments_Symmetric 對稱資料偏態為 0；{-1,0,1} 的 Pearson 峰度為 1.5。
func TestSampleMoments_Symmetric(t *testing.T) {
	if got := SampleSkew([]float64{-1, 0, 1}); math.Abs(got) > 1e-12 {
		t.Fatalf("SampleSkew(-1,0,1) = %v, want 0", got)
	}
	if got := SampleKurtosis([]float64{-1, 0, 1}); math.Abs(got-1.5) > 1e-12 {
		t.Fatalf("SampleKurtosis(-1,0,1) = %v, want 1.5", got)
	}
	if got := SampleSkew([]float64{1, 2}); !math.IsNaN(got) {
		t.Fatalf("SampleSkew n<3 = %v, want NaN", got)
	}
	if got := SampleKurtosis([]float64{1, 2, 3}); !math.IsNaN(got) {
		t.Fatalf("SampleKurtosis n<4 = %v, want NaN", got)
	}
}

// TestDeflatedSharpeRatio_PublishedExample 以公開算例為迴歸錨點：
// Bailey-LP 例（經 marti.ai 實作驗證）：SR=2.5（年化）、V=0.5/252、K=100、
// T=1250、skew=-3、kurt=10 → DSR ≈ 0.8997。
func TestDeflatedSharpeRatio_PublishedExample(t *testing.T) {
	got := DeflatedSharpeRatio(2.5/math.Sqrt(252), 0.5/252, 100, 1250, -3, 10)
	if math.Abs(got-0.8997) > 1e-3 {
		t.Fatalf("DeflatedSharpeRatio(published) = %v, want ≈0.8997", got)
	}
}

// TestDeflatedSharpeRatio_EdgeCases 證據不足時回 NaN，呼叫端視為 skip。
func TestDeflatedSharpeRatio_EdgeCases(t *testing.T) {
	if got := DeflatedSharpeRatio(1.5, 0.1, 1, 1250, 0, 3); !math.IsNaN(got) {
		t.Fatalf("K<2 = %v, want NaN (no selection bias to correct)", got)
	}
	if got := DeflatedSharpeRatio(1.5, 0.1, 10, 2, 0, 3); !math.IsNaN(got) {
		t.Fatalf("T<3 = %v, want NaN", got)
	}
	if got := DeflatedSharpeRatio(1.5, -0.1, 10, 1250, 0, 3); !math.IsNaN(got) {
		t.Fatalf("negative variance = %v, want NaN", got)
	}
}

// TestTrialSharpeVariance 樣本變異數；不足 2 點回 ok=false。
func TestTrialSharpeVariance(t *testing.T) {
	v, ok := TrialSharpeVariance([]float64{1, 2, 3})
	if !ok || math.Abs(v-1.0) > 1e-12 {
		t.Fatalf("TrialSharpeVariance(1,2,3) = %v,%v, want 1.0,true", v, ok)
	}
	if _, ok := TrialSharpeVariance([]float64{1.5}); ok {
		t.Fatal("single point must return ok=false")
	}
	if _, ok := TrialSharpeVariance(nil); ok {
		t.Fatal("empty must return ok=false")
	}
}
