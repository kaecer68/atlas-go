package live

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

func TestExecuteOrderPublishesFilledEventInDryRunMode(t *testing.T) {
	s := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	tmpDir := t.TempDir()
	cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
	cb.ResetDayState(0)

	o := &Orchestrator{
		stateStore:     s,
		eventBus:       bus,
		broker:         NewDryRunBroker(),
		circuitBreaker: cb,
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
	s := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	tmpDir := t.TempDir()
	cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
	cb.ResetDayState(0)

	o := &Orchestrator{
		stateStore:     s,
		eventBus:       bus,
		broker:         NewDryRunBroker(),
		circuitBreaker: cb,
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
		if got.Type != EventOrderError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventOrderError)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected order error event but none was received")
	}
}

func TestGuardedLiveBrokerRejectsWhenAdapterMissing(t *testing.T) {
	b := NewGuardedLiveBroker(nil)
	result, err := b.SubmitOrder(context.Background(), domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 10,
		Price:    100,
	})
	if err == nil {
		t.Fatal("SubmitOrder expected error for missing adapter, got nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %q, want to contain 'not configured'", err.Error())
	}
	if result.Status != "rejected" {
		t.Fatalf("status = %q, want rejected", result.Status)
	}
	if !strings.Contains(result.Reason, "not configured") {
		t.Fatalf("unexpected reject reason: %q", result.Reason)
	}
}
