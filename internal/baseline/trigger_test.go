package baseline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type triggerRecordingBus struct {
	mu     sync.Mutex
	subs   []eventbus.Subscription
	events []eventbus.BusEvent
}

func (b *triggerRecordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *triggerRecordingBus) Subscribe(_ eventbus.EventType, _ eventbus.EventHandler) eventbus.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := eventbus.Subscription{Cancel: func() {}}
	b.subs = append(b.subs, sub)
	return sub
}

func (b *triggerRecordingBus) SubscribeAll(_ eventbus.EventHandler) eventbus.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := eventbus.Subscription{Cancel: func() {}}
	b.subs = append(b.subs, sub)
	return sub
}

func (b *triggerRecordingBus) Close() error { return nil }

func TestTrigger_StartStopLifecycle(t *testing.T) {
	bus := &triggerRecordingBus{}
	mgr := NewManager("")
	trig := NewTrigger(mgr, bus)

	if err := trig.Start(context.Background()); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	if !trig.started {
		t.Fatal("expected trigger to be started")
	}
	if len(bus.subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(bus.subs))
	}

	if err := trig.Start(context.Background()); err == nil {
		t.Fatal("second Start should return error")
	}

	if err := trig.Stop(); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}
	if trig.started {
		t.Fatal("expected trigger to be stopped")
	}

	if err := trig.Stop(); err != nil {
		t.Fatalf("second Stop should be no-op: %v", err)
	}
}

func TestTrigger_StartRequiresManager(t *testing.T) {
	bus := &triggerRecordingBus{}
	trig := NewTrigger(nil, bus)
	if err := trig.Start(context.Background()); err == nil {
		t.Fatal("expected error when manager is nil")
	}
}

func TestTrigger_StartRequiresBus(t *testing.T) {
	mgr := NewManager("")
	trig := NewTrigger(mgr, nil)
	if err := trig.Start(context.Background()); err == nil {
		t.Fatal("expected error when bus is nil")
	}
}

func TestTrigger_Evaluate_StopLoss(t *testing.T) {
	trig := NewTrigger(NewManager(""), &triggerRecordingBus{})
	policy := Policy{
		Constraints: domain.SimulationConstraints{
			StopLossPct: 0.05,
		},
	}
	pos := domain.Position{
		Symbol:       "2330",
		AverageCost:  100,
		CurrentPrice: 94,
		EntryDate:    time.Now(),
	}
	violations := trig.evaluate(eventbus.PositionEventPayload{Symbol: "2330", Position: pos}, policy)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Field != "stop_loss_pct" {
		t.Errorf("expected field stop_loss_pct, got %s", violations[0].Field)
	}
	if violations[0].Severity != "error" {
		t.Errorf("expected severity error, got %s", violations[0].Severity)
	}
}

func TestTrigger_Evaluate_TakeProfit(t *testing.T) {
	trig := NewTrigger(NewManager(""), &triggerRecordingBus{})
	policy := Policy{
		Constraints: domain.SimulationConstraints{
			TakeProfitPct: 0.10,
		},
	}
	pos := domain.Position{
		Symbol:       "2454",
		AverageCost:  100,
		CurrentPrice: 112,
		EntryDate:    time.Now(),
	}
	violations := trig.evaluate(eventbus.PositionEventPayload{Symbol: "2454", Position: pos}, policy)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Field != "take_profit_pct" {
		t.Errorf("expected field take_profit_pct, got %s", violations[0].Field)
	}
	if violations[0].Severity != "warn" {
		t.Errorf("expected severity warn, got %s", violations[0].Severity)
	}
}

func TestTrigger_Evaluate_MaxHoldingDays(t *testing.T) {
	trig := NewTrigger(NewManager(""), &triggerRecordingBus{})
	policy := Policy{
		Constraints: domain.SimulationConstraints{
			MaxHoldingDays: 5,
		},
	}
	pos := domain.Position{
		Symbol:       "2317",
		AverageCost:  100,
		CurrentPrice: 101,
		EntryDate:    time.Now().Add(-7 * 24 * time.Hour),
	}
	violations := trig.evaluate(eventbus.PositionEventPayload{Symbol: "2317", Position: pos}, policy)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Field != "max_holding_days" {
		t.Errorf("expected field max_holding_days, got %s", violations[0].Field)
	}
}

func TestTrigger_Evaluate_NoViolation(t *testing.T) {
	trig := NewTrigger(NewManager(""), &triggerRecordingBus{})
	policy := Policy{
		Constraints: domain.SimulationConstraints{
			StopLossPct:    0.05,
			TakeProfitPct:  0.10,
			MaxHoldingDays: 5,
		},
	}
	pos := domain.Position{
		Symbol:       "2881",
		AverageCost:  100,
		CurrentPrice: 102,
		EntryDate:    time.Now().Add(-2 * 24 * time.Hour),
	}
	violations := trig.evaluate(eventbus.PositionEventPayload{Symbol: "2881", Position: pos}, policy)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestTrigger_Evaluate_AllViolations(t *testing.T) {
	trig := NewTrigger(NewManager(""), &triggerRecordingBus{})
	policy := Policy{
		Constraints: domain.SimulationConstraints{
			StopLossPct:    0.05,
			TakeProfitPct:  0.10,
			MaxHoldingDays: 5,
		},
	}
	pos := domain.Position{
		Symbol:       "0050",
		AverageCost:  100,
		CurrentPrice: 112,
		EntryDate:    time.Now().Add(-7 * 24 * time.Hour),
	}
	violations := trig.evaluate(eventbus.PositionEventPayload{Symbol: "0050", Position: pos}, policy)
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations (take_profit + max_holding_days), got %d", len(violations))
	}
	fields := make(map[string]bool)
	for _, v := range violations {
		fields[v.Field] = true
	}
	if !fields["take_profit_pct"] {
		t.Error("expected take_profit_pct violation")
	}
	if !fields["max_holding_days"] {
		t.Error("expected max_holding_days violation")
	}
	if fields["stop_loss_pct"] {
		t.Error("did not expect stop_loss_pct violation")
	}
}

func TestTrigger_OnPositionUpdate_Integration(t *testing.T) {
	bus := eventbus.NewChannelEventBus(16)
	defer bus.Close()

	trig := NewTrigger(NewManager(""), bus)
	if err := trig.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer trig.Stop()

	// Publish a position update that should produce a take-profit warning.
	bus.PublishPositionUpdate("2330", domain.Position{
		Symbol:       "2330",
		AverageCost:  100,
		CurrentPrice: 115,
		EntryDate:    time.Now(),
	}, "updated")

	// Give the async handler a moment to run.
	time.Sleep(100 * time.Millisecond)
}

func TestTrigger_OnPositionUpdate_IgnoresWrongPayload(t *testing.T) {
	trig := NewTrigger(NewManager(""), &triggerRecordingBus{})
	if err := trig.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer trig.Stop()

	err := trig.onPositionUpdate(context.Background(), eventbus.BusEvent{
		Type:    eventbus.EventPositionUpdate,
		Payload: "not-a-position-payload",
	})
	if err != nil {
		t.Fatalf("expected nil error for wrong payload, got %v", err)
	}
}
