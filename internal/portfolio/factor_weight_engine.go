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
	mu                 sync.RWMutex
	baseWeights        map[FactorType]float64
	eventWeights       map[string]map[FactorType]float64
	activeEvents       map[string]*narrative.NarrativeEvent
	lifecycle          *narrative.EventLifecycleManager
	strategyAdjustment map[FactorType]float64
	weightSource       string
	currentRegime      string

	taxonomyAdjust map[narrative.TaxonomyL1]map[narrative.TaxonomyL2]map[FactorType]float64
}

func (e *FactorWeightEngine) WeightSource() string {
	return e.weightSource
}

func (e *FactorWeightEngine) SetRegime(r string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.currentRegime = r
}

func fwConfig() *config.FactorWeightParameters {
	if cfg := config.GetParametersConfig(); cfg != nil {
		return &cfg.FactorWeight
	}
	return nil
}

func defaultBaseWeights() map[FactorType]float64 {
	return map[FactorType]float64{
		FactorMomentum:       0.25,
		FactorValue:          0.20,
		FactorQuality:        0.20,
		FactorAgent:          0.15,
		FactorInstSent:       0.10,
		FactorLiquidity:      0.05,
		FactorNarrative:      0.05,
		FactorIndustryCycle:  0.00,
		FactorPreciousMetals: 0.00,
		FactorETF:            0.00,
		FactorLinkage:        0.00,
		FactorTSMC:           0.05,
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
		baseWeights:        baseWeights,
		eventWeights:       make(map[string]map[FactorType]float64),
		activeEvents:       make(map[string]*narrative.NarrativeEvent),
		lifecycle:          narrative.NewEventLifecycleManager(),
		strategyAdjustment: make(map[FactorType]float64),
		weightSource:       source,
	}
}

func (e *FactorWeightEngine) GetWeights(regime string) map[FactorType]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	weights := make(map[FactorType]float64)
	// Merge stored weights with latest config (enables real-time sync from calibrator).
	maps.Copy(weights, e.baseWeights)
	if fw := fwConfig(); fw != nil && fw.BaseWeights.Value != nil {
		for k, v := range fw.BaseWeights.Value {
			weights[FactorType(k)] = v
		}
	}

	// Read clamp bounds from config with hardcoded fallback
	fw := fwConfig()
	var (
		clampMin = 0.02
		clampMax = 0.50
	)
	if fw != nil {
		clampMin = fw.ClampMin.Value
		clampMax = fw.ClampMax.Value
	}

	// Scale Narrative factor weight by active event intensity.
	if intensity := e.narrativeIntensity(); intensity > 0 {
		delta := intensity * 0.05
		if delta > 0.1 {
			delta = 0.1
		}
		weights[FactorNarrative] += delta
	}

	for _, event := range e.activeEvents {
		if adj, ok := e.eventWeights[event.ID]; ok {
			for ft, delta := range adj {
				weights[ft] += delta
			}
		}
	}
	// Apply regime event weights (keyed by "regime_*", not activeEvent IDs).
	for ft, delta := range e.eventWeights["regime_risk_on"] {
		weights[ft] += delta
	}
	for ft, delta := range e.eventWeights["regime_risk_off"] {
		weights[ft] += delta
	}
	for ft, delta := range e.eventWeights["regime_high_vol"] {
		weights[ft] += delta
	}

	for ft, delta := range e.strategyAdjustment {
		weights[ft] += delta
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
	maps.Copy(e.baseWeights, weights)
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
		riskOnMom   = 0.05
		riskOnQual  = -0.03
		riskOnVal   = -0.02
		riskOffMom  = -0.05
		riskOffQ    = 0.05
		riskOffVal  = 0.03
		riskOffLiq  = 0.03
		riskOffETF  = 0.04
		riskOffIC   = 0.04
		highVolLiq  = 0.05
		highVolMom  = -0.03
		highVolInst = -0.02
		highVolIC   = 0.03
	)
	if fw != nil {
		riskOnMom = fw.RiskOnMomentum.Value
		riskOnQual = fw.RiskOnQuality.Value
		riskOffMom = fw.RiskOffMomentum.Value
		riskOffQ = fw.RiskOffQuality.Value
		riskOffLiq = fw.RiskOffLiquidity.Value
	}

	switch newRegime {
	case "RISK_ON":
		e.eventWeights["regime_risk_on"] = map[FactorType]float64{
			FactorMomentum: riskOnMom,
			FactorQuality:  riskOnQual,
			FactorValue:    riskOnVal,
		}
	case "RISK_OFF":
		e.eventWeights["regime_risk_off"] = map[FactorType]float64{
			FactorMomentum:      riskOffMom,
			FactorQuality:       riskOffQ,
			FactorValue:         riskOffVal,
			FactorLiquidity:     riskOffLiq,
			FactorETF:           riskOffETF,
			FactorIndustryCycle: riskOffIC,
		}
	case "high_vol":
		e.eventWeights["regime_high_vol"] = map[FactorType]float64{
			FactorMomentum:      highVolMom,
			FactorLiquidity:     highVolLiq,
			FactorInstSent:      highVolInst,
			FactorIndustryCycle: highVolIC,
		}
	default:
		delete(e.eventWeights, "regime_risk_on")
		delete(e.eventWeights, "regime_risk_off")
		delete(e.eventWeights, "regime_high_vol")
	}
}

func (e *FactorWeightEngine) AddEvent(event *narrative.NarrativeEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeEvents[event.ID] = event
	e.lifecycle.AddEvent(event)

	event.NormalizeTaxonomy()
	if !e.applyTaxonomyAdjustment(event) {
		e.applyEventAdjustment(event)
	}
}

func (e *FactorWeightEngine) applyEventAdjustment(event *narrative.NarrativeEvent) {
	fw := fwConfig()
	var (
		sevCritical = 0.10
		sevHigh     = 0.05
		sevMedium   = 0.02
		sevLow      = 0.01
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
			FactorQuality:   delta,
			FactorMomentum:  delta,
			FactorETF:       delta,
			FactorNarrative: delta,
		}
	case "US_rates_up":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorValue:     delta,
			FactorInstSent:  -delta,
			FactorETF:       delta,
			FactorNarrative: delta,
		}
	case "oil_price_shock":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorLiquidity:     -delta,
			FactorMomentum:      -delta,
			FactorETF:           delta,
			FactorNarrative:     delta,
			FactorIndustryCycle: delta,
		}
	case "JPY_carry_unwind":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorLiquidity: -delta,
			FactorAgent:     -delta,
			FactorETF:       delta,
			FactorNarrative: delta,
		}
	case "gold_rally":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorPreciousMetals: delta,
			FactorValue:          -delta,
			FactorNarrative:      delta,
		}
	case "dollar_surge":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorPreciousMetals: -delta,
			FactorLiquidity:      delta,
			FactorNarrative:      delta,
		}
	case "inflation_spike":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorPreciousMetals: delta,
			FactorMomentum:       -delta,
			FactorNarrative:      delta,
		}
	case "geopolitical_risk_spike":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorAgent:     -delta,
			FactorNarrative: delta * 1.5,
			FactorLiquidity: -delta,
		}
	case "retail_institutional_divergence":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorInstSent:  delta,
			FactorAgent:     -delta,
			FactorNarrative: delta,
		}
	case "taiwan_political_risk":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorETF: -delta, FactorValue: delta, FactorQuality: delta * 0.5, FactorAgent: -delta,
			FactorNarrative: delta * 1.5, FactorInstSent: delta, FactorLiquidity: -delta * 0.5,
		}
	case "election_cycle":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorMomentum: -delta * 0.5, FactorETF: -delta * 0.5, FactorInstSent: delta * 0.5,
			FactorNarrative: delta * 0.5, FactorIndustryCycle: delta * 0.5,
		}
	case "spring_festival_season":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorLiquidity: delta * 0.5, FactorValue: delta * 0.3,
			FactorETF: delta * 0.3, FactorNarrative: delta * 0.3,
		}
	case "USD_TWD_volatility":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorMomentum: delta, FactorLiquidity: delta, FactorNarrative: delta,
		}
	case "tariff_shock":
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorMomentum:       -delta,
			FactorQuality:        -delta,
			FactorNarrative:      delta * 1.5,
			FactorPreciousMetals: delta,
			FactorETF:            -delta,
			FactorAgent:          -delta,
		}
	default:
		e.eventWeights[event.ID] = map[FactorType]float64{
			FactorInstSent:  delta,
			FactorNarrative: delta * 0.5,
		}
	}
}

func (e *FactorWeightEngine) applyTaxonomyAdjustment(event *narrative.NarrativeEvent) bool {
	if e.taxonomyAdjust == nil {
		return false
	}
	l2Map, ok := e.taxonomyAdjust[event.TaxonomyL1]
	if !ok {
		return false
	}
	deltaMap, ok := l2Map[event.TaxonomyL2]
	if !ok || len(deltaMap) == 0 {
		return false
	}
	fw := fwConfig()
	sevCritical, sevHigh, sevMedium, sevLow := 0.10, 0.05, 0.02, 0.01
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
	weights := make(map[FactorType]float64, len(deltaMap))
	for ft, factorDelta := range deltaMap {
		weights[ft] = delta * factorDelta
	}
	e.eventWeights[event.ID] = weights
	return true
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

// narrativeIntensity returns a composite intensity score from active events.
// Formula: event_count × avg_confidence × avg_hit_rate, capped at 1.0.
func (e *FactorWeightEngine) narrativeIntensity() float64 {
	events := e.lifecycle.GetActiveEvents()
	if len(events) == 0 {
		return 0
	}
	var totalConf, totalHit float64
	for _, ev := range events {
		totalConf += ev.Confidence
		totalHit += ev.HitRate
	}
	n := float64(len(events))
	return n * (totalConf / n) * (totalHit / n)
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

// strategyDeltas returns the factor adjustment map for a given risk appetite.
// Conservative: +Value, +Quality, −Momentum
// Aggressive:   +Momentum, +InstSent, −Value, −Quality
// Balanced:     no adjustment (empty map).
func (e *FactorWeightEngine) strategyDeltas(ra strategy.RiskAppetite) map[FactorType]float64 {
	fw := fwConfig()
	var (
		consValue    = 0.05
		consQuality  = 0.05
		consMomentum = -0.05
		aggMomentum  = 0.05
		aggInstSent  = 0.03
		aggValue     = -0.03
		aggQuality   = -0.03
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

	switch ra {
	case strategy.RiskAppetiteConservative:
		return map[FactorType]float64{
			FactorValue:    consValue,
			FactorQuality:  consQuality,
			FactorMomentum: consMomentum,
		}
	case strategy.RiskAppetiteAggressive:
		return map[FactorType]float64{
			FactorMomentum: aggMomentum,
			FactorInstSent: aggInstSent,
			FactorValue:    aggValue,
			FactorQuality:  aggQuality,
		}
	default:
		return nil
	}
}

func (e *FactorWeightEngine) ApplyStrategy(s *strategy.Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()

	deltas := e.strategyDeltas(s.RiskAppetite)
	e.strategyAdjustment = make(map[FactorType]float64)
	if deltas == nil {
		return
	}
	maps.Copy(e.strategyAdjustment, deltas)
}

// ApplyStrategyMix computes a weighted-average factor adjustment from multiple
// strategies and stores the result. It uses the strategy registry to look up
// each strategy's RiskAppetite, then blends their per-factor deltas according
// to the mix weights.
//
//	blendedDelta[f] = Σ (mix[s] × delta(s, f)) / Σ mix[s]
func (e *FactorWeightEngine) ApplyStrategyMix(mix strategy.StrategyMix, registry *strategy.Registry) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(mix) == 0 {
		e.strategyAdjustment = make(map[FactorType]float64)
		return
	}

	// Sum weights for normalization (StrategyMix should sum to ~1.0,
	// but normalize defensively in case it doesn't).
	var totalWeight float64
	for _, w := range mix {
		totalWeight += w
	}
	if totalWeight <= 0 {
		e.strategyAdjustment = make(map[FactorType]float64)
		return
	}

	// Accumulate weighted deltas across all strategies.
	acc := make(map[FactorType]float64)
	for strategyID, weight := range mix {
		s, ok := registry.Get(strategyID)
		if !ok {
			continue
		}
		deltas := e.strategyDeltas(s.RiskAppetite)
		for ft, d := range deltas {
			acc[ft] += weight * d
		}
	}

	// Normalize by total weight.
	e.strategyAdjustment = make(map[FactorType]float64)
	for ft, d := range acc {
		e.strategyAdjustment[ft] = d / totalWeight
	}
}
