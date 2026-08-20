# AGENTS.md — atlas-go

> **文件角色**：跨工具 AI 共用指引（OpenCode CLI / Claude Code / Kimi Code / GitHub Copilot）。
> 遵循 [AGENTS.md v1.0.0 規範](https://agents.md)：純 Markdown、無必須欄位。
> 通用規則（語言、ACI、workspace close）見 `~/.agents/AGENTS.md`。
> Claude Code 專屬設定請見 [`CLAUDE.md`](CLAUDE.md)。

> 🔧 **部署到 iMac 前必讀**: `~/workspace/a2a-dev/docs/deployment/IMAC-DEPLOY-RUNBOOK.md`（iMac 連線/服務 SOP/工具位置/hermes CLI/常見坑）。部署順序與 SSOT 對位另見 `~/workspace/a2a-dev/docs/deployment/HERMES-ECOSYSTEM.md`。

## 專案快照

- **Wave**：Wave 11+（L2.4 shipped, PRISM active）
- **最後更新**：2026-07-26
- **對應版本**：v0.0.2.0+
- **語言**：繁體中文（參照全域 `AGENTS.md`）
- **技術棧**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI 強制**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：60%
- **啟動入口**：[`docs/quickstart.md`](docs/quickstart.md)
- **雙機治理**：production 在 iMac（`atlas.goluck.uk`）、開發在 MacBook；跨設備規則見 `~/workspace/a2a-dev/docs/governance/雙機治理憲章.md` 與 `~/workspace/a2a-dev/docs/operations/iMac-RUNBOOK.md`

## 內容歸屬與防膨脹規則

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | [`docs/reference/traps.md`](docs/reference/traps.md) |
| PR lifecycle（改 code → ci-full → gh pr create → merge → production 驗收 → done） | [`docs/operations/pr-lifecycle.md`](docs/operations/pr-lifecycle.md) |
| 模組內部陷阱/API/流程 | `internal/<mod>/AGENTS.md`（15 個保留位置，見 `internal/AGENTS_INDEX.md`） |
| 模組技術規格 | `docs/specs/<topic>.md` |
| 金融工程 / 操作 playbook | `docs/guides/<topic>.md` |
| 技能 / 子代理指引 | `.claude/skills/atlas-<x>/SKILL.md` |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` |
| 憲法級強制規範 | `docs/reference/constitution.md`、`docs/reference/iteration-gate.md`、`internal/apigateway/CONSTITUTION.md` |

**防膨脹規則**：本文件上限 **160 行**；達到 **155 行**時觸發警告。超過時優先將內容外移到對應文件，而非持續膨脹本檔。

## 規範與治理

- **產品定位（最高仲裁）**：[`docs/reference/product-positioning.md`](docs/reference/product-positioning.md)（散戶定位、機器自動優先、網頁優先/MCP 為輔、台灣市場前提、資金勢力分類學）
- **開發憲法與流程**：[`docs/reference/constitution.md`](docs/reference/constitution.md)、[`docs/reference/iteration-gate.md`](docs/reference/iteration-gate.md)、[`docs/reference/guidelines-index.md`](docs/reference/guidelines-index.md)
- **跨模組陷阱**：[`docs/reference/traps.md`](docs/reference/traps.md)
- **CI 與編碼守則**：
  - [`.github/instructions/go-core.instructions.md`](.github/instructions/go-core.instructions.md)
  - [`.github/instructions/experiments-guardrails.instructions.md`](.github/instructions/experiments-guardrails.instructions.md)
  - [`.github/instructions/live-trading.guardrails.instructions.md`](.github/instructions/live-trading.guardrails.instructions.md)
- **文件治理：docs/documentation-standard.md、docs/documentation-map.md、.claude/skills/atlas-doc-governance/SKILL.md

## 架構與模組入口

- **分層架構**：[`docs/architecture.md`](docs/architecture.md)
- **參數系統**：[`docs/reference/parameter-system.md`](docs/reference/parameter-system.md)
- **數據源憲法**：[`internal/apigateway/CONSTITUTION.md`](internal/apigateway/CONSTITUTION.md)
- **模組索引**：[`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md)
- **成熟度對照**：[`internal/MATURITY.md`](internal/MATURITY.md)

## ACI 工具與 Agent 操作入口

通用 ACI 規則見全域 `~/.agents/AGENTS.md`。

- **MCP 入口**：[`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md)
- **內部檢查（introspection）**：內部開發 agent 優先透過 atlas-mcp 的 resources/tools（`atlas://tools/catalog`、`atlas://workflows/catalog`、`audit_state`、`mcp_get_*`、`system_get_maturity`）掌握系統結構/工作流/用量，而非僅 grep
- **設定與降級**：MCP 註冊範例與 backend 依賴說明見 [`.claude/skills/atlas-mcp-integration/SKILL.md`](.claude/skills/atlas-mcp-integration/SKILL.md) § 內部開發 agent 使用
- **Tool catalog**：[`docs/reference/tool-catalog.md`](docs/reference/tool-catalog.md) — 所有 MCP tool 的權威清單（117 tools）
- **Workflow map**：[`docs/reference/workflow-map.md`](docs/reference/workflow-map.md)
- **Process 標註**：[`docs/reference/processes.yaml`](docs/reference/processes.yaml)、[`docs/process-annotation-sop.md`](docs/process-annotation-sop.md)
- **修改程式碼前必跑**：[`.claude/skills/atlas-pre-change-protocol/SKILL.md`](.claude/skills/atlas-pre-change-protocol/SKILL.md)
- **除錯 / 審計 / 修復 workflow**：[`.claude/skills/atlas-audit-manifest-protocol/SKILL.md`](.claude/skills/atlas-audit-manifest-protocol/SKILL.md)（審計 → manifest → invariant tracker → commit → PR）
- **FactorType 變更協議**：[`.claude/skills/atlas-factor-change-protocol/SKILL.md`](.claude/skills/atlas-factor-change-protocol/SKILL.md)
- **Multi-CLI 與合併後清理**：[`docs/multi-cli-protocol.md`](docs/multi-cli-protocol.md)（worktree 隔離、PR merge 後自動刪除分支、planning artifacts 清理）
- **分支與 PR 工作流（強制）**：任何程式碼變更 MUST 走分支 → push → PR → merge 流程。禁止直接 push 到 `main`。分支命名慣例：`fix/YYYYMMDD-<desc>` / `feat/YYYYMMDD-<desc>`（日期前綴防止與其他工作區分支混淆）。push 後 MUST 立即執行 `gh pr create`，PR body 必含 Summary / Root Cause / Verification 三段；不可停留在「compare & pull request」未完成狀態。
- **測試同步紀律（強制）**：修改任何 `.go` 檔案的行為時，MUST 在同一個 commit 中更新對應的 `*_test.go`。不可「先 commit 功能，測試晚點補」— 這是 CI 反覆往返的根因。改 code 前先跑受影響的測試確認 baseline，改完立刻跑測試確認紅燈，修正 assertion 後一起 commit。

## ⚠️ ACI-first 強制規範（2026-08-15 定案，防 AI 幻覺）

> **本專案裝有 4 套 ACI 工具，agent 必須優先使用，禁止裸 grep 當主要探索手段。**
> 背景：專案半年來功能齊全但脆弱，主因之一是 agent 不清楚全局就 grep 亂改。

**強制順序（任何「找/查/改」前先走 ACI）**：

| 任務 | 工具 | 範例 |
|---|---|---|
| 全局架構速覽 | codegraph `explore` / gitnexus context | `codegraph explore "capital flow z-score"` |
| 找符號定義/源碼 | codegraph `node` | `codegraph node renderSevenForceBoard` |
| 呼叫鏈/影響面 | codegraph `node` trail / gitnexus impact | `codegraph node ZScoreCalculator` |
| 執行流程 | gitnexus `gitnexus://repo/atlas-go/process/{name}` | 追 execution flow |
| 內部系統狀態 | atlas-mcp（`audit_state`、`system_get_health`、`mcp_get_*`） | 查運行時而非猜 |
| 記憶/決策歷史 | codebase-memory `search_graph` / `trace_path` | `codebase-memory-mcp cli search_graph '{"query":"..."}'` |
| 程式碼語意搜尋 | gitnexus `query` / codegraph `query` | `gitnexus query "admin route"` |

**禁止**：
- ❌ 用 `grep -r` 掃全 repo 找符號（改用 `codegraph node` / `gitnexus query`）
- ❌ 不知道影響面就改碼（先 `codegraph node` 看 Called-by / codegraph 或 gitnexus impact）
- ❌ 不清楚運行狀態就猜（先用 atlas-mcp 查）

**工具就緒檢查**：
- gitnexus: `gitnexus status`（stale 則 `gitnexus analyze`）
- codegraph: `codegraph status`（.codegraph/ 存在即可用）
- atlas-mcp: `bin/atlas-mcp`（backend 需啟動）

## 高頻陷阱速查

> 完整列表見 [`docs/reference/traps.md`](docs/reference/traps.md)。

| 陷阱 | 一句話 |
|------|--------|
| JSON tag snake_case | API parsing struct 必須對齊 `domain.*` 的 snake_case JSON tag |
| Session 日期 | 以 `SessionID` 中的交易日為準，非 `RecordedAt` |
| LLM 路由繞過 | 不可直接呼叫 `clients/*Provider`，須透過 `DefaultRouter` |
| Live 旗標 | 本地測試切勿啟用 `-allow-live-broker` |
| db migration 路徑 | `runMigrations()` 使用 `file://` + 絕對路徑 |
| PR merge 後留分支 | 每次 merge 後必讀 `docs/multi-cli-protocol.md` §Post-merge cleanup，自動刪除遠端與本地 branch |
| Agent 危險操作 | 執行任何改狀態/讀密碼/觸及 production 的指令前，必須先跑 `./agent-guard --check '<command>'` |

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas-go** (28812 symbols, 119801 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

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
