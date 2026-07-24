package narrative

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/calibration"
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
	Source     string             `json:"source,omitempty"` // "macro_ingest" for live pipeline, "snapshot_backfill" for daily backfill, "synthetic" for stage-4 seed/SQLite DEFAULT; empty for in-memory Calculate() rows
}

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
	cfg := calibration.LoadWeightsConfig("")
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
	cfg := calibration.LoadWeightsConfig("")
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
		regime := ClassifyRegime(snap.VIX.Value)
		return c.regimeConfig.SelectConfig(regime)
	}
	if c.weightsConfig != nil {
		return *c.weightsConfig
	}
	return StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: calibration.StressScaleDXY, US10Y: calibration.StressScaleUS10Y,
			ForeignFlow: calibration.StressScaleForeignFlow, VIX: calibration.StressScaleVIX,
			JPY: calibration.StressScaleJPY, Geopolitical: calibration.StressScaleGeopolitical,
			Oil: calibration.StressScaleOil, Gold: calibration.StressScaleGold,
		},
		Weights: StressIndexWeights{
			DXY: calibration.StressWeightDXY, US10Y: calibration.StressWeightUS10Y,
			ForeignFlow: calibration.StressWeightForeignFlow, VIX: calibration.StressWeightVIX,
			JPY: calibration.StressWeightJPY, Geopolitical: calibration.StressWeightGeopolitical,
			Oil: calibration.StressWeightOil, Gold: calibration.StressWeightGold,
		},
		Thresholds: StressIndexThresholds{
			Crisis: calibration.StressThresholdCrisis,
			High:   calibration.StressThresholdHigh,
			Alert:  calibration.StressThresholdAlert,
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
	return calibration.StressThresholdCrisis, calibration.StressThresholdHigh, calibration.StressThresholdAlert
}

// ApplyCalibratedScales updates the calculator's weightsConfig with calibrated scales.
// If weightsConfig is nil, creates a new one with default weights and thresholds.
func (c *TaiwanStressCalculator) ApplyCalibratedScales(scaling StressIndexScaling) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.weightsConfig == nil {
		c.weightsConfig = &StressIndexWeightsConfig{
			Weights: StressIndexWeights{
				DXY: calibration.StressWeightDXY, US10Y: calibration.StressWeightUS10Y,
				ForeignFlow: calibration.StressWeightForeignFlow, VIX: calibration.StressWeightVIX,
				JPY: calibration.StressWeightJPY, Geopolitical: calibration.StressWeightGeopolitical,
				Oil: calibration.StressWeightOil, Gold: calibration.StressWeightGold,
			},
			Thresholds: StressIndexThresholds{
				Crisis: calibration.StressThresholdCrisis,
				High:   calibration.StressThresholdHigh,
				Alert:  calibration.StressThresholdAlert,
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
