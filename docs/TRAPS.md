# TRAPS.md — 高危陷阱參考

> 此文件為 `AGENTS.md` 陷阱節的詳細擴充。根 AGENTS.md 僅保留最關鍵的跨模組陷阱；模組特定陷阱請見 `internal/*/AGENTS.md`。

---

## 跨模組陷阱

### Data / Persistence

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **Replay 格式錯誤** | ledger | Replay 為 **JSONL**（每行獨立 JSON 物件），不是 JSON array。 |
| **Session 日期不可信賴 `RecordedAt`** | domain | `RecordedAt` 是計算完成時間。排序/比較請以 `SessionID` 中的交易日為準。 |
| **JSON tag 大小寫錯誤** | domain / API | API handler 讀取 JSONL 時，若 anonymous struct 的 JSON tag 用了 PascalCase 而 JSON 實際是 snake_case，unmarshal 會靜默失敗。 |
| **Scorecard OOS 欄位遺漏同步** | domain / ledger / monitoring | `domain.Scorecard` 新增 OOS 欄位（`rolling_sharpe_trend` 等）時，必須同步更新 `ledger.BuildScorecards()` 計算邏輯、`internal/monitoring/dashboard_api.go` 的 response mapping、`web-ui/js/dashboard.js` 的前端渲染，以及 `internal/web/field_types.go` 的 `go generate` 自動生成。遺漏任何一環會導致 OOS 欄位在 API 或前端消失。 |

### Orchestrator / Control

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **GuardOutcomes 與 outcomes 必須對齊** | orchestrator | 控制層（CIO）輸出應**保留原始 Agent ID**，不可覆寫為自己的 ID，否則 `PassedGuards` 會全部變 `false`。 |
| **OutcomeCount 必須是單場次數量** | ledger | `RecordSessionSummary` 絕對不可用 `ledger.LoadOutcomes()`（讀取全域檔案）來填 `OutcomeCount`。 |
| **同一件事不可有三種算法** | orchestrator | 放行/過濾筆數必須由單一權威來源（如 `GuardOutcomes`）計算，前端不可各自重算。 |
| **Darwinian 權重靜默夾制** | portfolio | 權重限制在 `[0.3, 2.5]`，超界會靜默正規化，不報錯。 |
| **重複使用 mutable `[]Recommendation`** | sim | 多次 simulation run 之間不可共用同一個 slice。 |

### 架構規範（Constitution 違反）

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **繞過共變異數優化回到線性加權** | portfolio | `optimizer.go` 已升級為 Ledoit-Wolf 共變異數矩陣 + Active-set QP。當 `o.history` 非 nil 時必須走共變異數路徑。修改 optimizer 前必須先閱讀 `docs/CONSTITUTION.md`。 |
| **繞過 BackgroundTaskManager 建立獨立排程** | apigateway | 所有定時任務**必須且只能**透過 `BackgroundTaskManager` 註冊。禁止在 goroutine 中直接啟動 `time.Ticker`。參見 `internal/apigateway/CONSTITUTION.md` 第四條。 |
| **繞過 ParametersConfig 硬編碼參數** | config | 所有可調整參數必須透過 `internal/config/parameters.go` 管理，禁止 magic number。參數必須包含 `Rationale`、`Source`、`Todo`。 |
| **建立獨立資料抓取通道** | marketdata | 所有外部資料抓取必須通過已註冊的 `marketdata.Provider`，禁止直接建立 HTTP client。參見 `internal/apigateway/CONSTITUTION.md` 第一條。 |
| **新增 internal/ 模組未標記成熟度** | 跨模組 | 每個 `internal/*/` Go package **必須**有 `doc.go` 含 `// Maturity: <tier>`。同時更新 `internal/MATURITY.md`。CI 強制。 |
| **新增/刪除/改名 FactorType** | portfolio | 因子變更必須同步更新 **7 個位置**。CI `factor-integrity` job 強制。 |
| **Save() 吃掉校準時間戳** | config | `ParametersConfig.Save()` 會覆寫整個 `parameters.json`，導致 raw JSON 欄位靜默遺失。所有校準器必須同時寫入 `last_calibrated`（Go struct）和 `calibration_timestamp`（raw JSON）。 |

### Config / Agent 治理

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **Enabled agent 缺少 prompt** | spawning | `configs/agents.json` 中每個 `enabled: true` 都需對應 `prompts/agents/<name>.md`。CI `agent-prompts` job 強制。 |
| **ScreeningCriteria 靜默過濾** | screener | `configs/agents.json` 中若設定了 `screening_criteria`，標的在進入 executor **之前**就會被過濾。這是預期行為，不是 bug。 |
| **Live 交易風險** | live | `cmd/atlas` 有 `-allow-live-broker`、`-allow-real-signor` 等旗標，本地測試時切勿意外啟用。 |

### Baseline / Experiment

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **Baseline 未載入** | baseline | 實驗執行/評估前必須確認 `data/state/baseline_policy.json` 存在且有效。 |

### Build Pipeline / 程式碼生成

| 陷阱 | 所屬模組 | 說明 |
|------|---------|------|
| **手動編輯 `web/static/js/shared/field_types.ts` 或 `valid_fields.json`** | domain / web | 這兩個檔案是 `cmd/gentags` 從 `internal/*/*.go` 的 struct JSON tag 自動產出(`go generate .` 觸發)。**禁止手動編輯** — 任何變更會在下次 `go generate` 被覆寫。<br><br>若需新增/修改/刪除前端可見的欄位或介面:<br>1. 修改對應 Go struct 的 `json:"..."` tag(在 `internal/<pkg>/`)<br>2. 跑 `go generate .` 重新產出這兩個檔<br>3. **不要**直接編輯這兩個檔<br><br>違反的後果:`go generate .` 會覆寫你的手動編輯,並且會在 quality.yml 的 `generate` job 報 "uncommitted changes" → 5 個 frontend PR 全 CI fail。<br><br>防護:`.githooks/pre-commit` Phase 5 自動跑 `go generate .`,若這兩個檔有 drift 會**阻擋 commit**。修正方式見 `web/AGENTS.md`「Generated Files」章節。 |

---

## 模組特定陷阱

以下陷阱屬於特定模組範圍，詳見各模組的 `AGENTS.md`：

- **Portfolio**: 權重、FactorEngine、FactorType 變更流程 → `internal/portfolio/AGENTS.md`
- **Orchestrator**: 三層 executor 路由、GuardOutcomes 對齊 → `internal/orchestrator/AGENTS.md`
- **Live**: 交易安全旗標 → `internal/live/AGENTS.md`
- **MarketData**: Provider 註冊規則 → `internal/marketdata/AGENTS.md`
- **Experiment**: Mutation → execute → judge → promote 生命週期 → `internal/experiment/AGENTS.md`
- **Baseline**: 升降級與版本控制 → `internal/baseline/AGENTS.md`
- **Monitoring**: Dashboard API、人工干預 → `internal/monitoring/AGENTS.md`
- **Narrative**: 宏觀敘事、因果鏈 → `internal/narrative/AGENTS.md`

---

## 文件歸屬規則

本文檔會隨項目演進持續更新。新增陷阱時，請判斷：

1. **跨模組**（影響 2+ 模組、無歸屬單一模組）→ 加入本文件
2. **單一模組** → 加入該模組的 `internal/<mod>/AGENTS.md`
3. **CI/流程相關** → 可能歸屬 `.github/instructions/` 下的領域守則
