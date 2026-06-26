# agents.md — atlas-go

> **文件角色**：本文件是 **AI 路由索引**，指向子文件的詳細規範。文章越短、AI 越不會浪費 token。
>
> **AI 須知**：修改 code 前請參閱 `.claude/skills/atlas-pre-change-protocol/SKILL.md` 執行 7 步驟檢查。
>
> **🌐 語言強制**：所有 AI 回覆必須使用**繁體中文**，禁止使用英文。使用者未特別要求英文時，不得以英文回應。

## 📜 內容歸屬規則（ALL AI MUST READ FIRST）

> 完整歸屬對照表見 **[`docs/DOCUMENTATION_STANDARD.md`](docs/DOCUMENTATION_STANDARD.md)**。
> 所有檔案當前位置見 **[`docs/DOCUMENTATION_MAP.md`](docs/DOCUMENTATION_MAP.md)**。

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | 本文件 |
| 模組內部陷阱/API/流程 | `internal/<mod>/AGENTS.md` |
| 操作程序 / playbook | `docs/` |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` |
| 憲法級強制規範 | `docs/CONSTITUTION.md`、`docs/ITERATION_GATE.md`、`internal/apigateway/CONSTITUTION.md` |
| 技能 / 子代理指引 | `.claude/SKILLS-MAP.md` |
| 規範性 / 設計文件 | `docs/`（**不應放 `.omo/`** — `.gitignore` 排除，新 clone 看不到）|

**防膨脹規則**：
- 本文件不超過 **160 行**（人類編寫部分）
- **155 行時觸發警告**，160 行時 PR 被拒絕
- 新知識預設加入 `internal/<mod>/AGENTS.md` 或 `docs/`，**不要**加到這裡

## 專案概覽

`atlas-go` — 模擬優先、稽核導向的台股投資研究系統。
- **語言**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：40%

## 快速啟動 / CI 指令 / Git 工作流

> 完整內容 → **[`docs/QUICKSTART.md`](docs/QUICKSTART.md)**

## 模組路由

| 職能群組 | 模組 |
|----------|------|
| 核心引擎 | [`sim`](internal/sim/AGENTS.md) · [`experiment`](internal/experiment/AGENTS.md) · [`baseline`](internal/baseline/AGENTS.md) · [`portfolio`](internal/portfolio/AGENTS.md) · [`prism`](internal/prism/AGENTS.md) · [`janus`](internal/janus/AGENTS.md) |
| 控制與決策 | [`orchestrator`](internal/orchestrator/AGENTS.md) · [`narrative`](internal/narrative/AGENTS.md) · [`risk`](internal/risk/AGENTS.md) · [`industry`](internal/industry/AGENTS.md) |
| 資料與基礎設施 | [`marketdata`](internal/marketdata/AGENTS.md) · [`ledger`](internal/ledger/AGENTS.md) · [`repository`](internal/repository/AGENTS.md) · [`eventbus`](internal/eventbus/AGENTS.md) · [`realtime`](internal/realtime/AGENTS.md) · [`apigateway`](internal/apigateway/CONSTITUTION.md) |
| 工具與輔助 | [`config`](internal/config/) · [`db`](internal/db/) · [`screener`](internal/screener/AGENTS.md) · [`spawning`](internal/spawning/AGENTS.md) · [`tax`](internal/tax/AGENTS.md) · [`live`](internal/live/AGENTS.md) · [`swarm`](internal/swarm/) |
| LLM | [`llm`](internal/llm/AGENTS.md) |

> 沒有 `AGENTS.md` 的模組為共享基礎設施，直接讀碼即可。
> **進入 `internal/*/` 目錄修改程式碼前，強制先讀該目錄下的 `AGENTS.md`（或 `CONSTITUTION.md`）。**

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
| LLM hot-path import | S/E 模組不可 import `internal/llm` 同步呼叫 |

## 文件索引

| 文件 | 用途 |
|------|------|
| `CLAUDE.md` | 工具進入點 |
| `docs/GUIDELINES_INDEX.md` | 規範階層與使用情境路由 |
| `docs/ENVIRONMENT.md` | 外部依賴與開發環境狀態 |
| `docs/TRAPS.md` | 完整陷阱參考 |
| `docs/DOCUMENTATION_STANDARD.md` | **文件歸屬規範**（每種文件放哪、命名、生命週期）|
| `docs/DOCUMENTATION_MAP.md` | **文件當前位置地圖**（所有檔案實際位置）|
| `docs/branch-hygiene/` | Branch 維護紀錄（PR #748 建立）|
| `docs/CONSTITUTION.md` | 深度開發憲法（矩陣運算、實證約束、AI Coding 流程）|
| `docs/ITERATION_GATE.md` | 迭代閘門（5 Gate 自我檢查規範）|
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
