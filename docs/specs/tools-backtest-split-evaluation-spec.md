# `tools_backtest.go` 拆分評估 — Round 3b 結論

> **狀態**: v1.0 結論文件 (2026-07-16)
> **目的**: 評估是否將 `cmd/atlas-mcp/server/tools_parameters.go` 中的 backtest 2 tools 拆出獨立檔案
> **目標讀者**: atlas-mcp maintainers + 下次 wave 規劃者
> **結果**: 🟢 **推薦拆**

---

## 1. 當前狀態盤點

`cmd/atlas-mcp/server/tools_parameters.go` 180 行,含 **9 個 tools 由 1 個 `registerParametersBacktestTools()` 統一註冊**:

| Domain | Tool 數 | Handler | Type | Backend endpoint |
|--------|---------|---------|------|------------------|
| **Parameters** | 7 | `handleParametersGet` / `GetCategories` / `GetAuditLog` / `GetMetadata` / `GetSnapshots` | `ParametersGet*` | `/api/parameters/...` |
| **Backtest** | 2 | `handleBacktestStatus` / `handleBacktestSignals` | `Backtest*` | `/api/backtest/status`, `/api/backtest/signals` |

Backend 部分各自獨立:
- `internal/parameters/` — parameters 模組
- `internal/backtest/backtest_pipeline.go` (5551 bytes) — backtest pipeline 模組,**已有獨立 module**

`registerParametersBacktestTools()`(line 138) 內含兩個 countedAddTool 區塊,先 Parameters 再 Backtest。

---

## 2. 拆分優缺點

### 🟢 拆的優點

| 面向 | 拆分後 |
|------|--------|
| 領域一致性 | backtest 自成 `tools_backtest.go`,與 `internal/backtest/` backend 模組鏡像對齊 |
| 檔案大小 | tools_parameters.go 縮為 ~140 行(僅 parameters);tools_backtest.go ~50 行(僅 backtest) |
| 測試鏡像 | `tools_parameters_test.go` 與 `tools_backtest_test.go` 各自對應所屬 domain,測試更聚焦 |
| 未來擴充 | backtest 工具若增加(portfolio backtest、Sharpe 細項、custom window)有自然落點 |
| 認知負擔 | 讀 `tools_parameters.go` 只需關注 parameters domain,不被 backtest 干擾 |

### 🔴 不拆的優點

| 面向 | 不拆 |
|------|------|
| 變更範圍 | 維持原狀,零遷移成本 |
| 程式碼行數 | 180 行不算大,合併視為「一組 configuration 工具」可讀 |
| 測試斷裂 | 不需要重組 mock body |

### ⚖️ 取捨

- 變更成本低:純檔案搬移 + 改 1 個 `tools.go:59` registerTools() 呼叫
- 效益持久:backtest 與 parameters 在概念上是兩個 domain(parameters = config;backtest = scored output),後者未來預期會擴
- 不會破壞對外契約:tool 名稱、handler signature、Input/Output struct 都不變

---

## 3. 推薦方案: 拆

### 3.1 動作

1. 新增 `cmd/atlas-mcp/server/tools_backtest.go`(從 tools_parameters.go line 51-181 移出 Backtest 區段)
   - 內含: `BacktestStatusOutput`, `BacktestSignalsOutput`, `handleBacktestStatus`, `handleBacktestSignals`, `registerBacktestTools(mcpSrv, s)`
2. 修改 `cmd/atlas-mcp/server/tools_parameters.go`:
   - 移除 `Backtest*Output` 結構 + handlers
   - 移除 `registerBacktestTools` 部分(原 line 167-181)
   - 函數改名:`registerParametersBacktestTools` → `registerParametersTools`(僅註冊 7 個 parameters tool)
3. 修改 `cmd/atlas-mcp/server/tools.go:59`:
   - `registerParametersBacktestTools(mcpSrv, s)` → 分為兩行:
     ```
     registerParametersTools(mcpSrv, s)
     registerBacktestTools(mcpSrv, s)
     ```
4. 同步檔名 `tools_parameters_test.go` → 拆為 `tools_parameters_test.go` + `tools_backtest_test.go`
5. `go generate ./cmd/atlas-mcp/...` 重新生成 `auto-desc.gen.json`(雖然 desc 內容不變,但 hex byte-array 會重 marshal,產生 noise diff)

### 3.2 影響半徑審查

- ✅ Tool 名稱不變(`backtest_status`, `backtest_signals`)— 消費者(client_web / agent / doc)不受影響
- ✅ Input/Output struct 名稱不變
- ✅ Backend endpoint 路徑不變(`/api/backtest/*`)
- ⚠️ `RegisteredToolCount` 不變(還是 110)
- ⚠️ `auto-desc.gen.json` 會有 byte-level noise diff(desc 內容不變,但 hex layout 可能重排)
- ⚠️ Worktree 內 commit 需要先跑 `gofmt -l` + `go test ./cmd/atlas-mcp/server/` 確保測試通過

### 3.3 前置檢查

- `git grep "tools_parameters"` 確認沒有跨檔 import(因為同 package,理論上無)
- `git grep "registerParametersBacktestTools"` 確認所有呼叫都在 `tools.go:59`
- 檢查 `tools_parameters_test.go` 是否使用 BacktestStatusOutput / BacktestSignalsOutput(若有用,test 也要跟著拆)

---

## 4. 不拆的條件

若下次 sprint 出現以下情況,重新評估:

- backtest 工具需要交叉讀 parameters 結構(目前兩 domain 零耦合)
- 部署打包時要 atomic unit 一起 build(目前同 `tools_parameters.go` 共一個 package,無差異)
- 性能熱點顯示兩個 register 函式需要分開 go-routine(目前都是同步)

---

## 5. 時程建議

| 階段 | 動作 | 工作量 |
|------|------|--------|
| 預備 | 跑 `git grep` 確認無外部相依 | 5 min |
| 搬移 | 新增 `tools_backtest.go`,從 `tools_parameters.go` 移除 | 20 min |
| 改名 | `registerParametersBacktestTools` → `registerParametersTools`,`tools.go:59` 拆兩行 | 5 min |
| 測試 | 拆 `tools_parameters_test.go` 為 `_parameters` + `_backtest` | 15 min |
| 驗證 | `gofmt` + `go vet` + `go test ./cmd/atlas-mcp/server/` | 10 min |
| 生成 | `go generate` + `git diff auto-desc.gen.json` 確認 noise only | 5 min |
| 提交 | 1 commit + push + PR + CI watch | 10 min |

**總計**: ~1 小時,屬「低風險、低 reward、低風險」的清理工作。建議排在下一個 sprint 開頭或作為 onboarding 任務。

---

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-16 | 初版結論文件(由 Round 3b 評估產出) |