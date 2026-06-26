package monitoring

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// DrawdownConsumer subscribes to EventDrawdownBreach and persists each
// event to the AlertStore as an AlertRecord. This is the downstream half
// of Decision 8 (alert-redesign-v2.md Part 3.6 PR-B) — it completes the
// loop started by RiskManager.publishDrawdownBreach (PR-A) so the
// portfolio drawdown alert becomes visible on the dashboard.
type DrawdownConsumer struct {
	bus   eventbus.EventBus
	store *AlertStore

	mu      sync.Mutex
	started bool
	sub     eventbus.Subscription
}

// NewDrawdownConsumer creates a new consumer.
// nil bus is allowed (Start becomes a no-op; consumer can still be Stopped).
// nil store is allowed (events are consumed but not persisted — useful for
// test scenarios that only need to verify the subscription wiring).
func NewDrawdownConsumer(bus eventbus.EventBus, store *AlertStore) *DrawdownConsumer {
	return &DrawdownConsumer{
		bus:   bus,
		store: store,
	}
}

// Start subscribes to EventDrawdownBreach. Idempotent: subsequent calls
// without a Stop in between are no-ops (avoids duplicate subscriptions).
// `started` is used as a gate because eventbus.Subscription is a struct
// (not interface / pointer) and cannot be compared to untyped nil.
func (c *DrawdownConsumer) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bus == nil || c.started {
		return
	}
	c.sub = c.bus.Subscribe(eventbus.EventDrawdownBreach, c.handleEvent)
	c.started = true
}

// Stop cancels the subscription. Idempotent: safe to call multiple times.
func (c *DrawdownConsumer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return
	}
	c.sub.Cancel()
	c.started = false
}

// handleEvent converts EventDrawdownBreach to a domain.AlertRecord and
// persists via AlertStore.Save. Payload type mismatch is logged and
// skipped (the eventbus AGENTS.md note about handler panic recovery
// applies here — we never let a bad payload crash the dispatcher).
func (c *DrawdownConsumer) handleEvent(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.DrawdownBreachPayload)
	if !ok {
		logging.Warn("drawdown_consumer", "payload_type_assertion_failed",
			logging.FStr("event_type", string(ev.Type)),
			logging.FStr("expected", "eventbus.DrawdownBreachPayload"),
			logging.FStr("actual_type", fmt.Sprintf("%T", ev.Payload)))
		return nil
	}
	if c.store == nil {
		return nil
	}
	record := c.buildAlertRecord(payload)
	if err := c.store.Save(record); err != nil {
		logging.Warn("drawdown_consumer", "alert_save_failed", logging.Err(err))
		return err
	}
	return nil
}

// buildAlertRecord converts the event payload to a domain AlertRecord.
// Field mapping is pinned by TestDrawdownConsumer_BuildAlertRecord_FieldMapping.
// Exposed (lowercase) intentionally so the same-package test can verify
// the mapping without exporting the method.
func (c *DrawdownConsumer) buildAlertRecord(p eventbus.DrawdownBreachPayload) domain.AlertRecord {
	return domain.AlertRecord{
		ID:        uuid.NewString(),
		Timestamp: p.Timestamp,
		Rule:      "portfolio_drawdown_breach",
		Severity:  "critical",
		Message: fmt.Sprintf("Portfolio drawdown %.2f%% exceeds limit %.2f%%",
			p.CurrentDrawdown*100, p.MaxDrawdownPct*100),
		Value:     p.CurrentDrawdown,
		Threshold: p.MaxDrawdownPct,
		Status:    domain.AlertStatusTriggered,
		DedupKey:  "portfolio_drawdown_breach",
		FirstSeen: &p.Timestamp,
		LastSeen:  &p.Timestamp,
		Count:     1,
	}
}
