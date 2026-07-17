package experiment

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// writeExpRecords appends records to <dir>/experiments.jsonl.
func writeExpRecords(t *testing.T, dir string, recs ...domain.ExperimentRecord) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := ledger.NewStore(dir)
	for _, r := range recs {
		if err := store.RecordExperiment(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadOldestPendingExperiment_SkipsResolved(t *testing.T) {
	dir := t.TempDir()
	writeExpRecords(t, dir,
		domain.ExperimentRecord{ID: "exp-a-1700000001", TargetAgentID: "agent-a", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "exp-b-1700000002", TargetAgentID: "agent-b", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "exp-a-1700000001", TargetAgentID: "agent-a", Status: domain.ExperimentRejected},
	)
	got := loadOldestPendingExperiment(dir)
	if got == nil {
		t.Fatal("expected a pending experiment, got nil")
	}
	if got.ID != "exp-b-1700000002" {
		t.Fatalf("oldest pending = %s, want exp-b-1700000002 (resolved exp-a must be skipped)", got.ID)
	}
}

func TestLoadOldestPendingExperiment_AllResolved(t *testing.T) {
	dir := t.TempDir()
	writeExpRecords(t, dir,
		domain.ExperimentRecord{ID: "exp-a-1700000001", TargetAgentID: "agent-a", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "exp-a-1700000001", TargetAgentID: "agent-a", Status: domain.ExperimentAccepted},
	)
	if got := loadOldestPendingExperiment(dir); got != nil {
		t.Fatalf("expected nil (all resolved), got %+v", got)
	}
}

func TestExpireStalePlanned(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour).Unix()
	fresh := time.Now().Unix()
	writeExpRecords(t, dir,
		domain.ExperimentRecord{ID: fmt.Sprintf("exp-old-%d", old), TargetAgentID: "a", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: fmt.Sprintf("exp-fresh-%d", fresh), TargetAgentID: "b", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "exp-resolved-1700000003", TargetAgentID: "c", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "exp-resolved-1700000003", TargetAgentID: "c", Status: domain.ExperimentAccepted},
	)

	n, err := ExpireStalePlanned(dir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ExpireStalePlanned: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d, want 1 (only the 60-day-old planned)", n)
	}
	if got := ledger.CountUnresolvedPlanned(dir); got != 1 {
		t.Fatalf("unresolved after expiry = %d, want 1 (only the fresh one)", got)
	}
	if got := loadOldestPendingExperiment(dir); got == nil || got.ID != fmt.Sprintf("exp-fresh-%d", fresh) {
		t.Fatalf("oldest pending after expiry = %+v", got)
	}
}

func TestCountUnresolvedPlannedFoldsDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeExpRecords(t, dir,
		domain.ExperimentRecord{ID: "x-1", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "x-1", Status: domain.ExperimentPlanned}, // duplicate planned
		domain.ExperimentRecord{ID: "y-1", Status: domain.ExperimentPlanned},
		domain.ExperimentRecord{ID: "y-1", Status: domain.ExperimentRejected},
	)
	if got := ledger.CountUnresolvedPlanned(dir); got != 1 {
		t.Fatalf("unresolved = %d, want 1", got)
	}
}
