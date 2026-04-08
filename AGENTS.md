# AGENTS.md - atlas-go

本檔是此儲存庫的 AI 開發代理工作守則。

## 建置與測試

預設使用以下指令（與 CI 對齊）：

```bash
# 格式檢查（CI 必跑）
test -z "$(gofmt -l .)"

# 格式修正
gofmt -w .

# 全域建置與測試
go build ./...
go test ./...

# 品質檢查
go vet ./...
staticcheck ./...

# 覆蓋率檢查
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1
```

常用的目標測試：

```bash
go test -v ./internal/sim -run TestRunBuildsPositions
go test ./internal/orchestrator/...
go test ./internal/portfolio/...
go test ./internal/prism/...
go test ./internal/reflexivity/...
go test ./internal/swarm/...
```

常用執行入口：

```bash
go run ./cmd/atlas
go run enhanced_experiment_runner.go
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
go run ./cmd/execute-experiment -brief <brief-file>
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

## 架構邊界

請把此專案視為分層式協調系統：

- 代理分層定義於 `internal/domain/types.go`（`context`、`sector`、`superinvestor`、`style`、`control`）。
- 核心流程為：market data -> orchestrator -> layer executors -> control filters (CRO/CIO) -> simulator -> ledger。
- 外掛介面位於 `internal/orchestrator/plugin_registry.go`：
  - `RegimeExecutor`：決定性（deterministic）regime 評分
  - `AgentExecutor`：產生投資建議
  - `ControlExecutor`：後置過濾與風控

關鍵元件邊界：

- `internal/orchestrator/`：流程協調與執行器路由
- `internal/sim/`：模擬引擎與部位狀態轉換
- `internal/portfolio/`：風險、波動度與 Darwinian 權重
- `internal/experiment/`、`internal/evolution/`、`internal/baseline/`：mutation/execute/judge/promote 生命週期
- `internal/ledger/`：結果記錄與分數卡讀取

## 專案慣例

遵循既有 Go 寫作模式：

- 介面保持小而聚焦（常見型式：`Supports(...)` + 一個操作方法）。
- 優先使用 early return，錯誤請包裝脈絡：`fmt.Errorf("context: %w", err)`。
- 既有領域狀態優先用字串 enum（如 `Regime`、`AgentLayer`）。
- 測試檔與原始碼同目錄，命名 `*_test.go`。
- import 分組順序：標準庫、外部套件、內部模組。

參考實作：

- Domain 型別：`internal/domain/types.go`
- Registry/介面：`internal/orchestrator/plugin_registry.go`
- 控制層過濾模式：`internal/orchestrator/plugin_control.go`
- 模擬約束與狀態更新：`internal/sim/engine.go`
- 風險/波動/權重邏輯：`internal/portfolio/*.go`

## 重要陷阱

調整行為前請先確認：

- Replay 視窗可能稀疏，評估實驗前先檢查資料可用性。
- `configs/agents.json` 每個啟用 agent 都應有有效 prompt 對應。
- Darwinian 權重會被夾在 `[0.3, 2.5]`；超界設定會被靜默正規化。
- 不要在多次 simulation run 之間重複使用可變 recommendation slice。
- 實驗執行與評估前必須先載入 baseline policy。
- Replay 匯入格式是 JSONL（每行一個 JSON 物件），不是 JSON 陣列。
- 控制層過濾是設計上在上游建議之後才套用。
- `internal/live/` 仍有部分 TODO 邊界；可靠路徑預設為 replay/simulation。

## 文件索引（連結優先，不重複內嵌）

進一步細節請直接參考：

- 架構總覽：`docs/architecture.md`
- AI 代理架構：`docs/ai-agent-architecture.md`
- 日常操作流程：`docs/operations-playbook.md`
- 迭代與 mutation 策略：`docs/iteration-playbook.md`
- 演化循環與接受門檻：`docs/evolution-loop.md`
- 資料來源與格式：`docs/data-sources.md`
- 腳本使用指南：`docs/SCRIPT_USAGE_GUIDE.md`
- 各階段實作說明：
  - `docs/phase2-implementation.md`
  - `docs/phase3-implementation.md`
  - `docs/phase4-implementation.md`
  - `docs/phase4-architecture.md`
  - `docs/phase5-architecture.md`
- OpenClaw 協定：
  - `docs/openclaw-protocol.md`
  - `docs/openclaw-protocol-v2.md`

## 不確定時

- 優先做小而精準的修改，避免大範圍重構。
- 先跑聚焦測試，再視範圍擴大到 `go test ./...`。
- 讓行為改動可透過現有 experiment/baseline 流程追溯與稽核。
