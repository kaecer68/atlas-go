package live

import (
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestExecuteOrderPublishesFilledlivestore.EventInDryRunMode(t *testing.T) {
	st := livestore.Newlivestore.StateStore(t.TempDir())
	bus := NewChannellivestore.EventBus(16)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	tmpDir := t.TempDir()
	cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
	cb.ResetDayState(0)

	o := &Orchestrator{
		stateStore:     st,
		eventBus:       bus,
		broker:         NewDryRunBroker(),
		circuitBreaker: cb,
	}

	eventCh := make(chan Buslivestore.Event, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event Buslivestore.Event) error {
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
		if got.Type != livestore.EventOrderFilled {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, livestore.EventOrderFilled)
		}
		payload, ok := got.Payload.(Orderlivestore.EventPayload)
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
	st := livestore.Newlivestore.StateStore(t.TempDir())
	bus := NewChannellivestore.EventBus(16)
	t.Cleanup(func() {
		_ = bus.Close()
	})

	tmpDir := t.TempDir()
	cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
	cb.ResetDayState(0)

	o := &Orchestrator{
		stateStore:     st,
		eventBus:       bus,
		broker:         NewDryRunBroker(),
		circuitBreaker: cb,
	}

	eventCh := make(chan Buslivestore.Event, 4)
	sub := bus.SubscribeAll(func(ctx context.Context, event Buslivestore.Event) error {
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
		if got.Type != livestore.EventOrderError {
			t.Fatalf("unexpected event type: got=%s want=%s", got.Type, livestore.EventOrderError)
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
