package narrative

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TaiwanStressIndex represents a composite market pressure score for Taiwan.
type TaiwanStressIndex struct {
	Score      float64            `json:"score"`  // 0 - 100
	Regime     string             `json:"regime"` // low / alert / high / crisis
	Components map[string]float64 `json:"components"`
	Timestamp  int64              `json:"timestamp"`
}

// --- 外資出逃壓力指數 六因子權重常數 ---
//
// 設計原則：權重反映該因子對「外資撤離台股」的 DIRECTNESS（直接性），而非解釋力。
// - 外商淨流向（25%）：最直接的可觀察市場數據，真實反映外資行為
// - 美債殖利率（20%）：強領先指標，機構資金對利率預期最敏感
// - DXY 美元指數（15%）：美元走強引導資金回流美國，屬於間接但重要的推力
// - VIX 恐慌指數（15%）：全球風險偏好溫度計，新興市場連動性高
// - 地緣政治風險（15%）：台海 / 中東 / 全球風險事件的衝擊，間歇性但高度影響
// - 日圓套利壓力（10%）：歷史相關性最弱，主要透過新興市場情緒間接傳導
// - 原油（7%）：中東/能源衝擊的即時壓力代理
// - 黃金（6%）：避險資金與通膨/地緣壓力代理
//
// 演進機制：這些權重不應永久固定。建議以下演進路徑：
//  1. 短期（當前）：固定權重，基於領域知識設定
//  2. 中期（回測校準）：根據 rolling 12-month 回溯測試中每個因子對外資流出的預測準確度重新校準
//  3. 長期（自適應）：依市場體制（bull / bear / crisis）使用不同權重組合
//     - Bull market: 提高 VIX、Geopolitical 權重（黑天鵝預警）
//     - Bear market: 提高 ForeignFlow、US10Y 權重（趨勢跟隨）
//     - Crisis: 所有權重拉平（全面壓力監控）
const (
	// 因子標準化縮放係數 — 將原始數據映射到 0-100 區間
	stressScaleDXY          = 5.0          // DXY 變化率 (%) → 壓力分數：每 1% 變化 = 5 分，20% 達上限
	stressScaleUS10Y        = 2.0          // 美債殖利率絕對值 → 壓力分數：每 1% = 2 分，50% 達上限
	stressScaleForeignFlow  = 10.0         // 外資淨賣超（億）→ 壓力分數：每 1 億 = 10 分，10 億達上限
	stressScaleVIX          = 100.0 / 40.0 // VIX 原始值 → 壓力分數：VIX=30 → 75 分，VIX=40 → 100 分
	stressScaleJPY          = 10.0         // 日圓變化率 (%) → 壓力分數：每 1% = 10 分，10% 達上限
	stressScaleGeopolitical = 1.0          // 地緣風險強度直接使用（已為 0-100）
	stressScaleOil          = 2.0          // 原油變化率 (%) → 每 $10 變化 = 2 分，$500 達上限
	stressScaleGold         = 2.0          // 黃金變化率 (%) → 每 $50 變化 = 2 分，$2500 達上限

	// 六因子權重 — 總和必須為 1.00
	stressWeightDXY          = 0.13 // DXY 美元指數：美元走強 → 資金回流美國 → 台股賣壓
	stressWeightUS10Y        = 0.18 // US10Y 美債殖利率：利率上升 → 資金流向美債 → 外資撤離
	stressWeightForeignFlow  = 0.22 // 外商淨流向：最直接的壓力指標，權重最高
	stressWeightVIX          = 0.13 // VIX 恐慌指數：全球避險情緒 → 新興市場資金流出
	stressWeightJPY          = 0.08 // 日圓套利平倉：間接影響，透過新興市場情緒傳導
	stressWeightGeopolitical = 0.13 // 地緣政治風險：間歇性但高度衝擊
	stressWeightOil          = 0.07 // 原油：中東/能源風險代理
	stressWeightGold         = 0.06 // 黃金：避險/通膨代理

	// 壓力等級閾值
	stressThresholdCrisis = 70.0 // 紅燈：系統性風險
	stressThresholdHigh   = 50.0 // 橙燈：明顯出逃
	stressThresholdAlert  = 30.0 // 黃燈：注意波動
)

// TaiwanStressCalculator computes the stress index from macro and capital flow data.
type TaiwanStressCalculator struct {
	geoProvider   GeopoliticalRiskProvider
	mu            sync.RWMutex
	cache         *TaiwanStressIndex
	cachedAt      time.Time
	cacheTTL      time.Duration
	weightsConfig *StressIndexWeightsConfig
}

// StressIndexWeightsConfig holds runtime-configurable weights for the stress index.
// When nil, the compile-time constants are used as defaults.
type StressIndexWeightsConfig struct {
	Scaling    StressIndexScaling    `json:"scaling"`
	Weights    StressIndexWeights    `json:"weights"`
	Thresholds StressIndexThresholds `json:"thresholds"`
}

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

type StressIndexThresholds struct {
	Crisis float64 `json:"crisis"`
	High   float64 `json:"high"`
	Alert  float64 `json:"alert"`
}

// LoadWeightsConfig reads stress index weights from the centralized parameters system.
// Deprecated: Use config.GetParametersConfig().Narrative for authoritative stress index parameters.
func LoadWeightsConfig(workDir string) *StressIndexWeightsConfig {
	return loadWeightsFromParameters()
}

func loadWeightsFromParameters() *StressIndexWeightsConfig {
	p := config.GetParametersConfig()
	if p == nil {
		return nil
	}
	n := p.Narrative
	cfg := &StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY:          paramOrDefault(n.TaiwanStressDXYScale.Value, stressScaleDXY),
			US10Y:        paramOrDefault(n.TaiwanStressUS10YScale.Value, stressScaleUS10Y),
			ForeignFlow:  paramOrDefault(n.TaiwanStressForeignScale.Value, stressScaleForeignFlow),
			VIX:          paramOrDefault(n.TaiwanStressVIXScale.Value, stressScaleVIX),
			JPY:          paramOrDefault(n.TaiwanStressJPYScale.Value, stressScaleJPY),
			Geopolitical: paramOrDefault(n.TaiwanStressGeoScale.Value, stressScaleGeopolitical),
			Oil:          paramOrDefault(n.TaiwanStressOilScale.Value, stressScaleOil),
			Gold:         paramOrDefault(n.TaiwanStressGoldScale.Value, stressScaleGold),
		},
		Weights: StressIndexWeights{
			DXY:          paramOrDefault(n.TaiwanStressDXYWeight.Value, stressWeightDXY),
			US10Y:        paramOrDefault(n.TaiwanStressUS10YWeight.Value, stressWeightUS10Y),
			ForeignFlow:  paramOrDefault(n.TaiwanStressForeignWeight.Value, stressWeightForeignFlow),
			VIX:          paramOrDefault(n.TaiwanStressVIXWeight.Value, stressWeightVIX),
			JPY:          paramOrDefault(n.TaiwanStressJPYWeight.Value, stressWeightJPY),
			Geopolitical: paramOrDefault(n.TaiwanStressGeoWeight.Value, stressWeightGeopolitical),
			Oil:          paramOrDefault(n.TaiwanStressOilWeight.Value, stressWeightOil),
			Gold:         paramOrDefault(n.TaiwanStressGoldWeight.Value, stressWeightGold),
		},
		Thresholds: StressIndexThresholds{
			Crisis: paramOrDefault(n.TaiwanStressCrisisThreshold.Value, stressThresholdCrisis),
			High:   paramOrDefault(n.TaiwanStressHighThreshold.Value, stressThresholdHigh),
			Alert:  paramOrDefault(n.TaiwanStressAlertThreshold.Value, stressThresholdAlert),
		},
	}
	if !cfg.isValid() {
		return nil
	}
	return cfg
}

func paramOrDefault(param, def float64) float64 {
	if param != 0 {
		return param
	}
	return def
}

func (c *StressIndexWeightsConfig) isValid() bool {
	sum := c.Weights.DXY + c.Weights.US10Y + c.Weights.ForeignFlow +
		c.Weights.VIX + c.Weights.JPY + c.Weights.Geopolitical + c.Weights.Oil + c.Weights.Gold
	return sum > 0.99 && sum < 1.01
}

// NewTaiwanStressCalculator creates a calculator with an optional geopolitical provider.
// Loads runtime weights from the centralized parameters system (config.GetParametersConfig).
func NewTaiwanStressCalculator(geoProvider GeopoliticalRiskProvider, workDir string) *TaiwanStressCalculator {
	if geoProvider == nil {
		geoProvider = NewRSSGeopoliticalProvider()
	}
	cfg := loadWeightsFromParameters()
	return &TaiwanStressCalculator{
		geoProvider:   geoProvider,
		cacheTTL:      5 * time.Minute,
		weightsConfig: cfg,
	}
}

// Calculate computes the stress index from the given snapshot and geopolitical score.
// The prev snapshot is used to compute change percentages for indicators where the current change is zero.
// Uses runtime weights from configs/stress_index_weights.json if loaded, falling back to compile-time defaults.
func (c *TaiwanStressCalculator) Calculate(snap, prev marketdata.MacroDataSnapshot, geoScore GeopoliticalRiskScore) TaiwanStressIndex {
	components := make(map[string]float64)

	scaleDXY, scaleUS10Y, scaleFlow, scaleVIX, scaleJPY, scaleGeo, scaleOil, scaleGold := c.getScaling()
	wDXY, wUS10Y, wFlow, wVIX, wJPY, wGeo, wOil, wGold := c.getWeights()
	tCrisis, tHigh, tAlert := c.getThresholds()

	dxyComponent := math.Abs(snap.DXY.ChangePct) * scaleDXY
	if dxyComponent > 100 {
		dxyComponent = 100
	}
	components["dxy"] = dxyComponent * wDXY

	us10yChange := snap.US10Y.Value
	if us10yChange < 0 {
		us10yChange = -us10yChange
	}
	us10yComponent := us10yChange * scaleUS10Y
	if us10yComponent > 100 {
		us10yComponent = 100
	}
	components["us10y"] = us10yComponent * wUS10Y

	foreignFlow := -snap.ForeignInvestorNet.Value
	foreignComponent := foreignFlow * scaleFlow
	if foreignComponent > 100 {
		foreignComponent = 100
	}
	if foreignComponent < -100 {
		foreignComponent = -100
	}
	components["foreign_flow"] = foreignComponent * wFlow

	vixComponent := snap.VIX.Value * scaleVIX
	if vixComponent > 100 {
		vixComponent = 100
	}
	components["vix"] = vixComponent * wVIX

	jpyChange := math.Abs(snap.JPY.ChangePct)
	if jpyChange == 0 && snap.JPY.Symbol != "" && prev.JPY.Symbol != "" && prev.JPY.Value != 0 {
		jpyChange = math.Abs((snap.JPY.Value-prev.JPY.Value)/prev.JPY.Value) * 100
	}
	jpyComponent := jpyChange * scaleJPY
	if jpyComponent > 100 {
		jpyComponent = 100
	}
	components["jpy"] = jpyComponent * wJPY

	geoComponent := geoScore.Intensity * scaleGeo
	components["geopolitical"] = geoComponent * wGeo

	oilComponent := math.Abs(snap.Oil.ChangePct) * scaleOil
	if oilComponent > 100 {
		oilComponent = 100
	}
	components["oil"] = oilComponent * wOil

	goldComponent := math.Abs(snap.Gold.ChangePct) * scaleGold
	if goldComponent > 100 {
		goldComponent = 100
	}
	components["gold"] = goldComponent * wGold

	score := components["dxy"] + components["us10y"] + components["foreign_flow"] +
		components["vix"] + components["jpy"] + components["geopolitical"] + components["oil"] + components["gold"]

	regime := "low"
	switch {
	case score >= tCrisis:
		regime = "crisis"
	case score >= tHigh:
		regime = "high"
	case score >= tAlert:
		regime = "alert"
	}

	return TaiwanStressIndex{
		Score:      score,
		Regime:     regime,
		Components: components,
		Timestamp:  snap.RecordedAt,
	}
}

func (c *TaiwanStressCalculator) getScaling() (dxy, us10y, flow, vix, jpy, geo, oil, gold float64) {
	if c.weightsConfig != nil {
		return c.weightsConfig.Scaling.DXY, c.weightsConfig.Scaling.US10Y,
			c.weightsConfig.Scaling.ForeignFlow, c.weightsConfig.Scaling.VIX,
			c.weightsConfig.Scaling.JPY, c.weightsConfig.Scaling.Geopolitical,
			c.weightsConfig.Scaling.Oil, c.weightsConfig.Scaling.Gold
	}
	return stressScaleDXY, stressScaleUS10Y, stressScaleForeignFlow,
		stressScaleVIX, stressScaleJPY, stressScaleGeopolitical,
		stressScaleOil, stressScaleGold
}

func (c *TaiwanStressCalculator) getWeights() (dxy, us10y, flow, vix, jpy, geo, oil, gold float64) {
	if c.weightsConfig != nil {
		return c.weightsConfig.Weights.DXY, c.weightsConfig.Weights.US10Y,
			c.weightsConfig.Weights.ForeignFlow, c.weightsConfig.Weights.VIX,
			c.weightsConfig.Weights.JPY, c.weightsConfig.Weights.Geopolitical,
			c.weightsConfig.Weights.Oil, c.weightsConfig.Weights.Gold
	}
	return stressWeightDXY, stressWeightUS10Y, stressWeightForeignFlow,
		stressWeightVIX, stressWeightJPY, stressWeightGeopolitical,
		stressWeightOil, stressWeightGold
}

func (c *TaiwanStressCalculator) getThresholds() (crisis, high, alert float64) {
	if c.weightsConfig != nil {
		return c.weightsConfig.Thresholds.Crisis, c.weightsConfig.Thresholds.High,
			c.weightsConfig.Thresholds.Alert
	}
	return stressThresholdCrisis, stressThresholdHigh, stressThresholdAlert
}

// CalculateFromSnapshot fetches the geopolitical score and computes the index.
// If the live fetch fails, it attempts to load the latest persisted score as fallback.
// Results are cached for 5 minutes to avoid repeated slow external calls on every dashboard refresh.
func (c *TaiwanStressCalculator) CalculateFromSnapshot(ctx context.Context, snap, prev marketdata.MacroDataSnapshot) (TaiwanStressIndex, error) {
	c.mu.RLock()
	if c.cache != nil && time.Since(c.cachedAt) < c.cacheTTL {
		idx := *c.cache
		c.mu.RUnlock()
		return idx, nil
	}
	c.mu.RUnlock()

	geoScore, err := c.geoProvider.FetchScore(ctx)
	if err != nil {
		return TaiwanStressIndex{}, fmt.Errorf("fetch geopolitical score: %w", err)
	}
	idx := c.Calculate(snap, prev, geoScore)

	c.mu.Lock()
	c.cache = &idx
	c.cachedAt = time.Now()
	c.mu.Unlock()
	return idx, nil
}

// CalculateFromSnapshotWithStore fetches the geopolitical score and computes the index,
// falling back to a persisted score from the provided store if the live fetch fails.
func (c *TaiwanStressCalculator) CalculateFromSnapshotWithStore(ctx context.Context, snap, prev marketdata.MacroDataSnapshot, store *GeopoliticalStore) (TaiwanStressIndex, error) {
	c.mu.RLock()
	if c.cache != nil && time.Since(c.cachedAt) < c.cacheTTL {
		idx := *c.cache
		c.mu.RUnlock()
		return idx, nil
	}
	c.mu.RUnlock()

	geoScore, err := c.geoProvider.FetchScore(ctx)
	if err != nil {
		if store != nil {
			fallback, loadErr := store.Load()
			if loadErr == nil {
				geoScore = fallback
			} else {
				return TaiwanStressIndex{}, fmt.Errorf("fetch geopolitical score: %w (fallback load also failed: %v)", err, loadErr)
			}
		} else {
			return TaiwanStressIndex{}, fmt.Errorf("fetch geopolitical score: %w", err)
		}
	}
	idx := c.Calculate(snap, prev, geoScore)

	c.mu.Lock()
	c.cache = &idx
	c.cachedAt = time.Now()
	c.mu.Unlock()
	return idx, nil
}
