package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func testCapitalConfig() config.Config {
	return config.Config{ReplayDataPath: "../../data/replay/tw_extended_90days.csv"}
}

func TestWithCapitalManagement_SetsFields(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	controller := risk.NewCapitalPhaseController(capitalCfg)
	allocator := portfolio.NewCapitalAllocator()
	workflow, _ := risk.NewApprovalWorkflow(t.TempDir())

	sys.WithCapitalManagement(controller, allocator, workflow)

	if sys.Risk().capitalController == nil {
		t.Fatal("capitalController should be set")
	}
	if sys.Port().capitalAllocator == nil {
		t.Fatal("capitalAllocator should be set")
	}
	if sys.Risk().approvalWorkflow == nil {
		t.Fatal("approvalWorkflow should be set")
	}
}

func TestCheckCapitalPhase_NilController(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	can, reason := sys.checkCapitalPhase()
	if can {
		t.Error("expected false when controller is nil")
	}
	if reason == "" {
		t.Error("expected reason when controller is nil")
	}
}

func TestCheckCapitalPhase_CannotAdvance(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	capitalCfg.MinDaysPerPhase = 30
	capitalCfg.PhaseStartDate = time.Now().Add(-5 * 24 * time.Hour)
	controller := risk.NewCapitalPhaseController(capitalCfg)
	controller.UpdateMetrics(1.5, 0.05)

	sys.WithCapitalManagement(controller, nil, nil)

	can, reason := sys.checkCapitalPhase()
	if can {
		t.Errorf("expected false, got reason: %s", reason)
	}
}

func TestCheckCapitalPhase_CanAdvance(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	capitalCfg.MinDaysPerPhase = 10
	capitalCfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	controller := risk.NewCapitalPhaseController(capitalCfg)
	controller.UpdateMetrics(1.5, 0.05)

	sys.WithCapitalManagement(controller, nil, nil)

	can, reason := sys.checkCapitalPhase()
	if !can {
		t.Errorf("expected true, got reason: %s", reason)
	}
}

func TestCheckCapitalPhase_LivePhaseRequestsApproval(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	capitalCfg.CurrentPhase = domain.PhaseLive
	capitalCfg.MinDaysPerPhase = 10
	capitalCfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	controller := risk.NewCapitalPhaseController(capitalCfg)
	controller.UpdateMetrics(1.5, 0.05)

	workflow, _ := risk.NewApprovalWorkflow(t.TempDir())
	sys.WithCapitalManagement(controller, nil, workflow)

	can, reason := sys.checkCapitalPhase()
	if can {
		t.Errorf("expected false when approval needed, got reason: %s", reason)
	}
	if reason == "" {
		t.Error("expected reason about approval")
	}

	pending, _ := workflow.PendingRequests()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending request, got %d", len(pending))
	}
	if pending[0].Type != "phase_advance_to_full" {
		t.Errorf("expected request type %q, got %q", "phase_advance_to_full", pending[0].Type)
	}
}

func TestUpdateCapitalMetrics_NilController(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	sys.Sim().portfolioHistory = []float64{100000, 101000, 102000}
	sys.Sim().returnHistory = []float64{0.01, 0.01}

	sys.updateCapitalMetrics(domain.SimulationResult{})
}

func TestUpdateCapitalMetrics_UpdatesController(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	controller := risk.NewCapitalPhaseController(capitalCfg)
	sys.WithCapitalManagement(controller, nil, nil)

	sys.Sim().portfolioHistory = []float64{100000, 101000, 102000, 99000}
	sys.Sim().returnHistory = []float64{0.01, 0.01, -0.029}

	sys.updateCapitalMetrics(domain.SimulationResult{})

	snap := controller.GetSnapshot()
	if snap.RollingSharpe == 0 {
		t.Error("expected non-zero Sharpe after update")
	}
	if snap.MaxDrawdown == 0 {
		t.Error("expected non-zero drawdown after update")
	}
}

func TestUpdateCapitalMetrics_ShortHistory(t *testing.T) {
	sys := NewSystem(testCapitalConfig())

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	controller := risk.NewCapitalPhaseController(capitalCfg)
	sys.WithCapitalManagement(controller, nil, nil)

	sys.Sim().returnHistory = []float64{0.01}

	sys.updateCapitalMetrics(domain.SimulationResult{})

	snap := controller.GetSnapshot()
	if snap.RollingSharpe != 0 {
		t.Errorf("expected zero Sharpe with short history, got %.4f", snap.RollingSharpe)
	}
}
