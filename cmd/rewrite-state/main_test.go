package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLegacyBaselinePolicy(t *testing.T, dir string) {
	t.Helper()
	legacy := `{
		"Version": 3,
		"PromptOverrides": {"agent-a": "override-v2"},
		"Constraints": {
			"StartingCash": 1000000,
			"MaxPositionWeight": 0.12,
			"MaxOpenPositions": 15,
			"MinTradableVolume": 500000,
			"MinRecommendationConviction": 55,
			"RequireCROPass": true,
			"TransactionCostBPS": 1.5,
			"SlippageBPS": 2.0,
			"ReserveCashFraction": 0.08
		},
		"ExecutionPolicy": {
			"ConvictionFloor": 55,
			"RequireCROPass": true
		},
		"Promotions": [],
		"RevertHistory": [],
		"LastUpdatedAt": "2026-04-01T00:00:00Z"
	}`
	if err := os.WriteFile(filepath.Join(dir, "baseline_policy.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write baseline_policy.json: %v", err)
	}
}

func writeLegacyRecommendationOutcomes(t *testing.T, dir string) {
	t.Helper()
	line := `{"AgentID":"agent-1","Skill":"test","Layer":"sector","Symbol":"2330.TW","Side":"BUY","Conviction":80,"TargetPrice":600,"StopLossPrice":550,"Window":"1d","ForwardReturn":0.02,"BenchmarkDelta":0.01,"Hit":true,"Reason":"test","Price":580,"PassedGuards":true,"GuardReason":"","RecordedAt":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatalf("write recommendation_outcomes.jsonl: %v", err)
	}
}

func writeLegacySessionRecommendationOutcomes(t *testing.T, dir, sessionID string) {
	t.Helper()
	sessionDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	line := `{"AgentID":"agent-2","Skill":"momentum","Layer":"style","Symbol":"2317.TW","Side":"SELL","Conviction":60,"TargetPrice":120,"StopLossPrice":110,"Window":"1d","ForwardReturn":-0.01,"BenchmarkDelta":0.005,"Hit":false,"Reason":"weak signal","Price":115,"PassedGuards":false,"GuardReason":"conviction too low","RecordedAt":"2026-01-02T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatalf("write session recommendation_outcomes.jsonl: %v", err)
	}
}

// writeLegacySessionSummary writes a PascalCase summary.json (legacy format before canonical rewrite).
func writeLegacySessionSummary(t *testing.T, dir, sessionID string) {
	t.Helper()
	sessionDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	// Legacy PascalCase format - LoadSessionSummaries unmarshals into domain.SessionSummary
	// which uses snake_case tags, so PascalCase causes silent field loss.
	legacy := `{
		"SessionID": "session-20260101",
		"Regime": "BULL",
		"OrderCount": 10,
		"PositionCount": 5,
		"EndingCash": 950000.0,
		"PortfolioValue": 1050000.0,
		"OutcomeCount": 8,
		"BrokerRuntime": {
			"Mode": "backtest",
			"Adapter": "twse"
		},
		"NextExperimentAgentID": "agent-x",
		"ProposalID": "prop-1",
		"CommitID": "abc123",
		"ApprovalID": "apv-1",
		"GuardOutcomes": [],
		"RecordedAt": "2026-01-01T12:00:00Z"
	}`
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write summary.json: %v", err)
	}
}

func writeLegacyExperimentsJSONL(t *testing.T, dir string) {
	t.Helper()
	line := `{"ID":"exp-1","TargetAgentID":"agent-1","Skill":"test","MutationType":"prompt_tightening","Status":"planned"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "experiments.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatalf("write experiments.jsonl: %v", err)
	}
}

func writeLegacyExperimentResult(t *testing.T, dir string) {
	t.Helper()
	experimentsDir := filepath.Join(dir, "experiments")
	if err := os.MkdirAll(experimentsDir, 0o755); err != nil {
		t.Fatalf("mkdir experiments dir: %v", err)
	}
	legacy := `{
		"Experiment": {"ID":"exp-1","TargetAgentID":"agent-1","MutationType":"prompt_tightening","Status":"accepted"},
		"Brief": {"window_id":"w1","mutation_type":"prompt_tightening"},
		"CandidatePrompt": "candidate.md",
		"EvaluationMode": "replay"
	}`
	if err := os.WriteFile(filepath.Join(experimentsDir, "exp-1.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write exp-1.json: %v", err)
	}
}

func buildFullFixtureState(t *testing.T) (stateDir, archiveBase string) {
	t.Helper()
	stateDir = filepath.Join(t.TempDir(), "state")
	archiveBase = filepath.Join(t.TempDir(), "state-archive")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	writeLegacyBaselinePolicy(t, stateDir)
	writeLegacyRecommendationOutcomes(t, stateDir)
	writeLegacySessionRecommendationOutcomes(t, stateDir, "session-20260101")
	writeLegacyExperimentsJSONL(t, stateDir)
	writeLegacyExperimentResult(t, stateDir)
	return stateDir, archiveBase
}

func isPascalCase(s string) bool {
	if s == "" {
		return false
	}
	first := rune(s[0])
	if first < 'A' || first > 'Z' {
		return false
	}
	return !strings.Contains(s, "_")
}

func assertNoPascalCaseKeysInJSON(t *testing.T, data []byte, path string) {
	t.Helper()
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("%s: unmarshal: %v", path, err)
	}
	for key := range raw {
		if isPascalCase(key) {
			t.Fatalf("%s: found PascalCase key %q", path, key)
		}
	}
}

func assertNoPascalCaseKeysInJSONL(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("%s line %d: unmarshal: %v", path, i+1, err)
		}
		for key := range raw {
			if isPascalCase(key) {
				t.Fatalf("%s line %d: found PascalCase key %q", path, i+1, key)
			}
		}
	}
}

func assertFileContainsSnakeCaseKeys(t *testing.T, path string, expectedKeys []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)
	for _, key := range expectedKeys {
		if !strings.Contains(text, `"`+key+`"`) {
			t.Fatalf("%s: expected snake_case key %q not found", path, key)
		}
	}
}

func TestRewriteStateFullWorkflow(t *testing.T) {
	stateDir, archiveBase := buildFullFixtureState(t)

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	archiveEntries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(archiveEntries) != 1 {
		t.Fatalf("expected 1 archive dir, got %d", len(archiveEntries))
	}
	archiveDir := filepath.Join(archiveBase, archiveEntries[0].Name())

	for _, rel := range []string{"baseline_policy.json", "recommendation_outcomes.jsonl", "experiments.jsonl", "experiments/exp-1.json"} {
		archivePath := filepath.Join(archiveDir, rel)
		if _, err := os.Stat(archivePath); err != nil {
			t.Fatalf("expected %s in archive: %v", rel, err)
		}
	}

	assertNoPascalCaseKeysInJSON(t, mustRead(t, filepath.Join(stateDir, "baseline_policy.json")), "baseline_policy.json")
	assertNoPascalCaseKeysInJSONL(t, filepath.Join(stateDir, "recommendation_outcomes.jsonl"))
	assertNoPascalCaseKeysInJSONL(t, filepath.Join(stateDir, "sessions", "session-20260101", "recommendation_outcomes.jsonl"))
	assertNoPascalCaseKeysInJSONL(t, filepath.Join(stateDir, "experiments.jsonl"))
	assertNoPascalCaseKeysInJSON(t, mustRead(t, filepath.Join(stateDir, "experiments", "exp-1.json")), "experiments/exp-1.json")

	assertFileContainsSnakeCaseKeys(t, filepath.Join(stateDir, "baseline_policy.json"), []string{"version", "prompt_overrides", "starting_cash", "conviction_floor"})
	assertFileContainsSnakeCaseKeys(t, filepath.Join(stateDir, "recommendation_outcomes.jsonl"), []string{"agent_id", "symbol", "conviction", "passed_guards"})
	assertFileContainsSnakeCaseKeys(t, filepath.Join(stateDir, "experiments.jsonl"), []string{"target_agent_id", "mutation_type"})
	assertFileContainsSnakeCaseKeys(t, filepath.Join(stateDir, "experiments", "exp-1.json"), []string{"candidate_prompt", "evaluation_mode"})
}

func TestRewriteStateArchiveFirst(t *testing.T) {
	stateDir, archiveBase := buildFullFixtureState(t)

	preStat, err := os.Stat(filepath.Join(stateDir, "baseline_policy.json"))
	if err != nil {
		t.Fatalf("stat pre: %v", err)
	}
	preModTime := preStat.ModTime()

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	archiveEntries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(archiveEntries) != 1 {
		t.Fatalf("expected 1 archive dir, got %d", len(archiveEntries))
	}
	archiveDir := filepath.Join(archiveBase, archiveEntries[0].Name())

	archiveBaseline, err := os.ReadFile(filepath.Join(archiveDir, "baseline_policy.json"))
	if err != nil {
		t.Fatalf("read archive baseline: %v", err)
	}
	if !strings.Contains(string(archiveBaseline), `"Version"`) {
		t.Fatalf("archive baseline_policy.json should still contain PascalCase (archive-first contract violated)")
	}

	postStat, err := os.Stat(filepath.Join(stateDir, "baseline_policy.json"))
	if err != nil {
		t.Fatalf("stat post: %v", err)
	}
	if postStat.ModTime().Equal(preModTime) && postStat.Size() == preStat.Size() {
		stateBaseline, err := os.ReadFile(filepath.Join(stateDir, "baseline_policy.json"))
		if err != nil {
			t.Fatalf("read state baseline: %v", err)
		}
		if strings.Contains(string(stateBaseline), `"Version"`) {
			t.Fatalf("state baseline_policy.json was not rewritten to snake_case")
		}
	}
}

func TestRewriteStateStopsOnConverterFailure(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	archiveBase := filepath.Join(t.TempDir(), "state-archive")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	writeLegacyBaselinePolicy(t, stateDir)

	malformed := `{"AgentID":"agent-1", broken json` + "\n"
	if err := os.WriteFile(filepath.Join(stateDir, "recommendation_outcomes.jsonl"), []byte(malformed), 0o644); err != nil {
		t.Fatalf("write malformed outcomes: %v", err)
	}

	writeLegacyExperimentsJSONL(t, stateDir)

	var stdout bytes.Buffer
	err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout)
	if err == nil {
		t.Fatalf("expected error when converter fails, got nil")
	}

	archiveEntries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(archiveEntries) != 1 {
		t.Fatalf("expected archive to exist even on failure, got %d entries", len(archiveEntries))
	}

	baselineData, err := os.ReadFile(filepath.Join(stateDir, "baseline_policy.json"))
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if strings.Contains(string(baselineData), `"Version"`) {
		t.Fatalf("baseline_policy.json should have been converted before failure")
	}

	expData, err := os.ReadFile(filepath.Join(stateDir, "experiments.jsonl"))
	if err != nil {
		t.Fatalf("read experiments: %v", err)
	}
	if !strings.Contains(string(expData), `"ID"`) {
		t.Fatalf("experiments.jsonl should NOT have been converted after failure stopped execution")
	}
}

func TestRewriteStateReportsArchivePathAndCounts(t *testing.T) {
	stateDir, archiveBase := buildFullFixtureState(t)

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	output := stdout.String()

	if !strings.Contains(output, archiveBase) {
		t.Fatalf("expected stdout to contain archive base path, got: %s", output)
	}

	if !strings.Contains(output, "5") && !strings.Contains(output, "6") && !strings.Contains(output, "rewritten") {
		t.Fatalf("expected stdout to report rewritten file count, got: %s", output)
	}
}

func TestRewriteStateNoopWhenNoArtifacts(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "empty-state")
	archiveBase := filepath.Join(t.TempDir(), "state-archive")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run on empty state: %v", err)
	}

	archiveEntries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(archiveEntries) != 1 {
		t.Fatalf("expected 1 archive dir even for empty state, got %d", len(archiveEntries))
	}
}

func TestRewriteStatePreservesUnrecognizedFiles(t *testing.T) {
	stateDir, archiveBase := buildFullFixtureState(t)

	unrecognized := `{"custom_key": "value"}`
	if err := os.WriteFile(filepath.Join(stateDir, "custom_data.json"), []byte(unrecognized), 0o644); err != nil {
		t.Fatalf("write unrecognized file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(stateDir, "custom_data.json")); err != nil {
		t.Fatalf("unrecognized file should be preserved: %v", err)
	}

	archiveEntries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	archiveDir := filepath.Join(archiveBase, archiveEntries[0].Name())
	if _, err := os.Stat(filepath.Join(archiveDir, "custom_data.json")); err != nil {
		t.Fatalf("unrecognized file should be in archive: %v", err)
	}
}

func TestRewriteStateConvertsSessionSummaryToSnakeCase(t *testing.T) {
	stateDir, archiveBase := buildFullFixtureState(t)
	writeLegacySessionSummary(t, stateDir, "session-20260101")

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	summaryPath := filepath.Join(stateDir, "sessions", "session-20260101", "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary.json: %v", err)
	}

	if !strings.Contains(string(summaryData), `"session_id"`) {
		t.Fatalf("expected summary.json to contain snake_case key session_id; got:\n%s", summaryData)
	}
	if strings.Contains(string(summaryData), `"SessionID"`) {
		t.Fatalf("expected summary.json NOT to contain PascalCase key SessionID; got:\n%s", summaryData)
	}

	if !strings.Contains(string(summaryData), `"outcome_count"`) {
		t.Fatalf("expected summary.json to contain snake_case key outcome_count; got:\n%s", summaryData)
	}
	if strings.Contains(string(summaryData), `"OutcomeCount"`) {
		t.Fatalf("expected summary.json NOT to contain PascalCase key OutcomeCount; got:\n%s", summaryData)
	}

	if !strings.Contains(string(summaryData), `"portfolio_value"`) {
		t.Fatalf("expected summary.json to contain snake_case key portfolio_value; got:\n%s", summaryData)
	}
	if strings.Contains(string(summaryData), `"PortfolioValue"`) {
		t.Fatalf("expected summary.json NOT to contain PascalCase key PortfolioValue; got:\n%s", summaryData)
	}
}

func TestRewriteStateArchiveRetainsOriginalSessionSummary(t *testing.T) {
	stateDir, archiveBase := buildFullFixtureState(t)
	writeLegacySessionSummary(t, stateDir, "session-20260101")

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	archiveEntries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive base: %v", err)
	}
	if len(archiveEntries) != 1 {
		t.Fatalf("expected 1 archive dir, got %d", len(archiveEntries))
	}
	archiveDir := filepath.Join(archiveBase, archiveEntries[0].Name())

	archiveSummaryPath := filepath.Join(archiveDir, "sessions", "session-20260101", "summary.json")
	archiveSummaryData, err := os.ReadFile(archiveSummaryPath)
	if err != nil {
		t.Fatalf("read archived summary.json: %v", err)
	}

	if !strings.Contains(string(archiveSummaryData), `"SessionID"`) {
		t.Fatalf("archive summary.json should retain PascalCase key SessionID; got:\n%s", archiveSummaryData)
	}
	if !strings.Contains(string(archiveSummaryData), `"OutcomeCount"`) {
		t.Fatalf("archive summary.json should retain PascalCase key OutcomeCount; got:\n%s", archiveSummaryData)
	}
	if strings.Contains(string(archiveSummaryData), `"session_id"`) {
		t.Fatalf("archive summary.json should NOT contain snake_case key session_id; got:\n%s", archiveSummaryData)
	}
}

func TestRewriteStateDefaults(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{}, &stdout)
	if err == nil {
		t.Fatalf("expected error when default state dir missing, got nil")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
