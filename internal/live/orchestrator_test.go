package live

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
			store := NewStateStore(t.TempDir())
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

			o := &Orchestrator{
				stateStore: store,
				eventBus:   bus,
				config: OrchestratorConfig{
					MaxPositionLossPct: tt.maxLossPct,
					StopLossEnabled:    tt.stopLoss,
					TakeProfitEnabled:  tt.takeProfit,
				},
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
