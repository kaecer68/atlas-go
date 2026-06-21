# `narrative.event`，宏觀敘事事件

> **Wave**：1-7
> **穩定性**：stable
> **首次上線**：v0.0.0.1
> **EventType 常數**：`eventbus.EventNarrative`
> **字串值**：`"narrative.event"`
> **Severity**：`warning`

---

## 用途

`MacroIngestor` 從全球總經指標（美債殖利率、美元指數、日圓、原油、黃金、VIX、台幣匯率等）偵測出宏觀敘事事件時發布本事件。每個事件包含主題（Theme）、地區（Region）、情緒方向（Sentiment）、信心度（Confidence）與歷史命中率（HitRate），供前端「宏觀敘事」頁面、Regime 偵測、FactorWeightEngine 動態調整使用。

**事件角色**：本事件是敘事感知計算鏈的入口點，承載從原始數據轉譯為領域事件的結果，連接 `MacroIngestor` 與下游訂閱者（`EventLifecycleManager`、SSE bridge、FactorWeightEngine）。

---

## 觸發點

| 觸發位置 | 說明 |
|---------|------|
| `internal/narrative/ingestor.go:139-144` | `MacroIngestor.publishEvents`，對每個非重複的 `NarrativeEvent` 呼叫 `m.eventBus.PublishNarrativeEvent` |
| `internal/eventbus/eventbus.go:786-831` | `PublishNarrativeEvent`，計算 sentimentText、查表 themeDescriptions、組裝 payload |
| `internal/eventbus/eventbus.go:68` | `EventNarrative EventType = "narrative.event"` 常數定義 |
| 信心度來源 | `confidence_source`（預設 `heuristic_fixed_v1`），由 detector 顯式傳入 |

**Producer 注入點**（`internal/narrative/ingestor.go:122-146`）：
```go
func (m *MacroIngestor) publishEvents(events []NarrativeEvent) {
    if m.eventBus == nil {
        return
    }
    for i := range events {
        e := &events[i]
        if m.lifecycle != nil {
            if m.lifecycle.IsThemeActive(e.Theme) {
                if existing := m.lifecycle.GetActiveByTheme(e.Theme); existing != nil {
                    if e.Confidence > existing.Confidence {
                        m.lifecycle.UpdateConfidence(existing.ID, e.Confidence)
                    }
                }
                continue
            }
            m.lifecycle.AddEvent(e)
        }
        m.eventBus.PublishNarrativeEvent(
            e.ID, e.Theme, e.Region,
            e.Sentiment, e.Confidence,
            e.ConfidenceSource, fmt.Sprintf("%.2f", e.HitRate),
            e.CapitalFlow, e.TimeWindow,
        )
    }
}
```

> **重要**：`publishEvents` 內含 lifecycle 去重邏輯：同 Theme 若已有 active 事件，只更新 Confidence 不重複發布；新事件才會走 Publish 路徑。

**`PublishNarrativeEvent` 主體**（`internal/eventbus/eventbus.go:786-831`）：
```go
func (b *ChannelEventBus) PublishNarrativeEvent(
    eventID, theme, region string,
    sentiment, confidence float64,
    confidenceSource, hitRate, capitalFlow, timeWindow string,
) {
    sentimentText := "中立"
    if sentiment > 0.3 {
        sentimentText = "利多"
    } else if sentiment < -0.3 {
        sentimentText = "利空"
    }

    themeDescriptions := map[string]string{
        "US_rates_up":                     "美國公債殖利率上升，可能引發資金流向調整",
        "JPY_carry_unwind":                "日圓套利平倉，顯示全球流動性收緊",
        "geopolitical_risk_spike":         "地緣政治風險攀升，市場避險情緒升溫",
        "oil_price_shock":                 "油價劇烈波動，影響通膨預期",
        "USD_TWD_volatility":              "美元兌台幣波動，反映台灣出口競爭力變化",
        "semiconductor_downturn":          "半導體出口下滑，景氣放緩訊號",
        "AI_capex_surge":                  "AI資本支出強勁，科技股展望正面",
        "retail_frenzy":                   "散戶融資餘額飆升，市場過熱風險",
        "retail_fear":                     "散戶融資餘額低迷，市場情緒低迷",
        "retail_institutional_divergence": "散戶與法人方向分歧，可能出現轉向",
    }

    description := themeDescriptions[theme]
    if description == "" {
        description = fmt.Sprintf("%s 區域發生 %s 事件，%s 信號", region, theme, sentimentText)
    }

    b.Publish(BusEvent{
        ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
        Type:      EventNarrative,
        Timestamp: time.Now(),
        Payload: NarrativeEventPayload{
            EventID:          eventID,
            Theme:            theme,
            Region:           region,
            Sentiment:        sentiment,
            SentimentText:    sentimentText,
            Confidence:       confidence,
            ConfidenceSource: confidenceSource,
            HitRate:          parseFloat(hitRate),
            CapitalFlow:      capitalFlow,
            TimeWindow:       timeWindow,
            Description:      description,
        },
        SchemaVersion: 1,
    })
}
```

> **注意**：`HitRate` 由 `parseFloat(hitRate)` 從字串解析而來，呼叫端以 `fmt.Sprintf("%.2f", e.HitRate)` 傳入；若字串格式錯誤則回傳 `0.0`。

---

## Payload Schema

### `NarrativeEventPayload`（11 欄位）

| 欄位 | 型別 | JSON tag | 必填 | 說明 |
|------|------|---------|------|------|
| `EventID` | `string` | `event_id` | ✓ | 事件唯一 ID（由 detector 產生） |
| `Theme` | `string` | `theme` | ✓ | 事件主題：`US_rates_up` / `JPY_carry_unwind` / `geopolitical_risk_spike` / `oil_price_shock` / `USD_TWD_volatility` / `semiconductor_downturn` / `AI_capex_surge` / `retail_frenzy` / `retail_fear` / `retail_institutional_divergence` |
| `Region` | `string` | `region` | ✓ | 事件發生地區（`US` / `JP` / `TW` / `global` 等） |
| `Sentiment` | `float64` | `sentiment` | ✓ | 情緒方向分數（`>0.3` 利多、`<-0.3` 利空、其餘中立） |
| `SentimentText` | `string` | `sentiment_text` | ✓ | 由 `Sentiment` 計算的中文標籤：`利多` / `利空` / `中立` |
| `Confidence` | `float64` | `confidence` | ✓ | 偵測信心度，區間 `[0.0, 1.0]` |
| `ConfidenceSource` | `string` | `confidence_source` | ✓ | 信心度來源識別，預設 `heuristic_fixed_v1` |
| `HitRate` | `float64` | `hit_rate` | ✓ | 該主題歷史回測命中率（`parseFloat(hitRate)` 解析），範圍 `[0.0, 1.0]` |
| `CapitalFlow` | `string` | `capital_flow` | ✓ | 資金流向描述（如 `foreign_inflow` / `foreign_outflow` / `neutral`） |
| `TimeWindow` | `string` | `time_window` | ✓ | 事件影響時間區間描述（如 `1-7d` / `30d`） |
| `Description` | `string` | `description` | ✓ | 人類可讀說明，由 `themeDescriptions` 表查詢；查不到時 fallback 為 `<region> 區域發生 <theme> 事件，<sentimentText> 信號` |

### 獨特欄位說明

- **`SentimentText`**：在 producer 端由 `Sentiment` 數值即時推導：
  - `Sentiment > 0.3` → `利多`
  - `Sentiment < -0.3` → `利空`
  - 否則 → `中立`
- **`Description`**：依 `Theme` 從 10 筆內建 `themeDescriptions` 表查詢人類可讀中文說明；若 Theme 不在表內則使用 fallback 格式 `<region> 區域發生 <theme> 事件，<sentimentText> 信號`。
- **`HitRate`**：呼叫端以 `fmt.Sprintf("%.2f", e.HitRate)` 傳入字串，producer 端透過 `parseFloat` 解析為 `float64`。解析失敗時為 `0.0`，呼叫端應保證格式正確。
- **`ConfidenceSource`**：保留信心度演算法的可追溯性，預設值 `heuristic_fixed_v1`，未來新增 detector 應使用獨立 source 標籤。

### Schema 版本

**目前版本**：v1（`SchemaVersion: 1` 顯式設定，遵循 PD-1 決策）
**向後相容**：所有欄位皆為 nullable-friendly，缺欄位時對應零值不破壞既有 consumer。

---

## SSE Catchup 行為

| 屬性 | 值 |
|------|-----|
| SSE Buffer | `BufferedNarrativeEvent`（`internal/monitoring/api/events/sse_handler.go`） |
| Buffer 大小 | 50 筆（FIFO，`maxBufferedNarrativeEvents = 50`） |
| Catchup 順序 | **narrative** → promotion → health-alert → risk-gate → backtest-completed → calibration-completed → trade-slippage → SubscribeAll |
| 客戶端重連時 | 自動 replay 最近 50 筆敘事事件 |

**事件流定位**：本事件位於 SSE catchup 迴圈**最前段**，早於所有其他事件。這是因為 narrative 事件觸發頻率最高（每次 `MacroIngestor` 偵測循環都可能產生），且為 RegimeChange 與 FactorWeightEngine 動態調整的源頭，需優先送達前端。

**FIFO 行為**：buffer 超過 50 筆時採尾部保留（`narrativeBuffer[len(narrativeBuffer)-maxBufferedNarrativeEvents:]`），最舊事件自動淘汰。

---

## 前端整合

| 項目 | 檔案 | 說明 |
|------|------|------|
| EventSource listener | `web/static/js/services/event-source.js:73-83` | `handleMessage()` 解析 `data.type` → `emit(eventType, data)`，generic handler |
| 既有敘事面板 | `web/static/js/components/narrative-panel.js` | 宏觀敘事頁面，透過 `/api/narrative/bundle` 載入初始資料 |
| 即時訂閱 | `web/static/js/event-listeners.js` | 透過 `eventSource.on('narrative.event', handler)` 訂閱 |

**渲染建議**（敘事頁面整合）：
- 敘事事件卡片顯示 `Theme` + `Region` + `SentimentText` badge
- `SentimentText == "利多"` → 綠色 badge
- `SentimentText == "利空"` → 紅色 badge
- `SentimentText == "中立"` → 灰色 badge
- 點擊展開 `Description` 與 `Confidence` / `HitRate` / `CapitalFlow` / `TimeWindow` 完整 payload
- 高 `HitRate`（`> 0.7`）可標示為「高命中率主題」並置頂

---

## 監控與告警（建議 Prometheus rules）

```yaml
# 範例：10 分鐘內 narrative 事件利空比率 > 70% → 警告
- alert: NarrativeBearishDominance
  expr: |
    (sum(rate(atlas_narrative_event_total{sentiment_text="利空"}[10m]))
     / sum(rate(atlas_narrative_event_total[10m]))) > 0.7
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "近 10 分鐘敘事事件以利空為主（{{ $value | humanizePercentage }}）"
    description: "可能預示 RegimeChange，建議監控後續 risk-gate 事件"

# 範例：高信心度（confidence > 0.8）事件 5 分鐘內 > 5 筆 → 警告
- alert: NarrativeHighConfidenceBurst
  expr: sum(rate(atlas_narrative_event_total{confidence_gt_08="true"}[5m])) > 1
  for: 0m
  labels:
    severity: info
  annotations:
    summary: "高信心度敘事事件短時間內密集出現"
    description: "可能代表宏觀環境劇烈變動，建議人工 review"
```

> 註：監控指標 `atlas_narrative_event_*` 待後續設計（本事件為 Wave 1-7 既有，metrics 標籤尚未對接）。

---

## 已知限制

| 限制 | 影響 | 規劃 |
|------|------|------|
| `themeDescriptions` 僅 10 筆內建值 | 未知 Theme 會走 fallback 格式，敘事較不精確 | 新增 Theme 需同步擴充表 |
| `HitRate` 透過字串解析 | 格式錯誤會回傳 `0.0`（靜默失敗） | 呼叫端已用 `fmt.Sprintf("%.2f", ...)`，實務風險低 |
| 同 Theme 重複事件僅更新 Confidence | 高頻偵測循環可能淹沒真正的轉折 | 由 `EventLifecycleManager` 提供 active/faded/expired 狀態機補充 |
| 客戶端重連只 replay 最近 50 筆 | 歷史 narrative 事件無法在 SSE 端查詢 | 待 JSONL 啟用後可從 ledger 回查 |
| `SentimentText` 閾值固定（±0.3） | 對極弱信號不夠敏感 | 未來可考慮加入自適應閾值或帶入 `ParametersConfig` |

---

## 測試覆蓋

| 測試 | 檔案 | 覆蓋範圍 |
|------|------|---------|
| `TestPublishNarrativeEvent` | `internal/eventbus/eventbus_test.go` | Publish 路由、payload 欄位傳遞、sentimentText 推導（利多/利空/中立）、themeDescriptions 表查詢、fallback 描述 |
| `TestMacroIngestor_PublishEvents` | `internal/narrative/ingestor_test.go` | `publishEvents` lifecycle 去重、Confidence 更新、eventBus 呼叫參數 |
| `TestSSEHandler_BufferNarrativeEvent` | `internal/monitoring/api/events/sse_handler_test.go` | Buffer 寫入、Get 讀取、EventType 對應、50 筆 FIFO 行為 |
| `TestEventLifecycleManager_AddEvent` | `internal/narrative/lifecycle_test.go` | 同 Theme 重複事件的 active 狀態管理 |

---

## 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v0.0.0.1 | 2026-03-01 | Wave 1 初始加入 EventNarrative 常數、NarrativeEventPayload、PublishNarrativeEvent |
| v0.0.0.3 | 2026-04-15 | Wave 3 加入 SSE buffer（`BufferedNarrativeEvent`）與 catchup 串接 |
| v0.0.0.5 | 2026-05-20 | Wave 5 `themeDescriptions` 表擴充至 10 筆主題覆蓋（新增 `retail_frenzy` / `retail_fear` / `retail_institutional_divergence`） |
| v0.0.0.7 | 2026-06-20 | 本文件建立 |

---

## 相關事件

- [`health.alert`](./health-alert.md)，系統健康告警（敘事事件常為 regime 壓力先行指標，後續 health-alert 可能伴隨觸發）
- [`risk.gate.rejected`](./risk-gate-allowed.md)，風控閘門拒絕（敘事事件嚴重時可能導致風控閘門啟動拒絕）
- [`experiment.backtest_completed`](./backtest-completed.md)，回測完成（敘事事件觸發後常接續回測驗證）
- `regime.change`，市場體制轉變（敘事事件是 RegimeChange 的上游驅動之一）
