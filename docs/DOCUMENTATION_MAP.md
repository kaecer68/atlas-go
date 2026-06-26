# 文件地圖 (Documentation Map)

> 與 `docs/DOCUMENTATION_STANDARD.md` 配套使用。本文件提供**所有**文檔的當前位置、歸屬邏輯，與「工作區起步 SOP」。
> 維護者：見下方「動作紀錄」段。
> 最後更新：2026-06-26（PR #756 重構，補上 `.omo/` 完整查找路徑）

## 根目錄（僅治理檔白名單）

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

**禁止根目錄新增任何其他 `.md` 檔案**。

## `docs/` 內容地圖

### 規範 / 憲法 / 核心 reference

| 檔案 | 用途 |
|------|------|
| `QUICKSTART.md` | 5 分鐘入門（**單一權威**）|
| `GUIDELINES_INDEX.md`, `MATURITY.md` | 規範階層、模組成熟度 |
| `CONSTITUTION.md` | 深度開發憲法（7 條文）|
| `ITERATION_GATE.md` | 5 Gate 自我檢查 |
| `DOCUMENTATION_STANDARD.md` | 文件存放規範（本文件的配套）|
| `DOCUMENTATION_MAP.md` | 本文件 |

### 架構 / 領域知識

| 檔案 | 用途 |
|------|------|
| `architecture.md`, `DATA_ARCHITECTURE.md` | 架構 |
| `events/` | 事件規格目錄 |
| `ai_agent_architecture.md` | AI 代理架構 |
| `TRAPS.md`, `CONVENTIONS_CHECKLIST.md` | 陷阱與慣例 |
| `PARAMETER_SYSTEM.md`, `JSON_SCHEMA_STANDARD.md` | 規格 |
| `MACRO_CALIBRATION.md` | Rolling 巨集觀校準框架 |
| `silicon_indicators_coverage.md` | 矽谷指標覆蓋 |
| `industry-ecosystem.md` | 產業生態 |

### Playbook / 指南

| 檔案 | 用途 |
|------|------|
| `operations_playbook.md`, `iteration_playbook.md` | 操作與迭代 |
| `evolution_loop.md` | 演化循環 |
| `data_sources.md` | 資料源說明 |
| `script_usage_guide.md` | 腳本使用 |
| `MULTI_CLI_PROTOCOL.md` | 多 CLI 並行協議 |
| `AI_PROMPT_FILES.md` | AI Prompt 政策 |
| `llm-promotion-evaluation.md` | LLM 功能晉升評估 |
| `developer_guide.md` | 開發者指南（人類向）|
| `guides/ai-productivity.md` | AI 助手指南 |
| `guides/git-tool-cache-policy.md` | 工具快取原則 |
| `guides/new-workspace-startup.md` | 新工作區起步 SOP（AI 必讀）|

### Reference / 工具

| 檔案 | 用途 |
|------|------|
| `ENVIRONMENT.md` | 外部依賴與環境 |
| `TOOLS.md` | 工具清單 |
| `AUDIT_TRAIL.md` | 稽核軌跡 |
| `calibration-loop.md` | 校準循環 |

### 數據標準

| 檔案 | 用途 |
|------|------|
| `DATA_CATALOG.md`, `DATA_CATALOG_TEMPLATE.md` | 數據目錄 |
| `DATA_DIRECTORY_STANDARD.md` | 數據目錄標準 |
| `DATA_MATURITY_STANDARD.md` | 數據成熟度標準 |
| `DATA_NAMING_CONVENTION.md` | 數據命名規則 |

### 規格 / 設計

| 檔案 | 用途 |
|------|------|
| `specs/real-time-regime-detection.md` | 即時 regime 偵測規格 |
| `llm-integration-strategy-framework.md`, `LLM_INTEGRATION_*` | LLM 框架 |

### 審計 / 交接 / 調查 / 修復計畫（時序敏感）

- `audit/` — 審計報告
  - `2026-06-20-risk-orphan-config.md` — `internal/risk` 孤兒組態檔清理紀錄（PR #756 後第二波整理）
- `handoff/` — 任務交接
  - `2026-wave12-llm-annotator-phase2.md` — Wave 12 `llm_annotator` Phase 2 canonical 介面交接（由 `internal/llm_annotator/AGENTS.md` 搬遷）
- `investigations/` — 根因調查
  - `2026-05-29-etf-nav-data-source.md` — ETF NAV 資料來源調查（由 `internal/marketdata/AGENTS.md` 搬遷）
  - `2026-06-fubonproxy-ipv4-uvloop.md` — Fubon proxy IPv4/IPv6 dual-stack 與 uvloop 問題 RCA（由 `internal/marketdata/AGENTS.md` 搬遷）
- `plans/` — 修復計畫
- `branch-hygiene/` — branch 維護紀錄

### Wave-specific（active）

- `wave-11/` — Wave 11 L2.3/L2.4 active work

### 歸檔 `docs/archive/`

**嚴格篩選**——只放對 6 個月後新貢獻者有教學價值的歷史檔案。詳見 `DOCUMENTATION_STANDARD.md` § `docs/archive/` 用途。

當前保留：
- 重大架構演進最終快照（phase2-5 系列）
- 重大決策 audit 報告（experiment-baseline-report、FIN_SKILLS_GAP_ANALYSIS）
- 重大 incident postmortem
- 已 RESOLVED 的規劃（2026-06-22-llm-trigger-analysis-RESOLVED）

已清理（PR #756）：
- `archive/superpowers/`（41 個短期 plan/spec，merge 後無教學價值）
- `archive/wave-10-observation-log.md`（觀察期日誌，過渡性）
- `archive/migration-0.0.0.5.md`（過時 migration，CHANGELOG 已有）

---

## `.claude/skills/` 技能地圖

技能文件是 AI Coding 的標準作業程序延伸。完整分類與載入規則見 **`.claude/SKILLS-MAP.md`**。

| 技能 | 檔案 | 用途 |
|------|------|------|
| Pre-change protocol | `.claude/skills/atlas-pre-change-protocol/SKILL.md` | 修改任何程式碼前強制執行的 7 步檢查 |
| Data visibility | `.claude/skills/atlas-data-visibility/SKILL.md` | 四層資料可見性防護 |
| LLM provider / capability | `.claude/skills/atlas-llm-provider-capability/SKILL.md` | 新增 LLM Provider client 或 Capability handler 的 SOP |
| Fubon supervisor invariants | `.claude/skills/atlas-fubon-supervisor-invariants/SKILL.md` | Fubon proxy ProcessManager 監督器不變式（F1~F9） |
| Factor change protocol | `.claude/skills/atlas-factor-change-protocol/SKILL.md` | FactorType 變更 8 步同步協議 |

---

## `.omo/` 查找地圖

`.omo/` 在 `.gitignore` 排除範圍，**新 clone 不會取得**。本節是給新 AI 工作區**起步時**用的查找指南。

### 工作區起步 SOP（必讀）

```bash
# 1. 讀規範精簡版（必讀）
sed -n '1,80p' docs/DOCUMENTATION_STANDARD.md
sed -n '1,80p' docs/DOCUMENTATION_MAP.md

# 2. 確認 .omo/ 結構合規
ls -la .omo/
# 對照下方白名單，不在白名單的子目錄/檔案都是違規

# 3. 找規劃文件（依用途）
ls .omo/briefs/         # 長壽規劃（roadmap、跨模組設計）
ls .omo/plans/          # 短期 PR 待辦
ls .omo/evidence/       # 驗證報告
ls .omo/handoffs/       # session 交接
ls .omo/notepads/       # 決策筆記
ls .omo/traces/         # sim 輸出
```

### `.omo/` 白名單與生命週期

| 子目錄 | 用途 | 命名規範 | 生命週期 | 範例（若有）|
|--------|------|---------|---------|------------|
| `briefs/` | **長壽** phase 規劃 | `<topic>-brief.md` 或 `<topic>.md`（無日期）| active → 升級到 `docs/` | `roadmap.md`, `ALERT_SYSTEM_REDESIGN.md` |
| `plans/` | **短壽** PR 待辦 | `P<n>-<slug>.md` 或 `YYYY-MM-DD-<slug>.md` | **merge 後必須刪除** | `2026-06-26-llm-router-fix.md` |
| `evidence/` | **短壽** 驗證報告 | `f<n>-<topic>.md` 或 `task-<n>-<topic>.md` | 驗證完即刪 | `f4-scope-fidelity.md` |
| `traces/` | sim JSONL | `sim-YYYYMMDD.jsonl` | 保留最新 5 個 | `sim-20260626.jsonl` |
| `notepads/` | 跨 session 決策筆記 | `<topic>/<file>.md` | 寫滿/過時歸檔或刪 | `decision-chain-evolution-v2/learnings.md` |
| `handoffs/` | session 交接 | `YYYY-MM-DD-<topic>.md` | session 結束即刪 | `2026-06-25-f1-handoff.md` |
| `workspaces/` | 跨 session 工作區 | `<workspace-name>/` | merged 後刪 | `wave-11-l2-4/` |
| `run-continuation/` | session state | `session-<id>.json` | session 結束即刪 | `session-abc123.json` |
| `phaseN/`, `wave-N/` | phase/wave 工作目錄 | `phase<N>/<slug>.md` | merged 後刪 | `phase12/llm-prompt-tuning.md` |
| `boulder.json` | 執行追蹤器 | — | 任務完成即清 | — |
| `maps/` | 自動產生架構快照 | `<topic>-map.md` | 重新生成時覆蓋 | `architecture-map.md` |

### `.omo/` 違規清單（**禁止**）

歷史上 AI 自由生成、PR #756 已清理：

- ❌ `archive/`（無規範的歸檔 → 刪除）
- ❌ `audits/`（複數，與 `docs/audit/` 衝突 → 已清）
- ❌ `client_ui/`、`drafts/`、`investor-ui/`（無命名規範 → 已清）
- ❌ `live-mode-macro-boundary.md`（獨立檔案 → 已清）
- ❌ `session-summary-2026-05-07.md`（應放 `handoffs/` → 已清）
- ❌ `evidence/decision-chain-v2/`（**禁止 evidence 子目錄** → 已清，改用 `evidence/task-N-decision-chain-v2.md`）

**新 AI 若想建立白名單外的新子目錄，必須先問使用者並更新本文件**。

### 工作區結束時的清理檢查

```bash
# Merge PR 前
ls .omo/plans/          # 對應 PR 的 plan 應在 merge 後刪
ls .omo/evidence/       # 對應 PR 的 evidence 應刪

# Session 結束時
rm -rf .omo/handoffs/*  # 已交接或無用的
rm -rf .omo/run-continuation/*  # session state 不需保留

# 定期
du -sh .omo/            # 若超過 100MB 幾乎確定有 stale traces
```

---

## 動作紀錄

### 2026-06-26 PR #756 重構文件規範（本次）

- **重寫 `docs/DOCUMENTATION_STANDARD.md`**：
  - 加入 `.omo/` 完整子目錄白名單 + 命名規範 + 生命週期 SOP
  - 加入「禁止 AI 自由生成新子目錄」機制
  - 加入「docs/ vs .omo/ 判斷準則」流程圖
  - 加入「工作區起步 SOP」
- **重寫 `docs/DOCUMENTATION_MAP.md`**（本文件）：
  - 加入 `.omo/` 完整查找地圖
  - 加入「工作區結束清理檢查」指令
  - 加入 `.omo/` 違規清單（禁止再生成）
- **清理 `docs/archive/` 違規歸檔**：
  - 刪除 `archive/superpowers/`（41 個短期 plan/spec）
  - 刪除 `archive/wave-10-observation-log.md`
  - 刪除 `archive/migration-0.0.0.5.md`
- **清理 `.omo/` 違規子目錄與散落檔案**：
  - 刪除 `archive/`、`audits/`、`client_ui/`、`drafts/`、`investor-ui/`
  - 刪除 `evidence/decision-chain-v2/`（改用 `evidence/task-N-decision-chain-v2.md`）
  - 刪除散落檔 `handoff-ci-fixes.md`、`live-mode-macro-boundary.md`、`session-summary-2026-05-07.md`
- 保留 `.omo/briefs/{roadmap,ALERT_SYSTEM_REDESIGN}.md`（長壽規劃，待新工作區重新驗證可行性）

### 2026-06-26 PR #755 docs/ 標準合規審計

- **A 類 archive**：`docs/wave-10-observation-log.md`、`docs/migration-0.0.0.5.md`、`docs/superpowers/` → `docs/archive/`
- **B 類移至 .omo/**：`docs/roadmap.md`、`docs/ALERT_SYSTEM_REDESIGN.md` → `.omo/briefs/`
- **C 類刪除重複**：`docs/plans/2026-04-16-*.md` 2 個檔案
- 補登 19 個檔案到 `DOCUMENTATION_MAP.md`
- 修復 10 個 broken markdown links

### 2026-06-26 PR #754 docs/ 內混合清理

- A 類 archive：`docs/llm-trigger-analysis.md` → `docs/archive/2026-06-22-llm-trigger-analysis-RESOLVED.md`；`docs/refactor-611-contract.md` → `docs/archive/2026-06-25-refactor-611-contract.md`
- B 類移至 .omo/：`docs/investor-ui/` → `.omo/investor-ui/`；`docs/live-mode-macro-boundary.md` → `.omo/live-mode-macro-boundary.md`

### 2026-06-26 PR #753 .omo/ 使用規則定義

- 在 `docs/DOCUMENTATION_STANDARD.md` 加入 `.omo/` 用途與規範段（+68 行）

### 2026-06-26 PR #752 修復文件斷裂

- `git mv .omo/CONSTITUTION.md → docs/CONSTITUTION.md`
- `git mv .omo/ITERATION_GATE.md → docs/ITERATION_GATE.md`
- 更新 `AGENTS.md` 引用

### 2026-06-26 PR #749 建立文件存放標準

- 8 個根目錄 .md 移入 `docs/` 子目錄
- 刪除 `quick_start.md`（重複）
- 新建 `docs/DOCUMENTATION_STANDARD.md` 與 `docs/DOCUMENTATION_MAP.md`
