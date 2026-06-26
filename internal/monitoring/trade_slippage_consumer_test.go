package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestTradeSlippageConsumer_Start_Stop_Subscribes verifies Start subscribes
// to EventTradeSlippage and Stop cancels the subscription. Idempotency:
// Stop can be called multiple times without panic.
func TestTradeSlippageConsumer_Start_Stop_Subscribes(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewTradeSlippageConsumer(bus, store, 50, 100)
	c.Start()
	c.Stop()
	c.Stop() // must not panic
}

// TestTradeSlippageConsumer_BelowThresholdNoAlert verifies that small
// slippage events (below warningBps) do NOT create an alert record.
// Avoids alert fatigue from benign slippage.
func TestTradeSlippageConsumer_BelowThresholdNoAlert(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewTradeSlippageConsumer(bus, store, 50, 100)
	c.Start()
	defer c.Stop()

	// SlippageBPS=30 (below 50 warning threshold) → no alert.
	bus.PublishTradeSlippage(eventbus.TradeSlippageEventPayload{
		OrderID:       "ord-001",
		Symbol:        "2330",
		Side:          "buy",
		Quantity:      1000,
		ExpectedPrice: 100.0,
		FillPrice:     100.30,
		SlippageBPS:   30,
		SlippageCost:  300,
		BrokerMode:    "fubon_dma",
		Timestamp:     time.Now(),
	})

	time.Sleep(100 * time.Millisecond)
	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 alerts for below-threshold slippage, got %d", len(all))
	}
}

// TestTradeSlippageConsumer_WarningThresholdCreatesWarning verifies that
// slippage between warningBps and errorBps creates a WARNING alert.
func TestTradeSlippageConsumer_WarningThresholdCreatesWarning(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewTradeSlippageConsumer(bus, store, 50, 100)
	c.Start()
	defer c.Stop()

	// SlippageBPS=75 (between 50 warning and 100 error) → WARNING alert.
	bus.PublishTradeSlippage(eventbus.TradeSlippageEventPayload{
		OrderID:       "ord-002",
		Symbol:        "2330",
		Side:          "buy",
		Quantity:      1000,
		ExpectedPrice: 100.0,
		FillPrice:     100.75,
		SlippageBPS:   75,
		SlippageCost:  750,
		BrokerMode:    "fubon_dma",
		Timestamp:     time.Now(),
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all, err := store.LoadAll()
		if err != nil {
			t.Fatalf("LoadAll: %v", err)
		}
		if len(all) >= 1 {
			if all[0].Severity != "warning" {
				t.Errorf("expected Severity=warning, got %q", all[0].Severity)
			}
			if all[0].Rule != "trade_slippage" {
				t.Errorf("expected Rule=trade_slippage, got %q", all[0].Rule)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for WARNING alert")
}

// TestTradeSlippageConsumer_ErrorThresholdCreatesError verifies that
// slippage above errorBps creates an ERROR alert (highest severity).
func TestTradeSlippageConsumer_ErrorThresholdCreatesError(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewTradeSlippageConsumer(bus, store, 50, 100)
	c.Start()
	defer c.Stop()

	// SlippageBPS=150 (above 100 error threshold) → ERROR alert.
	bus.PublishTradeSlippage(eventbus.TradeSlippageEventPayload{
		OrderID:       "ord-003",
		Symbol:        "2330",
		Side:          "sell",
		Quantity:      500,
		ExpectedPrice: 500.0,
		FillPrice:     507.5,
		SlippageBPS:   150,
		SlippageCost:  3750,
		BrokerMode:    "fubon_dma",
		Timestamp:     time.Now(),
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all, err := store.LoadAll()
		if err != nil {
			t.Fatalf("LoadAll: %v", err)
		}
		if len(all) >= 1 {
			if all[0].Severity != "error" {
				t.Errorf("expected Severity=error, got %q", all[0].Severity)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for ERROR alert")
}

// TestTradeSlippageConsumer_NilStoreIsSafe verifies the no-store branch
// does not panic when a slippage event is published.
func TestTradeSlippageConsumer_NilStoreIsSafe(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	c := NewTradeSlippageConsumer(bus, nil, 50, 100)
	c.Start()
	defer c.Stop()

	bus.PublishTradeSlippage(eventbus.TradeSlippageEventPayload{
		OrderID:     "ord-004",
		Symbol:      "2330",
		SlippageBPS: 75,
		Timestamp:   time.Now(),
	})

	time.Sleep(100 * time.Millisecond)
}

// TestTradeSlippageConsumer_NilBusNoop verifies lazy-init with nil bus.
func TestTradeSlippageConsumer_NilBusNoop(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	c := NewTradeSlippageConsumer(nil, store, 50, 100)
	c.Start()
	c.Stop()
}

// TestTradeSlippageConsumer_PayloadTypeMismatchIsSafe verifies that wrong
// payload type is logged and skipped (no crash).
func TestTradeSlippageConsumer_PayloadTypeMismatchIsSafe(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewTradeSlippageConsumer(bus, store, 50, 100)
	c.Start()
	defer c.Stop()

	bus.Publish(eventbus.BusEvent{
		ID:        "test-mismatch",
		Type:      eventbus.EventTradeSlippage,
		Timestamp: time.Now(),
		Payload:   42, // wrong type
		Severity:  "info",
	})

	time.Sleep(100 * time.Millisecond)
	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 alerts after type-mismatch, got %d", len(all))
	}
}

// TestTradeSlippageConsumer_BuildAlertRecord_FieldMapping pins the field
// mapping. Per atlas-go convention: Symbol in Message for human-readable
// alert, SlippageBPS as Value, SlippageCost as Threshold.
func TestTradeSlippageConsumer_BuildAlertRecord_FieldMapping(t *testing.T) {
	c := &TradeSlippageConsumer{warningBps: 50, errorBps: 100}
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	rec := c.buildAlertRecord(eventbus.TradeSlippageEventPayload{
		OrderID:       "ord-005",
		Symbol:        "2330",
		Side:          "buy",
		Quantity:      1000,
		ExpectedPrice: 100.0,
		FillPrice:     100.75,
		SlippageBPS:   75,
		SlippageCost:  750,
		BrokerMode:    "fubon_dma",
		Timestamp:     now,
	}, "warning")

	if rec.Rule != "trade_slippage" {
		t.Errorf("Rule mismatch: %q", rec.Rule)
	}
	if rec.Severity != "warning" {
		t.Errorf("Severity mismatch: %q", rec.Severity)
	}
	if rec.Status != domain.AlertStatusTriggered {
		t.Errorf("Status mismatch: %q", rec.Status)
	}
	if rec.Value != 75 {
		t.Errorf("Value (SlippageBPS) mismatch: %v", rec.Value)
	}
	if rec.Threshold != 750 {
		t.Errorf("Threshold (SlippageCost) mismatch: %v", rec.Threshold)
	}
	if rec.Count != 1 {
		t.Errorf("Count mismatch: %d", rec.Count)
	}
	if rec.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rec.DedupKey == "" {
		t.Error("expected non-empty DedupKey")
	}
	wantMsg := "Trade slippage 75 BPS for 2330 buy (cost: 750)"
	if rec.Message != wantMsg {
		t.Errorf("Message mismatch:\n  got:  %q\n  want: %q", rec.Message, wantMsg)
	}
	if rec.Timestamp != now {
		t.Errorf("Timestamp mismatch: %v", rec.Timestamp)
	}
}

// Compile-time guard: handler signature must match eventbus.Subscribe.
var _ tradeSlippageHandler = func(_ context.Context, _ eventbus.BusEvent) error { return nil }

type tradeSlippageHandler = func(context.Context, eventbus.BusEvent) error
