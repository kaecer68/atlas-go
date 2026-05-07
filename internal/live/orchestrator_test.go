package live

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live/store"
)

func TestCheckRiskTriggers(t *testing.T) {
	tests := []struct {
		name          string
		withPosition  bool
		position      domain.Position
		currentPrice  float64
		stopLoss      bool
		takeProfit    bool
		maxLossPct    float64
		expectedEvent EventType
	}{
		{
			name:          "no position no event",
			withPosition:  false,
			currentPrice:  90,
			stopLoss:      true,
			takeProfit:    true,
			maxLossPct:    5,
			expectedEvent: "",
		},
		{
			name:         "stop loss triggered",
			withPosition: true,
			position: domain.Position{
				Symbol:      "2330",
				Quantity:    10,
				AverageCost: 100,
			},
			currentPrice:  94,
			stopLoss:      true,
			takeProfit:    false,
			maxLossPct:    5,
			expectedEvent: EventStopLossTriggered,
		},
		{
			name:         "take profit triggered",
			withPosition: true,
			position: domain.Position{
				Symbol:      "2330",
				Quantity:    10,
				AverageCost: 100,
			},
			currentPrice:  111,
			stopLoss:      false,
			takeProfit:    true,
			maxLossPct:    5,
			expectedEvent: EventTakeProfitTriggered,
		},
		{
			name:         "loss not deep enough",
			withPosition: true,
			position: domain.Position{
				Symbol:      "2330",
				Quantity:    10,
				AverageCost: 100,
			},
			currentPrice:  96,
			stopLoss:      true,
			takeProfit:    false,
			maxLossPct:    5,
			expectedEvent: "",
		},
		{
			name:         "gain not high enough",
			withPosition: true,
			position: domain.Position{
				Symbol:      "2330",
				Quantity:    10,
				AverageCost: 100,
			},
			currentPrice:  108,
			stopLoss:      false,
			takeProfit:    true,
			maxLossPct:    5,
			expectedEvent: "",
		},
		{
			name:         "stop loss disabled",
			withPosition: true,
			position: domain.Position{
				Symbol:      "2330",
				Quantity:    10,
				AverageCost: 100,
			},
			currentPrice:  90,
			stopLoss:      false,
			takeProfit:    false,
			maxLossPct:    5,
			expectedEvent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := store.NewStateStore(t.TempDir())
			if tt.withPosition {
				store.UpdatePosition(tt.position)
			}

			bus := NewChannelEventBus(16)
			t.Cleanup(func() {
				_ = bus.Close()
			})

			eventCh := make(chan BusEvent, 4)
			sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
				select {
				case eventCh <- event:
				default:
				}
				return nil
			})
			t.Cleanup(sub.Cancel)

			tmpDir := t.TempDir()
			cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
			cb.ResetDayState(0)

			o := &Orchestrator{
				stateStore: store,
				eventBus:   bus,
				config: OrchestratorConfig{
					MaxPositionLossPct: tt.maxLossPct,
					StopLossEnabled:    tt.stopLoss,
					TakeProfitEnabled:  tt.takeProfit,
				},
				circuitBreaker: cb,
			}

			o.checkRiskTriggers("2330", tt.currentPrice)

			if tt.expectedEvent == "" {
				select {
				case got := <-eventCh:
					t.Fatalf("unexpected event type: %s", got.Type)
				case <-time.After(120 * time.Millisecond):
				}
				return
			}

			select {
			case got := <-eventCh:
				if got.Type != tt.expectedEvent {
					t.Fatalf("unexpected event type: got=%s want=%s", got.Type, tt.expectedEvent)
				}

				payload, ok := got.Payload.(RiskEventPayload)
				if !ok {
					t.Fatalf("unexpected payload type: %T", got.Payload)
				}
				if payload.Symbol != "2330" {
					t.Fatalf("unexpected payload symbol: %s", payload.Symbol)
				}
			case <-time.After(1 * time.Second):
				t.Fatalf("expected risk event %s but none was received", tt.expectedEvent)
			}
		})
	}
}

func TestExecuteOrderBlockedByCircuitBreaker(t *testing.T) {
	s := store.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	tmpDir := t.TempDir()
	cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
	cb.ResetDayState(1000000)
	// Halt trading via daily loss
	cb.Evaluate(store.PortfolioState{Cash: 1000000, DayPnL: -30000}, nil, nil)

	o := &Orchestrator{
		stateStore:     store,
		eventBus:       bus,
		circuitBreaker: cb,
		broker:         NewDryRunBroker(),
	}

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}
	err := o.executeOrder(context.Background(), order)
	if err == nil {
		t.Fatal("expected executeOrder to be blocked by circuit breaker")
	}
	if cb.State() != CircuitHalted {
		t.Fatalf("expected halted state, got %s", cb.State())
	}

	// Sell should also be blocked in halted state
	order.Side = domain.SideSell
	err = o.executeOrder(context.Background(), order)
	if err == nil {
		t.Fatal("expected sell order to be blocked in halted state")
	}

	// Reset and verify sell works in paused state
	cb.ResetDayState(1000000)
	cb.Evaluate(store.PortfolioState{Cash: 1000000, UnrealizedPnL: 0}, nil, nil)
	cb.Evaluate(store.PortfolioState{Cash: 965000, UnrealizedPnL: 0}, nil, nil) // 3.5% drawdown > 3% threshold
	if cb.State() != CircuitPaused {
		t.Fatalf("expected paused state, got %s", cb.State())
	}
	order.Side = domain.SideBuy
	err = o.executeOrder(context.Background(), order)
	if err == nil {
		t.Fatal("expected buy order to be blocked in paused state")
	}
	order.Side = domain.SideSell
	err = o.executeOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("expected sell order to pass in paused state: %v", err)
	}
}
