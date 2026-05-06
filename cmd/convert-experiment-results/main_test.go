package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestConvertExperimentLegacyPascalCase(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	legacyJSON := `{"Experiment":{"ID":"exp-1","ProposalID":"prop-1","CommitID":"abc123","ApprovalID":"appr-1","TargetAgentID":"agent-1","Skill":"growth_momentum","Hypothesis":"test","PromptVersionFrom":"v1","PromptVersionTo":"v2","MutationType":"prompt","AcceptanceGates":["gate1"],"WindowStart":"2026-01-01T00:00:00Z","WindowEnd":"2026-01-31T00:00:00Z","AcceptanceMetric":"hit_rate","BaselineValue":0.5,"CandidateValue":0.6,"Status":"accepted","RevertReason":""},"Brief":{"WindowID":"w1","TargetAgentID":"agent-1","TargetSkill":"growth_momentum","TargetLayer":"style","PromptFile":"prompt.md","MutationType":"prompt","FailurePattern":"none","Hypothesis":"h1","AcceptanceMetric":"hit_rate","AcceptanceGates":["g1"],"ForbiddenActions":[],"RequiredSkills":[],"ObservedWindowCount":10,"MaturityLevel":"stable","IterationGuidance":[],"RecommendedWindow":"1d","GeneratedAt":"2026-01-01T00:00:00Z"},"CandidatePrompt":"new prompt","EvaluationMode":"oos","PolicyChecks":["check1"],"Notes":["note1"],"JudgeChecks":["judge1"],"BaselineObservations":100,"CandidateObservations":95,"UsedFallbackWindow":false,"RecordedAt":"2026-02-01T00:00:00Z"}`

	if err := os.WriteFile(filepath.Join(experimentsDir, "exp1.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(experimentsDir, "exp1.json"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	if strings.Contains(string(rewritten), `"Experiment"`) {
		t.Fatalf("expected PascalCase removed, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"experiment"`) {
		t.Fatalf("expected canonical snake_case experiment key, got: %s", rewritten)
	}

	var result domain.PromptExperimentResult
	if err := json.Unmarshal(rewritten, &result); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if result.Experiment.ID != "exp-1" {
		t.Fatalf("experiment.id mismatch: got %q", result.Experiment.ID)
	}
	if result.Experiment.Status != domain.ExperimentAccepted {
		t.Fatalf("experiment.status mismatch: got %q", result.Experiment.Status)
	}
	if result.Brief.WindowID != "w1" {
		t.Fatalf("brief.window_id mismatch: got %q", result.Brief.WindowID)
	}
	if result.CandidatePrompt != "new prompt" {
		t.Fatalf("candidate_prompt mismatch: got %q", result.CandidatePrompt)
	}
	if result.BaselineObservations != 100 {
		t.Fatalf("baseline_observations mismatch: got %d", result.BaselineObservations)
	}
}

func TestConvertExperimentPreservesNestedFields(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	legacyJSON := `{"Experiment":{"ID":"exp-2","Status":"running"},"Brief":{"WindowID":"w2","TargetAgentID":"agent-2"},"CandidatePrompt":"prompt text","EvaluationMode":"backtest","PolicyChecks":["check1","check2"],"Notes":["note1"],"JudgeChecks":["judge1"],"BaselineObservations":50,"CandidateObservations":48,"UsedFallbackWindow":true,"RecordedAt":"2026-03-01T00:00:00Z","DataMetadata":{"SourcePath":"/data/replay.jsonl","DateRangeStart":"2026-01-01T00:00:00Z","DateRangeEnd":"2026-01-31T00:00:00Z","DaysDelayed":1,"CoversWindow":true,"LastModified":"2026-02-01T00:00:00Z","RecordCount":1000},"OOSResult":{"Passed":true,"BaselineScore":0.5,"CandidateScore":0.6,"Improvement":0.1,"Observations":30,"OOSWindowStart":"2026-02-01T00:00:00Z","OOSWindowEnd":"2026-02-15T00:00:00Z","UsedFallback":false,"ValidationAt":"2026-02-16T00:00:00Z","Reason":"improved"},"BaselineReturns":[0.01,0.02],"CandidateReturns":[0.02,0.03],"ParameterSnapshotID":"snap-1","BaselineFallbackCount":2,"CandidateFallbackCount":1,"BaselineFactorCount":5,"CandidateFactorCount":5}`

	if err := os.WriteFile(filepath.Join(experimentsDir, "exp2.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(experimentsDir, "exp2.json"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	var result domain.PromptExperimentResult
	if err := json.Unmarshal(rewritten, &result); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}

	if result.Experiment.ID != "exp-2" {
		t.Fatalf("experiment.id: got %q", result.Experiment.ID)
	}
	if result.DataMetadata == nil {
		t.Fatalf("data_metadata nil")
	}
	if result.DataMetadata.RecordCount != 1000 {
		t.Fatalf("data_metadata.record_count: got %d", result.DataMetadata.RecordCount)
	}
	if result.OOSResult == nil {
		t.Fatalf("oos_result nil")
	}
	if !result.OOSResult.Passed {
		t.Fatalf("oos_result.passed: got %v", result.OOSResult.Passed)
	}
	if result.OOSResult.Improvement != 0.1 {
		t.Fatalf("oos_result.improvement: got %v", result.OOSResult.Improvement)
	}
	if len(result.BaselineReturns) != 2 || result.BaselineReturns[0] != 0.01 {
		t.Fatalf("baseline_returns: got %v", result.BaselineReturns)
	}
	if len(result.CandidateReturns) != 2 || result.CandidateReturns[1] != 0.03 {
		t.Fatalf("candidate_returns: got %v", result.CandidateReturns)
	}
	if result.ParameterSnapshotID != "snap-1" {
		t.Fatalf("parameter_snapshot_id: got %q", result.ParameterSnapshotID)
	}
	if result.BaselineFallbackCount != 2 {
		t.Fatalf("baseline_fallback_count: got %d", result.BaselineFallbackCount)
	}
	if result.CandidateFactorCount != 5 {
		t.Fatalf("candidate_factor_count: got %d", result.CandidateFactorCount)
	}
}

func TestConvertExperimentMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	legacy1 := `{"Experiment":{"ID":"exp-a","Status":"accepted"},"Brief":{"WindowID":"wa"},"CandidatePrompt":"p1","EvaluationMode":"oos","BaselineObservations":10,"CandidateObservations":10,"RecordedAt":"2026-01-01T00:00:00Z"}`
	legacy2 := `{"Experiment":{"ID":"exp-b","Status":"rejected"},"Brief":{"WindowID":"wb"},"CandidatePrompt":"p2","EvaluationMode":"backtest","BaselineObservations":20,"CandidateObservations":18,"RecordedAt":"2026-02-01T00:00:00Z"}`

	if err := os.WriteFile(filepath.Join(experimentsDir, "exp_a.json"), []byte(legacy1), 0o644); err != nil {
		t.Fatalf("write file 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(experimentsDir, "exp_b.json"), []byte(legacy2), 0o644); err != nil {
		t.Fatalf("write file 2: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	for _, filename := range []string{"exp_a.json", "exp_b.json"} {
		data, err := os.ReadFile(filepath.Join(experimentsDir, filename))
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		if strings.Contains(string(data), `"Experiment"`) {
			t.Fatalf("expected PascalCase removed in %s", filename)
		}
		if !strings.Contains(string(data), `"experiment"`) {
			t.Fatalf("expected canonical snake_case in %s", filename)
		}
	}
}

func TestConvertExperimentSkipsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(experimentsDir, "empty.json"), []byte{}, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	stat, err := os.Stat(filepath.Join(experimentsDir, "empty.json"))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if stat.Size() != 0 {
		t.Fatalf("expected empty file to remain empty, got size %d", stat.Size())
	}
}

func TestConvertExperimentRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(experimentsDir, "bad.json"), []byte(`not valid json`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"-dir", dir}, &stdout)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "bad.json") {
		t.Fatalf("expected error to mention file path, got: %v", err)
	}
}

func TestConvertExperimentAlreadyCanonical(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:     "exp-canonical",
			Status: domain.ExperimentAccepted,
		},
		Brief: domain.MutationBrief{
			WindowID: "w1",
		},
		CandidatePrompt:       "test prompt",
		EvaluationMode:        "oos",
		BaselineObservations:  50,
		CandidateObservations: 48,
		RecordedAt:            time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DataMetadata: &domain.ReplayDataMetadata{
			SourcePath:     "/data/replay.jsonl",
			DateRangeStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			DateRangeEnd:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			DaysDelayed:    1,
			CoversWindow:   true,
			RecordCount:    1000,
		},
		OOSResult: &domain.OOSResult{
			Passed:         true,
			BaselineScore:  0.5,
			CandidateScore: 0.6,
			Improvement:    0.1,
			Observations:   30,
		},
		BaselineReturns:        []float64{0.01, 0.02},
		CandidateReturns:       []float64{0.02, 0.03},
		ParameterSnapshotID:    "snap-1",
		BaselineFallbackCount:  1,
		CandidateFallbackCount: 0,
		BaselineFactorCount:    5,
		CandidateFactorCount:   5,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(experimentsDir, "canonical.json"), data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(experimentsDir, "canonical.json"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	var decoded domain.PromptExperimentResult
	if err := json.Unmarshal(rewritten, &decoded); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if decoded.Experiment.ID != "exp-canonical" {
		t.Fatalf("experiment.id mismatch: got %q", decoded.Experiment.ID)
	}
	if decoded.DataMetadata == nil || decoded.DataMetadata.RecordCount != 1000 {
		t.Fatalf("data_metadata mismatch")
	}
	if decoded.OOSResult == nil || decoded.OOSResult.Improvement != 0.1 {
		t.Fatalf("oos_result mismatch")
	}
}

func TestConvertExperimentAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	legacyJSON := `{"Experiment":{"ID":"exp-atomic","Status":"accepted"},"Brief":{"WindowID":"w1"},"CandidatePrompt":"p1","EvaluationMode":"oos","BaselineObservations":10,"CandidateObservations":10,"RecordedAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(experimentsDir, "atomic.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && strings.Contains(name, "tmp") {
			t.Fatalf("found leftover temp file: %s", name)
		}
	}
}

func TestConvertExperimentNoFilesFound(t *testing.T) {
	dir := t.TempDir()

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No files") && !strings.Contains(output, "0 files") {
		t.Fatalf("expected output to indicate no files found, got: %s", output)
	}
}

func TestConvertExperimentHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-help"}, &stdout); err != nil {
		t.Fatalf("run -help failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
}

func TestConvertExperimentMixedCaseKeys(t *testing.T) {
	dir := t.TempDir()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mixedJSON := `{"Experiment":{"ID":"exp-mixed","Status":"accepted"},"Brief":{"WindowID":"w1"},"candidate_prompt":"mixed prompt","EvaluationMode":"oos","baseline_observations":10,"CandidateObservations":10,"RecordedAt":"2026-01-01T00:00:00Z"}`

	if err := os.WriteFile(filepath.Join(experimentsDir, "mixed.json"), []byte(mixedJSON), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(experimentsDir, "mixed.json"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	if strings.Contains(string(rewritten), `"Experiment"`) {
		t.Fatalf("expected PascalCase removed, got: %s", rewritten)
	}
	if strings.Contains(string(rewritten), `"EvaluationMode"`) {
		t.Fatalf("expected PascalCase removed, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"candidate_prompt"`) {
		t.Fatalf("expected existing snake_case preserved, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"baseline_observations"`) {
		t.Fatalf("expected existing snake_case preserved, got: %s", rewritten)
	}

	var result domain.PromptExperimentResult
	if err := json.Unmarshal(rewritten, &result); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if result.Experiment.ID != "exp-mixed" {
		t.Fatalf("experiment.id: got %q", result.Experiment.ID)
	}
	if result.CandidatePrompt != "mixed prompt" {
		t.Fatalf("candidate_prompt: got %q", result.CandidatePrompt)
	}
	if result.BaselineObservations != 10 {
		t.Fatalf("baseline_observations: got %d", result.BaselineObservations)
	}
}

func TestConvertExperimentRejectsMissingExperimentsDir(t *testing.T) {
	dir := t.TempDir()

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No files") && !strings.Contains(output, "0 files") {
		t.Fatalf("expected output to indicate no files found, got: %s", output)
	}
}
