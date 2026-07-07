# 文件地圖 (Documentation Map)

> 與 `docs/DOCUMENTATION_STANDARD.md` 配套使用。本文件提供**所有**文檔的當前位置、歸屬邏輯，與「工作區起步 SOP」。
> 維護者：見下方「動作紀錄」段。
> 最後更新：2026-06-28（Phases 1-6: AGENTS.md/CLAUDE.md 重構 + wave-11 解散 + .omo/ 清理）

## 根目錄（僅治理檔白名單）

| 檔案 | 用途 |
|------|------|
| `README.md` | 專案入口 |
| `AGENTS.md` | 跨工具 AI 共用指引（OpenCode/Claude Code/Kimi Code/Copilot）— 文件路由、陷阱速查、強制規則 |
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
| `guides/install-and-deploy.md` | 安裝/部署指南（PR #796 新增）|
| `guides/opencode-oh-my-openagent-tuning.md` | oh-my-openagent hook 機制與 token 防護 |
| `guides/adding-sector-agents.md` | 新增 sector agent 指南（deterministic + LLM-driven）|
| `operations/l2-4-runbook.md` | L2.4 觀察期操作手冊（pre-flight / acceptance / rollback；PR #821 follow-up）|
| `operations/l2-4-followup.md` | L2.4 後續工作報告（auto-cron / CLI flag / promotion 4 步）|

### Reference / 工具

| 檔案 | 用途 |
|------|------|
| `ENVIRONMENT.md` | 外部依賴與環境 |
| `TOOLS.md` | 工具清單 |
| `MCP_INDEX.md` | 外部 AI Agent 透過 MCP 接入 atlas 的入口指南（PR #911 新增） |
| `AGENT_TOOLS.md` | Agent MCP tool 決策樹與完整 catalog（84 tools, task→tool 反向索引） |
| `AGENT_ONBOARDING.md` | 5 分鐘速讀 atlas 全貌（AI agent 入門） |
| `WORKFLOW_MAP.md` | 21 條 workflow 盤查（WA-001–WA-701） |
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
| `specs/agent-loop-state-machine.md` | AgentLoop 狀態機規格（Phase/Transition/Invariants）|
| `specs/llm-sector-agent.md` | L2.3 LLM-driven sector agent 設計記錄 |
| `specs/llm-routing.md` | LLM Provider 路由策略 + 備援鏈（架構藍圖 §6 抽離）|
| `specs/llm-interface-contract.md` | LLM 統一介面合約（架構藍圖 §4.2-4.5 抽離）|
| `specs/domain-types.md` | Domain Canonical Types 規格（Wave 11 Batch 5a 從原 domain 模組的 AGENTS.md 抽離）|
| `specs/wave9-observability.md` | Wave 9 Observability 設計規格(5 偵測器架構、PR-926 metric、PR-931 auth 修復) |
| `specs/sim-engine.md` | 模擬引擎 7 步執行序 + 8 陷阱（Wave 11 Batch 5a）|
| `specs/janus-regime-detection.md` | JANUS meta-layer + Risk Gate 校準（Wave 11 Batch 5a）|
| `specs/prism-cohort-training.md` | PRISM 5-Regime queue + Synthetic flag（Wave 11 Batch 5a）|
| `specs/reporting-contract.md` | Markdown Reporting Render Contract（Wave 11 Batch 5c）|
| `specs/screener-contract.md` | Screener 篩選管線契約（Wave 11 Batch 5c）|
| `specs/backtest-pipeline.md` | 歷史回測執行與自動化排程規格（Wave 11 Batch 5c）|
| `specs/repository-dual-write.md` | PG + JSONL 雙寫持久化規格（Wave 11 Batch 5c）|
| `specs/taiwan-tax.md` | 台灣稅務計算規格（Wave 11 Batch 5c）|
| `llm-integration-strategy-framework.md` | LLM 整合策略框架（主檔，§4.2-4.5/6/8/10 已抽離）|
| `llm-adr-log.md` | LLM 整合架構決策紀錄（ADR-001 ~ ADR-010）|
| `agents-md-audit.md` | **Wave 11 AGENTS.md 整合決策表**（57 → 15 → 21 演化歷程 + 遷移計畫）|
| _（shipped via PR #828,詳見 `operations/l2-4-followup.md`）_ | L2.4 CLI flag 實作：`--use-llm-sector-agents` |
| `specs/l2-4-observation-spec.md` | L2.4 觀察指標 slog schema 規格（per-reco + aggregate metrics；PR #821 永久化）|

### 金融工程 / 操作 playbook

| 檔案 | 用途 |
|------|------|
| `guides/retail-sentiment.md` | RSI-tw 台灣散戶情緒指數規格（Wave 11 Batch 5c 從原 retail 模組的 AGENTS.md 抽離）|
| `operations/wave9-runbook.md` | Wave 9 Observability 操作手冊(5 偵測器啟用、troubleshoot、PR-931 LLM health 修復後的 probe path 規則) |
| `operations/loki-deployment.md` | Loki 集中式 log 部署設計(3 階段:Promtail + Loki + 4 LogQL rules、Alertmanager 整合、自監控 SOP) |

### 審計 / 交接 / 調查 / 修復計畫（時序敏感）

- `audit/` — 審計報告
  - `2026-06-20-risk-orphan-config.md` — `internal/risk` 孤兒組態檔清理紀錄（PR #756 後第二波整理）
- `handoff/` — 任務交接
  - `2026-wave12-llm-annotator-phase2.md` — Wave 12 `llm_annotator` Phase 2 canonical 介面交接（由原 `llm_annotator` 套件的 AGENTS.md 搬遷）
- `investigations/` — 根因調查
  - `2026-05-29-etf-nav-data-source.md` — ETF NAV 資料來源調查（由 `internal/marketdata/AGENTS.md` 搬遷）
  - `2026-06-fubonproxy-ipv4-uvloop.md` — Fubon proxy IPv4/IPv6 dual-stack 與 uvloop 問題 RCA（由 `internal/marketdata/AGENTS.md` 搬遷）
  - `2026-06-28-boot-loop-multi-service.md` — 啟動 5 服務 crash loop 連環根因(prism-worker ENTRYPOINT/command 衝突、env_file vs environment shadow、fubon-neo 公開分發、兩個 .env 模板 stale orphan、PRISM 系統實作不完整、alertmanager/otel-collector config 錯誤)
- `plans/` — 修復計畫與階段性重構方案（`YYYY-MM-DD-<slug>.md`），merge 後保留作為歷史決策紀錄
  - `2026-07-07-wave11-phase3-frontend-contract.md` — Wave 11 Phase 3 前端 API 欄位合約與頁面生命週期對齊計畫（PR #978）
  - `2026-07-07-wave11-phase4-provider-wiring.md` — Wave 11 Phase 4 後端資料提供者串接與 legacy stub 汰換計畫（PR #979）
- `branch-hygiene/` — branch 維護紀錄

### Wave-specific（active）

Wave-11 L2.4 觀察期 implementation 已 ship(PR #821 merged 2026-06-29)。規劃文件已從 `.omo/wave-11-l2-4/` 永久化到 `docs/operations/l2-4-runbook.md` + `docs/specs/l2-4-observation-spec.md` + `docs/operations/l2-4-followup.md`(見此 PR)。Wave 目錄生命週期規則見 `docs/DOCUMENTATION_STANDARD.md` § Wave 工作目錄。

**Wave 9 Observability + Monitoring 補強(2026-07-03)**:`operations/rss-feed-replacement.md` 記錄 PR-930 RSS feed 替換決策(為何換 4 個財經源、為何排除 工商時報)。相關 PR:#926(落地 `atlas_db_init_failures_total` + `atlas_channel_health_errors_total`)、#928(修正 Issue #927 dead metric)、#929(Loki 部署 spec)、#930(RSS 替換)、#931(`/api/llm/health` 401 修復)。

### 歸檔 `docs/archive/`

**嚴格篩選**——只放對 6 個月後新貢獻者有教學價值的歷史檔案。詳見 `DOCUMENTATION_STANDARD.md` § `docs/archive/` 用途。

當前保留：
- 重大架構演進最終快照（phase2-5 系列）
- 重大決策 audit 報告（experiment-baseline-report、FIN_SKILLS_GAP_ANALYSIS）
- 重大 incident postmortem
- 已 RESOLVED 的規劃（2026-06-22-llm-trigger-analysis-RESOLVED）
- 已完成實作計劃（2026-06-28 從 `.omo/plans/` 歸檔）：
  `atlas-architecture-fix`、`css-extraction`、`decision-chain-evolution-v2`、
  `eliminate-engine-dual-source`、`hardcoded-params-config-migration`、
  `industry-ecosystem-fix`、`seasonal-patterns-audit-fix`

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
| `briefs/` | **長壽** phase 規劃 | v1: `<topic>.md`; v2 redesigns: `<topic>-v2.md`(與 v1 並存)| v1 為 source/working; v2 為 refined → 升 `docs/<topic>.md` 後刪 v1+v2 | v1: `roadmap.md`, `ALERT_SYSTEM_REDESIGN.md`; v2: `roadmap-v2.md`, `alert-redesign-v2.md` |
| `plans/` | **短壽** PR 待辦 | `P<n>-<slug>.md` 或 `YYYY-MM-DD-<slug>.md` | **merge 後必須刪除** | `2026-06-26-llm-router-fix.md` |
| `evidence/` | **短壽** 驗證報告 | `f<n>-<topic>.md` 或 `task-<n>-<topic>.md` | 驗證完即刪 | `f4-scope-fidelity.md` |
| `traces/` | sim JSONL | `sim-YYYYMMDD.jsonl` | 保留最新 5 個 | `sim-20260626.jsonl` |
| `notepads/` | 跨 session 決策筆記 | `<topic>/<file>.md` 或 flat `<topic>.md` | 寫滿/過時歸檔或刪 | `decision-chain-evolution-v2/learnings.md`, `frontend-refactor-recovery.md`, `agent-interface-roadmap.md` |
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

### 2026-07-07 docs/plans/ governance fix（本次）

- **觸發**:User 發現 PR #971–#975 校正工作的規劃文件被置於 `docs/plans/`，且 `docs/plans/` 在 `DOCUMENTATION_MAP.md:261` 已記載「整個移除」但實際仍存在，導致文件歸屬規範與真實目錄 drift，並讓 AI agent 反覆將短期計畫放錯位置。

- **修正內容**:
  - `git rm -r docs/plans/`，刪除 `2026-07-07-wave11-phase3-frontend-contract.md` 與 `2026-07-07-wave11-phase4-provider-wiring.md`（已實作完畢，git reflog 可恢復）。
  - `git rm -r docs/superpowers/`，刪除 3 個 Wave 11 校正工作的執行計畫（`2026-07-07-wave11-cleanup-master.md`、`2026-07-07-wave11-phase1-auth-routing.md`、`2026-07-07-wave11-phase2-mcp-tools.md`）；該目錄本應在 2026-07-03 cleanup 移除卻遺留。
  - `docs/DOCUMENTATION_STANDARD.md`: 從 `docs/` 子目錄結構與文件歸屬對照表中移除 `docs/plans/`；將「短期 PR 待辦」與「修復計畫」統一歸入 `.omo/plans/`；更新「單數 vs 複數」例外說明。
  - `AGENTS.md`: 將「規範性 / 設計文件」列明確區分為「穩定 reference → `docs/`」與「短期 PR 計畫 → `.omo/plans/`」，避免 agent 泛化解讀。
  - `docs/archive/2026-04-16-dashboard-screening-sync.md`: 歷史引用該修復計畫文件（檔名：2026-04-16-dashboard-screening-sync.md）加註「歷史路徑，現行規範已移至 `.omo/plans/`」。
  - `docs/MACRO_CALIBRATION.md`: 移除對已刪除 Rolling 校準計畫書的引用。
  - `docs/archive/ui_optimization_plan.md`: 將對原始 UI 修復計畫書的引用標註為歷史路徑。
  - Phase 3.5 runbook stub: 因內容為 DRAFT/placeholder，從 `docs/operations/` 移動至 `.omo/plans/phase3-5-runbook-stub.md`（gitignored 工作區文件）。

- **審計發現（中輕度問題留待後續 PR）**: `docs/modules/`、`docs/research/`、`docs/spikes/`、`docs/INCIDENTS/` 未在 STANDARD/MAP 定義用途；`docs/operations/*-execution-plan.md` 命名與內容性質（post-verification 報告）不符；`AGENTS.md` 157 行已觸發 155 行警告線。

### 2026-07-03 docs cleanup（本次）

- **觸發**:User 評估 `docs/{briefs,branch-hygiene,investigations,plans}` 對 STANDARD + MAP 合規性 + 3 新任務(.omo/.ocx/.opencode gitignore + cleanup 規範一致性 + audit_assets/ 整併)。

- **docs/ 4 dirs cleanup** (14 verifications evidence-based):
  - `docs/briefs/` 整個移除 (3 v2 → `.omo/briefs/*-v2.md` 與 v1 並存)
  - `docs/plans/` 整個移除 (l2-4 PR #828 已 merge + roadmap → `.omo/notepads/`)
  - `docs/branch-hygiene/` 縮為 1 檔 (6 PNG 0 引用 4.2MB 釋放 + recovery.md → `.omo/notepads/`)
  - `docs/investigations/` 不動 (4 檔全 OK)
  - quick fixes:investigations L154 dangling link + main.go L2.4 註解 + GUIDELINES_INDEX/MAP 表格更新
  - 7 個 dangling refs 修復 (.claude/skills/, docs/operations/, docs/spikes/, .omo/notepads/)

- **v1/v2 關鍵發現**:PR #756 刻意保留 v1 sources 於 `.omo/briefs/` 作為「長壽規劃」(見本節 2026-06-26 PR #756 紀錄)。v2 redesigns 加 `-v2.md` 後綴與 v1 並存,保留演化關係;IMPL-9 DRY 驗證 DUP-A~G 全部 verdict,內容去重+一致性驗證協議完成。

- **Index sync**:`GUIDELINES_INDEX.md` L161 `手寫技能文件 | 10` → `20` (實際 .claude/skills/* = 20 dirs: 16 atlas-* + codebase-memory + codegraph + gitnexus + robot-communication);`MAP.md` L209/213 範例更新;本 entry。

- **audit_assets/ 整併**(歷史紀錄,Phase 2 投資人介面審計 13428B,2026-07-01):原 `audit_assets/` 為 gitignored ROOT dir,違反 STANDARD 原則,當時移入 `docs/audit/`。**該檔已於 PR #970 隨 mode 系統移除時一併刪除**。

- **gitignore 修正**:`.gitignore` L68 `!/.ocx/` (whitelist 逆邏輯) → `/.ocx/` (untrack `receipt.jsonc` 3447B);`.omo/handoffs/` mkdir + 收容 2 散落檔 (`handoff-ci-fixes.md` 5090B + `session-summary-2026-05-07.md` 2936B),解本節「❌ session-summary-2026-05-07.md (應放 handoffs/ → 已清)」紀錄與事實不符的違規。

- **cleanup 規範一致性**:`docs/branch-hygiene/2026-06-26-cleanup.md` (歷史記錄,PR #756 刪 16 stale branch) vs `docs/guides/git-tool-cache-policy.md` (canonical guide) **scope 不同,無衝突**;後者「已知清單」+ `git cleanup-tools` alias 補 `.ocx/receipt.jsonc` + `.omo/` 散落檔條目。

- **PR**:見 PR 編號

### 2026-06-29 SECURITY.md refresh（本次）

- **首次建立**：2026-05-17（commit `8b6da3d`，v0.0.0.18 之前）
- **P0 事實修正**：
  - `Known Limitations` > PostgreSQL sslmode：從 `sslmode=disable` 改為 `sslmode=prefer`（`docker-compose.yml` 10 個位置皆為 `prefer`），並補上「production 應使用 `verify-full` + CA」指引
  - `Known Limitations` > Grafana 密碼：移除「default admin password」警告（`docker-compose.yml` L207-208 已改用 `GF_SECURITY_ADMIN_USER` / `GF_SECURITY_ADMIN_PASSWORD` env-driven）
- **P1 新增章節**：
  - `Data Source Governance`（6 條文 + 3 附錄的 `internal/apigateway/CONSTITUTION.md`）
  - `Live Trading Guardrails`（`.github/instructions/live-trading.guardrails.instructions.md` + `-allow-live-broker` 旗標預設關閉）
  - `LLM Providers` 子段（`LLM_DEEPSEEK_API_KEY` / `LLM_MINIMAX_API_KEY` / `LLM_ANNOTATOR_API_KEY` + 5 個 capability flags）
  - `Data Visibility Safeguards`（4 層 gateway/adapter/service/UI，引用 `.claude/skills/atlas-data-visibility/SKILL.md`）
- **AGENTS.md 補強**：於「高頻陷阱速查」表新增「資安設定」一列
- **Oracle 審計**：APPROVE-WITH-MINOR-EDITS，10/10 事實查核通過，2 個 P2 套用（typo + Issue #719 引用）
- **PR**：見 PR 編號

### 2026-06-27 v3 ACP 遷移（本次）

- **ACP 全面接管 context 壓縮**：安裝 `opencode-acp@1.4.1`（DCP hardened fork +37 bug fixes）
- **關閉 oh-my-openagent 衝突設定**：`preemptive_compaction: false`、`dynamic_context_pruning.enabled: false`
- **關閉 opencode 原生 auto-compaction**：`compaction.auto: false`
- **重寫 `docs/guides/opencode-oh-my-openagent-tuning.md`**：從 DCP 配置指南轉為 ACP 方案
- **基準測試**：SQLite baseline（`.omo/evidence/2026-06-27-dcp-tuning-baseline.md`）

### 2026-06-27 Wave 11 AGENTS.md 整合（PR #779-787，本次）

- **AGENTS.md 從 57 → 21 個**（-63%）：原始 `internal/<mod>/AGENTS.md` 50 個，保留 21 個（hot-path 護欄）
- **Batch 1** (PR #780)：純刪除 5 個 D 類空殼（adversarial、importer、reflexivity、stress、taskexec）
- **Batch 2** (PR #781)：純刪除 1 個 B 類 deprecated 套件（llm_annotator，內容已遷移至 `internal/llm_annotator/doc.go`）
- **Batch 3** (PR #782)：合併 2 個 A 類至 `doc.go`（eval、feature）
- **Batch 4** (PR #783)：遷移 5 個邊界 C 類（bootstrap→QUICKSTART、globalmarket/storage→doc.go、spawning/metalearning→strategy-evolution skill）
- **Batch 5a** (PR #784)：遷移 4 個 C 類至 `docs/specs/`（domain、sim、janus、prism）
- **Batch 5b** (PR #786)：合併 5 個 X-tier 模組 AGENTS.md 至 `doc.go`（ml、replay、robustness、scheduler、swarm）
- **Batch 5c** (PR #787)：遷移 7 個 C 類至 `docs/specs/` 或 `docs/guides/`（retail/reporting/screener/autobacktest/backtest/repository/tax）
- **Batch 6** (PR #788)：更新根 `AGENTS.md` 模組路由表（21 個）+ `DOCUMENTATION_MAP.md` 補登 12 個新檔

### 2026-06-26 PR #756 重構文件規範

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

- **A 類 archive**：將 wave-10-observation-log、migration-0.0.0.5、superpowers/ 等搬遷至 `docs/archive/`
- **B 類移至 .omo/**：將 roadmap、ALERT_SYSTEM_REDESIGN 等長壽規劃移至 `.omo/briefs/`
- **C 類刪除重複**：`docs/plans/2026-04-16-*.md` 2 個檔案
- 補登 19 個檔案到 `DOCUMENTATION_MAP.md`
- 修復 10 個 broken markdown links

### 2026-06-26 PR #754 docs/ 內混合清理

- A 類 archive：llm-trigger-analysis → `docs/archive/2026-06-22-llm-trigger-analysis-RESOLVED.md`；refactor-611-contract → `docs/archive/2026-06-25-refactor-611-contract.md`
- B 類移至 .omo/：investor-ui/ → `.omo/investor-ui/`；live-mode-macro-boundary.md → `.omo/live-mode-macro-boundary.md`

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

### 2026-06-28 Phases 1-6: AGENTS.md/CLAUDE.md 文檔體系重構

- Phase 1: 解散 `docs/wave-11/` — L2.4 規劃移至 `.omo/wave-11-l2-4/`，生命週期規則吸收至 `DOCUMENTATION_STANDARD.md`，更新 8 處外部引用，修正 `parameters.go` 過時註解
- Phase 2: `AGENTS.md` 依 v1.0.0 規範重構 — 新增版本資訊、模組對照表、`.github/instructions/` 路由，陷阱表從 14 條濃縮為 top 7，移除 21 模組清單（改指向 `AGENTS_INDEX.md`）
- Phase 3: `CLAUDE.md` 重構 — 新增 `@AGENTS.md` 匯入，語言強制指向 AGENTS.md，路由表從 10 條減至 3 條 Claude 專屬條目，移除過時 "34 個" 模組數字
- Phase 4: `.omo/plans/` 清理 — 7 個已完成實作計劃移至 `docs/archive/`，保留 8 個（2 個未實作、1 個指引模式、1 個進行中、4 個近期修復）
- Phase 5: `.github/copilot-instructions.md` 精簡 — 移除與 AGENTS.md 重疊的 Workflows/Gotchas/Core Files 表
- Phase 6: `DOCUMENTATION_MAP.md` 更新 — 反映 Phase 1-5 所有變更，新增 archive 條目與動作紀錄
