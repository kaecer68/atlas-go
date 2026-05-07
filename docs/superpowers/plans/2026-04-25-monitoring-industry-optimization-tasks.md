# Atlas-Go 系統完整性優化工作任務清單

**版本**: v1.0  
**日期**: 2026-04-25  
**狀態**: 待執行  
**依據**: 監控系統架構分析、產業生態系資料完整性分析、網頁介面現況盤點

---

## 任務清單總覽

| 階段 | 任務數 | 預估工期 | 目標 |
|------|--------|----------|------|
| **Phase 1: 緊急修復** | 8 項 | 3-5 天 | 解決資料為空、功能斷線問題 |
| **Phase 2: 資料補強** | 6 項 | 2 週 | 建立資料流動與持久化機制 |
| **Phase 3: 功能完善** | 5 項 | 3 週 | 補齊產業生態系與監控功能 |
| **Phase 4: 體驗優化** | 4 項 | 2 週 | 提升使用者體驗與資料可視化 |
| **Phase 5: 長期架構** | 3 項 | Q3 規劃 | 建立可擴展的監控與產業分析架構 |

---

## Phase 1: 緊急修復（本週完成）

### 🔴 Task 1.1: 連結 Monitor 與 AlertStore
**嚴重程度**: 高 | **影響**: 警報系統完全失效

**問題描述**:
- `main.go` 建立了 `AlertStore` 但從未設定到 `Monitor`
- `Monitor.Alert()` 嘗試寫入 `alertStore` 但永遠為 nil
- 所有警報（包括 RuleEngine 觸發的）全部落入虛無

**實作內容**:
```go
// cmd/atlas/main.go: API mode
alertStore, err := monitoring.NewAlertStore(...)
if err == nil {
    alertAPI := monitoring.NewAlertAPI(alertStore)
    alertAPI.RegisterRoutes(mux)
    
    // 新增: 將 AlertStore 設定到 SystemMetrics 的 Monitor
    sysMetrics := monitoring.NewSystemMetrics(collector, monitor)
    sysMetrics.GetMonitor().SetAlertStore(alertStore)  // 需要新增 GetMonitor()
}

// cmd/atlas/main.go: Live mode  
monitor := monitoring.NewMonitor()
monitor.SetAlertStore(alertStore)  // 新增
```

**驗證方式**:
```bash
# 1. 啟動服務後觸發一個測試警報
# 2. 檢查檔案是否有內容
cat data/state/alerts/alerts.jsonl
# 3. API 應回傳非空陣列
curl localhost:8080/api/alerts
```

**相依任務**: 無

---

### 🔴 Task 1.2: 在 Orchestrator 插入指標收集
**嚴重程度**: 高 | **影響**: MetricsCollector 永遠為零

**問題描述**:
- `RecordScreening()` 和 `RecordAlert()` 僅在測試中被呼叫
- Orchestrator 的 `collectRecommendations()` 追蹤 rejects 但不記錄指標
- 整個 `orchestrator/` 目錄無任何 `MetricsCollector` 引用

**實作內容**:
```go
// internal/orchestrator/system.go

type SystemCore struct {
    // ... 現有欄位 ...
    metricsCollector *monitoring.MetricsCollector  // 新增
}

func (s *SystemCore) SetMetricsCollector(mc *monitoring.MetricsCollector) {
    s.metricsCollector = mc
}

// internal/orchestrator/executors.go
func (o *Orchestrator) collectRecommendations(ctx context.Context, registry *plugin.Registry) ([]domain.Recommendation, []domain.ScreeningReject, error) {
    // ... 現有邏輯 ...
    
    // 新增: 記錄篩選指標
    if o.system.metricsCollector != nil {
        passed := len(recs)
        rejected := len(rejects)
        o.system.metricsCollector.RecordScreening(int64(passed), int64(rejected))
    }
    
    return recs, rejects, nil
}
```

**驗證方式**:
```bash
# 執行模擬後檢查 Prometheus metrics
curl localhost:8080/metrics | grep screening_total
# 應顯示非零值
```

**相依任務**: Task 1.3（需先注入 MetricsCollector）

---

### 🔴 Task 1.3: 統一 MetricsCollector 實例
**嚴重程度**: 高 | **影響**: DashboardAPI 與主程式各自獨立

**問題描述**:
- `main.go:135` 建立一個 `MetricsCollector`
- `dashboard_api.go:109` 又建立另一個
- 兩個實例完全獨立，DashboardAPI 的永遠是空的

**實作內容**:
```go
// internal/monitoring/dashboard_api.go

// 修改: 接受外部 MetricsCollector
func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
    // ...
    return &DashboardAPI{
        // ...
        metricsCollector: metricsCollector,  // 使用傳入的實例
    }
}

// cmd/atlas/main.go
collector := monitoring.NewMetricsCollector()
// ...
dashboard := monitoring.NewDashboardAPI(cfg.WorkDir, cfg.LedgerDir, collector)  // 傳入同一個
```

**驗證方式**:
```bash
# 確認 DashboardAPI 使用的 collector 與主程式相同
# 可透過 log 或 debug endpoint 驗證
```

**相依任務**: 無

---

### 🔴 Task 1.4: 修復 Live Trading 警報持久化
**嚴重程度**: 高 | **影響**: Live 模式警報無法落地

**問題描述**:
- `main.go:471` 建立新的 `Monitor` 但沒有 `AlertStore`
- `RuleEngine` 使用這個 Monitor 觸發警報
- 警報只會 `log.Printf` 不會寫入檔案

**實作內容**:
```go
// cmd/atlas/main.go: runLiveTrading()
alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
if err != nil {
    log.Printf("[Alerts] failed to create alert store: %v", err)
} else {
    alertAPI := monitoring.NewAlertAPI(alertStore)
    alertAPI.RegisterRoutes(mux)
}

monitor := monitoring.NewMonitor()
if alertStore != nil {
    monitor.SetAlertStore(alertStore)  // 新增
}
```

**驗證方式**:
```bash
# 啟動 live trading 後觸發熔斷條件
# 檢查 alerts.jsonl 是否有記錄
cat data/state/alerts/alerts.jsonl | jq '.severity'
```

**相依任務**: Task 1.1

---

### 🟡 Task 1.5: 修復 Industry Cycle API 參數
**嚴重程度**: 中 | **影響**: `/api/industry/cycle` 永遠回傳 400

**問題描述**:
- `handleIndustryCycle` 要求 `industry` query parameter
- 前端 `loadIndustryData()` 未傳入此參數
- 導致 API 永遠回傳 `{"error":"industry parameter required"}`

**實作內容**:
```go
// 方案 A: 讓參數可選，回傳所有產業
func (a *DashboardAPI) handleIndustryCycle(w http.ResponseWriter, r *http.Request) {
    industryID := r.URL.Query().Get("industry")
    if industryID == "" {
        // 回傳所有產業的 cycle 資料
        var allPositions []map[string]interface{}
        for _, seg := range a.industryClassifier.GetAllSegments() {
            if pos, ok := a.cycleTracker.GetPosition(seg.ID); ok {
                allPositions = append(allPositions, map[string]interface{}{
                    "industry": seg.ID,
                    "phase": pos.BusinessCycle,
                    // ...
                })
            }
        }
        writeJSON(w, http.StatusOK, map[string]interface{}{
            "industries": allPositions,
        })
        return
    }
    // ... 原有單一產業邏輯 ...
}
```

**驗證方式**:
```bash
curl "localhost:8080/api/industry/cycle" | jq '.industries | length'
# 應回傳 > 0
```

**相依任務**: 無

---

### 🟡 Task 1.6: 移除/標記假資料端點
**嚴重程度**: 中 | **影響**: 使用者被誤導

**問題描述**:
- `/api/dashboard/tax-snapshot` 永遠回傳 `2330.TW` 的固定假資料
- `/api/dashboard/capital-phase` 回傳 `DefaultCapitalPhaseConfig()` 的靜態值
- 使用者可能誤以為這是真實資料

**實作內容**:
```go
// 方案 A: 標記為模擬資料
func (a *DashboardAPI) handleTaxSnapshot(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "symbol": "2330.TW",
        "transaction_tax": 150,
        "after_tax_pnl": 4850,
        "is_simulated": true,  // 新增標記
        "note": "此為示範資料，非真實交易記錄",
    })
}

// 方案 B: 從真實 ledger 計算（推薦）
func (a *DashboardAPI) handleTaxSnapshot(w http.ResponseWriter, r *http.Request) {
    // 讀取 ledger outcomes 計算實際稅務
    // 若無資料則回傳空陣列 + 說明
}
```

**驗證方式**:
```bash
curl localhost:8080/api/dashboard/tax-snapshot | jq '.is_simulated'
# 應顯示 true，或回傳空資料 + 說明
```

**相依任務**: 無

---

### 🟢 Task 1.7: 改善空狀態顯示（前端）
**嚴重程度**: 低 | **影響**: 使用者體驗

**問題描述**:
- 多數面板空狀態僅顯示「無資料」
- 缺乏操作指引（如「執行回測後即可看到」）

**實作內容**:
```javascript
// 為每個空狀態面板增加說明
function renderEmptyState(containerId, message, action) {
    const html = `
        <div class="empty" style="padding:20px;text-align:center">
            <div style="font-size:14px;color:var(--muted);margin-bottom:8px">${message}</div>
            <div style="font-size:12px;color:var(--accent)">${action}</div>
        </div>
    `;
    document.getElementById(containerId).innerHTML = html;
}

// 使用範例:
renderEmptyState('pipelinePanel', 
    '尚無回測場次資料', 
    '執行「go run ./cmd/run-experiment -brief <file>」後將自動顯示'
);
```

**驗證方式**:
- 手動檢查各頁面空狀態是否有操作指引

**相依任務**: 無

---

### 🟢 Task 1.8: 修復 Fugle 健康檢查
**嚴重程度**: 低 | **影響**: 資料通道狀態不準確

**問題描述**:
- `handleDataChannels` 中 Fugle 狀態僅檢查 API Key 是否存在
- 不反映真實 API 可用性

**實作內容**:
```go
// 新增實際健康檢查
func checkFugleHealth(apiKey string) (bool, string) {
    if apiKey == "" {
        return false, "API Key 未設定"
    }
    // 發送測試請求到 Fugle API
    resp, err := http.Get("https://api.fugle.tw/realtime/v0/health?apiKey=" + apiKey)
    if err != nil {
        return false, fmt.Sprintf("連線失敗: %v", err)
    }
    if resp.StatusCode != 200 {
        return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
    }
    return true, "正常"
}
```

**驗證方式**:
```bash
curl localhost:8080/api/dashboard/data-channels | jq '.channels[] | select(.name=="Fugle")'
# 應顯示實際狀態而非僅 "inactive"
```

**相依任務**: 無

---

## Phase 2: 資料補強（下個 Sprint）

### Task 2.1: 建立 MetricsStore（JSONL 持久化）
**目標**: 讓 MetricsCollector 的資料跨重啟保留

**實作內容**:
```go
// internal/monitoring/metrics_store.go

type MetricsStore struct {
    filePath string
    mu       sync.RWMutex
}

func NewMetricsStore(dir string) (*MetricsStore, error) {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, fmt.Errorf("create metrics store dir: %w", err)
    }
    return &MetricsStore{
        filePath: filepath.Join(dir, "metrics.jsonl"),
    }, nil
}

func (s *MetricsStore) Save(snapshot MetricsSnapshot) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if err != nil {
        return fmt.Errorf("open metrics file: %w", err)
    }
    defer f.Close()
    
    return json.NewEncoder(f).Encode(snapshot)
}

func (s *MetricsStore) LoadToday() (*MetricsSnapshot, error) {
    // 讀取今日最後一筆記錄，用於啟動時恢復
}
```

**整合點**:
```go
// MetricsCollector 啟動時載入
func NewMetricsCollector(store *MetricsStore) *MetricsCollector {
    mc := &MetricsCollector{
        metrics:      make(map[string]Metric),
        histograms:   make(map[string][]float64),
        alertsByType: make(map[string]int64),
        store:        store,
    }
    
    // 載入今日累計值
    if store != nil {
        if snapshot, err := store.LoadToday(); err == nil {
            mc.screeningTotal = snapshot.ScreeningTotal
            mc.screeningPassed = snapshot.ScreeningPassed
            mc.alertsTriggered = snapshot.AlertsTriggered
            mc.alertsAcknowledged = snapshot.AlertsAcknowledged
        }
    }
    
    return mc
}
```

**驗證方式**:
```bash
# 1. 啟動服務，執行一些操作
# 2. 重啟服務
# 3. 檢查指標是否保留
curl localhost:8080/api/dashboard/metrics | jq '.screening_total'
```

---

### Task 2.2: 建立警報規則引擎
**目標**: 讓警報可配置，而非硬編碼

**實作內容**:
```yaml
# configs/alert-rules.yaml
rules:
  - id: data_channel_stale
    name: 資料通道延遲
    condition:
      metric: data_channel.last_update_age
      operator: gt
      value: 300  # 5分鐘
    severity: warning
    message: "資料通道超過 5 分鐘未更新"
    
  - id: screening_rate_low
    name: 篩選率過低
    condition:
      metric: screening.rate
      operator: lt
      value: 0.1
    severity: warning
    message: "篩選率僅 {{value}}%，可能條件過嚴"
    
  - id: daily_loss_limit
    name: 單日虧損限制
    condition:
      metric: portfolio.daily_loss
      operator: gt
      value: 0.05
    severity: critical
    message: "單日虧損超過 5%"
    cooldown: 3600  # 1小時內不重複觸發
```

**驗證方式**:
```bash
# 修改規則後觸發條件
# 確認警報正確產生
curl localhost:8080/api/alerts | jq '.alerts[] | select(.rule=="screening_rate_low")'
```

---

### Task 2.3: 動態相關性計算
**目標**: 讓 CorrelationMatrix 反映真實市場

**實作內容**:
```go
// internal/industry/linkage.go

func (cm *CorrelationMatrix) CalculateFromMarketData(
    returns map[string][]float64,  // industry_id -> daily returns
    window int,
) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    for industryA, returnsA := range returns {
        for industryB, returnsB := range returns {
            if industryA >= industryB {
                continue
            }
            
            corr, err := pearsonCorrelation(returnsA, returnsB, window)
            if err != nil {
                continue
            }
            
            cm.updateCorrelationUnlocked(industryA, industryB, corr)
        }
    }
    
    return nil
}
```

**觸發時機**:
- 每日收盤後自動計算（background job）
- 或手動觸發 API

**驗證方式**:
```bash
# 觸發重新計算
curl -X POST localhost:8080/api/industry/linkage/recalculate

# 檢查相關係數是否更新
curl "localhost:8080/api/industry/linkage?industry=semiconductor" | jq '.correlations'
```

---

### Task 2.4: 產業別季節性篩選
**目標**: 讓季節性模式支援產業別查詢

**實作內容**:
```go
// /api/industry/seasonality?industry=semiconductor
func (a *DashboardAPI) handleIndustrySeasonality(w http.ResponseWriter, r *http.Request) {
    industryID := r.URL.Query().Get("industry")
    
    allPatterns := a.seasonalEngine.GetAllPatterns()
    var relevantPatterns []map[string]interface{}
    
    for _, p := range allPatterns {
        if industryID == "" || p.IsRelevantForIndustry(industryID) {
            impact := "neutral"
            for _, favored := range p.FavoredIndustries {
                if favored == industryID {
                    impact = "favored"
                    break
                }
            }
            for _, avoided := range p.AvoidedIndustries {
                if avoided == industryID {
                    impact = "avoided"
                    break
                }
            }
            
            relevantPatterns = append(relevantPatterns, map[string]interface{}{
                "id": p.ID,
                "name": p.Name,
                "impact": impact,
                "adjustment_factor": p.AdjustmentFactor,
                "historical_accuracy": p.HistoricalAccuracy,
            })
        }
    }
    
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "industry": industryID,
        "patterns": relevantPatterns,
    })
}
```

**驗證方式**:
```bash
curl "localhost:8080/api/industry/seasonality?industry=semiconductor" | jq '.patterns[].impact'
# 應顯示 favored / avoided / neutral
```

---

### Task 2.5: 建立 SeasonalPerformance 追蹤
**目標**: 記錄每年季節性模式的實際表現

**實作內容**:
```go
// internal/industry/seasonality_performance.go

type SeasonalPerformance struct {
    PatternID         string    `json:"pattern_id"`
    Year              int       `json:"year"`
    ActualReturn      float64   `json:"actual_return"`
    PredictedReturn   float64   `json:"predicted_return"`
    Accuracy          float64   `json:"accuracy"`  // 0-1
    FavoredIndustries []string  `json:"favored_industries"`
    RecordedAt        time.Time `json:"recorded_at"`
}

type SeasonalPerformanceStore struct {
    filePath string
}

func (s *SeasonalPerformanceStore) Record(perf SeasonalPerformance) error {
    // JSONL append
}

func (s *SeasonalPerformanceStore) GetRollingAccuracy(patternID string, years int) (float64, error) {
    // 計算過去 N 年的平均準確度
}
```

**驗證方式**:
```bash
# 每年結束後手動或自動記錄
# 查詢滾動準確度
curl "localhost:8080/api/industry/seasonality/performance?pattern=spring_festival&years=5"
```

---

### Task 2.6: 建立 LinkageHistory 追蹤
**目標**: 記錄產業連動分數的歷史變化

**實作內容**:
```go
// data/state/industry/linkage-history.jsonl
{"date":"2026-04-01","industry":"semiconductor","systemic_importance":0.85,"shock_propagation_speed":0.72,"avg_correlation":0.65}
{"date":"2026-04-15","industry":"semiconductor","systemic_importance":0.87,"shock_propagation_speed":0.75,"avg_correlation":0.68}
```

**前端顯示**:
- 趨勢圖（過去 30 天）
- 相關性熱力圖

**驗證方式**:
```bash
# 檢查歷史檔案
ls -la data/state/industry/linkage-history.jsonl
```

---

## Phase 3: 功能完善（3 週）

### Task 3.1: 衝擊模擬器 API
**目標**: 互動式衝擊傳導分析

**實作內容**:
```go
// POST /api/industry/shock-simulation
func (a *DashboardAPI) handleShockSimulation(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SourceIndustry string  `json:"source_industry"`
        ShockMagnitude float64 `json:"shock_magnitude"`  // e.g., -0.05 for 5% drop
        MaxDepth       int     `json:"max_depth"`
    }
    
    impacts := a.linkageAnalyzer.PropagateShock(req.SourceIndustry, req.ShockMagnitude, req.MaxDepth)
    
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "source": req.SourceIndustry,
        "shock": req.ShockMagnitude,
        "impacts": impacts,
    })
}
```

**前端互動**:
```javascript
// 使用者選擇產業與衝擊幅度
// 顯示受影響產業列表與預估影響程度
```

---

### Task 3.2: 供應鏈圖視覺化
**目標**: 節點圖顯示產業關係

**技術選型**:
- D3.js（force-directed graph）或
- Cytoscape.js（更適合生物/供應鏈網路）

**資料格式**:
```json
{
  "nodes": [
    {"id": "semiconductor", "name": "半導體", "systemic_importance": 0.85},
    {"id": "ai_supply_chain", "name": "AI供應鏈", "systemic_importance": 0.72}
  ],
  "edges": [
    {"source": "semiconductor", "target": "ai_supply_chain", "correlation": 0.85}
  ]
}
```

---

### Task 3.3: 警報通知渠道擴展
**目標**: 支援多種通知方式

**實作內容**:
```go
// internal/monitoring/notifier.go

type Notifier interface {
    Name() string
    IsConfigured() bool
    Notify(record domain.AlertRecord) error
}

// TelegramNotifier
// EmailNotifier  
// WebhookNotifier（Slack/Discord/企業微信）
// SMSNotifier（Critical 級別）
```

**配置**:
```yaml
# configs/notifications.yaml
channels:
  telegram:
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"
  
  webhook:
    url: "https://hooks.slack.com/services/..."
    headers:
      Content-Type: application/json
    
  email:
    smtp_host: smtp.gmail.com
    smtp_port: 587
    from: alerts@atlas-go.com
    to: ["admin@company.com"]
```

---

### Task 3.4: 指標趨勢視覺化
**目標**: 顯示歷史趨勢圖

**前端實作**:
```javascript
// 使用 Chart.js 或 ApexCharts
function renderMetricsTrend(containerId, metricName, data) {
    // 折線圖顯示過去 24h / 7d / 30d
    // 標註異常點（偏離平均值 2σ）
}
```

**API**:
```go
// /api/dashboard/metrics/trend?metric=screening_rate&period=7d
func (a *DashboardAPI) handleMetricsTrend(w http.ResponseWriter, r *http.Request) {
    metric := r.URL.Query().Get("metric")
    period := r.URL.Query().Get("period")  // 24h, 7d, 30d
    
    trend := a.metricsHistory.GetTrend(metric, parsePeriod(period))
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "metric": metric,
        "period": period,
        "data": trend,
    })
}
```

---

### Task 3.5: 產業詳細分析彈窗
**目標**: 點擊產業卡片顯示詳細分析

**已完成**: ✅ 
- 4 個 Tab（週期定位、供應鏈、季節性、風險）
- 並行呼叫 4 個 API
- CSS 樣式與互動

**待補強**:
- [ ] 衝擊模擬互動（整合 Task 3.1）
- [ ] 歷史趨勢圖（整合 Task 3.4）
- [ ] 相關性熱力圖（整合 Task 2.3）

---

## Phase 4: 體驗優化（2 週）

### Task 4.1: 分頁延遲載入
**目標**: 減少初始載入時間

**實作內容**:
```javascript
// 目前: 一次載入 21 個 API
// 改為: 切換到該頁才載入

function switchPage(id) {
    // ... 現有邏輯 ...
    
    // 延遲載入該頁資料
    if (id === 'industry' && !window.industryLoaded) {
        loadIndustryData();
        window.industryLoaded = true;
    }
}
```

---

### Task 4.2: 網路錯誤全局通知
**目標**: 取代靜默失敗

**實作內容**:
```javascript
async function getJSON(url) {
    try {
        const res = await fetch(url);
        if (!res.ok) {
            throw new Error(`HTTP ${res.status}`);
        }
        return await res.json();
    } catch (err) {
        notify(`載入失敗: ${url} - ${err.message}`, 'error');
        throw err;
    }
}
```

---

### Task 4.3: 空狀態操作指引
**目標**: 每個空狀態都有明確的下一步

**實作內容**:
```javascript
const EMPTY_STATE_GUIDES = {
    'pipelinePanel': {
        message: '尚無回測場次資料',
        action: '執行「go run ./cmd/run-experiment -brief <file>」後將自動顯示'
    },
    'alertsPanel': {
        message: '目前沒有警報',
        action: '系統運行正常時不會產生警報。當資料通道延遲或風險超過閾值時會自動觸發。'
    },
    // ... 其他面板
};
```

---

### Task 4.4: 回測輪詢 UX 改善
**目標**: 減少使用者等待焦慮

**實作內容**:
```javascript
function pollBacktestStatus() {
    const maxAttempts = 60;
    let attempts = 0;
    
    const interval = setInterval(async () => {
        attempts++;
        const progress = Math.min((attempts / maxAttempts) * 100, 95);
        
        // 顯示進度條而非無限轉圈
        updateProgressBar(progress);
        
        if (attempts >= maxAttempts) {
            clearInterval(interval);
            showError('回測執行時間過長，請檢查伺服器日誌或稍後再試');
        }
    }, 3000);
}
```

---

## Phase 5: 長期架構（Q3 規劃）

### Task 5.1: 評估時序資料庫整合
**選項**:
| 方案 | 優點 | 缺點 | 建議 |
|------|------|------|------|
| Prometheus | 生態成熟、Grafana 整合 | 需要額外服務 | ⭐ 推薦 |
| InfluxDB | 專用時序資料庫 | 需要額外服務 | 可選 |
| TimescaleDB | 與 PostgreSQL 整合 | 複雜度高 | 若已有 PG |
| 繼續 JSONL | 簡單、無依賴 | 查詢效率低 | 短期 |

**決策點**: 當 JSONL 檔案大小 > 100MB 或查詢延遲 > 1 秒時，評估遷移

---

### Task 5.2: 機器學習增強季節性預測
**目標**: 動態調整 adjustment factor

**概念**:
```python
# 基於多年 SeasonalPerformance 資料訓練
# 輸入: 當年宏觀敘事、產業營收、歷史模式準確度
# 輸出: 預測的 adjustment_factor

model = SeasonalStrengthPredictor()
model.train(historical_data)

prediction = model.predict(
    pattern="spring_festival",
    year=2027,
    macro_context={"vix": 25, "us_rates": "rising"}
)
# => {"predicted_accuracy": 0.68, "suggested_adjustment": 1.12}
```

---

### Task 5.3: 自動化資料品質監控
**目標**: 監控各資料來源的健康度

**實作內容**:
```go
// 定期檢查各資料來源
func (a *DashboardAPI) runDataQualityChecks() {
    checks := []DataQualityCheck{
        {Name: "macro_snapshot", Check: checkMacroSnapshotFreshness},
        {Name: "ledger_sessions", Check: checkLedgerSessionsCount},
        {Name: "industry_cycle", Check: checkCycleDataFreshness},
        {Name: "correlation_matrix", Check: checkCorrelationMatrixAge},
    }
    
    for _, check := range checks {
        result := check.Check()
        if result.Status == "fail" {
            a.monitor.Alert(monitoring.AlertLevelWarning, "data_quality", 
                fmt.Sprintf("%s: %s", check.Name, result.Message), nil)
        }
    }
}
```

---

## 附錄 A: 已完成的改善

以下項目已在本次對話中完成：

1. ✅ **系統警報頁面**: 添加說明文字，解釋為何沒有警報
2. ✅ **指標監控頁面**: 添加記憶體內累計值的說明
3. ✅ **供應鏈連動**: 添加數據說明區塊與統計摘要（平均/最高值）
4. ✅ **季節性模式**: 
   - 擴展 API 回傳所有模式（`all_patterns`）
   - 新增 `/api/industry/seasonality/calendar` 端點
   - 前端支援列表/日曆雙視圖切換

---

## 附錄 B: 相依關係圖

```
Phase 1 (本週)
├── Task 1.1: 連結 Monitor ↔ AlertStore
├── Task 1.2: Orchestrator 指標收集 [依賴: 1.3]
├── Task 1.3: 統一 MetricsCollector 實例
├── Task 1.4: Live Trading 警報持久化 [依賴: 1.1]
├── Task 1.5: 修復 Industry Cycle API
├── Task 1.6: 標記假資料
├── Task 1.7: 改善空狀態顯示
└── Task 1.8: 修復 Fugle 健康檢查

Phase 2 (下個 Sprint)
├── Task 2.1: MetricsStore 持久化
├── Task 2.2: 警報規則引擎 [依賴: 1.1]
├── Task 2.3: 動態相關性計算
├── Task 2.4: 產業別季節性篩選
├── Task 2.5: SeasonalPerformance 追蹤
└── Task 2.6: LinkageHistory 追蹤

Phase 3 (3 週)
├── Task 3.1: 衝擊模擬器 [依賴: 2.3]
├── Task 3.2: 供應鏈圖視覺化 [依賴: 2.3]
├── Task 3.3: 警報通知渠道 [依賴: 2.2]
├── Task 3.4: 指標趨勢視覺化 [依賴: 2.1]
└── Task 3.5: 產業詳細彈窗補強

Phase 4 (2 週)
├── Task 4.1: 分頁延遲載入
├── Task 4.2: 網路錯誤通知
├── Task 4.3: 空狀態指引
└── Task 4.4: 回測輪詢 UX

Phase 5 (Q3)
├── Task 5.1: 時序資料庫評估 [依賴: 2.1]
├── Task 5.2: ML 季節性預測 [依賴: 2.5]
└── Task 5.3: 資料品質監控 [依賴: 2.2]
```

---

## 附錄 C: 驗證檢查清單

### 每個 Task 完成後必須驗證：

- [ ] **Go 編譯通過**: `go build ./...`
- [ ] **測試通過**: `go test ./...`
- [ ] **格式化檢查**: `test -z "$(gofmt -l .)"`
- [ ] **API 測試**: 手動呼叫 API 確認回傳正確
- [ ] **前端驗證**: 瀏覽器檢查顯示正常
- [ ] **文件更新**: 更新相關 SKILL.md 或 AGENTS.md

### Phase 完成後必須驗證：

- [ ] **整合測試**: `go test -v -tags=integration ./...`
- [ ] **效能測試**: 確認 API 延遲 < 500ms
- [ ] **資料完整性**: 確認 JSONL 檔案正確寫入
- [ ] **使用者體驗**: 手動操作所有頁面確認無異常

---

**任務清單建立者**: Sisyphus (AI Agent)  
**審閱建議**: 請團隊確認優先順序與時程是否符合產品路線圖，並指派負責人
