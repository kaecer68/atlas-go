package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/baseline"
)

func TestMissingFilePrintsNoFileMessage(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer

	err := run([]string{"-state-dir", dir}, &stdout)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "baseline_policy.json") {
		t.Fatalf("expected output to mention baseline_policy.json, got %q", out)
	}
	if !strings.Contains(out, "not found") && !strings.Contains(out, "No file") && !strings.Contains(out, "no file") {
		t.Fatalf("expected clear no-file message, got %q", out)
	}
}

func TestLegacyPascalCaseRewrittenToSnakeCase(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

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

	if err := os.WriteFile(policyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy policy: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read rewritten policy: %v", err)
	}

	text := string(data)
	if strings.Contains(text, `"Version"`) {
		t.Fatalf("expected PascalCase Version to be rewritten, got %s", text)
	}
	if !strings.Contains(text, `"version"`) {
		t.Fatalf("expected snake_case version, got %s", text)
	}
	if !strings.Contains(text, `"prompt_overrides"`) {
		t.Fatalf("expected snake_case prompt_overrides, got %s", text)
	}
	if !strings.Contains(text, `"constraints"`) {
		t.Fatalf("expected snake_case constraints, got %s", text)
	}
	if !strings.Contains(text, `"execution_policy"`) {
		t.Fatalf("expected snake_case execution_policy, got %s", text)
	}
	if !strings.Contains(text, `"promotions"`) {
		t.Fatalf("expected snake_case promotions, got %s", text)
	}
	if !strings.Contains(text, `"revert_history"`) {
		t.Fatalf("expected snake_case revert_history, got %s", text)
	}
	if !strings.Contains(text, `"last_updated_at"`) {
		t.Fatalf("expected snake_case last_updated_at, got %s", text)
	}

	var policy baseline.Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("unmarshal rewritten policy: %v", err)
	}
	if policy.Version != 3 {
		t.Fatalf("expected version 3, got %d", policy.Version)
	}
	if policy.PromptOverrides["agent-a"] != "override-v2" {
		t.Fatalf("expected prompt override preserved")
	}
	if policy.Constraints.MaxPositionWeight != 0.12 {
		t.Fatalf("expected max_position_weight 0.12, got %f", policy.Constraints.MaxPositionWeight)
	}
	if policy.ExecutionPolicy.ConvictionFloor != 55 {
		t.Fatalf("expected conviction_floor 55, got %d", policy.ExecutionPolicy.ConvictionFloor)
	}
}

func TestCanonicalSnakeCaseRewrittenCleanly(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	canonical := `{
		"version": 7,
		"prompt_overrides": {"cio-01": "v3"},
		"constraints": {
			"starting_cash": 2000000,
			"max_position_weight": 0.10,
			"max_open_positions": 10,
			"min_tradable_volume": 1000000,
			"min_recommendation_conviction": 65,
			"require_cro_pass": false,
			"transaction_cost_bps": 1.0,
			"slippage_bps": 1.5,
			"reserve_cash_fraction": 0.05
		},
		"execution_policy": {
			"conviction_floor": 65,
			"require_cro_pass": false
		},
		"promotions": [],
		"revert_history": [],
		"last_updated_at": "2026-05-01T12:00:00Z"
	}`

	if err := os.WriteFile(policyPath, []byte(canonical), 0o644); err != nil {
		t.Fatalf("write canonical policy: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read rewritten policy: %v", err)
	}

	var policy baseline.Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("unmarshal rewritten policy: %v", err)
	}
	if policy.Version != 7 {
		t.Fatalf("expected version 7, got %d", policy.Version)
	}
	if policy.Constraints.MaxPositionWeight != 0.10 {
		t.Fatalf("expected max_position_weight 0.10, got %f", policy.Constraints.MaxPositionWeight)
	}
	if policy.ExecutionPolicy.ConvictionFloor != 65 {
		t.Fatalf("expected conviction_floor 65, got %d", policy.ExecutionPolicy.ConvictionFloor)
	}
}

func TestNestedConstraintsAndExecutionPolicyUseSnakeCase(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	legacy := `{
		"Version": 1,
		"Constraints": {
			"StartingCash": 500000,
			"MaxPositionWeight": 0.20,
			"MaxOpenPositions": 20,
			"MinTradableVolume": 300000,
			"MinRecommendationConviction": 50,
			"RequireCROPass": true,
			"TransactionCostBPS": 2.0,
			"SlippageBPS": 3.0,
			"ReserveCashFraction": 0.10
		},
		"ExecutionPolicy": {
			"ConvictionFloor": 50,
			"RequireCROPass": true
		}
	}`

	if err := os.WriteFile(policyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy policy: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read rewritten policy: %v", err)
	}

	text := string(data)
	if strings.Contains(text, `"StartingCash"`) {
		t.Fatalf("expected nested PascalCase StartingCash rewritten, got %s", text)
	}
	if !strings.Contains(text, `"starting_cash"`) {
		t.Fatalf("expected nested snake_case starting_cash, got %s", text)
	}
	if strings.Contains(text, `"ConvictionFloor"`) {
		t.Fatalf("expected nested PascalCase ConvictionFloor rewritten, got %s", text)
	}
	if !strings.Contains(text, `"conviction_floor"`) {
		t.Fatalf("expected nested snake_case conviction_floor, got %s", text)
	}
}

func TestMalformedJSONReturnsContextualError(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	if err := os.WriteFile(policyPath, []byte(`{ invalid json`), 0o644); err != nil {
		t.Fatalf("write malformed policy: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"-state-dir", dir}, &stdout)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, policyPath) && !strings.Contains(errMsg, "baseline_policy.json") {
		t.Fatalf("expected error to contain file path, got %q", errMsg)
	}
}

func TestSemanticValuesPreserved(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	legacy := `{
		"Version": 42,
		"PromptOverrides": {"growth-momentum-01": "prompt-v5", "value-yield-01": "prompt-v2"},
		"Constraints": {
			"StartingCash": 1500000,
			"MaxPositionWeight": 0.18,
			"MaxOpenPositions": 12,
			"MinTradableVolume": 750000,
			"MinRecommendationConviction": 70,
			"RequireCROPass": false,
			"TransactionCostBPS": 0.8,
			"SlippageBPS": 1.2,
			"ReserveCashFraction": 0.15
		},
		"ExecutionPolicy": {
			"ConvictionFloor": 70,
			"RequireCROPass": false
		},
		"Promotions": [
			{
				"ExperimentID": "exp-1",
				"TargetAgentID": "growth-momentum-01",
				"TargetSkill": "growth_momentum",
				"MutationType": "prompt_tightening",
				"CandidatePath": "prompts/experiments/growth/v2.md",
				"PromotedAt": "2026-03-15T10:00:00Z",
				"Status": "accepted",
				"VersionAfter": 2
			}
		],
		"RevertHistory": [],
		"LastUpdatedAt": "2026-04-10T08:30:00Z"
	}`

	if err := os.WriteFile(policyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy policy: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read rewritten policy: %v", err)
	}

	var policy baseline.Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("unmarshal rewritten policy: %v", err)
	}

	if policy.Version != 42 {
		t.Fatalf("expected version 42, got %d", policy.Version)
	}
	if len(policy.PromptOverrides) != 2 {
		t.Fatalf("expected 2 prompt overrides, got %d", len(policy.PromptOverrides))
	}
	if policy.PromptOverrides["growth-momentum-01"] != "prompt-v5" {
		t.Fatalf("expected growth-momentum-01 override preserved")
	}
	if policy.Constraints.StartingCash != 1500000 {
		t.Fatalf("expected starting_cash 1500000, got %f", policy.Constraints.StartingCash)
	}
	if policy.Constraints.MaxOpenPositions != 12 {
		t.Fatalf("expected max_open_positions 12, got %d", policy.Constraints.MaxOpenPositions)
	}
	if policy.Constraints.MinTradableVolume != 750000 {
		t.Fatalf("expected min_tradable_volume 750000, got %d", policy.Constraints.MinTradableVolume)
	}
	if policy.Constraints.MinRecommendationConviction != 70 {
		t.Fatalf("expected min_recommendation_conviction 70, got %d", policy.Constraints.MinRecommendationConviction)
	}
	if policy.ExecutionPolicy.RequireCROPass {
		t.Fatalf("expected require_cro_pass false")
	}
	if len(policy.Promotions) != 1 {
		t.Fatalf("expected 1 promotion, got %d", len(policy.Promotions))
	}
	if policy.Promotions[0].ExperimentID != "exp-1" {
		t.Fatalf("expected promotion experiment_id exp-1, got %s", policy.Promotions[0].ExperimentID)
	}
	if policy.Promotions[0].VersionAfter != 2 {
		t.Fatalf("expected promotion version_after 2, got %d", policy.Promotions[0].VersionAfter)
	}
}

func TestAtomicRewriteNoPartialFileOnError(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	if err := os.WriteFile(policyPath, []byte(`{"Version": 1}`), 0o644); err != nil {
		t.Fatalf("write initial policy: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-state-dir", dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") && strings.Contains(entry.Name(), "convert-baseline-policy") {
			t.Fatalf("expected no leftover temp files, found %s", entry.Name())
		}
	}
}
