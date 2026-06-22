package service

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const (
	RegimeStabilityWindow    = 30 * time.Second
	RegimeDebounceCheckInt   = 5 * time.Second
	RegimeChangeConfirmedVer = 1
)

type RegimeDebouncer interface {
	Start(ctx context.Context) error
	Stop() error
}

type regimeDebouncer struct {
	bus eventbus.EventBus

	mu             sync.Mutex
	pending        eventbus.RegimeEventPayload
	pendingSince   time.Time
	hasPending     bool
	lastEmittedNew string

	sub    eventbus.Subscription
	cancel context.CancelFunc
	done   chan struct{}
}

func NewRegimeDebouncer(bus eventbus.EventBus) RegimeDebouncer {
	return &regimeDebouncer{
		bus:  bus,
		done: make(chan struct{}),
	}
}

func (d *regimeDebouncer) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.sub = d.bus.Subscribe(eventbus.EventRegimeChange, d.onRegimeChange)
	go d.run(runCtx)
	return nil
}

func (d *regimeDebouncer) Stop() error {
	if d.sub.Cancel != nil {
		d.sub.Cancel()
	}
	if d.cancel != nil {
		d.cancel()
	}
	<-d.done
	return nil
}

func (d *regimeDebouncer) onRegimeChange(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.RegimeEventPayload)
	if !ok {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = payload
	d.pendingSince = ev.Timestamp
	if d.pendingSince.IsZero() {
		d.pendingSince = time.Now()
	}
	d.hasPending = true
	return nil
}

func (d *regimeDebouncer) run(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(RegimeDebounceCheckInt)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			d.check(t)
		}
	}
}

func (d *regimeDebouncer) check(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.hasPending {
		return
	}
	newRegime := string(d.pending.NewRegime)
	if newRegime == d.lastEmittedNew {
		return
	}
	if now.Sub(d.pendingSince) < RegimeStabilityWindow {
		return
	}

	stabilitySeconds := int(now.Sub(d.pendingSince).Seconds())
	oldRegime := string(d.pending.OldRegime)

	d.bus.Publish(eventbus.BusEvent{
		Type:          eventbus.EventRegimeChangeConfirmed,
		Timestamp:     now,
		Severity:      "info",
		SchemaVersion: RegimeChangeConfirmedVer,
		Payload: map[string]any{
			"old_regime":        oldRegime,
			"new_regime":        newRegime,
			"confirmed_at":      now,
			"stability_seconds": stabilitySeconds,
			"determined_by":     d.pending.DeterminedBy,
			"confidence":        d.pending.Confidence,
		},
	})
	d.lastEmittedNew = newRegime
}
