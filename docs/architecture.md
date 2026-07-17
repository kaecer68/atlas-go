# Architecture

## Product Intent

`atlas-go` is a simulation-first investment research system for Taiwan stocks. The system is built to let OpenClaw run strategy experiments, evaluate agent performance, and iterate prompts or rules without placing real orders.

## L2.3 PoC — LLM-driven sector agents (Wave 11)

L2.3 introduces an opt-in LLM-driven sector agent that drives a `plan → tool_call → reflect` loop via a `SectorAgentLLM` + `DriverAdapter`. Flag-gated behind `UseLLMSectorAgents` (default **off**) so the deterministic sector agents remain the production path until L2.4 observation validates LLM behavior.

**Components**:
- `internal/orchestrator/semiconductor_llm_agent.go` — `SemiconductorLLMAgent` (gate mechanism, deterministic fallback)
- `internal/orchestrator/llm_driver_adapter.go` — `DriverAdapter` (LLM call + response parsing)
- `internal/llm/prompts/{plan,reflect}.go` — Prompt templates with embedded JSON spec
- `internal/llm/test_tools.go` — `TestTools()` (3 mock tools: factor weight, regime, liquidity)
- `internal/orchestrator/sector_agent_llm_test_helpers.go` — `MockLLMDriver` (test-only)

**Documentation**: [`docs/specs/llm-sector-agent.md`](specs/llm-sector-agent.md) (moved from `wave-11/L2_3_PLAN_REFLECT.md`), [`docs/guides/adding-sector-agents.md`](guides/adding-sector-agents.md) (moved from `wave-11/SEMICONDUCTOR_EXECUTOR.md`), [`docs/specs/agent-loop-state-machine.md`](specs/agent-loop-state-machine.md) (moved from `wave-11/AGENT_LOOP_STATE_MACHINE.md`). L2.4 觀察期文件見 [`docs/operations/l2-4-runbook.md`](operations/l2-4-runbook.md) + [`docs/specs/l2-4-observation-spec.md`](specs/l2-4-observation-spec.md)（PR #821 / PR #824 永久化）。

**Tag**: `v0.0.0.21` (PR5b). Plan: `.omo/plans/wave10-l2.3-execution.md`.

## Core Principles

- Simulation before execution
- Audit trail over intuition
- Structured messages between layers
- Replaceable data providers
- Risk controls at the engine layer, not only in prompts

## Layered Agent Model

### Layer 1: Taiwan Market Context

Purpose: determine regime, risk budget, and sector bias.

Initial agents:

- 台灣總經
- 外資流向
- TWD / USD FX
- US Tech Spillover
- Semiconductor Cycle
- Index Breadth
- Futures and Options Sentiment

### Layer 2: Sector Desks

Purpose: select opportunities inside Taiwan-specific sectors.

Initial desks:

- 半導體產業桌
- AI 供應鏈產業桌
- PCB and Thermal
- 金融產業桌
- 航運產業桌
- Consumer and Tourism
- High Dividend and ETF Rotation
- Small Cap Momentum

### Layer 3: Style Filters

Purpose: filter raw ideas using style-specific lenses.

Initial styles:

- 成長動能
- 價值股息
- 獲利品質
- 技術突破
- Chip and Flow Confirmation

### Layer 4: Decision Layer

Purpose: enforce risk, simulate execution, and record final actions.

Decision agents:

- 風控長
- Execution Simulator
- 投資長
- Research Auditor

## Wave 11 投資核心框架（v0.0.0.31）

Layer 1-Layer 4 的決策鏈之上，Wave 11 引入 7 個新模組，圍繞「全球資金流向決定方向，資金勢力共鳴決定強度，事件驅動資金流決定節奏」的投資邏輯：

| 模組 | 角色 | 消費 | 產出 |
|------|------|------|------|
| `internal/strategy_validator` | 策略驗證器 | backtest 引擎 | Sharpe/最大回撤/勝率/TAIEX 相關係數 + 排名分層 |
| `internal/capitalflow` | 資金流向分析 | MacroDataSnapshot (7 勢力原始值) | Z-score + 共振係數 (1.5/0.5) + 品質分數 |
| `internal/eventdriven` | 事件預測 | `industry.EventCalendar` + `capitalflow` 品質分數 | 5 日 forward 預測 + ETF 規模×權重預估 + 營收驚喜 |
| `internal/strategy_ranker` | 策略排名 | `strategy_validator` 輸出 | ranked strategies + tier (public/paid) |
| `internal/subscription` | 使用者訂閱 | — | SQLite users + JWT + 3-tier middleware |
| `internal/recommender` | 推薦分層 | `subscription` user tier + `strategy_ranker` | tier-gated 策略推薦 + 市場燈號 |
| `internal/dailyreport` | 每日報告 | `capitalflow` + `eventdriven` + macro | JSON + Markdown 報告 |

**模組關係圖**（從數據到推薦）：

```text
              ┌──────────────────────────────────────────────────┐
              │ Market Data → MacroDataSnapshot                 │
              │ (TWSE, Fugle, FinMind, Fubon, Yahoo, TAIFEX...) │
              └────────────────────┬─────────────────────────────┘
                                   │
              ┌────────────────────▼─────────────────────────────┐
              │ capitalflow：7 勢力 Z-score + 共振 + 品質分數       │
              └────────────────────┬─────────────────────────────┘
                                   │
              ┌────────────────────▼─────────────────────────────┐
              │ eventdriven：5 日預測 + ETF 預估 + 營收驚喜        │
              └────────────────────┬─────────────────────────────┘
                                   │
              ┌────────────────────▼─────────────────────────────┐
              │ dailyreport：每日 JSON + Markdown 報告             │
              └──────────────────────────────────────────────────┘

              ┌──────────────────────────────────────────────────┐
              │ backtest 引擎 → strategy_validator               │
              └────────────────────┬─────────────────────────────┘
                                   │
              ┌────────────────────▼─────────────────────────────┐
              │ strategy_ranker：排名 + tier 標籤                  │
              └────────────────────┬─────────────────────────────┘
                                   │
              ┌────────────────────▼─────────────────────────────┐
              │ subscription：3-tier + JWT auth + 7 天試用         │
              └────────────────────┬─────────────────────────────┘
                                   │
              ┌────────────────────▼─────────────────────────────┐
              │ recommender：tier-gated 推薦（Phase B 渲染）     │
              └──────────────────────────────────────────────────┘
```

**API 端點**（v0.0.0.31 新增）：
- `/api/capital-flow/{daily,summary}` — `capitalflow`
- `/api/events/{calendar,prediction}` — `eventdriven`
- `/api/recommendations` — `recommender`（需 JWT）
- `/api/reports/{latest,archive,subscribe}` — `dailyreport`
- `/api/auth/{register,login}` + `/api/user/{profile,subscription}` — `subscription`

**前端整合**（Phase A/B/C）：
- `client_web/static/js/services/auth.js` — JWT + tier 解析
- `client_web/static/js/components/home-tier-sections.js` — 依 tier 渲染 dashboard（`renderHomeTierSections()`，4 個 API 平行呼叫）
- `client_web/static/js/page-shells/{login,register,premium,mcp,errors/404}.js` — 認證、MCP 整合、404 fallback

## Data Flow

```text
Market Data -> Layer 1 -> Screener -> Layer 2 -> Layer 3 -> 風控長 -> 投資長 -> Simulation Engine -> Scorecard
```

**Screener** (`internal/screener/`) runs before Layer 2/3 executors generate recommendations. It filters symbols using declarative criteria (P/E, P/B, dividend yield, momentum, volume, and total factor score) so that only qualifying stocks reach the sector desks and style filters.

### 決策鏈透明度（Audit Trail）

系統同時輸出完整的計算過程供人工稽核：

1. **個股因子** → `FactorScores` 含四因子（動能/價值/品質/Agent）與總分，每因子含公式、原始輸入、是否 fallback
2. **信念增減** → `ConvictionBreakdown` 含 base/floor/final 與每步 rule/delta/reason
3. **宏觀事件** → `NarrativeEvent` 含 `confidence_source` 與 `historical_hit_rate`

前端 `client_web/static/index.html` 的「決策鏈」頁面以五層卡片呈現（宏觀→行業→個股篩選→控制→績效），每層皆可展開查看計算明細。共享頁面邏輯位於 `shared_web/static/js/pages/`。

## Modes

### Daily Replay

- Uses daily bars
- End-of-day decision
- Next-session simulated fill
- Best first target for MVP

### Intraday Replay

- Uses minute bars or snapshots
- Event-driven decision points
- Requires more robust data and clock simulation

### Near-Real-Time Paper Trading

- Uses live snapshots or websockets
- No real orders
- Full audit trail required

## Simulation Constraints

- long-only for first release
- per-position max allocation
- portfolio gross exposure cap
- minimum liquidity filter
- transaction cost and slippage model
- cooldown after stop exit

## Autoresearch Loop

The system should eventually:

1. score each agent's recommendations
2. identify weak agents by rolling metrics
3. propose one prompt or rule change
4. replay on unseen periods
5. keep or discard the change by objective performance

## Go Package Intent

- `internal/domain`: canonical types
- `internal/marketdata`: provider abstraction and adapters
- `internal/orchestrator`: layered workflow
- `internal/screener`: declarative stock screening (fundamentals + technicals)
- `internal/portfolio`: Darwinian weights and multi-factor engine
- `internal/sim`: portfolio and execution engine
- `internal/config`: runtime configuration
- `internal/industry`: industry ecosystem analysis (supply chain linkage, seasonal patterns, business cycle compass)
- `internal/narrative`: macro narrative event detection, causal chains, and SeasonalBridge for industry correlation modulation
- `internal/llm`: capability-based multi-provider LLM router with DataClass governance gate (see [LLM Framework](#llm-framework))
- `internal/replay`: historical market data loading and forward return calculation
- `internal/reporting`: Markdown report generation and performance tables
- `cmd/atlas`: entrypoint for local simulation runs
- `cmd/calibrate-seasonal`: CLI for calibrating seasonal patterns from replay data

## LLM Framework

`internal/llm/` 提供 capability-based 多 Provider 路由層，支援 4 層 fallback chain 與 DataClass 治理閘門，定位為**非同步附掛**而非 hot-path 依賴。

### 分層

| 層 | 檔案 | 職責 |
|---|------|------|
| 介面 | `provider.go` | `ProviderImpl` 介面、`Capability`（12 種）、`DataClass`（4 階敏感度）、`RoutingChain` |
| 路由 | `router.go` | `DefaultRouter` 實作 Primary → Backup1 → Backup2 → LastResort；DataClass 閘門（ADR-010）。**Wave 11 L2.1 doc audit（Issue #720）**：實作上等於 3 層 fallback，因為 `defaultRoutingTable()` 與 `configs/llm_router.yaml` 預設把 Backup2 設為空字串。`ProviderOpenCodeGo`/`ProviderOpenCodeZen` 為 `[PLANNED]` 常數，無 client 實作 |
| 設定 | `config.go` | `LoadRouterConfig()` 載入 `configs/llm_router.yaml`；`TryLoadRouterConfig()` 錯誤時 fallback 預設表 |
| 健康 | `health.go` | Provider 健康聚合 + circuit breaker 狀態 |
| 客戶端 | `clients/` | 3 個 Provider HTTP client（DeepSeek V4 / MiniMax M3 / Kimi K2.7）+ 共享 `BaseClient` |
| 能力 | `capabilities/` | 12 個 capability handler（typed payload → Router → typed response） |
| 契約 | `schemas/` | 9 個 typed I/O schema（JSON-serialized, Zod-compatible JSON Schema） |
| 整合 | `adapters/` | Annotator / Router 整合層 |

### 12 個 Capability

`CapabilityFailureAttribution`（strategy.failure_attribution）· `CapabilityCodeReviewAnnotation`（dev.code_review_annotation）· `CapabilityPromptLint`（dev.prompt_lint）· `CapabilityRationaleGeneration`（narrative.rationale_translation_fallback）· `CapabilityStrategySummary`（strategy.frame_summary）· `CapabilityRiskSurfaceExtraction`（spawning.gap_description_enrichment）· `CapabilityRegimeExplanation`（narrative.event_headline）· `CapabilityPerformanceForensics`（risk.confidence_calibration_commentary）· `CapabilityScenarioSimulation`（orchestrator.prism_cohort_insight）· `CapabilitySentimentExplanation`（narrative.sentiment_explanation）· `CapabilityContraAttribution`（Phase 2 擴充）· `CapabilityConfidenceCommentary`（Phase 3.3 非阻塞 bypass）

### 治理閘門（ADR-010）

`MiniMax M3` 對 `DataClass ≥ Regulated` 的請求會在 `router.shouldGateProvider()` 強制 skip（中國管轄資料主權考量）。`ForceProvider` 設為 `ProviderMiniMax` 配合 `DataClass=Regulated` 也會回 `ErrProviderDisabled`，這是**有意設計**，不可繞過。

### 熱路徑護欄

`internal/sim/` 與 `internal/experiment/`（S/E-level）不可 import `internal/llm` 做同步 LLM 呼叫。理由：

- 模擬執行被 LLM 網路延遲拖慢
- replay 可重現性破壞（同一 replay 應得到相同結果）

觀察窗口內，**S/E 模組必須使用 deterministic 預設值**；`SectorAgentLLM.LLM == nil` 時 runner 回 `ErrNotImplemented`，由 `UseLLMSectorAgents` feature flag 控制是否啟用。

### 設定與監控

- **Routing table**：`configs/llm_router.yaml`（runtime 來源），載入失敗 fallback `router.go:defaultRoutingTable()`
- **Effective 3-tier fallback** (Wave 11 L2.1 doc audit, Issue #720): 雖然 `RoutingChain` 結構保留 4 層（Primary/Backup1/Backup2/LastResort），預設 `Backup2` 是空字串，等效於 Primary → Backup1 → LastResort。`ProviderOpenCodeGo`/`ProviderOpenCodeZen` 標記為 `[PLANNED]`，等未來 client 實作後可重用
- **新增 capability 必同步 4 處**：`provider.go` 常數 + `router.go:defaultRoutingTable()` + `config.go:isKnownCapability()` + `configs/llm_router.yaml`
- **健康端點**：`GET /api/llm/health` 暴露 Provider 狀態與 circuit breaker

> 設計權威：`docs/llm-integration-strategy-framework.md` · `internal/llm/AGENTS.md`

## Industry Ecosystem

### Supply Chain Linkage (`internal/industry/linkage.go`)

Models upstream/downstream relationships between Taiwan industries with configurable correlation matrices:
- `CorrelationMatrix` supports three initialization modes: hardcoded defaults, config-driven (parameters.json), and empirical recalculation from returns
- `ShockPropagation` propagates impact through the supply chain with narrative-aware correlation multipliers
- `LinkageAnalyzer` exposes scores, graphs, and shock simulation via `GET /api/industry/linkage`
- Graph topology is defined in `configs/supply_chain_graph.json` (hot-reloadable)
- Narrative themes from `SeasonalBridge` dynamically adjust pairwise correlations

### Seasonal Patterns (`internal/industry/seasonality.go`)

Calendar effect detection and calibration:
- `SeasonalEngine` manages per-industry patterns with start/end months and adjustment factors
- `cmd/calibrate-seasonal` CLI supports synthetic data, replay-based calibration, and automatic parameter update
- Evidence quality badges (`heuristic_awaiting_data` / `low` / `medium` / `high`) displayed in frontend
- `GetAdjustmentBreakdown()` provides four-layer decomposition: seasonal x narrative x cycle x environment

### Business Cycle Compass (`internal/industry/cycle.go`)

Multi-phase industry cycle tracking:
- `CycleTracker` manages five phases (expansion/recovery/mature/recession) with confidence scoring
- `DynamicEnvModulator` ingests macro data (oil, BDI, DXY) for real-time cycle adjustment
- External validators integrate seasonal engine and linkage analyzer for multi-dimensional confidence

---

## Wave 11 投資核心框架（v0.0.0.31 PR #972）

在 L1-L4 決策鏈之上新增**「資金流向 → 事件 → 推薦」**三層投資邏輯框架。設計哲學：

> 全球資金流向**決定方向**、資金勢力共鳴**決定強度**、事件驅動資金流**決定節奏**。

### 三層次關係

| 層次 | 模組 | 輸入 | 輸出 |
|------|------|------|------|
| **流向層**（決定方向） | `internal/capitalflow` | `MacroDataSnapshot` (7 勢力) | Z-score + 共振係數 (1.5/0.5) + 品質分數 |
| **事件層**（決定節奏） | `internal/eventdriven` | `industry.EventCalendar` + 流向品質分數 | 5 日 forward 預測 + ETF 規模預估 + 營收驚喜 |
| **推薦層**（決定強度） | `internal/strategy_validator` + `strategy_ranker` + `recommender` | 回測歷史 + 流向 + 事件 + user tier | 三層策略訊號 (public/registered/premium) |

### API 端點（新增）

| Path | Module | 用途 |
|------|--------|------|
| `GET /api/capital-flow/daily` | `capitalflow` | 七維錢潮雷達（3+2+2 分層）：官方法人 / 行為代理 / 領先＋跨市場訊號；actor 共識只看官方actor 層 — `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 |
| `GET /api/capital-flow/summary` | `capitalflow` | 摘要（品質分數 + 主力方向）|
| `GET /api/events/calendar` | `eventdriven` | 未來 14 天事件 + 預估方向 |
| `GET /api/events/prediction` | `eventdriven` | 5 日 forward 預測 + ETF 預估 |
| `GET /api/recommendations` | `recommender` | 依 user tier 返回分層推薦 |
| `POST /api/auth/{register,login}` | `subscription` | JWT 認證 + 7 天免費試用 |
| `GET /api/user/{profile,subscription}` | `subscription` | 使用者資訊查詢 |
| `GET /api/reports/latest` | `dailyreport` | 最新每日市場報告 (JSON+MD) |
| `GET /api/reports/archive` | `dailyreport` | 歷史報告查詢 |
| `POST /api/reports/subscribe` | `dailyreport` | 郵件訂閱 |

### 前端整合（`client_web/` Phase A0/A/B/C）

| 檔案 | 角色 |
|------|------|
| `services/auth.js` | JWT + tier 解析、`getTier()` 給 Phase B 渲染依據 |
| `page-shells/{login,register,premium}.js` | Phase A0 認證 page shells |
| `page-shells/{mcp,errors/404}.js` | Phase C 整合 + 404 fallback |
| `components/home-tier-sections.js` | Phase B tier-gated dashboard 渲染 |

### Maturity Tag

全部新模組標記為 **experimental**（X-tier），依照內部 SPEC 規範使用 `cmd/gentags/main.go` 自動產生 `field_types.ts` 與 `valid_fields.json`，搭配 CI `field-contract` 強制對齊 frontend/backend 欄位名稱。

