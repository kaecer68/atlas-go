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

	mgr := NewOrderManager(broker, bus, 1, 0)
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
	err := mgr.Run(context.Background(), domain.Order{
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
		if got.Type != EventOrderError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderError)
		}
		payload, ok := got.Payload.(OrderErrorEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if payload.ErrorCode != "rejected" {
			t.Fatalf("unexpected error code: %q", payload.ErrorCode)
		}
		if payload.ErrorMessage != "risk limit exceeded" {
			t.Fatalf("unexpected error message: %q", payload.ErrorMessage)
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

	mgr := NewOrderManager(broker, bus, 1, 0)
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

	mgr := NewOrderManager(broker, bus, 0, 0)
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
