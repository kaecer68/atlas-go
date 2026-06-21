# `experiment.calibration_completed` — 參數校準完成事件

> **Wave**：8.9
> **穩定性**：stable
> **首次上線**：v0.0.0.7
> **EventType 常數**：`eventbus.EventCalibrationCompleted`
> **字串值**：`"experiment.calibration_completed"`
> **Severity**：`info`

---

## 用途

當 `config.CalibrateParameters` 24 小時排程任務成功完成時，記錄參數校準結果供監控、SSE 串流、JSONL 審計軌跡使用。本事件監測的是「是否需要人工介入」或「是否出現模型參數漂移」。

校準任務是「參數優化迴圈」的核心：在 baseline 評估 → 優化參數 → 重新評估 → 比較 → 決定是否採用。產出結果可分為三類：
- `improved`：optimized score 優於 baseline，採用新參數
- `unchanged`：差異不顯著，保留 baseline
- `regressed`：optimized score 低於 baseline，rollback

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `cmd/atlas/main.go:2102-2121` | `linkage_calibrate` 排程任務（24h 週期）呼叫 `config.CalibrateParameters` |
| `internal/config/calibrator.go:14-22` | `CalibratorResult` 結構定義（Timestamp / ParamCount / Changes / BaselineScore / OptimizedScore / Verdict / Summary） |
| `internal/config/calibrator.go` | `LinkageAmplifierCalibrator` 校準器（本次串接的對象） |

**Producer 注入點**（`cmd/atlas/main.go:2110-2121`）：
```go
if dashEventBus != nil {
    topChangeParam := ""
    topChangeDeltaPct := 0.0
    if len(result.Changes) > 0 {
        topChangeParam = result.Changes[0].ParamName
        topChangeDeltaPct = result.Changes[0].DeltaPct
    }
    dashEventBus.PublishCalibrationCompleted(eventbus.CalibrationCompletedEventPayload{
        Module:            "linkage",
        CalibratorName:    "LinkageAmplifier",
        ParamCount:        result.ParamCount,
        BaselineScore:     result.BaselineScore,
        OptimizedScore:    result.OptimizedScore,
        Verdict:           result.Verdict,
        ChangeCount:       len(result.Changes),
        TopChangeParam:    topChangeParam,
        TopChangeDeltaPct: topChangeDeltaPct,
        GeneratedAt:       result.Timestamp,
        SyncSucceeded:     true, // best-effort, see Known Limitations
    })
}
```

> **觸發條件**：`config.CalibrateParameters` 回傳 nil error 時。失敗時 logging.Error 不發事件（屬於可觀察性 vs 噪音的權衡；若失敗率變高，應改用 `experiment.calibration_failed` 事件）。

---

## Payload Schema

### `CalibrationCompletedEventPayload`（11 欄位）

| 欄位 | 型別 | JSON tag | 必填 | 說明 |
|------|------|---------|------|------|
| `Module` | `string` | `module` | ✓ | 模組識別（如 `linkage`、`risk`），用於前端分類 |
| `CalibratorName` | `string` | `calibrator_name` | ✓ | 校準器名稱（如 `LinkageAmplifier`） |
| `ParamCount` | `int` | `param_count` | ✓ | 被校準的參數總數 |
| `BaselineScore` | `float64` | `baseline_score` | ✓ | baseline 評估分數（0.0-1.0） |
| `OptimizedScore` | `float64` | `optimized_score` | ✓ | 優化後評估分數（0.0-1.0） |
| `Verdict` | `string` | `verdict` | ✓ | 採納決策：`improved` / `unchanged` / `regressed` |
| `ChangeCount` | `int` | `change_count` | ✓ | 實際變更的參數數量（≤ `ParamCount`） |
| `TopChangeParam` | `string` | `top_change_param` | ✓ | 影響最大的參數名稱（`Changes[0].ParamName`） |
| `TopChangeDeltaPct` | `float64` | `top_change_delta_pct` | ✓ | 影響最大的參數變化幅度（百分比） |
| `GeneratedAt` | `time.Time` | `generated_at` | ✓ | 校準完成時間 |
| `SyncSucceeded` | `bool` | `sync_succeeded` | ✓ | 參數同步至 live store 是否成功（目前硬編為 true） |

### Schema 版本

**目前版本**：v0（未版本化）
**規劃**：依 Wave 8 PD-1 決策，未來將加入 `schema_version int` 欄位（預設 `1`）。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedCalibrationCompletedEvent`（`internal/monitoring/api/events/sse_handler.go:184-188`） |
| Buffer 大小 | 50 筆（FIFO） |
| Catchup 順序 | narrative → promotion → health-alert → risk-gate → backtest-completed → calibration-completed → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆 calibration-completed 事件 |

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有組件 | `web/static/js/components/risk-gate-panel.js` | 操作面板，**非 SSE-driven** |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('experiment.calibration_completed', handler)` 訂閱 |

**渲染建議**（Wave 8.10 整合測試階段）：
- 系統監控頁新增「Calibration History」區塊，列出最近 10 筆 calibration 事件
- `improved` verdict → 綠色 badge + 分數進步百分比
- `unchanged` verdict → 灰色 badge
- `regressed` verdict → 紅色 badge + TopChangeParam tooltip
- 點擊展開完整 payload（BaselineScore / OptimizedScore / Changes / Summary）

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：連續 3 個 calibration verdict 為 regressed → 嚴重
- alert: CalibrationRegressed
  expr: |
    count_over_time(
      atlas_calibration_completed_total{verdict="regressed"}[7d]
    ) >= 3
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "參數校準連續回歸（{{ $value }} 次）"
    description: "可能代表評估函式失準或市場 regime 變化過大，建議人工檢視 calibrator 配置"

# 範例：24h 內無 calibration 事件 → 警告
- alert: CalibrationStalled
  expr: |
    time() - atlas_calibration_completed_last_success_timestamp > 86400
  for: 0m
  labels:
    severity: warning
  annotations:
    summary: "calibration 排程已停滯超過 24 小時"
    description: "可能代表 CalibrateParameters 失敗或排程卡住，檢查 linkage_calibrate task 狀態"

# 範例：optimized score 提升超過 5% → 通知
- alert: CalibrationSignificantImprovement
  expr: |
    (atlas_calibration_optimized_score
     - atlas_calibration_baseline_score) > 0.05
  for: 0m
  labels:
    severity: info
  annotations:
    summary: "校準帶來顯著參數優化（{{ $value | humanizePercentage }}）"
```

> 註：監控指標 `atlas_calibration_*` 待 Wave 8 收尾後開新 issue 設計（不在本 PR scope）。

---

## 已知限制

| 限制 | 影響 | 規劃 |
|------|------|------|
| `SyncSucceeded: true` 硬編 | SSE 客戶端 / JSONL 審計若依賴此欄位做告警會永遠收不到同步失敗 | 待 config 層提供 `SyncToLiveStore` 實際回傳值 |
| 失敗時不發事件 | 校準失敗率變高時無告警來源 | Wave 9 規劃新增 `experiment.calibration_failed` 事件 |
| 失敗時不重試 | `CalibrateParameters` 失敗後直接 return，無指數退避 | main.go:2104 需補 retry 邏輯 |

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestPublishCalibrationCompleted` | `internal/eventbus/eventbus_test.go` | Publish 路由、payload 欄位傳遞、verdict 種類 |
| `TestSSEHandler_BufferCalibrationCompletedEvent` | `internal/monitoring/api/events/sse_handler_test.go` | Buffer 寫入、Get 讀取、EventType 對應 |
| `TestCalibrateParameters` | `internal/config/calibrator_test.go`（既有） | CalibratorResult 產出邏輯 |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.7 | 2026-06-20 | Wave 8.9 加入 EventCalibrationCompleted 常數、CalibrationCompletedEventPayload、PublishCalibrationCompleted、SSE buffer、producer bridge、本文件 |

---

## 相關事件

- [`experiment.backtest_completed`](./backtest-completed.md) — 回測完成事件（Wave 8.8）
- `experiment.insufficient_data` — 實驗資料不足事件（文件待撰寫）
- [`monitor.health_alert`](./health-alert.md) — 系統健康警告（既有）
- `monitor.risk_gate.overridden` — 風控覆寫事件（文件待撰寫）
