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

**適用範圍**：`resp.Body.Close()`（HTTP body）、`rows.Close()`（`*sql.Rows`）、`stmt.Close()`（`*sql.Stmt`）、唯讀 `f.Close()`。

**不適用**（保留原始 `defer X.Close()`）：寫入路徑（`os.O_WRONLY` / `O_CREATE` / `O_APPEND`）— Close error 可能代表資料未完整寫入磁碟。

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

> 完整 Go 反模式（mutable slice / Session 日期 / Darwinian 權重 / JSON tag 大小寫）見根 `AGENTS.md` §「關鍵跨模組陷阱」與 `docs/REFERENCE/TRAPS.md`。

## 測試規則

- 新增或更新測試時，使用同目錄同 package 的 `*_test.go`。
- 優先跑聚焦套件測試，再視需要擴大測試範圍。
- **覆蓋率門檻 60%**（CI 強制）：`go test -coverprofile=coverage.out ./...` 後執行 `go tool cover -func=coverage.out | grep total` 確認。

## 程式碼生成

修改 `internal/` 下任何 Go struct 的 JSON tag 後，**必須執行 `go generate .`**（CI `generate` job 強制）。`cmd/gentags` 會自動從 struct JSON tag 產出 `field_types.ts` 與 `valid_fields.json`，輸出到活躍前端目錄（`admin_web/`、`client_web/`、`shared_web/`）。

> **禁止手動編輯**任何一份 `field_types.ts` 或 `valid_fields.json` — 下次 `go generate .` 會覆寫。完整規範見 `docs/REFERENCE/TRAPS.md` §「手動編輯 field_types.ts」。

## 驗證清單

```bash
# 格式檢查（CI 強制）
test -z "$(gofmt -l .)"

# 聚焦測試（依變更套件調整）
go test ./internal/orchestrator/...
go test ./internal/sim/...

# 若影響範圍較大
go test ./...

# CI 完整品質檢查（建議 PR 前執行）
go vet ./...
staticcheck ./...
golangci-lint run --timeout=5m

# 覆蓋率檢查
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total   # 須 ≥ 60%
```

## 參考文件

- `AGENTS.md`：倉庫層級邊界、21 模組路由、關鍵跨模組陷阱
- `docs/REFERENCE/TRAPS.md`：高危陷阱完整參考（mutable slice、Session 日期、Darwinian 權重、JSON tag 大小寫等）
- `docs/QUICKSTART.md`：快速啟動 + CI 完整指令 + 系統初始化順序
- `internal/apigateway/CONSTITUTION.md`：數據源治理規範 — 6 條憲法，禁止直接 `os.Getenv`/`&http.Client{}`，強制 Gateway 模式
- `docs/REFERENCE/PARAMETER_SYSTEM.md`：參數管理系統 — 禁止硬編碼 magic number，強制使用 `ParametersConfig`
- `docs/architecture.md`：分層設計原則
