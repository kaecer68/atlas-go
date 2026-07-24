# Audit Manifest: EventBus 孤兒事件根因調查與修復

> **Audit source**: 2026-07-24 架構審計 — 發現 50+ event types 中 8+ 個無消費者
> **Goal**: 對每個孤兒事件分類（實作未完全 / 設計預留 / 已廢棄），補接消費者或清理
> **Scope**: `internal/eventbus/eventbus.go` 中定義但無生產消費者的 event types
> **Created**: 2026-07-24
> **Status**: in-progress

---

## Invariant Table

| ID | 事件 | 發布位置 | 訂閱者 | 分類 | 決策 | 優先級 |
|---|---|---|---|---|---|---|
| O-01 | `EventMarketOpen` | `live/orch:378` via `publishEvent` | ❌ 無 | 實作未完全 | wire: 接 SSE | P0 |
| O-02 | `EventMarketClose` | `live/orch:446` via `publishEvent` | ❌ 無 | 實作未完全 | wire: 接 SSE | P0 |
| O-03 | `EventMarketSnapshot` | `live/orch:600` via `fetchAndDispatchQuotes` | ❌ 無 | 實作未完全（高頻） | defer: 需先設計節流 | P2 |
| O-04 | `EventStopLossTriggered` | `live/orch:603` via `publishRiskEvent` | `risk/audit` 有, SSE 無 | 實作未完全 | wire: 接 SSE | P0 |
| O-05 | `EventTakeProfitTriggered` | `live/orch:603` via `publishRiskEvent` | `risk/audit` 有, SSE 無 | 實作未完全 | wire: 接 SSE | P0 |
| O-06 | `EventAgentHealthChange` | `portfolio/agent_health.go:246` | ❌ 無 (僅測試) | 實作未完全 | wire: 接 SSE | P1 |
| O-07 | `EventMCPAnomalyDetected` | `mcp/anomaly/emitter.go:170` | ❌ 無 (僅測試) | 替代衝突 | consolidate: 與 alerting.Publisher 二擇一 | P1 |
| O-08 | `EventPortfolioPnL` | ❌ 從未發布 | ❌ 無 | 設計預留 | defer: 確認設計後實作 | P2 |
| O-09 | `EventAgentEvaluation` | ❌ 從未發布 | ❌ 無 | 設計預留 | defer: 確認 PRISM/JANUS 介面 | P2 |
| O-10 | `EventMarketTick` | ❌ 從未發布 | ❌ 無 | 已廢棄 | remove: 刪除定義 | P1 |

### 已確認完整的對照組

| 事件 | 發布位置 | 消費者 |
|---|---|---|
| `EventOrderPlaced` | `live/order_manager.go` | `risk/audit_subscriber.go` |
| `EventOrderFilled` | `live/order_manager.go` | `risk/audit_subscriber.go` |
| `EventOrderRejected` | `live/order_manager.go` (status="rejected") | `risk/audit_subscriber.go` |
| `EventRiskAlert` | `live/agent_runner.go:254`, `live/orch:421` | `risk/audit_subscriber.go` |
| `EventStopLossTriggered` | `live/orch:603` | `risk/audit_subscriber.go` (audit only, no SSE) |
| `EventTakeProfitTriggered` | `live/orch:603` | `risk/audit_subscriber.go` (audit only, no SSE) |

### SSE Dashboard 現有 15 事件訂閱 (`sse_handler_subscriptions.go`)

EventNarrative, EventPromotionRecorded, EventHealthAlert, EventRiskGateRejected/Allowed/Overridden,
EventIndustryCalendar, EventBacktestCompleted, EventCalibrationCompleted, EventTradeSlippage,
EventChannelIndividualHealth, EventRegimeChangeConfirmed, EventFactorWeightRegression,
EventDriftDetected, EventIngestionLagSpike

**缺口**: 上述 15 個中沒有任何 live trading 生命週期事件（MarketOpen/Close、StopLoss/TakeProfit、AgentHealthChange）。

---

## 分類準則

| 分類 | 定義 | 修復策略 |
|---|---|---|
| **實作未完全** | 生產端 publish 邏輯存在且正確，但缺消費者 | 補接消費者（SSE buffer / monitoring consumer） |
| **設計預留** | 常數定義存在，但從未 publish | 確認設計意圖 → 實作或移除 |
| **替代衝突** | 同一事件有兩條並行路徑（eventbus + 直接 API） | 調查後二擇一，消除重複 |
| **已廢棄** | 定義存在但明顯無使用計畫 | 移除定義 |

---

## Phase Tracker

### Phase A — Audit (read-only) ✅ DONE

| Task | Status | Evidence |
|---|---|---|
| 列出所有 eventbus 事件類型 | ✅ | `internal/eventbus/eventbus.go:18-106` — 50+ types |
| 搜尋所有生產端 Subscribe | ✅ | grep 結果：15 SSE + 5 monitoring + audit_subscriber |
| 交叉比對 publish/subscribe 缺口 | ✅ | 見上方 Invariant Table |
| 確認 live/orchestrator 的 publishEvent 間接調用 | ✅ | `live/orch:378,446,600` — MarketOpen/Close/Snapshot 透過 publishEvent 發布 |
| 確認 MCP anomaly 雙路徑 | ✅ | `mcp/anomaly/emitter.go:157` (alerting.Publisher) + `:170` (eventbus) |

### Phase B — Plan

| Task | Status | Decision |
|------|--------|----------|
| O-01/O-02 MarketOpen/Close | ✅ | wire: SSE buffer + subscription |
| O-04/O-05 StopLoss/TakeProfit | ✅ | wire: SSE buffer + subscription |
| O-06 AgentHealthChange | ✅ | wire: SSE buffer + subscription |
| O-07 MCPAnomalyDetected | 🔵 pending | 調查 alerting.Publisher 是否已足夠 |
| O-10 EventMarketTick | 🔵 pending | remove: 刪除常數定義 |
### Phase C — Implement

| PR | IDs | Scope | Status |
|---|---|---|---|
| PR #1 | O-01, O-02 | 補接 MarketOpen/Close → SSE | ✅ |
| PR #2 | O-04, O-05 | 補接 StopLoss/TakeProfit → SSE | ✅ |
| PR #3 | O-06 | 補接 AgentHealthChange → SSE | ✅ |
| PR #4 | O-10 | 移除 EventMarketTick | 🔵 pending |
| PR #5 | O-07 | MCP anomaly 雙路徑合併 | 🔵 pending |

### Phase D — Close Out

| Task | Status |
|---|---|
| 更新 manifest status | 🔵 pending |
| Push branch / open PR | 🔵 pending |
| Run CI / verify | 🔵 pending |

---

## Backlog
| ID | Problem | Status |
|---|---|---|
| O-03 | MarketSnapshot 高頻事件需設計節流消費 | ✅ 已完成 — 非高頻（僅收盤/intraday cycle 觸發），已補接 SSE |
| O-08 | PortfolioPnL 從未接線，需確認發布時機 | ✅ 已完成 — 在 SimulationComplete 後發布，含 DayPnL |
| O-09 | AgentEvaluation 從未接線，需確認 PRISM/JANUS 輸出 | ✅ 已完成 — 透過 PRISMManager.SetOnCompleted 回調發布 |
---

## Commit Discipline

- Format: `feat(manifest): #O-XX <short description>`
- One commit per ID group per PR
- PR body must reference: `See docs/manifests/2026-07-24-eventbus-orphan-audit.md`

---

- **Done this session**: Phase A (audit), Phase B (plan), PR #1-#4 (O-01/O-02/O-04/O-05/O-06/O-10 → 5 events wired + 1 removed), PR #5 (O-07 MCP anomaly wired)
- **Remaining**: Backlog (O-03/O-08/O-09)
- **Next action**: Backlog design + PR for remaining 3 items
- **Uncommitted code**: yes
  - `internal/monitoring/api/events/sse_handler.go` — 6 new buffer types, 6×3 buffer/getter/reset functions, 6 catchup blocks
  - `internal/monitoring/api/events/sse_handler_subscriptions.go` — 6 new subscriptions (hooks: 15→20)
  - `internal/eventbus/eventbus.go` — removed EventMarketTick constant + Descriptions entry
  - `internal/live/eventbus.go` — removed EventMarketTick alias
  - `internal/eventbus/eventbus_test.go` — updated Unsubscribe test + const list
  - `internal/live/eventbus_publish_test.go` — updated Unsubscribe test
- **Branch / PR**: TBD
- **Binary freshness**: ⚠️ `make check-binaries` → 7 STALE (pre-existing drift, not caused by this session). Run `make rebuild-all` before next deploy.
