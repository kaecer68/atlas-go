# AGENTS.md — atlas-go

> **文件角色**：跨工具 AI 共用指引（OpenCode CLI / Claude Code / Kimi Code / GitHub Copilot）。
> 遵循 [AGENTS.md v1.0.0 規範](https://agents.md)：純 Markdown、無必須欄位。
> Claude Code 專屬設定（部署、前端架構、token 效率規則）請見 [`CLAUDE.md`](CLAUDE.md)。

## 🌐 語言強制

**所有 AI 回覆必須使用繁體中文**。除非使用者明確要求使用英文，否則禁止使用英文回應。
> 此規則亦載於 `CLAUDE.md`、`.github/copilot-instructions.md`。

## 版本資訊

- **Wave**：Wave 11（L2.3 PoC 已 ship，L2.4 觀察窗口 PLANNED — 見 `.omo/wave-11-l2-4/`）
- **最後更新**：2026-06-28
- **對應版本**：v0.0.0.23+

## 專案概覽

`atlas-go` — 模擬優先、稽核導向的台股投資研究系統。
- **語言**：Go 1.26，**DB**：PostgreSQL 15 + Redis 8
- **CI 強制**：`gofmt` / `go vet` / `staticcheck` / `golangci-lint` / `gosec`
- **覆蓋率門檻**：60%

> 詳細啟動流程、CI 指令、系統初始化順序見 **[`docs/QUICKSTART.md`](docs/QUICKSTART.md)**。

## 📊 模組數量對照

| 數字 | 含義 | 出處 |
|------|------|------|
| **21** | 保留 `AGENTS.md` 的內部模組（hot-path 陷阱寫在裡面） | 本文件原 57→Wave 11 精簡至 21 |
| **45** | 全部模組索引（含無 `AGENTS.md` 者，按成熟度 S/E/X/U 分組） | [`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md) |
| **60** | `internal/` 下全部目錄數 | 檔案系統 `ls internal/*/` |

## 📜 內容歸屬規則

| 知識類型 | 歸屬位置 |
|----------|---------|
| 跨模組全域規則 | 本文件 + `docs/TRAPS.md` |
| 模組內部陷阱/API/流程（hot-path） | `internal/<mod>/AGENTS.md`（**21 個保留模組**） |
| 模組技術規格 | `docs/specs/<topic>.md` |
| 金融工程 / 操作 playbook | `docs/guides/<topic>.md` |
| 技能 / 子代理指引 | `.claude/skills/atlas-<x>/SKILL.md` |
| CI / pipeline 設定 | `.github/workflows/`、`.github/instructions/` |
| 憲法級強制規範 | `docs/CONSTITUTION.md`、`docs/ITERATION_GATE.md`、`internal/apigateway/CONSTITUTION.md` |
| 規範性 / 設計文件 | `docs/`（**不應放 `.omo/`** — `.gitignore` 排除） |

**防膨脹規則**：
- 本文件不超過 **160 行**
- **155 行時觸發警告**，160 行時 PR 被拒絕
- 新知識預設加入 `internal/<mod>/AGENTS.md`（限 21 保留模組）或 `docs/`，**不要**加到這裡

## 🔗 文件路由

### 啟動必讀
- [`docs/QUICKSTART.md`](docs/QUICKSTART.md) — 首次啟動、CI 指令、系統初始化順序

### 規則體系
- [`docs/TRAPS.md`](docs/TRAPS.md) — **跨模組陷阱完整參考**（單一權威來源）
- [`docs/CONSTITUTION.md`](docs/CONSTITUTION.md) — 深度開發憲法
- [`docs/ITERATION_GATE.md`](docs/ITERATION_GATE.md) — 5 Gate 自我檢查規範
- [`docs/GUIDELINES_INDEX.md`](docs/GUIDELINES_INDEX.md) — 規範階層與使用情境路由

### 架構與設計
- [`docs/architecture.md`](docs/architecture.md) — 分層設計原則
- [`docs/PARAMETER_SYSTEM.md`](docs/PARAMETER_SYSTEM.md) — 參數管理系統（禁止硬編碼 magic number）
- [`internal/apigateway/CONSTITUTION.md`](internal/apigateway/CONSTITUTION.md) — 數據源憲法（6 條文 + 3 附錄）

### 模組索引
- [`internal/AGENTS_INDEX.md`](internal/AGENTS_INDEX.md) — 全部模組索引（45 個，按成熟度 S/E/X/U 分組）
- [`internal/MATURITY.md`](internal/MATURITY.md) — 模組成熟度對照
- 修改程式碼前必跑：[`.claude/skills/atlas-pre-change-protocol/SKILL.md`](.claude/skills/atlas-pre-change-protocol/SKILL.md)

### CI 與編碼守則
- [`.github/instructions/go-core.instructions.md`](.github/instructions/go-core.instructions.md) — Go 程式碼守則（全 Go 檔案強制）
- [`.github/instructions/experiments-guardrails.instructions.md`](.github/instructions/experiments-guardrails.instructions.md) — 實驗流程守則
- [`.github/instructions/live-trading.guardrails.instructions.md`](.github/instructions/live-trading.guardrails.instructions.md) — Live trading 守則

### 環境與工具
- [`docs/ENVIRONMENT.md`](docs/ENVIRONMENT.md) — 外部依賴與環境狀態
- [`docs/TOOLS.md`](docs/TOOLS.md) — 完整工具列表（GitNexus / codebase-memory）
- [`CLAUDE.md`](CLAUDE.md) — Claude Code 專屬設定（部署、前端架構、token 效率規則）

### 文件治理
- [`docs/DOCUMENTATION_STANDARD.md`](docs/DOCUMENTATION_STANDARD.md) — 文件歸屬規範與生命週期
- [`docs/DOCUMENTATION_MAP.md`](docs/DOCUMENTATION_MAP.md) — 文件當前位置地圖

## ⚠️ 高頻陷阱速查

> 完整列表 → **[`docs/TRAPS.md`](docs/TRAPS.md)**（單一權威來源）。以下僅列最常觸發的陷阱：

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

## 🔧 程式碼智慧工具（強制規則）

- 修改任何 function/class/method 前，執行 `gitnexus_impact({target, direction:"upstream"})`
- commit 前執行 `gitnexus_detect_changes()`
- impact 回傳 HIGH/CRITICAL 風險時，必須警告使用者並取得確認
- 若 GitNexus 提示 index stale，執行 `npx gitnexus analyze --skip-agents-md`

> 完整工具列表見 **[`docs/TOOLS.md`](docs/TOOLS.md)**；GitNexus 技能見 **[`.claude/skills/gitnexus/`](.claude/skills/gitnexus/)**。
