# Eliminate Engine Config Dual Source of Truth

## TL;DR

> **Quick Summary**: 用 `//go:embed` 將 `engine.json` 嵌入編譯期，讓 JSON 檔案成為唯一真相來源，消除 `defaultEngineConfig()` 中 177 行重複硬編碼。這是 10 分鐘的預防性修復，歷史已證明同樣的同步問題在一天內反覆出現了 3 次。

> **Deliverables**:
> - `internal/config/engine.json` — 從 `configs/` 移入的單一真相來源
> - `internal/config/engine_config.go` — `defaultEngineConfig()` 從 177 行簡化為 ~12 行
> - `internal/config/engine_config_test.go` — 簡化 `setProjectRoot()`，更新路徑引用

> **Estimated Effort**: Quick（1-2 個任務，<30 分鐘）
> **Parallel Execution**: NO（sequential — 兩個任務有依賴關係）
> **Critical Path**: 移動 engine.json → 重構 engine_config.go → 更新測試

---

## Context

### Original Request
消除 `configs/engine.json` 和 `defaultEngineConfig()` 之間的雙重真相來源，防止未來再次出現同步不同步的問題。

### Interview Summary

**Key Discussions**:
- **問題頻率**: 日常修改頻率低，但每當有人加新 sector/參數時，忘記同步更新 JSON 的機率極高（5/21 同一天內反覆出現 3 次）
- **問題嚴重性**: 不會 crash，但會靜默 fallback 到不完整的預設值，缺少的行業直接從計算中消失
- **維護成本**: `//go:embed` 方案實際上**減少**了維護負擔（只改一個地方，不再是兩個）
- **Test decision**: Tests-after（現有測試覆蓋行為，重構後微調）

**Research Findings**:
- Go 1.25.0: `//go:embed` 從 1.16 起支援 ✓
- Callers of `defaultEngineConfig()`: 僅 `GetEngineConfig()`（fallback）和測試檔案
- `//go:embed` pattern 不能含 `..` → engine.json 必須移到 `internal/config/`
- 無外部腳本 hardcode `configs/engine.json` 路徑
- `/admin/reload-config` HTTP 端點依賴 `ReloadEngineConfig()`，使用 env var override 時仍可用

### Metis Review

**Identified Gaps** (addressed):
- **Env var override 保留**: ✅ Hybrid 模式 — 設了 `ATLAS_ENGINE_CONFIG` 就用 disk 路徑，沒設就用 embedded bytes
- **Admin reload 端點保留**: ✅ 只在 env var override 啟用時生效，行為不變
- **舊檔案刪除**: ✅ 只留一份在 `internal/config/engine.json`
- **Race condition**: ✅ 明確排除，另外處理

---

## Work Objectives

### Core Objective
用 `//go:embed` 讓 `engine.json` 成為唯一真相來源，`defaultEngineConfig()` 從此直接解析 embedded JSON，不可能再不同步。

### Concrete Deliverables
- `configs/engine.json` → 移動到 `internal/config/engine.json`
- `internal/config/engine_config.go`: `defaultEngineConfig()` 替換為 embedded JSON 解析（~177 行 → ~12 行）
- `internal/config/engine_config_test.go`: `setProjectRoot()` 移除 chdir 邏輯，路徑引用更新

### Definition of Done
- [ ] `git mv configs/engine.json internal/config/engine.json` 完成
- [ ] `defaultEngineConfig()` 使用 `//go:embed engine.json` + `json.Unmarshal`
- [ ] `go build ./...` 通過
- [ ] `go test ./internal/config/...` 全部 PASS
- [ ] `gofmt -l .` 無差異
- [ ] `ATLAS_ENGINE_CONFIG` env var override 仍可運作
- [ ] `/admin/reload-config` HTTP 端點不受影響

### Must Have
- `//go:embed engine.json` 在 `defaultEngineConfig()` 中作為 fallback
- Hybrid 模式：env var override 優先，無 env var 時使用 embedded 版本
- `LoadEngineConfig()` 行為完全不變（讀取 disk 檔案 → Validate → 回傳）
- `GetEngineConfig()` 行為完全不變（已載入 → 回傳；未載入 → Load → fallback to default）
- `ReloadEngineConfig()` 行為完全不變（重讀 disk 檔案）

### Must NOT Have (Guardrails)
- ❌ 不要改變 `engine.json` 的 JSON 結構或內容
- ❌ 不要移除 `ATLAS_ENGINE_CONFIG` env var 支援
- ❌ 不要移除 `/admin/reload-config` HTTP 端點
- ❌ 不要修改 `LoadEngineConfig()` 的核心邏輯（仍是 disk read + Validate）
- ❌ 不要更改 `GetEngineConfig()` 的 singleton 快取邏輯
- ❌ 不要在此 PR 中修復 race condition
- ❌ 不要移動任何其他 config 檔案（僅 engine.json）

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES（Go test framework + CI integration）
- **Automated tests**: Tests-after（現有測試驗證行為不變）
- **Framework**: Go testing（`go test`）

### QA Policy
Every task MUST include agent-executed QA scenarios. Evidence saved to `.sisyphus/evidence/`.

- **Backend**: Use Bash (`go test`, `go build`, `gofmt`) — Check exit code and assertion output
- **API**: Use Bash (curl) — Verify `/admin/reload-config` behavior unchanged

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately):
├── Task 1: 移動 engine.json 並重構 defaultEngineConfig() [quick]
│   - git mv + //go:embed + 簡化 defaultEngineConfig()
│   - Depends: none
│
└── (no parallel tasks — single-file refactor)

Wave 2 (After Wave 1):
└── Task 2: 更新測試並驗證 [quick]
    - 簡化 setProjectRoot()，更新路徑引用
    - 執行完整測試套件
    - 驗證 env var override 和 admin reload
    - Depends: Task 1
```

### Dependency Matrix

- **1**: - 2, 1
- **2**: 1 - -, 1

### Agent Dispatch Summary

- **1**: **1** — T1 → `quick`
- **2**: **1** — T2 → `quick`
- **FINAL**: **0** — （Quick 任務，不需要正式 Final Verification Wave，T2 的 QA 場景即是最終驗證）

---

## TODOs

- [ ] 1. 移動 engine.json 並重構 defaultEngineConfig()

  **What to do**:

  **Step A — 移動檔案**:
  ```bash
  git mv configs/engine.json internal/config/engine.json
  ```

  **Step B — 更新 configPath 預設值**（`internal/config/engine_config.go:163`）:
  ```go
  // Before:
  configPath = envOr("ATLAS_ENGINE_CONFIG", "configs/engine.json")
  // After:
  configPath = envOr("ATLAS_ENGINE_CONFIG", "internal/config/engine.json")
  ```

  **Step C — 加入 embed 並重構 defaultEngineConfig()**:
  1. Import 加入 `_ "embed"`（或 `"embed"`）
  2. 在 package-level 加入：
     ```go
     //go:embed engine.json
     var defaultEngineJSON []byte
     ```
  3. 將 `defaultEngineConfig()` 函式體替換為 embedded JSON 解析：
     ```go
     func defaultEngineConfig() *EngineConfig {
         var cfg EngineConfig
         if err := json.Unmarshal(defaultEngineJSON, &cfg); err != nil {
             // Embedded JSON should always be valid (verified at build time by tests).
             // If this panics, engine.json is corrupted in the source tree.
             panic("embedded engine.json is invalid: " + err.Error())
         }
         return &cfg
     }
     ```
  4. 刪除原有的 177 行硬編碼 struct literal（從 `return &EngineConfig{` 到結尾的 `}`）

  **Must NOT do**:
  - ❌ 不要修改 `engine.json` 的內容（純移動）
  - ❌ 不要修改 `LoadEngineConfig()`、`GetEngineConfig()`、`ReloadEngineConfig()` 的邏輯
  - ❌ 不要移除 `ATLAS_ENGINE_CONFIG` env var 支援
  - ❌ 不要移除 import 中已有的 `encoding/json`（仍需用於 defaultEngineConfig）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 單檔案機械性重構，邏輯簡單明確，不需深度分析
  - **Skills**: `[]`
    - 不需要特殊技能，純 Go 程式碼編輯

  **Parallelization**:
  - **Can Run In Parallel**: NO（只有一個任務）
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 2
  - **Blocked By**: None（可立即開始）

  **References**:

  **Pattern References**（現有程式碼結構）:
  - `internal/config/engine_config.go:166-188` — `LoadEngineConfig()` 的檔案讀取 + json.Unmarshal + Validate 模式（defaultEngineConfig 將使用相同模式但來源為 embedded bytes）
  - `internal/config/engine_config.go:245-422` — 目前 `defaultEngineConfig()` 的完整 struct literal（重構後將被刪除，此為參考比對用）

  **API/Type References**（實作的合約）:
  - `internal/config/engine_config.go:11-22` — `EngineConfig` struct 定義（json.Unmarshal 的目標型別）
  - `internal/config/engine_config.go:159-164` — `configPath` 變數宣告和 `envOr()` 用法

  **External References**（語言功能）:
  - Go 官方文件: `//go:embed` directive — 路徑相對於原始碼檔案目錄，不能含 `..`，支援 `string`、`[]byte`、`embed.FS` 三種型別

  **Acceptance Criteria**:

  - [ ] `git mv configs/engine.json internal/config/engine.json` 完成，`configs/engine.json` 不存在
  - [ ] `configPath` 預設值改為 `"internal/config/engine.json"`
  - [ ] `//go:embed engine.json` 宣告存在於 package-level
  - [ ] `defaultEngineConfig()` 使用 `json.Unmarshal(defaultEngineJSON, &cfg)`
  - [ ] 原有的硬編碼 struct literal 已完全刪除
  - [ ] `go build ./...` 編譯成功（0 errors）

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 編譯驗證 — defaultEngineConfig() 使用 embedded JSON 可成功建置
    Tool: Bash
    Preconditions: engine.json 已 moved，defaultEngineConfig() 已重構
    Steps:
      1. cd /Users/kaecer/workspace/atlas
      2. go build ./...
    Expected Result: exit code 0，無 compile errors
    Failure Indicators: 任何 compile error，特別是 "pattern engine.json: no matching files found" 或 "undefined: defaultEngineJSON"
    Evidence: .sisyphus/evidence/task-1-build.{txt}

  Scenario: 語法格式檢查
    Tool: Bash
    Preconditions: 程式碼已修改
    Steps:
      1. test -z "$(gofmt -l internal/config/engine_config.go)"
    Expected Result: exit code 0（無格式差異）
    Failure Indicators: exit code 1 且有檔案路徑輸出
    Evidence: .sisyphus/evidence/task-1-gofmt.{txt}
  ```

  **Evidence to Capture**:
  - [ ] `.sisyphus/evidence/task-1-build.{txt}` — build output
  - [ ] `.sisyphus/evidence/task-1-gofmt.{txt}` — format check output

  **Commit**: YES
  - Message: `refactor(config): embed engine.json as single source of truth`
  - Files: `internal/config/engine.json`, `internal/config/engine_config.go`
  - Pre-commit: `go build ./... && test -z "$(gofmt -l internal/config/engine_config.go)"`

- [ ] 2. 更新測試、驗證行為不變

  **What to do**:

  **Step A — 簡化 setProjectRoot()**:
  `engine.json` 現在在 `internal/config/` 下（和測試同目錄），不需要 chdir 到 project root。將 `setProjectRoot()` 簡化為：
  ```go
  func setProjectRoot(t *testing.T) {
      t.Helper()
      // engine.json is now in the same directory as the test file (internal/config/).
      // No need to chdir. If it's missing, the test should fail immediately.
      if _, err := os.Stat("engine.json"); err != nil {
          t.Fatalf("engine.json not found in package directory: %v", err)
      }
  }
  ```

  **Step B — 更新測試中的路徑引用**:
  `TestEngineConfigLoadValidate` 的錯誤訊息中包含 `"Check that configs/engine.json is valid."`，改為 `"Check that internal/config/engine.json is valid."`。

  **Step C — 執行完整測試套件**:
  - `go test ./internal/config/... -v`（確認所有測試 PASS）
  - `go test ./internal/config/... -run TestEngineConfigLoadValidate -v`（確認 LoadValidate 仍可讀取 disk 檔案）
  - `go test ./internal/config/... -run TestDefaultConfigIncludesLEOSatellite -v`（確認 leo_satellite 仍在 embedded 版本中）
  - `go test ./internal/config/... -run TestEngineJSONBaseAllocationsMatchDefault -v`（確認 embedded JSON = disk JSON）

  **Step D — 驗證 env var override**:
  ```bash
  ATLAS_ENGINE_CONFIG=internal/config/engine.json go test ./internal/config/... -run TestEngineConfigLoadValidate -v
  ```
  確認 `TestEngineConfigLoadValidate` PASS（env var 指向 disk 檔案仍可讀取）

  **Must NOT do**:
  - ❌ 不要刪除任何現有測試
  - ❌ 不要修改測試的核心驗證邏輯（只更新路徑和輔助函式）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 測試微調 + 執行驗證，邏輯簡單
  - **Skills**: `[]`
    - 不需要特殊技能

  **Parallelization**:
  - **Can Run In Parallel**: NO（依賴 Task 1）
  - **Parallel Group**: Wave 2
  - **Blocks**: None（最後一個任務）
  - **Blocked By**: Task 1

  **References**:

  **Pattern References**（現有測試結構）:
  - `internal/config/engine_config_test.go:85-109` — 目前 `setProjectRoot()` 含 chdir 邏輯（將被簡化）
  - `internal/config/engine_config_test.go:10-19` — `TestEngineConfigLoadValidate` 的錯誤訊息（路徑需更新）
  - `internal/config/engine_config_test.go:50-83` — `TestEngineJSONBaseAllocationsMatchDefault`（行為不變，只更新 setProjectRoot）

  **Acceptance Criteria**:

  - [ ] `setProjectRoot()` 簡化為同目錄檢查（不再 chdir）
  - [ ] 測試中的 `"configs/engine.json"` 字串更新為 `"internal/config/engine.json"` 或簡化
  - [ ] `go test ./internal/config/... -v` → 全部 PASS（含 4 個 engine_config 測試）
  - [ ] `ATLAS_ENGINE_CONFIG=internal/config/engine.json go test ./internal/config/... -run TestEngineConfigLoadValidate` → PASS
  - [ ] `gofmt -l .` → 無差異

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 完整 config 測試套件通過
    Tool: Bash
    Preconditions: Task 1 已完成（engine.json 已移動，defaultEngineConfig 已重構），測試已更新
    Steps:
      1. cd /Users/kaecer/workspace/atlas
      2. go test ./internal/config/... -v 2>&1
    Expected Result: exit code 0，所有測試 PASS
    Failure Indicators: 任何 FAIL 或 "build failed"
    Evidence: .sisyphus/evidence/task-2-test-all.{txt}

  Scenario: leo_satellite 回歸測試通過
    Tool: Bash
    Preconditions: 同上
    Steps:
      1. cd /Users/kaecer/workspace/atlas
      2. go test ./internal/config/... -run TestDefaultConfigIncludesLEOSatellite -v 2>&1
    Expected Result: exit code 0，輸出含 "PASS: TestDefaultConfigIncludesLEOSatellite"
    Failure Indicators: FAIL 或 "missing leo_satellite"
    Evidence: .sisyphus/evidence/task-2-test-leo.{txt}

  Scenario: base_allocations 同步測試通過（embedded JSON = disk JSON）
    Tool: Bash
    Preconditions: 同上
    Steps:
      1. cd /Users/kaecer/workspace/atlas
      2. go test ./internal/config/... -run TestEngineJSONBaseAllocationsMatchDefault -v 2>&1
    Expected Result: exit code 0，輸出含 "PASS: TestEngineJSONBaseAllocationsMatchDefault"
    Failure Indicators: FAIL 或 "mismatch"
    Evidence: .sisyphus/evidence/task-2-test-match.{txt}

  Scenario: env var override 仍可運作
    Tool: Bash
    Preconditions: 同上
    Steps:
      1. cd /Users/kaecer/workspace/atlas
      2. ATLAS_ENGINE_CONFIG=internal/config/engine.json go test ./internal/config/... -run TestEngineConfigLoadValidate -v 2>&1
    Expected Result: exit code 0，輸出含 "PASS: TestEngineConfigLoadValidate"
    Failure Indicators: FAIL 或 "failed to read engine config"
    Evidence: .sisyphus/evidence/task-2-test-env-override.{txt}

  Scenario: admin reload 端點行為不變
    Tool: Bash
    Preconditions: atlas server 已啟動（`go run ./cmd/atlas` on port 8080）
    Steps:
      1. curl -s -X POST http://localhost:8080/admin/reload-config 2>&1
      2. 檢查 HTTP status code
    Expected Result: HTTP 200（或重載成功訊息），不應出現 500 或 panic
    Failure Indicators: HTTP 500, connection refused, 或 response 含 "panic"/"fatal"
    Evidence: .sisyphus/evidence/task-2-reload.{txt}
  ```

  **Evidence to Capture**:
  - [ ] `.sisyphus/evidence/task-2-test-all.{txt}` — 完整測試輸出
  - [ ] `.sisyphus/evidence/task-2-test-leo.{txt}` — leo_satellite 測試輸出
  - [ ] `.sisyphus/evidence/task-2-test-match.{txt}` — base_allocations 同步測試輸出
  - [ ] `.sisyphus/evidence/task-2-test-env-override.{txt}` — env var override 測試輸出
  - [ ] `.sisyphus/evidence/task-2-reload.{txt}` — admin reload 端點輸出

  **Commit**: YES
  - Message: `test(config): simplify setProjectRoot after engine.json move`
  - Files: `internal/config/engine_config_test.go`
  - Pre-commit: `go test ./internal/config/... && test -z "$(gofmt -l .)"`

---

## Final Verification Wave

> Quick 規模任務不設正式 Final Verification Wave。T2 的 Agent QA 場景即涵蓋完整驗證：build → test → env var override → admin reload。

---

## Commit Strategy

- **T1**: `refactor(config): embed engine.json as single source of truth` — `internal/config/engine.json`, `internal/config/engine_config.go`
- **T2**: `test(config): simplify setProjectRoot after engine.json move` — `internal/config/engine_config_test.go`

---

## Success Criteria

### Verification Commands
```bash
# Build
go build ./...

# Config tests
go test ./internal/config/... -v

# Format
test -z "$(gofmt -l .)"

# Env var override still works
ATLAS_ENGINE_CONFIG=internal/config/engine.json go test ./internal/config/... -run TestEngineConfigLoadValidate -v

# Admin reload endpoint (需要啟動 server)
# curl -X POST http://localhost:8080/admin/reload-config → 200 OK
```

### Final Checklist
- [ ] `engine.json` 只有一份（在 `internal/config/` 下）
- [ ] `configs/engine.json` 已不存在
- [ ] `defaultEngineConfig()` 使用 embedded JSON
- [ ] 所有 config 測試 PASS
- [ ] `ATLAS_ENGINE_CONFIG` env var override 可運作
- [ ] `/admin/reload-config` 行為不變
- [ ] `gofmt -l .` 無差異
