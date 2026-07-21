---
name: atlas-mcp-tool-tour
description: "Navigable guide to atlas-go's 80 MCP tools — grouped by task domain, with entry-point tools, companion relationships, and common task combinations."
version: "1.0"
category: "feature"
auto_load: false
load_policy: "manual_only"
created: "2026-07-02"
updated: "2026-07-02"
target_audience: "developer"
---

# Atlas MCP Tool Tour — 110 工具分群導覽

## 描述（Description）

本技能提供 atlas-mcp 的 110 個 MCP tool 的**任務導向分群導覽**。不重複 [`docs/reference/tool-catalog.md`](../../../docs/reference/tool-catalog.md) 的完整 catalog — 本技能聚焦於：
1. **入門 tool**：每個任務群組「第一個該呼叫的 tool」
2. **工具關係**：哪些是 companion（該一起用）、哪些是 alternative（擇一）
3. **常見組合**：daily briefing、risk review、experiment evaluation 等典型任務的 tool 序列

## 何時觸發（When to Trigger）

- 當 agent 說「atlas 有哪些 tool」「我能用 atlas 做什麼」「有哪些 MCP tool 可用」
- 當 agent 接入 atlas-mcp 後，需要快速定位「做 X 任務該用哪個 tool」
- 當 agent 面對 112 個 tool 不知從何開始、需要導覽
- 當 agent 呼叫了一個 tool 但回傳結果不夠、想知道「下一步該用哪個 companion tool」

## 核心概念（Core Concepts）

### 任務群組（Task Domain）
112 個 tool 依任務領域分為 16 個群組（從 26 個 `tools_*.go` 檔案 + `tools.go` 核心 entry-point），每個群組有明確的 **entry-point tool**（第一個該呼叫的）和 **deep-dive tools**（深入查詢用）。

| 群組 | Tool 數 | 入門 tool | 用途 |
|------|--------|----------|------|
| 市場狀態（Macro） | 6 | `macro_get_snapshot_latest` | 總經 snapshot、stress index、資金流 |
| 跨市場（Crossmarket） | 3 | `crossmarket_get_status` | 美股連動、相關性、S&P/NASDAQ/Dow |
| 體制（Regime） | 1 | `regime_get_history` | 市場體制演變（RISK_ON/OFF/NEUTRAL） |
| 敘事（Narrative） | 7 | `narrative_get_bundle` | 因果鏈、敘事事件、季節性、briefing bundle |
| 風險（Risk） | 5 | `risk_get_metrics` | 風險聚合、drawdown、相關矩陣、校準 |
| 策略（Strategy） | 5 | `strategy_list_active` | 線上策略、歸因、摘要、層級 |
| 實驗（Experiment） | 3 | `experiment_diff` | 候選 vs baseline 差異、評審、歷史 |
| 達爾文（Synergy） | 3 | `synergy_get_darwinian_status` | 達爾文權重、趨勢、L2.4 觀察窗口 |
| 警報（Alert） | 4 | `alert_list_unacknowledged` | 未確認警報、統計、規則 |
| 控制平面（Control） | 4 | `control_get_active_overrides` | 覆寫狀態、稽核記錄（皆 read-only） |
| 排程（Scheduler/Task） | 4 | `scheduler_get_status` | 背景排程、任務 CRUD |
| 系統健康（System/Health） | 9 | `system_get_health` | 整體健康、LLM router、資料品質、circuit breaker、anomaly 觀測 |
| 資料源（Data） | 4 | `data_get_channels` | Channel 健康、pipeline 監控 |
| LLM/Trace | 6 | `llm_get_health` | LLM router 健康、cost、推理追蹤 |
| PRISM | 1 | `prism_get_training_results` | 訓練結果 (PRISM cohort) |
| 報告/稅務 | 4 | `report_get_daily_summary` | 績效報告、稅務 snapshot |

### Companion Tool 關係
某些 tool 天然成對使用：

| 先呼叫 | 再呼叫 | 理由 |
|--------|--------|------|
| `system_get_health` | 任何其他 tool | 先確認系統健康再查業務 |
| `regime_get_history` | `crossmarket_get_status` | 體制 + 跨市場 = 完整市場全景 |
| `macro_get_snapshot_latest` | `macro_get_stress_index_current` | snapshot 給 overview，stress index 給深度 |
| `experiment_diff` | `experiment_judge` | 先看差異、再決定是否觸發 LLM 評審 |
| `alert_list_unacknowledged` | `alert_get_stats` | 先看有哪些警報、再看統計分布 |
| `strategy_list_active` | `strategy_get_attribution` | 先看有哪些策略、再看績效歸因 |

## 數據來源（Data Sources）

| 數據 | 模組/檔案 | 說明 |
|------|----------|------|
| Tool 全量定義 | `cmd/atlas-mcp/server/tools_*.go`（26 個檔案）+ `cmd/atlas-mcp/server/tools.go`（核心 entry-point） | 112 個 tool handler |
| 自動描述 | `cmd/atlas-mcp/auto-desc.gen.json`（1146 行） | descgen 生成的 tool description |
| Tool catalog | `docs/reference/tool-catalog.md` | 112 個 tool 的完整清單與決策樹 |
| MCP 規格 | `docs/specs/agent-mcp-server.md` | 設計規格、安全邊界、命名慣例 |

## 實作位置（Implementation Locations）

| 概念 | 檔案路徑 |
|------|---------|
| 全部 tool 註冊 | `cmd/atlas-mcp/server/tools.go` → `registerTools()` |
| Macro tools | `cmd/atlas-mcp/server/tools_macro.go` |
| Risk tools | `cmd/atlas-mcp/server/tools_risk_alert.go` |
| Strategy tools | `cmd/atlas-mcp/server/tools_strategy.go` |
| Experiment tools | `cmd/atlas-mcp/server/tools_experiment.go` |
| Alert tools | `cmd/atlas-mcp/server/tools_risk_alert.go` |

## 使用範例（Usage Examples）

### 範例 1: Daily Briefing（每日投資簡報）

```
1. system_get_health()           → 確認系統健康
2. regime_get_history()          → 取得近期市場體制
3. crossmarket_get_status()      → 跨市場連動狀態
4. macro_get_stress_index_current() → 壓力指數
5. narrative_get_bundle()        → 編譯好的 briefing bundle
6. alert_list_unacknowledged()   → 檢查未確認警報
```

### 範例 2: Risk Review（風險審查）

```
1. risk_get_metrics()            → 風險聚合指標
2. risk_get_drawdown()           → 當前 / 最大 drawdown
3. risk_get_correlation_matrix() → 跨策略相關性
4. strategy_list_active()        → 線上策略清單
5. control_get_active_overrides()→ 檢查是否有 active 覆寫
```

### 範例 3: Experiment Evaluation（實驗評審）

```
1. experiment_history()          → 歷史實驗清單
2. experiment_diff(exp_id)       → 候選 vs baseline 差異
3. experiment_judge(exp_id)      → 觸發 LLM judge（⚠️ 有 side-effect）
4. synergy_get_darwinian_status()→ 達爾文權重狀態
```

## 驗證規則（Validation Rules）

- [ ] 每個任務群組至少知道一個 entry-point tool 的名稱
- [ ] 知道 `system_get_health` 是任何任務的第一個呼叫（健康檢查）
- [ ] 知道 `experiment_judge` 有 side-effect（`destructiveHint=true`），需確認後呼叫
- [ ] 知道 `control_*` 系列全部是 read-only（實際操作需透過 admin HTTP API）

## 相關技能（Related Skills）

| 技能 | 關聯 |
|------|------|
| `atlas-mcp-integration` | 接入指引 — 先接入再導覽 |
| `atlas-risk-management` | 風險相關 tool 的金融背景 |
| `atlas-macro-narrative` | 宏觀敘事 tool 的金融背景 |
| `atlas-strategy-evolution` | 策略/實驗 tool 的金融背景 |

> **完整 catalog + 任務→工具反向索引** — 見 [`docs/reference/tool-catalog.md`](../../../docs/reference/tool-catalog.md)（112 tools × 16 種典型任務矩陣）。

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-07-02 | 初版 — 16 群組分群（108 tools）、入門 tool、companion 關係、3 個任務組合範例 |
| 1.1 | 2026-07-15 | 工具數同步 110（PRISM demote swarm）+ auto-desc.gen.json 1146 行 + registerTools() 名稱 + TokenAuth env 路徑;不變呼叫慣例 |
