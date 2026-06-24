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
- **語言**：Go 1.25，**DB**：PostgreSQL 15 + Redis 7
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

任何程式碼修改前，強制執行 7 步驟檢查：

```
skill(name="atlas-pre-change-protocol")
```

涵蓋：blast radius → 模組陷阱 → 數據源溯源 → 憲法檢查 → 模式匹配 → GitNexus 架構 → 代碼意圖。

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
| FactorType 變更 | 必須同步 7 個位置，CI 強制 |
| Live 旗標 | 本地測試切勿啟用 `-allow-live-broker` |
| Replay 格式 | JSONL，不是 JSON array |
| 資料可見性 | 通道靜默失敗時,Gateway/Adapter/Service/Frontend 四層須暴露 `data_status` / `failed_channels` / 紅色 badge,不得以零值掩蓋。詳見 `.claude/skills/atlas-data-visibility/SKILL.md` |

## 文件索引

| 文件 | 用途 |
|------|------|
| `CLAUDE.md` | 工具進入點、GitNexus 完整規範（**單一權威來源**） |
| `docs/GUIDELINES_INDEX.md` | 規範階層與使用情境路由（衝突時為最終仲裁者） |
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

## GitNexus 程式碼智慧

> 完整 GitNexus 規範（Always Do / Never Do / Resources / CLI）存放於 **`CLAUDE.md`**。
> 此處僅保留最低限度的路由資訊，避免與 CLAUDE.md 重複造成 token 浪費。

| 使用情境 | 對應工具 |
|---------|---------|
| 修改符號前影響分析 | `gitnexus_impact({target, direction: "upstream"})` |
| 提交前變更驗證 | `gitnexus_detect_changes()` |
| 程式碼探索 | `gitnexus_query({query})` |
| 符號上下文（callers/callees） | `gitnexus_context({name})` |
| 安全重命名 | `gitnexus_rename({symbol_name, new_name})` |

> **強制規則**: 修改任何 function/class/method 前必須執行 `gitnexus_impact`。詳細規範見 `CLAUDE.md`。

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (52662 symbols, 165265 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
