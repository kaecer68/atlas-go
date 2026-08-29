// Maturity: experimental
package industry

import (
	"maps"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// CycleOutcome records one data point for cycle compass layer accuracy tracking.
// Each outcome stores the raw layer signals from a CycleStatusCard alongside
// the actual market return to evaluate which layers were directionally correct.
type CycleOutcome struct {
	SessionID    string             `json:"session_id"`
	Date         time.Time          `json:"date"`
	LayerSignals map[string]float64 `json:"layer_signals"`
	ActualReturn float64            `json:"actual_return"`
}

// LayerMetrics holds per-layer accuracy statistics aggregated over the rolling window.
type LayerMetrics struct {
	TotalSignals   int       `json:"total_signals"`
	CorrectSignals int       `json:"correct_signals"`
	Accuracy       float64   `json:"accuracy"`
	LastUpdated    time.Time `json:"last_updated"`
}

// CycleCalibration tracks per-layer accuracy over a rolling window and adjusts
// the CardConfig layer weights based on observed hit rates. Layers with hit rate
// above the configured threshold are upweighted; layers below are downweighted.
//
// Thread-safe via internal mutex.
type CycleCalibration struct {
	mu       sync.RWMutex
	outcomes []CycleOutcome
	metrics  map[string]*LayerMetrics
	config   config.CycleCalibrationConfig
}

// NewCycleCalibration creates a calibration tracker with the given config.
func NewCycleCalibration(cfg config.CycleCalibrationConfig) *CycleCalibration {
	return &CycleCalibration{
		outcomes: make([]CycleOutcome, 0, cfg.WindowSize),
		metrics:  make(map[string]*LayerMetrics),
		config:   cfg,
	}
}

// RecordOutcome stores a data point and updates per-layer accuracy metrics.
// If the rolling window exceeds WindowSize, the oldest data point is dropped.
func (c *CycleCalibration) RecordOutcome(sessionID string, date time.Time, layerSignals map[string]float64, actualReturn float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	outcome := CycleOutcome{
		SessionID:    sessionID,
		Date:         date,
		LayerSignals: layerSignals,
		ActualReturn: actualReturn,
	}

	c.outcomes = append(c.outcomes, outcome)
	if len(c.outcomes) > c.config.WindowSize {
		c.outcomes = c.outcomes[len(c.outcomes)-c.config.WindowSize:]
	}

	c.recalculateMetrics()
}

// CalibrateWeights adjusts layer weights based on tracked accuracy.
// For each layer with at least MinSamples observations:
//   - Hit rate > HitRateHigh: increase weight by LearningRate (clamped to WeightClampMax)
//   - Hit rate < HitRateLow: decrease weight by LearningRate (clamped to WeightClampMin)
//
// Weights are normalized to sum to 1.0 after adjustment. The original baseWeights
// map is NOT mutated; a new calibrated map is returned.
func (c *CycleCalibration) CalibrateWeights(baseWeights map[string]float64) map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	calibrated := make(map[string]float64, len(baseWeights))
	maps.Copy(calibrated, baseWeights)

	if len(c.outcomes) < c.config.MinSamples {
		return calibrated
	}

	for layer, m := range c.metrics {
		if m.TotalSignals < c.config.MinSamples {
			continue
		}

		current, ok := calibrated[layer]
		if !ok {
			continue
		}

		switch {
		case m.Accuracy > c.config.HitRateHigh:
			current += c.config.LearningRate
		case m.Accuracy < c.config.HitRateLow:
			current -= c.config.LearningRate
		}

		current = clampFloat(current, c.config.WeightClampMin, c.config.WeightClampMax)
		calibrated[layer] = current
	}

	return normalizeWeights(calibrated)
}

// GetMetrics returns a snapshot of per-layer accuracy metrics.
func (c *CycleCalibration) GetMetrics() map[string]LayerMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]LayerMetrics, len(c.metrics))
	for layer, m := range c.metrics {
		result[layer] = *m
	}
	return result
}

// GetOutcomeCount returns the number of outcomes currently in the window.
func (c *CycleCalibration) GetOutcomeCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.outcomes)
}

// SetConfig updates the calibration parameters.
func (c *CycleCalibration) SetConfig(cfg config.CycleCalibrationConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = cfg
}

// recalculateMetrics rebuilds all layer metrics from the current outcomes window.
// Caller must hold c.mu (write lock).
func (c *CycleCalibration) recalculateMetrics() {
	layers := c.collectLayers()
	c.metrics = make(map[string]*LayerMetrics, len(layers))

	for _, layer := range layers {
		c.metrics[layer] = &LayerMetrics{}
	}

	for _, o := range c.outcomes {
		for layer := range c.metrics {
			signal, ok := o.LayerSignals[layer]
			if !ok {
				continue
			}

			correct := layerSignalMatchesReturn(layer, signal, o.ActualReturn)
			lm := c.metrics[layer]
			lm.TotalSignals++
			if correct {
				lm.CorrectSignals++
			}
		}
	}

	now := time.Now()
	for _, lm := range c.metrics {
		if lm.TotalSignals > 0 {
			lm.Accuracy = float64(lm.CorrectSignals) / float64(lm.TotalSignals)
		}
		lm.LastUpdated = now
	}
}

// collectLayers gathers all layer names across all outcomes.
// Caller must hold c.mu.
func (c *CycleCalibration) collectLayers() []string {
	seen := make(map[string]bool)
	for _, o := range c.outcomes {
		for layer := range o.LayerSignals {
			seen[layer] = true
		}
	}
	layers := make([]string, 0, len(seen))
	for layer := range seen {
		layers = append(layers, layer)
	}
	return layers
}

// layerSignalMatchesReturn determines whether a layer's signal direction
// matched the actual market return direction. The neutral threshold differs
// by layer type: silicon/business_cycle use 0.5, seasonal/events use 1.0,
// and supply_chain uses 0.
func layerSignalMatchesReturn(layer string, signal, actualReturn float64) bool {
	if math.Abs(actualReturn) < 1e-10 {
		return false
	}

	var bullishSignal bool
	switch layer {
	case "silicon", "business_cycle":
		bullishSignal = signal > 0.5
	case "seasonal", "events":
		bullishSignal = signal > 1.0
	case "supply_chain":
		bullishSignal = signal > 0
	default:
		bullishSignal = signal > 0.5
	}

	marketBullish := actualReturn > 0
	return bullishSignal == marketBullish
}

// normalizeWeights scales weights to sum to 1.0. Returns the input map unmodified
// if all weights are zero.
func normalizeWeights(weights map[string]float64) map[string]float64 {
	var sum float64
	for _, w := range weights {
		sum += w
	}
	if sum == 0 {
		return weights
	}
	result := make(map[string]float64, len(weights))
	for layer, w := range weights {
		result[layer] = math.Round(w/sum*10000) / 10000
	}
	return result
}

// clampFloat bounds v to [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
