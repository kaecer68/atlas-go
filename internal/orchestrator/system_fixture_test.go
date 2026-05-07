package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sim"
)

func newTestSystem(t *testing.T) *System {
	t.Helper()
	return &System{
		SystemCore: &SystemCore{
			SimulationCore: SimulationCore{
				cfg:      config.Config{PrimaryMarket: "TW"},
				provider: marketdata.NewMockProvider(),
				engine:   sim.NewEngine(domain.SimulationConstraints{StartingCash: 1_000_000}),
				registry: SeedRegistry(),
				session:  domain.ReplaySession{ID: "test-session"},
				ledger:   ledger.NewStore(t.TempDir()),
			},
			plugins: NewPluginRegistry(),
		},
	}
}

func TestNewTestSystem_Constructs(t *testing.T) {
	sys := newTestSystem(t)
	if sys == nil || sys.SimulationCore.provider == nil || sys.SimulationCore.engine == nil {
		t.Fatal("essential fields must be populated")
	}
}

func TestSystem_RunDailySimulation_WithMockProvider(t *testing.T) {
	sys := newTestSystem(t)
	result, err := sys.RunDailySimulation(time.Now())
	if err != nil {
		t.Fatalf("RunDailySimulation error: %v", err)
	}
	if result.Regime == "" {
		t.Error("regime should not be empty")
	}
}

func TestSystem_RecordSessionSummary_AfterRun(t *testing.T) {
	sys := newTestSystem(t)
	result, err := sys.RunDailySimulation(time.Now())
	if err != nil {
		t.Fatalf("RunDailySimulation error: %v", err)
	}
	if err := sys.RecordSessionSummary(result, nil); err != nil {
		t.Fatalf("RecordSessionSummary error: %v", err)
	}
}
