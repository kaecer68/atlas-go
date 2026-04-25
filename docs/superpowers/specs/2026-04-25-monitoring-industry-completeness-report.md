# Atlas-Go 監控與產業生態系完整性建議報告

**日期**: 2026-04-25  
**分析範圍**: 系統警報、指標監控、產業生態系（季節性模式、供應鏈連動）  
**分析依據**: `internal/monitoring/*`, `internal/industry/*`, `cmd/atlas/main.go`, `web/static/index.html`

---

## 執行摘要

經過對 atlas-go 系統的深入分析，發現四個主要領域存在**資料完整性**與**使用者體驗**問題。這些問題並非程式錯誤（bug），而是**架構設計上的缺口**——系統具備資料收集能力，但缺乏：

1. **警報產生機制**（有儲存，無觸發）
2. **指標持久化**（有計算，無累積）
3. **歷史數據視角**（有當前狀態，無長期趨勢）
4. **數據說明與脈絡**（有數值，無解釋）

本報告將問題分類為 **Immediate Fix**（立即修復）、**Short-term Enhancement**（短期增強）、**Long-term Architecture**（長期架構），並提供具體的實作建議。

---

## 一、系統警報頁面

### 1.1 現況診斷

| 項目 | 狀態 |
|------|------|
| 前端顯示 | 「目前沒有警報」 |
| API 回傳 | `{"alerts":[],"total":0}` |
| 資料儲存 | `data/state/alerts/alerts.jsonl`（空檔案） |
| 警報產生 | ❌ **無任何觸發機制** |

**根本原因**: 
- `AlertStore` 已實作（JSONL append-only 儲存）
- `Monitor.Alert()` 已實作（可發送警報並持久化）
- **但**: `main.go` 中雖然建立了 `AlertStore`，卻**沒有將 Monitor 與 AlertStore 連結**
- **更嚴重**: 系統中**沒有任何元件呼叫 `Monitor.Alert()`** 來觸發警報

```go
// main.go 第160-166行: 建立了 AlertStore 但沒有設定到 Monitor
alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
if err != nil {
    log.Printf("[Alerts] failed to create alert store: %v", err)
} else {
    alertAPI := monitoring.NewAlertAPI(alertStore)
    alertAPI.RegisterRoutes(mux)
}
// 缺少: monitor.SetAlertStore(alertStore)
```

### 1.2 影響評估

- **使用者體驗**: 警報頁面永遠空白，使用者無法判斷是「系統正常」還是「功能故障」
- **風險管理**: 系統健康度異常（如資料通道延遲）不會被記錄或通知
- **稽核追蹤**: 缺乏自動化的異常事件記錄

### 1.3 建議方案

#### Immediate Fix（本週完成）

**A. 連結 Monitor 與 AlertStore**

在 `main.go` 中將 `AlertStore` 設定到 `Monitor`：

```go
monitor := monitoring.NewMonitor()
monitor.SetAlertStore(alertStore)
```

**B. 建立系統健康度檢查警報**

在 `SystemMetrics.Start()` 中定期檢查並觸發警報：

```go
// 檢查項目:
// 1. 資料通道延遲 > 5 分鐘 → Warning
// 2. 磁碟空間 < 10% → Critical  
// 3. 記憶體使用率 > 90% → Warning
// 4. 未處理 panic / error 日誌增加 → Error
```

**C. 前端改善**（已完成 ✓）

當沒有警報時顯示說明文字，解釋警報觸發條件與當前狀態。

#### Short-term Enhancement（下個 Sprint）

**D. 警報規則引擎**

建立可配置的警報規則（類似 Prometheus alerting rules）：

```yaml
# configs/alert-rules.yaml
rules:
  - name: data_channel_stale
    condition: "data_channel.last_update > 5m"
    severity: warning
    message: "資料通道超過 5 分鐘未更新"
    
  - name: screening_rate_drop
    condition: "screening.rate < 0.1"
    severity: warning
    message: "篩選率過低，可能表示篩選條件過嚴"
    
  - name: daily_loss_limit
    condition: "portfolio.daily_loss > 0.05"
    severity: critical
    message: "單日虧損超過 5%"
```

**E. 警報通知渠道**

擴展 `Notifier` 介面實作：
- Telegram Bot（已部分支援）
- Email SMTP
- Webhook（Slack/Discord/企業微信）
- 簡訊（Critical 級別）

#### Long-term Architecture（Q3 規劃）

**F. 警報生命週期管理**

```
Triggered → Acknowledged → Resolved → Archived
   ↑           ↓              ↓
   └─ Escalation (未確認超過 30 分鐘)
```

- 支援警報分組（相同問題不重複觸發）
- 支援靜默期（maintenance window）
- 支援警報歷史查詢與趨勢分析

---

## 二、指標監控頁面

### 2.1 現況診斷

| 項目 | 狀態 |
|------|------|
| 篩選率 | 顯示 "0%" |
| 警報觸發 | 顯示 "0" |
| 資金階段 | 顯示 "Simulation" |
| 資料來源 | `MetricsCollector`（記憶體內） |

**根本原因**:
- `MetricsCollector` 是**純記憶體**計數器
- 系統重啟後所有計數歸零
- **沒有任何元件呼叫 `RecordScreening()` 或 `RecordAlert()`**

```go
// 搜尋整個程式碼庫，只有測試檔案呼叫這些方法:
// internal/monitoring/metrics_test.go:11  m.RecordScreening(10, 5)
// internal/monitoring/metrics_test.go:27  m.RecordAlert("circuit_breaker")
// 生產程式碼中零呼叫 ❌
```

### 2.2 影響評估

- **營運盲點**: 無法追蹤系統長期運行趨勢
- **除錯困難**: 問題發生時無法回溯指標變化
- **容量規劃**: 無法基於歷史數據進行資源規劃

### 2.3 建議方案

#### Immediate Fix（本週完成）

**A. 在關鍵路徑插入指標收集**

在 `orchestrator` 的篩選流程中呼叫 `RecordScreening()`：

```go
// internal/orchestrator/executors.go
func (o *Orchestrator) runScreener(reqs []domain.Recommendation) ([]domain.Recommendation, []domain.ScreeningReject) {
    passed, rejected := o.screener.Screen(reqs)
    o.metrics.RecordScreening(int64(len(passed)), int64(len(rejected)))
    return passed, rejected
}
```

**B. 前端改善**（已完成 ✓）

在 help panel 中說明「指標為記憶體內累計值，系統重啟後歸零」。

#### Short-term Enhancement（下個 Sprint）

**C. 指標持久化（JSONL）**

建立 `MetricsStore`（類似 `AlertStore` 的 JSONL append-only）：

```go
// data/state/metrics/metrics-2026-04-25.jsonl
{"timestamp":"2026-04-25T10:00:00Z","screening_total":150,"screening_passed":45,"alerts_triggered":2}
{"timestamp":"2026-04-25T10:30:00Z","screening_total":320,"screening_passed":98,"alerts_triggered":5}
```

優點：
- 與現有 `ledger` 哲學一致（append-only, 可稽核）
- 無需外部資料庫依賴
- 易於壓縮與備份

**D. 啟動時載入歷史指標**

```go
func (m *MetricsCollector) LoadFromStore(store *MetricsStore) {
    // 讀取今日已累計的指標，避免重啟歸零
}
```

#### Long-term Architecture（Q3 規劃）

**E. 時序資料庫整合**

評估引入 Prometheus / InfluxDB / TimescaleDB：

| 方案 | 優點 | 缺點 |
|------|------|------|
| Prometheus | 生態成熟、與 Grafana 整合 | 需要額外服務 |
| InfluxDB | 專用時序資料庫 | 需要額外服務 |
| TimescaleDB (PostgreSQL) | 與現有 DB 整合 | 複雜度較高 |
| 繼續 JSONL | 簡單、無依賴 | 查詢效率低 |

**建議**: 短期繼續 JSONL，中期（Q3）評估 Prometheus 作為可選元件。

**F. 指標視覺化增強**

- 趨勢圖（過去 24 小時 / 7 天 / 30 天）
- 同比/環比變化
- 異常檢測（基於歷史平均值的偏差）

---

## 三、產業生態系 - 季節性模式

### 3.1 現況診斷

| 項目 | 狀態 |
|------|------|
| API 回傳 | 僅當前活躍模式（`active_patterns`） |
| 歷史數據 | 有內建（`HistoricalAccuracy`, `AvgMarketReturn`），但 API 不回傳 |
| 日期範圍 | 7 個季節性模式，涵蓋全年 |
| 當前狀態 | 2026-04-25 不在任何模式範圍內 |

**根本原因**:
- `handleIndustrySeasonality()` 只呼叫 `DetectCurrentPatterns()`
- `SeasonalPattern` 結構已包含豐富的歷史統計欄位，但 API 沒有暴露
- 前端只能顯示「目前無活躍模式」，無法展示全年曆

### 3.2 影響評估

- **使用者困惑**: 「為什麼沒有資料？」「這功能是不是壞了？」
- **決策支援不足**: 無法提前規劃（如「下個月進入除權除息季，應調整配置」）
- **資料價值浪費**: 內建的歷史準確度、平均報酬等數據未被利用

### 3.3 建議方案

#### Immediate Fix（已完成 ✓）

**A. 擴展 API 回傳所有模式**

```go
// /api/industry/seasonality 現在回傳:
{
  "current_date": "2026-04-25",
  "active_patterns": [],  // 當前活躍的
  "all_patterns": [...],  // 所有模式（新增）
  "total_pattern_count": 7
}
```

**B. 新增日曆 API**

```go
// /api/industry/seasonality/calendar
{
  "year": 2026,
  "months": [
    {"month": 1, "patterns": [{"name": "春節行情", ...}]},
    {"month": 2, "patterns": [...]},
    ...
  ]
}
```

**C. 前端列表/日曆雙視圖**（已完成 ✓）

- **列表模式**: 表格顯示所有模式，標註進行中/非活躍
- **日曆模式**: 12 宮格顯示每月適用的模式

#### Short-term Enhancement（下個 Sprint）

**D. 產業別季節性分析**

目前 `all_patterns` 是全域的，應支援產業別篩選：

```go
// /api/industry/seasonality?industry=semiconductor
{
  "industry": "semiconductor",
  "relevant_patterns": [
    {
      "name": "科技旺季",
      "impact": "favored",  // favored | avoided | neutral
      "adjustment_factor": 1.25,
      "historical_accuracy": 0.75
    }
  ]
}
```

**E. 歷史績效追蹤**

建立 `SeasonalPerformance` 記錄：

```go
type SeasonalPerformance struct {
    PatternID        string    `json:"pattern_id"`
    Year             int       `json:"year"`
    ActualReturn     float64   `json:"actual_return"`     // 實際報酬
    PredictedReturn  float64   `json:"predicted_return"`  // 預測報酬
    Accuracy         float64   `json:"accuracy"`          // 當年準確度
    FavoredIndustries []string `json:"favored_industries"`
}
```

長期累積後可計算：
- 各模式的 **rolling accuracy**
- 產業別的 **seasonal alpha**
- 最佳進出場時機

#### Long-term Architecture（Q3 規劃）

**F. 機器學習增強**

- 基於多年數據訓練「季節性強度預測模型」
- 結合宏觀敘事（`narrative`）動態調整 adjustment factor
- 異常檢測：「今年春節行情與歷史模式偏差過大，發出提醒」

---

## 四、產業生態系 - 供應鏈連動

### 4.1 現況診斷

| 項目 | 狀態 |
|------|------|
| 資料來源 | `DefaultSupplyChainGraph()` + `DefaultCorrelationMatrix()`（靜態） |
| 更新頻率 | 永不更新（hard-coded） |
| 歷史比較 | 無法比較「現在 vs 過去」的連動變化 |
| 數據說明 | 僅有簡短文字，無圖例或互動說明 |

**根本原因**:
- 供應鏈圖與相關性矩陣是**編譯期常數**
- 沒有從市場數據動態計算相關性的機制
- 缺乏長期追蹤連動變化的能力

### 4.2 影響評估

- **數據可信度**: 使用者無法判斷這是「即時計算」還是「靜態範例」
- **時效性**: 產業連動關係會隨時間變化（如 AI 浪潮改變半導體與電子的連動）
- **決策品質**: 基於過時的連動模型可能導致錯誤的風險評估

### 4.3 建議方案

#### Immediate Fix（已完成 ✓）

**A. 數據說明區塊**

在表格上方添加：
> 「系統重要性」衡量該產業在整體經濟中的關鍵程度（0-1）；「連動分數」反映衝擊傳導速度...

**B. 統計摘要**

顯示平均系統重要性、平均連動分數、最高/最低值，提供脈絡。

#### Short-term Enhancement（下個 Sprint）

**C. 動態相關性計算**

從市場數據計算產業間的實際相關性：

```go
func (cm *CorrelationMatrix) CalculateFromMarketData(
    industryReturns map[string][]float64,  // 產業日報酬序列
    window int,  // 滾動窗口（如 60 天）
) {
    // 計算 Pearson correlation
    // 更新 cm.correlations
}
```

觸發時機：
- 每日收盤後自動計算
- 或手動觸發（`POST /api/industry/linkage/recalculate`）

**D. 連動變化追蹤**

儲存歷史連動分數：

```go
// data/state/industry/linkage-history.jsonl
{"date":"2026-04-01","industry":"semiconductor","systemic_importance":0.85,"shock_propagation_speed":0.72}
{"date":"2026-04-15","industry":"semiconductor","systemic_importance":0.87,"shock_propagation_speed":0.75}
```

前端顯示：
- 「連動分數趨勢圖」（過去 30 天）
- 「相關性熱力圖」（產業 × 產業矩陣）

#### Long-term Architecture（Q3 規劃）

**E. 衝擊模擬器**

互動功能：「如果半導體產業下跌 5%，哪些產業會受影響？影響多大？」

```go
func (sp *ShockPropagation) SimulateShock(
    sourceIndustry string,
    shockMagnitude float64,
    maxDepth int,
) ShockSimulationResult
```

**F. 供應鏈圖視覺化**

- 節點圖（D3.js / Cytoscape.js）顯示產業間的上游/下游關係
- 節點大小 = 系統重要性
- 邊線粗細 = 相關性強度
- 顏色 = 衝擊傳導速度

---

## 五、整體架構建議

### 5.1 資料收集與持久化架構

```
┌─────────────────────────────────────────────────────────────┐
│                      Atlas-Go 監控架構                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   生產元件    │    │   生產元件    │    │   生產元件    │  │
│  │ Orchestrator │    │    Live      │    │   Backtest   │  │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘  │
│         │                   │                   │          │
│         ▼                   ▼                   ▼          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              MetricsCollector (記憶體)                │  │
│  │  • RecordScreening()                                  │  │
│  │  • RecordAlert()                                      │  │
│  │  • RecordOrder()                                      │  │
│  └────────────────────┬─────────────────────────────────┘  │
│                       │                                     │
│         ┌─────────────┼─────────────┐                      │
│         ▼             ▼             ▼                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ 即時輸出  │  │  JSONL   │  │  時序DB  │                 │
│  │Prometheus│  │ 持久化   │  │ (可選)   │                 │
│  └──────────┘  └──────────┘  └──────────┘                 │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 優先順序矩陣

| 優先級 | 項目 | 難度 | 影響 | 建議時程 |
|--------|------|------|------|----------|
| P0 | 連結 Monitor ↔ AlertStore | 低 | 高 | 本週 |
| P0 | 在篩選流程插入指標收集 | 低 | 高 | 本週 |
| P1 | 警報規則引擎 | 中 | 高 | 下個 Sprint |
| P1 | 指標 JSONL 持久化 | 中 | 高 | 下個 Sprint |
| P1 | 動態相關性計算 | 中 | 中 | 下個 Sprint |
| P2 | 警報通知渠道擴展 | 中 | 中 | Q3 |
| P2 | 季節性績效追蹤 | 高 | 中 | Q3 |
| P2 | 衝擊模擬器 | 高 | 中 | Q3 |
| P3 | 時序資料庫整合 | 高 | 低 | Q4 評估 |

### 5.3 符合 atlas-go 設計哲學的檢核

| 原則 | 符合度 | 說明 |
|------|--------|------|
| 模擬優先 | ✅ | 所有改變先在模擬環境驗證 |
| 稽核導向 | ✅ | JSONL append-only 確保不可竄改 |
| 無全域狀態 | ✅ | MetricsCollector 透過依賴注入傳遞 |
| 小而聚焦 | ✅ | 每個改動都是獨立、可測試的單元 |
| 錯誤包裝 | ✅ | 所有 I/O 操作使用 `fmt.Errorf("context: %w", err)` |

---

## 六、立即行動清單

### 本週（Immediate Fix）

- [ ] **警報系統**
  - [ ] 在 `main.go` 中將 `AlertStore` 設定到 `Monitor`
  - [ ] 在 `SystemMetrics.Start()` 中加入健康度檢查警報
  - [ ] 測試警報觸發與持久化流程

- [ ] **指標收集**
  - [ ] 在 `orchestrator` 篩選流程中呼叫 `RecordScreening()`
  - [ ] 在 `Monitor.Alert()` 中呼叫 `RecordAlert()`
  - [ ] 驗證指標 API 回傳非零值

- [ ] **前端驗證**
  - [ ] 確認警報頁面說明文字正確顯示
  - [ ] 確認指標監控頁面說明文字正確顯示
  - [ ] 確認季節性模式列表/日曆雙視圖正常運作

### 下個 Sprint（Short-term）

- [ ] 設計並實作 `MetricsStore`（JSONL 持久化）
- [ ] 設計警報規則配置格式（YAML/JSON）
- [ ] 實作動態相關性計算（從市場數據）
- [ ] 產業別季節性 API 篩選功能

### Q3（Long-term）

- [ ] 評估 Prometheus / InfluxDB 整合
- [ ] 設計衝擊模擬器互動介面
- [ ] 建立季節性績效追蹤資料庫
- [ ] 供應鏈圖視覺化（節點圖）

---

## 附錄：已完成的改善

以下項目已在本次對話中完成：

1. ✅ **系統警報頁面**: 添加詳細說明文字，解釋為何沒有警報
2. ✅ **指標監控頁面**: 添加記憶體內累計值的說明
3. ✅ **供應鏈連動**: 添加數據說明區塊與統計摘要（平均/最高值）
4. ✅ **季節性模式**: 
   - 擴展 API 回傳所有模式（`all_patterns`）
   - 新增 `/api/industry/seasonality/calendar` 端點
   - 前端支援列表/日曆雙視圖切換

---

**報告產生者**: Sisyphus (AI Agent)  
**審閱建議**: 請團隊確認優先順序與時程是否符合產品路線圖
