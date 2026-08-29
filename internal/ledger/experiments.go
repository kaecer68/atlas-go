package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ExperimentsJSONL reads every record in experiments.jsonl (best-effort;
// malformed lines are skipped). Shared by the auto-experiment drain logic and
// the orchestrator's backlog cap.
func ExperimentsJSONL(ledgerDir string) []domain.ExperimentRecord {
	path := filepath.Join(ledgerDir, "experiments.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var records []domain.ExperimentRecord
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			records = append(records, rec)
		}
	}
	return records
}

// LatestExperimentStatusByID folds an append-only record stream into the
// latest status per experiment ID (last write wins).
func LatestExperimentStatusByID(records []domain.ExperimentRecord) map[string]domain.ExperimentStatus {
	latest := make(map[string]domain.ExperimentStatus, len(records))
	for _, rec := range records {
		if rec.ID == "" {
			continue
		}
		latest[rec.ID] = rec.Status
	}
	return latest
}

// CountUnresolvedPlanned returns how many experiment IDs are still in
// "planned" state after folding duplicates (last write wins). Used to cap
// new planned-experiment creation (fix manifest #D01).
func CountUnresolvedPlanned(ledgerDir string) int {
	latest := LatestExperimentStatusByID(ExperimentsJSONL(ledgerDir))
	n := 0
	for _, st := range latest {
		if st == domain.ExperimentPlanned {
			n++
		}
	}
	return n
}
