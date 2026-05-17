# agents.md — atlas-go

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
go run ./cmd/run-experiment -brief <file>
go run ./cmd/judge-experiment              # auto-discovers latest
go run ./cmd/promote-baseline              # auto-discovers latest accepted
go run ./cmd/revert-baseline --list

# 回測
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27

# 資料匯入（CSV → JSONL）
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

`cmd/experimental/` 下另有 10 個驗證/演練子命令目錄（含 9 個 CLI 與 `AGENTS.md`），如 `janus-backtest`、`validate-broker`、`staging-drill` 等。

---

## 核心架構

### 主要模組（核心資料流）

| 目錄 | 職責 |
|------|------|
| `internal/apigateway/` | 統一數據採集層：Gateway（14 通道管理、速率限制、熔斷器、快取）、BackgroundTaskManager（10+ 排程任務、重試策略、盤中保護） |
| `internal/domain/` | 領域型別（`Regime`、`Recommendation`、`Position` 等字串 enum） |
| `internal/orchestrator/` | 流程協調（`SystemCore`、`PluginHost`、多層 executor 路由） |
| `internal/sim/` | 模擬引擎與部位狀態轉換 |
| `internal/experiment/` | 實驗執行（`Executor`）與評判（`Judge`） |
| `internal/baseline/` | Baseline policy 升降級與版本控制 |
| `internal/ledger/` | JSONL append-only 持久化 |
| `internal/portfolio/` | Darwinian 權重管理（限制 `[0.3, 2.5]`）與 **FactorEngine**（動能/價值/品質多因子計算） |
| `internal/screener/` | 宣告式個股篩選（P/E、P/B、股息率、動能、成交量、總因子分數） |
| `internal/marketdata/` | 資料提供者抽象（TWSE OpenAPI、Fugle、Hybrid） |
| `internal/live/` | 已強化（context 統一、原子寫入、Dashboard 解耦），但 production live 仍需 `-allow-live-broker` 等旗標謹慎啟用 |
| `internal/prism/` | Regime-specific 訓練佇列（5 種 regime） |
| `internal/swarm/` | MiroFish swarm 模擬 |
| `internal/janus/` | 跨 cohort regime 偵測與 PRISM 權重動態調整 |
| `internal/narrative/` | 巨集觀敘事事件偵測、因果鏈、台灣壓力指數 |

### 輔助模組（支撐系統）

| 目錄 | 職責 |
|------|------|
| `internal/adversarial/` | 對抗性訓練（AdversarialTrainer、BattleResult、StressTest） |
| `internal/backtest/` | 視窗回測（Window.Run） |
| `internal/bootstrap/` | 系統初始化與儀表板路由註冊 |
| `internal/config/` | 環境變數讀取（ATLAS_* 前綴）、參數配置 |
| `internal/db/` | PostgreSQL 連接管理 |
| `internal/eventbus/` | 事件匯流排（ChannelEventBus、Publish/Subscribe） |
| `internal/evolution/` | 突變提案建構（BuildMutationBrief）、最弱代理選擇 |
| `internal/globalmarket/` | 全球總經資料管理 |
| `internal/importer/` | CSV → JSONL 資料匯入（TWSE、FinMind） |
| `internal/industry/` | 產業分析（行業輪動、供給需求分析） |
| `internal/logging/` | 統一日誌介面（Info/Error/Err） |
| `internal/monitoring/` | 監控 API 與 Dashboard（200 symbols，115 個 API handlers） |
| `internal/realtime/` | 即時資料轉接器（RealTimeAdapter） |
| `internal/reflexivity/` | 自反性價格動態引擎 |
| `internal/replay/` | TWSE CSV 載入與 forward return 計算 |
| `internal/reporting/` | 報告生成（Markdown、ASCII chart、Agent 績效表） |
| `internal/repository/` | PostgreSQL 持久化（DualWriteRepository） |
| `internal/risk/` | 風險管理（RiskManager、VaR、宏觀回撤） |
| `internal/spawning/` | Agent 生成管理（SpawningManager、PerformSpawningCycle） |
| `internal/strategy/` | 策略選擇器與登錄 |
| `internal/stress/` | 壓力測試場景（RunScenario） |
| `internal/taskexec/` | 非同步任務執行器（Manager、Cancel/Subscribe） |
| `internal/tax/` | 台灣稅務計算（TaiwanTaxCalculator） |
| `internal/metalearning/` | 元學習協調器（MetaLearner、策略選擇優化） |

**分層資料流**：
`Market Data → Gateway (internal/apigateway/: rate limit + circuit breaker + cache) → BackgroundTaskManager (scheduled fetch) → Orchestrator (context → screener → sector/style → control) → Simulator → Ledger`

> **注意**：`superinvestor` 不是獨立 executor，而是 sector/style agent 的角色型別（參見 `configs/agents.json` 中的 `layer: superinvestor`）。

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
  bash ./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
  bash ./scripts/openclaw/verify_operations_gate.sh
  ```

---

## 設定檔慣例

- `configs/agents.json`（及 `agents.yaml`）定義代理註冊表。**每個 `enabled: true` 的 agent 必須在 `prompts/agents/` 下有對應 prompt 檔案**。
- `configs/portfolio_allocation.json` 為投組配置版本檔案。
- `internal/config/config.go` 會自動讀取根目錄 `.env`，**不會覆蓋已存在的環境變數**；`.env` 中的值若帶引號（單雙引號）會被自動去除。
- 關鍵環境變數前綴為 `ATLAS_*`（如 `ATLAS_MARKET_DATA_PROVIDER`、`ATLAS_REPLAY_DATA_PATH`、`ATLAS_BASELINE_POLICY_PATH`、`ATLAS_BROKER_MODE`）。

---

## 參數管理慣例

參數系統是此專案核心基礎設施，所有 237+ 個參數必須統一納管於 `configs/parameters.json`，並遵循以下規範：

### 參數引用註解（Citation）

每個參數必須包含 `citation` 區塊，記錄其來源與有效性：

| 欄位 | 說明 | 可選值 |
|------|------|--------|
| `source_type` | 資料來源類別 | `literature` / `empirical` / `heuristic` / `calibrated` |
| `source_reference` | 具體來源描述（文獻 DOI、TWSE 公告、backtest 報告等） | 自由文字，避免泛泛描述 |
| `evidence_quality` | 證據品質 | `high` / `medium` / `low` |
| `update_policy` | 更新策略 | `auto`（可自動校準） / `manual`（需人工審查） / `frozen`（理論固定值） |
| `validation_method` | 驗證方式 | `backtest_optimization` / `empirical_calibration` / `literature_review` / `cross_reference` / `code_review` 等 |
| `dependencies` | 相依參數（建議填入） | 字串陣列 |
| `last_validated` | 最後驗證時間 | ISO 8601 時間戳 |

### AI 開發新參數流程

1. 在 `configs/parameters.json` 中新增參數與 `citation` 區塊
2. 在 `internal/config/parameters.go` 的對應 struct 中新增 `ParameterMetadata[T]` 欄位
3. 在 `internal/config/parameters_defaults.go` 中新增預設值
4. 在 `internal/config/inference.go` 的 `SetParameter`／`GetParameter` 中新增 case
5. 在 `internal/config/parameter_snapshot.go` 中新增 snapshot compare 調用
6. 驗證：執行 `go build ./...`、`go test ./internal/config/...` 與 `go run ./cmd/parameter-health-check`

### 參數驗證工具

- `go run ./cmd/parameter-health-check` — 輸出完整參數審計報告（citation 覆蓋率、證據品質分佈、問題清單）
- `go run ./cmd/calibrate-parameters --module=garch --dry-run` — GARCH/VaR 參數校準

### 完整參考文件

詳細參數系統設計與 schema 請參閱 `docs/PARAMETER_SYSTEM.md`。

---

## Git 工作流（強制）

### 分支策略

| 分支類型 | 命名規範 | 用途 |
|---------|---------|------|
| `main` | — | 僅接受 PR 合併，**禁止直接 push** |
| `feat/<name>` | `feat/apigateway-btm-migration` | 新功能開發 |
| `fix/<name>` | `fix/channel-adapter-race` | Bug 修復 |
| `refactor/<name>` | `refactor/bootstrap-cleanup` | 重構 |

### AI 執行流程（強制順序）

**絕對禁止直接 `git push origin main`**。無論任務多小，一律遵循：

```bash
# 1. 從最新 main 建立 feature branch
git checkout main
git pull origin main
git checkout -b feat/<descriptive-name>

# 2. 開發並提交
git add -A
git commit -m "feat(scope): description"

# 3. 推送 branch
git push -u origin feat/<descriptive-name>

# 4. 建立 PR（透過 gh CLI）
gh pr create --title "feat(scope): description" \
  --body "## Summary
- 變更內容
- 測試結果
- 風險評估" \
  --base main
```

### 提交前檢查清單

- [ ] 是否從 `main` checkout 新的 feature branch？
- [ ] 是否運行了 `go build ./...` 和 `go test ./...`？
- [ ] 是否運行了 `gofmt` 和 `staticcheck`？
- [ ] commit message 是否符合 `type(scope): description` 格式？
- [ ] 是否 push 到 `origin/<branch>` 而非 `origin/main`？

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
| **ScreeningCriteria 靜默過濾** | `configs/agents.json` 中若設定了 `screening_criteria`，標的在進入 sector/style executor **之前**就會被 `screener` 過濾。P/E、P/B 或成交量門檻過高可能導致某檔標的「完全沒有推薦」，這是預期行為，不是 bug。調整門檻前請先用 `go test ./internal/screener/...` 確認篩選邏輯。 |
| **JSON tag 大小寫錯誤** | API handler (`dashboard_api.go`) 讀取 JSONL 時，若 anonymous struct 的 JSON tag 用了 PascalCase（如 `json:"FactorScores"`）而 JSON 檔案實際寫入時是 snake_case（如 `factor_scores`），unmarshal 會靜默失敗，導致該欄位永遠為 nil/零值。所有 `domain.*` struct 的 JSON tag 均為 snake_case，API parsing struct 必須對齊。 |
| **前端欄位命名不一致 → 已自動解決** | 修改 `internal/domain/*.go` 中的 struct JSON tag 後，**git pre-commit hook 會自動執行 `go generate .`** 並將產生的 `field_names.js`、`field_types.ts`、`field_types.d.ts` 自動 staged。不需手動執行任何命令。AI 提交代碼時自動觸發，前端類型定義永遠與後端同步。 |
| **Go → 前端類型自動生成** | `cmd/gentags` 從 Go struct 的 JSON tag 自動生成前端類型定義（`field_names.js` + `field_types.ts` + `field_types.d.ts`，48 個 struct）。`go generate .` 由 pre-commit hook 自動觸發。CI（`quality.yml` 的 `generate` job）也會檢查。 |
| **Live 交易風險** | `cmd/atlas` 有 `-allow-live-broker`、`-allow-real-signor` 等旗標，本地測試時切勿意外啟用。 |

---

## 人工覆寫機制（Human-in-the-Loop）

投資管線頁面（`web/static/index.html`）提供三種按鈕：

- **放行** (`approve_rec`)：後續執行時確保該推薦不被控制層濾除。
- **否決** (`reject_rec`)：後續執行時強制排除該 `(symbol, agent_id)` 組合。
- **補追**：語義同放行，但僅針對已被控制層擋下（`passed_guards=false`）的項目。

所有人工干預均持久化至 `data/state/approvals/`，作為可稽核軌跡。

---

---

## 決策鏈透明化（Audit Trail）

系統已實作三階段透明度機制，將後端決策鏈的完整計算過程攤開在「決策鏈」前端頁面：

### 第一階段：個股因子分數透明化
- `FactorScores`（含 `Breakdown *FactorScoreBreakdown`）附加於每筆 `Recommendation` 與 `ScreeningReject`
- 每因子含：`Score`（計算結果）、`Weight`（權重）、`Formula`（計算公式）、`RawInputs`（原始輸入）、`IsFallback`（是否為 fallback 猜測）
- 實作：`internal/portfolio/factor_engine.go` 的 `CalculateAllScoresWithBreakdown()`
- 觸發時機：`collectRecommendations()`（`internal/orchestrator/executors.go`）對所有 recs 與 rejects 都呼叫計算

### 第二階段：行業信念計算透明化
- `ConvictionBreakdown`（含 `Base`/`Floor`/`Final` 與 `Steps[]`）附加於每筆 `Recommendation`
- 每步含：`Rule`（規則名）、`Delta`（增減分）、`Reason`（原因說明）
- 實作：`internal/orchestrator/conviction_builder.go` 的 `convictionBuilder`，由各 Sector/Style Executor 的 `Recommend()` 方法呼叫
- 已重寫：Semiconductor、AI Supply Chain、ETF Rotation、Financials、Shipping、ValueYield、EarningsQuality、TechnicalBreakout、GrowthMomentum 等 Executor

### 第三階段：宏觀事件信心度透明化
- `NarrativeEvent`（`internal/narrative/types.go`）新增 `ConfidenceSource`（信心度來源）與 `HitRate`（歷史命中率）
- 實作：`internal/narrative/ingestor.go` 與 `internal/narrative/knowledge_base.go` 的各 `detect*Event()` 函式
- 內建命中率：`US_rates_up: 0.72`、`JPY_carry_unwind: 0.68`、`geopolitical_risk: 0.65`、`oil_price_shock: 0.58`、`AI_capex_surge: 0.81`

### 資料流驗證
- API `/api/dashboard/recommendation-pipeline` 回傳的 `items[].factor_scores` 含完整 breakdown
- API 回傳的 `items[].conviction_breakdown` 含完整 steps
- `screened_items[].factor_scores` 含被篩選標的之因子分數

---

## 延伸指令檔

以下檔案依任務領域提供額外守則：

- `.github/instructions/go-core.instructions.md` — Go 編碼規則
- `.github/instructions/experiments-guardrails.instructions.md` — 實驗安全守則
- `.github/instructions/live-trading.guardrails.instructions.md` — Live trading 邊界
- `.github/copilot-instructions.md` — 綜合入口與常見工作流程

## Local AGENTS.md 導覽

以下子目錄已有局部說明，進入該區域工作時**先讀該目錄下的 `AGENTS.md`**，不要只依賴本檔：

| 目錄 | 主題 |
|------|------|
| `internal/orchestrator/` | `SystemCore`、`PluginHost`、三層 executor 路由 |
| `internal/experiment/` | mutation → execute → judge → promote / revert |
| `internal/portfolio/` | Darwinian 權重、FactorEngine、組合優化 |
| `internal/marketdata/` | TWSE / FinMind / Fubon / Fugle provider abstraction |
| `internal/monitoring/` | Dashboard API、監控、人工干預入口 |
| `internal/narrative/` | 巨集觀敘事、因果鏈、台灣壓力指數 |
| `internal/janus/` | cohort regime detection 與 PRISM 權重調整 |
| `internal/baseline/` | baseline policy 版本控制與回滾 |
| `internal/domain/` | canonical types / string enums / JSON schema |
| `cmd/experimental/` | 驗證 / drill / smoke-test 類 CLI |
| `scripts/openclaw/` | OpenClaw 治理、審核、promote / revert 腳本 |

### 什麼情況要往下讀局部 AGENTS.md

- 你正在改 `cmd/experimental/*` 的驗證 CLI。
- 你正在跑或修改 `scripts/openclaw/*` 的治理腳本。
- 你碰到某個 `internal/*` 子系統有自己的術語、陷阱或資料流。

### 什麼情況不用再拆更多 AGENTS.md

- `internal/config/`、`internal/db/`、`internal/ledger/`、`internal/repository/`、`internal/taskexec/` 屬於共享基礎設施；通常由本檔 + 對應程式碼即可覆蓋。
- `data/`、`graphify-out/`、`.worktrees/`、`.gocache/` 屬於狀態 / 產物 / 快取，不作為主要開發規範來源。

## 技能地圖與 AI 代理指南

**所有 AI 代理在進行任何程式修改前，必須先閱讀以下文件以理解系統架構與設計意圖：**

### 統一入口
- `.claude/SKILLS-MAP.md` — **唯一技能地圖入口**，包含所有技能的位置、職責與使用時機

### 核心技能（依任務類型）
- **架構相關**: `.claude/skills/atlas-core-architecture/SKILL.md`
- **宏觀敘事**: `.claude/skills/atlas-macro-narrative/SKILL.md`
- **風險管理**: `.claude/skills/atlas-risk-management/SKILL.md`
- **策略進化**: `.claude/skills/atlas-strategy-evolution/SKILL.md`
- **操作指南**: `.claude/skills/atlas-operations-guide/SKILL.md`

### 補充文件（詳細規格）
- `docs/architecture.md` — 架構詳細說明
- `docs/ai_agent_architecture.md` — AI 代理架構
- `docs/operations_playbook.md` — 操作手冊
- `docs/iteration_playbook.md` — 迭代指南
- `docs/evolution_loop.md` — 演化循環
- `docs/roadmap.md` — 開發路線圖

> **文件優先順序**（當內容衝突時）：
> 1. `.claude/SKILLS-MAP.md`
> 2. `.claude/skills/atlas-*/SKILL.md`
> 3. `agents.md`
> 4. `docs/*.md`
> 5. 原始碼（最終真理來源）

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (24286 symbols, 53398 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/atlas-go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/atlas-go/clusters` | All functional areas |
| `gitnexus://repo/atlas-go/processes` | All execution flows |
| `gitnexus://repo/atlas-go/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

## graphify

This project has a graphify knowledge graph at graphify-out/.

Rules:
- Before answering architecture or codebase questions, read graphify-out/GRAPH_REPORT.md for god nodes and community structure
- If graphify-out/wiki/index.md exists, navigate it instead of reading raw files
- After modifying code files in this session, run `graphify update .` to keep the graph current (AST-only, no API cost)
