# AGENTS.md — atlas-go

> **文件角色**：跨工具 AI 共用指引（OpenCode CLI / Claude Code / Kimi Code / GitHub Copilot）。
> 遵循 [AGENTS.md v1.0.0 規範](https://agents.md)：純 Markdown、無必須欄位。
> 通用規則（語言、ACI、workspace close）見 `~/.agents/AGENTS.md`。
> Claude Code 專屬設定請見 [`CLAUDE.md`](CLAUDE.md)。

## 專案快照

- **Wave**：Wave 11+（L2.4 shipped, PRISM active）
- **最後更新**：2026-07-26
- **對應版本**：v0.0.2.0+
- **語言**：繁體中文（參照全域 `AGENTS.md`）
- **技術棧**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI 強制**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：60%
- **啟動入口**：[`docs/quickstart.md`](docs/quickstart.md)

## 內容歸屬與防膨脹規則

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | [`docs/reference/traps.md`](docs/reference/traps.md) |
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
- **Tool catalog**：[`docs/reference/tool-catalog.md`](docs/reference/tool-catalog.md) — 所有 MCP tool 的權威清單與數量
- **Workflow map**：[`docs/reference/workflow-map.md`](docs/reference/workflow-map.md)
- **Process 標註**：[`docs/reference/processes.yaml`](docs/reference/processes.yaml)、[`docs/process-annotation-sop.md`](docs/process-annotation-sop.md)
- **修改程式碼前必跑**：[`.claude/skills/atlas-pre-change-protocol/SKILL.md`](.claude/skills/atlas-pre-change-protocol/SKILL.md)
- **除錯 / 審計 / 修復 workflow**：[`.claude/skills/atlas-audit-manifest-protocol/SKILL.md`](.claude/skills/atlas-audit-manifest-protocol/SKILL.md)（審計 → manifest → invariant tracker → commit → PR）
- **FactorType 變更協議**：[`.claude/skills/atlas-factor-change-protocol/SKILL.md`](.claude/skills/atlas-factor-change-protocol/SKILL.md)
- **Multi-CLI 與合併後清理**：[`docs/multi-cli-protocol.md`](docs/multi-cli-protocol.md)（worktree 隔離、PR merge 後自動刪除分支、planning artifacts 清理）
- **分支與 PR 工作流（強制）**：任何程式碼變更 MUST 走分支 → push → PR → merge 流程。禁止直接 push 到 `main`。分支命名慣例：`fix/YYYYMMDD-<desc>` / `feat/YYYYMMDD-<desc>`（日期前綴防止與其他工作區分支混淆）。push 後 MUST 立即執行 `gh pr create`，PR body 必含 Summary / Root Cause / Verification 三段；不可停留在「compare & pull request」未完成狀態。

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
