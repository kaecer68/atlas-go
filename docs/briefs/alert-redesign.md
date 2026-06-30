# Alert System Redesign v2（2026-06-26 重設計）

> **狀態**：草稿，待用戶確認。v1 仍保留於 `.omo/briefs/ALERT_SYSTEM_REDESIGN.md` 對照。
> **取代**：`.omo/briefs/ALERT_SYSTEM_REDESIGN.md`（Phase 2 缺 3 端點 + Phase 4 缺 severity 推送 + Decision 1/4/5 未完成 + §7 規格已被新設計取代）
> **設計來源**：`RESEARCH-SUPPLEMENT.md` Part 3.2 + `VALIDATION-REPORT.md` Part 2
> **重設計原則**：
> 1. 已實作項目標記 `[DONE: <commit/PR>]`
> 2. 未實作項目拆成 sub-task（每個 < 8 小時工作量）
> 3. **severity 推送管道（CRITICAL → SMS/Slack）整段刪除**——atlas-go 是模擬優先 + 單人開發 + 零售規模，SMS 不解真實痛點
> 4. **Decision 5 反轉**：experiment_rejected → CRITICAL alert（保留 audit trail）；success/deferred → INFO
> 5. 新增 Decision 6/7/8/9（Trade Slippage / Position Concentration / Portfolio Drawdown / SLA Meta-Alert）
> 6. §7 Configuration 整段重寫（盤點現有 13 個欄位 + 對應 alert rule 實作狀態）
> 7. §6 API 補 sort 參數 + 3 個端點

---

## Part 1：已實作段落 [DONE]

### 1.1 Phase 1: Backend [DONE]

**檔案**：
- `internal/domain/types.go:141-163` — `AlertRecord` struct（含 lifecycle 欄位）
- `internal/monitoring/alert_store.go`（303 行）— `AlertStore` + JSONL persistence
- `internal/monitoring/monitor.go`（355 行）— `Monitor` struct（含 `deduplicator`、`autoHandler`、`currentRegime`）

### 1.2 Phase 3: Frontend [DONE]

**檔案**：`shared_web/static/js/pages/alerts.js`（413 行）

**驗證**：完整 UI 實作（per VALIDATION-REPORT Part 2.1）

### 1.3 Decision 2: Suppress RISK_OFF 0-orders [DONE]

**檔案**：`configs/parameters.json:58-65` 的 `suppress_categories: ["gateway", "simulation:risk_off"]`

### 1.4 Phase 2: API Expansion [DONE: 部分 5/8 端點]

**已完成端點**（`internal/monitoring/alert_api.go:235 行`）：
- ✅ `GET /api/alerts`（含 page、page_size、severity、status、rule、from、to）—— **缺 sort**（見 Part 4.1）
- ✅ `GET /api/alerts/stats`（KPI summary 含 total、severity、last_24h）

**未完成端點**（見 Part 4.2）：
- ❌ `POST /api/alerts/acknowledge-bulk`
- ❌ `POST /api/alerts/silence`
- ❌ `GET /api/alerts/rules`

### 1.5 Phase 4: Automation [DONE: 部分核心實作]

**已完成**：
- ✅ `internal/monitoring/dedup.go`（78 行）— `AlertDeduplicator`
- ✅ `internal/monitoring/autohandler.go`（120 行）— `AutoHandler`（INFO 自動 ack 硬編碼）

**未完成**：
- ❌ severity 推送管道（**整段刪除**，見 Part 2）
- ❌ background_task 3-fail 邏輯（見 Part 3 Decision 4）

---

## Part 2：原本 §4.3 Severity Routing 表 → 整段刪除

> **金融工程理由**：atlas-go 是**模擬優先** + **單人開發** + **零售規模**（< 1000 萬 TWD），沒有 on-call 文化。SMS / Slack page-on-call 解的不是真實痛點（銀行 / HFT / 24-7 監控中心的問題）。**真正的痛點是「CRITICAL alert 沒人接」**，用稽核指標追蹤而非 paging 文化。

### 2.1 原本 §4.3 表（**刪除**）

| Severity | Route | Response Time | Example |
|----------|-------|---------------|---------|
| CRITICAL | Push + SMS + Slack page on-call | 5 min | daily_loss_critical breach |
| ERROR | Slack @mention | 15 min | background_task 3 failures |
| WARNING | Slack channel, no page | 1 hour | etf_nav fetch delayed |
| INFO | Dashboard only | N/A | Heartbeats (demoted to widget) |

### 2.2 改為：AlertStore 欄位 + Dashboard panel + SLA 規則

**新增欄位**（`internal/domain/types.go`）：
```go
type AlertRecord struct {
    // ... 既有欄位 ...
    AcknowledgedWithinSec *int     // ack 延遲（秒），從 emit 到 ack
    SLAViolated            bool     // ack 延遲超過 SLA 閾值時為 true
}
```

**新增 SLA 參數**（`configs/parameters.json:alert` section）：
```json
{
    "alert_sla_critical_sec": 1800,      // CRITICAL 必須 30 分鐘內 ack
    "alert_sla_error_sec": 7200,         // ERROR 必須 2 小時內 ack
    "alert_sla_warning_sec": 86400,      // WARNING 必須 24 小時內 ack
    "sla_violation_meta_alert": true     // 違規者本身是 alert
}
```

**新增 Dashboard panel**（`shared_web/static/js/pages/alerts.js`）：
- `alert_ack_latency_p50` — 中位數 ack 延遲
- `alert_ack_latency_p95` — P95 ack 延遲
- `sla_compliance_rate` — SLA 達標率（%）

**Meta-Alert 邏輯**：SLA 違規自動 emit 新 alert（`severity=CRITICAL`, `rule=sla_violation`, `message="Alert {ID} 未在 {SLA} 秒內 ack (delay={delay})"`）

---

## Part 3：決策反轉與新增

> 每個決策給出：現有實作 / 缺口 / 金融工程重要性 / 實作難度 / 預估工作量 / owner 條件 / Acceptance

### 3.1 Decision 1：One-time Auto-cleanup of Heartbeats（程式碼補完）

**現有實作**：
- ✅ `channel_health_summary` 移至 `internal/monitoring/health.go:60`（health log 而非 alert）—— 概念實現
- ✅ `parameters.json:alert.suppress_categories` 含 `"gateway"`

**缺口**：
- ❌ 一次性清理 16,806 個舊 alert 的程式碼**未實作**
- ❌ `heartbeat_ttl_minutes` 參數**未加入 parameters.json**

**金融工程重要性**：HIGH（16,806 個 noise alert → 影響 on-call 文化）

**實作難度**：LOW / **工作量**：2-4 小時（1 個 PR）

**Acceptance**：
- 一次性 migration：刪除 `rule=channel_health_summary` 且 `timestamp < NOW() - INTERVAL '24 hours'`
- `parameters.json:alert` 新增 `heartbeat_ttl_minutes: 5`
- `monitor.go` health check 用 `heartbeat_ttl_minutes` 判定 channel down

### 3.2 Decision 4：background_task 3-fail + recovery（程式碼補完）

**現有實作**：
- ✅ `internal/monitoring/universe_scheduler.go:57` 有 `ConsecutiveFailures` 欄位（但用於 universe watching）
- ✅ `internal/monitoring/service/circuitbreaker.go:31` 有 `ConsecutiveSL`（但用於 circuit breaker）

**缺口**：
- ❌ `monitor.go` 或 `autohandler.go` 沒有「3 連續失敗才 alert」邏輯
- ❌ 沒有「recovery signal → auto-resolve」邏輯

**金融工程重要性**：HIGH（1 fail transient、3 fail systematic；正確區分避免 alert 疲勞）

**實作難度**：LOW / **工作量**：4-6 小時（1 個 PR）

**Acceptance**：
- `monitor.go` 新增 `backgroundTaskFailureTracker`，每次 `background_task` 失敗遞增 `consecutive_failures`
- `consecutive_failures >= 3` 時 emit `EventBackgroundTaskSustainedFailure`（ERROR severity）
- 任一次成功時 reset counter + emit `EventBackgroundTaskRecovered`
- `EventBackgroundTaskRecovered` 自動 resolve 對應 alert（auto-resolve）

### 3.3 Decision 5（**反轉**）：experiment_rejected → CRITICAL alert

**原 brief 內容**：experiment_success/reject/deferred → log
**重設計理由**：
- `experiment_rejected` 直接影響倉位與資金（baseline 拒絕 promote）
- audit trail 比 log 強（log 易被 rotate；alert 有 lifecycle、可 dashboard、可 SLA review）
- retail investor 需要知道「我的倉位被自動調整了」

**現有實作**：
- ✅ `internal/experiment/judge.go:188,195,211` 有 `TransitionExperimentStatus(... ExperimentRejected)`
- ✅ `domain.ExperimentStatus` 有 `ExperimentAccepted` / `ExperimentRejected`

**缺口**：
- ❌ 沒有 `EventExperimentAccepted` / `EventExperimentRejected` / `EventExperimentDeferred`
- ❌ `TransitionExperimentStatus` 不 emit 事件

**金融工程重要性**：HIGH（baseline 升降級 = 倉位調整 = 資金影響）

**實作難度**：MEDIUM / **工作量**：8-12 小時（1-2 個 PR）

**Owner 條件**：熟悉 `internal/experiment/` + `internal/eventbus/`

**Acceptance**：
- `internal/eventbus/eventbus.go` 新增 3 個 EventType：`EventExperimentAccepted` / `EventExperimentRejected` / `EventExperimentDeferred`
- `internal/domain/types.go:ExperimentStatus` 的 transition 函式 emit 對應 event
- Alert 規則：
  - `experiment_rejected` → **CRITICAL alert**（lifecycle: triggered → resolved-by-user）
  - `experiment_accepted` / `experiment_deferred` → **INFO alert**（保留 audit trail 但降級）

### 3.4 Decision 6（**新增**）：Trade Slippage Anomaly Alert

**現有實作**：
- ✅ `EventTradeSlippage` 已定義（`internal/eventbus/eventbus.go:89`）
- ✅ `PublishTradeSlippage` 函式已實作（eventbus.go:936）
- ✅ payload schema 已定義：`FillPrice` + `ExpectedPrice` + `SlippageBPS` + `SlippageCost`（eventbus.go:255-258）

**缺口**：
- ❌ 沒有 alert consumer
- ❌ 沒有 `SlippageBPS > X` 的 threshold 規則

**金融工程重要性**：HIGH（實交易風控核心；retail investor 對 slippage 極敏感）

**實作難度**：LOW / **工作量**：4-8 小時（1 個 PR）

**Owner 條件**：熟悉 `internal/live/` + `internal/monitoring/`

**Acceptance**：
- `internal/monitoring/` 新增 `trade_slippage_consumer.go`，訂閱 `EventTradeSlippage`
- Alert 規則：
  - `SlippageBPS > 50` (0.5%) → WARNING
  - `SlippageBPS > 100` (1%) → ERROR
- Alert payload 含 symbol、expected_price、fill_price、slippage_bps、slippage_cost

### 3.5 Decision 7（**新增**）：Position / Sector Concentration Alert

**現有實作**：
- ✅ `parameters.json:alert.max_position_weight_pct = 0.15`
- ✅ `parameters.json:alert.max_positions_count = 20`
- ✅ `parameters.json:alert.sector_concentration_threshold` / `_high`
- ✅ `internal/risk/portfolio_risk.go:6` 有 `ConcentrationScore`
- ✅ `internal/risk/macro_aware_drawdown.go:403` 用 ConcentrationScore 觸發 drawdown escalate

**缺口**：
- ❌ 沒有 alert rule（grep `ConcentrationScore.*[Aa]lert` 無結果）
- ❌ parameters 與 risk scoring 都已存在，**只缺 alert 連接**

**金融工程重要性**：HIGH（retail investor 最常見失敗；比 channel health 重要 100x）

**實作難度**：MEDIUM / **工作量**：8-12 小時（1-2 個 PR）

**Owner 條件**：熟悉 `internal/risk/` + `internal/portfolio/` + `internal/monitoring/`

**Acceptance**：
- `internal/risk/` 新增 `concentration_alert_emitter.go`，每次 `AssembleRiskAssessment()` 計算後比對 threshold
- Alert 規則：
  - `single_position_weight > max_position_weight_pct` → ERROR
  - `positions_count > max_positions_count` → WARNING
  - `sector_weight > sector_concentration_threshold_high` → ERROR

### 3.6 Decision 8（**新增**）：Portfolio Drawdown Alert

**現有實作**：
- ✅ `EventDrawdownBreach` 已定義（`internal/eventbus/eventbus.go:75`）
- ✅ `internal/reflexivity/concrete_rules.go:38` 有 "portfolio drawdown >10%" 規則
- ✅ `internal/config/defaults_portfolio.go:521` 有 `MaxDrawdown: 0.08`

**缺口**：
- ❌ 沒有 alert consumer
- ❌ `EventDrawdownBreach` 沒有 publish 點

**金融工程重要性**：HIGH（retail investor 最關心指標；8% 已是業界標準）

**實作難度**：LOW / **工作量**：4-8 小時（1 個 PR）

**Owner 條件**：熟悉 `internal/risk/` + `internal/monitoring/`

**Acceptance**：
- `internal/monitoring/` 新增 `drawdown_consumer.go`，訂閱 `EventDrawdownBreach`
- `internal/risk/` 新增 publish 點：當 portfolio drawdown > 8% 時 emit `EventDrawdownBreach`
- Alert 規則：drawdown > 8% → CRITICAL；drawdown > 5% → WARNING

### 3.7 Decision 9（**新增**）：Alert SLA Meta-Alert（取代原本 SMS / Slack page）

**現有實作**：無（全新設計）

**金融工程重要性**：HIGH（取代原本 paging 文化的稽核指標；確保每個 CRITICAL 有人接）

**實作難度**：MEDIUM / **工作量**：12-16 小時（2 個 PR）

**Owner 條件**：熟悉 `internal/monitoring/` + `internal/domain/` + `shared_web/static/js/`

**Acceptance**（詳見 Part 2.2）：
- `AlertRecord` 新增 `AcknowledgedWithinSec` + `SLAViolated` 欄位
- `parameters.json:alert` 新增 4 個 SLA 參數
- Dashboard 新增 3 個 panel：`alert_ack_latency_p50` / `alert_ack_latency_p95` / `sla_compliance_rate`
- SLA 違規自動 emit meta-alert（CRITICAL）

---

## Part 4：§6 API 規格補完

### 4.1 補 sort 參數

**位置**：`GET /api/alerts`

**現有**：`page`, `page_size`, `severity`, `status`, `rule`, `from`, `to`
**新增**：`sort`（enum：`timestamp_desc` / `timestamp_asc` / `severity_desc` / `first_seen_desc`）

**Acceptance**：
- `internal/monitoring/alert_api.go:31-119` 新增 sort 參數解析
- 預設 `sort=timestamp_desc`
- 對應 SQL 加 `ORDER BY`

### 4.2 補 3 個端點

#### `POST /api/alerts/acknowledge-bulk`

**Request**：
```json
{"ids": ["uuid1", "uuid2", ...]}
```

**Response**：
```json
{"acknowledged": 2, "failed": 0}
```

**Acceptance**：
- `internal/monitoring/alert_api.go` 新增 handler
- 呼叫既有 `AlertStore.AcknowledgeWhere` 批次更新
- 計算 `AcknowledgedWithinSec` 並寫入
- 回傳 `{acknowledged, failed}`

#### `POST /api/alerts/silence`

**Request**：
```json
{"rule": "simulation", "duration_minutes": 60, "reason": "RISK_OFF expected"}
```

**Response**：
```json
{"silenced_until": "2026-06-10T11:00:00Z"}
```

**Acceptance**：
- `internal/monitoring/alert_api.go` 新增 handler
- `AlertStore` 新增 `SilenceRule(rule, until)` 方法
- Silence 期間的 alert 寫入但不 emit（filter）

#### `GET /api/alerts/rules`

**Response**：
```json
{
    "rules": [
        {"rule": "channel_health_summary", "active_count": 0, "last_seen": "2026-06-10T09:58:00Z"},
        {"rule": "background_task", "active_count": 0, "last_seen": "2026-06-10T09:55:00Z"}
    ]
}
```

**Acceptance**：
- `internal/monitoring/alert_api.go` 新增 handler
- 從 `AlertStore` aggregate rule + count + last_seen

---

## Part 5：§7 Configuration 整段重寫

> **重設計理由**：原 §7 規格列了 6 個欄位，但實際 `parameters.json:alert` section 已有 **13 個欄位**。規格已過時。本次重寫為「**現有 13 個欄位盤點 + 對應 alert rule 實作狀態表**」。

### 5.1 現有 13 個 alert 參數盤點

| 參數 | 值 | 用途 | 對應 alert rule 實作狀態 |
|------|-----|------|------------------------|
| `daily_loss_critical_pct` | -0.02 | Daily PnL critical threshold | ⚠️ 參數有，rule 待確認 |
| `daily_loss_warning_pct` | -0.015 | Daily PnL warning threshold | ⚠️ 同上 |
| `max_alert_trigger_rate` | 100/h | alert 觸發率上限 | ❌ 未實作 rule |
| `max_position_weight_pct` | 0.15 | 倉位集中度閾值 | ❌ 缺 alert rule（見 Decision 7） |
| `max_positions_count` | 20 | 總倉位上限 | ❌ 缺 alert rule（見 Decision 7） |
| `max_unacknowledged_alerts` | 10 | 未確認累積上限 | ⚠️ 參數有，rule 待確認 |
| `max_unrealized_loss_pct` | -0.05 | 未實現虧損 | ❌ 缺 alert rule |
| `min_cash_threshold` | 100000 | 現金儲備下限 | ❌ 缺 alert rule |
| `min_screening_rate` | 0.1 | screening 通過率下限 | ❌ 缺 alert rule |
| `rule_engine_cooldown_sec` | 300 | rule engine 冷卻 | ✅ 內部機制 |
| `rule_engine_interval_sec` | 30 | rule engine 評估週期 | ✅ 內部機制 |
| `suppress_categories` | ["gateway", "simulation:risk_off"] | 自動抑制類別 | ✅ 已實作 |
| `system_metrics_interval_sec` | 30 | 系統 metrics 收集週期 | ✅ 內部機制 |

### 5.2 原 §7 規格參數的實作狀態

| 原 §7 規格參數 | 狀態 | 處理 |
|--------------|------|------|
| `heartbeat_ttl_minutes` | ❌ 未實作 | **新增到 Decision 1 補完** |
| `dedup_window_minutes` | ❌ 未實作 | **不建議新增**（dedup 邏輯不需參數控制） |
| `auto_ack_severity` | ❌ 未實作 | **改寫為 Decision 4 的規則**（限縮到純 health 類） |
| `suppress_rules` | ✅ `suppress_categories` | 已實作 |

### 5.3 新增參數建議（基於金融工程缺口）

| 參數 | 預設值 | 用途 | 對應 Decision |
|------|-------|------|--------------|
| `slippage_warning_bps` | 50 | Trade Slippage WARNING 閾值 | Decision 6 |
| `slippage_error_bps` | 100 | Trade Slippage ERROR 閾值 | Decision 6 |
| `drawdown_warning_pct` | -0.05 | Portfolio Drawdown WARNING 閾值 | Decision 8 |
| `drawdown_critical_pct` | -0.08 | Portfolio Drawdown CRITICAL 閾值 | Decision 8 |
| `sector_concentration_warning_pct` | 0.30 | Sector concentration WARNING 閾值 | Decision 7 |
| `sector_concentration_critical_pct` | 0.50 | Sector concentration CRITICAL 閾值 | Decision 7 |
| `alert_sla_critical_sec` | 1800 | CRITICAL SLA（30 分鐘內 ack）| Decision 9 |
| `alert_sla_error_sec` | 7200 | ERROR SLA（2 小時內 ack）| Decision 9 |
| `alert_sla_warning_sec` | 86400 | WARNING SLA（24 小時內 ack）| Decision 9 |

---

## Part 6：實作優先序（與 `roadmap-v2.md` Part 3 Bucket A 對齊）

> **排序原則**：金融工程風險降低價值 ÷ 實作成本

| 序 | 工作 | 預估 PR 數 | 金融工程風險降低 | 實作難度 | 阻塞依賴 |
|---|------|----------|----------------|---------|---------|
| **P0-1** | Decision 8：Portfolio Drawdown alert rule | 1 | HIGH | LOW | 無 |
| **P0-2** | Decision 6：Trade Slippage Anomaly alert rule | 1 | HIGH | LOW | 無 |
| **P0-3** | Decision 7：Position / Sector Concentration alert rule | 1-2 | HIGH | MEDIUM | 無 |
| **P0-4** | Decision 5 反轉：experiment events lifecycle | 1-2 | HIGH | MEDIUM | 無 |
| P1-1 | §6 API 補 3 個端點 + sort 參數 | 2 | MEDIUM | LOW | 無 |
| P1-2 | Decision 9：Alert SLA Meta-Alert + AcknowledgedWithinSec | 2 | HIGH | MEDIUM | P1-1 |
| P1-3 | Decision 4 補完：background_task 3-fail 邏輯 | 1 | MEDIUM | LOW | 無 |
| P2-1 | Decision 1 程式碼補完：heartbeat 一次性清理 + heartbeat_ttl_minutes 參數 | 1 | LOW | LOW | 無 |
| P2-2 | Decision 5 補完：transition 函式加 event emit（與 P0-4 同檔） | 1 | LOW | LOW | 無 |
| P2-3 | AutoHandler 限縮：auto_ack_severity 只對純 health 類生效 | 1 | MEDIUM | LOW | 無 |

**預估總工作量**：約 **5-8 個 PR、3-5 個工作天**

**為什麼這個順序**：
- **P0 全部是「事件 / 邏輯已存在，缺 alert rule」**——實作成本低但金融工程價值高
- **P1 補 API 缺口**——但要排在 P0 後，因為 retail investor 看到 alert 但沒法 ack / silence 比「少一個端點」更痛
- **P1-2（SLA Meta-Alert）排在 P1-1 後**：因為需要先有 ack 端點才能計算 ack latency
- **P2 全部是 backfill**——價值較低，排在後面

---

## Part 7：測試覆蓋盤點

### 7.1 已有測試（覆蓋完整）

- `internal/monitoring/alert_api_test.go`
- `internal/monitoring/dedup_test.go`（如存在）
- `internal/monitoring/autohandler_test.go`（如存在）

### 7.2 缺口測試（每個 PR 必須補）

| Decision | 需要新增測試 |
|---------|------------|
| Decision 1 | 一次性清理的 dry-run + 執行 + rollback 測試 |
| Decision 4 | 3-fail 觸發 + recovery auto-resolve + 邊界（2 fail 不觸發、3 fail 觸發、第 4 次成功 reset） |
| Decision 5 | experiment_rejected 觸發 CRITICAL + lifecycle（triggered → resolved-by-user）+ 邊界 |
| Decision 6 | SlippageBPS > 50 觸發 WARNING、> 100 觸發 ERROR、邊界 |
| Decision 7 | single_position_weight > 0.15 觸發 ERROR、sector_weight > 0.50 觸發 ERROR、邊界 |
| Decision 8 | drawdown > 8% 觸發 CRITICAL、> 5% 觸發 WARNING、邊界 |
| Decision 9 | SLA 違規觸發 meta-alert、SLA 達標不觸發、邊界 |

---

## Part 8：決策依據矩陣

> 評分：1（最低）~ 10（最高）

| Decision | 金融工程價值 | 實作難度（越低越好）| 風險 | 建議優先序 |
|---------|------------|------------------|------|----------|
| Decision 1（heartbeat cleanup） | 6 | 8 | LOW | P2 |
| Decision 2（suppress RISK_OFF）| 8 | — | — | DONE |
| Decision 4（3-fail）| 7 | 8 | LOW | P1-3 |
| Decision 5 反轉（experiment alert）| 9 | 5 | MEDIUM | P0-4 |
| Decision 6（Trade Slippage）| 9 | 8 | LOW | P0-2 |
| Decision 7（Concentration）| 9 | 5 | MEDIUM | P0-3 |
| Decision 8（Portfolio Drawdown）| 9 | 8 | LOW | P0-1 |
| Decision 9（SLA Meta-Alert）| 9 | 5 | MEDIUM | P1-2 |

**總分前 3 名**：Decision 5、6、7、8、9 並列（9 分），建議優先序按實作難度排：P0 全部先做（事件 / 邏輯已存在），P1 再做 API + SLA，P2 最後 backfill。

---

## Part 9：升級時機

完成 P0 全部 4 個 PR 後（估 4-6 PR、2-3 工作天），把本檔升級到 `docs/alerts-redesign.md`，刪除 `.omo/briefs/ALERT_SYSTEM_REDESIGN.md` 與 `.omo/briefs/alert-redesign-v2.md` 副本。

---

## 附錄 A：本檔與 v1 對照

| v1 段落 | v2 對應 | 改動 |
|---------|---------|------|
| §1 Problem Statement | 刪除（已是歷史 context，保留於 audit trail）| 改寫 |
| §2 Root Cause Analysis | 刪除 | 改寫 |
| §3 Critical Decisions | Part 3 | Decision 1/2/4 保留；**Decision 5 反轉**；新增 Decision 6/7/8/9 |
| §4 Architecture | Part 1 + Part 2 | 標記 [DONE]；**§4.3 Severity Routing 表整段刪除**改稽核指標 |
| §5 Phase Plan | Part 1 + Part 3 + Part 4 | 已實作標記 [DONE]；未實作改 sub-task |
| §6 API Specification | Part 4 | 補 sort + 3 個端點 |
| §7 Configuration | Part 5 | **整段重寫**：13 欄位盤點 + 對應 alert rule 狀態 |
| §8 Risk & Mitigation | 整合到 Part 7 | 改為測試覆蓋盤點 |
| §9 Acceptance Criteria | Part 7 | 改為每個 Decision 的 Acceptance |
| §10 References | 保留 | 保留 |

---

## 附錄 B：與 `roadmap-v2.md` 對應

| roadmap-v2 段落 | 本檔對應 |
|----------------|---------|
| Part 3 Bucket A | Part 6 實作優先序（聯合排序）|
| Part 4.1-4.4（金融工程缺口專章）| Part 3.3-3.6（Decision 5/6/7/8 詳細內容）|
| Part 1.7 Issue #611 解除 | （間接支援 Decision 8，因 `FactorWeightEngine.GetWeights(regime)` 已備）|