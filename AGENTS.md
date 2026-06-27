# agents.md — atlas-go

> **文件角色**：本文件是 **AI 路由索引**，指向子文件的詳細規範。文章越短、AI 越不會浪費 token。
>
> **AI 須知**：修改 code 前請參閱 `.claude/skills/atlas-pre-change-protocol/SKILL.md` 執行 7 步驟檢查。
>
> **🌐 語言強制**：所有 AI 回覆必須使用**繁體中文**，禁止使用英文。使用者未特別要求英文時，不得以英文回應。

## 📜 內容歸屬規則（ALL AI MUST READ FIRST）

> 完整歸屬對照表見 **[`docs/DOCUMENTATION_STANDARD.md`](docs/DOCUMENTATION_STANDARD.md)**。
> 所有檔案當前位置見 **[`docs/DOCUMENTATION_MAP.md`](docs/DOCUMENTATION_MAP.md)**（包含 Wave 11 AGENTS.md 整合結果）。

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | 本文件 + `docs/TRAPS.md` |
| 模組內部陷阱/API/流程（hot-path） | `internal/<mod>/AGENTS.md`（**僅 21 個保留模組**）|
| 模組技術規格/契約 | `docs/specs/<topic>.md` |
| 金融工程 / 操作 playbook | `docs/guides/<topic>.md` |
| 套件文件 | `internal/<mod>/doc.go` |
| 技能 / 子代理指引 | `.claude/skills/atlas-<x>/SKILL.md` |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` |
| 憲法級強制規範 | `docs/CONSTITUTION.md`、`docs/ITERATION_GATE.md`、`internal/apigateway/CONSTITUTION.md` |
| 規範性 / 設計文件 | `docs/`（**不應放 `.omo/`** — `.gitignore` 排除）|

**防膨脹規則**：
- 本文件不超過 **160 行**（人類編寫部分）
- **155 行時觸發警告**，160 行時 PR 被拒絕
- 新知識預設加入 `internal/<mod>/AGENTS.md`（限 21 保留模組）或 `docs/`，**不要**加到這裡

## 專案概覽

`atlas-go` — 模擬優先、稽核導向的台股投資研究系統。
- **語言**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：60%

## 快速啟動 / CI 指令 / Git 工作流

> 完整內容 → **[`docs/QUICKSTART.md`](docs/QUICKSTART.md)**

## 模組路由（Wave 11 後：21 個 `internal/<mod>/AGENTS.md`）

> **重要（token 經濟）**：**不要預先讀全部子 AGENTS.md**——只在**準備修改** `internal/<mod>/` 下任一檔案時，才讀該目錄的 AGENTS.md。預先讀 21 個會浪費 ~1,300 token。
>
> **Wave 11 精簡**：原始 57 個精簡至 21 個（-63%），其他 36 個模組的內容已遷移至 `docs/specs/`、`docs/guides/`、`.claude/skills/` 或合併至 `doc.go`。

| 職能群組 | 保留 `AGENTS.md` 的模組（21 個）|
|----------|------|
| 核心引擎 | `llm` · `portfolio` · `narrative` · `live` · `experiment` · `orchestrator` |
| 資料與基礎設施 | `marketdata` · `industry` · `ledger` · `baseline` · `apigateway` · `eventbus` · `fubonproxy` · `monitoring` · `risk` |
| 策略與執行 | `strategy` · `strategy_techniques` |
| 工具與輔助 | `config` · `db` · `logging` · `realtime` |

> 沒有 `AGENTS.md` 的模組（如 `sim`、`domain`、`janus`、`prism`、`tax`、`backtest` 等）：讀 `docs/specs/` 或 `internal/<mod>/doc.go` 即可。

## 關鍵跨模組陷阱

> 完整列表 → **[`docs/TRAPS.md`](docs/TRAPS.md)**

| 陷阱 | 一句話 |
|------|--------|
| JSON tag snake_case | API parsing struct 必須對齊 `domain.*` 的 snake_case JSON tag |
| Session 日期 | 以 `SessionID` 中的交易日為準，非 `RecordedAt` |
| GuardOutcomes 對齊 | CIO 輸出必須保留原始 Agent ID |
| OutcomeCount | 不可用 `ledger.LoadOutcomes()` 填寫 |
| 權威來源單一 | 放行/過濾筆數由 `GuardOutcomes` 計算，前端不可各自重算 |
| Constitution 違反 | 不得繞過 BackgroundTaskManager、ParametersConfig、marketdata.Provider |
| 模組成熟度 | 新增 `internal/` 模組必須有 `doc.go` + 更新 `MATURITY.md` |
| FactorType 變更 | 必須同步 8 個位置，見 `.claude/skills/atlas-factor-change-protocol/SKILL.md` |
| Live 旗標 | 本地測試切勿啟用 `-allow-live-broker` |
| Replay 格式 | JSONL，不是 JSON array |
| 平行重複實作 | 新增功能前用 GitNexus `query` + codebase-memory 檢查重疊 |
| 資料可見性 | 通道靜默失敗時須暴露 `data_status` / `failed_channels` |
| LLM 路由繞過 | 不可直接呼叫 `clients/*Provider`，須透過 `DefaultRouter` |
| LLM hot-path import | S/E 模組不可 import `internal/llm` 做同步呼叫 |

## 文件索引

| 文件 | 用途 |
|------|------|
| `CLAUDE.md` | 工具進入點 |
| `docs/QUICKSTART.md` | 啟動流程 + 系統初始化順序 |
| `docs/GUIDELINES_INDEX.md` | 規範階層與使用情境路由 |
| `docs/ENVIRONMENT.md` | 外部依賴與開發環境狀態 |
| `docs/TRAPS.md` | 完整陷阱參考 |
| `docs/DOCUMENTATION_STANDARD.md` | **文件歸屬規範** |
| `docs/DOCUMENTATION_MAP.md` | **文件當前位置地圖（含 Wave 11 AGENTS.md 整合）**|
| `docs/agents-md-audit.md` | Wave 11 AGENTS.md 整合決策表（57→15→21 演化）|
| `docs/branch-hygiene/` | Branch 維護紀錄（PR #748 建立）|
| `docs/CONSTITUTION.md` | 深度開發憲法 |
| `docs/ITERATION_GATE.md` | 迭代閘門（5 Gate 自我檢查規範）|
| `docs/specs/` | 模組技術規格（Wave 11 從 AGENTS.md 抽離）|
| `docs/guides/` | 金融工程 / 操作 playbook（Wave 11 從 AGENTS.md 抽離）|
| `internal/apigateway/CONSTITUTION.md` | 數據源憲法（Data Source 6 條文 + 3 附錄）|

## 程式碼智慧工具

> 完整工具列表見 **`docs/TOOLS.md`**；GitNexus 技能見 **`.claude/skills/gitnexus/`**。
>
> **強制規則**：
> - 修改任何 function/class/method 前，執行 `gitnexus_impact({target, direction:"upstream"})`。
> - commit 前執行 `gitnexus_detect_changes()`。
> - impact 回傳 HIGH/CRITICAL 風險時，必須警告使用者並取得確認。
> - 探索程式碼用 `gitnexus_query`，查單一符號上下文用 `gitnexus_context`。
>
> **索引更新**：若 GitNexus 提示 index stale，執行 `npx gitnexus analyze --skip-agents-md`（避免自動注入 markdown 區塊）。
