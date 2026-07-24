package narrative

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// TaiwanStressIndex represents a composite market pressure score for Taiwan.
//
// Date and Source are populated when the row is read from the ledger
// (stress_index_history SQLite table via StressRow.Date / StressRow.Source);
// they are zero values when the row is computed in-memory by Calculate().
// Consumers that need to join rows to trading dates should use Date, not
// Timestamp (which is captured_at in epoch seconds and may fall on a
// weekend / holiday even when the row is backfill for a trading day).
type TaiwanStressIndex struct {
	Score      float64            `json:"score"`  // 0 - 100
	Regime     string             `json:"regime"` // low / alert / high / crisis (or legacy RISK_ON/.../TRANSITIONAL — see Source)
	Components map[string]float64 `json:"components"`
	Timestamp  int64              `json:"timestamp"`        // captured_at in epoch seconds (may not equal Date)
	Date       string             `json:"date,omitempty"`   // YYYY-MM-DD; empty for in-memory Calculate() rows
	Source     string             `json:"source,omitempty"` // "macro_ingest" for live rows, "backfill" for legacy rows; empty for in-memory Calculate() rows
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
	geoProvider    geopolitical.GeopoliticalRiskProvider
	mu             sync.RWMutex
	cache          *TaiwanStressIndex
	cachedAt       time.Time
	cacheTTL       time.Duration
	weightsConfig  *StressIndexWeightsConfig
	regimeConfig   *RegimeCalibratedConfig
	baselines      *BaselineConfig
	signalStrategy SignalStrategy
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
			DXY:          n.TaiwanStressDXYScale.Value,
			US10Y:        n.TaiwanStressUS10YScale.Value,
			ForeignFlow:  n.TaiwanStressForeignScale.Value,
			VIX:          n.TaiwanStressVIXScale.Value,
			JPY:          n.TaiwanStressJPYScale.Value,
			Geopolitical: n.TaiwanStressGeoScale.Value,
			Oil:          n.TaiwanStressOilScale.Value,
			Gold:         n.TaiwanStressGoldScale.Value,
		},
		Weights: StressIndexWeights{
			DXY:          n.TaiwanStressDXYWeight.Value,
			US10Y:        n.TaiwanStressUS10YWeight.Value,
			ForeignFlow:  n.TaiwanStressForeignWeight.Value,
			VIX:          n.TaiwanStressVIXWeight.Value,
			JPY:          n.TaiwanStressJPYWeight.Value,
			Geopolitical: n.TaiwanStressGeoWeight.Value,
			Oil:          n.TaiwanStressOilWeight.Value,
			Gold:         n.TaiwanStressGoldWeight.Value,
		},
		Thresholds: StressIndexThresholds{
			Crisis: n.TaiwanStressCrisisThreshold.Value,
			High:   n.TaiwanStressHighThreshold.Value,
			Alert:  n.TaiwanStressAlertThreshold.Value,
		},
	}
	if !cfg.isValid() {
		return nil
	}
	return cfg
}

func (c *StressIndexWeightsConfig) isValid() bool {
	sum := c.Weights.DXY + c.Weights.US10Y + c.Weights.ForeignFlow +
		c.Weights.VIX + c.Weights.JPY + c.Weights.Geopolitical + c.Weights.Oil + c.Weights.Gold
	return sum > 0.99 && sum < 1.01
}

// NewTaiwanStressCalculator creates a calculator with an optional geopolitical provider.
// When geoProvider is nil, stress index geo component will be zero (caller should inject
// a proper provider via the apigateway GeopoliticalChannelAdapter).
// Loads runtime weights from the centralized parameters system (config.GetParametersConfig).
// If workDir is non-empty and persisted baselines exist at
// workDir/data/state/calibration/baselines.json, the calculator auto-enables
// hybrid signal (max of |level deviation| and |change_pct|) for
// DXY/JPY/US10Y/Oil/Gold. Production hot path: callers do not need to
// invoke NewTaiwanStressCalculatorWithBaseline explicitly. First-run
// (no baselines file) gracefully falls back to pure change_pct.
func NewTaiwanStressCalculator(geoProvider geopolitical.GeopoliticalRiskProvider, workDir string) *TaiwanStressCalculator {
	cfg := loadWeightsFromParameters()
	calc := &TaiwanStressCalculator{
		geoProvider:   geoProvider,
		cacheTTL:      5 * time.Minute,
		weightsConfig: cfg,
	}
	if workDir != "" {
		if bl, err := LoadBaselines(workDir); err == nil && bl != nil {
			calc.baselines = bl
			calc.signalStrategy = SignalHybrid
		}
	}
	return calc
}

// NewTaiwanStressCalculatorWithBaseline creates a calculator with baseline-aware hybrid
// signal computation for DXY, JPY, Oil, and Gold components. When baselines is non-nil
// and signalStrategy is SignalHybrid, these four components use max(|level deviation|, |change_pct|)
// instead of pure change_pct. Other components (US10Y, ForeignFlow, VIX, Geopolitical) are unchanged.
func NewTaiwanStressCalculatorWithBaseline(geoProvider geopolitical.GeopoliticalRiskProvider, workDir string, baselines *BaselineConfig, strategy SignalStrategy) *TaiwanStressCalculator {
	cfg := loadWeightsFromParameters()
	return &TaiwanStressCalculator{
		geoProvider:    geoProvider,
		cacheTTL:       5 * time.Minute,
		weightsConfig:  cfg,
		baselines:      baselines,
		signalStrategy: strategy,
	}
}

// useHybridSignal returns true when the calculator should use hybrid signal
// for DXY/JPY/Oil/Gold components.
func (c *TaiwanStressCalculator) useHybridSignal() bool {
	return c.baselines != nil && c.signalStrategy == SignalHybrid
}

// computeStressComponent computes a single stress component for the given factor.
// When hybrid signal is enabled, it returns the larger of |level deviation| and |change_pct|
// scaled appropriately. Otherwise, it uses the raw changePct.
func (c *TaiwanStressCalculator) computeStressComponent(factor string, snap, prev marketdata.MacroDataSnapshot, scale float64) float64 {
	changePct := factorChangePct(factor, snap, prev)
	if !c.useHybridSignal() {
		return clampComponent(math.Abs(changePct) * scale)
	}

	levelDev := ComputeLevelSignal(factor, snap, 0, c.baselines)
	changeAbs := math.Abs(changePct)
	if math.Abs(levelDev) > changeAbs {
		return clampComponent(math.Abs(levelDev) * scale)
	}
	return clampComponent(changeAbs * scale)
}

// factorChangePct extracts the change percentage for a factor from the snapshot pair.
func factorChangePct(factor string, snap, prev marketdata.MacroDataSnapshot) float64 {
	switch factor {
	case "dxy":
		return snap.DXY.ChangePct
	case "jpy":
		if snap.JPY.ChangePct != 0 {
			return snap.JPY.ChangePct
		}
		if snap.JPY.Symbol != "" && prev.JPY.Symbol != "" && prev.JPY.Value != 0 {
			return (snap.JPY.Value - prev.JPY.Value) / prev.JPY.Value * 100
		}
		return 0
	case "us10y":
		if snap.US10Y.ChangePct != 0 {
			return snap.US10Y.ChangePct
		}
		if snap.US10Y.Symbol != "" && prev.US10Y.Symbol != "" && prev.US10Y.Value != 0 {
			return (snap.US10Y.Value - prev.US10Y.Value) / prev.US10Y.Value * 100
		}
		return 0
	case "oil":
		return snap.Oil.ChangePct
	case "gold":
		return snap.Gold.ChangePct
	default:
		return 0
	}
}

// clampComponent clamps a stress component to [0, 100].
func clampComponent(v float64) float64 {
	if v > 100 {
		return 100
	}
	if v < 0 {
		return 0
	}
	return v
}

// selectConfigForSnapshot returns the regime-appropriate config based on VIX.
// Falls back to the calculator's default weightsConfig when regimeConfig is nil.
func (c *TaiwanStressCalculator) selectConfigForSnapshot(snap marketdata.MacroDataSnapshot) StressIndexWeightsConfig {
	if c.regimeConfig != nil {
		regime := classifyRegime(snap.VIX.Value)
		return c.regimeConfig.SelectConfig(regime)
	}
	if c.weightsConfig != nil {
		return *c.weightsConfig
	}
	return StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: stressScaleDXY, US10Y: stressScaleUS10Y,
			ForeignFlow: stressScaleForeignFlow, VIX: stressScaleVIX,
			JPY: stressScaleJPY, Geopolitical: stressScaleGeopolitical,
			Oil: stressScaleOil, Gold: stressScaleGold,
		},
		Weights: StressIndexWeights{
			DXY: stressWeightDXY, US10Y: stressWeightUS10Y,
			ForeignFlow: stressWeightForeignFlow, VIX: stressWeightVIX,
			JPY: stressWeightJPY, Geopolitical: stressWeightGeopolitical,
			Oil: stressWeightOil, Gold: stressWeightGold,
		},
		Thresholds: StressIndexThresholds{
			Crisis: stressThresholdCrisis,
			High:   stressThresholdHigh,
			Alert:  stressThresholdAlert,
		},
	}
}

// Calculate computes the stress index from the given snapshot and geopolitical score.
// The prev snapshot is used to compute change percentages for indicators where the current change is zero.
// Uses runtime weights from configs/stress_index_weights.json if loaded, falling back to compile-time defaults.
// When baselines are configured with SignalHybrid, DXY/JPY/Oil/Gold components use hybrid signal.
func (c *TaiwanStressCalculator) Calculate(snap, prev marketdata.MacroDataSnapshot, geoScore geopolitical.GeopoliticalRiskScore) TaiwanStressIndex {
	components := make(map[string]float64)

	cfg := c.selectConfigForSnapshot(snap)
	scaleDXY, scaleUS10Y, scaleFlow, scaleVIX, scaleJPY, scaleGeo, scaleOil, scaleGold := cfg.Scaling.DXY,
		cfg.Scaling.US10Y, cfg.Scaling.ForeignFlow, cfg.Scaling.VIX,
		cfg.Scaling.JPY, cfg.Scaling.Geopolitical, cfg.Scaling.Oil, cfg.Scaling.Gold
	wDXY, wUS10Y, wFlow, wVIX, wJPY, wGeo, wOil, wGold := cfg.Weights.DXY,
		cfg.Weights.US10Y, cfg.Weights.ForeignFlow, cfg.Weights.VIX,
		cfg.Weights.JPY, cfg.Weights.Geopolitical, cfg.Weights.Oil, cfg.Weights.Gold
	tCrisis, tHigh, tAlert := cfg.Thresholds.Crisis, cfg.Thresholds.High, cfg.Thresholds.Alert

	if c.useHybridSignal() {
		components["dxy"] = c.computeStressComponent("dxy", snap, prev, scaleDXY) * wDXY
	} else {
		dxyComponent := math.Abs(snap.DXY.ChangePct) * scaleDXY
		if dxyComponent > 100 {
			dxyComponent = 100
		}
		components["dxy"] = dxyComponent * wDXY
	}

	if c.useHybridSignal() {
		components["us10y"] = c.computeStressComponent("us10y", snap, prev, scaleUS10Y) * wUS10Y
	} else {
		us10yChange := snap.US10Y.Value
		if us10yChange < 0 {
			us10yChange = -us10yChange
		}
		us10yComponent := us10yChange * scaleUS10Y
		if us10yComponent > 100 {
			us10yComponent = 100
		}
		components["us10y"] = us10yComponent * wUS10Y
	}

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

	if c.useHybridSignal() {
		components["jpy"] = c.computeStressComponent("jpy", snap, prev, scaleJPY) * wJPY
	} else {
		jpyChange := math.Abs(snap.JPY.ChangePct)
		if jpyChange == 0 && snap.JPY.Symbol != "" && prev.JPY.Symbol != "" && prev.JPY.Value != 0 {
			jpyChange = math.Abs((snap.JPY.Value-prev.JPY.Value)/prev.JPY.Value) * 100
		}
		jpyComponent := jpyChange * scaleJPY
		if jpyComponent > 100 {
			jpyComponent = 100
		}
		components["jpy"] = jpyComponent * wJPY
	}

	geoComponent := geoScore.Intensity * scaleGeo
	components["geopolitical"] = geoComponent * wGeo

	if c.useHybridSignal() {
		components["oil"] = c.computeStressComponent("oil", snap, prev, scaleOil) * wOil
	} else {
		oilComponent := math.Abs(snap.Oil.ChangePct) * scaleOil
		if oilComponent > 100 {
			oilComponent = 100
		}
		components["oil"] = oilComponent * wOil
	}

	if c.useHybridSignal() {
		components["gold"] = c.computeStressComponent("gold", snap, prev, scaleGold) * wGold
	} else {
		goldComponent := math.Abs(snap.Gold.ChangePct) * scaleGold
		if goldComponent > 100 {
			goldComponent = 100
		}
		components["gold"] = goldComponent * wGold
	}

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

func (c *TaiwanStressCalculator) getThresholds() (crisis, high, alert float64) {
	if c.weightsConfig != nil {
		return c.weightsConfig.Thresholds.Crisis, c.weightsConfig.Thresholds.High,
			c.weightsConfig.Thresholds.Alert
	}
	return stressThresholdCrisis, stressThresholdHigh, stressThresholdAlert
}

// ApplyCalibratedScales updates the calculator's weightsConfig with calibrated scales.
// If weightsConfig is nil, creates a new one with default weights and thresholds.
func (c *TaiwanStressCalculator) ApplyCalibratedScales(scaling StressIndexScaling) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.weightsConfig == nil {
		c.weightsConfig = &StressIndexWeightsConfig{
			Weights: StressIndexWeights{
				DXY: stressWeightDXY, US10Y: stressWeightUS10Y,
				ForeignFlow: stressWeightForeignFlow, VIX: stressWeightVIX,
				JPY: stressWeightJPY, Geopolitical: stressWeightGeopolitical,
				Oil: stressWeightOil, Gold: stressWeightGold,
			},
			Thresholds: StressIndexThresholds{
				Crisis: stressThresholdCrisis,
				High:   stressThresholdHigh,
				Alert:  stressThresholdAlert,
			},
		}
	}
	c.weightsConfig.Scaling = scaling
}

// SetRegimeConfig installs a regime-aware config. When set, Calculate()
// automatically selects the appropriate weights/scales/thresholds based on VIX.
func (c *TaiwanStressCalculator) SetRegimeConfig(rc *RegimeCalibratedConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.regimeConfig = rc
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

	if c.geoProvider == nil {
		idx := c.Calculate(snap, prev, geopolitical.GeopoliticalRiskScore{})
		c.mu.Lock()
		c.cache = &idx
		c.cachedAt = time.Now()
		c.mu.Unlock()
		return idx, nil
	}
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
func (c *TaiwanStressCalculator) CalculateFromSnapshotWithStore(ctx context.Context, snap, prev marketdata.MacroDataSnapshot, store *geopolitical.GeopoliticalStore) (TaiwanStressIndex, error) {
	c.mu.RLock()
	if c.cache != nil && time.Since(c.cachedAt) < c.cacheTTL {
		idx := *c.cache
		c.mu.RUnlock()
		return idx, nil
	}
	c.mu.RUnlock()

	if c.geoProvider == nil {
		if store != nil {
			fallback, loadErr := store.Load()
			if loadErr == nil {
				idx := c.Calculate(snap, prev, fallback)
				c.mu.Lock()
				c.cache = &idx
				c.cachedAt = time.Now()
				c.mu.Unlock()
				return idx, nil
			}
		}
		idx := c.Calculate(snap, prev, geopolitical.GeopoliticalRiskScore{})
		c.mu.Lock()
		c.cache = &idx
		c.cachedAt = time.Now()
		c.mu.Unlock()
		return idx, nil
	}
	geoScore, err := c.geoProvider.FetchScore(ctx)
	if err != nil {
		if store != nil {
			fallback, loadErr := store.Load()
			if loadErr == nil {
				geoScore = fallback
			} else {
				return TaiwanStressIndex{}, fmt.Errorf("fetch geopolitical score: %w (fallback load also failed: %w)", err, loadErr)
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
