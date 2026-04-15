# AGENTS.md — atlas-go

本檔是此儲存庫的 AI 開發代理工作守則。閱讀者應假設對本專案一無所知，所有資訊均以實際程式碼與設定為準，不做臆測。

---

## 專案概覽

`atlas-go`（模組名稱 `github.com/kaecer68/atlas-go`）是一套**模擬優先、稽核導向**的台股投資研究系統。無 Makefile，全部使用原生 Go 工具鏈與 shell script。

- **語言**：Go 1.25.0
- **主要依賴**：`golang.org/x/time`、`golang.org/x/text`、`github.com/redis/go-redis/v9`、`github.com/alicebob/miniredis/v2`
- **資料庫**：PostgreSQL 15（持久化）、Redis 7（快取 / nonce store）
- **CI 工具**：`gofmt`、`go vet`、`staticcheck`、`golangci-lint`、`gosec`

---

## CI 對齊指令（修改後必跑）

```bash
# 格式檢查（失敗會擋 PR）
test -z "$(gofmt -l .)"

# 建置與測試
go build ./...
go test ./...

# 品質檢查
go vet ./...
staticcheck ./...

# 覆蓋率（門檻 40%）
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

---

## 常用執行入口

```bash
# 主程式（HTTP server，預設 port 8080，含 /health）
go run ./cmd/atlas

# 實驗生命週期
go run ./cmd/execute-experiment -brief <file>
go run ./cmd/judge-experiment              # auto-discovers latest
go run ./cmd/promote-baseline              # auto-discovers latest accepted
go run ./cmd/revert-baseline --list

# 回測
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27

# 資料匯入（CSV → JSONL）
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

`cmd/experimental/` 下另有 11 個驗證/演練子命令（如 `janus-backtest`、`validate-broker`、`staging-drill` 等）。

---

## 核心架構

| 目錄 | 職責 |
|------|------|
| `internal/domain/` | 領域型別（`Regime`、`Recommendation`、`Position` 等字串 enum） |
| `internal/orchestrator/` | 流程協調（`SystemCore`、`PluginHost`、多層 executor 路由） |
| `internal/sim/` | 模擬引擎與部位狀態轉換 |
| `internal/experiment/` | 實驗執行（`Executor`）與評判（`Judge`） |
| `internal/baseline/` | Baseline policy 升降級與版本控制 |
| `internal/ledger/` | JSONL append-only 持久化 |
| `internal/portfolio/` | Darwinian 權重管理（限制 `[0.3, 2.5]`） |
| `internal/marketdata/` | 資料提供者抽象（TWSE OpenAPI、Fugle、Hybrid） |
| `internal/live/` | **仍有 TODO 邊界**；預設安全路徑為 replay/simulation |
| `internal/prism/` | Regime-specific 訓練佇列（5 種 regime） |
| `internal/swarm/` | MiroFish swarm 模擬 |
| `internal/janus/` | 跨 cohort regime 偵測與 PRISM 權重動態調整 |
| `internal/narrative/` | 巨集觀敘事事件偵測、因果鏈、台灣壓力指數 |

**分層資料流**：
`Market Data → Orchestrator (context/sector/style/superinvestor → control) → Simulator → Ledger`

---

## 程式碼慣例

- **介面風格**：小而聚焦，常見為 `Supports(...) bool` + 一個操作方法（參考 `internal/orchestrator/plugin.go`）。
- **Early return**：優先使用，減少巢狀縮排。
- **錯誤包裝**：一律 `fmt.Errorf("context: %w", err)`。
- **Import 順序**：標準庫 → 外部套件 → `github.com/kaecer68/atlas-go/...`。
- **測試檔**：與原始碼同目錄同 package，`*_test.go` 命名。
- **領域 enum**：維持字串型別（方便 JSON roundtrip）。
- **禁止**：引入全域可變狀態做執行期協調；跨層洩漏（domain 型別留在 `internal/domain`，協調邏輯留在 `internal/orchestrator`）。

---

## 測試須知

- **整合測試**：CI 使用 `go test -v -tags=integration ./...`，但**目前 repo 內沒有任何 `//go:build integration` 標籤**；根目錄的 `integration_test.go` 屬於 `package main`，會隨 `go test ./...` 常規執行。
- **Race detector**：`ci-cd.yml` 對 unit test 啟用 `-race`。
- **Coverage 門檻**：總覆蓋率不得低於 **40%**。
- **治理與操作 gate**：
  ```bash
  bash ./scripts/openclaw/verify-governance-gates.sh --require-scenario-diversity
  bash ./scripts/openclaw/verify-operations-gate.sh
  ```

---

## 設定檔慣例

- `configs/agents.json`（及 `agents.yaml`）定義代理註冊表。**每個 `enabled: true` 的 agent 必須在 `prompts/agents/` 下有對應 prompt 檔案**。
- `configs/portfolio-allocation.v23.json` 為投組配置版本檔案。
- `internal/config/config.go` 會自動讀取根目錄 `.env`，**不會覆蓋已存在的環境變數**；`.env` 中的值若帶引號（單雙引號）會被自動去除。
- 關鍵環境變數前綴為 `ATLAS_*`（如 `ATLAS_MARKET_DATA_PROVIDER`、`ATLAS_REPLAY_DATA_PATH`、`ATLAS_BASELINE_POLICY_PATH`、`ATLAS_BROKER_MODE`）。

---

## 高危陷阱

調整行為前請先確認：

| 陷阱 | 說明與預防 |
|------|-----------|
| **Enabled agent 缺少 prompt** | `configs/agents.json` 中每個 `enabled: true` 都需對應 `prompts/agents/<name>.md`。 |
| **Darwinian 權重靜默夾制** | 權重限制在 `[0.3, 2.5]`，超界會靜默正規化，不報錯。 |
| **重複使用 mutable `[]Recommendation`** | 多次 simulation run 之間不可共用同一個 slice。 |
| **Baseline 未載入** | 實驗執行/評估前必須確認 `data/state/baseline_policy.json` 存在且有效。 |
| **Replay 格式錯誤** | Replay 為 **JSONL**（每行獨立 JSON 物件），不是 JSON array。 |
| **Session 日期不可信賴 `RecordedAt`** | `RecordedAt` 是計算完成時間。排序/比較請以 `SessionID` 中的交易日為準（如 `session-20260413-daily` → `2026-04-13`）。 |
| **GuardOutcomes 與 outcomes 必須對齊** | 控制層（CIO）輸出應**保留原始 Agent ID**，不可覆寫為自己的 ID，否則 `PassedGuards` 會全部變 `false`。 |
| **OutcomeCount 必須是單場次數量** | `RecordSessionSummary` 絕對不可用 `ledger.LoadOutcomes()`（讀取全域檔案）來填 `OutcomeCount`。 |
| **同一件事不可有三種算法** | 放行/過濾筆數必須由單一權威來源（如 `GuardOutcomes`）計算，前端不可各自重算。 |
| **Live 交易風險** | `cmd/atlas` 有 `-allow-live-broker`、`-allow-real-signer` 等旗標，本地測試時切勿意外啟用。 |

---

## 人工覆寫機制（Human-in-the-Loop）

投資管線頁面（`web/static/index.html`）提供三種按鈕：

- **放行** (`approve_rec`)：後續執行時確保該推薦不被控制層濾除。
- **否決** (`reject_rec`)：後續執行時強制排除該 `(symbol, agent_id)` 組合。
- **補追**：語義同放行，但僅針對已被控制層擋下（`passed_guards=false`）的項目。

所有人工干預均持久化至 `data/state/approvals/`，作為可稽核軌跡。

---

## 已知議題

- `.github/workflows/daily-maintenance.yml` 硬編碼 `GO_VERSION: '1.21'`，與 `go.mod` 的 `1.25.0` 不一致。若修改此 workflow，應改為 `go-version-file: go.mod`。

---

## 延伸指令檔

以下檔案依任務領域提供額外守則：

- `.github/instructions/go-core.instructions.md` — Go 編碼規則
- `.github/instructions/experiments-guardrails.instructions.md` — 實驗安全守則
- `.github/instructions/live-trading.guardrails.instructions.md` — Live trading 邊界
- `.github/copilot-instructions.md` — 綜合入口與常見工作流程

進一步架構與操作細節請參考 `docs/` 目錄（繁體中文為主）。
