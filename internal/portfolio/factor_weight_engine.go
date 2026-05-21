package portfolio

import (
	"maps"
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

type FactorWeightEngine struct {
	mu           sync.RWMutex
	baseWeights  map[FactorType]float64
	eventWeights map[string]map[FactorType]float64
	activeEvents map[string]*narrative.NarrativeEvent
	lifecycle    *narrative.EventLifecycleManager
	weightSource string
}

func (e *FactorWeightEngine) WeightSource() string {
	return e.weightSource
}

func fwConfig() *config.FactorWeightParameters {
	if cfg := config.GetParametersConfig(); cfg != nil {
		return &cfg.FactorWeight
	}
	return nil
}

func defaultBaseWeights() map[FactorType]float64 {
	return map[FactorType]float64{
		FactorMomentum:      0.25,
		FactorValue:         0.20,
		FactorQuality:       0.20,
		FactorAgent:         0.15,
		FactorInstSent:      0.10,
		FactorLiquidity:     0.05,
		FactorNarrative:     0.05,
		FactorIndustryCycle: 0.00,
	}
}

func NewFactorWeightEngine() *FactorWeightEngine {
	baseWeights := defaultBaseWeights()
	source := "builtin_defaults"
	if fw := fwConfig(); fw != nil && fw.BaseWeights.Value != nil {
		bw := make(map[FactorType]float64, len(fw.BaseWeights.Value))
		for k, v := range fw.BaseWeights.Value {
			bw[FactorType(k)] = v
		}
		baseWeights = bw
		source = "config"
	}
	return &FactorWeightEngine{
		baseWeights:  baseWeights,
		eventWeights: make(map[string]map[FactorType]float64),
		activeEvents: make(map[string]*narrative.NarrativeEvent),
		lifecycle:    narrative.NewEventLifecycleManager(),
		weightSource: source,
	}
}

func (e *FactorWeightEngine) GetWeights(regime string) map[FactorType]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	weights := make(map[FactorType]float64)
	maps.Copy(weights, e.baseWeights)

	// Read regime deltas and clamp bounds from config with hardcoded fallback
	fw := fwConfig()
	var (
		bullMomentum float64 = 0.05
		bullQuality  float64 = -0.03
		bullValue    float64 = -0.02
		bearQuality  float64 = 0.05
		bearValue    float64 = 0.03
		bearMomentum float64 = -0.05
		highVolLiq   float64 = 0.05
		highVolMom   float64 = -0.03
		highVolInst  float64 = -0.02
		clampMin     float64 = 0.02
		clampMax     float64 = 0.50
	)
	if fw != nil {
		bullMomentum = fw.RegimeBullMomentum.Value
		bullQuality = fw.RegimeBullQuality.Value
		bullValue = fw.RegimeBullValue.Value
		bearQuality = fw.RegimeBearQuality.Value
		bearValue = fw.RegimeBearValue.Value
		bearMomentum = fw.RegimeBearMomentum.Value
		highVolLiq = fw.RegimeHighVolLiquidity.Value
		highVolMom = fw.RegimeHighVolMomentum.Value
		highVolInst = fw.RegimeHighVolInstSent.Value
		clampMin = fw.ClampMin.Value
		clampMax = fw.ClampMax.Value
	}

	switch regime {
	case "bull":
		weights[FactorMomentum] += bullMomentum
		weights[FactorQuality] += bullQuality
		weights[FactorValue] += bullValue
	case "bear":
		weights[FactorQuality] += bearQuality
		weights[FactorValue] += bearValue
		weights[FactorMomentum] += bearMomentum
	case "high_vol":
		weights[FactorLiquidity] += highVolLiq
		weights[FactorMomentum] += highVolMom
		weights[FactorInstSent] += highVolInst
	}

	for _, event := range e.activeEvents {
		if adj, ok := e.eventWeights[event.ID]; ok {
			for ft, delta := range adj {
				weights[ft] += delta
			}
		}
	}

	for ft := range weights {
		if weights[ft] < clampMin {
			weights[ft] = clampMin
		}
		if weights[ft] > clampMax {
			weights[ft] = clampMax
		}
	}

	e.normalizeWeights(weights)

	for ft := range weights {
		if weights[ft] < clampMin {
			weights[ft] = clampMin
		}
		if weights[ft] > clampMax {
			weights[ft] = clampMax
		}
	}

	e.normalizeWeights(weights)
	return weights
}

func (e *FactorWeightEngine) SetBaseWeights(weights map[FactorType]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.baseWeights = make(map[FactorType]float64)
	for k, v := range weights {
		e.baseWeights[k] = v
	}
}

func (e *FactorWeightEngine) normalizeWeights(weights map[FactorType]float64) {
	var total float64
	for _, w := range weights {
		total += w
	}
	if total <= 0 || math.Abs(total-1.0) < 0.001 {
		return
	}
	for ft := range weights {
		weights[ft] /= total
	}
}

func (e *FactorWeightEngine) OnRegimeChange(oldRegime, newRegime string, confidence float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	fw := fwConfig()
	var (
		riskOnMom  float64 = 0.05
		riskOnQual float64 = -0.03
		riskOffMom float64 = -0.05
		riskOffQ   float64 = 0.05
		riskOffLiq float64 = 0.03
	)
	if fw != nil {
		riskOnMom = fw.RiskOnMomentum.Value
		riskOnQual = fw.RiskOnQuality.Value
		riskOffMom = fw.RiskOffMomentum.Value
		riskOffQ = fw.RiskOffQuality.Value
		riskOffLiq = fw.RiskOffLiquidity.Value
	}

	if newRegime == "RISK_ON" {
		e.eventWeights["regime_risk_on"] = map[FactorType]float64{
			FactorMomentum: riskOnMom,
			FactorQuality:  riskOnQual,
		}
	} else if newRegime == "RISK_OFF" {
		e.eventWeights["regime_risk_off"] = map[FactorType]float64{
			FactorMomentum:  riskOffMom,
			FactorQuality:   riskOffQ,
			FactorLiquidity: riskOffLiq,
		}
	} else {
		delete(e.eventWeights, "regime_risk_on")
		delete(e.eventWeights, "regime_risk_off")
	}
}

func (e *FactorWeightEngine) AddEvent(event *narrative.NarrativeEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeEvents[event.ID] = event
	e.lifecycle.AddEvent(event)
	e.applyEventAdjustment(event)
}

func (e *FactorWeightEngine) applyEventAdjustment(event *narrative.NarrativeEvent) {
	fw := fwConfig()
	var (
		sevCritical float64 = 0.10
		sevHigh     float64 = 0.05
		sevMedium   float64 = 0.02
		sevLow      float64 = 0.01
	)
	if fw != nil {
		sevCritical = fw.SeverityCritical.Value
		sevHigh = fw.SeverityHigh.Value
		sevMedium = fw.SeverityMedium.Value
		sevLow = fw.SeverityLow.Value
	}

	var delta float64
	switch event.Severity {
	case "critical":
		delta = sevCritical
	case "high":
		delta = sevHigh
	case "medium":
		delta = sevMedium
	case "low":
		delta = sevLow
	default:
		delta = sevMedium
	}
	switch event.Theme {
	case "AI_capex_surge":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorQuality:  delta,
			FactorMomentum: delta,
		}
	case "US_rates_up":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorValue:    delta,
			FactorInstSent: -delta,
		}
	case "oil_price_shock":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorLiquidity: -delta,
			FactorMomentum:  -delta,
		}
	default:
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorInstSent: delta,
		}
	}
}

func (e *FactorWeightEngine) RemoveEvent(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.activeEvents, id)
	delete(e.eventWeights, id)
	e.lifecycle.ExpireEvent(id)
}

func (e *FactorWeightEngine) GetActiveEvents() []*narrative.NarrativeEvent {
	return e.lifecycle.GetActiveEvents()
}

func (e *FactorWeightEngine) Update() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lifecycle.UpdateStatuses()
	for id, event := range e.activeEvents {
		if event.Status == "faded" || event.Status == "expired" {
			delete(e.activeEvents, id)
		}
	}
}

func (e *FactorWeightEngine) ApplyStrategy(s *strategy.Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()

	fw := fwConfig()
	var (
		consValue    float64 = 0.05
		consQuality  float64 = 0.05
		consMomentum float64 = -0.05
		aggMomentum  float64 = 0.05
		aggInstSent  float64 = 0.03
		aggValue     float64 = -0.03
		aggQuality   float64 = -0.03
	)
	if fw != nil {
		consValue = fw.ConservativeValue.Value
		consQuality = fw.ConservativeQuality.Value
		consMomentum = fw.ConservativeMomentum.Value
		aggMomentum = fw.AggressiveMomentum.Value
		aggInstSent = fw.AggressiveInstSent.Value
		aggValue = fw.AggressiveValue.Value
		aggQuality = fw.AggressiveQuality.Value
	}

	strategyKey := "strategy_adjustment"
	delete(e.eventWeights, strategyKey)

	switch s.RiskAppetite {
	case strategy.RiskAppetiteConservative:
		e.eventWeights[strategyKey] = map[FactorType]float64{
			FactorValue:    consValue,
			FactorQuality:  consQuality,
			FactorMomentum: consMomentum,
		}
	case strategy.RiskAppetiteAggressive:
		e.eventWeights[strategyKey] = map[FactorType]float64{
			FactorMomentum: aggMomentum,
			FactorInstSent: aggInstSent,
			FactorValue:    aggValue,
			FactorQuality:  aggQuality,
		}
	default:
	}
}
