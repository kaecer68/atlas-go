# 持久化格式統一 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 將實驗、baseline 與治理相關持久化契約統一為 snake_case，修正依賴 PascalCase 的 reader / shell flow，並建立重建/轉換前的驗證護欄。

**Architecture:** 先 canonicalize writers（domain + baseline struct tags），再修正所有依賴舊 PascalCase keys 的讀取路徑與 shell 腳本，最後補上 artifact inventory / writer-consistency 驗證與測試。對可重建資料只建立驗證工具，不在本階段直接改寫實際 state。對不可重建的治理證據資料保留轉換入口與 archive-first 流程。

**Tech Stack:** Go 1.25、bash/OpenClaw scripts、JSON/JSONL persistence、go test/go vet

---

## Task 1: Canonicalize experiment persistence contracts

**Files:**
- Modify: `internal/domain/registry.go`
- Test: `internal/domain/registry_test.go`

- [ ] **Step 1: 寫失敗測試 — ExperimentRecord 必須輸出 snake_case**

```go
func TestExperimentRecordMarshalUsesSnakeCase(t *testing.T) {
	record := ExperimentRecord{
		ID:            "exp-1",
		ProposalID:    "proposal-1",
		TargetAgentID: "growth-momentum-01",
		MutationType:  "prompt_tightening",
		Status:        ExperimentAccepted,
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"id"`) {
		t.Fatalf("expected snake_case id; got %s", text)
	}
	if strings.Contains(text, `"TargetAgentID"`) {
		t.Fatalf("unexpected PascalCase TargetAgentID; got %s", text)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/domain -run TestExperimentRecordMarshalUsesSnakeCase -count=1`

Expected: FAIL，因為 `ExperimentRecord` 目前沒有 `json` tags，會輸出 PascalCase。

- [ ] **Step 3: 補齊 `ExperimentRecord` 的 snake_case json tags**

要補的欄位至少包括：

```go
type ExperimentRecord struct {
	ID                string           `json:"id"`
	ProposalID        string           `json:"proposal_id"`
	CommitID          string           `json:"commit_id"`
	ApprovalID        string           `json:"approval_id"`
	TargetAgentID     string           `json:"target_agent_id"`
	Skill             string           `json:"skill"`
	Hypothesis        string           `json:"hypothesis"`
	PromptVersionFrom string           `json:"prompt_version_from"`
	PromptVersionTo   string           `json:"prompt_version_to"`
	MutationType      string           `json:"mutation_type"`
	AcceptanceGates   []string         `json:"acceptance_gates"`
	WindowStart       time.Time        `json:"window_start"`
	WindowEnd         time.Time        `json:"window_end"`
	AcceptanceMetric  string           `json:"acceptance_metric"`
	BaselineValue     float64          `json:"baseline_value"`
	CandidateValue    float64          `json:"candidate_value"`
	Status            ExperimentStatus `json:"status"`
	RevertReason      string           `json:"revert_reason"`
}
```

- [ ] **Step 4: 寫失敗測試 — PromptExperimentResult 外層欄位也必須 snake_case**

```go
func TestPromptExperimentResultMarshalUsesSnakeCaseEnvelope(t *testing.T) {
	result := PromptExperimentResult{
		Experiment: ExperimentRecord{ID: "exp-1", Status: ExperimentAccepted},
		Brief: MutationBrief{WindowID: "window-1", TargetAgentID: "growth-momentum-01", TargetSkill: "growth_momentum", TargetLayer: LayerStyle, PromptFile: "prompts/agents/growth_momentum.md", MutationType: "prompt_tightening", FailurePattern: "weak_momentum", Hypothesis: "tighten exit", AcceptanceMetric: "sharpe_like", AcceptanceGates: []string{"improve_sharpe_like"}},
		CandidatePrompt: "v2 prompt",
		EvaluationMode:  "replay",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"experiment"`) || !strings.Contains(text, `"brief"`) {
		t.Fatalf("expected snake_case envelope; got %s", text)
	}
	if strings.Contains(text, `"Experiment"`) || strings.Contains(text, `"Brief"`) {
		t.Fatalf("unexpected PascalCase envelope; got %s", text)
	}
}
```

- [ ] **Step 5: 執行測試確認失敗**

Run: `go test ./internal/domain -run TestPromptExperimentResultMarshalUsesSnakeCaseEnvelope -count=1`

Expected: FAIL，因為 `PromptExperimentResult` 外層欄位目前仍是 PascalCase。

- [ ] **Step 6: 補齊 `PromptExperimentResult` 外層欄位 tags**

至少補：

```go
type PromptExperimentResult struct {
	Experiment            ExperimentRecord `json:"experiment"`
	Brief                 MutationBrief    `json:"brief"`
	CandidatePrompt       string           `json:"candidate_prompt"`
	EvaluationMode        string           `json:"evaluation_mode"`
	PolicyChecks          []string         `json:"policy_checks"`
	Notes                 []string         `json:"notes"`
	JudgeChecks           []string         `json:"judge_checks"`
	BaselineObservations  int              `json:"baseline_observations"`
	CandidateObservations int              `json:"candidate_observations"`
	UsedFallbackWindow    bool             `json:"used_fallback_window"`
	RecordedAt            time.Time        `json:"recorded_at"`
	// 既有 snake_case tagged fields 保持不變
}
```

- [ ] **Step 7: 執行 domain 測試確認通過**

Run: `go test ./internal/domain -count=1`

Expected: PASS

---

## Task 2: Canonicalize baseline policy persistence contracts

**Files:**
- Modify: `internal/baseline/policy.go`
- Modify: `internal/baseline/rollback.go` (若有落地結構需對齊)
- Test: `internal/baseline/*_test.go`

- [ ] **Step 1: 寫失敗測試 — baseline policy 必須輸出 snake_case**

```go
func TestPolicyMarshalUsesSnakeCase(t *testing.T) {
	policy := Policy{Version: 2, PromptOverrides: map[string]string{"growth_momentum": "v2"}}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"prompt_overrides"`) {
		t.Fatalf("expected prompt_overrides; got %s", text)
	}
	if strings.Contains(text, `"PromptOverrides"`) {
		t.Fatalf("unexpected PascalCase PromptOverrides; got %s", text)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/baseline -run TestPolicyMarshalUsesSnakeCase -count=1`

Expected: FAIL

- [ ] **Step 3: 補齊 `Policy` / `PromotionRecord` / `RevertRecord` json tags**

至少補：

```go
type Policy struct {
	Version         int               `json:"version"`
	PromptOverrides map[string]string `json:"prompt_overrides"`
	Constraints     domain.SimulationConstraints `json:"constraints"`
	ExecutionPolicy domain.ExecutionPolicy       `json:"execution_policy"`
	Promotions      []PromotionRecord `json:"promotions"`
	RevertHistory   []RevertRecord    `json:"revert_history"`
	LastUpdatedAt   time.Time         `json:"last_updated_at"`
}
```

- [ ] **Step 4: 執行 baseline 測試確認通過**

Run: `go test ./internal/baseline -count=1`

Expected: PASS

---

## Task 3: Remove PascalCase dependency from OpenClaw decision flow

**Files:**
- Modify: `scripts/openclaw/decide.sh`
- Test/Verify: add focused shell-safe verification via existing JSON fixtures or Go-generated files

- [ ] **Step 1: 寫失敗驗證案例 — `decide.sh` 必須能讀 snake_case experiment JSON**

建立最小 fixture JSON（snake_case）並驗證：

```json
{
  "experiment": {
    "status": "accepted",
    "baseline_value": 0.1,
    "candidate_value": 0.2,
    "target_agent_id": "growth-momentum-01"
  }
}
```

Run: `bash scripts/openclaw/decide.sh --promote exp-test --reason "ok" --dry-run`

Expected: 目前 FAIL 或解析為空值，因為腳本 grep 的是 `"Status"` 等 PascalCase keys。

- [ ] **Step 2: 修改 `decide.sh` 使其接受 canonical snake_case**

策略：
- 優先支援 snake_case
- 若需要短期過渡，可同時接受 PascalCase
- 不再把 PascalCase 視為正式契約

建議方式：把：

```bash
grep '"Status"'
```

改為同時支援：

```bash
jq -r '.experiment.status // .Experiment.Status // empty'
```

同理套用到：
- baseline value
- candidate value
- target agent id
- version

- [ ] **Step 3: 執行腳本驗證**

Run: `bash scripts/openclaw/decide.sh --promote exp-test --reason "ok" --dry-run`

Expected: 能正確讀出 snake_case fixture 的 experiment 資訊

---

## Task 4: Add artifact inventory / writer consistency verification

**Files:**
- Create or modify: lightweight verification script/tool under `scripts/openclaw/` or `cmd/` (choose one consistent with existing patterns)
- Test: focused verification command output

- [ ] **Step 1: 建立 inventory/verification 入口**

功能至少包括：
- 掃描 `data/state/**`
- 報告 PascalCase / snake_case / mixed-case 檔案
- 特別標示：
  - `sessions/*/summary.json`
  - `sessions/*/recommendation_outcomes.jsonl`
  - `recommendation_outcomes.jsonl`
  - `experiments.jsonl`
  - `experiments/*.json`
  - `baseline_policy.json`

- [ ] **Step 2: 加入 recommendation_outcomes writer consistency 檢查**

檢查重點：
- session-level 與 root-level `recommendation_outcomes.jsonl` 是否都能被 `domain.RecommendationOutcome` 正確 decode
- 重新 encode 後是否都變成 snake_case

- [ ] **Step 3: 執行驗證命令**

Run one of:

```bash
go run ./cmd/<tool>
```

or

```bash
bash ./scripts/openclaw/<tool>.sh
```

Expected: 清楚列出 artifact 分類與 writer consistency 結果

---

## Task 5: Final verification and pre-launch cleanup summary

**Files:**
- Modify as needed: tests only
- Output: verification summary in session / follow-up note

- [ ] **Step 1: 格式化所有變更檔案**

Run: `gofmt -w internal/domain/*.go internal/baseline/*.go internal/monitoring/service/*.go`

- [ ] **Step 2: 執行聚焦測試**

Run:

```bash
go test ./internal/domain -count=1
go test ./internal/baseline -count=1
go test ./internal/monitoring/service -count=1
go test ./internal/ledger -count=1
```

- [ ] **Step 3: 執行治理相關驗證**

Run:

```bash
go test ./internal/experiment/... ./internal/baseline/... -count=1
```

- [ ] **Step 4: 執行 repo 級驗證**

Run:

```bash
go test ./...
go vet ./...
```

If available:

```bash
staticcheck ./...
```

- [ ] **Step 5: 列出仍待後續處理的 pre-launch cleanup gaps**

至少檢查：
- legacy compatibility 是否仍必要
- 哪些 artifact 屬於 rebuild、哪些屬於 convert
- 是否還有 shell / monitoring 路徑依賴 PascalCase
