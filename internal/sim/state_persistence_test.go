package sim

import (
	"os"
	"path/filepath"
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
