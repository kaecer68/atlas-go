---
applyTo: "internal/**/*.go,cmd/**/*.go"
description: "適用於 atlas-go 的 Go 程式碼修改。涵蓋 import 分組、錯誤包裝、介面邊界與 internal/cmd 套件的測試優先驗證。"
---

# Go 核心守則

## 適用範圍

修改 internal/ 與 cmd/ 下的 Go 程式碼時套用。

## 程式撰寫規則

- 介面保持小而聚焦，優先沿用既有 executor 風格：`Supports` 加上一個操作方法。
- 優先使用 early-return 流程以降低巢狀層級。
- 回傳錯誤請補上操作脈絡，使用 `fmt.Errorf("operation: %w", err)`。
- import 分組維持：標準庫、第三方、內部模組。
- 既有 enum 與領域狀態維持字串型別。

## 安全規則

- 除非任務明確包含 migration，否則維持既有公開 API 簽名。
- 避免跨層洩漏：domain 型別留在 internal/domain；協調邏輯留在 internal/orchestrator。
- 不要引入全域可變狀態做執行期協調。

## 唯讀 Close 錯誤處理

關於 `defer X.Close()` 的 errcheck 規範：

### 既有程式碼（`_ =` 模式）

唯讀檔案（`os.OpenFile` 不含寫入旗標）或 cleanup-only 的 `Close()`，直接採用 `_ =` 模式：

```go
// 唯讀：errcheck 要求處理 Close error，但實際風險僅為 fd 延遲釋放
defer func() { _ = f.Close() }()
```

**適用範圍**：
- `resp.Body.Close()` — HTTP response body（always safe）
- `rows.Close()` — `*sql.Rows`（return `error`，但實際僅釋放連線）
- `stmt.Close()` — `*sql.Stmt`
- 唯讀 `f.Close()` — 以 `os.Open` 或 `os.OpenFile` 不含寫入旗標開啟的檔案

**不適用**（保留原始 `defer X.Close()`）：
- 寫入路徑（`os.O_WRONLY` / `os.O_CREATE` / `os.O_APPEND`）— Close error 可能代表資料未完整寫入磁碟
- `pgx.Rows.Close()` — 不回傳 error，errcheck 不會檢查

### 新程式碼（closure + logging）

新撰寫的 `defer Close()` 應使用 closure 搭配專案 `logging` 套件：

```go
defer func() {
    if err := f.Close(); err != nil {
        logging.Warn("io", "close_failed",
            logging.FStr("path", path),
            logging.Err(err))
    }
}()
```

## 常見 Go 反模式（atlas-go 特有）

以下為此 codebase 高頻踩踏區。修改或新增 Go 程式碼時請檢查：

1. **mutable `[]domain.Recommendation` slice 共享** — 多次 simulation run、executor 之間不可共用同一個 slice。每次需要時請 `make` 新的並 `copy` 資料，否則會導致資料競爭。

2. **Session 日期從 `RecordedAt` 推斷** — `RecordedAt` 是計算完成時間，不是交易日。排序/比較請從 `SessionID` 提取（格式 `session-YYYYMMDD-daily` → `2006-01-02`）。

3. **Darwinian 權重超界靜默夾制** — 權重範圍 `[0.3, 2.5]`。超界不報錯，而是靜態正規化。撰寫權重邏輯時請驗證邊界。

4. **JSON tag 大小寫不一致** — Go struct 的 JSON tag 一律用 snake_case（如 `factor_scores`）。若 API parsing struct 用了 PascalCase（如 `FactorScores`），unmarshal 會無聲失敗，該欄位永遠為零值。變更 JSON tag 後 `go generate .` 會自動同步前端型別。

## 測試規則

- 新增或更新測試時，使用同目錄同 package 的 *_test.go。
- 優先跑聚焦套件測試，再視需要擴大測試範圍。

## 驗證清單

```bash
# 格式檢查
test -z "$(gofmt -l .)"

# 聚焦測試（依變更套件調整）
go test ./internal/orchestrator/...
go test ./internal/sim/...
```

若影響範圍較大：

```bash
go test ./...
```

## 參考文件

- `agents.md`：倉庫層級邊界與預設指令
- `internal/apigateway/CONSTITUTION.md`：數據源治理規範 — 禁止直接 `os.Getenv`/`&http.Client{}`，強制 Gateway 模式
- `docs/PARAMETER_SYSTEM.md`：參數管理系統 — 禁止硬編碼 magic number，強制使用 `ParametersConfig`
- `docs/architecture.md`：分層設計原則
- `docs/ai_agent_architecture.md`：代理協調細節
