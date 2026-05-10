package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
)

const persistentStateFile = "simulation_state.json"

func SavePersistentState(baseDir string, state *domain.SimulationState) error {
	if state == nil {
		return nil
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir persistent state dir: %w", err)
	}
	path := filepath.Join(baseDir, persistentStateFile)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal persistent state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write persistent state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename persistent state: %w", err)
	}
	return nil
}

func LoadPersistentState(baseDir string) (*domain.SimulationState, error) {
	path := filepath.Join(baseDir, persistentStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read persistent state: %w", err)
	}
	var state domain.SimulationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal persistent state: %w", err)
	}
	if state.Positions == nil {
		state.Positions = make([]domain.Position, 0)
	}
	if state.EquityCurve == nil {
		state.EquityCurve = make([]float64, 0)
	}
	if state.DailyReturns == nil {
		state.DailyReturns = make([]float64, 0)
	}
	if state.PreviousValues == nil {
		state.PreviousValues = make(map[string]float64)
	}
	return &state, nil
}
