package live

import (
	"context"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

// TestOrchestrator_CircuitEvaluationMirrorsHaltIntoRiskGate is the end-to-end
// half of FU-20261016-32: a breaker that reaches CircuitHalted during a normal
// intraday cycle must make the risk gate reject orders, and a breaker that
// returns to CircuitNormal must let them through again.
//
// The order is submitted to the orchestrator's own OrderManager, which is the
// only OrderManager built with a non-nil RiskGate (orchestrator.go:151). See the
// known limitation in the FU entry: Orchestrator.ExecuteOrder goes through
// ExecutionManager, whose OrderManager gets a nil gate, so on that path the
// breaker's own CanPlaceOrder is what blocks the order.
func TestOrchestrator_CircuitEvaluationMirrorsHaltIntoRiskGate(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	cfg := DefaultOrchestratorConfig()
	// Zero the gate's own daily-loss limit so the rejection below can only come
	// from the halt branch. VaRCriticalThreshold keeps its default (5%) and the
	// cycle must still leave var_value at 0: that is the "do not mirror VaR" half.
	cfg.MaxDailyLossPct = 0

	o := NewOrchestrator(context.Background(), st, bus,
		stubProvider{name: "stub", quotes: []domain.Quote{}},
		domain.AgentRegistry{}, nil, cfg)
	if o == nil {
		t.Fatal("expected orchestrator")
	}

	tmp := t.TempDir()
	cb := NewCircuitBreaker(tmp+"/cb_log.jsonl", tmp+"/cb_state.json")
	o.SetCircuitBreaker(cb)

	// Broker that records whether it was ever asked to submit.
	broker := &submitRecordingBroker{result: BrokerResult{OrderID: "oid-wired", Status: "filled", FillPrice: 100}}
	o.SetBroker(broker)

	// Market open seeds the breaker's day-start value, then the day loses 3%,
	// which is past the default 2% daily-loss rule.
	st.UpdatePortfolio(livestore.PortfolioState{Cash: 1_000_000})
	o.onMarketOpen()
	st.UpdatePortfolio(livestore.PortfolioState{Cash: 1_000_000, DayPnL: -30_000})

	o.onIntradayCycle()

	if cb.State() != CircuitHalted {
		t.Fatalf("expected breaker halted after a 3%% daily loss, got %s", cb.State())
	}
	status := o.riskGate.Status()
	if halted, _ := status["halt_trading"].(bool); !halted {
		t.Fatal("expected the cycle to mirror the halted breaker into RiskGate.halt_trading")
	}
	if got, _ := status["var_value"].(float64); got != 0 {
		t.Fatalf("expected VaR not to be mirrored by the cycle, got %v", status["var_value"])
	}

	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 1, Price: 100}
	err := o.orderMgr.Run(context.Background(), order)
	if err == nil {
		t.Fatal("expected the halted breaker to block the order through RiskGate.Check")
	}
	if !strings.Contains(err.Error(), "trading halted") {
		t.Fatalf("expected the trading-halted rejection, got: %v", err)
	}
	if broker.submits != 0 {
		t.Fatalf("expected no broker submission while halted, got %d", broker.submits)
	}

	// Clear the day and run the next cycle: the halt must lift and the same order
	// must reach the broker.
	cb.ResetDayState(1_000_000)
	st.UpdatePortfolio(livestore.PortfolioState{Cash: 1_000_000})
	o.onIntradayCycle()

	if halted, _ := o.riskGate.Status()["halt_trading"].(bool); halted {
		t.Fatal("expected the next cycle to clear halt_trading after the breaker returned to normal")
	}
	if err := o.orderMgr.Run(context.Background(), order); err != nil {
		t.Fatalf("expected the order to pass once the halt cleared, got: %v", err)
	}
	if broker.submits != 1 {
		t.Fatalf("expected exactly one broker submission after the halt cleared, got %d", broker.submits)
	}
}

type submitRecordingBroker struct {
	result  BrokerResult
	submits int
}

func (b *submitRecordingBroker) Mode() string { return "test" }

func (b *submitRecordingBroker) SubmitOrder(_ context.Context, _ domain.Order) (BrokerResult, error) {
	b.submits++
	return b.result, nil
}
