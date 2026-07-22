# Atlas-go Workflow Map（For AI Agents）

> **狀態**: v2（post-#821）— Wave 11（2026-07-22）
> **Audience**: AI Agent（Claude Code / Codex / Cursor / OpenCode 等）需要理解 atlas-go 運作的全貌時
> **產生方式**: Stage 1 骨架盤查（從 `cmd/atlas/main.go` + `internal/orchestrator/` + `internal/eventbus/` + `docs/specs/` + `internal/scheduler/` + `internal/marketdata/` 入口往外推導）
> **下一步**: v3 由 Stage 3 deep dive 補完
> **權威來源**: 本文件編號 `WA-XXX`（W = Workflow，A = Atlas）。v2 後將成為 agent-facing 唯一參照入口

---

## 1. 5 分鐘速覽

atlas-go 是**模擬優先、稽核導向的台股投資研究系統**。進入 HTTP（`:18080`）前必須先理解三層次：

- **入口層**：`cmd/atlas/main.go` 提供 5 個 run mode（不是單一 monolith）。
- **內部基礎設施**：所有跨工作流程通訊都透過 `internal/eventbus`（publish/subscribe）+ `internal/orchestrator/system_plugins.go`（plugin host）。
- **執行模型**：所有「會產生 recommendation 的 workflow」最終都進入 `SectorAgent` 介面，分成 **deterministic executor**（如 `SemiconductorExecutor`）與 **LLM-driven Plan→ToolCall→Reflect loop**（如 `SectorAgentLLM`，見 `internal/orchestrator/agent_loop.go`）。

沒有「單一條永遠執行的 workflow」。系統是**事件驅動 + 多插件掛載**。

---

## 2. 入口 Run Mode（5 個）

從 `cmd/atlas/main.go:run()` 推導：

| Mode | Flag | 用途 | Bootstrap 依賴 |
|------|------|------|--------------|
| **WA-001 API 服務** | `--api`（預設 :18080） | 啟 Dashboard API server + Gateway + Plugin chain + Fubon-proxy | 完整（Janus, MaturityTracker, fubon-proxy, Gateway） |
| **WA-002 Live Trading** | `--live` | broker mode（dry-run/paper/live）+ 即時 orchestrator | 完整 + broker config |
| **WA-003 Simulation** | `--simulate` | 一次性 daily simulation，結束後 exit | 簡化（runtime + janus） |
| **WA-004 Build Universe** | `--build-universe {run\|map\|scrape\|status}` | SmartUniverseBuilder pipeline，exit | 輕量（只跑 universe 子系統） |
| **WA-005 Prism Worker** | subcommand `prism worker` | lightweight daemon（PRISM cohort training 子任務） | 輕量（不需要完整 runtime） |

**共同前置**：每個 run mode 都會執行 `(1) OTel init → (2) -check-integrity exit → (3) ensurePostgres() → (4) ApplyBrokerConfig → (5) InitRuntime → (6) cleanup 24h+ stale gateway alerts`。

`--simulate` 與 `--build-universe` 與 `prism worker` 在 init 流程完成前/後提早 dispatch，以避免誤配置的 DB 阻塞 sub-command 啟動。

---

## 3. Workflow 候選清單（42 條）

下列由 `internal/eventbus` 的 subscribe 點 + `internal/orchestrator/` 的 plugin chain + `internal/scheduler/` 的 **6 個 task（13 個 .go 檔含測試 + doc.go）** + `internal/marketdata/` 的 channel adapter 推導。每條列出已確認的入口與**已標記的缺口**（給 Stage 2 補完）。

### 3.1 資料與市場層

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-100** | Market Data Ingestion | `internal/marketdata/provider.go` + **56 個 provider 檔**（marketdata/ 目錄共 121 個 .go 檔） | cron + Gateway channel adapter 啟動時 | 各 channel snapshot（TWSE/TPEX/Fugle/Yahoo/Fubon 等）→ Redis cache + PostgreSQL | 各 provider 的更新頻率、限流策略 |
| **WA-101** | Realtime Regime Detection | `internal/marketdata/realtime/`（realtime flag） | websocket 推播（需要 `-allow-realtime` flag） | 即時 regime 訊號 | ⚠️ flag 預設關閉 |
| **WA-102** | US Market Refresh | `internal/apigateway/us_market_refresh.go:NewUSMarketRefreshTask` | BackgroundTaskFunc 排程 | 美股指數 / 個股資料刷新 | 排程頻率未明 |
| **WA-103** | Ingestion Lag Monitor | `NewIngestionLagMonitor`（訂閱 eventbus） | event-driven（每個 ingestion event） | 落後指標 event | threshold / debounce 邏輯 |

### 3.2 體制與智能層

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-200** | Regime Detection (JANUS) | `internal/janus`（`janus.NewEngine()` 於 main.go:271） | API 啟動時呼叫 `EnsureAllRegimes` + `Update` | RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL | 體制分類細則 |
| **WA-201** | Macro Ingestion Pipeline | `internal/macro_pipeline`（GitNexus 顯示有 integration test） | daily macro cron | MacroDataSnapshot | 排程時點 |
| **WA-202** | Narrative Conviction Modulator | `internal/orchestrator/narrative_conviction_modulator.go`（L2.4 觀察期功能） | planreflect loop event | conviction 調整 | 是否仍在 L2.4 觀察 |

### 3.3 決策與執行層

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-300** | Deterministic Executor（例：Semiconductor） | `internal/orchestrator/semiconductor_llm_agent.go:SemiconductorLLMAgent.Supports` | per-symbol 排程 | Recommendation | 若改 LLM 模式，`--use-llm-sector-agents` 細節 |
| **WA-301** | LLM Sector Agent Loop（Plan→ToolCall→Reflect） | `internal/orchestrator/agent_loop.go:AgentLoop` + `agent_loop_state_machine.md` specs | per-symbol + LLM enable flag | 最終 conviction | Round 計算細則（Issue #711 #6） |
| **WA-302** | Multi-Strategy Allocator | `internal/strategy/registry.go:Register/Get/List/ListByRegime` | orchestrator 整合 | strategy 選擇 | allocator 演算法 |
| **WA-303** | Conviction Builder | `internal/orchestrator/conviction_builder.go` | recommendation event | 最終 conviction score | 因子加權如何 |

### 3.4 風險與控制層

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-400** | Risk Gate（Pre/In/Post Trade） | `internal/risk/gate.go:RiskGate` | 每筆訂單 | RiskDecision（Phase / Verdict / Mode / Action） | 4 種 mode 切換時機 |
| **WA-401** | Mode 變更發布 | `RiskGate.SetMode()` | 內部觸發 | RiskDecision event → eventbus | 變更時的 audit 路徑 |
| **WA-402** | Approval Workflow | `internal/risk/approval_workflow.go:ApprovalWorkflow` | RiskDecision 需人工 review | ApprovalRequest | 對接的 audit consumer |
| **WA-403** | Maturity Tracker | `internal/domain.MaturityTracker` | API 啟動時載入 | Mode（BurnIn/Calibrating/FullAuto） | metrics 計算邏輯 |

### 3.5 校準與演化層

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-500** | Conviction Calibration | `cmd/atlas/calibration_tasks.go:registerCalibrationCycle` + `internal/scheduler/auto_calibration.go` | daily 排程 + `CycleCalibration:internal/industry/cycle_calibration.go` | CalibrationTask report | 校正演算法 |
| **WA-501** | PRISM Cohort Training | `cmd/atlas` 中的 `prism worker` subcommand + `internal/prism` | 獨立 daemon | cohort 訓練結果 | train trigger 機制 |
| **WA-502** | Strategy Evolution / Agent Spawn | `internal/scheduler/strategy_evolution.go` + `internal/spawning/agent_factory.go` | 定期檢查 | 突變 / 淘汰 / 新生 | 接受閾值（docs/reference/parameter-system.md） |
| **WA-503** | ML Retrain | `internal/scheduler/ml_retrain.go` | 排程 | ML model 重訓 | 觸發條件 |
| **WA-504** | Auto Rollback | `internal/scheduler/auto_rollback.go` | strategy 表現下降偵測 | 退回 baseline | 判斷條件 |
| **WA-505** | Seasonal Task | `internal/scheduler/seasonal_task.go` | season-bound cron | 季節性校正 | 細則 |

### 3.6 監控與維運層

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-600** | Stale Alert Cleanup | `cmd/atlas/main.go:run()` 啟動時 | 啟動時 | 24h+ gateway alert 清理 | 完整 cleanup 規則 |
| **WA-601** | Drift Detection | `NewDriftDetector[WithTargets]`（eventbus subscriber） | event-driven | drift alert | 三種 detection 演算法差異 |
| **WA-602** | Factor Weight Regression | `NewFactorWeightRegressionDetector` | event-driven | 回歸警示 | 因子 weight 反應 |
| **WA-603** | Channel Health Synthesizer | `NewChannelHealthSynthesizer` | event-driven | 各 channel 綜合健康度 | 從何事件綜合 |
| **WA-604** | System Health Check | `internal/scheduler/system_health.go` | cron | 系統整體健康度 | 排程頻率 |
| **WA-605** | L2.4 Observation Window | `docs/operations/l2-4-runbook.md` | `--use-llm-sector-agents=true` + `LLM_SECTOR_AGENTS_ENABLED=true` | slog event 觀察（**已 ship — PR #821, 2026-06-29**） | observation metrics 計算細則 |
| **WA-606** | Drawdown Breach Consumer | `NewDrawdownConsumer` | event-driven | 觸發停損決策 | 對接的 risk gate |

### 3.7 系統內部 sub-daemon

| ID | 工作流程 | 入口位置 | 觸發條件 | 主要產出 | 缺口 |
|----|---------|---------|---------|---------|------|
| **WA-700** | Fubon-proxy Manager（process supervisor） | `.claude/skills/atlas-fubon-supervisor-invariants` skill + `cmd/atlas/main.go:299` | API 啟動時 | python proxy 子行程 | F1-F9 invariants 細則 |
| **WA-701** | OTel / observability | `obsotel.Init`（main.go:166） | 啟動時 | distributed trace | sampling policy |
| **WA-800** | 資金流向日報 | `internal/capitalflow/handler.go` | HTTP `GET /api/capital-flow/daily` | 7 勢力 Z-score + 共振 | Phase B tier-gated dashboard |
| **WA-801** | 資金流向摘要 | `internal/capitalflow/handler.go` | HTTP `GET /api/capital-flow/summary` | 共振 + 品質分數 + 主力方向 | Phase B 摘要渲染 |
| **WA-801.1** | 資金流向歷史 | `internal/capitalflow/handler.go` | HTTP `GET /api/capital-flow/history?days=N` | 多日滾動樣本時序（max 60） | PR #1228 CL-1 新增；CF-INV-15/16/17 |
| **WA-802** | 事件日曆 | `internal/eventdriven/handler.go` | HTTP `GET /api/events/calendar` | 未來 14 天事件 + 預估方向 | Phase B 近期事件區塊 |
| **WA-803** | 5 日事件預測 | `internal/eventdriven/handler.go` | HTTP `GET /api/events/prediction` | 5 日 forward + ETF 預估 | Phase B 預測區塊 |
| **WA-804** | 推薦分層 | `internal/recommender/handler.go` | HTTP `GET /api/recommendations`（需 JWT） | tier-gated 策略推薦 + 市場燈號 | Phase B 推薦區塊 |
| **WA-805** | 訂閱認證 | `internal/subscription/handler.go` | HTTP `POST /api/auth/{register,login}` | JWT token + cookie | Phase A0 frontend auth |
| **WA-806** | 每日報告 | `internal/dailyreport/report.go` | HTTP `GET /api/reports/latest\|archive` + `POST /api/reports/subscribe` | 報告 JSON + Markdown | Phase B 報告區塊 |

---

## 4. 跨工作流程基礎設施（Cross-cutting）

### 4.1 Event Bus 通道圖

`internal/eventbus` 集中所有跨 workflow 通訊。已識別的 subscribers 集中在 `NewProductionSystemWithEventBus` 註冊：

```
                          ┌────────────────────────────┐
                          │   internal/eventbus        │
                          │   (Subscribe/Publish)      │
                          └────────────┬───────────────┘
                                       │
        ┌──────────────────────────────┼──────────────────────────────┐
        │                              │                              │
   ┌────▼─────┐                 ┌──────▼──────┐                ┌──────▼──────┐
   │ RiskGate │                 │ Janus Engine │                │ Orchestrator│
   │ publish  │ ◄──────────────┤ (regime)    ├──────────────► │ plugins     │
   │ (WA-400) │                 │ (WA-200)    │                │ (host)      │
   └──────────┘                 └─────────────┘                └─────────────┘
                                       │
        ┌──────────────────────────────┼─────────────────────────────────────┐
        │                              │                                     │
   ┌────▼─────┐                  ┌─────▼──────┐                       ┌──────▼──────┐
   │ Schedule │                  │ Calibration│                       │  Audit      │
   │ Periodic │ ◄────────────────┤   Cycle    ├───────────────────────►│  Trail      │
   │ Tasks    │                  │ (WA-500)   │                       │             │
   └──────────┘                  └────────────┘                       └─────────────┘
```

Subscribers（**12 個已確認**，從 `internal/monitoring/service/` + `internal/orchestrator/factory.go` 的 `NewProductionSystemWithEventBus` 註冊點推導）：

- **Monitoring service 訂閱者（7）**：`ChannelHealthSynthesizer`、`DriftDetector`、`FactorWeightRegressionDetector`、`IngestionLagMonitor`、`RegimeDebouncer`、`TradeSlippageConsumer`、`Trigger`（`TradeSlippageConsumer` 在 `internal/monitoring/trade_slippage_consumer.go`，先前版本未列入）
- **Orchestrator/system 訂閱者（5）**：`MaturityTracker`、`TrainingQueue`、`PRISMManager`、`PreTradeGate`、`DrawdownConsumer`（於 `internal/orchestrator/factory.go` 註冊）

> **Stage 2 補完候選**：每個 subscriber 的具體事件過濾條件、`AllowOnly` policy、bus 衝突時的優先級 — 詳見 `internal/orchestrator/factory.go:NewProductionSystemWithEventBus`。

### 4.2 Plugin Host 生命週期

`internal/orchestrator/plugin_host.go:PluginHost` 管理 chain：

```go
Register(p Plugin, core *SystemCore)
// 觸發 methods:
AttachAll(core)              // 插件初始化掛載
ProcessRecommendations(...)   // 處理每筆 recommendation
PostSimulation(...)           // 每輪 simulation 結束
```

已確認 plugin（依 `internal/orchestrator/system_plugins.go` 的 `With*` 方法，共 7 個）：
`janusPlugin` (WithJANUS)、`prismPlugin` (WithPRISM)、`spawningPlugin` (WithSpawning)、`phase3Plugin` (WithPhase3Controller)、`strategyTechniquesPlugin` (WithStrategyTechniques)、`llmSectorAgentsPlugin` (WithLLMSectorAgents)。

> **命名慣例**：原始碼使用 lowerCamelCase（小寫開頭）。文件中也沿用 lowerCamelCase，與 struct 引用一致。

### 4.3 Strategy Registry

`internal/strategy/registry.go:83`：
- `Register(name, factory)` / `Get(name)` / `List()` / `ListByRegime(regime)`
- 註冊的策略是 dynamic — 啟動時依參數決定載入哪些。

### 4.4 Risk Gate Modes

| Mode | 觸發 | 行為 |
|------|------|------|
| `NORMAL` | 預設 | 全部 risk rule 正常運作 |
| `CAUTIOUS` | post-trade 觀察期偵測 | 部分 rule 加大 severity |
| `DEFENSIVE` | drawdown 接近上限 | `TargetPct` 上限 0.5 |
| `SUSPENDED` | `Verdict = HALT` | 全停（manual unfreeze） |

每次 mode 變化都 publish RiskDecision 到 eventbus。

### 4.5 AgentLoop State Machine

已完整文件於 `docs/specs/agent-loop-state-machine-spec.md`：

```
Initial → Plan → ToolCall → Reflect
                              │
                              ├── Continue=true → loop back
                              └── Continue=false → Final
                                                  │
                                                  └── Exhausted()
                                                      yes → force Final
```

關鍵 invariant：`Round += len(steps)`（不是 +1），由 Issue #711 #6 修正。

---

## 5. 觸發條件總表（給 Stage 2 補完用）

每條 workflow 的 trigger 大致分為：

| Trigger 類型 | 範例 workflows |
|-------------|--------------|
| CLI flag | WA-005（`prism worker`） |
| API 啟動時 | WA-200、WA-403、WA-700 |
| Cron / 排程 | WA-500、WA-503、WA-504、WA-604 |
| Event-driven（subscribe） | WA-103、WA-202、WA-401、WA-601、WA-602、WA-603、WA-606 |
| Per-symbol 排程 | WA-300、WA-301 |
| Per-order | WA-400 |
| User-initiated（HTTP） | (待 Stage 2 補：backtest / portfolio query / Web UI 互動) |

---

## 6. CLI Flag 速查表

| Flag | 預設 | 說明 |
|------|------|------|
| `-api` | false | 啟 Dashboard API server |
| `-addr` | `:18080` | API listen address（見 `internal/constants/port.go`）|
| `-swagger` | false | 啟 Swagger docs endpoints |
| `-live` | false | 啟 live trading orchestrator |
| `-simulate` | false | one-shot simulation，exit |
| `-build-universe` | "" | `run/map/scrape/status` |
| `-use-llm-sector-agents` | "" | L2.4 override（"true"/"false"）|
| `-allow-live-broker` | false | allow live broker mode ⚠️ **本地測試勿開** |
| `-allow-realtime` | false | 啟 real-time regime detection adapter |
| `-allow-http-broker` | false | allow http broker adapter in live mode |
| `-allow-real-signer` | false | allow non-placeholder signer |
| `-check-integrity` | false | configs/parameters.json 完整性檢查 |
| `-date` | "" | 模擬日期 override (2006-01-02) |
| `-fubon-port` | :18081 | fubon-proxy Python listen port |
| `-log-format` | text | text / json |
| `-verbose` | false | color-coded terminal trace |

Broker config 旗標（10 個）：`-broker-mode`、`-broker-adapter`、`-broker-signer`、`-broker-key-id`、`-broker-retry-status-codes`、`-broker-max-retries`、`-broker-max-clock-skew-sec`、`-broker-nonce-ttl-sec`、`-broker-nonce-store`、`-broker-nonce-store-path`、`-broker-nonce-redis-url`、`-broker-nonce-redis-key-prefix`

---

## 7. 資料來源（Provider）清單

`internal/marketdata/` 共 121 檔，分為：

| 類型 | 範例 provider |
|------|--------------|
| TWSE / TPEX 本地市場 | `twse.go`、`otc_provider.go`、`twse_margin_provider.go` |
| 美股 / 美指數 | `us_market_refresh.go`、`us_tech_provider.go`、`us_index_provider.go` |
| 匯率 | `exchangerate_provider.go`、`frankfurter_provider.go` |
| 替代 data source | `fugle_client.go`、`fubon_provider.go`、`tej_provider.go`、`finmind_*` |
| 即時資料 | `realtime/router.go`、`realtime/fugle_ws.go`、`realtime/redis_subscriber.go` |
| Macro | `macro_provider.go`、`yahoo_macro_provider.go` |
| Sector | `sector_data_provider.go`、`twse_sector_index_provider.go` |
| Index | `sox_index_provider.go`、`bdi_provider.go`、`tsm_adr_*` |
| ETF | `etf_nav_provider.go`、`twse_etf_provider.go` |
| Calendar / Microstructure | `calendar_provider.go`、`microstructure_provider.go` |

所有 provider 都透過 **Gateway**（`internal/apigateway`）呼叫，受 6 條憲法管轄。

---

## 8. 已標記的 Stage 2 缺口清單

下列資訊在 Stage 1 階段無法輕易確定，需要 Stage 2 對特定模組 deep dive 才能補完：

### 8.1 細節缺口
- WA-100 / WA-102：排程頻率、限流策略、quota 規則
- WA-202 / WA-203（Narrative）：是否仍在 L2.4 觀察期、與 JANUS 的 event flow
- WA-301：Plan→ToolCall→Reflect 在 LLM 模式下的完整 round-trip 含 tool registry、PlanStep 結構
- WA-302 / WA-303：Multi-strategy allocator 與 conviction builder 的具體加權演算法
- WA-402 / Approval：哪些 decision 觸發人工 review、誰是 consumer
- WA-500 / 503 / 504 / 505：各 scheduler task 的排程時間與觸發條件

### 8.2 治理缺口
- 各 plugin 的 precedence 與 init order
- Gateway channel adapter 的完整清單與 fallback chain
- RiskGate 各 mode 切換的歷史路徑
- Live vs Sim vs Backtest 的 env vars 差異（`LLM_*_ENABLED` 系列）

### 8.3 文件缺口
- 各 Workflow 的失敗處理（failover、circuit breaker）
- HTTP API endpoint 對應表（哪些 workflow 對應 `/api/*` 哪些路徑）— 這是 P1 MCP Server 範圍盤查的關鍵
- 「查詢型」workflow 與「變更型」workflow 的邊界（給 P1 安全設計用）

### 8.4 Agent-facing 缺口
- 對外 OpenAPI schema（Swagger 在 `/api/docs`，但需盤完整 schema）
- 哪些事件可以由 agent subscribe（non-sensor 入口）

---

## 9. Wave / 版本資訊對照（給 Agent 確認時點）

| Wave | 狀態 | 對應文件 |
|------|------|---------|
| Wave 11（L2.3 / L2.4） | L2.3 shipped, L2.4 觀察期 active via PR #821 | `docs/specs/llm-sector-agent-spec.md`、`docs/operations/l2-4-runbook.md` |
| Wave 11 L2.4 followup | 未來工作 | `docs/operations/l2-4-followup.md` |
| Phase A3（gateway alert cleanup） | shipped | main.go:233 註解 |
| Orchestrator plugin 進化 | active（janus/prism/phase3/spawning） | `internal/orchestrator/system_plugins.go` |

---

## 10. 不要做的事（給 Agent 安全邊界）

來自 `AGENTS.md` + `docs/reference/traps.md` + `internal/apigateway/CONSTITUTION.md`：

1. ❌ **不要啟用 `-allow-live-broker` 做本地測試**（生產金融指令會真實送出）
2. ❌ **不要繞過 BackgroundTaskManager** 自行啟動 goroutine
3. ❌ **不要繞過 Gateway** 直接呼叫外部資料源（必須經 `apigateway`）
4. ❌ **不要繞過 ParametersConfig** 寫死 magic number
5. ❌ **不要直接呼叫 `clients/*Provider`** 跳過 LLM DefaultRouter
6. ❌ **不要修改 security 相關配置**前不看 `SECURITY.md` 與 `internal/apigateway/CONSTITUTION.md`
7. ❌ **改 FactorType** 須走 8 步驟 protocol（見 `.claude/skills/atlas-factor-change-protocol`）
8. ❌ **改 AgentLoop state machine** 須更新 `docs/specs/agent-loop-state-machine-spec.md`
9. ❌ **JSON tag** 必須對齊 snake_case `domain.*` 格式

---

## 11. 給 Stage 2 的下一步建議

Stage 2 應依優先級補完：

1. **HTTP API Endpoints ↔ Workflows 對照表**（給 P1 MCP Server 範圍鎖定）
2. **WA-301 LLM Loop 完整 round-trip 文件**（已是 L2.4 觀察期重點）
3. **WA-500/503/504/505 Scheduler Task 排程詳細時間與相依**
4. **WA-400 RiskGate 各 Rule 觸發與 DecisionEvent 完整 payload**
5. **Plugin precedence / init order**（這關係到 P1 啟動順序設計）

每補完一條即從本文檔 v1 升級到 v2。
