# AGENTS.md — atlas-go

> 語言：所有回覆、分析、程式碼註解使用**繁體中文**。
> 本文件為本專案 AI 開發代理的唯一權威規則。全局 `~/.claude/CLAUDE.md` 核心規則亦適用。

## 專案身分
- Go 模組 `github.com/kaecer68/atlas-go`，台股模擬投資研究系統。
- 原生 Go 工具鏈，無 Makefile。GitNexus 索引：`atlas-go`。

## 高危陷阱（靜默錯誤，務必遵守）
- 跨 simulation run 必須重建或深拷貝 `[]Recommendation`。
- `Save()` 寫入 `parameters.json` 時，務必同步更新 raw JSON 欄位（如 `calibration_timestamp`）。
- 所有 struct JSON tag 必須為 snake_case，API parsing struct 必須對齊。
- 修改 `ScreeningCriteria` 門檻前，務必執行 `go test ./internal/screener/...`。
- Darwinian 權重範圍 `[0.3, 2.5]`，超界值會靜默正規化，不可假設原值傳播。
- **RecordedAt 不可作為排序依據**，排序應以 `SessionID` 中的交易日為準。
- **GuardOutcomes 會覆寫 Agent ID**，導致 `PassedGuards` 全部變為 false，操作時務必保留原始 ID。

## 架構禁令（CI 強制）
- optimizer 不得降級為線性加權。
- 背景任務一律經 `BackgroundTaskManager` 註冊啟動；手動長任務可用 `TaskExec`，兩者可共存。
- 業務邏輯參數一律取自 `ParametersConfig`（含 Value, Rationale, Source, Todo），禁止硬編碼 magic number。
- 外部資料必須透過 `marketdata.Provider`，禁止直接建立 HTTP client。
- domain 型別留在 `internal/domain`，協調邏輯留在 `internal/orchestrator`。
- 禁止全域可變狀態進行執行期協調。
- 本機測試禁止啟用 `-allow-live-broker` 或 `-allow-real-signor`。
- `FactorType` 增刪改名需同步 7 個位置（CI 強制檢查）。
- 新增 `internal/` 模組需含 `doc.go` 並更新 `internal/MATURITY.md`。
- 每個 enabled agent 在 `configs/agents.json` 必須有對應 prompt 檔案（CI 檢查）。

## GitNexus 使用原則（條件觸發，非每次必做）
- **僅在以下情況執行 impact 分析**：變更超過 3 個符號、跨模組重構、或涉及執行流程變更。
- 一般小修改（單一函數內部調整、新增輔助函數）無需 blast radius。
- 提交前僅在不確定影響面或大型重構後才執行 `gitnexus_detect_changes()`。
- 重新命名符號一律使用 `gitnexus_rename`，禁止 find-and-replace。
- 需要理解符號全貌時，優先 `gitnexus_context`，其次 `codegraph trace`。

## 規範衝突
以 `internal/apigateway/CONSTITUTION.md` 為最終仲裁。

## 完整開發手冊
需要架構細節、所有執行指令、完整陷阱列表與資源導航時，請查閱 `docs/DEVELOPMENT.md`（或 `docs/AGENTS.md`，視你搬移後的檔名）。一般任務無需載入。
