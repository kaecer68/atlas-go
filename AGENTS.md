# agents.md — atlas-go

> **文件角色**：本文件是 **AI 路由索引**，指向子文件的詳細規範。文章越短、AI 越不會浪費 token。
>
> **AI 須知**：修改 code 前請跑 `skill(name="atlas-pre-change-protocol")`。不要跳過。
>
> **🌐 語言強制**：所有 AI 回覆必須使用**繁體中文**，禁止使用英文。使用者未特別要求英文時，不得以英文回應。

## 📜 內容歸屬規則（ALL AI MUST READ FIRST）

| 知識類型 | 歸屬位置 | 何時讀取 |
|----------|---------|---------|
| 模組內部陷阱/API/流程 | `internal/<mod>/AGENTS.md` | 進入該模組工作時 |
| 操作程序 / playbook | `docs/` | 執行特定操作時 |
| 跨模組全域規則 | 本文件（根 AGENTS.md） | 每個 session 必讀 |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` | CI 失敗時 |
| 憲法級強制規範 | `internal/apigateway/CONSTITUTION.md`、`.omo/CONSTITUTION.md` | 修改架構時 |
| 技能 / 子代理指引 | `.claude/SKILLS-MAP.md` | 任務需要特定技能時 |
| 設計文件 / 規劃 | `.omo/plans/`、`.planning/` | 執行計畫時 |

**防膨脹規則**：
- 本文件不超過 **160 行**（人類編寫部分，不含末尾自動注入區塊）
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

| 目錄 | 角色 | 陷阱參考 |
|------|------|---------|
| `internal/orchestrator/` | SystemCore、PluginHost、三層 executor 路由 | `internal/orchestrator/AGENTS.md` |
| `internal/sim/` | 模擬引擎與部位狀態轉換 | `internal/sim/AGENTS.md` |
| `internal/experiment/` | 實驗執行與評判 | `internal/experiment/AGENTS.md` |
| `internal/baseline/` | Baseline policy 升降級與版本控制 | `internal/baseline/AGENTS.md` |
| `internal/llm/` | LLM capability-based 多 Provider 路由 + DataClass 治理閘門 + 4 層 fallback chain | `internal/llm/AGENTS.md` |
| `internal/ledger/` | JSONL append-only 持久化 | `internal/ledger/AGENTS.md` |
| `internal/portfolio/` | Darwinian 權重、FactorEngine、組合優化 | `internal/portfolio/AGENTS.md` |
| `internal/screener/` | 宣告式個股篩選 | 直接讀碼 |
| `internal/marketdata/` | TWSE / FinMind / Fugle provider abstraction | `internal/marketdata/AGENTS.md` |
| `internal/live/` | 已強化但 production live 需謹慎旗標 | `internal/live/AGENTS.md` |
| `internal/prism/` | Regime-specific 訓練佇列 | `internal/prism/AGENTS.md` |
| `internal/janus/` | 跨 cohort regime 偵測與 PRISM 權重 | `internal/janus/AGENTS.md` |
| `internal/narrative/` | 宏觀敘事、因果鏈、台灣壓力指數 | `internal/narrative/AGENTS.md` |
| `internal/risk/` | RiskManager、VaR、宏觀回撤 | `internal/risk/AGENTS.md` |
| `internal/swarm/` | MiroFish swarm 模擬 | 直接讀碼 |
| `internal/industry/` | 產業分析（輪動、供給需求、季節性、週期） | `internal/industry/AGENTS.md` |
| `internal/monitoring/` | Dashboard API、監控、人工干預入口 | `internal/monitoring/AGENTS.md` |
| `internal/config/` | 環境變數、ParametersConfig（含 `ReportingParameters` 0.0.0.5：`win_rate_threshold`、`sharpe_min_samples`） | 直接讀碼 |
| `internal/db/` | PostgreSQL 連接管理 | 直接讀碼 |
| `internal/eventbus/` | 事件匯流排 | `internal/eventbus/AGENTS.md` |
| `internal/spawning/` | Agent 生成管理 | `internal/spawning/AGENTS.md` |
| `internal/tax/` | 台灣稅務計算 | `internal/tax/AGENTS.md` |
| `internal/realtime/` | 即時資料轉接器 | `internal/realtime/AGENTS.md` |
| `internal/apigateway/` | API Gateway、BackgroundTaskManager | `internal/apigateway/CONSTITUTION.md` |
| `internal/repository/` | PostgreSQL 持久化（DualWriteRepository） | 直接讀碼 |

> 沒有 `AGENTS.md` 的模組為共享基礎設施，直接讀碼即可。
> `superinvestor` 不是獨立 executor，而是 sector/style agent 的角色型別。
>
> **進入 `internal/*/` 目錄修改程式碼前，強制先讀該目錄下的 `AGENTS.md`（或 `CONSTITUTION.md`）。** 模組特有陷阱寫在裡面，跳過會踩坑。

## 修改前必讀

任何程式碼修改前，強制執行 8 步驟檢查：

```
skill(name="atlas-pre-change-protocol")
```

涵蓋：重疊檢查 → blast radius → 模組陷阱 → 數據源溯源 → 憲法檢查 → 模式匹配 → GitNexus 架構 → 代碼意圖。

## 關鍵跨模組陷阱

> 完整陷阱列表 → **`docs/TRAPS.md`**（按模組分類）

| 陷阱 | 一句話 |
|------|--------|
| JSON tag snake_case | API parsing struct 必須對齊 `domain.*` 的 snake_case JSON tag |
| Session 日期 | 以 `SessionID` 中的交易日為準，非 `RecordedAt` |
| GuardOutcomes 對齊 | CIO 輸出必須保留原始 Agent ID |
| OutcomeCount | 不可用 `ledger.LoadOutcomes()` 填寫 |
| 權威來源單一 | 放行/過濾筆數由 `GuardOutcomes` 計算，前端不可各自重算 |
| Constitution 違反 | 不得繞過 BackgroundTaskManager、ParametersConfig、marketdata.Provider |
| 模組成熟度 | 新增 `internal/` 模組必須有 `doc.go` + 更新 `MATURITY.md` |
| FactorType 變更 | 必須同步 8 個位置，CI 強制（for the canonical 8-location list, see `internal/portfolio/AGENTS.md` §12） |
| Live 旗標 | 本地測試切勿啟用 `-allow-live-broker` |
| Replay 格式 | JSONL，不是 JSON array |
| 平行重複實作 | 新增功能前必須用 GitNexus `query` + codebase-memory `semantic_query` 檢查是否已有重疊實作；孤立 code（無 caller）須分類為「未完成/已取代/意外斷連/組態驅動」後才決定去留，不可逕行刪除 |
| 資料可見性 | 通道靜默失敗時,Gateway/Adapter/Service/Frontend 四層須暴露 `data_status` / `failed_channels` / 紅色 badge,不得以零值掩蓋。詳見 `.claude/skills/atlas-data-visibility/SKILL.md` |
| LLM 路由繞過 | 不可直接呼叫 `clients/*Provider` 跳過 `DefaultRouter` | 必須透過 `llm.DefaultRouter.Call()` 或 `capabilities/*Handler`，見 `internal/llm/AGENTS.md` §1 |
| LLM hot-path import | S/E 模組（`internal/sim/`, `internal/experiment/`）不可 import `internal/llm` 直接同步呼叫 | 觀察窗口內用 deterministic 預設值；replay 必須可重現 |

## 文件索引

| 文件 | 用途 |
|------|------|
| `CLAUDE.md` | 工具進入點、GitNexus 完整規範（**單一權威來源**） |
| `docs/GUIDELINES_INDEX.md` | 規範階層與使用情境路由（衝突時為最終仲裁者） |
| `docs/ENVIRONMENT.md` | 外部依賴與開發環境狀態單一真相來源（PR #700） |
| `docs/TRAPS.md` | 完整陷阱參考（跨模組 + 模組特定，按類別分類） |
| `docs/architecture.md` | 系統架構詳細說明 |
| `docs/DATA_ARCHITECTURE.md` | 資料儲存層、讀寫路徑 |
| `.claude/SKILLS-MAP.md` | 技能入口地圖（42 技能，6 大分類） |
| `.claude/skills/atlas-pre-change-protocol/SKILL.md` | 修改前強制 7 步驟檢查清單 |
| `internal/apigateway/CONSTITUTION.md` | 憲法級強制規範 |
| `.omo/CONSTITUTION.md` | 深度憲法（矩陣運算、真實數據、證偽要求） |
| `configs/agents.json` | Agent 註冊表（prompt 映射） |
| `.github/instructions/` | 領域守則（Go 編碼、實驗安全、Live trading） |

---

## 程式碼智慧工具（GitNexus + codebase-memory）

> 兩個 MCP 並行索引 atlas-go（皆已預先索引完成）：
> - **GitNexus**：`atlas-go` — 53,385 symbols / 169,008 edges / 300 個執行流（強調 process + community）
> - **codebase-memory**：`Users-kaecer-workspace-atlas` — 29,757 nodes / 127,367 edges（強調 Cypher 查詢 + Leiden 叢集）
>
> 完整規範見 **`CLAUDE.md`** 與 **`docs/TOOLS.md`**；此處僅保留路由速查表。

### 路由速查表

| 使用情境 | 優先工具 | 備用工具 |
|---------|---------|---------|
| 修改符號前影響分析（**強制**） | `gitnexus_impact({target, direction:"upstream"})` | `codebase-memory_trace_path({direction:"inbound"})` |
| 提交前 blast radius 檢查 | `gitnexus_detect_changes()` | — |
| 程式碼探索（自然語言 → 執行流） | `gitnexus_query({query})` | `codebase-memory_search_graph({query, semantic_query})` |
| 單一符號上下文（callers/callees） | `gitnexus_context({name})` | `codebase-memory_explore({query})` |
| 安全重命名（跨檔案） | `gitnexus_rename({symbol_name, new_name})` | — |
| 複雜度熱點掃描（cyclomatic/cognitive/loop） | `codebase-memory_query_graph({query:"MATCH (f:Function) WHERE ... RETURN ..."})` | — |
| 架構叢集與 Leiden 社群偵測 | `codebase-memory_get_architecture()` | `gitnexus_query({query:"concept"})` |
| 跨倉分析（multi-repo group） | `gitnexus_query({repo:"@<group>"})` | — |
| ADR / 架構決策紀錄管理 | `codebase-memory_manage_adr({mode})` | — |
| API 路由映射 / response shape 比對 | `gitnexus_route_map()` / `gitnexus_shape_check()` | — |

### 互補原則

- **改動前必跑 GitNexus**：`impact` + `detect_changes` 是 PR 安全閘，codebase-memory 無對等指令。
- **分析架構/複雜度時用 codebase-memory**：Cypher 查詢可一次掃所有 hot path；GitNexus 無 query language。
- **重構與跨模組分析用 GitNexus**：`rename` / `group_sync` / `route_map` 是 GitNexus 獨有。
- **遇到語意模糊的查詢**：先 codebase-memory `semantic_query`，再用 GitNexus `query` 收斂到 process。
- **查跨模組執行流必用 GitNexus Process**：atlas 管線化架構（MarketData→Orchestrator→CRO/CIO→Simulator→Ledger）高度依賴 Process 抽象。`trace_path` 只能從已知符號手動逐跳追蹤；`query`/`context` 直接給預計算的完整 step list。詳見 `docs/TOOLS.md`。

> **強制規則**: 修改任何 function/class/method 前必須執行 `gitnexus_impact`。詳細規範見 `CLAUDE.md` 與 `docs/TOOLS.md`。

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (53385 symbols, 169008 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
