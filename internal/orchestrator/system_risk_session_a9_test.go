package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// newResetTestSystem builds a daily-mode System whose ledger has NO
// simulation_state.json, so ensurePersistentStateLoaded must take the
// rebuild path (audit A9 silent-reset protection).
func newResetTestSystem(t *testing.T, baseDir string) *System {
	t.Helper()
	return &System{
		SystemCore: &SystemCore{
			sim: SimulationCore{
				cfg:      config.Config{PrimaryMarket: "TW", LedgerDir: baseDir},
				provider: marketdata.NewMockProvider(),
				engine:   sim.NewEngine(domain.SimulationConstraints{StartingCash: 3_000_000}),
				registry: SeedRegistry(),
				session:  domain.ReplaySession{ID: "session-a9-reset", Mode: "daily"},
				ledger:   ledger.NewStore(baseDir),
				policy: baseline.Policy{
					Constraints: domain.SimulationConstraints{StartingCash: 3_000_000},
				},
			},
			plugins: NewPluginRegistry(),
		},
	}
}

// TestEnsurePersistentStateLoaded_MissingFileRecordsReset verifies the A9
// guard: when the persistent state file is missing, the system rebuilds from
// scratch BUT records a durable persistent_state_reset event in the
// human_interventions audit trail and wires the fresh state in.
func TestEnsurePersistentStateLoaded_MissingFileRecordsReset(t *testing.T) {
	baseDir := t.TempDir()
	sys := newResetTestSystem(t, baseDir)

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}

	if sys.Sim().persistentState == nil {
		t.Fatal("persistentState must be non-nil after rebuild")
	}
	if sys.Sim().persistentState.StartingCash != 3_000_000 {
		t.Fatalf("starting cash = %v, want 3000000", sys.Sim().persistentState.StartingCash)
	}

	// The reset event must be durably recorded in human_interventions.jsonl.
	ivs, err := sys.Sim().ledger.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions: %v", err)
	}
	if len(ivs) != 1 {
		t.Fatalf("expected 1 recorded intervention, got %d", len(ivs))
	}
	iv := ivs[0]
	if iv.Type != "persistent_state_reset" {
		t.Fatalf("intervention type = %s, want persistent_state_reset", iv.Type)
	}
	if iv.Operator != "system" {
		t.Fatalf("operator = %s, want system", iv.Operator)
	}
	if iv.Reason == "" {
		t.Fatal("reason must not be empty")
	}
}

// TestEnsurePersistentStateLoaded_ExistingFileNoReset verifies the no-op path:
// when simulation_state.json exists, NO reset intervention is recorded and the
// loaded state is used as-is.
func TestEnsurePersistentStateLoaded_ExistingFileNoReset(t *testing.T) {
	baseDir := t.TempDir()
	sys := newResetTestSystem(t, baseDir)

	existing := domain.NewSimulationState(3_000_000)
	existing.Cash = 1_234_567 // carry over some equity
	if err := sim.SavePersistentState(baseDir, &existing); err != nil {
		t.Fatalf("SavePersistentState: %v", err)
	}

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}

	if sys.Sim().persistentState == nil || sys.Sim().persistentState.Cash != 1_234_567 {
		t.Fatalf("expected loaded state with cash 1234567, got %+v", sys.Sim().persistentState)
	}
	ivs, err := sys.Sim().ledger.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions: %v", err)
	}
	if len(ivs) != 0 {
		t.Fatalf("expected NO interventions when state file exists, got %d", len(ivs))
	}
}

// TestEnsurePersistentStateLoaded_PublishesResetEvent verifies the reset is
// also published to the event bus (nil-safe), so SSE/dashboard subscribers
// see the wipe instead of a silent reset.
func TestEnsurePersistentStateLoaded_PublishesResetEvent(t *testing.T) {
	baseDir := t.TempDir()
	sys := newResetTestSystem(t, baseDir)

	bus := eventbus.NewChannelEventBus(16)
	defer bus.Close()
	sys.SetEventBus(bus)

	got := make(chan eventbus.BusEvent, 8)
	bus.Subscribe(eventbus.EventPersistentStateReset, func(_ context.Context, ev eventbus.BusEvent) error {
		got <- ev
		return nil
	})

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}

	select {
	case ev := <-got:
		if ev.Type != eventbus.EventPersistentStateReset {
			t.Fatalf("event type = %s, want %s", ev.Type, eventbus.EventPersistentStateReset)
		}
		if ev.Severity != "warning" {
			t.Fatalf("severity = %s, want warning", ev.Severity)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected persistent_state.reset event on the bus, got none")
	}
}

// TestEnsurePersistentStateLoaded_NonDailyModeSkips verifies the mode gate:
// non-daily/replay modes return early without touching the ledger.
func TestEnsurePersistentStateLoaded_NonDailyModeSkips(t *testing.T) {
	baseDir := t.TempDir()
	sys := newResetTestSystem(t, baseDir)
	sys.Sim().session.Mode = "live"

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}
	if sys.Sim().persistentState != nil {
		t.Fatal("persistentState must stay nil for non-daily/replay modes")
	}
	ivs, err := sys.Sim().ledger.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("LoadHumanInterventions: %v", err)
	}
	if len(ivs) != 0 {
		t.Fatalf("expected NO interventions in non-daily mode, got %d", len(ivs))
	}
}

// TestPersistentStateResetJSONLShape ensures the reset record survives a
// JSONL read-back as the domain type would be consumed downstream.
func TestPersistentStateResetJSONLShape(t *testing.T) {
	baseDir := t.TempDir()
	sys := newResetTestSystem(t, baseDir)

	if err := sys.ensurePersistentStateLoaded(); err != nil {
		t.Fatalf("ensurePersistentStateLoaded: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(baseDir, "human_interventions.jsonl"))
	if err != nil {
		t.Fatalf("read human_interventions.jsonl: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal jsonl line: %v", err)
	}
	if raw["type"] != "persistent_state_reset" {
		t.Fatalf("jsonl type = %v, want persistent_state_reset", raw["type"])
	}
	if raw["operator"] != "system" {
		t.Fatalf("jsonl operator = %v, want system", raw["operator"])
	}
}
