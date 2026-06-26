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

// TradeSlippageConsumer subscribes to EventTradeSlippage (emitted per order
// fill by internal/live/order_manager.go) and persists a domain.AlertRecord
// when SlippageBPS exceeds the configured warningBps (WARNING) or errorBps
// (ERROR) thresholds. Below-threshold events are dropped silently.
//
// This is Decision 6 (alert-redesign-v2.md Part 3.4) — the consumer half
// of the Trade Slippage Anomaly alert. The publisher side already exists
// in internal/live/order_manager.go:190; this PR adds the persistence side.
//
// Thresholds are passed at construction time (not read from config inside
// the consumer) so the consumer is fully testable without mutating the
// global config singleton. Production wiring loads the thresholds from
// ParametersConfig.Alert.SlippageWarningBps / SlippageErrorBps.
type TradeSlippageConsumer struct {
	bus        eventbus.EventBus
	store      *AlertStore
	warningBps float64
	errorBps   float64

	mu      sync.Mutex
	started bool
	sub     eventbus.Subscription
}

// NewTradeSlippageConsumer creates a new consumer.
// nil bus → Start becomes a no-op; nil store → events are consumed but
// not persisted; zero thresholds → no alert is ever created (consume only).
func NewTradeSlippageConsumer(bus eventbus.EventBus, store *AlertStore, warningBps, errorBps float64) *TradeSlippageConsumer {
	return &TradeSlippageConsumer{
		bus:        bus,
		store:      store,
		warningBps: warningBps,
		errorBps:   errorBps,
	}
}

// Start subscribes to EventTradeSlippage. Idempotent via `started` bool
// (eventbus.Subscription is a struct, not interface — cannot be compared
// to untyped nil).
func (c *TradeSlippageConsumer) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bus == nil || c.started {
		return
	}
	c.sub = c.bus.Subscribe(eventbus.EventTradeSlippage, c.handleEvent)
	c.started = true
}

// Stop cancels the subscription. Idempotent.
func (c *TradeSlippageConsumer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return
	}
	c.sub.Cancel()
	c.started = false
}

// handleEvent converts EventTradeSlippage to a domain.AlertRecord when
// SlippageBPS exceeds the warning threshold. Returns nil (no error) for
// below-threshold events so the dispatcher does not retry — the event
// is intentionally dropped.
//
// Severity decision:
//   - SlippageBPS > errorBps   → "error"
//   - SlippageBPS > warningBps → "warning"
//   - else                    → drop (no record)
func (c *TradeSlippageConsumer) handleEvent(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.TradeSlippageEventPayload)
	if !ok {
		logging.Warn("trade_slippage_consumer", "payload_type_assertion_failed",
			logging.FStr("event_type", string(ev.Type)),
			logging.FStr("expected", "eventbus.TradeSlippageEventPayload"),
			logging.FStr("actual_type", fmt.Sprintf("%T", ev.Payload)))
		return nil
	}
	if c.store == nil {
		return nil
	}
	severity := c.classifySeverity(payload.SlippageBPS)
	if severity == "" {
		// Below threshold: drop silently. This is the financial-engineering
		// design choice — avoid alert fatigue from benign slippage.
		return nil
	}
	record := c.buildAlertRecord(payload, severity)
	if err := c.store.Save(record); err != nil {
		logging.Warn("trade_slippage_consumer", "alert_save_failed", logging.Err(err))
		return err
	}
	return nil
}

// classifySeverity returns "warning", "error", or "" (below threshold).
// Public for testability.
func (c *TradeSlippageConsumer) classifySeverity(slippageBps float64) string {
	if c.errorBps > 0 && slippageBps > c.errorBps {
		return "error"
	}
	if c.warningBps > 0 && slippageBps > c.warningBps {
		return "warning"
	}
	return ""
}

// buildAlertRecord converts a slippage event payload to a domain.AlertRecord.
// Per the v2 brief (alert-redesign-v2.md Part 3.4): payload contains
// symbol, expected_price, fill_price, slippage_bps, slippage_cost. Field
// mapping:
//   - Rule:     "trade_slippage"
//   - Value:    SlippageBPS (the trigger metric)
//   - Threshold: SlippageCost (cost impact for downstream analytics)
//   - Severity: passed in (caller decides)
func (c *TradeSlippageConsumer) buildAlertRecord(p eventbus.TradeSlippageEventPayload, severity string) domain.AlertRecord {
	return domain.AlertRecord{
		ID:        uuid.NewString(),
		Timestamp: p.Timestamp,
		Rule:      "trade_slippage",
		Severity:  severity,
		Message: fmt.Sprintf("Trade slippage %g BPS for %s %s (cost: %g)",
			p.SlippageBPS, p.Symbol, p.Side, p.SlippageCost),
		Value:     p.SlippageBPS,
		Threshold: p.SlippageCost,
		Status:    domain.AlertStatusTriggered,
		DedupKey:  "trade_slippage",
		FirstSeen: &p.Timestamp,
		LastSeen:  &p.Timestamp,
		Count:     1,
	}
}
