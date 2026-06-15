package live

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type scriptedBroker struct {
	results []BrokerResult
	errors  []error
	idx     int
}

func (b *scriptedBroker) Mode() string { return "test" }

func (b *scriptedBroker) SubmitOrder(_ context.Context, _ domain.Order) (BrokerResult, error) {
	idx := b.idx
	b.idx++

	if idx < len(b.errors) && b.errors[idx] != nil {
		return BrokerResult{}, b.errors[idx]
	}
	if idx < len(b.results) {
		return b.results[idx], nil
	}
	return BrokerResult{}, errors.New("script exhausted")
}

func TestOrderManagerRetriesThenPublishesFilled(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	broker := &scriptedBroker{
		errors: []error{errors.New("temporary network error"), nil},
		results: []BrokerResult{
			{},
			{OrderID: "oid-1", Status: "filled", FillPrice: 101.5},
		},
	}

	mgr := NewOrderManager(broker, bus, 1, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 10,
		Price:    101.5,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderFilled && got.Type != EventOrderPlaced {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderFilled)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected order error event but none was received")
	}
}

func TestOrderManagerPublishSystemErrorAfterRetryExhausted(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	broker := &scriptedBroker{
		errors: []error{errors.New("timeout"), errors.New("timeout")},
	}

	mgr := NewOrderManager(broker, bus, 1, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2603",
		Side:     domain.SideSell,
		Quantity: 5,
		Price:    40,
	})
	if err == nil {
		t.Fatalf("expected error when retries exhausted")
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderError)
		}
		payload, ok := got.Payload.(OrderErrorEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if payload.ErrorCode != "retry_exhausted" {
			t.Fatalf("unexpected error code: %q", payload.ErrorCode)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected order error event but none was received")
	}
}

func TestOrderManager_Mode(t *testing.T) {
	broker := NewDryRunBroker()
	mgr := NewOrderManager(broker, nil, 0, 0, nil)
	if mgr.Mode() != "dry-run" {
		t.Fatalf("expected dry-run mode, got %q", mgr.Mode())
	}
}

func TestOrderManager_RecordAndGetOrder(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)

	rec := OrderRecord{
		OrderID:  "oid-1",
		Symbol:   "2330",
		Side:     "buy",
		Quantity: 1000,
		Price:    500,
		Status:   "filled",
	}
	mgr.RecordOrder(rec)

	got, err := mgr.GetOrder("oid-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.OrderID != "oid-1" {
		t.Fatalf("order ID mismatch: got=%q want=oid-1", got.OrderID)
	}
}

func TestOrderManager_GetOrder_NotFound(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	_, err := mgr.GetOrder("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent order")
	}
}

func TestOrderManager_GetOrders_DefaultFilter(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	mgr.RecordOrder(OrderRecord{OrderID: "oid-1"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-2"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-3"})

	orders, total, err := mgr.GetOrders(OrderFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total mismatch: got=%d want=3", total)
	}
	if len(orders) != 3 {
		t.Fatalf("count mismatch: got=%d want=3", len(orders))
	}
}

func TestOrderManager_GetOrders_SymbolFilter(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	mgr.RecordOrder(OrderRecord{OrderID: "oid-1", Symbol: "2330"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-2", Symbol: "2603"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-3", Symbol: "2330"})

	orders, total, _ := mgr.GetOrders(OrderFilter{Symbol: "2330"})
	if total != 2 {
		t.Fatalf("expected 2 2330 orders, got %d", total)
	}
	_ = orders
	for _, o := range orders {
		if o.Symbol != "2330" {
			t.Fatalf("unexpected symbol in filtered results: %s", o.Symbol)
		}
	}
}

func TestOrderManager_GetOrders_StatusFilter(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	mgr.RecordOrder(OrderRecord{OrderID: "oid-1", Status: "filled"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-2", Status: "pending"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-3", Status: "filled"})

	_, total, _ := mgr.GetOrders(OrderFilter{Status: "filled"})
	if total != 2 {
		t.Fatalf("expected 2 filled orders, got %d", total)
	}
}

func TestOrderManager_GetOrders_SideFilter(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	mgr.RecordOrder(OrderRecord{OrderID: "oid-1", Side: "buy"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-2", Side: "sell"})
	mgr.RecordOrder(OrderRecord{OrderID: "oid-3", Side: "buy"})

	_, total, _ := mgr.GetOrders(OrderFilter{Side: "buy"})
	if total != 2 {
		t.Fatalf("expected 2 buy orders, got %d", total)
	}
}

func TestOrderManager_GetOrders_DateRangeFilter(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	mgr.RecordOrder(OrderRecord{OrderID: "old", CreatedAt: old})
	mgr.RecordOrder(OrderRecord{OrderID: "recent", CreatedAt: recent})

	orders, total, _ := mgr.GetOrders(OrderFilter{
		DateFrom: now.Add(-24 * time.Hour),
	})
	if total != 1 {
		t.Fatalf("expected 1 recent order, got %d", total)
	}
	if orders[0].OrderID != "recent" {
		t.Fatalf("expected recent order, got %s", orders[0].OrderID)
	}

	orders, total, _ = mgr.GetOrders(OrderFilter{
		DateTo: now.Add(-24 * time.Hour),
	})
	if total != 1 {
		t.Fatalf("expected 1 old order, got %d", total)
	}
	if orders[0].OrderID != "old" {
		t.Fatalf("expected old order, got %s", orders[0].OrderID)
	}
}

func TestOrderManager_GetOrders_Pagination(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	for i := range 5 {
		mgr.RecordOrder(OrderRecord{OrderID: fmt.Sprintf("oid-%d", i+1)})
	}

	// Page 1 with page size 2
	orders, total, _ := mgr.GetOrders(OrderFilter{Page: 1, PageSize: 2})
	if total != 5 {
		t.Fatalf("total mismatch: got=%d want=5", total)
	}
	if len(orders) != 2 {
		t.Fatalf("page 1 size mismatch: got=%d want=2", len(orders))
	}

	// Page 3 (last page with 1 item)
	orders, _, _ = mgr.GetOrders(OrderFilter{Page: 3, PageSize: 2})
	if len(orders) != 1 {
		t.Fatalf("page 3 size mismatch: got=%d want=1", len(orders))
	}

	// Page beyond range
	orders, total, _ = mgr.GetOrders(OrderFilter{Page: 99, PageSize: 2})
	if total != 5 {
		t.Fatalf("total mismatch: got=%d want=5", total)
	}
	if len(orders) != 0 {
		t.Fatalf("expected empty slice for out-of-range page, got %d", len(orders))
	}
}

func TestOrderManager_GetOrders_ZeroPageDefaults(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	mgr.RecordOrder(OrderRecord{OrderID: "oid-1"})

	// Both Page and PageSize are zero, should default to page 1 / size 20
	orders, total, _ := mgr.GetOrders(OrderFilter{Page: 0, PageSize: 0})
	if total != 1 {
		t.Fatalf("total mismatch: got=%d want=1", total)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order with defaults, got %d", len(orders))
	}
}

func TestOrderManager_NewOrderManager_NilBrokerFallback(t *testing.T) {
	mgr := NewOrderManager(nil, nil, 0, 0, nil)
	if mgr.broker == nil {
		t.Fatal("expected broker to be initialized with DryRunBroker")
	}
	if mgr.Mode() != "dry-run" {
		t.Fatalf("expected dry-run mode, got %q", mgr.Mode())
	}
}

func TestOrderManager_NewOrderManager_NegativeValuesClamped(t *testing.T) {
	mgr := NewOrderManager(NewDryRunBroker(), nil, -5, -100*time.Millisecond, nil)
	if mgr.maxRetries != 0 {
		t.Fatalf("expected maxRetries 0, got %d", mgr.maxRetries)
	}
	if mgr.retryBackoff != 0 {
		t.Fatalf("expected retryBackoff 0, got %v", mgr.retryBackoff)
	}
}

func TestOrderManager_Run_RiskGateBlocks(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	riskGate := NewRiskGate(RiskGateConfig{MaxDailyLossPct: 0.03})
	riskGate.SetHaltTrading(true)

	mgr := NewOrderManager(NewDryRunBroker(), bus, 0, 0, riskGate)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 100,
		Price:    500,
	})
	if err == nil {
		t.Fatal("expected risk gate to block order")
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderError)
		}
		payload, ok := got.Payload.(OrderErrorEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if payload.ErrorCode != "risk_gate_blocked" {
			t.Fatalf("unexpected error code: %q", payload.ErrorCode)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected risk gate blocked event but none was received")
	}
}

func TestOrderManager_Run_BrokerRejectsOrder(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	broker := &scriptedBroker{
		results: []BrokerResult{
			{OrderID: "r-1", Status: "rejected", Reason: "insufficient funds"},
		},
	}

	mgr := NewOrderManager(broker, bus, 0, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideSell,
		Quantity: 10,
		Price:    100,
	})
	if err == nil {
		t.Fatal("expected error for rejected order")
	}
	if !strings.Contains(err.Error(), "insufficient funds") {
		t.Fatalf("expected error to mention insufficient funds, got: %v", err)
	}

	// Drain events until we find the rejected error event. ChannelEventBus
	// dispatches each handler in its own goroutine, so the order.rejected
	// lifecycle event and the order.error event may arrive in either order.
	found := false
	for {
		select {
		case got := <-eventCh:
			if got.Type != EventOrderError {
				continue
			}
			payload, ok := got.Payload.(OrderErrorEventPayload)
			if !ok {
				t.Fatalf("unexpected payload type: %T", got.Payload)
			}
			if payload.ErrorCode != "rejected" {
				t.Fatalf("unexpected error code: %q", payload.ErrorCode)
			}
			found = true
		case <-time.After(1 * time.Second):
			if !found {
				t.Fatal("expected rejected event but none was received")
			}
		}
		if found {
			break
		}
	}
}

func TestOrderManager_Run_EmptyStatusDefaultsToPlaced(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	broker := &scriptedBroker{
		results: []BrokerResult{
			{OrderID: "oid-empty-status", Status: ""}, // empty status
		},
	}

	mgr := NewOrderManager(broker, bus, 0, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 5,
		Price:    100,
	})
	if err != nil {
		t.Fatalf("expected success with empty status defaulting to placed, got: %v", err)
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderPlaced {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderPlaced)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected order placed event but none was received")
	}
}

func TestOrderManager_Run_NilEventBus_DoesNotPanic(t *testing.T) {
	// Verify no panic when eventBus is nil
	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1,
		Price:    100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOrderManager_Run_NilEventBus_RiskGateBlocked_DoesNotPanic(t *testing.T) {
	riskGate := NewRiskGate(RiskGateConfig{MaxDailyLossPct: 0.03})
	riskGate.SetHaltTrading(true)

	mgr := NewOrderManager(NewDryRunBroker(), nil, 0, 0, riskGate)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1,
		Price:    100,
	})
	if err == nil {
		t.Fatal("expected risk gate to block order")
	}
	if !strings.Contains(err.Error(), "risk gate blocked") {
		t.Fatalf("expected risk gate blocked error, got: %v", err)
	}
}

func TestOrderManagerPublishesSignerErrorClassification(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	broker := &scriptedBroker{
		errors: []error{fmt.Errorf("broker rejected request: code=auth.signature_invalid status=401 body=bad signature")},
	}

	mgr := NewOrderManager(broker, bus, 0, 0, nil)
	err := mgr.Run(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1,
		Price:    100,
	})
	if err == nil {
		t.Fatalf("expected error when signer auth fails")
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderError)
		}
		payload, ok := got.Payload.(OrderErrorEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if !strings.Contains(payload.ErrorMessage, "auth.signature_invalid") {
			t.Fatalf("expected signer classification in error message: %v", payload.ErrorMessage)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected order error event but none was received")
	}
}
