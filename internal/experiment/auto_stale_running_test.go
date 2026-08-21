package experiment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// TestExpireStaleRunning mirrors TestExpireStalePlanned: only experiments
// whose latest status is "running" and whose oldest record predates the TTL
// get an appended "expired" record (audit A5: stuck running).
func TestExpireStaleRunning(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour).Unix()
	fresh := time.Now().Unix()
	writeExpRecords(
		t, dir,
		domain.ExperimentRecord{ID: fmt.Sprintf("exp-stuck-%d", old), TargetAgentID: "a", Skill: "tech", MutationType: "m1", Status: domain.ExperimentRunning},
		domain.ExperimentRecord{ID: fmt.Sprintf("exp-fresh-%d", fresh), TargetAgentID: "b", Status: domain.ExperimentRunning},
		domain.ExperimentRecord{ID: "exp-resolved-1700000003", TargetAgentID: "c", Status: domain.ExperimentRunning},
		domain.ExperimentRecord{ID: "exp-resolved-1700000003", TargetAgentID: "c", Status: domain.ExperimentAccepted},
	)

	n, err := ExpireStaleRunning(dir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ExpireStaleRunning: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d, want 1 (only the 60-day-old running)", n)
	}

	latest := ledger.LatestExperimentStatusByID(ledger.ExperimentsJSONL(dir))
	if got := latest[fmt.Sprintf("exp-stuck-%d", old)]; got != domain.ExperimentExpired {
		t.Fatalf("stuck status after expiry = %s, want expired", got)
	}
	if got := latest[fmt.Sprintf("exp-fresh-%d", fresh)]; got != domain.ExperimentRunning {
		t.Fatalf("fresh status = %s, want running (untouched)", got)
	}
	if got := latest["exp-resolved-1700000003"]; got != domain.ExperimentAccepted {
		t.Fatalf("resolved status = %s, want accepted (untouched)", got)
	}
}

// TestExpireStaleRunning_NoStaleReturnsZero verifies the no-op path.
func TestExpireStaleRunning_NoStaleReturnsZero(t *testing.T) {
	dir := t.TempDir()
	fresh := time.Now().Unix()
	writeExpRecords(
		t, dir,
		domain.ExperimentRecord{ID: fmt.Sprintf("exp-fresh-%d", fresh), Status: domain.ExperimentRunning},
	)
	n, err := ExpireStaleRunning(dir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ExpireStaleRunning: %v", err)
	}
	if n != 0 {
		t.Fatalf("expired %d, want 0", n)
	}
}

// TestLoadPendingExperiments_SkipsStuckRunning verifies LoadPendingExperiments
// no longer feeds stuck-running (>30d) or expired result files to the
// auto-judge pipeline (audit A5).
func TestLoadPendingExperiments_SkipsStuckRunning(t *testing.T) {
	workDir := t.TempDir()
	expDir := filepath.Join(workDir, "data", "state", "experiments")
	if err := os.MkdirAll(expDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Stuck running: recorded 40 days ago → must be skipped.
	writeExp(t, expDir, "stuck.json", domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exp-stuck", Status: domain.ExperimentRunning},
		RecordedAt: time.Now().Add(-40 * 24 * time.Hour),
	})
	// Fresh running: should still be returned as pending.
	writeExp(t, expDir, "fresh.json", domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exp-fresh", Status: domain.ExperimentRunning},
		RecordedAt: time.Now().Add(-1 * 24 * time.Hour),
	})
	// Accepted / expired: skipped.
	writeExp(t, expDir, "accepted.json", domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exp-accepted", Status: domain.ExperimentAccepted},
		RecordedAt: time.Now().Add(-1 * 24 * time.Hour),
	})
	writeExp(t, expDir, "expired.json", domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exp-expired", Status: domain.ExperimentExpired},
		RecordedAt: time.Now().Add(-1 * 24 * time.Hour),
	})

	pending := LoadPendingExperiments(workDir)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending (fresh running), got %d: %+v", len(pending), pending)
	}
	if pending[0].Experiment.ID != "exp-fresh" {
		t.Fatalf("expected exp-fresh, got %s", pending[0].Experiment.ID)
	}
}

// TestAutoJudgePromoter_RunDaily_SkipsStuckRunning verifies the defense-in-depth
// guard in RunDaily: a stuck-running result is never evaluated (judge stays nil).
func TestAutoJudgePromoter_RunDaily_SkipsStuckRunning(t *testing.T) {
	p := NewAutoJudgePromoter(nil, nil) // judge nil: guard must fire before any judge use
	results, err := p.RunDaily(context.Background(), []experiment.PromptExperimentResult{
		{Experiment: domain.ExperimentRecord{ID: "exp-stuck", Status: domain.ExperimentRunning},
			RecordedAt: time.Now().Add(-40 * 24 * time.Hour)},
		{Experiment: domain.ExperimentRecord{ID: "exp-expired", Status: domain.ExperimentExpired},
			RecordedAt: time.Now().Add(-1 * 24 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 evaluations for stuck/expired, got %d", len(results))
	}
}
