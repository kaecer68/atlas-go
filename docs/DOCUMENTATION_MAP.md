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
- `data_sources.md`, `migration-0.0.0.5.md` — 各種程序

### 規格 / 設計文檔

- `specs/real-time-regime-detection.md` — 即時 regime 偵測規格
- `llm-integration-strategy-framework.md`, `LLM_INTEGRATION_*` — LLM 框架

### 審計 / 歸檔

- `audit/` — 審計報告（含原本 docs/audit + 從根目錄移入的 3 個）
- `archive/` — 歸檔（已存在 20+ 已歸檔的歷史文件）

### 時序敏感文檔（YYYY-MM-DD 前綴）

- `handoff/` — 任務交接
- `investigations/` — 根因調查
- `plans/` — 修復計畫（與 `.omo/plans/` 不同：omo 是執行 plan，docs 是 repair plan）

### 維護 / Hygiene

- `branch-hygiene/2026-06-26-cleanup.md` — PR #748 建立的 hygiene audit

### Wave-specific（active）

- `wave-10-observation-log.md` — Wave 10 L1+L2 觀察記錄
- `wave-11/` — Wave 11 L2.3/L2.4 active work（含 L2.4 RUNBOOK）

## .omo/（執行性 / iteration-bound）

### Governance

- `CONSTITUTION.md` — Atlas 深度開發憲法
- `ITERATION_GATE.md` — 迭代閘門規則
- `boulder.json` — 執行追蹤

### Iteration artifacts

- `briefs/` — phase 任務 briefs（P0-1、P0-2、P1-1、P2、P3）
- `evidence/` — 驗證證據（F1-F4、task-1..16）
- `plans/` — 規劃（執行 plan）
- `drafts/`, `phase6/`, `workspaces/`, `notepads/`, `run-continuation/` — session 狀態

### Observability

- `traces/` — 模擬執行記錄（45 個 JSONL）
- `maps/` — 架構地圖

### Audit / Handoff

- `audits/experimental-todos-20260526.md` — 實驗待辦（active）
- `handoff-ci-fixes.md`, `session-summary-2026-05-07.md` — 過渡交接
- `wave-8-surface.md`, `wave-8.1-risk-gate-rejected-surface.md` — 已拒絕表面記錄
- `handoffs/` — 交接紀錄

### ⚠️ 待處理（follow-up）

- `.omo/briefs/P0-1_covarianc.md` vs `P0-1_covariance.md` — typo 與命名混淆，前者為中文 task brief，後者為英文 status update。需審查合併或保留兩者但加說明。

## 動作紀錄

- **2026-06-26** PR #749 套用本標準：
  - 8 個根目錄 .md 移入 `docs/` 子目錄
  - 刪除 `quick_start.md`（重複）
  - 新建 `docs/DOCUMENTATION_STANDARD.md` 與本文件
  - 更新 `AGENTS.md` §「內容歸屬規則」