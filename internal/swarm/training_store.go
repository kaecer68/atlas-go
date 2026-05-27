package swarm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrainingStore persists training scenarios from swarm simulations
// to disk as JSONL files, making them available for downstream consumers
// (analysis, evolution, experiment).
type TrainingStore struct {
	outputDir string
	mu        sync.Mutex
}

// StoredScenario wraps a TrainingScenario with metadata for persistence.
type StoredScenario struct {
	FishID      string          `json:"fish_id"`
	Scenario    string          `json:"scenario"`
	ScenarioID  string          `json:"scenario_id"`
	Performance FishPerformance `json:"performance"`
	StepCount   int             `json:"step_count"`
	RecordedAt  time.Time       `json:"recorded_at"`
	States      []MarketState   `json:"states,omitempty"`
	Predictions []Prediction    `json:"predictions,omitempty"`
}

// NewTrainingStore creates a TrainingStore rooted at outputDir.
func NewTrainingStore(outputDir string) *TrainingStore {
	return &TrainingStore{outputDir: outputDir}
}

// Store persists training data from the swarm to disk.
// Each scenario type gets its own JSONL file for easy consumption.
func (ts *TrainingStore) Store(scenarios []TrainingScenario) error {
	if len(scenarios) == 0 {
		return nil
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	if err := os.MkdirAll(ts.outputDir, 0o755); err != nil {
		return fmt.Errorf("create training store dir: %w", err)
	}

	// Group by scenario name for per-scenario files
	byScenario := make(map[string][]StoredScenario)
	now := time.Now()
	for _, s := range scenarios {
		stored := StoredScenario{
			FishID:      s.ID,
			Scenario:    s.Scenario,
			Performance: s.Performance,
			StepCount:   len(s.States),
			RecordedAt:  now,
			States:      s.States,
			Predictions: s.Predictions,
		}
		byScenario[s.Scenario] = append(byScenario[s.Scenario], stored)
	}

	for scenarioName, entries := range byScenario {
		fileName := fmt.Sprintf("swarm_%s.jsonl", sanitizeFileName(scenarioName))
		filePath := filepath.Join(ts.outputDir, fileName)

		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open %s: %w", filePath, err)
		}

		for _, entry := range entries {
			line, err := json.Marshal(entry)
			if err != nil {
				f.Close()
				return fmt.Errorf("marshal scenario: %w", err)
			}
			if _, err := fmt.Fprintln(f, string(line)); err != nil {
				f.Close()
				return fmt.Errorf("write scenario: %w", err)
			}
		}

		if err := f.Close(); err != nil {
			return fmt.Errorf("close %s: %w", filePath, err)
		}
	}

	return nil
}

func sanitizeFileName(name string) string {
	// Replace spaces and special characters for safe filenames
	result := make([]byte, 0, len(name))
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			result = append(result, c)
		} else if c == ' ' {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "unknown"
	}
	return string(result)
}
