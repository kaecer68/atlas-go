package experiment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestAutoJudgePromoter_BurnInSkips(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	judge := NewJudge(nil, "", "")
	p := NewAutoJudgePromoter(judge, nil).WithMaturityTracker(tr)

	pending := []experiment.PromptExperimentResult{
		{Experiment: experiment.ExperimentRecord{ID: "exp-001"}},
	}
	results, err := p.RunDaily(context.Background(), pending)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results during burn_in, got %d", len(results))
	}
}

func TestAutoJudgePromoter_NotJudgeable(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	judge := NewJudge(nil, "", "")
	p := NewAutoJudgePromoter(judge, nil).WithMaturityTracker(tr)

	// Too few observations
	pending := []experiment.PromptExperimentResult{
		{
			Experiment:            experiment.ExperimentRecord{ID: "exp-001"},
			BaselineObservations:  10,
			CandidateObservations: 10,
			RecordedAt:            time.Now().Add(-7 * 24 * time.Hour),
		},
	}
	results, err := p.RunDaily(context.Background(), pending)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (not judgeable), got %d", len(results))
	}
}

func TestAutoJudgePromoter_JudgeableButTooRecent(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	judge := NewJudge(nil, "", "")
	p := NewAutoJudgePromoter(judge, nil).WithMaturityTracker(tr)

	// Enough observations but recorded today
	pending := []experiment.PromptExperimentResult{
		{
			Experiment:            experiment.ExperimentRecord{ID: "exp-001"},
			BaselineObservations:  100,
			CandidateObservations: 100,
			RecordedAt:            time.Now(),
		},
	}
	results, err := p.RunDaily(context.Background(), pending)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (too recent), got %d", len(results))
	}
}

func TestAutoJudgePromoter_AutoRejectsDuringBurnIn(t *testing.T) {
	// Even with judgeable data, burn_in rejects
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	store := ledger.NewStore(t.TempDir()).(ledger.ExperimentStore)
	judge := NewJudge(store, "", "").WithMaturityTracker(tr)
	p := NewAutoJudgePromoter(judge, nil).WithMaturityTracker(tr)

	pending := []experiment.PromptExperimentResult{
		makeJudgeableResult("exp-001", 100, 100),
	}
	results, err := p.RunDaily(context.Background(), pending)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results during burn_in, got %d", len(results))
	}
}

func TestAutoJudgePromoter_CooldownBlocksSecondPromote(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	judge := NewJudge(nil, "", "")
	p := NewAutoJudgePromoter(judge, nil).
		WithMaturityTracker(tr).
		WithMinObservations(2) // lower threshold for testing

	// Manually set last promote to now
	p.lastPromote = time.Now()

	pending := []experiment.PromptExperimentResult{
		makeJudgeableResult("exp-001", 100, 100),
	}
	results, err := p.RunDaily(context.Background(), pending)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AutoPromoted {
		t.Error("expected AutoPromoted=false due to cooldown")
	}
}

func TestAutoRevertCallsBaselineMgrRevert(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	// Create a policy at version 2 (has 1 promotion) so RevertLast has something to revert.
	policy := baseline.DefaultPolicy()
	policy.Version = 2
	policy.Promotions = []baseline.PromotionRecord{
		{
			ExperimentID:  "exp-promoted-001",
			TargetAgentID: "test-agent",
			TargetSkill:   "test-skill",
			MutationType:  "prompt_tightening",
			PromotedAt:    time.Now().Add(-48 * time.Hour),
			VersionAfter:  2,
		},
	}
	if err := baseline.Save(policyPath, policy); err != nil {
		t.Fatalf("save baseline policy: %v", err)
	}

	// Mature tracker so burn-in does not skip processing.
	mt := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	judge := NewJudge(nil, "", "").WithMaturityTracker(mt)
	mgr := baseline.NewManager(policyPath)
	p := NewAutoJudgePromoter(judge, mgr).
		WithMaturityTracker(mt).
		WithMinObservations(2)

	// Experiment that passes isJudgeable but will be REJECTED (candidate ≤ baseline).
	pending := []experiment.PromptExperimentResult{
		{
			Experiment: experiment.ExperimentRecord{
				ID:              "exp-rejected-001",
				AcceptanceGates: []string{"improve_sharpe_like"},
				BaselineValue:   0.02,
				CandidateValue:  0.01, // ≤ baseline → rejected
			},
			BaselineObservations:  100,
			CandidateObservations: 100,
			RecordedAt:            time.Now().Add(-7 * 24 * time.Hour),
			BaselineReturns:       []float64{0.01, 0.02, 0.015},
			CandidateReturns:      []float64{0.005, 0.01, 0.008},
		},
	}

	results, err := p.RunDaily(context.Background(), pending)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Fatal("expected experiment to be rejected")
	}

	// Verify that autoRevert called baselineMgr.Revert — policy now has revert history
	// and version decreased from 2 to 1.
	loadedPolicy, err := baseline.Load(policyPath)
	if err != nil {
		t.Fatalf("load policy after revert: %v", err)
	}
	if len(loadedPolicy.RevertHistory) != 1 {
		t.Errorf("expected 1 revert history entry, got %d", len(loadedPolicy.RevertHistory))
	}
	if loadedPolicy.Version != 1 {
		t.Errorf("expected policy version 1 after revert from version 2, got %d", loadedPolicy.Version)
	}
}

func TestAutoExperimentPromotionRespectsCooldown(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")
	candPath := filepath.Join(tmpDir, "cand_prompt.txt")

	// Default policy at version 1.
	policy := baseline.DefaultPolicy()
	if err := baseline.Save(policyPath, policy); err != nil {
		t.Fatalf("save baseline policy: %v", err)
	}
	// Candidate prompt file.
	if err := os.WriteFile(candPath, []byte("improved prompt content"), 0o644); err != nil {
		t.Fatalf("write candidate prompt: %v", err)
	}

	judge := NewJudge(nil, "", "")
	mgr := baseline.NewManager(policyPath)
	p := NewAutoJudgePromoter(judge, mgr).
		WithMinObservations(2)

	// First promotion attempt — should succeed.
	expPath1 := filepath.Join(tmpDir, "exp_result1.json")
	r1 := makeAcceptedResult("exp-001", candPath)
	writeResult(t, expPath1, r1)

	promoted, note, err := p.TryPromoteFromPath(expPath1)
	if err != nil {
		t.Fatalf("first TryPromoteFromPath: %v", err)
	}
	if !promoted {
		t.Fatalf("first promotion should succeed, got note: %s", note)
	}

	// Second promotion immediately after — cooldown should block.
	expPath2 := filepath.Join(tmpDir, "exp_result2.json")
	r2 := makeAcceptedResult("exp-002", candPath)
	writeResult(t, expPath2, r2)

	promoted2, note2, err := p.TryPromoteFromPath(expPath2)
	if err != nil {
		t.Fatalf("second TryPromoteFromPath: %v", err)
	}
	if promoted2 {
		t.Fatal("second promotion within cooldown should have been blocked")
	}
	if !containsCooldown(note2) {
		t.Errorf("expected cooldown message, got: %s", note2)
	}

	// Verify that only ONE promotion was recorded.
	loaded, err := baseline.Load(policyPath)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if len(loaded.Promotions) != 1 {
		t.Errorf("expected 1 promotion (cooldown blocked second), got %d", len(loaded.Promotions))
	}
}

// Test helpers.

func makeAcceptedResult(id, candPromptPath string) experiment.PromptExperimentResult {
	r := makeJudgeableResult(id, 100, 100)
	r.Experiment.Status = domain.ExperimentAccepted
	r.Experiment.TargetAgentID = "test-agent"
	r.Experiment.MutationType = "prompt_tightening"
	r.CandidatePrompt = candPromptPath
	return r
}

func writeResult(t *testing.T, path string, r experiment.PromptExperimentResult) {
	t.Helper()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}
}

func containsCooldown(s string) bool {
	return strings.Contains(s, "cooldown")
}

func makeJudgeableResult(id string, baseObs, candObs int) experiment.PromptExperimentResult {
	return experiment.PromptExperimentResult{
		Experiment: experiment.ExperimentRecord{
			ID:              id,
			AcceptanceGates: []string{"improve_sharpe_like"},
			BaselineValue:   0.01,
			CandidateValue:  0.02,
		},
		BaselineObservations:  baseObs,
		CandidateObservations: candObs,
		RecordedAt:            time.Now().Add(-7 * 24 * time.Hour),
		BaselineReturns:       []float64{0.01, 0.02, 0.015, 0.01, 0.02},
		CandidateReturns:      []float64{0.02, 0.03, 0.025, 0.02, 0.03},
	}
}

type fakeRecorder struct {
	calls []struct {
		experimentID string
		sharpe       float64
	}
}

func (f *fakeRecorder) RecordPromotion(experimentID string, prePromotionSharpe float64) {
	f.calls = append(f.calls, struct {
		experimentID string
		sharpe       float64
	}{experimentID, prePromotionSharpe})
}

func TestAutoJudgePromoter_RecordsPromotionOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	policy := baseline.DefaultPolicy()
	if err := baseline.Save(policyPath, policy); err != nil {
		t.Fatalf("save baseline policy: %v", err)
	}

	judge := NewJudge(nil, "", "")
	mgr := baseline.NewManager(policyPath)
	rec := &fakeRecorder{}

	p := NewAutoJudgePromoter(judge, mgr).
		WithMinObservations(2).
		WithPromotionRecorder(rec)

	candPath := filepath.Join(tmpDir, "candidate.txt")
	if err := os.WriteFile(candPath, []byte("test candidate prompt"), 0o644); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	result := makeAcceptedResult("exp-record-001", candPath)

	if err := p.autoPromote(result); err != nil {
		t.Fatalf("autoPromote: %v", err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 RecordPromotion call, got %d", len(rec.calls))
	}
	if rec.calls[0].experimentID != "exp-record-001" {
		t.Errorf("expected ID 'exp-record-001', got %q", rec.calls[0].experimentID)
	}
}
