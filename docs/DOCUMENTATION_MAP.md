# 文件地圖 (Documentation Map)

> 與 `docs/DOCUMENTATION_STANDARD.md` 配套使用。本文件列出**所有**文檔的當前位置與歸屬邏輯。
> 建立日期：2026-06-26（隨 PR #749 一起建立）

## 根目錄（僅保留治理檔）

| 檔案 | 用途 |
|------|------|
| `README.md` | 專案入口 |
| `AGENTS.md` | AI 路由索引（人類編寫 ≤ 160 行）|
| `CLAUDE.md` | 工具進入點 |
| `CHANGELOG.md` | 版本變更 |
| `LICENSE`, `NOTICE` | 法務 |
| `SECURITY.md` | 資安政策 |
| `CONTRIBUTING.md` | 貢獻指南 |
| `VERSION` | 當前版本 |
| `Dockerfile`, `docker-compose.yml` | 建構配置 |
| `go.mod`, `go.sum` | Go 模組 |
| `.gitignore`, `.gitattributes`, `.editorconfig`, `.golangci.yml` | Git / 工具配置 |

## docs/（規範性 + reference）

### 規範 / 規格 / 設計

- `QUICKSTART.md` — 5 分鐘入門（**單一權威**，禁用副本）
- `GUIDELINES_INDEX.md`, `MATURITY.md` — 規範階層
- `architecture.md`, `DATA_ARCHITECTURE.md`, `events/` — 架構
- `TRAPS.md`, `CONVENTIONS_CHECKLIST.md` — 陷阱與慣例
- `PARAMETER_SYSTEM.md`, `JSON_SCHEMA_STANDARD.md` — 規格

### 操作程序 / Playbook

- `MULTI_CLI_PROTOCOL.md` — 多 CLI 並行協議
- `operations_playbook.md`, `iteration_playbook.md` — 操作 playbook
- `developer_guide.md` — 開發者指南（人類向）
- `guides/ai-productivity.md` — AI 助手指南（AI 向）
- `guides/git-tool-cache-policy.md` — 工具快取原則（GitNexus / opencode 等 derived artifact 不該 commit）
- `data_sources.md` — 各種程序
- `evolution_loop.md` — 演化循環
- `script_usage_guide.md` — 腳本使用指南
- `AI_PROMPT_FILES.md` — AI Prompt 檔案管理政策
- `llm-promotion-evaluation.md` — LLM 功能晉升評估清單

### 參考 / Reference

- `ENVIRONMENT.md` — 外部依賴與開發環境狀態
- `TOOLS.md` — 工具列表與使用時機
- `ai_agent_architecture.md` — AI 代理架構
- `audit-trail.md` — 稽核軌跡
- `calibration-loop.md` — 校準循環
- `industry-ecosystem.md` — 產業生態分析
- `silicon_indicators_coverage.md` — 矽谷指標覆蓋範圍
- `MACRO_CALIBRATION.md` — Rolling 巨集觀校準框架（取代 docs/superpowers/plans/ 已歸檔計畫）

### 數據標準 / Data Standards

- `DATA_CATALOG.md` — 數據目錄
- `DATA_CATALOG_TEMPLATE.md` — 數據目錄模板
- `DATA_DIRECTORY_STANDARD.md` — 數據目錄標準
- `DATA_MATURITY_STANDARD.md` — 數據成熟度標準
- `DATA_NAMING_CONVENTION.md` — 數據命名規則

### 規格 / 設計文檔

- `specs/real-time-regime-detection.md` — 即時 regime 偵測規格
- `llm-integration-strategy-framework.md`, `LLM_INTEGRATION_*` — LLM 框架

### 憲法 / 閘門

- `CONSTITUTION.md` — 深度開發憲法（7 條文：深度定義、第一性原理、程式碼預算、修改邊界、迭代閘門、溝通規範、失敗處理）
- `ITERATION_GATE.md` — 迭代閘門（5 Gate 自我檢查：數學深度、資產通用性、Falsifiability、程式碼預算、回歸測試）

### 審計 / 歸檔

- `audit/` — 審計報告（含原本 docs/audit + 從根目錄移入的 3 個）
- `archive/` — 歸檔（20+ 已歸檔歷史文件，含 superpowers/ 歷史規劃、已完成 wave-10 觀察日誌、migration-0.0.0.5 等）

### 時序敏感文檔（YYYY-MM-DD 前綴）

- `handoff/` — 任務交接
- `investigations/` — 根因調查
- `plans/` — 修復計畫

### 維護 / Hygiene

- `branch-hygiene/2026-06-26-cleanup.md` — PR #748 建立的 hygiene audit

### Wave-specific（active）

- `wave-11/` — Wave 11 L2.3/L2.4 active work（含 L2.4 RUNBOOK）

## .omo/（AI agent ephemeral working dir，**未 git tracked**）

`.omo/` 在 `.gitignore` 排除範圍，**新 clone 不會取得此目錄內容**。此目錄為 AI agent 個人工作區，不該被當作 canonical 規範文件。
完整用途與規範見 `docs/DOCUMENTATION_STANDARD.md` 的 `.omo/` 段。

| 類別 | 範例 | 生命週期 |
|------|------|---------|
| `briefs/` | phase 任務 brief | active → merge 後刪除 |
| `plans/` | 執行 plan | active → merge 後刪除 |
| `evidence/` | 驗證報告 | short-lived |
| `traces/` | sim 執行 JSONL | transient |
| `run-continuation/` | session state | session-only |
| `notepads/`, `workspaces/`, `handoffs/` | 決策/交接/工作區 | transient |
| `phaseN/`, `wave-N*/` | phase/wave 規劃 | merged 後刪除 |
| `maps/` | 自動產生的架構快照 | 需定期重新生成 |
| `boulder.json` | 執行追蹤器 | 短暫 |

**⚠️ 重要**：AGENTS.md 原本引用 `.omo/CONSTITUTION.md` 與 `.omo/ITERATION_GATE.md`，**這是文件斷裂**（新 clone 看不到），已由 PR 修為 `docs/CONSTITUTION.md` 與 `docs/ITERATION_GATE.md`。本目錄未來不應被當作 canonical 來源。

## 動作紀錄

- **2026-06-26** PR #749 套用本標準：
  - 8 個根目錄 .md 移入 `docs/` 子目錄
  - 刪除 `quick_start.md`（重複）
  - 新建 `docs/DOCUMENTATION_STANDARD.md` 與本文件
  - 更新 `AGENTS.md` §「內容歸屬規則」
- **2026-06-26** PR #752 修復文件斷裂：
  - `git mv .omo/CONSTITUTION.md → docs/CONSTITUTION.md`
  - `git mv .omo/ITERATION_GATE.md → docs/ITERATION_GATE.md`
  - `AGENTS.md`：`.omo/CONSTITUTION.md` 引用改為 `docs/CONSTITUTION.md`，新增 `docs/ITERATION_GATE.md`
  - `AGENTS.md`：「設計文件 / 規劃」行移除對 `.omo/briefs/`、`.omo/plans/`、`.omo/evidence/` 的引用
  - `DOCUMENTATION_MAP.md`：撤回 `.omo/` 內部檔案描述，改為說明 `.omo/` 是 ephemeral agent working dir
- **2026-06-26** PR #754 清理 docs/ 內混合的暫時性與未實作檔案：
  - **A 類 archive**（已 RESOLVED）：`docs/llm-trigger-analysis.md` → `docs/archive/2026-06-22-llm-trigger-analysis-RESOLVED.md`；`docs/refactor-611-contract.md` → `docs/archive/2026-06-25-refactor-611-contract.md`
  - **B 類移至 .omo/**（未實作）：`docs/investor-ui/` 整個目錄 → `.omo/investor-ui/`；`docs/live-mode-macro-boundary.md` → `.omo/live-mode-macro-boundary.md`)
- **2026-06-26** docs/ 目錄標準合規審計（PR TBD）：
  - **A 類 archive**：`docs/wave-10-observation-log.md` → `docs/archive/wave-10-observation-log.md`；`docs/migration-0.0.0.5.md` → `docs/archive/migration-0.0.0.5.md`；`docs/superpowers/` 整目錄 → `docs/archive/superpowers/`
  - **B 類移至 .omo/**（active 規劃/設計）：`docs/roadmap.md` → `.omo/briefs/roadmap.md`；`docs/ALERT_SYSTEM_REDESIGN.md` → `.omo/briefs/ALERT_SYSTEM_REDESIGN.md`
  - **C 類刪除重複**：`docs/plans/2026-04-16-*.md` 2 個檔案（已存在於 `docs/archive/plans/`）刪除
