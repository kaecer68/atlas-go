package live

import (
	"context"
	"errors"
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

	mgr := NewOrderManager(broker, bus, 1, 0)
	err := mgr.Execute(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 10,
		Price:    101.5,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderFilled {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderFilled)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected filled event but none was received")
	}
}

func TestOrderManagerPublishRejectedWithReason(t *testing.T) {
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
		results: []BrokerResult{{OrderID: "oid-2", Status: "rejected", Reason: "risk limit exceeded"}},
	}

	mgr := NewOrderManager(broker, bus, 0, 0)
	err := mgr.Execute(context.Background(), domain.Order{
		Symbol:   "2317",
		Side:     domain.SideBuy,
		Quantity: 200,
		Price:    88,
	})
	if err == nil {
		t.Fatalf("expected rejected order error")
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderRejected {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderRejected)
		}
		payload, ok := got.Payload.(OrderEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if payload.Order.Reason != "risk limit exceeded" {
			t.Fatalf("unexpected reject reason: %q", payload.Order.Reason)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected rejected event but none was received")
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

	mgr := NewOrderManager(broker, bus, 1, 0)
	err := mgr.Execute(context.Background(), domain.Order{
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
		if got.Type != EventSystemError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventSystemError)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected system error event but none was received")
	}
}
