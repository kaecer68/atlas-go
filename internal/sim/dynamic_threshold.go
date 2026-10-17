package sim

import (
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type RiskAppetite int

const (
	RiskAppetiteConservative RiskAppetite = 1
	RiskAppetiteBalanced     RiskAppetite = 2
	RiskAppetiteAggressive   RiskAppetite = 3
)

type RegimeType string

const (
	RegimeBull    RegimeType = "bull"
	RegimeBear    RegimeType = "bear"
	RegimeNeutral RegimeType = "neutral"
	RegimeHighVol RegimeType = "highvol"
)

type MarketIndicators struct {
	VIX      float64
	SPXTrend float64
}

type DynamicThresholdEngine struct {
	mu                sync.RWMutex
	baseThreshold     float64
	minThreshold      float64
	maxThreshold      float64
	regimeMultipliers map[RegimeType]float64
	currentRegime     RegimeType
	lastVIX           float64
	updateCount       int
	riskAppetite      RiskAppetite
}

func NewDynamicThresholdEngine() *DynamicThresholdEngine {
	return &DynamicThresholdEngine{
		baseThreshold: 0.70,
		minThreshold:  0.40,
		maxThreshold:  0.85,
		regimeMultipliers: map[RegimeType]float64{
			RegimeBull:    -0.05,
			RegimeBear:    0.10,
			RegimeNeutral: 0.00,
			RegimeHighVol: 0.15,
		},
		currentRegime: RegimeNeutral,
		lastVIX:       20.0,
		riskAppetite:  RiskAppetiteBalanced,
	}
}

func (e *DynamicThresholdEngine) GetThreshold(vix float64, regime RegimeType) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastVIX = vix
	e.currentRegime = regime
	e.updateCount++

	vixAdjustment := (vix - 20.0) / 100.0
	regimeAdjustment := e.regimeMultipliers[regime]
	appetiteAdjustment := e.getAppetiteAdjustment()

	threshold := e.baseThreshold + vixAdjustment + regimeAdjustment + appetiteAdjustment

	if threshold < e.minThreshold {
		threshold = e.minThreshold
	}
	if threshold > e.maxThreshold {
		threshold = e.maxThreshold
	}

	return threshold
}

func (e *DynamicThresholdEngine) GetCurrentThreshold() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.calculateThresholdUnsafe(e.lastVIX, e.currentRegime)
}

func (e *DynamicThresholdEngine) calculateThresholdUnsafe(vix float64, regime RegimeType) float64 {
	vixAdjustment := (vix - 20.0) / 100.0
	regimeAdjustment := e.regimeMultipliers[regime]
	appetiteAdjustment := e.getAppetiteAdjustment()
	threshold := e.baseThreshold + vixAdjustment + regimeAdjustment + appetiteAdjustment

	if threshold < e.minThreshold {
		threshold = e.minThreshold
	}
	if threshold > e.maxThreshold {
		threshold = e.maxThreshold
	}

	return threshold
}

func (e *DynamicThresholdEngine) DetectRegime(indicators MarketIndicators) RegimeType {
	vix := indicators.VIX

	if vix > 30 {
		return RegimeHighVol
	}
	if vix > 25 && indicators.SPXTrend < 0 {
		return RegimeBear
	}
	if vix < 15 && indicators.SPXTrend > 0 {
		return RegimeBull
	}
	return RegimeNeutral
}

// GetRegimeMultiplier returns the regime multiplier currently configured for
// regime. It is a read-only accessor: the same value is applied inside
// GetThreshold / calculateThresholdUnsafe (`regimeAdjustment :=
// e.regimeMultipliers[regime]`), so a caller is not required for the regime
// adjustment to take effect.
//
// Audit note (FU-20261016-32, verified 2026-10-16): this accessor has no
// production caller, and that is not by itself a control gap — nothing here
// claims to gate anything. The engine's *inputs* are the separate story: the
// production calls into this type are SetRiskAppetite
// (orchestrator/system_dispatcher.go:122 and :135), and no production path calls
// GetThreshold or GetCurrentThreshold, so the dynamic threshold is written but
// never read. That wiring question is recorded in FU-20261016-32 and is not
// resolved here (it would change production risk thresholds).
func (e *DynamicThresholdEngine) GetRegimeMultiplier(regime RegimeType) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.regimeMultipliers[regime]
}

func (e *DynamicThresholdEngine) SetBaseThreshold(threshold float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.baseThreshold = threshold
}

func (e *DynamicThresholdEngine) SetRiskAppetite(ra RiskAppetite) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.riskAppetite = ra
}

func (e *DynamicThresholdEngine) GetRiskAppetite() RiskAppetite {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.riskAppetite
}

func (e *DynamicThresholdEngine) getAppetiteAdjustment() float64 {
	switch e.riskAppetite {
	case RiskAppetiteConservative:
		return 0.10
	case RiskAppetiteAggressive:
		return -0.05
	default:
		return 0.00
	}
}

func (e *DynamicThresholdEngine) GetStats() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return map[string]any{
		"base_threshold":    e.baseThreshold,
		"min_threshold":     e.minThreshold,
		"max_threshold":     e.maxThreshold,
		"current_regime":    e.currentRegime,
		"last_vix":          e.lastVIX,
		"current_threshold": e.calculateThresholdUnsafe(e.lastVIX, e.currentRegime),
		"update_count":      e.updateCount,
	}
}

func RegimeFromDomain(r domain.Regime) RegimeType {
	switch r {
	case domain.RegimeRiskOn:
		return RegimeBull
	case domain.RegimeRiskOff:
		return RegimeBear
	default:
		return RegimeNeutral
	}
}
