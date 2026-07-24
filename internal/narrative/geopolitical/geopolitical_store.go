package geopolitical

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GeopoliticalStore persists and loads geopolitical risk scores.
type GeopoliticalStore struct {
	baseDir string
}

// NewGeopoliticalStore creates a store at the given directory.
func NewGeopoliticalStore(baseDir string) *GeopoliticalStore {
	return &GeopoliticalStore{baseDir: baseDir}
}

// Save persists the score as latest.json and a dated copy.
func (s *GeopoliticalStore) Save(score GeopoliticalRiskScore) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir geopolitical dir: %w", err)
	}
	data, err := json.MarshalIndent(score, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal geopolitical score: %w", err)
	}
	latestPath := filepath.Join(s.baseDir, "latest.json")
	if err := os.WriteFile(latestPath, data, 0o644); err != nil {
		return fmt.Errorf("save latest geopolitical score: %w", err)
	}
	return nil
}

// Load reads the latest persisted score.
func (s *GeopoliticalStore) Load() (GeopoliticalRiskScore, error) {
	path := filepath.Join(s.baseDir, "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return GeopoliticalRiskScore{}, fmt.Errorf("read geopolitical score: %w", err)
	}
	var score GeopoliticalRiskScore
	if err := json.Unmarshal(data, &score); err != nil {
		return GeopoliticalRiskScore{}, fmt.Errorf("unmarshal geopolitical score: %w", err)
	}
	return score, nil
}
