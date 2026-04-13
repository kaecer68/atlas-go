# AGENTS.md - atlas-go

本檔是此儲存庫的 AI 開發代理工作守則。閱讀者應假設對本專案一無所知，所有資訊均以實際程式碼與設定為準，不做臆測。

---

## 專案概覽

`atlas-go` 是一套**模擬優先（simulation-first）、稽核導向**的台股投資研究系統。核心目標是讓多層 AI 代理在歷史資料上進行回測實驗，評估 agent 績效，並透過 mutation / judge / promote / revert 的閉環迭代策略規則與 prompt，整個過程不下真實單。

系統設計原則：
- Simulation before execution
- Audit trail over intuition
- Structured messages between layers
- Replaceable data providers
- Risk controls at the engine layer, not only in prompts

---

## 技術棧

| 層級 | 技術 |
|------|------|
| 語言 | Go 1.25 |
| 主要依賴 | `golang.org/x/time`、`golang.org/x/text`、`github.com/redis/go-redis/v9` |
| 容器化 | Docker（multi-stage build）、Docker Compose |
| 資料庫 | PostgreSQL 15（持久化）、Redis 7（快取 / nonce store / queue） |
| 監控 | Prometheus + Grafana |
| CI/CD | GitHub Actions（`ci.yml`、`quality.yml`、`ci-cd.yml`、`release.yml`、`daily-maintenance.yml`） |
| 靜態分析 | `gofmt`、`go vet`、`staticcheck`、`golangci-lint`、`gosec` |

**無 Makefile**；所有建置與測試均透過原生 Go 工具鏈與 shell script 完成。

---

## 建置與測試

### CI 對齊的必跑指令

```bash
# 格式檢查（CI 必跑，失敗會擋 PR）
test -z "$(gofmt -l .)"

# 格式修正
gofmt -w .

# 全域建置與測試
go build ./...
go test ./...

# 品質檢查
go vet ./...
staticcheck ./...

# 覆蓋率檢查（CI 門檻為 40%）
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1
```

### 常用的目標測試

```bash
go test -v ./internal/sim -run TestRunBuildsPositions
go test ./internal/orchestrator/...
go test ./internal/portfolio/...
go test ./internal/prism/...
go test ./internal/reflexivity/...
go test ./internal/swarm/...
```

### 常用的執行入口（`cmd/` 說明見下文）

```bash
# 主應用程式（HTTP dashboard + API）
go run ./cmd/atlas

# 回測指定日期區間
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27

# 實驗生命週期
go run ./cmd/execute-experiment -brief <brief-file>
go run ./cmd/judge-experiment              # 自動尋找最新實驗結果
# 或
# go run ./cmd/judge-experiment -result <experiment-result-file>

go run ./cmd/promote-baseline              # 自動尋找最新實驗結果
# 或
# go run ./cmd/promote-baseline -result <accepted-result-file>
go run ./cmd/revert-baseline --list

# 資料匯入（CSV → JSONL）
go run ./cmd/import-replay -source <csv> -target <jsonl>

# 市場資料與監控測試
go run ./cmd/experimental/test-fugle
go run ./cmd/experimental/test-hybrid
go run ./cmd/experimental/test-monitor
```

---

## 專案結構與架構

### 目錄結構

```text
.
├── cmd/                    # 11 個 CLI 入口點（見下表）
├── internal/               # 核心系統套件（26 個子套件）
├── configs/                # 設定檔：agents.json / agents.yaml / monitor-limits.json 等
├── prompts/                # Agent prompt 與實驗 prompt
├── data/                   # 運行期狀態與 replay 資料（state/、replay/）
├── docs/                   # 架構與操作文件（繁體中文為主）
├── scripts/                # 操作輔助腳本（OpenClaw、darwinian-adjust、swarm-manage 等）
├── monitoring/             # Prometheus 設定與 Grafana dashboard/datasource
├── integration_test.go     # 根目錄整合測試（Phase 2 / Phase 3 元件）
├── docker-compose.yml      # 完整本地堆疊（atlas + postgres + redis + prometheus + grafana + cron + prism-worker + swarm-runner）
├── Dockerfile              # 多階段建置主映像檔
├── Dockerfile.cron         # Cron job 映像檔
└── go.mod / go.sum         # Go 模組定義（module: github.com/kaecer68/atlas-go）
```

### `cmd/` 入口點一覽

| 指令 | 用途 |
|------|------|
| `cmd/atlas` | 主應用程式（HTTP server，預設 port 8080，含 `/health`） |
| `cmd/backtest-window` | 回測指定日期區間 |
| `cmd/execute-experiment` | 執行 mutation 實驗 |
| `cmd/judge-experiment` | 評估實驗結果（對比 baseline） |
| `cmd/promote-baseline` | 晉升通過的實驗為新的 baseline policy |
| `cmd/revert-baseline` | 回滾 baseline 到指定版本或實驗前狀態 |
| `cmd/import-replay` | 將 TWSE/TPEX CSV 轉為 JSONL replay 格式 |
| `cmd/experimental/test-fugle` | 測試 Fugle API 連線（支援 `--help`） |
| `cmd/experimental/test-hybrid` | 測試 Hybrid Provider（Fugle + TWSE 備援，支援 `--help`） |
| `cmd/experimental/test-monitor` | 測試監控系統與 live 協調模式（支援 `--help`） |

### `internal/` 核心套件

| 套件 | 職責 |
|------|------|
| `internal/domain/` | 領域型別（`Regime`、`AgentLayer`、`Quote`、`Recommendation`、`Position`、`Order`、`SimulationConstraints` 等） |
| `internal/orchestrator/` | 流程協調與執行器路由（`PluginHost`、`Plugin`、`RegimeExecutor`、`AgentExecutor`、`ControlExecutor`） |
| `internal/sim/` | 模擬引擎與部位狀態轉換 |
| `internal/portfolio/` | 風險、波動度與 Darwinian 權重管理 |
| `internal/marketdata/` | 市場資料提供者抽象與 adapter（TWSE、Fugle、Hybrid、Yahoo） |
| `internal/experiment/` | 實驗執行（`Executor`）與評判（`Judge`） |
| `internal/evolution/` | 演化循環（mutation 選擇、weakest agent 挑選） |
| `internal/baseline/` | Baseline policy 管理（promote / revert / version control） |
| `internal/ledger/` | 結果記錄與 scorecard 讀取 |
| `internal/backtest/` | 回測視窗執行器 |
| `internal/replay/` | Replay 資料載入（JSONL / CSV） |
| `internal/config/` | 執行期設定（讀取 `.env` 與環境變數） |
| `internal/live/` | 即時交易路徑（**仍有 TODO 邊界，預設使用 replay/simulation**） |
| `internal/monitoring/` | Dashboard API 與指標收集 |
| `internal/prism/` |  regime-specific 訓練佇列（5 種 regime） |
| `internal/swarm/` | MiroFish swarm 模擬 |
| `internal/spawning/` | Agent auto-spawning |
| `internal/reflexivity/` | Soros reflexivity 引擎 |
| `internal/adversarial/` | 紅藍隊對抗訓練 |
| `internal/metalearning/` | Meta-learning |
| `internal/globalmarket/` | 全球市場資料 |
| `internal/realtime/` | 即時資料處理 |
| `internal/importer/` | 資料匯入器 |

### 分層式資料流

```text
Market Data (configs/agents.json, replay/*.jsonl)
         ↓
Orchestrator (internal/orchestrator/)
    ├─ RegimeExecutor    (context layer: 決定性 regime 評分)
    ├─ AgentExecutor     (sector / style / superinvestor layers: 產生投資建議)
    └─ ControlExecutor   (control layer: CRO / CIO 後置過濾與風控)
         ↓
Simulator (internal/sim/)
    ├─ RunSymbol() 執行每檔標的
    ├─ ApplyRecommendations() + ApplyControl()
    └─ 產生 Position mutations
         ↓
Ledger (internal/ledger/)
    └─ 持久化 outcomes 供後續評判
```

代理分層定義於 `internal/domain/types.go`：
- `context`（總經 regime）
- `sector`（產業 desk）
- `style`（風格過濾）
- `superinvestor`（超級投資者層）
- `control`（CRO / CIO / 風控）

---

## 程式碼風格與慣例

### Go 寫作模式

- **介面保持小而聚焦**：常見型式為 `Supports(...)` + 一個操作方法。參考 `internal/orchestrator/plugin.go`（`Plugin` 生命週期介面）與 `internal/orchestrator/plugin_registry.go`（執行器介面）。
- **優先使用 early return**，減少巢狀縮排。
- **錯誤包裝脈絡**：一律使用 `fmt.Errorf("context: %w", err)`。
- **領域狀態優先用字串 enum**：如 `type Regime string`、`type AgentLayer string`，方便 JSON roundtrip。
- **Import 分組順序**：標準庫 → 外部套件 → 內部模組 (`github.com/kaecer68/atlas-go/...`)。
- **測試檔位置**：與原始碼同目錄，命名 `*_test.go`，package 通常與被測程式碼相同（偶有 `package xxx_test`）。

### 設定檔慣例

- `configs/agents.json` 與 `configs/agents.yaml` 並存，內容對應；**每個 `enabled: true` 的 agent 都必須在 `prompts/agents/` 下有對應的 prompt 檔案**。
- 環境變數優先於 `.env` 檔案。`internal/config/config.go` 會自動讀取專案根目錄的 `.env`，但已存在的環境變數不會被覆蓋；`.env` 中的值若使用引號（單引號或雙引號）會被自動去除。
- 關鍵環境變數前綴為 `ATLAS_*`，例如 `ATLAS_MARKET_DATA_PROVIDER`、`ATLAS_REPLAY_DATA_PATH`、`ATLAS_BASELINE_POLICY_PATH`。

---

## 測試策略

### 單元測試

- 分散在各 `internal/` 子套件中，命名 `*_test.go`。
- 執行：`go test ./...`

### 整合測試

- 根目錄 `integration_test.go` 驗證 Phase 2 / Phase 3 元件整合（Darwinian weights、superinvestor layer、spawning、PRISM、reflexivity、swarm）。
- 執行：`go test -v ./... -run Integration`
- CI 中的 `ci-cd.yml` 另有帶 `redis` + `postgres` service 的 integration job，需使用 `-tags=integration` 執行（雖然目前 `integration_test.go` 未使用 build tag）。

### 效能與基準測試

- `integration_test.go` 內含 `BenchmarkDarwinianAdjustment`，可用於權重調整計算基準。
- 執行：`go test -bench=. -benchmem ./...`

### CI 測試門檻

- `.github/workflows/ci.yml`：build + test + governance gate (`verify-governance-gates.sh`) + operations gate (`verify-operations-gate.sh`)。
- `.github/workflows/quality.yml`：fmt / vet / staticcheck / coverage（**總覆蓋率不得低於 40%**）/ license check。
- `.github/workflows/ci-cd.yml`：build + race detector (`-race`)、lint (`golangci-lint`)、security scan (`gosec`)、Docker image build/push。

### 治理與操作 Gate 腳本

```bash
# 治理 gate（含 replay 確定性、hard-guard 阻擋行為、trace 持久化檢查）
bash ./scripts/openclaw/verify-governance-gates.sh --require-scenario-diversity

# 操作 gate（含 runbook 覆蓋率、Prometheus 設定檢查、rollback drill、human-approval 事件驗證）
bash ./scripts/openclaw/verify-operations-gate.sh
```

---

## 部署與維運

### Docker 多階段建置

`Dockerfile` 使用 `golang:1.25-alpine` 建置，最終映像檔基於 `alpine:latest`，以非 root 使用者 `atlas`（uid 1000）執行。建置時注入版本資訊：

```dockerfile
-ldflags="-w -s -X main.version=$(git describe --tags --always) -X main.buildTime=$(date -u +%Y%m%d%H%M%S)"
```

### Docker Compose 服務

| 服務 | 說明 |
|------|------|
| `atlas` | 主應用程式，port 8080 |
| `redis` | 快取與訊息佇列，port 6379 |
| `postgres` | 持久化儲存，port 5432 |
| `prometheus` | 指標收集，port 9090 |
| `grafana` | 儀表板，port 3000 |
| `cron-darwinian` | 每日 09:00 執行 Darwinian 權重調整 |
| `prism-worker` | PRISM 訓練佇列處理器 |
| `swarm-runner` | MiroFish swarm 模擬執行器（profile: `swarm`） |

啟動完整環境：

```bash
docker-compose up -d
# 或僅啟動主服務
docker-compose up -d atlas redis postgres
```

### GitHub Actions CI/CD

- **Container Registry**：`ghcr.io`（GitHub Container Registry）
- **觸發條件**：`push` 到 `main` / `develop`、`pull_request` 到 `main`、`release` created
- **部署階段**：
  - `develop` 分支 → staging（目前為 placeholder）
  - `release` → production（目前為 placeholder）

### 常用維運腳本

| 腳本 | 用途 |
|------|------|
| `scripts/darwinian-adjust.sh` | 每日 Darwinian 權重調整（`--dry-run`、`--reset`） |
| `scripts/openclaw/run-validated-round.sh` | 一鍵執行完整實驗循環 |
| `scripts/openclaw/status.sh` | 報告系統當前狀態 |
| `scripts/openclaw/propose-mutation.sh` | 產生 mutation 建議 |
| `scripts/openclaw/execute-next.sh` | 執行下一個準備好的實驗 |
| `scripts/openclaw/judge-latest.sh` | 評判最新完成的實驗 |
| `scripts/openclaw/human-approval.sh` | Human-in-the-loop 決策包裝 |
| `scripts/swarm-manage.sh` | Swarm 模擬管理 |
| `scripts/prism-manage.sh` | PRISM 訓練佇列管理 |
| `scripts/spawning-manage.sh` | Agent spawning 管理 |
| `scripts/reflexivity-report.sh` | Reflexivity 分析報告 |

---

## 安全考量

- **容器以非 root 執行**：`atlas` user（uid 1000）。
- **Secret 管理**：API key（如 `FUGLE_API_KEY`、`ATLAS_BROKER_API_SECRET`）透過 `.env` 或 GitHub Secrets 注入，**不應寫死在程式碼中**。
- **Broker 模式**：預設為 `dry-run`（`ATLAS_BROKER_MODE=dry-run`）， live 交易需明確切換。
- **Security Scan**：CI 使用 `gosec` 掃描，結果以 SARIF 格式上傳至 GitHub Security tab。
- **Nonce Store**：支援 `memory`、`file`、`redis` 三種模式，live 交易時建議使用 Redis 避免重放攻擊。
- **Live 交易路徑**：`internal/live/` 仍有部分 TODO 邊界，修改時請格外謹慎，優先以 replay/simulation 驗證。

---

## 重要陷阱與禁忌

調整行為前請先確認：

| 陷阱 | 說明與預防 |
|------|-----------|
| **Replay 視窗稀疏** | 評估實驗前先檢查資料可用性（n≥10 較理想），否則 judge 可能因觀察值不足而失敗。 |
| **Enabled agent 缺少 prompt** | `configs/agents.json` 每個 `enabled: true` 的 agent 都必須在 `prompts/agents/` 有對應檔案。 |
| **Darwinian 權重靜默夾制** | 權重會被限制在 `[0.3, 2.5]`；超界設定會被靜默正規化，不會報錯。 |
| **重複使用可變 recommendation slice** | 不要在多次 simulation run 之間重複使用同一個 mutable `[]Recommendation`，避免狀態汙染。 |
| **未載入 baseline policy** | 實驗執行與評估前必須先確認 `data/state/baseline_policy.json` 存在且有效。 |
| **Replay 格式錯誤** | Replay 匯入格式是 **JSONL**（每行一個獨立 JSON 物件），不是 JSON 陣列。 |
| **Control 過濾順序** | 控制層過濾（CRO/CIO）是設計上在「上游產生建議之後」才套用，非前置條件。 |
| **Live 路徑未完成** | `internal/live/` 仍有部分 TODO 邊界；可靠路徑預設為 replay/simulation。 |

---

## 文件索引（連結優先，不重複內嵌）

進一步細節請直接參考：

- 架構總覽：`docs/architecture.md`
- AI 代理架構：`docs/ai-agent-architecture.md`
- 日常操作流程：`docs/operations-playbook.md`
- 迭代與 mutation 策略：`docs/iteration-playbook.md`
- 演化循環與接受門檻：`docs/evolution-loop.md`
- 資料來源與格式：`docs/data-sources.md`
- 腳本使用指南：`docs/SCRIPT_USAGE_GUIDE.md`
- OpenClaw 快速參考：`scripts/openclaw/QUICK_REFERENCE.md`
- GitHub Copilot 指令中心：`.github/copilot-instructions.md`
- 各階段實作說明：
  - `docs/phase2-implementation.md`
  - `docs/phase3-implementation.md`
  - `docs/phase4-implementation.md`
  - `docs/phase4-architecture.md`
  - `docs/phase5-architecture.md`
- OpenClaw 協定：
  - `docs/openclaw-protocol.md`
  - `docs/openclaw-protocol-v2.md`

---

## 不確定時

- 優先做小而精準的修改，避免大範圍重構。
- 先跑聚焦測試，再視範圍擴大到 `go test ./...`。
- 讓行為改動可透過現有 experiment / baseline 流程追溯與稽核。
