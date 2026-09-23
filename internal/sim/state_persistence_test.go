package sim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSavePersistentState(t *testing.T) {
	dir := t.TempDir()
	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{
		{Symbol: "2330.TW", Quantity: 1000, AverageCost: 500},
	}

	err := SavePersistentState(dir, &state)
	if err != nil {
		t.Fatalf("SavePersistentState failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "simulation_state.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected state file at %s", path)
	}
}

func TestSavePersistentState_NilState(t *testing.T) {
	dir := t.TempDir()

	err := SavePersistentState(dir, nil)
	if err != nil {
		t.Fatalf("expected nil state to be a no-op, got error: %v", err)
	}
}

func TestSavePersistentState_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	state := domain.NewSimulationState(1_000_000)

	err := SavePersistentState(dir, &state)
	if err != nil {
		t.Fatalf("SavePersistentState with new dir failed: %v", err)
	}
}

func TestLoadPersistentState_FileNotFound(t *testing.T) {
	dir := t.TempDir()

	state, err := LoadPersistentState(dir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for missing file")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	original := domain.NewSimulationState(500_000)
	original.Positions = []domain.Position{
		{Symbol: "2330.TW", Quantity: 500, AverageCost: 600},
		{Symbol: "2317.TW", Quantity: 1000, AverageCost: 150},
	}
	original.EquityCurve = []float64{500000, 510000, 520000}
	original.DailyReturns = []float64{0.02, 0.0196}
	original.PreviousValues = map[string]float64{"2330.TW": 600}

	err := SavePersistentState(dir, &original)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadPersistentState(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil state")
	}

	if loaded.Cash != original.Cash {
		t.Errorf("cash: got %f, want %f", loaded.Cash, original.Cash)
	}
	if len(loaded.Positions) != len(original.Positions) {
		t.Fatalf("positions: got %d, want %d", len(loaded.Positions), len(original.Positions))
	}
	for i, pos := range loaded.Positions {
		if pos.Symbol != original.Positions[i].Symbol {
			t.Errorf("position[%d] symbol: got %s, want %s", i, pos.Symbol, original.Positions[i].Symbol)
		}
		if pos.Quantity != original.Positions[i].Quantity {
			t.Errorf("position[%d] quantity: got %d, want %d", i, pos.Quantity, original.Positions[i].Quantity)
		}
	}
	if len(loaded.EquityCurve) != len(original.EquityCurve) {
		t.Errorf("equity curve: got %d, want %d", len(loaded.EquityCurve), len(original.EquityCurve))
	}
	if len(loaded.PreviousValues) != len(original.PreviousValues) {
		t.Errorf("previous values: got %d, want %d", len(loaded.PreviousValues), len(original.PreviousValues))
	}
}

func TestLoadPersistentState_NilFieldsInitialized(t *testing.T) {
	dir := t.TempDir()
	// Write a minimal JSON with no arrays
	minimal := `{"Cash": 100000}`
	os.WriteFile(filepath.Join(dir, "simulation_state.json"), []byte(minimal), 0o644)

	state, err := LoadPersistentState(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if state.Positions == nil {
		t.Error("expected Positions to be initialized to empty slice")
	}
	if state.EquityCurve == nil {
		t.Error("expected EquityCurve to be initialized to empty slice")
	}
	if state.DailyReturns == nil {
		t.Error("expected DailyReturns to be initialized to empty slice")
	}
	if state.PreviousValues == nil {
		t.Error("expected PreviousValues to be initialized to empty map")
	}
}

// TestLoadPersistentState_LegacyFileWithoutSessionDate covers the #1900
// migration path: a state file written before last_session_date /
// session_base_value existed must load without panic, keep its whole history,
// report an unknown session date, and gain the fields on the next write.
func TestLoadPersistentState_LegacyFileWithoutSessionDate(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
  "cash": 1010000,
  "positions": [],
  "starting_cash": 1000000,
  "equity_curve": [1000000, 1010000],
  "daily_returns": [0.01],
  "previous_values": {"_portfolio_": 1010000},
  "max_equity": 1010000,
  "current_drawdown": 0,
  "locked_cash": []
}`
	path := filepath.Join(dir, "simulation_state.json")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	state, err := LoadPersistentState(dir)
	if err != nil {
		t.Fatalf("legacy state must load: %v", err)
	}
	if len(state.DailyReturns) != 1 || len(state.EquityCurve) != 2 {
		t.Fatalf("legacy history was not preserved: returns=%v equity=%v", state.DailyReturns, state.EquityCurve)
	}
	if state.LastSessionDate != "" {
		t.Errorf("LastSessionDate = %q, want empty (unknown) for a legacy file", state.LastSessionDate)
	}
	if state.SessionBaseValue != 0 {
		t.Errorf("SessionBaseValue = %v, want 0 for a legacy file", state.SessionBaseValue)
	}

	// Writing the loaded state back adds the fields without touching history.
	state.LastSessionDate = "2026-09-23"
	state.SessionBaseValue = 1_010_000
	if err := SavePersistentState(dir, state); err != nil {
		t.Fatalf("save migrated state: %v", err)
	}
	reloaded, err := LoadPersistentState(dir)
	if err != nil {
		t.Fatalf("reload migrated state: %v", err)
	}
	if reloaded.LastSessionDate != "2026-09-23" || reloaded.SessionBaseValue != 1_010_000 {
		t.Errorf("session fields did not round-trip: %q / %v", reloaded.LastSessionDate, reloaded.SessionBaseValue)
	}
	if len(reloaded.DailyReturns) != 1 || len(reloaded.EquityCurve) != 2 {
		t.Errorf("migration changed history: returns=%v equity=%v", reloaded.DailyReturns, reloaded.EquityCurve)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated state: %v", err)
	}
	if !strings.Contains(string(raw), `"last_session_date": "2026-09-23"`) {
		t.Errorf("migrated state file does not carry the session date: %s", raw)
	}
}

// TestSavePersistentState_OmitsUnknownSessionDate keeps the legacy shape clean:
// a state that never saw a dated run must not grow spurious JSON keys, so old
// readers stay happy.
func TestSavePersistentState_OmitsUnknownSessionDate(t *testing.T) {
	dir := t.TempDir()
	state := domain.NewSimulationState(1_000_000)

	if err := SavePersistentState(dir, &state); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "simulation_state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if strings.Contains(string(raw), "last_session_date") || strings.Contains(string(raw), "session_base_value") {
		t.Errorf("undated state must not serialise the session keys: %s", raw)
	}
}

func TestLoadPersistentState_BadJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "simulation_state.json"), []byte("not json"), 0o644)

	_, err := LoadPersistentState(dir)
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestSavePersistentState_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	state := domain.NewSimulationState(1_000_000)

	err := SavePersistentState(dir, &state)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// No .tmp file should remain
	tmpPath := filepath.Join(dir, "simulation_state.json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("expected no leftover .tmp file after atomic rename")
	}
}
