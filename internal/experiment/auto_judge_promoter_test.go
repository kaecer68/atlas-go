package experiment

import (
	"context"
	"testing"
	"time"

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
