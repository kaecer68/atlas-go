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
`Market Data → Orchestrator (context → screener → sector/style → control) → Simulator → Ledger`

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

- **整合測試**：CI 使用 `go test -v -tags=integration ./...`（需要 Redis + PostgreSQL 服務）。根目錄的 `integration_test.go` 與 `internal/repository/*_test.go` 皆有 `//go:build integration` 標籤，因此 `go test ./...`（不含 `-tags=integration`）會正確略過這些檔案，不會常規執行。
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
- [ ] commit 前確認 staging area：`git diff --cached --stat`（pre-commit hook 可能變更 staged files）

### Solo 開發者 AI Code Review 流程

本專案為 solo 開發，main 分支保護規則要求 1 個 approving review。
以下流程用 AI 做第二人審查，滿足四人原則精神：

```bash
# 1. 建立 feature branch 並開發
git checkout -b feat/<name>
# ... 開發、測試、提交 ...

# 2. 推送並建立 PR
git push -u origin feat/<name>
gh pr create --title "feat(scope): description" --body "..." --base main

# 3. AI Code Review（二選一或兩者都跑）
/codex review        # OpenAI Codex — 對抗式審查（找漏洞、邊界條件）
/claude review       # Claude — 獨立 diff review（找盲點）

# 4. 確認 CI 通過 + AI review 無 critical issue 後合併
gh pr merge --admin  # admin 權限繞過 required review
```

**原理**：Branch protection 保留但不阻擋開發。`enforce_admins: false` 允許 admin bypass。
AI review 提供第二雙眼睛。CI 閘門（governance、operations、coverage、lint、commitlint）確保自動化品質。

## 高危陷阱

調整行為前請先確認：

| 陷阱 | 說明與預防 |
|------|-----------|
| **Enabled agent 缺少 prompt** | `configs/agents.json` 中每個 `enabled: true` 都需對應 `prompts/agents/<name>.md`。CI（`quality.yml` 的 `agent-prompts` job）會強制檢查，缺少 prompt 檔案的 PR 會被拒絕。 |
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
| **繞過共變異數優化回到線性加權** | `optimizer.go` 已升級為 Ledoit-Wolf 共變異數矩陣 + Active-set QP（見 `internal/portfolio/AGENTS.md` §4.1）。當 `o.history` 非 nil 時必須走共變異數路徑，不可降級為線性歸一化。修改 optimizer 前必須先閱讀 `.omo/CONSTITUTION.md`（深度憲法）和 `.omo/ITERATION_GATE.md`（迭代閘門），任何純線性加權視為違反憲法第一條。 |
| **繞過 BackgroundTaskManager 建立獨立排程** | 所有定時自動執行的後台任務**必須且只能**透過 `BackgroundTaskManager` 註冊（`cmd/atlas/main.go`）。禁止在 goroutine 中直接啟動 `time.Ticker`、禁止在 `init()` 中啟動後台工作、禁止繞過統一架構直接呼叫業務邏輯的定時執行。參見 `internal/apigateway/CONSTITUTION.md` 第四條。 |
| **繞過 ParametersConfig 硬編碼參數** | 所有可調整的參數必須透過 `internal/config/parameters.go` 的 `ParametersConfig` 管理，禁止在業務邏輯中硬編碼 magic number。參數必須包含 `Rationale`、`Source`、`Todo` 欄位說明權威性溯源。 |
| **建立獨立資料抓取通道** | 所有外部資料抓取必須通過已註冊的 `marketdata.Provider`，禁止為了「方便」而繞過 Gateway 直接建立 HTTP client。參見 `internal/apigateway/CONSTITUTION.md` 第一條。 |
| **新增 internal/ 模組未標記成熟度** | 每個 `internal/*/` Go package **必須**有 `doc.go`，內含 `// Maturity: <tier>` 標記（`stable`/`evolving`/`experimental`/`utility`）。同時必須更新 `internal/MATURITY.md` 參考表。CI 會強制檢查一致性。違反此規則的 PR 會被 `quality.yml` 的 `maturity` job 拒絕。 |
| **新增/刪除/改名 FactorType** | 因子變更必須同步更新 **7 個位置**：`optimizer.go` FactorType 常數 → `factor_weight_engine.go` defaultBaseWeights → `shared.go` FactorScoreBreakdown + FactorScores → `optimizer.go` symbolScore + totalScore + buildPositions → `factor_engine.go` CalculateAllScoresWithBreakdown → `factor_weight_engine.go` applyEventAdjustment + strategyDeltas + GetWeights → 跑 `go generate .` 同步前端型別 → 跑 `bash scripts/ci/verify_factor_integrity.sh` 驗證 G1-G10。違反此規則的 PR 會被 `quality.yml` 的 `factor-integrity` job 拒絕。 |

---

## 統一架構規範（強制）

### 背景任務統一排程

所有需要「定時自動執行」的後台任務，**必須且只能**透過 `BackgroundTaskManager` 管理：

- **實作位置**：`internal/apigateway/background.go`
- **憲法規範**：`internal/apigateway/CONSTITUTION.md`（第一條、第四條）
- **註冊位置**：`cmd/atlas/main.go`（搜尋 `taskMgr.Register` 或 `taskMgr.RegisterTask`）

**TaskExec vs BackgroundTaskManager 區別**：

| 機制 | 用途 | 觸發方式 |
|------|------|---------|
| `internal/taskexec` | 使用者手動提交的長時間任務（可取消、可訂閱） | HTTP API / 手動 |
| `BackgroundTaskManager` | 系統自動定時執行的維護任務 | 排程（每日/每小時等） |

兩者可共存：BackgroundTaskManager 的定時任務可直接呼叫業務邏輯，不需經過 TaskExec。

### 參數統一管理

所有可調整的參數必須透過 `internal/config/parameters.go` 管理：

- **參數定義**：`ParametersConfig` struct 中的 `ParameterMetadata[T]`
- **預設值**：`internal/config/parameters_defaults.go`
- **驗證**：`ParametersConfig.Validate()`
- **讀取**：`config.GetParametersConfig()`

**參數必須包含**：
- `Value`：當前值
- `Rationale`：設定理由
- `Source`：權威性溯源（`heuristic`、`backtest`、`academic` 等）
- `Todo`：未來校準計劃

### 資料統一管理

所有外部資料抓取必須通過統一通道：

- **API Gateway**：`internal/apigateway/gateway.go`
- **Provider 介面**：`internal/marketdata/provider.go`
- **已註冊 Channel**：`cmd/atlas/main.go` 中 `gateway.RegisterChannel()` 呼叫

**禁止行為**：
- 禁止直接 `os.Getenv` 建立 HTTP client
- 禁止繞過 Gateway 直接呼叫外部 API
- 禁止為單一功能建立獨立資料抓取邏輯

### 模組成熟度標記

所有 `internal/*/` Go package 必須透過 `doc.go` 標記成熟度，並與 `internal/MATURITY.md` 保持一致。

**成熟度層級**：

| Tier | 標記 | 含義 |
|------|------|------|
| S | `stable` | 穩定生產 — API 穩定，breaking change 需 migration plan |
| E | `evolving` | 演進中 — API 可能調整，可能晉升為 stable |
| X | `experimental` | 實驗中 — 研究性質，不應被其他模組依賴 |
| U | `utility` | 輔助工具 — CLI 工具/資料轉換，非 runtime |

**AI agent 工作流程**：

1. **新建 `internal/` 模組**：
   - 建立 `doc.go`，加入 `// Maturity: <tier>` 標記
   - 更新 `internal/MATURITY.md`，將新模組加入對應層級的表格
   - 執行 `bash scripts/ci/check_maturity.sh` 確認通過

2. **變更成熟度**：
   - 修改 `doc.go` 中的 Maturity 標記
   - 同步更新 `internal/MATURITY.md`
   - X→E 或 E→S 視為晉升，需 PR review
   - S→E 或任何降級需 migration plan

3. **本地驗證**：
   ```bash
   # 快速檢查
   bash scripts/ci/check_maturity.sh
   # Go 工具（更詳細的輸出）
   go run ./cmd/check-maturity
   ```

**CI 強制**：`quality.yml` 的 `maturity` job 會在每個 PR 自動執行檢查，不一致會導致 CI 失敗。

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

---

## 產業生態系（Industry Ecosystem）

前端頁面「產業生態系」包含三個核心板塊，各自對應完整的後端計算鏈：

### 供應鏈連動（Supply Chain Linkage）
- **核心檔案**：`internal/industry/linkage.go`、`configs/supply_chain_graph.json`
- **圖譜定義**：`configs/supply_chain_graph.json` 定義節點關係（upstream/downstream/supplier），可在不重新編譯下修改。
- **圖譜載入**：`LoadSupplyChainGraph()` 從 JSON 載入後同時填入 `SupplyChainGraph` 與 `CorrelationMatrix`。
- **相關矩陣**：`CorrelationMatrix` 支援三種初始化方式：
  1. `DefaultCorrelationMatrix()` — 硬編碼預設值（回退方案）
  2. `LoadCorrelationMatrixFromConfig()` — 從 `configs/parameters.json` 的 `industry.linkage_params.correlation_matrix` 讀取
  3. `RecalculateFromReturns()` — 從產業報酬率時間序列實證計算
- **敘事感知調整**：`NarrativeLinkageProvider` 介面允許宏觀敘事主題（如 `oil_price_shock`、`AI_capex_surge`）動態調整產業間相關係數。實作位於 `SeasonalBridge.CorrelationMultiplier()`。
- **衝擊傳導**：`PropagateShock()` 計算衝擊從來源產業向下游（顧客）與上游（供應商）的傳導，使用可配置的衰減因子（`downstream_decay_factor`/`upstream_decay_factor`）。
- **系統重要性**：`CalculateLinkageScore()` 基於產業在圖中的連線數與 `systemic_importance_divisor`（預設 10.0）計算系統重要性分數。
- **實證校準**：`cmd/calibrate-seasonal` 支援 `--replay` 旗標載入歷史回測數據進行實證相關矩陣計算。TWSE 產業指數提供者 `TWSESectorIndexProvider` 可從 TWSE API 抓取產業指數歷史資料。

### 季節性模式（Seasonal Patterns）
- **核心檔案**：`internal/industry/seasonality.go`、`internal/industry/seasonal_calibrator.go`
- **季節性引擎**：`SeasonalEngine` 管理各產業的季節性模式（月曆效應），每種模式包含 `StartMonth/Day`、`EndMonth/Day`、`AdjustmentFactor`、`HistoricalAccuracy` 等欄位。
- **校準管道**：`cmd/calibrate-seasonal` CLI 支援：
  - 合成數據（預設）或實際歷史回測數據（`--replay`）
  - `--update` 旗標將校準結果寫回 `configs/parameters.json`
  - `--update-threshold` 設定最小觀測數門檻
- **證據品質標記**：每個模式參數包含 `evidence_quality` 欄位（`high`/`medium`/`low`/`heuristic_awaiting_data`），前端根據品質顯示對應 badge（「待驗證」）。
- **參數驗證**：`ParametersConfig.Validate()` 確保 `seasonal_decay_factor`（預設 0.30）等在合理範圍。
- **API**：`/api/industry/seasonality` 回傳季節性模式列表；`/api/industry/seasonality/calendar` 回傳年度行事曆。
- **決策鏈透明化**：`GetAdjustmentBreakdown()` 提供四層調整分解（季節性 × 敘事 × 循環 × 環境），前端逐層展示。

### 週期羅盤（Cycle Compass）
- **核心檔案**：`internal/industry/cycle.go`、`internal/industry/dynamic_env.go`
- **商業週期偵測**：`CycleTracker` 管理五種產業階段（`expansion`/`recovery`/`mature`/`recession`）的偵測。
- **動態環境調變**：`DynamicEnvModulator` 將宏觀數據（原油、BDI、DXY 等）納入週期評分計算。
- **API**：`/api/industry/cycles` 回傳各產業的週期位置與趨勢。

---

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

### 🔴 修改前必讀（Pre-Change Protocol）

**任何程式碼修改前，必須執行此技能的檢查清單，禁止跳過：**
- `.claude/skills/atlas-pre-change-protocol/SKILL.md` — **強制 7 步驟檢查：blast radius → 模組陷阱 → 數據源溯源 → 憲法檢查 → 模式匹配 → graphify 架構 → 代碼意圖**

### 統一入口
- `.claude/SKILLS-MAP.md` — 技能地圖入口（v2.0，含全部 39 技能）
- `docs/GUIDELINES_INDEX.md` — **規範文件總索引**，包含所有規範的權威階層與使用情境路由

### 核心技能（依任務類型）
- **修改前必讀**: `.claude/skills/atlas-pre-change-protocol/SKILL.md`
- **架構相關**: `.claude/skills/atlas-core-architecture/SKILL.md`
- **宏觀敘事**: `.claude/skills/atlas-macro-narrative/SKILL.md`
- **風險管理**: `.claude/skills/atlas-risk-management/SKILL.md`
- **策略進化**: `.claude/skills/atlas-strategy-evolution/SKILL.md`
- **操作指南**: `.claude/skills/atlas-operations-guide/SKILL.md`
- **GitNexus 工具**: `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md`, `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md`, `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md`

### 補充文件（詳細規格）
- `docs/architecture.md` — 架構詳細說明
- `docs/ai_agent_architecture.md` — AI 代理架構
- `docs/DATA_ARCHITECTURE.md` — **資料架構權威文件**（資料儲存層、讀寫路徑、AI 代理常見錯誤）
- `docs/operations_playbook.md` — 操作手冊
- `docs/iteration_playbook.md` — 迭代指南
- `docs/evolution_loop.md` — 演化循環
- `docs/roadmap.md` — 開發路線圖

> **文件優先順序**（當內容衝突時）：
> 1. `docs/GUIDELINES_INDEX.md` — **規範索引為最終仲裁者**
> 2. `internal/apigateway/CONSTITUTION.md` — 憲法（強制規範，CI 檢查）
> 3. `.omo/CONSTITUTION.md` — 深度憲法（矩陣運算、真實數據、證偽要求，適用於 optimizer/portfolio/risk）
> 4. `.github/instructions/*.md` — 領域守則
> 5. `.claude/skills/atlas-pre-change-protocol/SKILL.md` — **修改前強制檢查清單**
> 6. `.claude/SKILLS-MAP.md` / `.claude/skills/atlas-*/SKILL.md` — 技能文件
> 7. `agents.md` — 倉庫層級邊界
> 8. `internal/*/AGENTS.md` — 模組特有陷阱
> 9. `docs/*.md` — 參考文件
> 10. 原始碼（最終真理來源）

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (34690 symbols, 79521 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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

## 自主校準閉環架構（2026-05-22 新增）

從 Phase C6-D5 開始導入的自主演化架構，目標是讓系統自我校準、自我進化，人工只負責監管與否決。

### 校準閉環流程

JANUS regime detection（每小時）→ regime change 偵測 → RiskGate.SelfCalibrate()：
1. 載入最近 30 個 session 的推薦與 forward return
2. 重播 pre-trade 決策（哪些被 block、哪些被 allow）
3. 對比實際結果（block 壞單 = TP, block 好單 = FP）
4. 計算 F1 score + precision/recall
5. Bayesian optimizer 搜尋最佳 threshold
6. 套用新參數 → 記錄 CalibrationReport

固定排程校準（每 24h）：risk_gate_calibrate task，同上但使用全部可用 session。

CalibrationReport → GET /api/dashboard/risk-calibration 端點，回傳最近一次校準結果。

### 校準範圍

| 規則 | 參數 | 預設值 |
|------|------|--------|
| max_position_pct | risk_max_position_size | 0.15 |
| cash_buffer | risk_max_daily_loss_pct | 0.03 |

### 監控方式

所有閉環行為透過結構化 logging 輸出，不需要人工介入即可觀察。

### 背景任務一覽

| Task | 間隔 | 觸發條件 | 行為 |
|------|------|----------|------|
| risk_gate_calibrate | 24h | 時間到 | 載入 30 session → 校準參數 |
| regime_calibrate | 1h | regime 變化 | 載入 20 session → 校準參數 |
| rule_engine_check | 30s | 時間到 | 檢查警報規則 |

## graphify

This project has a graphify knowledge graph at graphify-out/.

Rules:
- Before answering architecture or codebase questions, read graphify-out/GRAPH_REPORT.md for god nodes and community structure
- If graphify-out/wiki/index.md exists, navigate it instead of reading raw files
- After modifying code files in this session, run `graphify update .` to keep the graph current (AST-only, no API cost)
- **子圖譜更新**：執行 `bash scripts/regenerate-subgraphs.sh`，會依序跑 `graphify update .` + `python3 scripts/slice-graph.py`，將 master graph 切成 4 個子圖譜（core / analysis / research / infra）並產生互動式 HTML，每個子圖譜 < 700 節點
- 子圖譜導覽入口：`graphify-out/subgraphs/index.html`
