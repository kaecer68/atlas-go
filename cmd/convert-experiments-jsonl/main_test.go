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

func TestConvertLegacyPascalCase(t *testing.T) {
	dir := t.TempDir()

	legacyLine := `{"ID":"exp-1","ProposalID":"proposal-1","CommitID":"abc123","ApprovalID":"approval-1","TargetAgentID":"growth-momentum-01","Skill":"growth_momentum","Hypothesis":"tighten exit","PromptVersionFrom":"v1","PromptVersionTo":"v2","MutationType":"prompt_tightening","AcceptanceGates":["improve_sharpe_like"],"WindowStart":"2026-01-01T00:00:00Z","WindowEnd":"2026-01-31T00:00:00Z","AcceptanceMetric":"sharpe_like","BaselineValue":0.5,"CandidateValue":0.7,"Status":"accepted","RevertReason":""}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(legacyLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if strings.Contains(string(rewritten), `"ID"`) {
		t.Fatalf("expected PascalCase removed, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"id"`) {
		t.Fatalf("expected canonical snake_case id, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"proposal_id"`) {
		t.Fatalf("expected canonical snake_case proposal_id, got: %s", rewritten)
	}

	var rec domain.ExperimentRecord
	if err := json.Unmarshal(rewritten, &rec); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if rec.ID != "exp-1" {
		t.Fatalf("id mismatch: got %q", rec.ID)
	}
	if rec.ProposalID != "proposal-1" {
		t.Fatalf("proposal_id mismatch: got %q", rec.ProposalID)
	}
	if rec.Status != domain.ExperimentAccepted {
		t.Fatalf("status mismatch: got %q", rec.Status)
	}
}

func TestConvertCanonicalSnakeCase(t *testing.T) {
	dir := t.TempDir()

	canonicalLine := `{"id":"exp-2","proposal_id":"proposal-2","commit_id":"def456","approval_id":"approval-2","target_agent_id":"value-yield-01","skill":"value_yield","hypothesis":"increase dividend focus","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"prompt_expansion","acceptance_gates":["improve_hit_rate"],"window_start":"2026-02-01T00:00:00Z","window_end":"2026-02-28T00:00:00Z","acceptance_metric":"hit_rate","baseline_value":0.4,"candidate_value":0.6,"status":"rejected","revert_reason":"insufficient_observations"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(canonicalLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if !strings.Contains(string(rewritten), `"id"`) {
		t.Fatalf("expected canonical snake_case id, got: %s", rewritten)
	}

	var rec domain.ExperimentRecord
	if err := json.Unmarshal(rewritten, &rec); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if rec.ID != "exp-2" {
		t.Fatalf("id mismatch: got %q", rec.ID)
	}
	if rec.Status != domain.ExperimentRejected {
		t.Fatalf("status mismatch: got %q", rec.Status)
	}
}

func TestConvertPreservesLineCount(t *testing.T) {
	dir := t.TempDir()

	lines := []string{
		`{"id":"exp-1","proposal_id":"p1","commit_id":"c1","approval_id":"a1","target_agent_id":"agent-1","skill":"s1","hypothesis":"h1","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"m1","acceptance_gates":["g1"],"window_start":"2026-01-01T00:00:00Z","window_end":"2026-01-31T00:00:00Z","acceptance_metric":"sharpe","baseline_value":0.1,"candidate_value":0.2,"status":"accepted","revert_reason":""}`,
		`{"id":"exp-2","proposal_id":"p2","commit_id":"c2","approval_id":"a2","target_agent_id":"agent-2","skill":"s2","hypothesis":"h2","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"m2","acceptance_gates":["g2"],"window_start":"2026-02-01T00:00:00Z","window_end":"2026-02-28T00:00:00Z","acceptance_metric":"hit_rate","baseline_value":0.3,"candidate_value":0.4,"status":"rejected","revert_reason":"low_observations"}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	rewrittenLines := strings.Split(strings.TrimSuffix(string(rewritten), "\n"), "\n")
	if len(rewrittenLines) != len(lines) {
		t.Fatalf("expected %d lines, got %d", len(lines), len(rewrittenLines))
	}
}

func TestConvertPreservesOrder(t *testing.T) {
	dir := t.TempDir()

	lines := []string{
		`{"id":"first","proposal_id":"p1","commit_id":"c1","approval_id":"a1","target_agent_id":"agent-1","skill":"s1","hypothesis":"h1","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"m1","acceptance_gates":["g1"],"window_start":"2026-01-01T00:00:00Z","window_end":"2026-01-31T00:00:00Z","acceptance_metric":"sharpe","baseline_value":0.1,"candidate_value":0.2,"status":"accepted","revert_reason":""}`,
		`{"id":"second","proposal_id":"p2","commit_id":"c2","approval_id":"a2","target_agent_id":"agent-2","skill":"s2","hypothesis":"h2","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"m2","acceptance_gates":["g2"],"window_start":"2026-02-01T00:00:00Z","window_end":"2026-02-28T00:00:00Z","acceptance_metric":"hit_rate","baseline_value":0.3,"candidate_value":0.4,"status":"rejected","revert_reason":"low_observations"}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	rewrittenLines := strings.Split(strings.TrimSuffix(string(rewritten), "\n"), "\n")
	for i, line := range rewrittenLines {
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal line %d: %v", i, err)
		}
		switch i {
		case 0:
			if rec.ID != "first" {
				t.Fatalf("line 0: expected first, got %s", rec.ID)
			}
		case 1:
			if rec.ID != "second" {
				t.Fatalf("line 1: expected second, got %s", rec.ID)
			}
		}
	}
}

func TestConvertSkipsEmptyFile(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	stat, err := os.Stat(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if stat.Size() != 0 {
		t.Fatalf("expected empty file to remain empty, got size %d", stat.Size())
	}
}

func TestConvertRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(`not valid json`+"\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"-dir", dir}, &stdout)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "experiments.jsonl") {
		t.Fatalf("expected error to mention file path, got: %v", err)
	}
}

func TestConvertNoFilesFound(t *testing.T) {
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

func TestConvertHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-help"}, &stdout); err != nil {
		t.Fatalf("run -help failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
}

func TestConvertAtomicWrite(t *testing.T) {
	dir := t.TempDir()

	line := `{"id":"exp-1","proposal_id":"p1","commit_id":"c1","approval_id":"a1","target_agent_id":"agent-1","skill":"s1","hypothesis":"h1","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"m1","acceptance_gates":["g1"],"window_start":"2026-01-01T00:00:00Z","window_end":"2026-01-31T00:00:00Z","acceptance_metric":"sharpe","baseline_value":0.1,"candidate_value":0.2,"status":"accepted","revert_reason":""}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
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

func TestConvertMixedLegacyAndCanonical(t *testing.T) {
	dir := t.TempDir()

	lines := []string{
		`{"ID":"exp-legacy","ProposalID":"p1","CommitID":"c1","ApprovalID":"a1","TargetAgentID":"agent-1","Skill":"s1","Hypothesis":"h1","PromptVersionFrom":"v1","PromptVersionTo":"v2","MutationType":"m1","AcceptanceGates":["g1"],"WindowStart":"2026-01-01T00:00:00Z","WindowEnd":"2026-01-31T00:00:00Z","AcceptanceMetric":"sharpe","BaselineValue":0.1,"CandidateValue":0.2,"Status":"accepted","RevertReason":""}`,
		`{"id":"exp-canonical","proposal_id":"p2","commit_id":"c2","approval_id":"a2","target_agent_id":"agent-2","skill":"s2","hypothesis":"h2","prompt_version_from":"v1","prompt_version_to":"v2","mutation_type":"m2","acceptance_gates":["g2"],"window_start":"2026-02-01T00:00:00Z","window_end":"2026-02-28T00:00:00Z","acceptance_metric":"hit_rate","baseline_value":0.3,"candidate_value":0.4,"status":"rejected","revert_reason":"low_observations"}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	rewrittenLines := strings.Split(strings.TrimSuffix(string(rewritten), "\n"), "\n")
	if len(rewrittenLines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(rewrittenLines))
	}

	for i, line := range rewrittenLines {
		if strings.Contains(line, `"ID"`) || strings.Contains(line, `"ProposalID"`) {
			t.Fatalf("line %d: expected no PascalCase, got: %s", i, line)
		}
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal line %d: %v", i, err)
		}
		switch i {
		case 0:
			if rec.ID != "exp-legacy" {
				t.Fatalf("line 0: expected exp-legacy, got %s", rec.ID)
			}
		case 1:
			if rec.ID != "exp-canonical" {
				t.Fatalf("line 1: expected exp-canonical, got %s", rec.ID)
			}
		}
	}
}

func TestConvertAlreadyCanonicalRoundtrip(t *testing.T) {
	dir := t.TempDir()

	rec := domain.ExperimentRecord{
		ID:                "exp-rt",
		ProposalID:        "proposal-rt",
		CommitID:          "commit-rt",
		ApprovalID:        "approval-rt",
		TargetAgentID:     "agent-rt",
		Skill:             "test_skill",
		Hypothesis:        "roundtrip test",
		PromptVersionFrom: "v1",
		PromptVersionTo:   "v2",
		MutationType:      "test",
		AcceptanceGates:   []string{"gate1"},
		WindowStart:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:         time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		AcceptanceMetric:  "sharpe",
		BaselineValue:     0.5,
		CandidateValue:    0.6,
		Status:            domain.ExperimentRunning,
		RevertReason:      "",
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	var decoded domain.ExperimentRecord
	if err := json.Unmarshal(rewritten, &decoded); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if decoded.ID != rec.ID {
		t.Fatalf("id mismatch: got %q", decoded.ID)
	}
	if decoded.Status != rec.Status {
		t.Fatalf("status mismatch: got %q", decoded.Status)
	}
	if decoded.WindowStart != rec.WindowStart {
		t.Fatalf("window_start mismatch: got %v", decoded.WindowStart)
	}
}
