package service

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const (
	FactorWeightRegressionThreshold = 0.5
	FactorWeightRegressionVersion   = 1
)

type FactorWeightRegressionDetector interface {
	Start(ctx context.Context) error
	Stop() error
}

type factorWeightRegressionDetector struct {
	bus      eventbus.EventBus
	provider WeightProvider

	mu          sync.Mutex
	prevWeights map[string]float64
	hasPrev     bool

	sub eventbus.Subscription
}

func NewFactorWeightRegressionDetector(bus eventbus.EventBus, provider WeightProvider) FactorWeightRegressionDetector {
	return &factorWeightRegressionDetector{
		bus:      bus,
		provider: provider,
	}
}

func (d *factorWeightRegressionDetector) Start(_ context.Context) error {
	d.sub = d.bus.Subscribe(eventbus.EventRegimeChange, d.onRegimeChange)
	return nil
}

func (d *factorWeightRegressionDetector) Stop() error {
	if d.sub.Cancel != nil {
		d.sub.Cancel()
	}
	return nil
}

func (d *factorWeightRegressionDetector) onRegimeChange(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.RegimeEventPayload)
	if !ok {
		return nil
	}
	if d.provider == nil {
		return nil
	}

	newRegime := string(payload.NewRegime)
	currentWeights := d.provider.GetWeights(newRegime)
	if currentWeights == nil {
		return nil
	}

	d.mu.Lock()
	prev := d.prevWeights
	hasPrev := d.hasPrev
	d.prevWeights = currentWeights
	d.hasPrev = true
	d.mu.Unlock()

	if !hasPrev {
		return nil
	}

	score := regressionScore(prev, currentWeights)
	if score < FactorWeightRegressionThreshold {
		return nil
	}

	d.bus.Publish(eventbus.BusEvent{
		Type:          eventbus.EventFactorWeightRegression,
		Timestamp:     time.Now(),
		Severity:      "info",
		SchemaVersion: FactorWeightRegressionVersion,
		Payload: map[string]any{
			"regime":           newRegime,
			"factor_diffs":     diffMap(prev, currentWeights),
			"regression_score": score,
			"threshold":        FactorWeightRegressionThreshold,
		},
	})
	return nil
}

func regressionScore(prev, curr map[string]float64) float64 {
	var sum float64
	for k, v := range curr {
		diff := math.Abs(v - prev[k])
		sum += diff
	}
	for k, v := range prev {
		if _, ok := curr[k]; !ok {
			sum += math.Abs(v)
		}
	}
	return sum
}

func diffMap(prev, curr map[string]float64) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range curr {
		d := v - prev[k]
		if d != 0 {
			out[k] = d
		}
	}
	return out
}
