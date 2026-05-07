# Historical State Canonical Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 對 `data/state/` 歷史 persistence artifacts 執行 archive-first 的 canonical rewrite，將 PascalCase / mixed-case JSON 與 JSONL 重寫為正式 snake_case 契約，並保留完整可回復封存。

**Architecture:** 先補齊仍缺失的 canonical struct tags，確保正式 writer contract 完整；再新增 archive-first CLI 與各 artifact 類型的 converter，最後用單一 orchestration command 對實際 `data/state` 執行封存、轉換與驗證。所有轉換都必須走「decode old artifact through compatibility path → re-marshal canonical snake_case → atomic rewrite」模式，不做字串取代。

**Tech Stack:** Go 1.25、`encoding/json`、`os` / `filepath`、native `go test`、現有 `cmd/check-persistence-format`

---

## 檔案與責任分解

| 路徑 | 責任 |
|------|------|
| `internal/domain/types.go` | 補齊 `SimulationConstraints` 與 `ExecutionPolicy` 的 snake_case json tags，解除 `baseline_policy.json` canonical rewrite blocker |
| `internal/domain/domain_test.go` | 驗證上述兩個 struct marshal 後確實只輸出 snake_case |
| `cmd/archive-state/main.go` | 封存 `data/state` 至 `data/state-archive/<timestamp>/` |
| `cmd/archive-state/main_test.go` | 驗證 archive 目錄、結構與檔案內容保留 |
| `cmd/convert-recommendation-outcomes/main.go` | 重寫 root + session `recommendation_outcomes.jsonl` |
| `cmd/convert-recommendation-outcomes/main_test.go` | 驗證 legacy PascalCase line decode + canonical rewrite |
| `cmd/convert-experiments-jsonl/main.go` | 重寫 root + session `experiments.jsonl` |
| `cmd/convert-experiments-jsonl/main_test.go` | 驗證 `ExperimentRecord` JSONL rewrite |
| `cmd/convert-experiment-results/main.go` | 重寫 `data/state/experiments/*.json` |
| `cmd/convert-experiment-results/main_test.go` | 驗證 `PromptExperimentResult` nested rewrite |
| `cmd/convert-baseline-policy/main.go` | 重寫 `data/state/baseline_policy.json` |
| `cmd/convert-baseline-policy/main_test.go` | 驗證 `Policy` + nested constraints / execution_policy canonical rewrite |
| `cmd/rewrite-state/main.go` | 串接 archive + converters + verifier |
| `cmd/rewrite-state/main_test.go` | 驗證完整 archive-first workflow |

---

## 風險與相依

### GitNexus 風險摘要

- `Store.RecordOutcomes` → **CRITICAL** upstream risk；因此本 phase 不修改 runtime writer 行為，只新增 archive/conversion 工具。
- `Store.RecordSessionSummary` → **LOW** upstream risk；現階段不預設重寫 summary，除非 inventory 顯示仍非 canonical。

### 執行順序

1. **Task 1** 必須先完成，否則 `baseline_policy.json` 的 nested `constraints` / `execution_policy` 仍會輸出 PascalCase。
2. Archive CLI 完成前，不可改寫任何正式 state。
3. Recommendation outcomes、experiments JSONL、experiment result JSON 可平行實作。
4. `rewrite-state` orchestration 必須最後才做。

---

## Task 1: 補齊 `SimulationConstraints` / `ExecutionPolicy` 的 canonical tags

**Files:**
- Modify: `internal/domain/types.go`
- Test: `internal/domain/domain_test.go`

- [ ] **Step 1: 寫失敗測試 — `SimulationConstraints` 只能輸出 snake_case**

```go
func TestSimulationConstraintsMarshalUsesSnakeCase(t *testing.T) {
	constraints := SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.05,
		MaxOpenPositions:            10,
		MinTradableVolume:           100000,
		MinRecommendationConviction: 60,
		RequireCROPass:              true,
		TransactionCostBPS:          5,
		SlippageBPS:                 3,
		ReserveCashFraction:         0.1,
	}

	data, err := json.Marshal(constraints)
	if err != nil {
		t.Fatalf("marshal constraints: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"starting_cash"`) {
		t.Fatalf("expected snake_case starting_cash; got %s", text)
	}
	if strings.Contains(text, `"StartingCash"`) {
		t.Fatalf("unexpected PascalCase StartingCash; got %s", text)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/domain -run TestSimulationConstraintsMarshalUsesSnakeCase -count=1`

Expected: FAIL，因為目前 `SimulationConstraints` 缺少 json tags。

- [ ] **Step 3: 寫失敗測試 — `ExecutionPolicy` 只能輸出 snake_case**

```go
func TestExecutionPolicyMarshalUsesSnakeCase(t *testing.T) {
	policy := ExecutionPolicy{ConvictionFloor: 60, RequireCROPass: true}

	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"conviction_floor"`) {
		t.Fatalf("expected snake_case conviction_floor; got %s", text)
	}
	if strings.Contains(text, `"ConvictionFloor"`) {
		t.Fatalf("unexpected PascalCase ConvictionFloor; got %s", text)
	}
}
```

- [ ] **Step 4: 執行測試確認失敗**

Run: `go test ./internal/domain -run TestExecutionPolicyMarshalUsesSnakeCase -count=1`

Expected: FAIL

- [ ] **Step 5: 補齊 struct tags**

在 `internal/domain/types.go` 為下列欄位補上 snake_case tags：

```go
type SimulationConstraints struct {
	StartingCash                float64 `json:"starting_cash"`
	MaxPositionWeight           float64 `json:"max_position_weight"`
	MaxOpenPositions            int     `json:"max_open_positions"`
	MinTradableVolume           int64   `json:"min_tradable_volume"`
	MinRecommendationConviction int     `json:"min_recommendation_conviction"`
	RequireCROPass              bool    `json:"require_cro_pass"`
	TransactionCostBPS          float64 `json:"transaction_cost_bps"`
	SlippageBPS                 float64 `json:"slippage_bps"`
	ReserveCashFraction         float64 `json:"reserve_cash_fraction"`
	StopLossPct                 float64 `json:"stop_loss_pct,omitempty"`
	TakeProfitPct               float64 `json:"take_profit_pct,omitempty"`
}

type ExecutionPolicy struct {
	ConvictionFloor int  `json:"conviction_floor"`
	RequireCROPass  bool `json:"require_cro_pass"`
}
```

- [ ] **Step 6: 執行 domain / baseline 測試確認通過**

Run:

```bash
go test ./internal/domain -count=1
go test ./internal/baseline -count=1
```

Expected: PASS

---

## Task 2: 建立 archive CLI (`cmd/archive-state`)

**Files:**
- Create: `cmd/archive-state/main.go`
- Test: `cmd/archive-state/main_test.go`

- [ ] **Step 1: 寫失敗測試 — 建立 timestamped archive 目錄**

```go
func TestArchiveStateCreatesTimestampedCopy(t *testing.T) {
	src := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(filepath.Join(src, "sessions", "s1"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "baseline_policy.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	archiveBase := filepath.Join(t.TempDir(), "state-archive")

	var stdout bytes.Buffer
	if err := run([]string{"-src", src, "-dst-base", archiveBase}, &stdout); err != nil {
		t.Fatalf("run archive: %v", err)
	}

	entries, err := os.ReadDir(archiveBase)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 timestamped archive dir, got %d", len(entries))
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./cmd/archive-state -run TestArchiveStateCreatesTimestampedCopy -count=1`

Expected: FAIL

- [ ] **Step 3: 實作最小 archive workflow**

需求：
- flags: `-src` (default `data/state`), `-dst-base` (default `data/state-archive`)
- 生成 UTC timestamp 目錄（例：`20260506T142300Z`）
- 遞迴複製檔案與目錄
- stdout 印出 archive path

- [ ] **Step 4: 再補測試 — 保留目錄結構與檔案內容**

```go
func TestArchiveStatePreservesDirectoryStructure(t *testing.T) {
	// 建立 sessions/s1/summary.json 與 root recommendation_outcomes.jsonl
	// 執行 run()
	// 驗證 archive 下相同相對路徑存在且內容一致
}
```

- [ ] **Step 5: 執行測試確認通過**

Run: `go test ./cmd/archive-state -count=1`

Expected: PASS

---

## Task 3: 建立 recommendation outcomes converter (`cmd/convert-recommendation-outcomes`)

**Files:**
- Create: `cmd/convert-recommendation-outcomes/main.go`
- Test: `cmd/convert-recommendation-outcomes/main_test.go`

- [ ] **Step 1: 寫失敗測試 — PascalCase line 可 decode 並 rewrite 為 snake_case**

```go
func TestConvertLineReEncodesPascalCaseToSnakeCase(t *testing.T) {
	legacy := []byte(`{"AgentID":"agent-1","Skill":"test","Layer":"sector","Symbol":"2330.TW","Side":"BUY","Conviction":80,"TargetPrice":600,"StopLossPrice":550,"Window":"1d","ForwardReturn":0.02,"BenchmarkDelta":0.01,"Hit":true,"Reason":"test","Price":580,"PassedGuards":true,"GuardReason":"","RecordedAt":"2026-01-01T00:00:00Z"}`)

	converted, err := convertLine(legacy)
	if err != nil {
		t.Fatalf("convertLine: %v", err)
	}
	text := string(converted)
	if !strings.Contains(text, `"agent_id"`) || strings.Contains(text, `"AgentID"`) {
		t.Fatalf("expected canonical snake_case output; got %s", text)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./cmd/convert-recommendation-outcomes -run TestConvertLineReEncodesPascalCaseToSnakeCase -count=1`

Expected: FAIL

- [ ] **Step 3: 實作 `convertLine()` 與 `convertFile()`**

規則：
- `domain.RecommendationOutcome` 做 decode
- `json.Marshal` 做 canonical rewrite
- `.tmp` 寫入後 rename
- 保留 line count

- [ ] **Step 4: 再補測試 — root + session-level JSONL line count 不變**

```go
func TestConvertFilePreservesLineCount(t *testing.T) {
	// 建立兩行 legacy JSONL，執行 convertFile()
	// 驗證輸出仍為兩行，且每行都是 snake_case
}
```

- [ ] **Step 5: 執行測試確認通過**

Run: `go test ./cmd/convert-recommendation-outcomes -count=1`

Expected: PASS

---

## Task 4: 建立 experiments JSONL converter (`cmd/convert-experiments-jsonl`)

**Files:**
- Create: `cmd/convert-experiments-jsonl/main.go`
- Test: `cmd/convert-experiments-jsonl/main_test.go`

- [ ] **Step 1: 寫失敗測試 — legacy `ExperimentRecord` line rewrite 為 snake_case**

```go
func TestConvertExperimentLineReEncodesPascalToSnake(t *testing.T) {
	legacy := []byte(`{"ID":"exp-1","TargetAgentID":"agent-1","Skill":"test","MutationType":"prompt_tightening","Status":"planned"}`)
	converted, err := convertLine(legacy)
	if err != nil {
		t.Fatalf("convertLine: %v", err)
	}
	text := string(converted)
	if !strings.Contains(text, `"target_agent_id"`) || strings.Contains(text, `"TargetAgentID"`) {
		t.Fatalf("expected snake_case rewrite; got %s", text)
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./cmd/convert-experiments-jsonl -run TestConvertExperimentLineReEncodesPascalToSnake -count=1`

Expected: FAIL

- [ ] **Step 3: 實作最小 converter**

使用 `domain.ExperimentRecord` decode/re-encode。

- [ ] **Step 4: 補 file-level 測試並確認通過**

Run: `go test ./cmd/convert-experiments-jsonl -count=1`

Expected: PASS

---

## Task 5: 建立 experiment result converter (`cmd/convert-experiment-results`)

**Files:**
- Create: `cmd/convert-experiment-results/main.go`
- Test: `cmd/convert-experiment-results/main_test.go`

- [ ] **Step 1: 寫失敗測試 — `PromptExperimentResult` outer + nested 都 rewrite**

```go
func TestConvertExperimentResultFileReEncodesPascalToSnake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exp.json")
	legacy := `{"Experiment":{"ID":"exp-1","TargetAgentID":"agent-1","MutationType":"prompt_tightening","Status":"accepted"},"Brief":{"window_id":"w1","mutation_type":"prompt_tightening"},"CandidatePrompt":"candidate.md","EvaluationMode":"replay"}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := convertFile(path); err != nil {
		t.Fatalf("convertFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"experiment"`) || strings.Contains(text, `"Experiment"`) {
		t.Fatalf("expected canonical outer envelope; got %s", text)
	}
	if !strings.Contains(text, `"target_agent_id"`) || strings.Contains(text, `"TargetAgentID"`) {
		t.Fatalf("expected canonical nested field; got %s", text)
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./cmd/convert-experiment-results -run TestConvertExperimentResultFileReEncodesPascalToSnake -count=1`

Expected: FAIL

- [ ] **Step 3: 實作 file converter**

規則：
- `domain.PromptExperimentResult` decode
- `json.MarshalIndent` rewrite
- atomic write

- [ ] **Step 4: Run suite**

Run: `go test ./cmd/convert-experiment-results -count=1`

Expected: PASS

---

## Task 6: 建立 baseline policy converter (`cmd/convert-baseline-policy`)

**Files:**
- Create: `cmd/convert-baseline-policy/main.go`
- Test: `cmd/convert-baseline-policy/main_test.go`

- [ ] **Step 1: 寫失敗測試 — baseline policy nested constraints / execution_policy 也必須 snake_case**

```go
func TestConvertBaselinePolicyReEncodesPascalToSnake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline_policy.json")
	legacy := `{"Version":1,"PromptOverrides":{"agent-1":"prompt-v2"},"Constraints":{"StartingCash":1000000,"MaxPositionWeight":0.05,"MaxOpenPositions":10,"MinTradableVolume":100000,"MinRecommendationConviction":60,"RequireCROPass":true,"TransactionCostBPS":5,"SlippageBPS":3,"ReserveCashFraction":0.1},"ExecutionPolicy":{"ConvictionFloor":60,"RequireCROPass":true}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := convertFile(path); err != nil {
		t.Fatalf("convertFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"version"`) || strings.Contains(text, `"Version"`) {
		t.Fatalf("expected canonical version; got %s", text)
	}
	if !strings.Contains(text, `"starting_cash"`) || strings.Contains(text, `"StartingCash"`) {
		t.Fatalf("expected canonical nested constraints; got %s", text)
	}
	if !strings.Contains(text, `"conviction_floor"`) || strings.Contains(text, `"ConvictionFloor"`) {
		t.Fatalf("expected canonical nested execution policy; got %s", text)
	}
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./cmd/convert-baseline-policy -run TestConvertBaselinePolicyReEncodesPascalToSnake -count=1`

Expected: FAIL until Task 1 tags exist and converter is implemented.

- [ ] **Step 3: 實作 policy converter**

使用 `baseline.Policy` decode/re-encode。

- [ ] **Step 4: Run suite**

Run: `go test ./cmd/convert-baseline-policy -count=1`

Expected: PASS

---

## Task 7: 建立 orchestration command (`cmd/rewrite-state`)

**Files:**
- Create: `cmd/rewrite-state/main.go`
- Test: `cmd/rewrite-state/main_test.go`

- [ ] **Step 1: 寫失敗整合測試 — archive + rewrite 全流程**

```go
func TestRewriteStateFullWorkflow(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	// 建立 baseline_policy.json、recommendation_outcomes.jsonl、experiments.jsonl、experiments/exp.json 的 PascalCase fixture
	// 執行 run([]string{"-state-dir", stateDir, "-archive-base", archiveBase}, &stdout)
	// 驗證 archive 存在
	// 驗證所有正式 artifacts 都不再含 PascalCase keys
}
```

- [ ] **Step 2: Run red test**

Run: `go test ./cmd/rewrite-state -run TestRewriteStateFullWorkflow -count=1`

Expected: FAIL

- [ ] **Step 3: 實作 orchestration**

流程：
1. archive state
2. rewrite baseline policy
3. rewrite root + session recommendation outcomes
4. rewrite root + session experiments.jsonl
5. rewrite `experiments/*.json`
6. run inventory verifier and print summary

- [ ] **Step 4: Run suite**

Run: `go test ./cmd/rewrite-state -count=1`

Expected: PASS

---

## Task 8: 實際執行 canonical rewrite 與驗證

**Files:**
- Modify: runtime state only under `data/state/**`
- Output: archive under `data/state-archive/<timestamp>/`

- [ ] **Step 1: Dry-run inventory before rewrite**

Run:

```bash
go run ./cmd/check-persistence-format -dir data/state
```

Expected: 顯示大量 `recommendation_outcomes.jsonl` legacy PascalCase evidence。

- [ ] **Step 2: 執行 rewrite-state**

Run:

```bash
go run ./cmd/rewrite-state -state-dir data/state -archive-base data/state-archive
```

Expected:
- 新 archive 目錄建立
- 正式 `data/state` artifacts 被 canonical rewrite

- [ ] **Step 3: 再跑 inventory 驗證正式 state**

Run:

```bash
go run ./cmd/check-persistence-format -dir data/state
```

Expected:
- `recommendation_outcomes.jsonl` 不再回報 PascalCase keys
- type B artifacts 轉為 snake_case
- 若仍有 `summary.json` / 其他檔案不是 snake_case，記錄為 follow-up

- [ ] **Step 4: 執行 focused + repo verification**

Run:

```bash
go test ./cmd/archive-state ./cmd/convert-recommendation-outcomes ./cmd/convert-experiments-jsonl ./cmd/convert-experiment-results ./cmd/convert-baseline-policy ./cmd/rewrite-state -count=1
go test ./...
go vet ./...
```

If available:

```bash
staticcheck ./...
```

- [ ] **Step 5: 記錄 rollback 指令**

```bash
rm -rf data/state && cp -R data/state-archive/<timestamp> data/state
go run ./cmd/check-persistence-format -dir data/state
```

---

## Self-Review

- Spec coverage: 已覆蓋 archive、type B conversion、recommendation outcomes rewrite、verification、rollback。
- Placeholder scan: 無 TBD / TODO。
- Consistency: 所有 converters 都使用「decode old → marshal canonical → atomic rewrite」同一模式；`baseline_policy` 明確依賴 Task 1。
