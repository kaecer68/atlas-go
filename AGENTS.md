# agents.md — atlas-go

本檔是 AI 開發代理的參考手冊。所有資訊以實際程式碼與設定為準，不做臆測。

## 專案概覽

- **語言**：Go 1.25.0，模組 `github.com/kaecer68/atlas-go`
- **定位**：模擬優先、稽核導向的台股投資研究系統
- **資料庫**：PostgreSQL 15（持久化）、Redis 7（快取）
- **CI 工具**：gofmt、go vet、staticcheck、golangci-lint、gosec
- 無 Makefile，使用原生 Go 工具鏈

## CI 對齊指令（修改後必跑）

```bash
test -z "$(gofmt -l .)"        # 格式檢查
go build ./...                  # 建置
go test ./...                   # 測試
go vet ./...                    # 靜態分析
staticcheck ./...               # 進階 lint
# 覆蓋率（門檻 40%）
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

## 常用執行入口

```bash
go run ./cmd/atlas                              # HTTP server（port 8080）
go run ./cmd/run-experiment -brief <file>        # 執行實驗
go run ./cmd/judge-experiment                    # 評判最新實驗
go run ./cmd/promote-baseline                    # 晉升最新 accepted 實驗
go run ./cmd/revert-baseline --list              # 列出版本
go run ./cmd/backtest-window -start <date> -end <date>
go run ./cmd/import-replay -source <csv> -target <jsonl>
```

`cmd/experimental/` 下另有驗證/演練子命令，詳見該目錄的 `AGENTS.md`。

## 核心架構

### 主要模組（核心資料流）

| 目錄 | 職責 |
|------|------|
| `internal/domain/` | 領域型別（Regime、Recommendation、Position 等字串 enum） |
| `internal/orchestrator/` | 流程協調（SystemCore、PluginHost、多層 executor 路由） |
| `internal/sim/` | 模擬引擎與部位狀態轉換 |
| `internal/experiment/` | 實驗執行與評判 |
| `internal/baseline/` | Baseline policy 升降級與版本控制 |
| `internal/ledger/` | JSONL append-only 持久化 |
| `internal/portfolio/` | Darwinian 權重管理 + FactorEngine（多因子計算） |
| `internal/screener/` | 宣告式個股篩選 |
| `internal/marketdata/` | 資料提供者抽象（TWSE、Fugle、Hybrid） |
| `internal/live/` | 已強化但需謹慎啟用（`-allow-live-broker` 旗標） |
| `internal/prism/` | Regime-specific 訓練佇列 |
| `internal/swarm/` | MiroFish swarm 模擬 |
| `internal/janus/` | 跨 cohort regime 偵測與 PRISM 權重調整 |
| `internal/narrative/` | 宏觀敘事事件偵測、因果鏈、台灣壓力指數 |

> 輔助模組（adversarial、backtest、bootstrap、config、db、eventbus、evolution、globalmarket、importer、industry、logging、monitoring、realtime、reflexivity、replay、reporting、repository、risk、spawning、strategy、stress、taskexec、tax、metalearning）的詳細職責見 `docs/architecture.md` 與各模組的 `AGENTS.md`。

**分層資料流**：`Market Data → Orchestrator (context → screener → sector/style → control) → Simulator → Ledger`

## 程式碼慣例

- **介面風格**：小而聚焦，常見為 `Supports(...) bool` + 一個操作方法
- **Early return**：優先使用，減少巢狀縮排
- **錯誤包裝**：一律 `fmt.Errorf("context: %w", err)`
- **Import 順序**：標準庫 → 外部套件 → `github.com/kaecer68/atlas-go/...`
- **測試檔**：與原始碼同目錄同 package，`*_test.go` 命名
- **領域 enum**：字串型別（方便 JSON roundtrip）

## 測試須知

- **整合測試**：`go test -v -tags=integration ./...`（需要 Redis + PostgreSQL）。不含 `-tags=integration` 會正確略過。
- **Race detector**：CI 對 unit test 啟用 `-race`
- **覆蓋率門檻**：≥ 40%
- **治理 gate**：
  ```bash
  bash ./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
  bash ./scripts/openclaw/verify_operations_gate.sh
  ```

## 設定檔慣例

- `configs/agents.json` — 代理註冊表。每個 `enabled: true` 的 agent 必須在 `prompts/agents/` 下有對應 prompt 檔案（CI 強制檢查）。
- `internal/config/config.go` 自動讀取 `.env`，不覆蓋已存在的環境變數。`.env` 中的引號會被自動去除。
- 環境變數前綴 `ATLAS_*`（如 `ATLAS_MARKET_DATA_PROVIDER`、`ATLAS_BASELINE_POLICY_PATH`、`ATLAS_BROKER_MODE`）。

## Git 工作流（強制）

- 分支命名：`feat/<name>` / `fix/<name>` / `refactor/<name>`
- **禁止直接 push main**，一律 PR
- 提交前通過：`go build ./...` + `go test ./...` + `gofmt` + `staticcheck`
- commit 前確認 staging area：`git diff --cached --stat`（pre-commit hook 可能變更 staged files）
- Solo 開發 AI review 流程：`/codex review` 或 `/claude review` → CI 通過 → `gh pr merge --admin`

## 高危陷阱

### 靜默錯誤（CI 抓不到）

| 陷阱 | 後果 |
|------|------|
| 重複使用 mutable `[]Recommendation` 跨 simulation run | 資料污染，無報錯 |
| `Save()` 覆寫 `parameters.json` | raw JSON 欄位（`calibration_timestamp`）靜默遺失 |
| JSON tag 大小寫不對齊（PascalCase vs snake_case） | API unmarshal 靜默失敗，欄位永遠 nil |
| `ScreeningCriteria` 過濾 | 標的進 executor 前被靜默移除 |
| Darwinian 權重夾制 `[0.3, 2.5]` | 超界值靜默正規化 |
| `RecordedAt` 作為排序依據 | 不可信賴，應以 `SessionID` 中的交易日為準 |
| `GuardOutcomes` 覆寫 Agent ID | `PassedGuards` 全變 false |

### CI 強制（違反會被拒絕）

| 陷阱 | CI job |
|------|--------|
| Enabled agent 缺少 prompt 檔案 | `agent-prompts` |
| FactorType 新增/刪除/改名未同步 7 個位置 | `factor-integrity` |
| 新增 `internal/` 模組未標記 Maturity 或未更新 `internal/MATURITY.md` | `maturity` |
| `Save()` 導致校準時間戳遺失 | `validate-parameters` |
| Go → 前端類型未同步（pre-commit hook 自動處理，CI 二次檢查） | `generate` |

## 統一架構規範

### 背景任務統一排程

所有定時任務必須透過 `BackgroundTaskManager`（`internal/apigateway/background.go`）註冊。禁止在 goroutine 中直接啟動 `time.Ticker`、禁止在 `init()` 中啟動後台工作。註冊位置：`cmd/atlas/main.go`。

`TaskExec`（`internal/taskexec`）用於使用者手動提交的長時間任務；`BackgroundTaskManager` 用於系統自動排程。兩者可共存。

### 參數統一管理

所有可調整參數必須透過 `internal/config/parameters.go` 的 `ParametersConfig` 管理。每個參數必須含 `Value`、`Rationale`、`Source`（權威性溯源）、`Todo`。禁止在業務邏輯中硬編碼 magic number。

### 資料統一通道

所有外部資料抓取必須通過 `marketdata.Provider` 介面。禁止繞過 Gateway 直接建立 HTTP client。

> 完整憲法規範見 `internal/apigateway/CONSTITUTION.md`；深度憲法（optimizer/portfolio/risk）見 `.omo/CONSTITUTION.md`。

## 人工覆寫機制

投資管線頁面提供三種按鈕：**放行**（`approve_rec`）、**否決**（`reject_rec`）、**補追**（語義同放行，僅針對已被控制層擋下的項目）。所有干預持久化至 `data/state/approvals/`。

## 資源導航

| 需求 | 路徑 |
|------|------|
| 架構細節 | `docs/architecture.md` |
| 資料架構權威文件 | `docs/DATA_ARCHITECTURE.md` |
| 模組成熟度系統 | `docs/MATURITY.md` |
| 產業生態系（供應鏈、季節性、週期） | `docs/industry-ecosystem.md` |
| 決策鏈透明化（Audit Trail） | `docs/audit-trail.md` |
| 自主校準閉環 | `docs/calibration-loop.md` |
| 參數系統 | `docs/PARAMETER_SYSTEM.md` |
| 模組特有陷阱 | `internal/*/AGENTS.md` |
| 實驗安全守則 | `.github/instructions/experiments-guardrails.instructions.md` |
| Live trading 邊界 | `.github/instructions/live-trading.guardrails.instructions.md` |
| Go 編碼規則 | `.github/instructions/go-core.instructions.md` |
| 規範衝突最終仲裁 | `docs/GUIDELINES_INDEX.md` |
| 技能地圖入口（39 技能） | `.claude/SKILLS-MAP.md` |

## 延伸指令檔

- `.github/instructions/go-core.instructions.md` — Go 編碼規則
- `.github/instructions/experiments-guardrails.instructions.md` — 實驗安全守則
- `.github/instructions/live-trading.guardrails.instructions.md` — Live trading 邊界
- `.github/copilot-instructions.md` — 綜合入口與常見工作流程

GitNexus 使用規則見 `CLAUDE.md`（Always Do / Never Do / Resources / CLI）。兩者皆注入 context，本檔不重複。
