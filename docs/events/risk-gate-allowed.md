# `monitor.risk_gate.allowed` — 風險閘道純通過事件

> **Wave**：8.2
> **穩定性**：stable
> **首次上線**：v0.0.0.7
> **EventType 常數**：`eventbus.EventRiskGateAllowed`
> **字串值**：`"monitor.risk_gate.allowed"`
> **Severity**：`info`

---

## 用途

當 RiskGate 評估後產出**純通過**決策（`ALLOW`，即所有條件符合、無需覆寫）時記錄該事件，供監控、SSE 串流、JSONL 審計軌跡使用。

Wave 8.2 起 routing 改為三向 split：
- `ALLOW` → `monitor.risk_gate.allowed`（**純通過**，本文件）
- `REDUCE` / `ALERT_ONLY` → `monitor.risk_gate.overridden`（覆寫路徑）
- `BLOCK` / `HALT` → `monitor.risk_gate.rejected`（阻擋）

本事件僅承載 `ALLOW` 語意。若需要覆寫（REDUCE / ALERT_ONLY）事件，請參考 [`risk-gate-overridden.md`](risk-gate-overridden.md)。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/risk/gate.go:164-175` | `RiskGate.publish()` 同步疊代 `g.subs` slice |
| `internal/risk/decision.go:69` | `RiskDecision` 結構定義（含 `Phase` / `Verdict` / `Reason` / `Action` / `Mode` / `Symbol` / `Recorded`） |
| `cmd/atlas/main.go:1603-1614` | Producer bridge：`riskGate.Subscribe(...)` 回呼呼叫 `dashEventBus.PublishRiskGateEvent(payload)` |

**Producer 注入點**（與 `monitor.risk_gate.rejected` 同一個 bridge）：
```go
riskGate.Subscribe(func(rd risk.RiskDecision) {
    dashEventBus.PublishRiskGateEvent(eventbus.RiskGateEventPayload{
        Phase:             string(rd.Phase),
        Verdict:           string(rd.Verdict),
        Reason:            rd.Reason,
        ActionType:        string(rd.Action.Type),
        ActionDescription: rd.Action.Description,
        Mode:              rd.Mode,
        Symbol:            rd.Symbol,
        Timestamp:         rd.Recorded,
    })
})
```

**Auto-routing 邏輯**（在 `PublishRiskGateEvent` 內，見 `eventbus.go:708-720`）：
```go
eventType := EventRiskGateAllowed  // 預設為 overridden
if payload.Verdict == "BLOCK" || payload.Verdict == "HALT" {
    eventType = EventRiskGateRejected  // 阻擋類走 rejected
}
```

→ BLOCK / HALT 路由至 `monitor.risk_gate.rejected`，其餘（ALLOW / REDUCE / ALERT_ONLY）路由至 `monitor.risk_gate.allowed`。

---

## Payload Schema

### `RiskGateEventPayload`（與 rejected 共用）

| 欄位 | 型別 | JSON tag | 必填 | 說明 |
|------|------|---------|------|------|
| `Phase` | `string` | `phase` | ✓ | 評估階段：`pre_trade` / `in_trade` / `post_trade` |
| `Verdict` | `string` | `verdict` | ✓ | 決策結果：`ALLOW` / `REDUCE` / `BLOCK` / `HALT` / `ALERT_ONLY`（overridden 路徑不含 BLOCK/HALT） |
| `Reason` | `string` | `reason` | ✓ | 決策原因（含覆寫說明） |
| `ActionType` | `string` | `action_type` | ✓ | 後續動作型別：`SELL` / `REDUCE` / `FREEZE` / `LIQUIDATE` / `NOTIFY`（空字串表示無 action） |
| `ActionDescription` | `string` | `action_description` | ✓ | 人類可讀的動作描述 |
| `Mode` | `string` | `mode` | ✓ | 閘道當前模式：`NORMAL` / `CAUTIOUS` / `DEFENSIVE` / `SUSPENDED` |
| `Symbol` | `string` | `symbol` | ✓ | 個股代號（如 `2330`），空字串表示組合層級決策 |
| `Timestamp` | `time.Time` | `timestamp` | ✓ | 決策記錄時間 |

### Schema 版本

**目前版本**：v0（未版本化）
**規劃**：依 Wave 8 PD-1 決策，未來將加入 `schema_version int` 欄位（預設 `1`）。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedRiskGateEvent`（`internal/monitoring/api/events/sse_handler.go:73-79`） |
| Buffer 大小 | 50 筆（FIFO） |
| Catchup 順序 | narrative → promotion → health-alert → risk-gate → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆 risk-gate 事件（含 rejected + overridden） |

**注意**：risk-gate buffer 同時緩存 rejected 與 overridden，無法只 replay 其中一種。

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有組件 | `web/static/js/components/risk-gate-panel.js` | 操作面板，**非 SSE-driven**，定期 fetch `/api/dashboard/risk` |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('monitor.risk_gate.allowed', handler)` 訂閱 |

**渲染建議**（Wave 8.10 整合測試階段）：
- 風控操作面板新增「Recent Overrides」section，列出最近 10 筆 overridden 事件
- BLOCK/HALT 拒絕事件以紅色 badge 顯示
- ALLOW/REDUCE/ALERT_ONLY 覆寫以橘色 badge 顯示
- 點擊事件展開完整 payload（Phase / Reason / Action / Mode / Timestamp）

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：5 分鐘內 overridden 事件 ≥ 10 筆 → 警告
- alert: RiskGateOverrideBurst
  expr: increase(atlas_risk_gate_overridden_total[5m]) >= 10
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "風控覆寫頻率過高（5 分鐘內 {{ $value }} 次）"
    description: "可能代表上游策略產生過多需覆寫的決策，建議檢視 gate mode 與 rule 閾值"

# 範例：SUSPENDED 模式下發生 overridden → 嚴重
- alert: RiskGateOverrideInSuspendedMode
  expr: |
    atlas_risk_gate_overridden_total{mode="SUSPENDED"} > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "SUSPENDED 模式下不應出現覆寫事件"
```

> 註：監控指標 `atlas_risk_gate_overridden_total` 待 Wave 8 收尾後開新 issue 設計（不在本 PR scope）。

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestPublishRiskGateEvent` | `internal/eventbus/eventbus_test.go:668` | BLOCK → rejected、ALLOW → overridden |
| `TestPublishRiskGateEvent_HALTRouting` | `internal/eventbus/eventbus_test.go`（本 PR 新增） | HALT → rejected |
| `TestPublishRiskGateEvent_ReduceRouting` | `internal/eventbus/eventbus_test.go`（本 PR 新增） | REDUCE → overridden |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.7 | 2026-06-20 | Wave 8.0 加入 EventRiskGateAllowed 常數、RiskGateEventPayload、PublishRiskGateEvent |
| v0.0.0.7 | 2026-06-20 | Wave 8.1 加入 producer bridge（與 rejected 共用） |
| v0.0.0.7 | 2026-06-20 | Wave 8.2 加入本文件 + HALT/REDUCE 路由測試 |

---

## 相關事件

- `monitor.risk_gate.rejected` — 阻擋類決策（文件待撰寫）
- `risk.stoploss.triggered` — 停損觸發（文件待撰寫）
- `risk.takeprofit.triggered` — 停利觸發（文件待撰寫）
- `risk.alert` — 風險警告（文件待撰寫）
