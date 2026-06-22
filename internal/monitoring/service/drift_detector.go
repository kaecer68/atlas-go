package service

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const (
	DriftMaxConcentrationThreshold = 0.25
	DriftTurnoverThreshold         = 0.15
	DriftCheckInterval             = 5 * time.Minute
	DriftEventSchemaVer            = 1
)

type DriftDetector interface {
	Start(ctx context.Context) error
	Stop() error
}

type driftSnapshot struct {
	value     float64
	updatedAt time.Time
}

type driftDetector struct {
	bus eventbus.EventBus

	mu          sync.Mutex
	snapshots   map[string]*driftSnapshot
	prevTotal   float64
	periodStart time.Time

	sub    eventbus.Subscription
	cancel context.CancelFunc
	done   chan struct{}
}

func NewDriftDetector(bus eventbus.EventBus) DriftDetector {
	return &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
		done:      make(chan struct{}),
	}
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

	if maxWeight <= DriftMaxConcentrationThreshold && turnover <= DriftTurnoverThreshold {
		d.prevTotal = total
		d.periodStart = now
		return
	}

	reasons := []string{}
	if maxWeight > DriftMaxConcentrationThreshold {
		reasons = append(reasons, "concentration")
	}
	if turnover > DriftTurnoverThreshold {
		reasons = append(reasons, "turnover")
	}

	d.bus.Publish(eventbus.BusEvent{
		Type:          eventbus.EventDriftDetected,
		Timestamp:     now,
		Severity:      "info",
		SchemaVersion: DriftEventSchemaVer,
		Payload: map[string]any{
			"max_concentration": maxWeight,
			"max_symbol":        maxSymbol,
			"turnover":          turnover,
			"total_value":       total,
			"period_start":      d.periodStart,
			"reasons":           reasons,
			"thresholds": map[string]float64{
				"concentration": DriftMaxConcentrationThreshold,
				"turnover":      DriftTurnoverThreshold,
			},
		},
	})
	d.prevTotal = total
	d.periodStart = now
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
