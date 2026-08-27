package stockpicker

import (
	"fmt"
	"math"
)

// CalibrationStatus 校準狀態。
type CalibrationStatus string

const (
	// CalibrationCalibrating 樣本不足，僅供觀察，不得驅動資金權重。
	CalibrationCalibrating CalibrationStatus = "calibrating"
	// CalibrationEligible 樣本充足，可參考。
	CalibrationEligible CalibrationStatus = "eligible"
)

// SignalOutcome 單一訊號結果。
type SignalOutcome struct {
	Symbol        string
	TriggerDate   string // YYYY-MM-DD
	ForwardReturn float64
	Hit           bool   // 已扣成本後是否命中；由呼叫端依 NetHit 預先計算，供顯示/溯源用
	Source        string // e.g. "stockpicker-momentum" or agent id
}

// SignalWinRateSummary 勝率摘要。
type SignalWinRateSummary struct {
	Symbol            string
	Source            string
	Observations      int
	Hits              int
	WinRate           float64
	WilsonLower       float64
	WilsonUpper       float64
	Confidence        float64
	CalibrationStatus CalibrationStatus
	NetCostRate       float64
	AvgForwardReturn  float64
}

// WinRate 樣本勝率 = hits / observations。
// 觀察數為 0 或輸入無效（hits < 0、observations < 0、hits > observations）時回傳 0。
func WinRate(hits, observations int) float64 {
	if observations <= 0 || hits < 0 || hits > observations {
		return 0
	}
	return float64(hits) / float64(observations)
}

// zScoreForConfidence 回傳信賴水準對應的常態 z 值。
// 僅支援 0.95 與 0.99；其他值文件化回退為 0.95。
func zScoreForConfidence(confidence float64) float64 {
	if confidence == 0.99 {
		return 2.5758293035489004
	}
	return 1.959963984540054 // 0.95
}

// WilsonScoreInterval 計算 Wilson score interval（lower, upper）。
// 觀察數為 0 或輸入無效（hits < 0、observations < 0、hits > observations）時回傳 (0, 0)。
// confidence 僅支援 0.95 與 0.99，其他值回退為 0.95。
func WilsonScoreInterval(hits, observations int, confidence float64) (lower, upper float64) {
	if observations <= 0 || hits < 0 || hits > observations {
		return 0, 0
	}
	z := zScoreForConfidence(confidence)
	z2 := z * z
	n := float64(observations)
	p := float64(hits) / n

	denom := 1 + z2/n
	center := (p + z2/(2*n)) / denom
	margin := z * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / denom
	return math.Max(0, center-margin), math.Min(1, center+margin)
}

// CalibrationStatusFor 依觀察數判定校準狀態：observations < minSamples 為 calibrating。
// minSamples <= 0 視為無門檻，任何觀察數皆為 eligible。
func CalibrationStatusFor(observations, minSamples int) CalibrationStatus {
	if observations < minSamples {
		return CalibrationCalibrating
	}
	return CalibrationEligible
}

// NetHit 淨報酬命中判定 = forwardReturn - costRate > 0（嚴格大於，打平不算命中）。
// costRate 為來回交易成本率（如台股 0.585% 傳 0.00585），假設非負；
// 負 costRate 等同加乘報酬，呼叫端應避免。
func NetHit(forwardReturn, costRate float64) bool {
	return forwardReturn-costRate > 0
}

// SignalWinRate 給定 outcome 切片與成本率計算勝率摘要。
//
// 命中數以注入的 costRate 重新計算（NetHit(ForwardReturn, costRate)），
// 確保勝率與成本假設一致（P0-3：hit = 淨報酬 > 0）；outcome.Hit 欄位
// 不參與聚合，僅供呼叫端顯示/溯源。所有 outcomes 的 Symbol 與 Source
// 必須一致，否則回傳錯誤。空切片回傳 0 observations 且無 error。
func SignalWinRate(outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64) (SignalWinRateSummary, error) {
	var summary SignalWinRateSummary
	if len(outcomes) == 0 {
		summary.Confidence = confidence
		summary.CalibrationStatus = CalibrationStatusFor(0, minSamples)
		return summary, nil
	}

	symbol := outcomes[0].Symbol
	source := outcomes[0].Source
	var totalReturn float64
	for _, o := range outcomes {
		if o.Symbol != symbol {
			return SignalWinRateSummary{}, fmt.Errorf("stockpicker: mixed symbols in SignalWinRate: %q and %q", symbol, o.Symbol)
		}
		if o.Source != source {
			return SignalWinRateSummary{}, fmt.Errorf("stockpicker: mixed sources in SignalWinRate: %q and %q", source, o.Source)
		}
		if NetHit(o.ForwardReturn, costRate) {
			summary.Hits++
		}
		totalReturn += o.ForwardReturn
	}

	n := len(outcomes)
	summary.Symbol = symbol
	summary.Source = source
	summary.Observations = n
	summary.WinRate = WinRate(summary.Hits, n)
	summary.WilsonLower, summary.WilsonUpper = WilsonScoreInterval(summary.Hits, n, confidence)
	summary.Confidence = confidence
	summary.CalibrationStatus = CalibrationStatusFor(n, minSamples)
	summary.NetCostRate = costRate
	summary.AvgForwardReturn = totalReturn / float64(n)
	return summary, nil
}

// StockWinRate 股票層級勝率摘要（跨訊號來源聚合的語意，便於未來擴展）。
// 數學與 SignalWinRate 共用；Source 欄位語意為「股票層級」。
func StockWinRate(outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64) (SignalWinRateSummary, error) {
	return SignalWinRate(outcomes, costRate, minSamples, confidence)
}

// StrategyWinRate 策略層級勝率摘要（跨股票聚合的語意，便於未來擴展）。
// 數學與 SignalWinRate 共用；Source 欄位語意為「策略層級」。
func StrategyWinRate(outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64) (SignalWinRateSummary, error) {
	return SignalWinRate(outcomes, costRate, minSamples, confidence)
}
