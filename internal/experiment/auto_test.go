package experiment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
)

func TestAutoExperimentNilSystemReturnsError(t *testing.T) {
	err := AutoExperiment(context.Background(), AutoExperimentConfig{})
	if err == nil {
		t.Fatal("expected error when System is nil, got nil")
	}
}

type testMonitor struct {
	lastLevel   string
	lastMessage string
}

func (m *testMonitor) Alert(level, category, message string, details map[string]any) {
	m.lastLevel = level
	m.lastMessage = message
}

func TestAutoExperimentMonitorInterface(t *testing.T) {
	m := &testMonitor{}
	cfg := AutoExperimentConfig{Monitor: m}
	_ = cfg
	var _ AutoExperimentMonitor = m // compile-time interface check
}

func TestToCandidate_MatchingAgent(t *testing.T) {
	p := &pendingExperiment{
		ID:            "exp-agent1-1",
		TargetAgentID: "agent1",
		Skill:         "sector_rotation",
		MutationType:  "prompt_tightening",
		BaselineValue: 0.75,
	}

	reg := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent1", Skill: "sector_rotation", Enabled: true},
			{ID: "agent2", Skill: "style_timing", Enabled: true},
		},
	}

	candidate := p.toCandidate(reg)
	if candidate == nil {
		t.Fatal("expected candidate for matching agent, got nil")
	}
	if candidate.Agent.ID != "agent1" {
		t.Errorf("expected agent1, got %s", candidate.Agent.ID)
	}
	if candidate.Scorecard.SharpeLike != 0.75 {
		t.Errorf("expected SharpeLike 0.75, got %v", candidate.Scorecard.SharpeLike)
	}
	if candidate.Experiment.TargetAgentID != "agent1" {
		t.Errorf("expected TargetAgentID agent1, got %s", candidate.Experiment.TargetAgentID)
	}
	if candidate.Experiment.Status != domain.ExperimentPlanned {
		t.Errorf("expected status ExperimentPlanned, got %v", candidate.Experiment.Status)
	}
}

func TestToCandidate_AgentNotFound(t *testing.T) {
	p := &pendingExperiment{
		ID:            "exp-missing-1",
		TargetAgentID: "nonexistent",
	}

	reg := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent1", Skill: "sector_rotation", Enabled: true},
		},
	}

	candidate := p.toCandidate(reg)
	if candidate != nil {
		t.Errorf("expected nil for unknown agent, got %v", candidate)
	}
}

func TestToCandidate_AgentDisabled(t *testing.T) {
	p := &pendingExperiment{
		ID:            "exp-disabled-1",
		TargetAgentID: "agent1",
	}

	reg := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent1", Skill: "sector_rotation", Enabled: false},
		},
	}

	candidate := p.toCandidate(reg)
	if candidate != nil {
		t.Errorf("expected nil for disabled agent, got %v", candidate)
	}
}

func TestToCandidate_EmptyRegistry(t *testing.T) {
	p := &pendingExperiment{
		ID:            "exp-empty-1",
		TargetAgentID: "agent1",
	}

	candidate := p.toCandidate(domain.AgentRegistry{})
	if candidate != nil {
		t.Errorf("expected nil for empty registry, got %v", candidate)
	}
}

func TestLoadOldestPendingExperiment_NoFile(t *testing.T) {
	dir := t.TempDir()
	result := loadOldestPendingExperiment(dir)
	if result != nil {
		t.Errorf("expected nil when no experiments.jsonl, got %v", result)
	}
}

func TestLoadOldestPendingExperiment_NoPlanned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "experiments.jsonl")

	records := []domain.ExperimentRecord{
		{ID: "exp-1", TargetAgentID: "agent1", Status: domain.ExperimentAccepted},
		{ID: "exp-2", TargetAgentID: "agent2", Status: domain.ExperimentRejected},
	}
	writeExperimentJSONL(t, path, records)

	result := loadOldestPendingExperiment(dir)
	if result != nil {
		t.Errorf("expected nil when no planned experiments, got %v", result)
	}
}

func TestLoadOldestPendingExperiment_FindsPlanned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "experiments.jsonl")

	records := []domain.ExperimentRecord{
		{ID: "exp-1", TargetAgentID: "agent1", Status: domain.ExperimentAccepted},
		{ID: "exp-2", TargetAgentID: "agent2", Status: domain.ExperimentPlanned, Skill: "sector", MutationType: "prompt_tightening", BaselineValue: 0.75},
		{ID: "exp-3", TargetAgentID: "agent3", Status: domain.ExperimentPlanned, Skill: "style", MutationType: "risk_rule_change", BaselineValue: 0.50},
	}
	writeExperimentJSONL(t, path, records)

	result := loadOldestPendingExperiment(dir)
	if result == nil {
		t.Fatal("expected to find planned experiment, got nil")
	}
	if result.ID != "exp-2" {
		t.Errorf("expected first planned exp-2, got %s", result.ID)
	}
	if result.BaselineValue != 0.75 {
		t.Errorf("expected BaselineValue 0.75, got %v", result.BaselineValue)
	}
}

func TestLoadOldestPendingExperiment_SkipsEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "experiments.jsonl")

	records := []domain.ExperimentRecord{
		{ID: "exp-1", Status: domain.ExperimentPlanned},
		{ID: "exp-2", TargetAgentID: "agent2", Status: domain.ExperimentPlanned, Skill: "sector"},
	}
	writeExperimentJSONL(t, path, records)

	result := loadOldestPendingExperiment(dir)
	if result == nil {
		t.Fatal("expected to find exp-2, got nil")
	}
	if result.ID != "exp-2" {
		t.Errorf("expected exp-2, got %s", result.ID)
	}
}

func TestLoadOldestPendingExperiment_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "experiments.jsonl")
	os.WriteFile(path, []byte("not valid json\n"), 0o644)

	result := loadOldestPendingExperiment(dir)
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func writeExperimentJSONL(t *testing.T, path string, records []domain.ExperimentRecord) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(b)
		f.Write([]byte("\n"))
	}
}

func TestLoadPendingExperiments(t *testing.T) {
	tmpDir := t.TempDir()
	expDir := filepath.Join(tmpDir, "data", "state", "experiments")
	if err := os.MkdirAll(expDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mkResult := func(id string, status domain.ExperimentStatus) string {
		r := experiment.PromptExperimentResult{
			Experiment: domain.ExperimentRecord{
				ID:              id,
				Status:          status,
				TargetAgentID:   "test-agent",
				Skill:           "test-skill",
				MutationType:    "prompt_tightening",
				AcceptanceGates: []string{"improve_sharpe_like"},
			},
			BaselineObservations:  100,
			CandidateObservations: 100,
			BaselineReturns:       []float64{0.01, 0.02, 0.015},
			CandidateReturns:      []float64{0.02, 0.03, 0.025},
		}
		b, _ := json.Marshal(r)
		return string(b)
	}

	for _, c := range []struct {
		id     string
		status domain.ExperimentStatus
	}{
		{"exp-planned-001", domain.ExperimentPlanned},
		{"exp-accepted-001", domain.ExperimentAccepted},
		{"exp-rejected-001", domain.ExperimentRejected},
	} {
		if err := os.WriteFile(filepath.Join(expDir, c.id+".json"), []byte(mkResult(c.id, c.status)), 0o644); err != nil {
			t.Fatalf("write %s: %v", c.id, err)
		}
	}

	results := LoadPendingExperiments(tmpDir)
	if len(results) != 1 {
		t.Fatalf("expected 1 pending (planned), got %d", len(results))
	}
	if results[0].Experiment.ID != "exp-planned-001" {
		t.Errorf("expected exp-planned-001, got %s", results[0].Experiment.ID)
	}

	if got := LoadPendingExperiments(t.TempDir()); len(got) != 0 {
		t.Errorf("expected empty for non-existent dir, got %d", len(got))
	}
}
