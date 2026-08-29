package eval

import "math"

// MismatchResult 儲存監督學習（SL）與強化學習（RL）效能失配偵測的結果。
// 當模型的預測品質良好但無法轉化為交易績效時，表示 SL/RL 失配。
type MismatchResult struct {
	// MismatchDetected 為 true 時表示偵測到失配。
	MismatchDetected bool `json:"mismatch_detected"`
	// PredictionR2 為模型預測的樣外 R²。
	PredictionR2 float64 `json:"prediction_r2"`
	// TradingSharpe 為由預測訊號所產生之年化 Sharpe 比率。
	TradingSharpe float64 `json:"trading_sharpe"`
	// RankCorrelation 為預測排序與實際報酬排序之 Spearman 等級相關係數。
	RankCorrelation float64 `json:"rank_correlation"`
	// Severity 為失配嚴重程度："none"、"moderate"、"severe"。
	Severity string `json:"severity"`
	// Diagnosis 為人類可讀的診斷說明。
	Diagnosis string `json:"diagnosis"`
}

// CheckSLRLAlignment 偵測監督學習預測品質與交易績效之間的失配。
//
// 參數：
//   - yTrue：真實目標值（如實際報酬）
//   - yPred：模型預測值
//   - dailyReturns：根據模型訊號進行交易所產生的每日報酬序列
//
// 當 R² > 0.05（模型預測優於隨機）且 (Sharpe < 0.3 或排序相關 < 0.2) 時判定為失配。
// 嚴重程度："severe" 當 R² > 0.1 且排序相關 < 0.1；"moderate" 當 R² > 0.05 且排序相關 < 0.2。
func CheckSLRLAlignment(yTrue, yPred, dailyReturns []float64) MismatchResult {
	n := len(yTrue)
	if n == 0 || len(yPred) != n || len(dailyReturns) != n {
		return MismatchResult{
			MismatchDetected: false,
			Diagnosis:        "insufficient data",
		}
	}

	r2 := OOSR2(yTrue, yPred)
	sharpe := SharpeRatio(dailyReturns, 0)
	rankCorr := spearmanRankCorrelation(yPred, yTrue)

	result := MismatchResult{
		PredictionR2:    r2,
		TradingSharpe:   sharpe,
		RankCorrelation: rankCorr,
	}

	// 判定嚴重程度
	if r2 > 0.1 && rankCorr < 0.1 {
		result.Severity = "severe"
		result.MismatchDetected = true
		result.Diagnosis = "嚴重失配：模型預測品質良好（R² > 0.1）但排序相關極低（< 0.1），預測排序無法對應實際報酬排序"
	} else if r2 > 0.05 && (sharpe < 0.3 || rankCorr < 0.2) {
		if r2 > 0.05 && rankCorr < 0.2 {
			result.Severity = "moderate"
			result.MismatchDetected = true
			result.Diagnosis = "中度失配：模型預測能力尚可（R² > 0.05）但排序相關偏低（< 0.2），預測與交易績效脫節"
		} else {
			// r2 > 0.05, rankCorr >= 0.2, but sharpe < 0.3
			result.Severity = "moderate"
			result.MismatchDetected = true
			result.Diagnosis = "中度失配：模型預測能力尚可（R² > 0.05）但交易 Sharpe 偏低（< 0.3），良好預測未轉化為交易利潤"
		}
	} else {
		result.Severity = "none"
		result.MismatchDetected = false
		if r2 <= 0.05 {
			result.Diagnosis = "無失配：模型預測能力不足（R² ≤ 0.05），問題在於模型本身而非 SL/RL 失配"
		} else {
			result.Diagnosis = "無失配：預測品質與交易績效一致"
		}
	}

	return result
}

// spearmanRankCorrelation 計算兩組數值之間的 Spearman 等級相關係數。
// 使用 Pearson 相關係數作用於等級（rank）上來近似 Spearman 相關。
// 回傳 0 表示輸入長度不足或為空。
func spearmanRankCorrelation(x, y []float64) float64 {
	n := len(x)
	if n < 2 || len(y) != n {
		return 0
	}

	rankX := computeRanks(x)
	rankY := computeRanks(y)

	// 計算等級的 Pearson 相關係數
	var sumX, sumY float64
	for i := range n {
		sumX += rankX[i]
		sumY += rankY[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	var covXY, varX, varY float64
	for i := range n {
		dx := rankX[i] - meanX
		dy := rankY[i] - meanY
		covXY += dx * dy
		varX += dx * dx
		varY += dy * dy
	}

	if varX < 1e-15 || varY < 1e-15 {
		return 0
	}

	return covXY / math.Sqrt(varX*varY)
}

// computeRanks 將數值轉換為等級（1-based），平手（tie）時使用平均等級。
func computeRanks(values []float64) []float64 {
	n := len(values)
	type indexed struct {
		val   float64
		index int
	}

	sorted := make([]indexed, n)
	for i, v := range values {
		sorted[i] = indexed{v, i}
	}

	// 簡單插入排序，適合小型資料集
	for i := 1; i < n; i++ {
		for j := i; j > 0 && sorted[j].val < sorted[j-1].val; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	ranks := make([]float64, n)
	i := 0
	for i < n {
		j := i
		for j < n && math.Abs(sorted[j].val-sorted[i].val) < 1e-15 {
			j++
		}
		// sorted[i:j] 為平手組，使用平均等級（1-based）
		avgRank := float64(i+j+1) / 2.0
		for k := i; k < j; k++ {
			ranks[sorted[k].index] = avgRank
		}
		i = j
	}

	return ranks
}
