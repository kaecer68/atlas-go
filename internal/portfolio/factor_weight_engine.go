package portfolio

import (
	"sync"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

type FactorWeightEngine struct {
	mu           sync.RWMutex
	baseWeights  map[FactorType]float64
	eventWeights map[string]map[FactorType]float64
	activeEvents map[string]*narrative.NarrativeEvent
	lifecycle    *narrative.EventLifecycleManager
}

func NewFactorWeightEngine() *FactorWeightEngine {
	return &FactorWeightEngine{
		baseWeights: map[FactorType]float64{
			FactorMomentum:  0.20,
			FactorValue:     0.15,
			FactorQuality:   0.15,
			FactorAgent:     0.20,
			FactorInstSent:  0.15,
			FactorLiquidity: 0.15,
		},
		eventWeights: make(map[string]map[FactorType]float64),
		activeEvents: make(map[string]*narrative.NarrativeEvent),
		lifecycle:    narrative.NewEventLifecycleManager(),
	}
}

func (e *FactorWeightEngine) GetWeights(regime string) map[FactorType]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	weights := make(map[FactorType]float64)
	for k, v := range e.baseWeights {
		weights[k] = v
	}
	for _, event := range e.activeEvents {
		if adj, ok := e.eventWeights[event.Theme]; ok {
			for ft, delta := range adj {
				weights[ft] += delta
			}
		}
	}
	for ft := range weights {
		if weights[ft] < 0.05 {
			weights[ft] = 0.05
		}
		if weights[ft] > 0.50 {
			weights[ft] = 0.50
		}
	}
	return weights
}

func (e *FactorWeightEngine) OnRegimeChange(oldRegime, newRegime string, confidence float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if newRegime == "RISK_ON" {
		e.eventWeights["regime_risk_on"] = map[FactorType]float64{
			FactorMomentum: 0.05,
			FactorQuality:  -0.03,
		}
	} else if newRegime == "RISK_OFF" {
		e.eventWeights["regime_risk_off"] = map[FactorType]float64{
			FactorMomentum:  -0.05,
			FactorQuality:   0.05,
			FactorLiquidity: 0.03,
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
	var delta float64
	switch event.Severity {
	case "critical":
		delta = 0.10
	case "high":
		delta = 0.05
	case "medium":
		delta = 0.02
	case "low":
		delta = 0.01
	default:
		delta = 0.02
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

	strategyKey := "strategy_adjustment"
	delete(e.eventWeights, strategyKey)

	switch s.RiskAppetite {
	case strategy.RiskAppetiteConservative:
		e.eventWeights[strategyKey] = map[FactorType]float64{
			FactorValue:    0.05,
			FactorQuality:  0.05,
			FactorMomentum: -0.05,
		}
	case strategy.RiskAppetiteAggressive:
		e.eventWeights[strategyKey] = map[FactorType]float64{
			FactorMomentum: 0.05,
			FactorInstSent: 0.03,
			FactorValue:    -0.03,
			FactorQuality:  -0.03,
		}
	default:
	}
}
