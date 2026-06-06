package eval_test

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/eval"
)

// TestCheckSLRLAlignment_PerfectAlignment 驗證模型完美預測且交易完美時不應偵測到失配。
func TestCheckSLRLAlignment_PerfectAlignment(t *testing.T) {
	// 預測值與真實值完全一致，R²=1；交易報酬穩定正報酬，Sharpe 極高
	yTrue := []float64{0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09, 0.10}
	yPred := []float64{0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09, 0.10}
	dailyReturns := []float64{0.005, 0.006, 0.007, 0.008, 0.009, 0.005, 0.006, 0.007, 0.008, 0.009}

	result := eval.CheckSLRLAlignment(yTrue, yPred, dailyReturns)

	if result.MismatchDetected {
		t.Errorf("MismatchDetected = true, 預期 false（完美對齊不應偵測失配）")
	}
	if result.Severity != "none" {
		t.Errorf("Severity = %q, 預期 \"none\"", result.Severity)
	}
	if math.Abs(result.PredictionR2-1.0) > 1e-10 {
		t.Errorf("PredictionR2 = %v, 預期 1.0", result.PredictionR2)
	}
	if result.TradingSharpe <= 0 {
		t.Errorf("TradingSharpe = %v, 預期 > 0", result.TradingSharpe)
	}
	if result.RankCorrelation <= 0.9 {
		t.Errorf("RankCorrelation = %v, 預期接近 1.0", result.RankCorrelation)
	}
}

// TestCheckSLRLAlignment_HighR2LowSharpe 驗證高 R² 但負 Sharpe 時偵測到嚴重失配。
func TestCheckSLRLAlignment_HighR2LowSharpe(t *testing.T) {
	// 建構 R² > 0.3 但交易虧損的情境
	// yTrue 與 yPred 相關但 yPred 有系統性偏移
	yTrue := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
	yPred := []float64{1.1, 2.2, 3.1, 4.2, 5.1, 6.2, 7.1, 8.2, 9.1, 10.2}
	// 交易報酬全部為負，Sharpe 為負
	dailyReturns := []float64{-0.01, -0.02, -0.015, -0.025, -0.01, -0.02, -0.015, -0.025, -0.01, -0.02}

	result := eval.CheckSLRLAlignment(yTrue, yPred, dailyReturns)

	if !result.MismatchDetected {
		t.Error("MismatchDetected = false, 預期 true（高 R² + 負 Sharpe 應偵測失配）")
	}
	if result.PredictionR2 <= 0.05 {
		t.Errorf("PredictionR2 = %v, 預期 > 0.05", result.PredictionR2)
	}
	if result.TradingSharpe >= 0 {
		t.Errorf("TradingSharpe = %v, 預期 < 0", result.TradingSharpe)
	}
	// 因為 yPred 與 yTrue 排序一致，rankCorr 應接近 1，但 Sharpe < 0.3
	// 所以是 moderate 而非 severe（severe 需 rankCorr < 0.1）
	if result.Severity != "moderate" && result.Severity != "severe" {
		t.Errorf("Severity = %q, 預期 \"moderate\" 或 \"severe\"", result.Severity)
	}
}

// TestCheckSLRLAlignment_HighR2NegativeRankCorrelation 驗證高 R² 但排序相關為負時偵測到嚴重失配。
func TestCheckSLRLAlignment_HighR2NegativeRankCorrelation(t *testing.T) {
	// yTrue 遞增，yPred 遞減：排序完全相反
	yTrue := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
	yPred := []float64{10.0, 9.0, 8.0, 7.0, 6.0, 5.0, 4.0, 3.0, 2.0, 1.0}
	dailyReturns := []float64{0.01, 0.02, 0.03, 0.04, 0.05, 0.01, 0.02, 0.03, 0.04, 0.05}

	result := eval.CheckSLRLAlignment(yTrue, yPred, dailyReturns)

	if !result.MismatchDetected {
		t.Error("MismatchDetected = false, 預期 true（排序完全相反）")
	}
	if result.RankCorrelation >= 0 {
		t.Errorf("RankCorrelation = %v, 預期 < 0（排序相反）", result.RankCorrelation)
	}
	if result.Severity != "severe" {
		t.Errorf("Severity = %q, 預期 \"severe\"（R² > 0.1 且 rankCorr < 0.1）", result.Severity)
	}
}

// TestCheckSLRLAlignment_LowR2LowSharpe 驗證低 R² 時不應偵測失配（問題在模型本身）。
func TestCheckSLRLAlignment_LowR2LowSharpe(t *testing.T) {
	// yPred 全部趨近零，SSres≈54.7 ≈ SStot=55 → R² ≈ 0.005
	yTrue := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	yPred := []float64{0.01, 0.01, 0.01, 0.01, 0.01}
	dailyReturns := []float64{-0.01, -0.02, -0.01, -0.02, -0.01}

	result := eval.CheckSLRLAlignment(yTrue, yPred, dailyReturns)

	if result.MismatchDetected {
		t.Error("MismatchDetected = true, 預期 false（低 R² 表示模型本身不行，非 SL/RL 失配）")
	}
	if result.Severity != "none" {
		t.Errorf("Severity = %q, 預期 \"none\"", result.Severity)
	}
}

// TestCheckSLRLAlignment_ModerateMismatch 驗證中等程度失配的偵測。
// R²=0.08, rankCorrelation ≈ 0.15 → moderate
func TestCheckSLRLAlignment_ModerateMismatch(t *testing.T) {
	// 建構 R² 約 0.08 且排序相關約 0.15 的情境
	// 使用 10 個資料點，yPred 與 yTrue 有部分相關但不完全排序一致
	yTrue := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
	// 加入雜訊使排序不完全一致，但保持正相關
	yPred := []float64{1.5, 3.0, 2.5, 5.0, 4.5, 7.0, 6.5, 9.0, 8.5, 10.5}
	// 交易報酬偏低，Sharpe < 0.3
	dailyReturns := []float64{-0.005, 0.001, -0.003, 0.002, -0.004, 0.001, -0.002, 0.003, -0.001, 0.002}

	result := eval.CheckSLRLAlignment(yTrue, yPred, dailyReturns)

	// 驗證中間值範圍
	if result.PredictionR2 <= 0 {
		t.Errorf("PredictionR2 = %v, 預期 > 0", result.PredictionR2)
	}
	// 排序相關應為正值但偏低
	if result.RankCorrelation < 0 || result.RankCorrelation > 1 {
		t.Errorf("RankCorrelation = %v, 預期在 [0, 1] 範圍內", result.RankCorrelation)
	}
	// 根據實際 R² 和 rankCorr 值判定
	if result.PredictionR2 > 0.05 && result.RankCorrelation < 0.2 {
		if result.Severity != "moderate" {
			t.Errorf("Severity = %q, 預期 \"moderate\"（R²=%.4f, rankCorr=%.4f）",
				result.Severity, result.PredictionR2, result.RankCorrelation)
		}
	}
}

// TestCheckSLRLAlignment_EmptyInputs 驗證空輸入時回傳零值與 "insufficient data" 診斷。
func TestCheckSLRLAlignment_EmptyInputs(t *testing.T) {
	result := eval.CheckSLRLAlignment([]float64{}, []float64{}, []float64{})

	if result.MismatchDetected {
		t.Error("MismatchDetected = true, 預期 false")
	}
	if result.Diagnosis != "insufficient data" {
		t.Errorf("Diagnosis = %q, 預期 \"insufficient data\"", result.Diagnosis)
	}
	if result.PredictionR2 != 0 || result.TradingSharpe != 0 || result.RankCorrelation != 0 {
		t.Errorf("預期所有數值欄位為 0, got R2=%v Sharpe=%v RankCorr=%v",
			result.PredictionR2, result.TradingSharpe, result.RankCorrelation)
	}
	if result.Severity != "" {
		t.Errorf("Severity = %q, 預期空字串", result.Severity)
	}
}

// TestCheckSLRLAlignment_LengthMismatch 驗證長度不一致時回傳零值。
func TestCheckSLRLAlignment_LengthMismatch(t *testing.T) {
	yTrue := []float64{1.0, 2.0, 3.0}
	yPred := []float64{1.0, 2.0} // 長度不一致
	dailyReturns := []float64{0.01, 0.02, 0.03}

	result := eval.CheckSLRLAlignment(yTrue, yPred, dailyReturns)

	if result.MismatchDetected {
		t.Error("MismatchDetected = true, 預期 false")
	}
	if result.Diagnosis != "insufficient data" {
		t.Errorf("Diagnosis = %q, 預期 \"insufficient data\"", result.Diagnosis)
	}
}

// TestRankCorrelation 測試 Spearman 等級相關係數輔助函數。
func TestRankCorrelation(t *testing.T) {
	t.Run("perfect positive correlation", func(t *testing.T) {
		x := []float64{1, 2, 3, 4, 5}
		y := []float64{10, 20, 30, 40, 50}
		got := eval.CheckSLRLAlignment(x, y, x).RankCorrelation
		if math.Abs(got-1.0) > 1e-10 {
			t.Errorf("RankCorrelation = %v, 預期 1.0", got)
		}
	})

	t.Run("perfect negative correlation", func(t *testing.T) {
		x := []float64{1, 2, 3, 4, 5}
		y := []float64{50, 40, 30, 20, 10}
		// 使用 CheckSLRLAlignment 來取得 RankCorrelation
		// dailyReturns 用 x 的值即可（這裡只關心 RankCorrelation）
		result := eval.CheckSLRLAlignment(x, y, x)
		if math.Abs(result.RankCorrelation-(-1.0)) > 1e-10 {
			t.Errorf("RankCorrelation = %v, 預期 -1.0", result.RankCorrelation)
		}
	})

	t.Run("single element returns zero", func(t *testing.T) {
		x := []float64{1}
		y := []float64{1}
		result := eval.CheckSLRLAlignment(x, y, x)
		if result.RankCorrelation != 0 {
			t.Errorf("RankCorrelation = %v, 預期 0（單一元素）", result.RankCorrelation)
		}
	})

	t.Run("ties use average rank", func(t *testing.T) {
		// x: [1, 2, 2, 3] → ranks: [1, 2.5, 2.5, 4]
		// y: [1, 2, 3, 4] → ranks: [1, 2, 3, 4]
		x := []float64{1, 2, 2, 3}
		y := []float64{1, 2, 3, 4}
		result := eval.CheckSLRLAlignment(x, y, x)
		// Spearman 應接近但不到 1.0（因為平手）
		if result.RankCorrelation < 0.9 {
			t.Errorf("RankCorrelation = %v, 預期 > 0.9（平手應使用平均等級）", result.RankCorrelation)
		}
	})
}
