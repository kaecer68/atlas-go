package calibration

import "fmt"

// 因子標準化縮放係數 — 將原始數據映射到 0-100 區間
const (
	StressScaleDXY          = 5.0          // DXY 變化率 (%) → 壓力分數：每 1% 變化 = 5 分，20% 達上限
	StressScaleUS10Y        = 2.0          // 美債殖利率絕對值 → 壓力分數：每 1% = 2 分，50% 達上限
	StressScaleForeignFlow  = 10.0         // 外資淨賣超（億）→ 壓力分數：每 1 億 = 10 分，10 億達上限
	StressScaleVIX          = 100.0 / 40.0 // VIX 原始值 → 壓力分數：VIX=30 → 75 分，VIX=40 → 100 分
	StressScaleJPY          = 10.0         // 日圓變化率 (%) → 壓力分數：每 1% = 10 分，10% 達上限
	StressScaleGeopolitical = 1.0          // 地緣風險強度直接使用（已為 0-100）
	StressScaleOil          = 2.0          // 原油變化率 (%) → 每 $10 變化 = 2 分，$500 達上限
	StressScaleGold         = 2.0          // 黃金變化率 (%) → 每 $50 變化 = 2 分，$2500 達上限

	// 六因子權重 — 總和必須為 1.00
	StressWeightDXY          = 0.13 // DXY 美元指數：美元走強 → 資金回流美國 → 台股賣壓
	StressWeightUS10Y        = 0.18 // US10Y 美債殖利率：利率上升 → 資金流向美債 → 外資撤離
	StressWeightForeignFlow  = 0.22 // 外商淨流向：最直接的壓力指標，權重最高
	StressWeightVIX          = 0.13 // VIX 恐慌指數：全球避險情緒 → 新興市場資金流出
	StressWeightJPY          = 0.08 // 日圓套利平倉：間接影響，透過新興市場情緒傳導
	StressWeightGeopolitical = 0.13 // 地緣政治風險：間歇性但高度衝擊
	StressWeightOil          = 0.07 // 原油：中東/能源風險代理
	StressWeightGold         = 0.06 // 黃金：避險/通膨代理

	// 壓力等級閾值
	StressThresholdCrisis = 70.0 // 紅燈：系統性風險
	StressThresholdHigh   = 50.0 // 橙燈：明顯出逃
	StressThresholdAlert  = 30.0 // 黃燈：注意波動
)

// StressIndexWeightsConfig holds runtime-configurable weights for the stress index.
// When nil, the compile-time constants are used as defaults.
type StressIndexWeightsConfig struct {
	Scaling    StressIndexScaling    `json:"scaling"`
	Weights    StressIndexWeights    `json:"weights"`
	Thresholds StressIndexThresholds `json:"thresholds"`
}

// IsValid returns true if the weights sum to approximately 1.0.
func (c *StressIndexWeightsConfig) IsValid() bool {
	if c == nil {
		return false
	}
	sum := c.Weights.DXY + c.Weights.US10Y + c.Weights.ForeignFlow +
		c.Weights.VIX + c.Weights.JPY + c.Weights.Geopolitical + c.Weights.Oil + c.Weights.Gold
	return sum > 0.99 && sum < 1.01
}

// StressIndexScaling maps each macro factor to its normalization scale.
type StressIndexScaling struct {
	DXY          float64 `json:"dxy"`
	US10Y        float64 `json:"us10y"`
	ForeignFlow  float64 `json:"foreign_flow"`
	VIX          float64 `json:"vix"`
	JPY          float64 `json:"jpy"`
	Geopolitical float64 `json:"geopolitical"`
	Oil          float64 `json:"oil"`
	Gold         float64 `json:"gold"`
}

// StressIndexWeights maps each macro factor to its weight in the composite index.
// The sum should be 1.0.
type StressIndexWeights struct {
	DXY          float64 `json:"dxy"`
	US10Y        float64 `json:"us10y"`
	ForeignFlow  float64 `json:"foreign_flow"`
	VIX          float64 `json:"vix"`
	JPY          float64 `json:"jpy"`
	Geopolitical float64 `json:"geopolitical"`
	Oil          float64 `json:"oil"`
	Gold         float64 `json:"gold"`
}

// StressIndexThresholds defines the alert/high/crisis levels for the stress index.
type StressIndexThresholds struct {
	Crisis float64 `json:"crisis"`
	High   float64 `json:"high"`
	Alert  float64 `json:"alert"`
}

// FormatWeights returns a concise string representation for logging/debugging.
func FormatWeights(w StressIndexWeights) string {
	return fmt.Sprintf(
		"DXY=%.3f US10Y=%.3f Flow=%.3f VIX=%.3f JPY=%.3f Geo=%.3f Oil=%.3f Gold=%.3f",
		w.DXY, w.US10Y, w.ForeignFlow, w.VIX, w.JPY, w.Geopolitical, w.Oil, w.Gold,
	)
}
