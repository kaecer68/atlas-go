package strategy_ranker

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

const (
	tradingDaysTWSE = 243 // 台灣加權指數年交易日數（用於年化換算）
)

// Validator 計算策略歷史回測的完整績效指標。
type Validator struct {
	cfg ValidationConfig
}

// NewValidator 建立使用預設配置的策略驗證器。
func NewValidator() *Validator {
	return &Validator{cfg: DefaultValidationConfig()}
}

// NewValidatorWithConfig 建立使用自訂配置的策略驗證器。
func NewValidatorWithConfig(cfg ValidationConfig) *Validator {
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 10
	}
	return &Validator{cfg: cfg}
}

// Validate 對一組策略每日報酬與基準報酬計算完整績效指標。
//
// dailyReturns 為策略的每日報酬序列（已扣費用的淨報酬），taiexReturns 為
// 同期的加權指數每日報酬序列（長度必須相等）。
//
// 回傳的報告中 TaiexCorrelation 反映策略收益與市場 β 的相關性：
//   - 接近 1.0 → 策略主要是 Beta（跟隨大盤）
//   - 接近 0.0 → 策略有獨立 Alpha（相對獨立於大盤）
//
// 若輸入序列長度為 0 或 taiexReturns 長度與 dailyReturns 不一致，回傳 nil。
func (v *Validator) Validate(strategyID, strategyName string, dailyReturns, taiexReturns []float64) *StrategyReport {
	n := len(dailyReturns)
	if n == 0 || len(taiexReturns) != n {
		return nil
	}

	report := &StrategyReport{
		StrategyID:   strategyID,
		StrategyName: strategyName,
		SampleDays:   n,
	}

	report.TotalReturn = totalReturnPct(dailyReturns)
	report.AnnualizedReturn = annualizeReturn(dailyReturns)
	report.MaxDrawdown = maxDrawdownPct(dailyReturns)
	report.WinRate = winRate(dailyReturns)
	report.TaiexCorrelation = pearsonCorrelation(dailyReturns, taiexReturns)
	report.AlphaScore = report.AnnualizedReturn - annualizeReturn(taiexReturns)

	if n >= v.cfg.MinSamples {
		report.SharpeRatio = shared.ComputeSharpe(dailyReturns, shared.SharpeConfig{
			Frequency:    shared.FrequencyTWSE,
			RiskFreeRate: v.cfg.RiskFreeRate / float64(tradingDaysTWSE),
			MinSamples:   v.cfg.MinSamples,
		})
	}

	return report
}

// totalReturnPct 計算序列的累積報酬百分比。
func totalReturnPct(returns []float64) float64 {
	cumulative := 1.0
	for _, r := range returns {
		cumulative *= (1 + r)
	}
	return (cumulative - 1) * 100
}

// annualizeReturn 將每日報酬年化為百分比。
func annualizeReturn(dailyReturns []float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}
	cumulative := 1.0
	for _, r := range dailyReturns {
		cumulative *= (1 + r)
	}
	years := float64(len(dailyReturns)) / float64(tradingDaysTWSE)
	if years <= 0 {
		return 0
	}
	return (math.Pow(cumulative, 1.0/years) - 1) * 100
}

func maxDrawdownPct(dailyReturns []float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}
	equity := 1.0
	peak := 1.0
	var maxDD float64
	for _, r := range dailyReturns {
		equity *= (1 + r)
		if equity > peak {
			peak = equity
		}
		dd := (peak - equity) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD * 100
}

// winRate 計算正報酬日的比例。
func winRate(dailyReturns []float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}
	wins := 0
	for _, r := range dailyReturns {
		if r > 0 {
			wins++
		}
	}
	return float64(wins) / float64(len(dailyReturns))
}

// pearsonCorrelation 計算兩個等長序列的 Pearson 相關係數。
func pearsonCorrelation(x, y []float64) float64 {
	n := len(x)
	if n == 0 || len(y) != n {
		return 0
	}
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}
	numerator := float64(n)*sumXY - sumX*sumY
	denomX := float64(n)*sumX2 - sumX*sumX
	denomY := float64(n)*sumY2 - sumY*sumY
	if denomX <= 0 || denomY <= 0 {
		return 0
	}
	return numerator / math.Sqrt(denomX*denomY)
}

// Rank 對一組策略報告進行綜合排名，回傳附帶排名資訊的報告。
//
// 綜合分數權重：Sharpe*0.35 + AlphaScore 正規化*0.25 + WinRate*0.20 − MaxDrawdown 正規化*0.20。
// 排名 1 為最佳。
//
// 若 reports 為空，回傳空 slice。
func Rank(reports []*StrategyReport) []RankedReport {
	if len(reports) == 0 {
		return nil
	}

	// 正規化各指標到 0~1 範圍
	maxAbsAlpha := 1.0
	maxDD := 1.0
	for _, r := range reports {
		if abs(r.AlphaScore) > maxAbsAlpha {
			maxAbsAlpha = abs(r.AlphaScore)
		}
		if r.MaxDrawdown > maxDD {
			maxDD = r.MaxDrawdown
		}
	}

	ranked := make([]RankedReport, len(reports))
	for i, r := range reports {
		// Sharpe 分數：負值轉正規化
		sharpeScore := math.Max(0, math.Min(1, (r.SharpeRatio+2)/6))
		// Alpha 分數
		alphaScore := math.Max(0, math.Min(1, (r.AlphaScore+maxAbsAlpha)/(2*maxAbsAlpha)))
		// 回撤分數：低回撤高分
		ddScore := 1.0
		if maxDD > 0 {
			ddScore = math.Max(0, 1-(r.MaxDrawdown/maxDD))
		}

		score := sharpeScore*35 + alphaScore*25 + r.WinRate*20 + ddScore*20

		ranked[i] = RankedReport{
			StrategyReport: *r,
			Score:          math.Round(score*100) / 100,
		}
	}

	// 按分數降冪排序，賦予排名
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	return ranked
}

// AssignTiers 根據排名賦予分層標籤。
//
// 前 2 名 → premium（付費深度內容）
// 第 3~4 名 → registered（註冊用戶）
// 其餘 → free（免費公開）
func AssignTiers(ranked []RankedReport) {
	for i := range ranked {
		switch {
		case ranked[i].Rank <= 2:
			ranked[i].Tier = "premium"
		case ranked[i].Rank <= 4:
			ranked[i].Tier = "registered"
		default:
			ranked[i].Tier = "free"
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
