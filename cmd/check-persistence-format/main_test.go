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

func TestClassifyArtifactSnakeCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, []byte(`{"session_id":"s1","order_count":1}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := classifyArtifact(path)
	if result != "snake_case" {
		t.Fatalf("expected snake_case, got %s", result)
	}
}

func TestClassifyArtifactPascalCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, []byte(`{"SessionID":"s1","OrderCount":1}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := classifyArtifact(path)
	if result != "pascal_case" {
		t.Fatalf("expected pascal_case, got %s", result)
	}
}

func TestClassifyArtifactMixed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, []byte(`{"session_id":"s1","OrderCount":1}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := classifyArtifact(path)
	if result != "mixed" {
		t.Fatalf("expected mixed, got %s", result)
	}
}

func TestClassifyArtifactEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := classifyArtifact(path)
	if result != "empty" {
		t.Fatalf("expected empty, got %s", result)
	}
}

func TestClassifyArtifactMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	result := classifyArtifact(path)
	if result != "missing" {
		t.Fatalf("expected missing, got %s", result)
	}
}

func TestClassifyArtifactUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, []byte(`not json at all`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := classifyArtifact(path)
	if result != "unknown" {
		t.Fatalf("expected unknown, got %s", result)
	}
}

func TestCheckWriterConsistencyRootLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommendation_outcomes.jsonl")

	outcome := domain.RecommendationOutcome{
		AgentID:        "agent-1",
		Skill:          "test",
		Layer:          domain.LayerSector,
		Symbol:         "2330.TW",
		Side:           domain.SideBuy,
		Conviction:     80,
		TargetPrice:    600,
		StopLossPrice:  550,
		Window:         "1d",
		ForwardReturn:  0.02,
		BenchmarkDelta: 0.01,
		Hit:            true,
		Reason:         "test",
		Price:          580,
		PassedGuards:   true,
		GuardReason:    "",
		RecordedAt:     time.Now(),
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	enc := json.NewEncoder(f)
	if err := enc.Encode(outcome); err != nil {
		f.Close()
		t.Fatalf("encode: %v", err)
	}
	f.Close()

	issues := checkWriterConsistency(path)
	if len(issues) > 0 {
		t.Fatalf("expected no issues for properly written file, got: %v", issues)
	}
}

func TestCheckWriterConsistencySessionLevel(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "session-20260101")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")

	outcome := domain.RecommendationOutcome{
		AgentID:        "agent-1",
		Skill:          "test",
		Layer:          domain.LayerSector,
		Symbol:         "2330.TW",
		Side:           domain.SideBuy,
		Conviction:     80,
		TargetPrice:    600,
		StopLossPrice:  550,
		Window:         "1d",
		ForwardReturn:  0.02,
		BenchmarkDelta: 0.01,
		Hit:            true,
		Reason:         "test",
		Price:          580,
		PassedGuards:   true,
		GuardReason:    "",
		RecordedAt:     time.Now(),
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	enc := json.NewEncoder(f)
	if err := enc.Encode(outcome); err != nil {
		f.Close()
		t.Fatalf("encode: %v", err)
	}
	f.Close()

	issues := checkWriterConsistency(path)
	if len(issues) > 0 {
		t.Fatalf("expected no issues for properly written session file, got: %v", issues)
	}
}

func TestCheckWriterConsistencyDetectsPascalCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommendation_outcomes.jsonl")

	legacyLine := `{"AgentID":"agent-1","Skill":"test","Layer":"sector","Symbol":"2330.TW","Side":"BUY","Conviction":80,"TargetPrice":600,"StopLossPrice":550,"Window":"1d","ForwardReturn":0.02,"BenchmarkDelta":0.01,"Hit":true,"Reason":"test","Price":580,"PassedGuards":true,"GuardReason":"","RecordedAt":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(legacyLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	issues := checkWriterConsistency(path)
	if len(issues) == 0 {
		t.Fatalf("expected issues for PascalCase file, got none")
	}

	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "PascalCase") || strings.Contains(issue, "snake_case") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected PascalCase/snake_case issue, got: %v", issues)
	}
}

func TestCheckWriterConsistencyDetectsDecodeError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommendation_outcomes.jsonl")

	if err := os.WriteFile(path, []byte(`not valid json`+"\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	issues := checkWriterConsistency(path)
	if len(issues) == 0 {
		t.Fatalf("expected issues for invalid JSON, got none")
	}
}

func TestRunCommandScansDirectory(t *testing.T) {
	dir := t.TempDir()

	sessionDir := filepath.Join(dir, "sessions", "session-20260101")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), []byte(`{"session_id":"session-20260101","order_count":1}`), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:        "agent-1",
		Skill:          "test",
		Layer:          domain.LayerSector,
		Symbol:         "2330.TW",
		Side:           domain.SideBuy,
		Conviction:     80,
		TargetPrice:    600,
		StopLossPrice:  550,
		Window:         "1d",
		ForwardReturn:  0.02,
		BenchmarkDelta: 0.01,
		Hit:            true,
		Reason:         "test",
		Price:          580,
		PassedGuards:   true,
		GuardReason:    "",
		RecordedAt:     time.Now(),
	}
	f, err := os.Create(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	json.NewEncoder(f).Encode(outcome)
	f.Close()

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "session-20260101/summary.json") {
		t.Fatalf("expected output to mention summary.json, got:\n%s", output)
	}
	if !strings.Contains(output, "recommendation_outcomes.jsonl") {
		t.Fatalf("expected output to mention recommendation_outcomes.jsonl, got:\n%s", output)
	}
	if !strings.Contains(output, "snake_case") {
		t.Fatalf("expected output to classify as snake_case, got:\n%s", output)
	}
}

func TestRunCommandDefaultDir(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{}, &stdout)
	if err != nil {
		t.Fatalf("run with default dir failed: %v", err)
	}
}

func TestRunCommandHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-help"}, &stdout); err != nil {
		t.Fatalf("run -help failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
}

func TestInventoryAllArtifactTypes(t *testing.T) {
	dir := t.TempDir()

	sessionDir := filepath.Join(dir, "sessions", "session-20260101")
	os.MkdirAll(sessionDir, 0o755)
	os.WriteFile(filepath.Join(sessionDir, "summary.json"), []byte(`{"session_id":"s1"}`), 0o644)

	outcome := domain.RecommendationOutcome{
		AgentID: "a1", Skill: "s1", Layer: domain.LayerSector, Symbol: "2330.TW",
		Side: domain.SideBuy, Conviction: 50, TargetPrice: 100, StopLossPrice: 90,
		Window: "1d", ForwardReturn: 0.01, BenchmarkDelta: 0, Hit: true,
		Reason: "r", Price: 95, PassedGuards: true, RecordedAt: time.Now(),
	}
	f, _ := os.Create(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"))
	json.NewEncoder(f).Encode(outcome)
	f.Close()

	f, _ = os.Create(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	json.NewEncoder(f).Encode(outcome)
	f.Close()

	os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(`{"id":"e1"}`+"\n"), 0o644)

	os.MkdirAll(filepath.Join(dir, "experiments"), 0o755)
	os.WriteFile(filepath.Join(dir, "experiments", "exp1.json"), []byte(`{"experiment":{"id":"e1"}}`), 0o644)

	os.WriteFile(filepath.Join(dir, "baseline_policy.json"), []byte(`{"version":1}`), 0o644)

	items := inventoryArtifacts(dir)

	expectedPaths := []string{
		"sessions/session-20260101/summary.json",
		"sessions/session-20260101/recommendation_outcomes.jsonl",
		"recommendation_outcomes.jsonl",
		"experiments.jsonl",
		"experiments/exp1.json",
		"baseline_policy.json",
	}

	for _, expected := range expectedPaths {
		found := false
		for _, item := range items {
			if strings.HasSuffix(item.Path, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected to find artifact %s in inventory", expected)
		}
	}
}

func TestInventoryDetectsPascalCaseExperiment(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "experiments"), 0o755)
	os.WriteFile(filepath.Join(dir, "experiments", "exp1.json"), []byte(`{"Experiment":{"ID":"e1"}}`), 0o644)

	items := inventoryArtifacts(dir)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Classification != "pascal_case" {
		t.Fatalf("expected pascal_case, got %s", items[0].Classification)
	}
}

func TestInventoryHandlesEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	items := inventoryArtifacts(dir)
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty dir, got %d", len(items))
	}
}

func TestCheckWriterConsistencyMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.jsonl")
	issues := checkWriterConsistency(path)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue for missing file, got %d", len(issues))
	}
	if !strings.Contains(issues[0], "missing") {
		t.Fatalf("expected 'missing' in issue, got: %s", issues[0])
	}
}

func TestCheckWriterConsistencyEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte{}, 0o644)
	issues := checkWriterConsistency(path)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue for empty file, got %d", len(issues))
	}
	if !strings.Contains(issues[0], "empty") {
		t.Fatalf("expected 'empty' in issue, got: %s", issues[0])
	}
}

func TestFormatClassifyJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")

	line1, _ := json.Marshal(map[string]string{"agent_id": "a1"})
	line2, _ := json.Marshal(map[string]string{"agent_id": "a2"})
	content := string(line1) + "\n" + string(line2) + "\n"
	os.WriteFile(path, []byte(content), 0o644)

	result := classifyArtifact(path)
	if result != "snake_case" {
		t.Fatalf("expected snake_case for JSONL, got %s", result)
	}
}

func TestCheckWriterConsistencyEmptyWithZeroOutcomeCount(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "session-20260326")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	summaryPath := filepath.Join(sessionDir, "summary.json")
	summaryContent := `{"session_id":"session-20260326","outcome_count":0}`
	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	outcomesPath := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")
	if err := os.WriteFile(outcomesPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty outcomes: %v", err)
	}

	issues := checkWriterConsistency(outcomesPath)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for empty outcomes file with outcome_count==0, got: %v", issues)
	}
}

func TestCheckWriterConsistencyEmptyWithPositiveOutcomeCount(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "session-20260326")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	summaryPath := filepath.Join(sessionDir, "summary.json")
	summaryContent := `{"session_id":"session-20260326","outcome_count":5}`
	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	outcomesPath := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")
	if err := os.WriteFile(outcomesPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty outcomes: %v", err)
	}

	issues := checkWriterConsistency(outcomesPath)
	if len(issues) == 0 {
		t.Fatalf("expected issues for empty outcomes file with outcome_count==5, got none")
	}
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "outcome_count") || strings.Contains(issue, "expected") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected issue mentioning outcome_count mismatch, got: %v", issues)
	}
}

func TestClassifyArtifactEmptyWithSiblingSummary(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "session-20260326")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	summaryPath := filepath.Join(sessionDir, "summary.json")
	os.WriteFile(summaryPath, []byte(`{"session_id":"session-20260326","outcome_count":3}`), 0o644)

	outcomesPath := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")
	os.WriteFile(outcomesPath, []byte{}, 0o644)

	result := classifyArtifact(outcomesPath)
	if result != "empty" {
		t.Fatalf("expected classification 'empty' for zero-byte outcomes file, got %q", result)
	}
}
