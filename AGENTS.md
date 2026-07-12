# AGENTS.md — atlas-go

> **文件角色**：跨工具 AI 共用指引（OpenCode CLI / Claude Code / Kimi Code / GitHub Copilot）。
> 遵循 [AGENTS.md v1.0.0 規範](https://agents.md)：純 Markdown、無必須欄位。
> 通用規則（語言、ACI、workspace close）見 `~/.config/opencode/AGENTS.md`。
> Claude Code 專屬設定請見 [`CLAUDE.md`](CLAUDE.md)。

## 專案快照

- **Wave**：Wave 11+ + Phase 0-7 UX Redesign
- **最後更新**：2026-07-12
- **對應版本**：v0.0.0.32+
- **語言**：繁體中文（參照全域 `AGENTS.md`）
- **技術棧**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI 強制**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：60%
- **啟動入口**：[`docs/QUICKSTART.md`](docs/QUICKSTART.md)

## 內容歸屬與防膨脹規則

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | [`docs/REFERENCE/TRAPS.md`](docs/REFERENCE/TRAPS.md) |
| 模組內部陷阱/API/流程 | `internal/<mod>/AGENTS.md`（15 個保留位置，見 `internal/AGENTS_INDEX.md`） |
| 模組技術規格 | `docs/specs/<topic>.md` |
| 金融工程 / 操作 playbook | `docs/guides/<topic>.md` |
| 技能 / 子代理指引 | `.claude/skills/atlas-<x>/SKILL.md` |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` |
| 憲法級強制規範 | `docs/REFERENCE/CONSTITUTION.md`、`docs/REFERENCE/ITERATION_GATE.md`、`internal/apigateway/CONSTITUTION.md` |

**防膨脹規則**：本文件上限 **160 行**；達到 **155 行**時觸發警告。超過時優先將內容外移到對應文件，而非持續膨脹本檔。

## 規範與治理

- **開發憲法與流程**：[`docs/REFERENCE/CONSTITUTION.md`](docs/REFERENCE/CONSTITUTION.md)、[`docs/REFERENCE/ITERATION_GATE.md`](docs/REFERENCE/ITERATION_GATE.md)、[`docs/REFERENCE/GUIDELINES_INDEX.md`](docs/REFERENCE/GUIDELINES_INDEX.md)
- **跨模組陷阱**：[`docs/REFERENCE/TRAPS.md`](docs/REFERENCE/TRAPS.md)
- **CI 與編碼守則**：
  - [`.github/instructions/go-core.instructions.md`](.github/instructions/go-core.instructions.md)
  - [`.github/instructions/experiments-guardrails.instructions.md`](.github/instructions/experiments-guardrails.instructions.md)
  - [`.github/instructions/live-trading.guardrails.instructions.md`](.github/instructions/live-trading.guardrails.instructions.md)
- **文件治理**：[`docs/DOCUMENTATION_STANDARD.md`](docs/DOCUMENTATION_STANDARD.md)、[`docs/DOCUMENTATION_MAP.md`](docs/DOCUMENTATION_MAP.md)

## 架構與模組入口

- **分層架構**：[`docs/architecture.md`](docs/architecture.md)
- **參數系統**：[`docs/REFERENCE/PARAMETER_SYSTEM.md`](docs/REFERENCE/PARAMETER_SYSTEM.md)
- **數據源憲法**：[`internal/apigateway/CONSTITUTION.md`](internal/apigateway/CONSTITUTION.md)
- **模組索引**：[`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md)
- **成熟度對照**：[`internal/MATURITY.md`](internal/MATURITY.md)

## ACI 工具與 Agent 操作入口

通用 ACI 規則見全域 `~/.config/opencode/AGENTS.md`。

- **MCP 入口**：[`cmd/atlas-mcp/README.md`](cmd/atlas-mcp/README.md)
- **Tool catalog**：[`docs/REFERENCE/tool-catalog.md`](docs/REFERENCE/tool-catalog.md)
- **Workflow map**：[`docs/REFERENCE/WORKFLOW_MAP.md`](docs/REFERENCE/WORKFLOW_MAP.md)
- **Process 標註**：[`docs/REFERENCE/PROCESSES.yaml`](docs/REFERENCE/PROCESSES.yaml)、[`docs/PROCESS_ANNOTATION_SOP.md`](docs/PROCESS_ANNOTATION_SOP.md)
- **修改程式碼前必跑**：[`.claude/skills/atlas-pre-change-protocol/SKILL.md`](.claude/skills/atlas-pre-change-protocol/SKILL.md)
- **FactorType 變更協議**：[`.claude/skills/atlas-factor-change-protocol/SKILL.md`](.claude/skills/atlas-factor-change-protocol/SKILL.md)

## 高頻陷阱速查

> 完整列表見 [`docs/REFERENCE/TRAPS.md`](docs/REFERENCE/TRAPS.md)。

| 陷阱 | 一句話 |
|------|--------|
| JSON tag snake_case | API parsing struct 必須對齊 `domain.*` 的 snake_case JSON tag |
| Session 日期 | 以 `SessionID` 中的交易日為準，非 `RecordedAt` |
| LLM 路由繞過 | 不可直接呼叫 `clients/*Provider`，須透過 `DefaultRouter` |
| Live 旗標 | 本地測試切勿啟用 `-allow-live-broker` |
| db migration 路徑 | `runMigrations()` 使用 `file://` + 絕對路徑 |
