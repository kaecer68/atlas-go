package live

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestExecuteOrderPublishesFilledEventInDryRunMode(t *testing.T) {
	store := NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	o := &Orchestrator{
		stateStore: store,
		eventBus:   bus,
		broker:     NewDryRunBroker(),
	}

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	err := o.executeOrder(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 10,
		Price:    100,
		Reason:   "phase6 kickoff",
	})
	if err != nil {
		t.Fatalf("executeOrder returned error: %v", err)
	}

	select {
	case got := <-eventCh:
		if got.Type != EventOrderFilled {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderFilled)
		}
		payload, ok := got.Payload.(OrderEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", got.Payload)
		}
		if payload.OrderID == "" {
			t.Fatalf("expected non-empty order id")
		}
		if payload.Status != "filled" {
			t.Fatalf("unexpected order status: %s", payload.Status)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected order filled event but none was received")
	}
}

func TestExecuteOrderPublishesSystemErrorWhenOrderInvalid(t *testing.T) {
	store := NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	o := &Orchestrator{
		stateStore: store,
		eventBus:   bus,
		broker:     NewDryRunBroker(),
	}

	eventCh := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		select {
		case eventCh <- event:
		default:
		}
		return nil
	})
	t.Cleanup(sub.Cancel)

	err := o.executeOrder(context.Background(), domain.Order{
		Symbol:   "",
		Side:     domain.SideBuy,
		Quantity: 10,
		Price:    100,
	})
	if err == nil {
		t.Fatalf("expected executeOrder error for invalid order")
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
