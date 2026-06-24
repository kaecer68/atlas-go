package live

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

func TestOrchestrator_MarketSnapshot_EmitsPositionUpdate(t *testing.T) {
	tests := []struct {
		name         string
		withPosition bool
		wantEvent    bool
	}{
		{
			name:         "emits position update when position exists",
			withPosition: true,
			wantEvent:    true,
		},
		{
			name:         "no event when no position",
			withPosition: false,
			wantEvent:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := livestore.NewStateStore(t.TempDir())
			if tt.withPosition {
				st.UpdatePosition(domain.Position{
					Symbol:      "2330",
					Quantity:    10,
					AverageCost: 100,
				})
			}

			bus := NewChannelEventBus(16)
			t.Cleanup(func() { _ = bus.Close() })

			o := NewOrchestrator(context.Background(), st, bus,
				stubProvider{name: "stub", quotes: []domain.Quote{}},
				domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())
			if o == nil {
				t.Fatal("expected orchestrator")
			}

			eventCh := make(chan BusEvent, 4)
			sub := bus.Subscribe(EventPositionUpdate, func(ctx context.Context, event BusEvent) error {
				select {
				case eventCh <- event:
				default:
				}
				return nil
			})
			t.Cleanup(sub.Cancel)

			bus.Publish(BusEvent{
				ID:        "evt-test-market-snapshot",
				Type:      EventMarketSnapshot,
				Timestamp: time.Now(),
				Payload: MarketEventPayload{
					Symbol: "2330",
					Quote: domain.Quote{
						Symbol: "2330",
						Last:   105,
					},
				},
			})

			if tt.wantEvent {
				select {
				case got := <-eventCh:
					if got.Type != EventPositionUpdate {
						t.Fatalf("unexpected event type: got=%s want=%s", got.Type, EventPositionUpdate)
					}
					payload, ok := got.Payload.(PositionEventPayload)
					if !ok {
						t.Fatalf("unexpected payload type: %T", got.Payload)
					}
					if payload.Symbol != "2330" {
						t.Fatalf("unexpected symbol: %s", payload.Symbol)
					}
					if payload.ChangeType != "updated" {
						t.Fatalf("unexpected change type: got=%q want=updated", payload.ChangeType)
					}
					if payload.Position.Symbol != "2330" {
						t.Fatalf("unexpected position symbol: %s", payload.Position.Symbol)
					}
					if payload.Position.CurrentPrice != 105 {
						t.Fatalf("expected current price 105, got %v", payload.Position.CurrentPrice)
					}
				case <-time.After(1 * time.Second):
					t.Fatal("expected EventPositionUpdate but none received")
				}
				return
			}

			select {
			case got := <-eventCh:
				t.Fatalf("unexpected event type: %s", got.Type)
			case <-time.After(120 * time.Millisecond):
			}
		})
	}
}

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
			st := livestore.NewStateStore(t.TempDir())
			if tt.withPosition {
				st.UpdatePosition(tt.position)
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
				stateStore: st,
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

func TestDefaultOrchestratorConfig_DefaultValues(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	if cfg.MarketOpenTime != "09:00" {
		t.Fatalf("expected 09:00, got %q", cfg.MarketOpenTime)
	}
	if cfg.MarketCloseTime != "13:30" {
		t.Fatalf("expected 13:30, got %q", cfg.MarketCloseTime)
	}
	if cfg.BrokerMode != "dry-run" {
		t.Fatalf("expected dry-run, got %q", cfg.BrokerMode)
	}
	if cfg.StopLossEnabled != true {
		t.Fatal("expected StopLossEnabled to be true")
	}
	if cfg.TakeProfitEnabled != false {
		t.Fatal("expected TakeProfitEnabled to be false")
	}
	if cfg.BrokerMaxRetries != 1 {
		t.Fatalf("expected BrokerMaxRetries=1, got %d", cfg.BrokerMaxRetries)
	}
	if cfg.PreMarketCheck != true {
		t.Fatal("expected PreMarketCheck to be true")
	}
}

func TestOrchestrator_SetBroker_NonNil(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	o.SetBroker(NewDryRunBroker())
	if o.broker == nil {
		t.Fatal("expected broker to be set")
	}
	if o.effectiveBrokerMode != "dry-run" {
		t.Fatalf("expected dry-run mode, got %q", o.effectiveBrokerMode)
	}
}

func TestOrchestrator_SetBroker_NilFallback(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	o.SetBroker(nil)
	if o.broker == nil {
		t.Fatal("expected broker to fall back to DryRunBroker")
	}
	if o.effectiveBrokerMode != "dry-run" {
		t.Fatalf("expected dry-run mode, got %q", o.effectiveBrokerMode)
	}
}

func TestOrchestrator_SetWatchlist(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	o.SetWatchlist([]string{"2330", "2603"})
	if len(o.watchlist) != 2 {
		t.Fatalf("expected 2 watchlist symbols, got %d", len(o.watchlist))
	}
}

func TestOrchestrator_SetTradingMetrics(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	if o.metrics != nil {
		t.Fatal("expected nil metrics before SetTradingMetrics")
	}
	o.SetTradingMetrics(nil)
	if o.metrics != nil {
		t.Fatal("expected nil metrics after SetTradingMetrics(nil)")
	}
}

func TestOrchestrator_SetCircuitBreaker(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	cb := NewCircuitBreaker(t.TempDir()+"/cb.jsonl", t.TempDir()+"/cb_state.json")
	o.SetCircuitBreaker(cb)
	if o.circuitBreaker != cb {
		t.Fatal("expected circuitBreaker to be updated")
	}
}

func TestOrchestrator_Start_AlreadyRunning(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	o.isRunning = true
	err := o.Start()
	if err == nil {
		t.Fatal("expected error when orchestrator already running")
	}
}

func TestOrchestrator_Stop_NotRunning(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	err := o.Stop()
	if err != nil {
		t.Fatalf("expected nil when stopping not-running orchestrator: %v", err)
	}
}

func TestOrchestrator_ExecuteOrder(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	o := NewOrchestrator(context.Background(), store, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, DefaultOrchestratorConfig())

	// With executionMgr set, it delegates
	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}
	err := o.ExecuteOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("unexpected error from ExecuteOrder: %v", err)
	}
}

func TestOrchestrator_executeOrder_NilExecutionMgr_NilBroker(t *testing.T) {
	store := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	cb := NewCircuitBreaker(t.TempDir()+"/cb.jsonl", t.TempDir()+"/cb_state.json")
	cb.ResetDayState(1000000)

	o := &Orchestrator{
		stateStore:     store,
		eventBus:       bus,
		circuitBreaker: cb,
		broker:         nil,
		executionMgr:   nil,
		config:         DefaultOrchestratorConfig(),
	}

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}
	err := o.executeOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.broker == nil {
		t.Fatal("expected broker to be auto-initialized")
	}
}

func TestResolveBrokerMode_TWSE_MissingFieldsFallsBack(t *testing.T) {
	tests := []struct {
		name   string
		cfg    OrchestratorConfig
		expect string
	}{
		{
			name:   "missing URL",
			cfg:    OrchestratorConfig{BrokerMode: "live", BrokerAdapter: "twse"},
			expect: "live-guarded",
		},
		{
			name: "missing APIKey",
			cfg: OrchestratorConfig{
				BrokerMode:    "live",
				BrokerAdapter: "twse",
				TWSEAPIURL:    "https://twse.example",
			},
			expect: "live-guarded",
		},
		{
			name: "missing APISecret",
			cfg: OrchestratorConfig{
				BrokerMode:    "live",
				BrokerAdapter: "twse",
				TWSEAPIURL:    "https://twse.example",
				TWSEAPIKey:    "key1",
			},
			expect: "live-guarded",
		},
		{
			name: "missing AccountID",
			cfg: OrchestratorConfig{
				BrokerMode:    "live",
				BrokerAdapter: "twse",
				TWSEAPIURL:    "https://twse.example",
				TWSEAPIKey:    "key1",
				TWSEAPISecret: "secret1",
			},
			expect: "live-guarded",
		},
		{
			name: "all configured",
			cfg: OrchestratorConfig{
				BrokerMode:    "live",
				BrokerAdapter: "twse",
				TWSEAPIURL:    "https://twse.example",
				TWSEAPIKey:    "key1",
				TWSEAPISecret: "secret1",
				TWSEAccountID: "acct1",
			},
			expect: "live-twse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, effective, _, _ := resolveBrokerMode(tt.cfg)
			if effective != tt.expect {
				t.Fatalf("effective mode mismatch: got=%q want=%q", effective, tt.expect)
			}
		})
	}
}

func TestResolveBrokerMode_FubonDMA_MissingFieldsFallsBack(t *testing.T) {
	tests := []struct {
		name   string
		cfg    OrchestratorConfig
		expect string
	}{
		{
			name: "missing PersonalID",
			cfg: OrchestratorConfig{
				BrokerMode:    "live",
				BrokerAdapter: "fubon-dma",
			},
			expect: "live-guarded",
		},
		{
			name: "missing APIKey",
			cfg: OrchestratorConfig{
				BrokerMode:         "live",
				BrokerAdapter:      "fubon-dma",
				FubonDMAPersonalID: "PID123",
			},
			expect: "live-guarded",
		},
		{
			name: "all configured",
			cfg: OrchestratorConfig{
				BrokerMode:         "live",
				BrokerAdapter:      "fubon-dma",
				FubonDMAPersonalID: "PID123",
				FubonDMAAPIKey:     "api-key-1",
			},
			expect: "live-fubon-dma",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, effective, _, _ := resolveBrokerMode(tt.cfg)
			if effective != tt.expect {
				t.Fatalf("effective mode mismatch: got=%q want=%q", effective, tt.expect)
			}
		})
	}
}

func TestResolveBrokerMode_Live_UnknownAdapter(t *testing.T) {
	_, effective, _, audit := resolveBrokerMode(OrchestratorConfig{
		BrokerMode:    "live",
		BrokerAdapter: "unsupported-adapter-xyz",
	})
	if effective != "live-guarded" {
		t.Fatalf("expected live-guarded fallback, got %q", effective)
	}
	if audit == "" {
		t.Fatal("expected non-empty audit message for unknown adapter")
	}
}

func TestResolveBrokerMode_EmptyModeDefaultsToDryRun(t *testing.T) {
	_, effective, broker, _ := resolveBrokerMode(OrchestratorConfig{})
	if effective != "dry-run" {
		t.Fatalf("expected dry-run effective mode, got %q", effective)
	}
	if broker == nil || broker.Mode() != "dry-run" {
		t.Fatal("expected dry-run broker")
	}
}

func TestResolveBrokerMode_DryRun(t *testing.T) {
	_, effective, broker, _ := resolveBrokerMode(OrchestratorConfig{BrokerMode: "dry-run"})
	if effective != "dry-run" {
		t.Fatalf("expected dry-run, got %q", effective)
	}
	if broker == nil || broker.Mode() != "dry-run" {
		t.Fatal("expected dry-run broker")
	}
}

func TestResolveBrokerMode_Paper(t *testing.T) {
	_, effective, broker, _ := resolveBrokerMode(OrchestratorConfig{BrokerMode: "paper"})
	if effective != "dry-run" {
		t.Fatalf("expected paper→dry-run, got %q", effective)
	}
	if broker == nil || broker.Mode() != "dry-run" {
		t.Fatal("expected dry-run broker for paper mode")
	}
}

func TestExecuteOrderBlockedByCircuitBreaker(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	tmpDir := t.TempDir()
	cb := NewCircuitBreaker(tmpDir+"/cb_log.jsonl", tmpDir+"/cb_state.json")
	cb.ResetDayState(1000000)
	// Halt trading via daily loss
	cb.Evaluate(livestore.PortfolioState{Cash: 1000000, DayPnL: -30000}, nil, nil)

	o := &Orchestrator{
		stateStore:     st,
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
	cb.Evaluate(livestore.PortfolioState{Cash: 1000000, UnrealizedPnL: 0}, nil, nil)
	cb.Evaluate(livestore.PortfolioState{Cash: 965000, UnrealizedPnL: 0}, nil, nil) // 3.5% drawdown > 3% threshold
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
