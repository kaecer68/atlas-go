package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRecordAndLoadOutcomes(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	outcomes := []domain.RecommendationOutcome{
		{AgentID: "a1", Skill: "s1", Window: "1d", ForwardReturn: 0.01, Hit: true, RecordedAt: time.Now()},
		{AgentID: "a2", Skill: "s2", Window: "5d", ForwardReturn: -0.02, Hit: false, RecordedAt: time.Now()},
	}

	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("record outcomes: %v", err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("load outcomes: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(loaded))
	}
	if loaded[0].AgentID != "a1" {
		t.Fatalf("expected first outcome agent a1, got %s", loaded[0].AgentID)
	}
}

func TestRecordAndLoadSessionOutcomes(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	session := domain.ReplaySession{ID: "session-1"}

	outcomes := []domain.RecommendationOutcome{
		{AgentID: "a1", Skill: "s1", Window: "1d", ForwardReturn: 0.05, Hit: true, RecordedAt: time.Now()},
	}

	if err := store.RecordSessionOutcomes(session, outcomes); err != nil {
		t.Fatalf("record session outcomes: %v", err)
	}

	path := filepath.Join(dir, "sessions", session.ID, "recommendation_outcomes.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected session outcomes file to exist: %v", err)
	}
}

func TestLoadOutcomesMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("load missing outcomes: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for missing outcomes, got %v", loaded)
	}
}

func TestRecordAndUpdatePromptExperimentResult(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:     "exp-1",
			Status: domain.ExperimentPlanned,
		},
		CandidatePrompt: "prompts/experiments/test/v2.md",
		EvaluationMode:  "test",
		RecordedAt:      time.Now(),
	}

	if err := store.RecordPromptExperimentResult("exp-1", result); err != nil {
		t.Fatalf("record prompt experiment result: %v", err)
	}

	result.Experiment.Status = domain.ExperimentAccepted
	if err := store.UpdatePromptExperimentResult("exp-1", result); err != nil {
		t.Fatalf("update prompt experiment result: %v", err)
	}

	path := filepath.Join(dir, "experiments", "exp-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read experiment file: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty experiment file")
	}
}

func TestRecordExperimentJSONL(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	record := domain.ExperimentRecord{
		ID:     "exp-2",
		Status: domain.ExperimentRunning,
	}

	if err := store.RecordExperiment(record); err != nil {
		t.Fatalf("record experiment: %v", err)
	}

	path := filepath.Join(dir, "experiments.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected experiments.jsonl to exist: %v", err)
	}
}

func TestRecordSessionExperiment(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	session := domain.ReplaySession{ID: "session-exp"}

	record := domain.ExperimentRecord{
		ID:     "exp-session-1",
		Status: domain.ExperimentPlanned,
	}

	if err := store.RecordSessionExperiment(session, record); err != nil {
		t.Fatalf("record session experiment: %v", err)
	}

	path := filepath.Join(dir, "sessions", session.ID, "experiments.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected session experiments file to exist: %v", err)
	}
}

func TestRecordMutationBrief(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	brief := domain.MutationBrief{
		WindowID:      "w1",
		TargetAgentID: "agent-1",
		MutationType:  "prompt_tightening",
		GeneratedAt:   time.Now(),
	}

	if err := store.RecordMutationBrief("w1", brief); err != nil {
		t.Fatalf("record mutation brief: %v", err)
	}

	path := filepath.Join(dir, "windows", "w1-mutation-brief.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected mutation brief file to exist: %v", err)
	}
}

func TestRecordAndLoadSpawnRecords(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	record := SpawnRecord{
		AgentID:    "spawn-1",
		GapID:      "gap-1",
		GapPattern: "pattern-1",
		CreatedAt:  time.Now(),
		FinalFate:  "active",
	}

	if err := store.RecordSpawnRecord(record); err != nil {
		t.Fatalf("record spawn record: %v", err)
	}

	loaded, err := store.LoadSpawnRecords()
	if err != nil {
		t.Fatalf("load spawn records: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 spawn record, got %d", len(loaded))
	}
	if loaded[0].AgentID != "spawn-1" {
		t.Fatalf("unexpected agent id: %s", loaded[0].AgentID)
	}
}

func TestRecordAndLoadHumanInterventions(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	intervention := domain.HumanIntervention{
		ID:            "hi-1",
		Type:          "pause_agent",
		TargetAgentID: "agent-1",
		Reason:        "market volatility",
		Operator:      "admin",
		RecordedAt:    time.Now(),
	}

	if err := store.RecordHumanIntervention(intervention); err != nil {
		t.Fatalf("record human intervention: %v", err)
	}

	loaded, err := store.LoadHumanInterventions()
	if err != nil {
		t.Fatalf("load human interventions: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 intervention, got %d", len(loaded))
	}
	if loaded[0].Type != "pause_agent" {
		t.Fatalf("unexpected type: %s", loaded[0].Type)
	}
}

func TestLoadSessionSummaries(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	session := domain.ReplaySession{ID: "session-sum-1"}

	summary := domain.SessionSummary{
		SessionID:      session.ID,
		Regime:         domain.RegimeRiskOn,
		EndingCash:     100_000,
		PortfolioValue: 1_000_000,
		ProposalID:     "p1",
		RecordedAt:     time.Now(),
	}

	if err := store.RecordSessionSummary(session, summary); err != nil {
		t.Fatalf("record session summary: %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("load session summaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].ProposalID != "p1" {
		t.Fatalf("unexpected proposal id: %s", summaries[0].ProposalID)
	}
}

func TestLoadSessionSummariesMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("load missing summaries: %v", err)
	}
	if summaries != nil {
		t.Fatalf("expected nil for missing summaries, got %v", summaries)
	}
}

// TestLoadSessionSummariesSkipsCorruptedPascalCase verifies the load guard
// that drops summary.json files whose JSON casing does not match the
// domain.SessionSummary snake_case struct tags. Such files silently decode
// to all zero values (SessionID="", PortfolioValue=0, …). Without the guard
// they would sort first (empty < "session-…") and break the
// reporting.GenerateReport period=all branch (start_date=0001-01-01,
// starting_value=0).
//
// We simulate the corruption by writing the summary with PascalCase keys,
// mirroring the historical encoding that was used before PR #681
// (postgres_audit.go) added struct tag alignment to the audit path.
func TestLoadSessionSummariesSkipsCorruptedPascalCase(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	// 1) One healthy session — written via the normal RecordSessionSummary
	//    path so the struct tags produce the canonical snake_case JSON.
	good := domain.ReplaySession{ID: "session-20260601-daily"}
	if err := store.RecordSessionSummary(good, domain.SessionSummary{
		SessionID:      good.ID,
		Regime:         domain.RegimeRiskOn,
		PortfolioValue: 2_500_000,
		EndingCash:     500_000,
		OutcomeCount:   10,
		TotalTaxPaid:   1500,
		RecordedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("record healthy summary: %v", err)
	}

	// 2) One corrupted session — hand-written with PascalCase keys. The
	//    decoder silently drops every key, leaving SessionID="".
	corruptDir := filepath.Join(dir, "sessions", "session-20260326-daily")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatalf("mkdir corrupt session: %v", err)
	}
	pascalJSON := []byte(`{
  "SessionID": "session-20260326-daily",
  "Regime": "NEUTRAL",
  "OrderCount": 0,
  "PositionCount": 0,
  "EndingCash": 0,
  "PortfolioValue": 0,
  "OutcomeCount": 0,
  "RecordedAt": "2026-06-27T22:25:54Z"
}`)
	if err := os.WriteFile(filepath.Join(corruptDir, "summary.json"), pascalJSON, 0o644); err != nil {
		t.Fatalf("write corrupt summary: %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("load summaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (corrupted one skipped), got %d: %+v", len(summaries), summaries)
	}
	if summaries[0].SessionID != good.ID {
		t.Fatalf("expected only healthy summary %q, got %q", good.ID, summaries[0].SessionID)
	}
	if summaries[0].PortfolioValue != 2_500_000 {
		t.Fatalf("expected healthy portfolio value 2,500,000, got %f", summaries[0].PortfolioValue)
	}
}

func TestLoadOutcomeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outcomes.jsonl")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("create outcome file: %v", err)
	}
	f.WriteString(`{"AgentID":"a1","Skill":"s1","Window":"1d","ForwardReturn":0.01,"Hit":true}` + "\n")
	f.Close()

	outcomes, err := loadOutcomeFile(path)
	if err != nil {
		t.Fatalf("load outcome file: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
}

func TestLoadOutcomeFileMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.jsonl")

	outcomes, err := loadOutcomeFile(path)
	if err != nil {
		t.Fatalf("load missing outcome file: %v", err)
	}
	if outcomes != nil {
		t.Fatalf("expected nil for missing file, got %v", outcomes)
	}
}
