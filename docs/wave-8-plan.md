# Wave 8 Plan — Event-Driven Expansion

> **建立日期**：2026-06-20
> **對應 VERSION**：v0.0.0.6（Wave 7.5 已收尾）
> **對應 repo**：kaecer68/atlas-go
> **執行工作區**：獨立 opencode CLI（非當前 CLI）
> **執行依據**：`docs/llm-integration-strategy-framework.md`（v2.1）+ `b29` Wave 7 audit 缺口
> **總體估時**：6-9 個工作天（9 個 RED 事件 + 5 個 YELLOW 事件 + 基礎設施）

---

## 1. Context

Wave 7.5 在 v0.0.0.6 完整收尾（PRs #599-#610）。b29 audit 識別出 4 個 RED + 5 個 YELLOW 事件缺口，這些事件目前沒有 EventBus 整合、無 SSE 通知、無 frontend 渲染。

Wave 8 目標：把 audit 識別的高優先事件實作成統一的事件流（type 定義 + producer + SSE handler + frontend component），讓 dashboard 與監控體系可以即時反應這些信號。

## 2. Scope

### 2.1 Wave 8 RED（9 個高優先事件，1 PR = 1 事件）

| 序 | 事件 | 預估 LOC | 觸發點 | 依賴 |
|----|------|---------|--------|------|
| 1 | `RiskGateRejected` | ~180 | `internal/risk/gate.go` reject path | `internal/risk/` 已存在 |
| 2 | `RiskGateOverride` | ~150 | `internal/risk/gate.go` override callback | 同上 |
| 3 | `IndustryCalendarEvent` | ~220 | `internal/data/industry_calendar.go` ingest | `industry_calendar` 已存在 |
| 4 | `BacktestCompleted` | ~200 | `internal/backtest/runner.go` | 已存在 |
| 5 | `CalibrationCompleted` | ~220 | `internal/calibration/engine.go` | 已存在 |
| 6 | `TradeSlippage` | ~200 | `internal/broker/order.go` fill callback | broker 介面已存在 |
| 7 | `LLMAnnotatorCircuitOpen` | ~180 | `internal/llm_annotator/circuit_breaker.go` | 需 Phase 2 #610 完成 |
| 8 | `LLMAnnotatorFallbackUsed` | ~150 | `internal/llm_annotator/router.go` | 同上 |
| 9 | `LLMAnnotatorQuotaExceeded` | ~140 | `internal/llm_annotator/quota.go` | 同上 |

### 2.2 Wave 9 YELLOW（5 個觀測性擴展，排隊等 Wave 8 完成）

- `ChannelIndividualHealth`（per-channel, not aggregated）
- `FactorWeightRegression`（factor drift signal）
- `DriftDetector`（post-rebalance drift score）
- `RegimeChangeConfirmed`（regime 轉換穩定後 30 秒 emit）
- `IngestionLagSpike`（ingestion → processing > 5s）

## 3. Boundary（嚴格）

### ✅ 可動
- `internal/monitoring/api/events/`（新增事件 .go 檔）
- `internal/monitoring/api/events/llm_*.go`（僅 type 定義與 payload schema）
- `internal/monitoring/service/`（事件 producer）
- `internal/monitoring/handlers.go`（SSE endpoints）
- `web/services/sse.js`、`web/components/event-list.js`（frontend 擴展）
- `monitoring/rules/`（alertmanager rules）
- `docs/roadmap.md`、`docs/events/`（文件）

### ⛔ 不可動
- `internal/llm/`、`internal/llm_annotator/`、`internal/narrative/`、`internal/spawning/`、`internal/orchestrator/`
- `cmd/atlas/main.go`（provider wiring 部分；其餘背景任務註冊可新增）
- `internal/config/` 的 Provider/Config 結構
- `internal/monitoring/service/crossmarket.go`（Wave 7 已結案）

---

## 4. Preliminary Decisions（前置決策檢查清單）

> **這三項是 Wave 8 第一個 PR 開工前必須拍板的架構決策**，不可在 PR 內臨時決定。

### PD-1：事件 Payload Schema 版本化

**決策**：每個事件類型必須包含 `schema_version` 欄位（int，預設 `1`），consumer 端拒絕未知版本（回傳 426 Upgrade Required 或靜默丟棄）。

**理由**：
- 9 個新事件 + 既有 28 個事件，未來不可避免會修改 payload
- 向後相容 vs 破壞性變更需要明確標記
- frontend SSE consumer 可依版本決定 render 策略

**實作位置**：
- `internal/monitoring/api/events/types.go` 新增 `EventSchemaVersion = 1` 常數
- 每個事件 struct 加 `SchemaVersion int \`json:"schema_version"\``
- `internal/monitoring/handlers.go` SSE 解析時檢查版本

**Owner**：Wave 8 CLI 第一個 PR 帶入

### PD-2：JSONL 審計軌跡策略

**決策**：9 個新事件**必須**同步寫入 AnnotationStore（JSONL），格式沿用既有 `AnnotationRecord` schema。

**理由**：
- `docs/llm-integration-strategy-framework.md` §7 已定義 `AnnotationRecord` 為審計軌跡標準
- EventBus 是即時通道，JSONL 是歷史軌跡——兩者互補
- 9 events × 預估產生頻率（保守估 100/sec peak）= JSONL 寫入負載約 900 行/sec，需驗證效能

**實作位置**：
- `internal/llm_annotator/persistence.go` 的 `AnnotationStore` 介面擴充 `WriteEvent(EventType, payload)` 方法
- 或新增 `internal/monitoring/persistence/event_store.go` 獨立 EventStore（依既有架構偏好）
- 每個事件 producer 在 emit EventBus 後同步呼叫 store

**效能預算**：
- 單節點 max 900 events/sec × 16 bytes/line = 14.4 KB/sec 寫入，遠低於 SSD 瓶頸
- 必須加 batch flush（每 100 筆或 1 秒 flush 一次）

**Owner**：Wave 8 CLI 第三個 PR 帶入（事件 3 `IndustryCalendarEvent`，因為它的事件頻率最穩定適合做效能驗證）

### PD-3：效能預算與去重策略

**決策**：

1. **單節點事件產生上限**：max 500 events/sec（baseline），burst 1000 events/sec（短暫尖峰）
2. **高頻事件去重**：
   - `TradeSlippage`：每筆訂單只 emit 一次（producer 端 dedup）
   - `DriftDetector`：time bucket（5 秒）內相同 score 只 emit 一次
   - `IngestionLagSpike`：同 source 5 秒內只 emit 一次
3. **Producer 端節流**：當 backend lag > 1 秒時，producer 自動降級為只寫 JSONL，跳過 EventBus（避免記憶體堆積）

**理由**：
- 9 個事件中 4 個為高頻（TradeSlippage/DriftDetector/IngestionLagSpike/FactorWeightRegression）
- 沒有去重 / 節流策略會把 dashboard 與 SSE 通道塞爆
- 9 events × 100Hz × N 訂單 = 數千 events/sec 是常態

**實作位置**：
- `internal/monitoring/service/throttle.go` 新增通用節流器
- 每個高頻 producer 套用對應策略

**Owner**：Wave 8 CLI 第五個 PR 帶入（事件 5 `LLMAnnotatorCircuitOpen`，低頻但需要節流框架先就位）

---

## 5. Atomic PR Breakdown

### PR 順序（9 events，分批 ship）

| PR | 事件 | 帶入的前置決策 | 預估 |
|----|------|---------------|------|
| Wave 8.0 | infrastructure | PD-1 (schema_version)、PD-2 (EventStore 介面) | 1 day |
| Wave 8.1 | `RiskGateRejected` | （使用 Wave 8.0 基礎） | 0.5 day |
| Wave 8.2 | `RiskGateOverride` | — | 0.5 day |
| Wave 8.3 | `IndustryCalendarEvent` | PD-2 效能驗證 | 1 day |
| Wave 8.6 | `TradeSlippage` | PD-3 (high-freq dedup) | 1 day |
| Wave 8.11 | `LLMAnnotatorCircuitOpen` | PD-3 (throttle framework) | 0.5 day |
| Wave 8.12 | `LLMAnnotatorFallbackUsed` | — | 0.5 day |
| Wave 8.13 | `LLMAnnotatorQuotaExceeded` | — | 0.5 day |
| Wave 8.8 | `BacktestCompleted` | — | 0.5 day |
| Wave 8.9 | `CalibrationCompleted` | — | 0.5 day |
| Wave 8.10 | frontend 整合測試 + docs | — | 1 day |
| **總計** | | | **~7.5 days** |

### Wave 9 順序（5 events，獨立 milestone）

待 Wave 8 收尾後再開新 plan。

## 6. Dependencies

### 上游（必須先完成）

- ✅ Wave 7.5 完成（v0.0.0.6，PRs #599-#610）— **已就緒**
- ⏳ Phase 2 CLI 修復 #610 的 5 項 review findings（capability 名稱 drift 等）— **in-flight**
- ⏳ Phase 2 capability→model 路由介面穩定 — **依 Phase 2 進度**

### 下游（被阻塞）

- Phase 4 Production Trading（event-driven live monitoring 完備前不進入）
- `refactor: split 4 oversized files`（見 issue 待開）— Wave 8 結束後執行

## 7. Risks

| 風險 | 機率 | 影響 | 緩解 |
|------|------|------|------|
| Phase 2 進度延遲導致事件 5-7 卡住 | 中 | 中 | 事件 5-7 可獨立 ship，引用 capability 字串時用 interface 而非 string literal |
| JSONL 寫入變成瓶頸 | 低 | 中 | PD-2 batch flush 100 筆或 1 秒 |
| frontend SSE 重連時漏事件 | 中 | 低 | 沿用 PR #606 catchup pattern |
| schema_version 設計過於複雜拖慢開發 | 低 | 低 | 預設 v1，未來變更時再加 |

## 8. Exit Criteria

- [ ] 9 個 RED 事件全部 ship 並 production 上線
- [ ] 5 個 YELLOW 事件在 Wave 9 plan 中排隊（不必本 wave 完成）
- [ ] 所有事件都有對應 frontend component render
- [ ] 所有事件都有對應 Prometheus alert rule
- [ ] 所有事件都有對應 `docs/events/<event-name>.md` 說明
- [ ] `docs/roadmap.md` 更新為 v0.0.0.7 + Wave 8 段落
- [ ] `CHANGELOG.md` 加入 v0.0.0.7 entry
- [ ] `VERSION` 從 0.0.0.6 bump 到 0.0.0.7

## 9. References

- `docs/llm-integration-strategy-framework.md`（v2.1）— Provider 與 capability 架構
- `b29` 壓縮區段 — Wave 7 audit 與事件缺口依據
- `.opencode/prompts/wave-8-bootstrap.md` — 新 CLI 工作區啟動提示詞
- `docs/roadmap.md` — Wave 7.5 段落（Wave 8 上游）
- 既有事件 pattern：`internal/monitoring/api/events/health_alert.go`
- 既有 SSE catchup：`internal/monitoring/api/events/health_alert_catchup.go`（PR #606）