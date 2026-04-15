package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestExpireOldExperiments(t *testing.T) {
	dir := t.TempDir()

	// fresh planned experiment
	fresh := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:     "exp-fresh",
			Status: domain.ExperimentPlanned,
		},
		RecordedAt: time.Now(),
	}
	writeExp(t, dir, "exp-fresh.json", fresh)

	// stale running experiment
	stale := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:     "exp-stale",
			Status: domain.ExperimentRunning,
		},
		RecordedAt: time.Now().Add(-25 * time.Hour),
	}
	writeExp(t, dir, "exp-stale.json", stale)

	// already rejected experiment (should not be touched)
	rejected := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:     "exp-rejected",
			Status: domain.ExperimentRejected,
		},
		RecordedAt: time.Now().Add(-48 * time.Hour),
	}
	writeExp(t, dir, "exp-rejected.json", rejected)

	count, err := ExpireOldExperiments(dir, DefaultExperimentTTL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 expired, got %d", count)
	}

	// verify stale is now expired
	staleBytes, _ := os.ReadFile(filepath.Join(dir, "exp-stale.json"))
	var staleResult domain.PromptExperimentResult
	if err := json.Unmarshal(staleBytes, &staleResult); err != nil {
		t.Fatalf("unmarshal stale: %v", err)
	}
	if staleResult.Experiment.Status != domain.ExperimentExpired {
		t.Fatalf("expected expired, got %s", staleResult.Experiment.Status)
	}
	if staleResult.Experiment.RevertReason == "" {
		t.Fatalf("expected revert reason to be set")
	}

	// verify fresh is still planned
	freshBytes, _ := os.ReadFile(filepath.Join(dir, "exp-fresh.json"))
	var freshResult domain.PromptExperimentResult
	if err := json.Unmarshal(freshBytes, &freshResult); err != nil {
		t.Fatalf("unmarshal fresh: %v", err)
	}
	if freshResult.Experiment.Status != domain.ExperimentPlanned {
		t.Fatalf("expected planned, got %s", freshResult.Experiment.Status)
	}
}

func writeExp(t *testing.T, dir, name string, result domain.PromptExperimentResult) {
	t.Helper()
	b, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
