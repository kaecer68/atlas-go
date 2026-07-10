# AGENTS.md — atlas-go

> **文件角色**：跨工具 AI 共用指引（OpenCode CLI / Claude Code / Kimi Code / GitHub Copilot）。
> 遵循 [AGENTS.md v1.0.0 規範](https://agents.md)：純 Markdown、無必須欄位。
> Claude Code 專屬設定（部署、前端架構、token 效率規則）請見 [`CLAUDE.md`](CLAUDE.md)。

## 🌐 語言強制

**所有 AI 回覆必須使用繁體中文**；除非使用者明確要求使用英文，否則禁止使用英文回應（亦載於 `CLAUDE.md`、`.github/copilot-instructions.md`）。

## 版本資訊

- **Wave**：Wave 11+ + Phase 0-7 UX Redesign（IA + 首頁層級 + Onboarding + Design System 全部 ship via PR #945–#950+）
- **最後更新**：2026-07-06
- **對應版本**：v0.0.0.32+

## 專案概覽

`atlas-go` — 模擬優先、稽核導向的台股投資研究系統。
- **語言**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI 強制**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：60%

> 詳細啟動流程、CI 指令、系統初始化順序見 **[`docs/QUICKSTART.md`](docs/QUICKSTART.md)**。

## 📊 模組數量對照

| 數字 | 含義 | 出處 |
|------|------|------|
| **27** | 保留 `AGENTS.md` 的內部模組（hot-path 陷阱寫在裡面） | 完整清單見 [`internal/AGENTS_INDEX.md § 保留模組 AGENTS.md`](internal/AGENTS_INDEX.md) |
| **59** | 全部模組索引（含無 `AGENTS.md` 者，按成熟度 S/E/X/U 分組） | [`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md)（v0.0.0.32 +7: `capitalflow`、`eventdriven`、`strategy_ranker`、`strategy_validator`、`subscription`、`recommender`、`dailyreport`） |
| **73** | `internal/` 下全部模組目錄數（不含 testdata/testutil） | 檔案系統 `ls internal/*/` |

## 📜 內容歸屬規則

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | 本文件 + `docs/REFERENCE/TRAPS.md` |
| 模組內部陷阱/API/流程（hot-path） | `internal/<mod>/AGENTS.md`（**27 個保留模組**，清單見 `internal/AGENTS_INDEX.md`） |
| 模組技術規格 | `docs/specs/<topic>.md` |
| 金融工程 / 操作 playbook | `docs/guides/<topic>.md` |
| 技能 / 子代理指引 | `.claude/skills/atlas-<x>/SKILL.md` |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` |
| 憲法級強制規範 | `docs/REFERENCE/CONSTITUTION.md`、`docs/REFERENCE/ITERATION_GATE.md`、`internal/apigateway/CONSTITUTION.md` |
| 規範性 / 設計文件 / 穩定 reference | `docs/`（**不應放 `.omo/`**）；**短期 PR 計畫 → `.omo/plans/`** |

**防膨脹規則**：
- 本文件不超過 **160 行**
- **155 行時觸發警告**，160 行時 PR 被拒絕
- 新知識預設加入 `internal/<mod>/AGENTS.md`（限 27 保留模組）或 `docs/`，**不要**加到這裡

## 🔗 文件路由

### Agent 入門（外部 AI 優先讀）
- [`docs/INVESTOR/README.md`](docs/INVESTOR/README.md) — 5 分鐘速讀 atlas 全貌
- [`docs/REFERENCE/tool-catalog.md`](docs/REFERENCE/tool-catalog.md) — **89–91** 個 tool 完整 catalog
- [`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md) — **MCP 給 agent 的唯一入口**，部署/配置/89–91 tool 總覽
- [`.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md`](.claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md) — 50 行 MCP 設定 SOP（Hermes/OpenClaw/Claude/Cursor/OpenCode 5 種 client）

### 啟動必讀
- [`docs/QUICKSTART.md`](docs/QUICKSTART.md) — 首次啟動、CI 指令、系統初始化順序

### 規則體系
- [`docs/REFERENCE/TRAPS.md`](docs/REFERENCE/TRAPS.md) — **跨模組陷阱完整參考**（單一權威來源）
- [`docs/REFERENCE/CONSTITUTION.md`](docs/REFERENCE/CONSTITUTION.md) — 深度開發憲法
- [`docs/REFERENCE/ITERATION_GATE.md`](docs/REFERENCE/ITERATION_GATE.md) — 5 Gate 自我檢查規範
- [`docs/REFERENCE/GUIDELINES_INDEX.md`](docs/REFERENCE/GUIDELINES_INDEX.md) — 規範階層與使用情境路由

### 架構與設計
- [`docs/architecture.md`](docs/architecture.md) — 分層設計原則
- [`docs/REFERENCE/PARAMETER_SYSTEM.md`](docs/REFERENCE/PARAMETER_SYSTEM.md) — 參數管理系統（禁止硬編碼 magic number）
- [`internal/apigateway/CONSTITUTION.md`](internal/apigateway/CONSTITUTION.md) — 數據源憲法（6 條文 + 3 附錄）

### 模組索引
- [`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md) — 全部模組索引（59 個，按成熟度 S/E/X/U 分組）
- [`internal/MATURITY.md`](internal/MATURITY.md) — 模組成熟度對照
- 修改程式碼前必跑：[`.claude/skills/atlas-pre-change-protocol/SKILL.md`](.claude/skills/atlas-pre-change-protocol/SKILL.md)

### CI 與編碼守則
- [`.github/instructions/go-core.instructions.md`](.github/instructions/go-core.instructions.md) — Go 程式碼守則（全 Go 檔案強制）
- [`.github/instructions/experiments-guardrails.instructions.md`](.github/instructions/experiments-guardrails.instructions.md) — 實驗流程守則
- [`.github/instructions/live-trading.guardrails.instructions.md`](.github/instructions/live-trading.guardrails.instructions.md) — Live trading 守則

### 環境與工具
- [`docs/ENVIRONMENT.md`](docs/ENVIRONMENT.md) — 外部依賴與環境狀態
- [`docs/TOOLS.md`](docs/TOOLS.md) — 程式碼智慧工具（GitNexus / codebase-memory / codegraph 路由決策樹）
- [`docs/REFERENCE/tool-catalog.md`](docs/REFERENCE/tool-catalog.md) — **atlas-mcp 業務工具**（市場查詢、風險、策略操作 — 與 TOOLS.md 用途不同）
- [`docs/operations/version-bumping.md`](docs/operations/version-bumping.md) — **Version bump SOP**（AI coding release 必走 `make bump-version` + `make sync-version` + `make ci` 三步）
- [`CLAUDE.md`](CLAUDE.md) — Claude Code 專屬設定（部署、前端架構、token 效率規則）

### 文件治理
- [`docs/DOCUMENTATION_STANDARD.md`](docs/DOCUMENTATION_STANDARD.md) — 文件歸屬規範與生命週期
- [`docs/DOCUMENTATION_MAP.md`](docs/DOCUMENTATION_MAP.md) — 文件當前位置地圖

## 🤖 Agent Interface（AI Agent 操作入口）

atlas-go 從「人類 web UI 為主」升級為「人機雙軌」。AI Agent 透過 **MCP protocol** 操作 atlas：

| 層級 | 入口 | 文件 |
|------|------|------|
| Roadmap (規劃藍圖) | `docs/specs/agent-mcp-server.md` | canonical spec + 開放議題(roadmap v2 內容已併入此 spec) |
| MCP 接入（外部 agent） | `[`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md) + [skill](.claude/skills/atlas-mcp-integration/SKILL.md) | 部署配置、Claude/Cursor/OpenCode 設定 |
| Tool 導覽 | `[`docs/REFERENCE/tool-catalog.md`](docs/REFERENCE/tool-catalog.md) + [skill](.claude/skills/atlas-mcp-tool-tour/SKILL.md) | 89–91 tools 決策樹與完整 catalog |
| Workflow Map | `docs/REFERENCE/WORKFLOW_MAP.md` | 21 條 workflow 盤查（WA-001–WA-701） |
| Process 標註 | `docs/REFERENCE/PROCESSES.yaml` | 結構化 workflow metadata |
| MCP Server 規格 | `docs/specs/agent-mcp-server.md` | 84+ tools、auth、audit、JSON Schema 模板 |
| MCP 模組陷阱 | `[`cmd/atlas-mcp/server/AGENTS.md`](cmd/atlas-mcp/server/AGENTS.md) | 66 .go 檔案的 hot-path 陷阱（34 非測試 + 32 測試,命名/相依/稽核/認證） |
| 標註 SOP | `docs/PROCESS_ANNOTATION_SOP.md` | 如何維護 PROCESSES.yaml |
| Onboarding | `docs/INVESTOR/README.md` | 5 分鐘上手 |

**狀態**：P0 已完成（PROCESSES.yaml + AGENTS.md 本章節）；P0 補遺（2026-07-03）增加 2 個 atlas-mcp-* skill、MCP 模組陷阱、README 更新、AGENT_TOOLS 補完；SSE/streamable-HTTP transport + audit log retention 已 ship（Phase 4, PR #807 / PR #1064）；P1 殘留：atlas-mcp onboarding 改進；P2 殘留：binary merge、retention period、license、WebSocket。

## ⚠️ 高頻陷阱速查

> 完整列表 → **[`docs/REFERENCE/TRAPS.md`](docs/REFERENCE/TRAPS.md)**（單一權威來源）。以下僅列最常觸發的陷阱：

| 陷阱 | 一句話 |
|------|--------|
| JSON tag snake_case | API parsing struct 必須對齊 `domain.*` 的 snake_case JSON tag |
| Session 日期 | 以 `SessionID` 中的交易日為準，非 `RecordedAt` |
| Constitution 違反 | 不得繞過 BackgroundTaskManager、ParametersConfig、marketdata.Provider |
| FactorType 變更 | 必須同步 8 個位置，見 `.claude/skills/atlas-factor-change-protocol/SKILL.md` |
| LLM 路由繞過 | 不可直接呼叫 `clients/*Provider`，須透過 `DefaultRouter` |
| Live 旗標 | 本地測試切勿啟用 `-allow-live-broker` |
| 資安設定 | 修改 security 相關配置（API key、sslmode、live broker、data source channel）前必看 [SECURITY.md](SECURITY.md) 與 [internal/apigateway/CONSTITUTION.md](internal/apigateway/CONSTITUTION.md) |
| 平行重複實作 | 新增功能前用 GitNexus `query` + codebase-memory 檢查重疊 |
| **LLM health 401** | `/api/llm/health` 必須**同步**加到 `handler.go authFreeExactPaths` + `main.go isPublicPath`，只改一處 rebuild 後仍 401。見 `docs/REFERENCE/TRAPS.md` 對應 entry 與 PR #931。 |
| **Prometheus metric 命名空間** | 新 metric 必須 `atlas_<feature>_<measurement>_total` 格式，無前綴的舊名（如 `channel_errors_total`）會與 Prometheus default metric 衝突。見 PR #926 + Issue #927。 |
| **校準 Artifact 遺留** | `parameters.json` + `*.snapshot.bak` 為背景校準任務的執行結果。`.snapshot.bak` 已 gitignore；`parameters.json` 應 commit。見 `docs/REFERENCE/CONSTITUTION.md` §第八條。 |
| **AI-Generated Doc 當 gospel** | `followup.md` / `docs/specs/*.md` / `docs/operations/*.md` 等為 AI agent 產出，非 human owner hard rule。衝突時：① 讀 doc ② 讀 code ③ 標記 doc 過時 + 修 code/doc。完整協議見 `docs/REFERENCE/TRAPS.md`。 |

## 🔧 程式碼智慧工具（強制規則）

atlas-go 有三套互補的程式碼智慧工具，各司其職。詳細能力對照與路由決策樹見 **[`docs/TOOLS.md`](docs/TOOLS.md)**。

### GitNexus（改動風險評估 + Process 抽象，**改 code 前必用**）
- 修改任何 function/class/method 前，執行 `gitnexus_impact({target, direction:"upstream"})`
- commit 前執行 `gitnexus_detect_changes()`
- impact 回傳 HIGH/CRITICAL 風險時，必須警告使用者並取得確認
- 若 GitNexus 提示 index stale，執行 `npx gitnexus analyze --skip-agents-md`

### codebase-memory（深度圖分析 + 語意搜尋，**補充 GitNexus**）
- 新增功能前，用 `codebase-memory_search_graph({semantic_query:[...]})` 檢查是否已有語意相似的實作
- 複雜度熱點掃描、跨服務資料流追蹤、ADR 管理 → 用 codebase-memory 的 Cypher / `trace_path` / `manage_adr`
- Fork 強化版（`codebase-memory-mcp-pro`）提供 `explore()` 和 `detect_changes({depth:N})` 作為輕量替代
### codegraph（輕量快速源碼探索，**快速瀏覽用**）
- 用 `codegraph_explore()` 快速理解源碼 + 呼叫路徑（單次 call，Read-equivalent）；動態分派 hop 追蹤（callbacks、React re-render）是其獨有強項
- 與 codebase-memory 重疊的功能（單次源碼查詢），**優先使用 codebase-memory**

> GitNexus 技能見 **[`.claude/skills/gitnexus/`](.claude/skills/gitnexus/)**。  
> codegraph 官方 docs：[github.com/colbymchenry/codegraph](https://github.com/colbymchenry/codegraph)。
