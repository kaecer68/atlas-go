# 文件地圖 (Documentation Map)

> **用途**：本文件為 `docs/` 全部文檔的**結構化目錄索引**。與 [`documentation-standard.md`](documentation-standard.md) 配套使用。
> **最後驗證**：2026-08-17（docs/ 治理瘦身：226 → 161 tracked 檔）
> **原則**：`docs/` 路徑均為相對於 repo root 的完整路徑且必須存在；`.omo/` 僅標示私有工作目錄的承接位置，不保證新 clone 中存在。

---

## 根目錄治理檔（白名單）

| 檔案 | 用途 |
|------|------|
| `README.md` | 專案快速入口（~100 行 gateway） |
| `AGENTS.md` | 跨工具 AI 共用指引（80 行，≤ 155 行警告線） |
| `CLAUDE.md` | Claude Code 專屬設定（80 行，精簡路由表） |
| `CHANGELOG.md` | 版本變更記錄 |
| `LICENSE`, `NOTICE` | 法務文件 |
| `SECURITY.md` | 資安政策 |
| `CONTRIBUTING.md` | 貢獻指南 |
| `VERSION` | 當前版本號 |
| `go.mod`, `go.sum` | Go 模組定義 |

> **禁止根目錄新增任何其他 `.md` 檔案**（見 `documentation-standard.md`）。

---

## docs/ 內容地圖（依類別分組）

### 🧹 文件瘦身治理（2026-08-17）

2026-08-17 起，`docs/` 只保留 canonical 知識，tracked 檔由 **226 減至 161**；歷史、調查、manifest 與執行計畫不再放在公開 tree。

| 原公開位置 | 處理 |
|---|---|
| `docs/archive/` | 目錄解散；歷史知識移至 `.omo/audit/` 或提煉為 canonical，其餘 13 檔刪除 |
| docs/investigations/ | 移至 `.omo/investigations/`（私有調查紀錄） |
| docs/work/ | 移至 `.omo/manifests/`（個別 manifest） |
| 根目錄 `ATLAS_SYSTEM_STATE.md`、`llm-promotion-evaluation.md` | 移至 `.omo/audit/` |
| 根目錄 `script-usage-guide.md` | 移至 `.omo/handoffs/` |
| 根目錄 `manifest-constitution-gap-audit.md`、`manifest-constitution-implementation.md`、`manifest-methodology-e4-review.md` | 移至 `.omo/manifests/` |
| docs/specs/dark-launch-tracker-spec.md | 移至 `.omo/evidence/` |
| docs/operations/commercial-flow-scope-2026-07-22.md、docs/operations/mcp-2026-07-28-migration-roadmap.md、docs/operations/l2-4-unblocking-roadmap.md | 移至 `.omo/plans/` |
| docs/operations/l2-4-fault-tolerance-design.md、`production-rollout-runbook.md`、`sprint3-rollout-runbook.md` | 已刪除，不在公開目錄保留 |
| docs/archive/2026-08-12-sector-prediction-runbook.md、docs/archive/2026-08-12-sector-dimension-prediction-spec.md、docs/archive/l2-4-followup.md、docs/archive/l2-4-observation-log.md 等舊流程檔 | 已移除或刪除，不再列為 public runbook/spec／log |

#### 本次 distill 對照

| 原 archive 原檔 | Canonical 目標 |
|---|---|
| docs/archive/2026-06-28-decision-chain-evolution-v2.md | docs/audit-trail.md（決策鏈進化 v2／ConvictionStep 參數溯源） |
| docs/archive/2026-07-21-binary-freshness-protocol.md | docs/developer-guide.md（Binary Freshness 檢查） |
| docs/archive/sector-allocation-closure-rollback-drills.md | docs/operations/sector-allocation-closure-runbook.md（SA11.B Rollback Drills） |

`docs/manifests/` 仍只保留 `README.md` + `TEMPLATE.md`；`docs/contracts/channel-index.json`、`docs/contracts/mcp-tools.contract.json` 與 `docs/swagger.json` 維持 canonical。

### 🏛️ 規範 / 憲法 / 核心 Reference

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/reference/constitution.md` | 深度開發憲法（7 條文） | ✅ |
| `docs/reference/iteration-gate.md` | 5 Gate 自我檢查規範 | ✅ |
| `docs/reference/guidelines-index.md` | 規範階層與使用情境路由 | ✅ |
| `docs/reference/traps.md` | 跨模組陷阱完整參考（單一權威來源） | ✅ |
| `docs/reference/parameter-system.md` | 參數管理系統（禁止硬編碼） | ✅ |
| `docs/quickstart.md` | 5 分鐘入門（單一權威） | ✅ |
| `docs/environment.md` | 外部依賴與環境狀態 | ✅ |
| `docs/tools.md` | 程式碼智慧工具（GitNexus/codebase-memory/codegraph 路由決策樹） | ✅ |
| `docs/documentation-standard.md` | 文件存放規範 | ✅ |
| `docs/documentation-map.md` | 本文件 | ✅ |
| `docs/conventions-checklist.md` | 慣例檢查清單 | ✅ |
| `docs/reference/workflow-map.md` | 42 條 workflow 盤查（WA-001–WA-701） | ✅ |
| `docs/reference/tool-catalog.md` | atlas-mcp 117 tools 完整 catalog | ✅ |

### 🏗️ 架構 / 領域知識

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/architecture.md` | 分層設計原則 | ✅ |
| `docs/data-architecture.md` | 資料架構 | ✅ |
| `docs/ai-agent-architecture.md` | AI 代理架構 | ✅ |
| `docs/industry-ecosystem.md` | 產業生態系統 | ✅ |
| `docs/macro-calibration.md` | Rolling 宏觀校準框架 | ✅ |
| `docs/silicon-indicators-coverage.md` | 矽谷指標覆蓋 | ✅ |
| `docs/llm-integration-strategy-framework.md` | LLM 整合策略框架（主檔） | ✅ |
| `docs/llm-adr-log.md` | LLM 架構決策紀錄（ADR-001~010） | ✅ |
| `docs/calibration-loop.md` | 校準循環 | ✅ |
| `docs/evolution-loop.md` | 演化循環 | ✅ |

### 📖 Playbook / 指南

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/operations-playbook.md` | 操作手冊 | ✅ |
| `docs/iteration-playbook.md` | 迭代指南 | ✅ |
| `docs/data-sources.md` | 資料源說明 | ✅ |
| `docs/developer-guide.md` | 開發者指南（人類向，含 binary freshness 收尾流程） | ✅ |
| `docs/multi-cli-protocol.md` | 多 CLI 並行協議 | ✅ |
| `docs/ai-prompt-files.md` | AI Prompt 政策 | ✅ |
| `docs/design.md` | 設計文件 | ✅ |
| `docs/process-annotation-sop.md` | 如何維護 processes.yaml | ✅ |
| `docs/audit-trail.md` | 三階段決策稽核軌跡，含決策鏈 v2 參數溯源 | ✅ |
| `docs/mcp-integration-local.md` | MCP 本機接入完整指南 | ✅ |
| `docs/guides/adding-sector-agents.md` | 新增 sector agent 指南 | ✅ |
| `docs/guides/ai-productivity.md` | AI 生產力指南 | ✅ |
| `docs/guides/frontend-architecture.md` | 前端架構指南 | ✅ |
| `docs/guides/git-tool-cache-policy.md` | Git 工具快取原則 | ✅ |
| `docs/guides/new-workspace-startup.md` | 新工作區起步 SOP | ✅ |
| `docs/guides/install-and-deploy.md` | 安裝/部署指南 | ✅ |
| `docs/guides/opencode-oh-my-openagent-tuning.md` | OpenCode hook 機制與 token 防護 | ✅ |
| `docs/guides/retail-sentiment.md` | RSI-tw 散戶情緒指數規格 | ✅ |

### 🔧 Operations（維運）

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/operations/README.md` | Operations 目錄入口 | ✅ |
| `docs/operations/version-bumping.md` | Version bump SOP | ✅ |
| `docs/operations/wave9-runbook.md` | Wave 9 Observability 操作手冊 | ✅ |
| `docs/operations/l2-4-runbook.md` | L2.4 觀察期操作手冊 | ✅ |
| `docs/operations/l3-runbook.md` | L3 操作手冊 | ✅ |
| `docs/operations/loki-deployment.md` | Loki 集中式 log 部署 | ✅ |
| `docs/operations/mcp-deploy.md` | MCP 部署指南 | ✅ |
| `docs/operations/local-deploy.md` | 本機 Docker 部署設定（從 CLAUDE.md 移出） | ✅ |
| `docs/operations/cmd-atlas-coverage-policy.md` | cmd/atlas 覆蓋率政策 | ✅ |
| `docs/operations/tier-boundary.md` | Tier 邊界定義 | ✅ |
| `docs/operations/stock-mcp-query-templates.md` | 個股 MCP 查詢範本 | ✅ |
| `docs/operations/sector-allocation-closure-runbook.md` | Sector Allocation Closure 操作手冊，含 SA11.B rollback drills | ✅ |
| `docs/operations/rss-feed-replacement.md` | RSS feed 替換決策記錄 | ✅ |

> **2026-08-17 清理**：移出、刪除與 distill 對照完整列於上方「文件瘦身治理」段。

### 📐 Specs（技術規格）

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/specs/agent-mcp-server-spec.md` | MCP server 規格（canonical spec） | ✅ |
| `docs/specs/agent-mcp-phase3-residual-spec.md` | MCP Phase 3 殘留項目 | ✅ |
| `docs/specs/agent-mcp-phase4-spec.md` | MCP Phase 4 規格 | ✅ |
| `docs/specs/agent-loop-state-machine-spec.md` | AgentLoop 狀態機規格 | ✅ |
| `docs/specs/llm-sector-agent-spec.md` | L2.3 LLM-driven sector agent 設計 | ✅ |
| `docs/specs/llm-routing-spec.md` | LLM Provider 路由策略 | ✅ |
| `docs/specs/llm-interface-contract-spec.md` | LLM 統一介面合約 | ✅ |
| `docs/specs/domain-types-spec.md` | Domain Canonical Types 規格 | ✅ |
| `docs/specs/wave9-observability-spec.md` | Wave 9 Observability 設計規格 | ✅ |
| `docs/specs/l2-4-observation-spec.md` | L2.4 觀察指標 slog schema | ✅ |
| `docs/specs/sim-engine-spec.md` | 模擬引擎 7 步執行序 | ✅ |
| `docs/specs/janus-regime-detection-spec.md` | JANUS meta-layer + Risk Gate | ✅ |
| `docs/specs/prism-cohort-training-spec.md` | PRISM 5-Regime queue | ✅ |
| `docs/specs/reporting-contract-spec.md` | Markdown Reporting Render Contract | ✅ |
| `docs/specs/screener-contract-spec.md` | Screener 篩選管線契約 | ✅ |
| `docs/specs/backtest-pipeline-spec.md` | 歷史回測執行規格 | ✅ |
| `docs/specs/repository-dual-write-spec.md` | PG + JSONL 雙寫持久化規格 | ✅ |
| `docs/specs/taiwan-tax-spec.md` | 台灣稅務計算規格 | ✅ |
| `docs/specs/real-time-regime-detection-spec.md` | 即時 regime 偵測規格 | ✅ |
| `docs/specs/security-audit-spec.md` | 資安審計規格 | ✅ |
| `docs/specs/stock-api-contract-spec.md` | 個股 API 合約 | ✅ |
| `docs/specs/stock-quote-page-spec.md` | 個股快查頁面規格 | ✅ |
| `docs/specs/experimental-feature-launch-gate-spec.md` | 實驗性功能啟動閘門 | ✅ |
| `docs/specs/phase3-5-spec.md` | Phase 3.5 規格 | ✅ |
| `docs/specs/mcp-sdk-api-surface-spec.md` | MCP SDK API surface | ✅ |
| `docs/specs/atlas-mcp-ranker-design-spec.md` | atlas-mcp stock/capitalflow/ranker 設計 | ✅ |
| `docs/specs/dashboard-api-contract-spec.md` | Dashboard API 合約 | ✅ |
| `docs/specs/sector-allocation-simulation-closure-spec.md` | Canonical sector allocation、legacy 遷移、simulation application 與 F06 close-out 契約 | ✅ |
| `docs/specs/guest-mode-spec.md` | Guest mode 規格 | ✅ |

### 📋 數據標準

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/data-catalog.md` | 數據目錄 | ✅ |
| `docs/data-catalog-template.md` | 數據目錄模板 | ✅ |
| `docs/data-directory-standard.md` | 數據目錄標準 | ✅ |
| `docs/data-maturity-standard.md` | 數據成熟度標準 | ✅ |
| `docs/data-naming-convention.md` | 數據命名規則 | ✅ |
| `docs/json-schema-standard.md` | JSON Schema 標準 | ✅ |
| `docs/maturity.md` | 模組成熟度（規範用） | ✅ |

### 📰 reference/events（事件規格）

| 檔案 | 用途 |
|------|------|
| `docs/reference/events/index.md` | 事件目錄入口 |
| `docs/reference/events/backtest-completed.md` | 回測完成事件 |
| `docs/reference/events/calibration-completed.md` | 校準完成事件 |
| `docs/reference/events/channel-individual-health.md` | 通道個別健康事件 |
| `docs/reference/events/drawdown-breach.md` | 回撤突破事件 |
| `docs/reference/events/drift-detector.md` | 漂移偵測事件 |
| `docs/reference/events/experiment-insufficient-data.md` | 實驗資料不足事件 |
| `docs/reference/events/factor-weight-regression.md` | 因子權重回歸事件 |
| `docs/reference/events/health-alert.md` | 健康警報事件 |
| `docs/reference/events/industry-calendar.md` | 產業日曆事件 |
| `docs/reference/events/ingestion-lag-spike.md` | 資料攝取延遲事件 |
| `docs/reference/events/narrative-event.md` | 敘事事件 |
| `docs/reference/events/promotion-recorded.md` | 晉升記錄事件 |
| `docs/reference/events/regime-change-confirmed.md` | Regime 變更確認事件 |
| `docs/reference/events/risk-alert.md` | 風險警報事件 |
| `docs/reference/events/risk-gate-allowed.md` | 風險閘門放行事件 |
| `docs/reference/events/risk-gate-overridden.md` | 風險閘門覆寫事件 |
| `docs/reference/events/risk-gate-rejected.md` | 風險閘門拒絕事件 |
| `docs/reference/events/risk-stoploss-triggered.md` | 停損觸發事件 |
| `docs/reference/events/risk-takeprofit-triggered.md` | 停利觸發事件 |
| `docs/reference/events/sharpe-degradation.md` | Sharpe 惡化事件 |
| `docs/reference/events/trade-slippage.md` | 交易滑價事件 |

### 📦 模組文件

| 檔案 | 用途 | 驗證 |
|------|------|------|
| `docs/modules/README.md` | 模組文件入口 | ✅ |
| `docs/modules/alert-system.md` | 警報系統模組 | ✅ |
| `docs/modules/capital-management.md` | 資金管理模組 | ✅ |
| `docs/modules/factor-engine.md` | 因子引擎模組 | ✅ |
| `docs/modules/screening.md` | 篩選模組 | ✅ |
| `docs/modules/tax.md` | 稅務模組 | ✅ |

### 📂 其他目錄

| 目錄／檔案 | 用途 |
|------|------|
| `docs/manifests/` | Manifest 治理模板；只保留 `README.md` + `TEMPLATE.md` |
| `docs/contracts/` | 契約來源；保留 `channel-index.json` + `mcp-tools.contract.json` |
| `docs/swagger.json` | 對外 API schema |
| `docs/investor/` | 投資人入口 + use cases |
| `.omo/audit/` | 私有歷史稽計；承接 `docs/archive/` 的歷史檔與根目錄審計檔 |
| `.omo/investigations/` | 私有調查紀錄；承接 `docs/investigations/` |
| `.omo/manifests/` | 個別 transient manifest；承接 `docs/work/` 與已移出的根目錄 manifest |
| `.omo/plans/` | 私有短期執行計畫；承接移出的 operations/spec plans |
| `.omo/evidence/` | 私有驗證證據；承接 `dark-launch-tracker-spec.md` 等已移出證據 |
| `.omo/handoffs/` | 私有 session 交接；承接 `script-usage-guide.md` |

---

## 內部模組 AGENTS.md 索引

共 **15 個**保留 AGENTS.md 的 hot-path 模組。完整清單與成熟度見 [`internal/AGENTS_INDEX.md`](../internal/AGENTS_INDEX.md)。

---

## `.claude/skills/` 技能地圖

技能文件是 AI Coding 的 SOP 延伸。完整分類見 **`.claude/SKILLS-MAP.md`**。核心技能清單見 `AGENTS.md` §「程式碼智慧工具」。

---

## `.omo/` 查找地圖

`.omo/` 在 `.gitignore` 排除範圍，新 clone 不會取得；它承接本治理表列出的 audit、investigations、manifests、plans、evidence 與 handoffs 私有產物。結構、生命週期與白名單見 [`documentation-standard.md`](documentation-standard.md) § `.omo/`。

---

## 關聯文件

| 檔案 | 用途 |
|------|------|
| `docs/reference/workflow-map.md` | 42 條 workflow 盤查（WA-001–WA-701） |
| `docs/reference/tool-catalog.md` | atlas-mcp 117 tools 完整 catalog |
| `docs/reference/processes.yaml` | 結構化 workflow metadata |
| `cmd/atlas-mcp/README.md` | MCP 給 agent 的唯一入口 |
| `docs/investor/README.md` | 投資人 5 分鐘入門 |
| `internal/AGENTS_INDEX.md` | 59 模組成熟度索引 |
| `internal/MATURITY.md` | 模組成熟度對照表 |

---

> **維護者**：以 `docs/documentation-standard.md` 為治理規範。最後更新：2026-08-17。
>
> **本次更新**：補記 `docs/` 226→161 tracked 瘦身、已解散目錄、移出／刪除項、私有 `.omo/` 承接位置，以及 3 個 archive distill 的 canonical 對照。
