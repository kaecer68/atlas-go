package risk

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// PositionInfo is the minimal position data needed for concentration
// checks. Defined in internal/risk/ (not internal/portfolio/) to avoid
// a cross-layer import — callers (RiskManager, cmd/atlas/main.go) are
// responsible for converting their internal Position types to
// PositionInfo before invoking EvaluateAndPublish.
type PositionInfo struct {
	Symbol string
	Value  float64
}

// ConcentrationAlertEmitter publishes EventConcentrationBreach events
// when portfolio concentration metrics exceed configured thresholds.
// Three checks are performed per evaluation:
//
//  1. Per-position weight: for each position, weight = value/currentValue;
//     breach if weight > maxPositionWeightPct → severity "error"
//  2. Positions count: breach if len(positions) > maxPositionsCount
//     → severity "warning"
//  3. Sector concentration: for each (sector, exposure) pair, breach
//     if exposure > sectorConcentrationThresholdHigh → severity "error"
//
// The emitter holds a reference to the event bus; if bus is nil,
// EvaluateAndPublish is a no-op (safe for lazy DI / test scenarios).
// Thresholds are set at construction time and are immutable — the
// struct is safe for concurrent EvaluateAndPublish calls without
// locking because the fields are effectively final after construction.
//
// This is Decision 7 (alert-redesign-v2.md Part 3.5) — the publisher
// half of the Position / Sector Concentration alert. A future
// monitoring consumer will subscribe to EventConcentrationBreach and
// persist to AlertStore (mirroring the DrawdownConsumer pattern).
type ConcentrationAlertEmitter struct {
	bus *eventbus.ChannelEventBus

	maxPositionWeightPct             float64
	maxPositionsCount                int
	sectorConcentrationThresholdHigh float64
}

// NewConcentrationAlertEmitter creates a new emitter.
// nil bus → EvaluateAndPublish becomes a no-op.
func NewConcentrationAlertEmitter(
	bus *eventbus.ChannelEventBus,
	maxPositionWeightPct float64,
	maxPositionsCount int,
	sectorConcentrationThresholdHigh float64,
) *ConcentrationAlertEmitter {
	return &ConcentrationAlertEmitter{
		bus:                              bus,
		maxPositionWeightPct:             maxPositionWeightPct,
		maxPositionsCount:                maxPositionsCount,
		sectorConcentrationThresholdHigh: sectorConcentrationThresholdHigh,
	}
}

// EvaluateAndPublish checks all three concentration conditions and
// publishes EventConcentrationBreach for each breach. Safe with nil
// bus (no-op). Safe with empty/nil inputs (no breach emitted).
//
// The function does not return an error because eventbus.Publish is
// fire-and-forget per internal/eventbus/AGENTS.md — caller cannot
// observe dropped events. Logging happens inside the bus.
func (e *ConcentrationAlertEmitter) EvaluateAndPublish(
	positions []PositionInfo,
	currentValue float64,
	sectorExposure map[string]float64,
) {
	if e.bus == nil {
		return
	}
	now := time.Now()

	// Check 1: per-position weight
	if currentValue > 0 && e.maxPositionWeightPct > 0 {
		for _, p := range positions {
			weight := p.Value / currentValue
			if weight > e.maxPositionWeightPct {
				e.bus.PublishConcentrationBreach(eventbus.ConcentrationBreachPayload{
					Type:      "position",
					Symbol:    p.Symbol,
					Value:     weight,
					Threshold: e.maxPositionWeightPct,
					Timestamp: now,
				}, "error")
			}
		}
	}

	// Check 2: positions count
	if e.maxPositionsCount > 0 && len(positions) > e.maxPositionsCount {
		e.bus.PublishConcentrationBreach(eventbus.ConcentrationBreachPayload{
			Type:      "count",
			Value:     float64(len(positions)),
			Threshold: float64(e.maxPositionsCount),
			Timestamp: now,
		}, "warning")
	}

	// Check 3: sector concentration (sectorExposure is sector → weight 0..1)
	if e.sectorConcentrationThresholdHigh > 0 {
		for sector, exposure := range sectorExposure {
			if exposure > e.sectorConcentrationThresholdHigh {
				e.bus.PublishConcentrationBreach(eventbus.ConcentrationBreachPayload{
					Type:      "sector",
					Sector:    sector,
					Value:     exposure,
					Threshold: e.sectorConcentrationThresholdHigh,
					Timestamp: now,
				}, "error")
			}
		}
	}
}

// String returns a human-readable summary for logging.
func (e *ConcentrationAlertEmitter) String() string {
	return fmt.Sprintf("ConcentrationAlertEmitter{maxPositionWeight=%.2f, maxPositions=%d, sectorThresholdHigh=%.2f}",
		e.maxPositionWeightPct, e.maxPositionsCount, e.sectorConcentrationThresholdHigh)
}
