package experiment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/screener"
)

func TestScreenedRecommendationsFlowThroughExperimentAndJudge(t *testing.T) {
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{
				ID:       "growth-momentum-test",
				Name:     "Test Growth Momentum",
				Layer:    domain.LayerStyle,
				Skill:    "growth_momentum",
				Enabled:  true,
				Universe: []string{"HIGH_VOL.TW", "LOW_VOL.TW"},
				ScreeningCriteria: domain.ScreeningCriteria{
					VolumeIntraday: &domain.MinFilter{Min: int64Ptr(1_000_000)},
				},
			},
		},
	}

	quotes := []domain.Quote{
		{Symbol: "HIGH_VOL.TW", Open: 100, Last: 105, Volume: 5_000_000, IsTradable: true},
		{Symbol: "LOW_VOL.TW", Open: 100, Last: 105, Volume: 100_000, IsTradable: true},
	}

	fe := portfolio.NewFactorEngine()
	fp := portfolio.NewFundamentalProvider()
	scr := screener.NewEngine(fe, fp)
	plugins := orchestrator.NewPluginRegistry().WithScreener(scr)
	policy := baseline.DefaultPolicy().ExecutionPolicy

	_, raw, final, _ := orchestrator.ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins(registry, quotes, nil, policy, plugins)

	foundHighVol := false
	for _, rec := range raw {
		if rec.Symbol == "HIGH_VOL.TW" {
			foundHighVol = true
		}
		if rec.Symbol == "LOW_VOL.TW" {
			t.Fatal("LOW_VOL.TW should have been screened out")
		}
	}
	if !foundHighVol {
		t.Fatal("expected HIGH_VOL.TW to pass screening")
	}
	if len(final) != len(raw) {
		t.Logf("control layer returned %d final recs from %d raw", len(final), len(raw))
	}

	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir)
	asOf := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	outcomes := make([]domain.RecommendationOutcome, 0, len(raw))
	for _, rec := range raw {
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:        rec.Agent,
			Skill:          rec.Skill,
			Layer:          rec.Layer,
			Symbol:         rec.Symbol,
			Side:           rec.Side,
			Conviction:     rec.Conviction,
			Window:         asOf.Format("2006-01-02"),
			ForwardReturn:  0.02,
			BenchmarkDelta: 0.015,
			Hit:            true,
			PassedGuards:   true,
			RecordedAt:     asOf,
		})
	}
	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("record outcomes: %v", err)
	}
	scorecards := ledger.BuildScorecards(outcomes)
	if len(scorecards) == 0 {
		t.Fatal("expected scorecards from screened recommendation outcomes")
	}

	candidate := domain.SelectWeakestAgent(registry, scorecards)
	if candidate == nil {
		t.Fatal("expected evolution candidate from screened recommendations")
	}
	if candidate.Agent.ID != "growth-momentum-test" {
		t.Errorf("expected candidate agent growth-momentum-test, got %s", candidate.Agent.ID)
	}

	brief := domain.BuildMutationBrief("window-20260325-20260327", candidate)
	if brief == nil {
		t.Fatal("expected mutation brief")
	}
	if brief.TargetAgentID != candidate.Agent.ID {
		t.Errorf("expected brief target %s, got %s", candidate.Agent.ID, brief.TargetAgentID)
	}

	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	windows := []prism.TrainingWindow{
		{Start: asOf.AddDate(0, 0, -5), End: asOf, RegimeSet: true},
	}
	if err := pm.ScheduleTraining(candidate.Agent, windows); err != nil {
		t.Fatalf("prism schedule training: %v", err)
	}
	stats := pm.GetOverallStats()
	if stats.TotalTasks == 0 {
		t.Error("expected PRISM to have scheduled tasks")
	}

	promptDir := t.TempDir()
	promptFile := filepath.Join(promptDir, "growth_momentum.md")
	if err := os.WriteFile(promptFile, []byte("require trend confirmation\ndowngrade conviction\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	brief.PromptFile = promptFile

	briefPath := filepath.Join(stateDir, "brief.json")
	briefBytes, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, briefBytes, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	windowPath := filepath.Join(stateDir, "windows", brief.WindowID+".json")
	if err := os.MkdirAll(filepath.Dir(windowPath), 0o755); err != nil {
		t.Fatalf("mkdir windows dir: %v", err)
	}
	window := domain.BacktestWindowSummary{
		WindowID:     brief.WindowID,
		StartDate:    time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
		SessionCount: 3,
		OutcomeCount: 10,
		GeneratedAt:  time.Now(),
	}
	windowBytes, err := json.Marshal(window)
	if err != nil {
		t.Fatalf("marshal window: %v", err)
	}
	if err := os.WriteFile(windowPath, windowBytes, 0o644); err != nil {
		t.Fatalf("write window: %v", err)
	}

	baselinePath := filepath.Join(stateDir, "baseline_policy.json")
	baselinePolicy := baseline.DefaultPolicy()
	baselineBytes, _ := json.Marshal(baselinePolicy)
	if err := os.WriteFile(baselinePath, baselineBytes, 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	wd, _ := os.Getwd()
	root := filepath.Clean(filepath.Join(wd, "../.."))
	replayPath := filepath.Join(root, "samples", "replay", "twse_stock_day_all_sample.csv")
	if _, err := os.Stat(replayPath); err != nil {
		t.Skipf("sample replay data missing, skipping judge integration: %v", err)
	}

	exec := experiment.NewExecutor(store.(ledger.FullStore), baselinePath)
	execResult, err := exec.Run(briefPath, replayPath)
	if err != nil {
		t.Fatalf("experiment run: %v", err)
	}
	if execResult.Experiment.Status != domain.ExperimentRunning {
		t.Errorf("expected experiment status running, got %s", execResult.Experiment.Status)
	}
	if execResult.CandidatePrompt == "" {
		t.Fatal("expected candidate prompt path from experiment executor")
	}

	resultPath := filepath.Join(stateDir, "experiments", "e2e-experiment.json")
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		t.Fatalf("mkdir result dir: %v", err)
	}
	judgeResult := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:               execResult.Experiment.ID,
			TargetAgentID:    candidate.Agent.ID,
			Skill:            candidate.Agent.Skill,
			MutationType:     brief.MutationType,
			AcceptanceMetric: brief.AcceptanceMetric,
			AcceptanceGates:  brief.AcceptanceGates,
			Status:           domain.ExperimentRunning,
		},
		Brief: domain.MutationBrief{
			WindowID:            brief.WindowID,
			TargetAgentID:       brief.TargetAgentID,
			TargetSkill:         brief.TargetSkill,
			TargetLayer:         brief.TargetLayer,
			PromptFile:          brief.PromptFile,
			MutationType:        brief.MutationType,
			AcceptanceMetric:    brief.AcceptanceMetric,
			AcceptanceGates:     brief.AcceptanceGates,
			ForbiddenActions:    brief.ForbiddenActions,
			RequiredSkills:      brief.RequiredSkills,
			ObservedWindowCount: brief.ObservedWindowCount,
			MaturityLevel:       brief.MaturityLevel,
		},
		CandidatePrompt: execResult.CandidatePrompt,
		EvaluationMode:  "policy_checked_pending_replay",
		PolicyChecks:    execResult.PolicyChecks,
	}
	resultBytes, _ := json.Marshal(judgeResult)
	if err := os.WriteFile(resultPath, resultBytes, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	judge := experiment.NewJudge(store.(ledger.ExperimentStore), replayPath, baselinePath)
	judged, err := judge.Evaluate(resultPath)
	if err != nil {
		t.Fatalf("judge evaluate: %v", err)
	}
	if judged.Experiment.Status != domain.ExperimentAccepted && judged.Experiment.Status != domain.ExperimentRejected {
		t.Errorf("expected judge to reach accepted or rejected, got %s", judged.Experiment.Status)
	}
}

//go:fix inline
func int64Ptr(v int64) *int64 {
	return new(v)
}
