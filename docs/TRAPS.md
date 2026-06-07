# TRAPS.md — 高危陷阱參考

> 此文件為 `AGENTS.md` 陷阱節的詳細擴充。根 AGENTS.md 僅保留最關鍵的跨模組陷阱；模組特定陷阱請見 `internal/*/AGENTS.md`。

---

## 跨模組陷阱

### Data / Persistence

| 陷阱 | 說明 |
|------|------|
| **Replay 格式錯誤** | Replay 為 **JSONL**（每行獨立 JSON 物件），不是 JSON array。 |
| **Session 日期不可信賴 `RecordedAt`** | `RecordedAt` 是計算完成時間。排序/比較請以 `SessionID` 中的交易日為準（如 `session-20260413-daily` → `2026-04-13`）。 |
| **JSON tag 大小寫錯誤** | API handler 讀取 JSONL 時，若 anonymous struct 的 JSON tag 用了 PascalCase（如 `json:"FactorScores"`）而 JSON 檔案實際寫入時是 snake_case，unmarshal 會靜默失敗。所有 `domain.*` struct 的 JSON tag 均為 snake_case，API parsing struct 必須對齊。 |

### Orchestrator / Control

| 陷阱 | 說明 |
|------|------|
| **GuardOutcomes 與 outcomes 必須對齊** | 控制層（CIO）輸出應**保留原始 Agent ID**，不可覆寫為自己的 ID，否則 `PassedGuards` 會全部變 `false`。 |
| **OutcomeCount 必須是單場次數量** | `RecordSessionSummary` 絕對不可用 `ledger.LoadOutcomes()`（讀取全域檔案）來填 `OutcomeCount`。 |
| **同一件事不可有三種算法** | 放行/過濾筆數必須由單一權威來源（如 `GuardOutcomes`）計算，前端不可各自重算。 |
| **Darwinian 權重靜默夾制** | 權重限制在 `[0.3, 2.5]`，超界會靜默正規化，不報錯。 |
| **重複使用 mutable `[]Recommendation`** | 多次 simulation run 之間不可共用同一個 slice。 |

### 架構規範（Constitution 違反）

| 陷阱 | 說明 |
|------|------|
| **繞過共變異數優化回到線性加權** | `optimizer.go` 已升級為 Ledoit-Wolf 共變異數矩陣 + Active-set QP（見 `internal/portfolio/AGENTS.md` §4.1）。當 `o.history` 非 nil 時必須走共變異數路徑。修改 optimizer 前必須先閱讀 `.omo/CONSTITUTION.md`。 |
| **繞過 BackgroundTaskManager 建立獨立排程** | 所有定時任務**必須且只能**透過 `BackgroundTaskManager` 註冊。禁止在 goroutine 中直接啟動 `time.Ticker`。參見 `internal/apigateway/CONSTITUTION.md` 第四條。 |
| **繞過 ParametersConfig 硬編碼參數** | 所有可調整參數必須透過 `internal/config/parameters.go` 管理，禁止 magic number。參數必須包含 `Rationale`、`Source`、`Todo`。 |
| **建立獨立資料抓取通道** | 所有外部資料抓取必須通過已註冊的 `marketdata.Provider`，禁止直接建立 HTTP client。參見 `internal/apigateway/CONSTITUTION.md` 第一條。 |
| **新增 internal/ 模組未標記成熟度** | 每個 `internal/*/` Go package **必須**有 `doc.go` 含 `// Maturity: <tier>`。同時更新 `internal/MATURITY.md`。CI 強制。 |
| **新增/刪除/改名 FactorType** | 因子變更必須同步更新 **7 個位置**。CI `factor-integrity` job 強制。 |
| **Save() 吃掉校準時間戳** | `ParametersConfig.Save()` 會覆寫整個 `parameters.json`，導致 raw JSON 欄位靜默遺失。所有校準器必須同時寫入 `last_calibrated`（Go struct）和 `calibration_timestamp`（raw JSON）。 |

### Config / Agent 治理

| 陷阱 | 說明 |
|------|------|
| **Enabled agent 缺少 prompt** | `configs/agents.json` 中每個 `enabled: true` 都需對應 `prompts/agents/<name>.md`。CI `agent-prompts` job 強制。 |
| **ScreeningCriteria 靜默過濾** | `configs/agents.json` 中若設定了 `screening_criteria`，標的在進入 executor **之前**就會被過濾。這是預期行為，不是 bug。 |
| **Live 交易風險** | `cmd/atlas` 有 `-allow-live-broker`、`-allow-real-signor` 等旗標，本地測試時切勿意外啟用。 |

### Baseline / Experiment

| 陷阱 | 說明 |
|------|------|
| **Baseline 未載入** | 實驗執行/評估前必須確認 `data/state/baseline_policy.json` 存在且有效。 |

### 自動化（不會踩到的）

| 陷阱 | 說明 |
|------|------|
| **前端欄位命名不一致 → 已自動解決** | git pre-commit hook 會自動執行 `go generate .` 同步前端類型定義。 |
| **Go → 前端類型自動生成** | `cmd/gentags` 從 Go struct JSON tag 自動生成。pre-commit hook 自動觸發。 |

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
