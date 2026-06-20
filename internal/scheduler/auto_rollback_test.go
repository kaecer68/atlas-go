package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestAutoRollback_BurnInSkips(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")
	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results during burn_in, got %d", len(results))
	}
}

func TestAutoRollback_AgentDisable(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sick_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Inject negative returns so RollingSharpe < -1.0 (prevents count reset).
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			dw.RecordOutcome("sick_agent", -0.03, false)
		} else {
			dw.RecordOutcome("sick_agent", -0.01, false)
		}
	}

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	// Simulate 30 days of catastrophic Sharpe
	ar.agentFailCount["sick_agent"] = 30

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result, got %d", len(results))
	}
	if results[0].Action != "disable_agent" {
		t.Errorf("expected action=disable_agent, got %s", results[0].Action)
	}
	if results[0].TargetID != "sick_agent" {
		t.Errorf("expected target=sick_agent, got %s", results[0].TargetID)
	}

	// Verify agent was actually removed
	_, ok := dw.GetAgentWeightData("sick_agent")
	if ok {
		t.Error("expected sick_agent to be removed after rollback execution")
	}
}

func TestAutoRollback_PromotionDegradation(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	// Seed with positive returns to get a moderate pre-promotion Sharpe.
	// Use varied returns to ensure stddev > 0.
	for i := 0; i < 60; i++ {
		ret := 0.005 + float64(i%5)*0.002 // 0.005 ~ 0.013
		dw.RecordOutcome("agent_1", ret, true)
	}

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	preSharpe := ar.computeSystemSharpe()
	if preSharpe <= 0 {
		t.Fatalf("pre-promotion sharpe should be > 0, got %f", preSharpe)
	}
	ar.RecordPromotion("exp-001", preSharpe)

	// Now degrade performance: negative returns to drop Sharpe > 20%.
	dw.Reset()
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for i := 0; i < 60; i++ {
		ret := -0.03 - float64(i%5)*0.001 // -0.03 ~ -0.034
		dw.RecordOutcome("agent_1", ret, false)
	}

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	// Should detect degradation (>20% drop)
	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result for degraded promotion, got %d", len(results))
	}
	if results[0].Action != "revert_baseline" {
		t.Errorf("expected action=revert_baseline, got %s", results[0].Action)
	}
	if results[0].TargetID != "exp-001" {
		t.Errorf("expected target=exp-001, got %s", results[0].TargetID)
	}

	// Verify alert-only rollback is recorded in history.
	history := ar.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Action != "revert_baseline" {
		t.Errorf("expected history action=revert_baseline, got %s", history[0].Action)
	}
}

func TestAutoRollback_CalibrationDegradation(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	// Snapshot before calibration
	ar.RecordCalibration(1.0)

	// Current score drops >15% (simulate by manipulating dwManager state)
	// computeSystemCompositeScore returns 0 with no agents, which is < 0.85
	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	// Should detect calibration degradation
	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result, got %d", len(results))
	}
	if results[0].Action != "revert_calibration" {
		t.Errorf("expected action=revert_calibration, got %s", results[0].Action)
	}

	// Verify alert-only rollback is recorded in history.
	history := ar.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Action != "revert_calibration" {
		t.Errorf("expected history action=revert_calibration, got %s", history[0].Action)
	}
}

func TestAutoRollback_History(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sick_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Inject negative returns so RollingSharpe < -1.0.
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			dw.RecordOutcome("sick_agent", -0.03, false)
		} else {
			dw.RecordOutcome("sick_agent", -0.01, false)
		}
	}

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)
	ar.agentFailCount["sick_agent"] = 30

	ar.RunDaily(context.Background())

	history := ar.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Action != "disable_agent" {
		t.Errorf("expected history action=disable_agent, got %s", history[0].Action)
	}
}

func TestAutoRollback_RecordPromotion_EmitsEvent(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(eventbus.EventPromotionRecorded, func(_ context.Context, _ eventbus.BusEvent) error {
		received.Add(1)
		return nil
	})

	ar := NewAutoRollback(nil, nil, nil).WithEventBus(bus)
	ar.RecordPromotion("exp-event-001", 1.42)

	time.Sleep(100 * time.Millisecond)
	if got := received.Load(); got != 1 {
		t.Fatalf("expected 1 EventPromotionRecorded emitted, got %d", got)
	}
}
