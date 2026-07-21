# Wave 9 Observability — 設計規格

> **狀態**: Shipped(PR #926 + Issue #927 + PR #928 + PR #931)
> **模組**: `internal/monitoring/`
> **對應任務**: T11 驗證 + Loki 設計 spec 整合
> **設計協調者**: `Wave9Observability`(live mode 統一啟動/關閉)

---

## 1. 目標與非目標

### 1.1 目標

- 統一啟動 5 個觀測偵測器,確保啟動順序正確、任一失敗時可完整 rollback
- 為 Prometheus 提供「真實」failure mode metric(避免 `atlas-metrics-endpoint-empty-body` 事件再次發生)
- 與 Loki(Promtail + Loki + 4 LogQL rules,PR #929)互補:metric 監控「什麼壞了」,log 監控「為什麼壞了」

### 1.2 非目標

- 不取代 Prometheus metric-based alert(P0-1 ~ P0-3 維持)
- 不處理商業指標(regime / factor / recommendation);由 `internal/narrative/` 與 `internal/orchestrator/` 負責
- 不設計新 metric 格式;沿用 Prometheus counter(`_total` 後綴)+ label convention

---

## 2. 架構總覽

```
                        ┌─────────────────────┐
                        │   EventBus          │
                        └──────────┬──────────┘
                                   │
            ┌──────────────────────┼──────────────────────┐
            │                      │                      │
   ┌────────▼────────┐  ┌─────────▼────────┐  ┌─────────▼────────┐
   │ Regime          │  │ Factor Weight    │  │ Channel Health  │
   │ Debouncer       │  │ Regression       │  │ Synthesizer     │
   │                 │  │ Detector         │  │                 │
   │ (start 1st)     │  │ (parallel)       │  │ (parallel)      │
   └────────┬────────┘  └─────────┬────────┘  └─────────┬────────┘
            │                     │                     │
   ┌────────▼────────┐  ┌─────────▼────────┐  ┌─────────▼────────┐
   │ Ingestion       │  │ Drift            │  │                 │
   │ Lag Monitor     │  │ Detector         │  │                 │
   │ (parallel)      │  │ (start 5th,      │  │                 │
   │                 │  │  last)            │  │                 │
   └─────────────────┘  └──────────────────┘  └─────────────────┘
```

**Start 順序**:RegimeDebouncer → {IngestionLag, ChannelHealth, FactorWeight}(並行) → DriftDetector
**Stop 順序**:LIFO(Drift → Factor → Channel → Ingestion → Regime)
**理由**:RegimeDebouncer 必須先建立 baseline;DriftDetector 依賴前三者的 steady-state output

---

## 3. 5 個偵測器

| 偵測器 | 啟動順序 | 監控目標 | 主要輸出 |
|--------|---------|---------|---------|
| `RegimeDebouncer` | 1st | regime 切換事件去抖 | `EventRegimeChangeConfirmed` |
| `IngestionLagMonitor` | 2nd(並行) | data provider 延遲 | p99 latency, ingest lag events |
| `ChannelHealthSynthesizer` | 2nd(並行) | 多通道健康聚合 | L1-L4 cross-market data visibility |
| `FactorWeightRegressionDetector` | 2nd(並行) | factor weight 偏離 | regression event + severity |
| `DriftDetector` | 5th(最後) | weight drift | `EventDriftDetected` v1+v2 payload |

詳見 `internal/monitoring/service/` 對應檔案。

---

## 4. Startup 與 Failure 處理

### 4.1 Start idempotency

```go
Wave9Observability.Start(ctx)  // 第一次啟動
Wave9Observability.Start(ctx)  // 第二次: 立刻返回 error("already started")
```

### 4.2 Partial Failure Recovery

任一偵測器啟動失敗 → 已啟動的偵測器以 LIFO 順序 Stop → 內部 reference 清空 → 下次 `Start()` 重新建立乾淨實例(避免 stale bus subscription)

**實作**:`wave9_runtime.go:152-181` 的 deferred cleanup block。

### 4.3 Stop LIFO

`Stop()` 即使中途有 error 也會繼續嘗試停止所有偵測器,聚合錯誤但只回傳第一個(給 caller 警示)。

---

## 5. Prometheus Metrics(PR #926 落地)

### 5.1 Metric 索引

| Metric 名稱 | 常數 | 用途 | Labels |
|------------|------|------|--------|
| `atlas_db_init_failures_total` | `MetricDBInitFailures` | bootstrap DB 連線失敗計數 | `phase` |
| `atlas_channel_health_errors_total` | `MetricChannelHealthErrors` | 通道 health 錯誤計數 | `channel` |

### 5.2 命名慣例

- `atlas_` 前綴避免與 Prometheus default metrics 衝突
- `_total` 後綴標示 counter(Prometheus 慣例)
- label 名小寫 snake_case;值域受限避免 cardinality 爆炸(僅 `phase=startup` 與已知 channel 名)

### 5.3 問題重現:Issue #927

PR #925 / Issue #927 經典場景:alert rule 引用 `channel_errors_total`,但 runtime 從未 emit 此 metric,導致 rule 永遠不會觸發(dead code)。**PR #926 + PR #928 修復**:
- 改 emit `atlas_channel_health_errors_total{channel="..."}`
- `monitoring/rules/wave9_channel_individual_health.yml` 改用真實 metric,加 `sum by (channel)` 逐通道評估
- rule 仍 `enabled: "false"`(PD-W9-1),需 operator 驗證後手動啟用

### 5.4 命名空間清理建議

未來新增 metric **必須**遵循 `atlas_<feature>_<measurement>_total` 格式;`channel_errors_total` 這種無前綴命名應在後續清理週期移除。

---

## 6. Configuration

`NewWave9Observability(bus, opts...)` 接受 4 個 optional provider:

| Provider | 必要性 | 影響 |
|----------|-------|------|
| `ChannelHealthProvider` | **必填** | nil → `NewWave9Observability` 返回 error |
| `IngestionLagProvider` | **必填** | nil → `NewWave9Observability` 返回 error |
| `WeightProvider` | optional | nil → FactorWeightRegressionDetector no-op |
| `TargetWeightsProvider` | optional | nil → DriftDetector 退回 v1(無 target_drift) |

測試可注入 `withDetectorFactory(f)` 替換建構,無需改 production code。

---

## 7. Health Endpoint 規則(PR #931)

`/api/llm/health` 是**觀測型端點**,需在兩處同步列入 auth-free path:
- `internal/monitoring/api/shared/handler.go:authFreeExactPaths`
- `cmd/atlas/main.go:isPublicPath`

**陷阱**:只改其中一處,rebuild 後仍 401。**永遠兩處同步**。見 traps.md 對應 entry。

---

## 8. 與 Loki 的整合

PR #929 建立的 Loki 設計 spec 規劃 4 條 LogQL rules(PR #929 §3.4),對應 5 個偵測器的不同失敗模式:

| 偵測器失敗 | LogQL rule 對應 |
|-----------|----------------|
| RegimeDebouncer 沒 emit `EventRegimeChangeConfirmed` | `AtlasGoHighErrorLogRate` (ERROR > 5/sec 5m) |
| IngestionLagMonitor panic | `AtlasGoPanicDetected` (1m 內 panic) |
| ChannelHealthSynthesizer 靜默失敗 | `AtlasGoAPIGateway5xxSpike` (5xx > 10/5m) |
| FactorWeightRegressionDetector 沒 emit event | `AtlasGoServiceUnavailableLog` (1/5m) |
| DriftDetector OOM | `AtlasGoPanicDetected` |

Loki 上線後,operator 可從 log 快速定位 5 個偵測器哪個出問題。

---

## 9. 與 EventBus 的關係

5 個偵測器皆訂閱 `eventbus.EventBus` 事件;若 EventBus 在 wave9 啟動後失敗,所有偵測器會進入 degraded 模式(無事件流入)。

**EventBus 必填**:`NewWave9Observability` 不接受 nil event bus。

---

## 10. 已知限制

1. **Live mode only**:`Wave9Observability` 在 `cmd/atlas/main.go` 僅於 live 模式啟動;replay 模式不啟用(避免重放歷史事件)
2. **LIFO stop 可能漏 cleanup**:若 `Stop()` 中途 panic,已呼叫 Stop 的 detector 會關閉,未呼叫的會 leak。**緩解**:defer block 內建 cleanup,但無法防 panic
3. **DriftDetector v2 依賴 target weight**:若 `TargetWeightsProvider` 未設定,fallback 到 v1(無 target_drift 偵測)

---

## 11. 測試覆蓋

- `internal/monitoring/wave9_runtime_test.go`:Start 順序、partial failure、LIFO stop、idempotency
- `internal/monitoring/startup_metrics_test.go`:nil collector 安全、label 注入
- `internal/monitoring/service/drift_detector_test.go`:v1/v2 切換

---

## 12. 相關文件

- `internal/monitoring/AGENTS.md` — 模組 hot-path 規則
- `internal/monitoring/wave9_runtime.go` — coordinator 實作
- `internal/monitoring/startup_metrics.go` — metric emission helper
- `internal/monitoring/service/drift_detector.go` — DriftDetector v1/v2
- `docs/handoff/2026-06-26-wave9-observability-coordinator.md` — 設計演進
- `docs/operations/wave9-runbook.md` — 操作手冊(姊妹文件)
- `docs/operations/loki-deployment.md` — 集中式 log 設計
- `docs/reference/traps.md` — `/api/llm/health` 401 防回歸 entry

---

## 13. 版本歷史

- **2026-07-03**:PR #926 落地 `atlas_db_init_failures_total` + `atlas_channel_health_errors_total`
- **2026-07-03**:Issue #927 + PR #928 修正 `wave9_channel_individual_health.yml` 引用 dead metric
- **2026-07-03**:PR #929 整合 Loki 4 條 LogQL rules
- **2026-07-03**:PR #931 修正 `/api/llm/health` 401,雙路徑 auth-free 同步
- **2026-07-03**:本文檔建立,整合上述決策
