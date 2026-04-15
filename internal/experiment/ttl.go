package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// DefaultExperimentTTL is the maximum duration an experiment may stay in planned/running.
const DefaultExperimentTTL = 24 * time.Hour

// ExpireOldExperiments scans the experiments directory and transitions any
// planned/running experiments older than ttl to expired.
func ExpireOldExperiments(experimentsDir string, ttl time.Duration) (int, error) {
	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	expired := 0
	cutoff := time.Now().Add(-ttl)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(experimentsDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			continue
		}
		if result.Experiment.Status != domain.ExperimentPlanned && result.Experiment.Status != domain.ExperimentRunning {
			continue
		}
		if result.RecordedAt.IsZero() || result.RecordedAt.After(cutoff) {
			continue
		}
		if err := domain.TransitionExperimentStatus(&result.Experiment, domain.ExperimentExpired); err != nil {
			continue
		}
		result.Experiment.RevertReason = fmt.Sprintf("Auto-expired after %v", ttl)
		result.RecordedAt = time.Now()
		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			continue
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			continue
		}
		expired++
	}
	return expired, nil
}
