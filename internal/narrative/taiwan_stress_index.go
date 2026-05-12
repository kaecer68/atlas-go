package narrative

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

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
//
// 演進機制：這些權重不應永久固定。建議以下演進路徑：
//   1. 短期（當前）：固定權重，基於領域知識設定
//   2. 中期（回測校準）：根據 rolling 12-month 回溯測試中每個因子對外資流出的預測準確度重新校準
//   3. 長期（自適應）：依市場體制（bull / bear / crisis）使用不同權重組合
//      - Bull market: 提高 VIX、Geopolitical 權重（黑天鵝預警）
//      - Bear market: 提高 ForeignFlow、US10Y 權重（趨勢跟隨）
//      - Crisis: 所有權重拉平（全面壓力監控）
const (
	// 因子標準化縮放係數 — 將原始數據映射到 0-100 區間
	stressScaleDXY          = 5.0  // DXY 變化率 (%) → 壓力分數：每 1% 變化 = 5 分，20% 達上限
	stressScaleUS10Y        = 2.0  // 美債殖利率絕對值 → 壓力分數：每 1% = 2 分，50% 達上限
	stressScaleForeignFlow  = 10.0 // 外資淨賣超（億）→ 壓力分數：每 1 億 = 10 分，10 億達上限
	stressScaleVIX          = 100.0 / 40.0 // VIX 原始值 → 壓力分數：VIX=30 → 75 分，VIX=40 → 100 分
	stressScaleJPY          = 10.0 // 日圓變化率 (%) → 壓力分數：每 1% = 10 分，10% 達上限
	stressScaleGeopolitical = 1.0  // 地緣風險強度直接使用（已為 0-100）

	// 六因子權重 — 總和必須為 1.00
	stressWeightDXY          = 0.15 // DXY 美元指數：美元走強 → 資金回流美國 → 台股賣壓
	stressWeightUS10Y        = 0.20 // US10Y 美債殖利率：利率上升 → 資金流向美債 → 外資撤離
	stressWeightForeignFlow  = 0.25 // 外商淨流向：最直接的壓力指標，權重最高
	stressWeightVIX          = 0.15 // VIX 恐慌指數：全球避險情緒 → 新興市場資金流出
	stressWeightJPY          = 0.10 // 日圓套利平倉：間接影響，透過新興市場情緒傳導
	stressWeightGeopolitical = 0.15 // 地緣政治風險：間歇性但高度衝擊

	// 壓力等級閾值
	stressThresholdCrisis = 70.0 // 紅燈：系統性風險
	stressThresholdHigh   = 50.0 // 橙燈：明顯出逃
	stressThresholdAlert  = 30.0 // 黃燈：注意波動
)

// TaiwanStressCalculator computes the stress index from macro and capital flow data.
type TaiwanStressCalculator struct {
	geoProvider GeopoliticalRiskProvider
	mu          sync.RWMutex
	cache       *TaiwanStressIndex
	cachedAt    time.Time
	cacheTTL    time.Duration
}

// NewTaiwanStressCalculator creates a calculator with an optional geopolitical provider.
func NewTaiwanStressCalculator(geoProvider GeopoliticalRiskProvider) *TaiwanStressCalculator {
	if geoProvider == nil {
		geoProvider = NewRSSGeopoliticalProvider()
	}
	return &TaiwanStressCalculator{
		geoProvider: geoProvider,
		cacheTTL:    5 * time.Minute,
	}
}

// Calculate computes the stress index from the given snapshot and geopolitical score.
// The prev snapshot is used to compute change percentages for indicators where the current change is zero.
func (c *TaiwanStressCalculator) Calculate(snap, prev marketdata.MacroDataSnapshot, geoScore GeopoliticalRiskScore) TaiwanStressIndex {
	components := make(map[string]float64)

	dxyComponent := math.Abs(snap.DXY.ChangePct) * stressScaleDXY
	if dxyComponent > 100 {
		dxyComponent = 100
	}
	components["dxy"] = dxyComponent * stressWeightDXY

	us10yChange := snap.US10Y.Value
	if us10yChange < 0 {
		us10yChange = -us10yChange
	}
	us10yComponent := us10yChange * stressScaleUS10Y
	if us10yComponent > 100 {
		us10yComponent = 100
	}
	components["us10y"] = us10yComponent * stressWeightUS10Y

	// Positive when foreign investors sell (stress), negative when they buy (relief).
	foreignFlow := -snap.ForeignInvestorNet.Value
	foreignComponent := foreignFlow * stressScaleForeignFlow
	if foreignComponent > 100 {
		foreignComponent = 100
	}
	if foreignComponent < -100 {
		foreignComponent = -100
	}
	components["foreign_flow"] = foreignComponent * stressWeightForeignFlow

	vixComponent := snap.VIX.Value * stressScaleVIX
	if vixComponent > 100 {
		vixComponent = 100
	}
	components["vix"] = vixComponent * stressWeightVIX

	jpyChange := math.Abs(snap.JPY.ChangePct)
	if jpyChange == 0 && snap.JPY.Symbol != "" && prev.JPY.Symbol != "" && prev.JPY.Value != 0 {
		jpyChange = math.Abs((snap.JPY.Value-prev.JPY.Value)/prev.JPY.Value) * 100
	}
	jpyComponent := jpyChange * stressScaleJPY
	if jpyComponent > 100 {
		jpyComponent = 100
	}
	components["jpy"] = jpyComponent * stressWeightJPY

	geoComponent := geoScore.Intensity * stressScaleGeopolitical
	components["geopolitical"] = geoComponent * stressWeightGeopolitical

	score := components["dxy"] + components["us10y"] + components["foreign_flow"] +
		components["vix"] + components["jpy"] + components["geopolitical"]

	regime := "low"
	switch {
	case score >= stressThresholdCrisis:
		regime = "crisis"
	case score >= stressThresholdHigh:
		regime = "high"
	case score >= stressThresholdAlert:
		regime = "alert"
	}

	return TaiwanStressIndex{
		Score:      score,
		Regime:     regime,
		Components: components,
		Timestamp:  snap.RecordedAt,
	}
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
