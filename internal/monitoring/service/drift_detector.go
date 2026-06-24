package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type driftDetector struct {
	bus eventbus.EventBus

	mu          sync.Mutex
	snapshots   map[string]*driftSnapshot
	prevTotal   float64
	periodStart time.Time

	sub eventbus.Subscription
	//lint:ignore U1000 regimeSub will be wired when EventRegimeChangeConfirmed subscription is implemented in Wave 4.
	regimeSub eventbus.Subscription // subscribes to EventRegimeChangeConfirmed
	cancel    context.CancelFunc
	done      chan struct{}

	provider      TargetWeightsProvider // nil-safe target weight provider
	currentRegime string                // regime snapshot from EventRegimeChangeConfirmed
}

func NewDriftDetector(bus eventbus.EventBus) DriftDetector {
	return &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
		done:      make(chan struct{}),
	}
}

func NewDriftDetectorWithTargets(bus eventbus.EventBus, provider TargetWeightsProvider) DriftDetector {
	return &driftDetector{
		bus:       bus,
		provider:  provider,
		snapshots: make(map[string]*driftSnapshot),
		done:      make(chan struct{}),
	}
}

// onRegimeChangeConfirmed updates currentRegime and re-baselines prevTotal.
// Implementation in Wave 4 — for now this is an empty stub.
func (d *driftDetector) onRegimeChangeConfirmed(_ context.Context, _ eventbus.BusEvent) error {
	return nil
}

func (d *driftDetector) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.mu.Lock()
	d.periodStart = time.Now()
	d.mu.Unlock()
	d.sub = d.bus.Subscribe(eventbus.EventPositionUpdate, d.onPositionUpdate)
	go d.run(runCtx)
	return nil
}

func (d *driftDetector) Stop() error {
	if d.sub.Cancel != nil {
		d.sub.Cancel()
	}
	if d.cancel != nil {
		d.cancel()
	}
	<-d.done
	return nil
}

func (d *driftDetector) run(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(DriftCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			d.checkPeriod(t)
		}
	}
}

func (d *driftDetector) onPositionUpdate(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.PositionEventPayload)
	if !ok {
		return nil
	}
	symbol := payload.Symbol
	value := payload.Position.MarketValue
	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if payload.ChangeType == "removed" {
		delete(d.snapshots, symbol)
		return nil
	}
	d.snapshots[symbol] = &driftSnapshot{value: value, updatedAt: now}
	return nil
}

func (d *driftDetector) totalValue() float64 {
	var total float64
	for _, s := range d.snapshots {
		total += s.value
	}
	return total
}

func (d *driftDetector) checkPeriod(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	total := d.totalValue()
	if total <= 0 {
		d.prevTotal = total
		d.periodStart = now
		return
	}

	if d.prevTotal == 0 {
		d.prevTotal = total
		d.periodStart = now
		return
	}

	var maxSymbol string
	var maxWeight float64
	for sym, s := range d.snapshots {
		w := s.value / total
		if w > maxWeight {
			maxWeight = w
			maxSymbol = sym
		}
	}

	var turnover float64
	if d.prevTotal > 0 {
		turnover = absDiff(total, d.prevTotal) / d.prevTotal
	}

	var (
		targetDriftChecked = false
		targetWeights      map[string]float64
		actualWeights      = make(map[string]float64, len(d.snapshots))
		maxDrift           float64
		maxDriftSymbol     string
	)
	if d.provider != nil {
		targetWeights = d.provider.GetTargetWeights(d.currentRegime)
		if len(targetWeights) > 0 {
			targetDriftChecked = true
			symbols := make([]string, 0, len(d.snapshots))
			for sym := range d.snapshots {
				symbols = append(symbols, sym)
			}
			sort.Strings(symbols)
			for _, sym := range symbols {
				s := d.snapshots[sym]
				actual := s.value / total
				actualWeights[sym] = actual
				target := targetWeights[sym]
				drift := absDiff(actual, target)
				if drift > maxDrift {
					maxDrift = drift
					maxDriftSymbol = sym
				}
			}
		}
	}

	hasConcentration := maxWeight > DriftMaxConcentrationThreshold
	hasTurnover := turnover > DriftTurnoverThreshold
	hasTargetDrift := targetDriftChecked && maxDrift > DriftTargetWeightThreshold

	if !hasConcentration && !hasTurnover && !hasTargetDrift {
		d.prevTotal = total
		d.periodStart = now
		return
	}

	reasons := []string{}
	if hasConcentration {
		reasons = append(reasons, ReasonConcentration)
	}
	if hasTurnover {
		reasons = append(reasons, ReasonTurnover)
	}
	if hasTargetDrift {
		reasons = append(reasons, ReasonTargetDrift)
	}

	payload := map[string]any{
		"max_concentration": maxWeight,
		"max_symbol":        maxSymbol,
		"turnover":          turnover,
		"total_value":       total,
		"period_start":      d.periodStart,
		"reasons":           reasons,
		"thresholds": map[string]float64{
			"concentration": DriftMaxConcentrationThreshold,
			"turnover":      DriftTurnoverThreshold,
			"target_drift":  DriftTargetWeightThreshold,
		},
	}

	if targetDriftChecked {
		payload["target_weights"] = targetWeights
		payload["actual_weights"] = actualWeights
		payload["max_drift"] = maxDrift
		payload["max_drift_symbol"] = maxDriftSymbol
		payload["current_regime"] = d.currentRegime
	}

	d.bus.Publish(eventbus.BusEvent{
		Type:          eventbus.EventDriftDetected,
		Timestamp:     now,
		Severity:      "info",
		SchemaVersion: DriftEventSchemaVer,
		Payload:       payload,
	})
	d.prevTotal = total
	d.periodStart = now
}
