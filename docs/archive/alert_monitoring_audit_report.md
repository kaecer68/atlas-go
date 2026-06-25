# Atlas-Go 系統警報與指標監控盤查報告

**分支**: `feat/alert-monitoring-audit-20260521`  
**日期**: 2026-05-21  
**盤查範圍**: 【系統警報】與【指標監控】前後台功能  
**對標基準**: 宏觀敘事、產業生態系、投資管線、決策鏈、決策追蹤等已更新板塊的統一架構

---

## 一、盤查摘要

| 檢查項目 | 狀態 | 說明 |
|---------|------|------|
| 前端頁面存在 | ✅ | `alerts.js`、`metrics.js`、`risk.js` 均存在 |
| 後端 API 存在 | ✅ | `/api/alerts`、`/api/dashboard/metrics`、`/api/dashboard/risk` 等 |
| 參數統一管理 (ParametersConfig) | ⚠️ **部分缺失** | 風控閾值已參數化，但警報規則閾值仍硬編碼 |
| 背景任務統一排程 (BackgroundTaskManager) | ⚠️ **警報未接入** | 資料抓取已統一，但警報觸發檢查仍使用獨立 goroutine |
| 決策鏈透明化 | ❌ **缺失** | 警報觸發原因無 breakdown，風控計算無詳細步驟 |
| 資料來源統一 (Gateway) | ⚠️ **部分繞過** | 部分監控指標直接讀取檔案，未通過 Gateway |
| 測試覆蓋 | ⚠️ **不足** | 警報規則邏輯缺乏系統性測試 |

---

## 二、【系統警報】詳細盤查

### 2.1 前端頁面

**檔案**: `web/static/js/pages/alerts.js` (50 行)

- **功能**: 顯示警報列表、嚴重度分類、確認操作
- **API 呼叫**:
  - `GET /api/alerts` — 取得所有警報
  - `POST /api/alerts/acknowledge` — 確認警報
- **顯示欄位**: 時間、嚴重度、規則、訊息、數值、操作
- **限制**:
  - 無警報趨勢圖表
  - 無警報統計分析（僅基礎計數）
  - 無警報觸發條件說明（使用者無法理解為何觸發）
  - 無警報歷史查詢或篩選功能

### 2.2 後端 API

**檔案**: `internal/monitoring/alert_api.go` (79 行)

| Endpoint | Method | 功能 |
|---------|--------|------|
| `/api/alerts` | GET | 列出所有警報 |
| `/api/alerts/unacknowledged` | GET | 列出未確認警報 |
| `/api/alerts/acknowledge` | POST | 確認警報 |

**AlertStore** (`internal/monitoring/alert_store.go`):
- JSONL append-only 持久化
- 支援讀取全部、讀取未確認、確認操作
- 無分頁、無時間範圍查詢

### 2.3 警報觸發機制

**Monitor** (`internal/monitoring/monitor.go`):
- 四級嚴重度: INFO / WARNING / ERROR / CRITICAL
- 支援 Handler 註冊（回調函數）
- 支援 Notifier（通知派發）
- 歷史記錄上限 1000 條（記憶體）
- 持久化到 AlertStore（JSONL）

**RuleEngine** (`internal/monitoring/rules.go`):
- 基於 `livestore.State` 的規則評估
- 預設規則:
  - `portfolio_value_drop`: 現金 < 100,000（硬編碼）
  - `position_concentration`: 持倉 > 20 檔（硬編碼）
  - `system_ready`: 系統初始化（資訊級）
  - `circuit_breaker_triggered`: 日損益 < -2%（硬編碼）
  - `daily_loss_warning`: 日損益 -1.5% ~ -2%（硬編碼）
  - `high_position_concentration`: 單一持倉 > 15%（硬編碼）

### 2.4 架構問題

| 問題 | 嚴重度 | 說明 |
|------|--------|------|
| 閾值硬編碼 | 🔴 **高** | 所有警報閾值均為 magic number，未使用 ParametersConfig |
| 獨立 goroutine | 🔴 **高** | RuleEngine 使用 `time.Ticker` 獨立運行，未接入 BackgroundTaskManager |
| 無決策鏈透明 | 🟡 **中** | 警報觸發時無詳細 breakdown（類似 ConvictionBreakdown） |
| 無參數校準 | 🟡 **中** | 閾值無法通過回測自動校準 |
| 資料來源不一致 | 🟡 **中** | 部分規則讀取 `livestore.State`，部分直接計算 |
| 無警報抑制機制 | 🟡 **中** | 僅有 cooldown，無智能抑制（如市場異常波動期間暫停非關鍵警報） |

---

## 三、【指標監控】詳細盤查

### 3.1 前端頁面

**檔案**: `web/static/js/pages/metrics.js` (74 行)

- **功能**: 顯示篩選率、警報觸發數、資金階段、儲存清理統計
- **API 呼叫**:
  - `GET /api/dashboard/metrics?type=all`
  - `GET /api/dashboard/capital-phase`
  - `GET /api/metrics/storage`
- **顯示內容**:
  - 警報類型分佈（簡易表格）
  - 篩選統計（總數/通過/拒絕）
  - 儲存清理記錄
- **限制**:
  - 無趨勢圖表（僅文字表格）
  - 無關鍵風控指標（VaR、回撤、夏普比率等）
  - 無資料品質評分趨勢
  - 無通道健康度視覺化

### 3.2 風控指標頁面

**檔案**: `web/static/js/pages/risk.js` (221 行)

- **功能**: 顯示即時狀態、風險卡片、持倉集中度、板塊曝險
- **API 呼叫**:
  - `GET /api/dashboard/risk` — VaR、CVaR、最大回撤
  - `GET /api/dashboard/capital-phase` — 資金階段
  - `GET /api/dashboard/circuit-breaker` — 熔斷狀態
- **顯示指標**:
  - VaR 95%、VaR 99%、CVaR 95%
  - 最大回撤、Rolling Sharpe
  - 投組淨值、資金階段、持倉數
  - 持倉集中度（前 5 大、前 3 大、最大）
  - 板塊曝險分布（長條圖）

### 3.3 後端 API 結構

**DashboardAPI** (`internal/monitoring/dashboard_api.go`, 794 行):

| 路由群組 | 檔案 | 功能 |
|---------|------|------|
| `/api/dashboard/*` | `dashboard_api.go` | 核心儀表板 |
| `/api/dashboard/risk` | `api/risk/handlers.go` | 風險指標（VaR、回撤） |
| `/api/dashboard/metrics` | `api/metrics/handlers.go` | 系統指標 |
| `/api/dashboard/system-health` | `api/system/handlers.go` | 系統健康度 |
| `/api/dashboard/capital-phase` | `api/system/handlers.go` | 資金階段 |
| `/api/dashboard/circuit-breaker` | `api/circuitbreaker/handlers.go` | 熔斷機制 |
| `/api/dashboard/performance-report` | `api/performance/handlers.go` | 績效報告 |
| `/api/macro/*` | `api/macro/handlers.go` | 宏觀數據 |
| `/api/scheduler/*` | `api/scheduler/handlers.go` | 排程管理 |

### 3.4 風控計算邏輯

**VaR / CVaR 計算** (`internal/monitoring/api/risk/handlers.go`):
- 從 session summary 讀取投組淨值歷史
- 計算日報酬率
- 需要 ≥30 個資料點才計算（否則回傳 "資料不足"）
- 使用 `risk.ComputeRiskSnapshot()` 進行計算

**風險參數化狀態**:
- ✅ VaR 信賴水準: `ParametersConfig.Risk.VaRConfidenceLevel`
- ✅ 最大回撤限制: `ParametersConfig.Risk.MaxDrawdownPct`
- ✅ 日損失限制: `ParametersConfig.Risk.MaxDailyLossPct`
- ✅ 停損/停利: `ParametersConfig.Risk.StopLoss / TakeProfit`
- ❌ 警報閾值: 硬編碼在 `rules.go`

### 3.5 架構問題

| 問題 | 嚴重度 | 說明 |
|------|--------|------|
| 指標趨勢無圖表 | 🟡 **中** | 前端僅文字表格，無視覺化趨勢 |
| 資料品質檢查簡化 | 🟡 **中** | `CheckDataQuality()` 回傳固定 100 分，無實際檢查 |
| 風險指標計算依賴檔案 | 🟡 **中** | 直接讀取 `sessions/*/summary.json`，未通過統一資料層 |
| 無宏觀風險等級顯示 | 🟡 **中** | 前端未顯示 MacroRiskLevel（綠/黃/橙/紅） |
| 無結構性趨勢覆蓋指示 | 🟡 **中** | 未顯示 StructuralOverride 狀態 |
| 熔斷機制獨立於風控 | 🟡 **中** | CircuitBreaker 與 MacroAwareDrawdown 未整合 |

---

## 四、統一架構對齊檢查

### 4.1 參數統一管理 (ParametersConfig)

**已對齊**:
- ✅ 風控參數: `RiskParameters`（VaR、回撤、停損等）
- ✅ 回撤參數: `DrawdownParameters`（五級回撤、覆蓋閾值）
- ✅ 敘事參數: `NarrativeParameters`（事件偵測閾值、壓力指數權重）
- ✅ 產業參數: `IndustryParameters`（季節性、週期、風險評分）

**未對齊**:
- ❌ 警報閾值: `rules.go` 中所有閾值均為硬編碼
- ❌ 警報冷卻時間: 硬編碼 `5 * time.Minute` 等
- ❌ 資料品質檢查標準: 無參數化

### 4.2 背景任務統一排程 (BackgroundTaskManager)

**已對齊**:
- ✅ 資料抓取: `auto_backfill`、`auto_capital_flow`、`auto_margin`、`auto_export`、`auto_geopolitical` 等
- ✅ 通道健康同步: `channel_health_sync`
- ✅ 任務失敗告警: `taskMgr.SetFailureHandler()` 整合 Monitor

**未對齊**:
- ❌ 警報規則檢查: RuleEngine 使用獨立 goroutine + time.Ticker
- ❌ 風險指標計算: 無定時重新計算任務
- ❌ 資料品質檢查: 無定時檢查任務

### 4.3 資料統一管理 (Gateway)

**已對齊**:
- ✅ 宏觀資料: 通過 `marketdata.Provider` 介面
- ✅ 產業資料: 通過 `IndustryService`

**未對齊**:
- ⚠️ 部分監控直接讀檔: `risk/handlers.go` 直接讀取 `sessions/*/summary.json`
- ⚠️ 系統健康檢查直接讀檔: `system/handlers.go` 直接讀取多個 state 目錄

### 4.4 決策鏈透明化

**已對齊板塊**:
- ✅ 投資管線: `FactorScores` + `ConvictionBreakdown`
- ✅ 產業生態系: `AdjustmentBreakdown`（四層分解）
- ✅ 決策鏈: `GuardOutcomes` 詳細記錄

**未對齊**:
- ❌ 警報觸發: 無 breakdown（僅有 rule name 和 message）
- ❌ 風險計算: 無詳細步驟（如 VaR 計算方法、資料點數量、信賴區間）
- ❌ 回撤決策: 無 `MacroAwareDrawdownDecision` 的詳細分解前端顯示

---

## 五、風控計算驗證

### 5.1 VaR / CVaR 計算

**實作**: `internal/risk/var_calculator.go`

- 使用歷史模擬法（Historical Simulation）
- 95% 和 99% 信賴水準
- 需要 ≥30 個資料點
- 從 session portfolio value 計算日報酬

**驗證結果**:
- ✅ 計算方法正確（歷史模擬法為業界標準）
- ✅ 資料點不足時回傳 "資料不足"（避免誤導）
- ⚠️ 無壓力測試情境（如 2008 金融危機、2020 疫情）
- ⚠️ 無模型風險調整（歷史模擬法假設過去能代表未來）

### 5.2 回撤防護

**實作**: `internal/risk/macro_aware_drawdown.go`

- 五級回撤: None / Light / Moderate / Severe / Emergency
- 宏觀風險等級: Green / Yellow / Orange / Red
- 結構性趨勢覆蓋機制
- 產業輪動約束

**驗證結果**:
- ✅ 三層決策架構完整（宏觀風險 → 結構性趨勢 → 動態執行）
- ✅ 覆蓋機制邏輯正確（Orange/Red 時檢查 structural score）
- ✅ 參數已統一管理（`DrawdownParameters`）
- ⚠️ 無歷史回測驗證（未驗證各等級閾值在歷史情境下的表現）
- ⚠️ 無前端顯示（使用者無法看到當前回撤決策的詳細推理）

### 5.3 熔斷機制

**實作**: `internal/monitoring/api/circuitbreaker/handlers.go`

- 獨立於 MacroAwareDrawdown 運作
- 可手動重置
- 狀態查詢

**驗證結果**:
- ⚠️ 與回撤決策引擎未整合（應該根據 MacroRiskLevel 自動調整熔斷閾值）
- ⚠️ 無自動恢復機制（需手動重置）
- ⚠️ 無歷史觸發記錄分析

---

## 六、迭代建議

### 6.1 高優先級（立即執行）

#### 1. 警報閾值參數化
**檔案**: `internal/monitoring/rules.go`、`internal/config/parameters.go`

```go
// 新增 AlertParameters 到 ParametersConfig
type AlertParameters struct {
    CashLowThreshold          ParameterMetadata[float64] `json:"cash_low_threshold"`
    PositionCountMax          ParameterMetadata[int]     `json:"position_count_max"`
    DailyLossWarningPct       ParameterMetadata[float64] `json:"daily_loss_warning_pct"`
    DailyLossCriticalPct      ParameterMetadata[float64] `json:"daily_loss_critical_pct"`
    PositionConcentrationPct  ParameterMetadata[float64] `json:"position_concentration_pct"`
    AlertCooldownMinutes      ParameterMetadata[int]     `json:"alert_cooldown_minutes"`
}
```

**影響**: 消除所有 magic number，支援動態調整和回測校準。

#### 2. 警規則引擎接入 BackgroundTaskManager
**檔案**: `cmd/atlas/main.go`、`internal/monitoring/rules.go`

```go
// 在 main.go 中註冊
taskMgr.Register(&apigateway.ScheduledTask{
    Name:     "alert_rule_evaluation",
    Interval: 30 * time.Second,
    Enabled:  true,
    Task: func(ctx context.Context) error {
        return ruleEngine.Evaluate(ctx, stateStore)
    },
})
```

**影響**: 統一 goroutine 生命週期管理，避免資源洩漏。

#### 3. 警報觸發透明化（AlertBreakdown）
**檔案**: `internal/monitoring/rules.go`、`internal/domain/types.go`、`web/static/js/pages/alerts.js`

```go
// 新增 AlertBreakdown
type AlertBreakdown struct {
    Rule        string            `json:"rule"`
    Condition   string            `json:"condition"`
    CurrentValue float64          `json:"current_value"`
    Threshold   float64           `json:"threshold"`
    Operator    string            `json:"operator"`  // ">", "<", "=="
    Steps       []AlertStep       `json:"steps"`
}

type AlertStep struct {
    Name   string  `json:"name"`
    Value  float64 `json:"value"`
    Pass   bool    `json:"pass"`
    Reason string  `json:"reason"`
}
```

**影響**: 使用者可理解警報為何觸發，提升信任度。

### 6.2 中優先級（近期規劃）

#### 4. 風險指標趨勢圖表
**檔案**: `web/static/js/pages/risk.js`、`web/static/js/pages/metrics.js`

- 整合 Chart.js 或類似函式庫
- 顯示 VaR、回撤、Sharpe 的時間序列
- 標註宏觀風險等級變化點

#### 5. 宏觀風險等級前端顯示
**檔案**: `web/static/js/pages/risk.js`、`internal/monitoring/api/macro/handlers.go`

- 在風控頁面顯示當前 MacroRiskLevel（綠/黃/橙/紅）
- 顯示各維度評分（美元、美債、日圓、匯率、商品、地緣政治）
- 顯示結構性覆蓋狀態

#### 6. 資料品質檢查實作
**檔案**: `internal/monitoring/service/metrics.go`

```go
func (dq *dataQualityChecker) RunAll(ctx context.Context) *DataQualityReport {
    report := &DataQualityReport{...}
    
    // 實際檢查
    report.Checks = append(report.Checks, dq.checkReplayDataFreshness())
    report.Checks = append(report.Checks, dq.checkMacroDataCompleteness())
    report.Checks = append(report.Checks, dq.checkBaselinePolicyLoaded())
    report.Checks = append(report.Checks, dq.checkAgentPromptsExist())
    report.Checks = append(report.Checks, dq.checkChannelHealth())
    
    // 計算總分
    report.Score = dq.calculateScore(report.Checks)
    report.Overall = dq.determineOverallStatus(report.Score)
    
    return report
}
```

#### 7. 熔斷機制與回撤決策整合
**檔案**: `internal/monitoring/api/circuitbreaker/handlers.go`、`internal/risk/macro_aware_drawdown.go`

- 根據 MacroRiskLevel 自動調整熔斷閾值
- Red 等級時自動觸發熔斷
- 提供熔斷歷史記錄和觸發原因

### 6.3 低優先級（長期規劃）

#### 8. 警報智能抑制
- 市場異常波動期間（VIX > 30）暫停非關鍵警報
- 根據 MacroRiskLevel 調整警報靈敏度

#### 9. 警報回測驗證
- 使用歷史資料驗證警報閾值的有效性
- 計算警報的命中率、誤報率
- 自動校準閾值

#### 10. 多維度監控儀表板
- 整合所有監控指標於單一頁面
- 支援自定義時間範圍
- 支援指標對比分析

---

## 七、風險評估

| 風險 | 影響 | 機率 | 緩解措施 |
|------|------|------|---------|
| 警報閾值過時 | 誤報/漏報 | 高 | 參數化 + 定期校準 |
| 獨立 goroutine 洩漏 | 資源耗盡 | 中 | 接入 BackgroundTaskManager |
| 風險指標計算錯誤 | 錯誤決策 | 低 | 增加單元測試 + 資料點驗證 |
| 前端顯示不完整 | 使用者誤解 | 中 | 增加趨勢圖表 + 詳細說明 |
| 資料品質檢查無效 | 髒資料進入系統 | 中 | 實作實際檢查邏輯 |

---

## 八、結論

【系統警報】和【指標監控】板塊的**基礎功能已存在**，但與其他已更新板塊（宏觀敘事、產業生態系、投資管線等）相比，存在明顯的**架構落差**:

1. **參數管理**: 警報閾值未參數化，違反統一架構規範
2. **背景任務**: 警報規則檢查未接入 BackgroundTaskManager
3. **決策透明**: 缺乏類似 ConvictionBreakdown 的警報觸發分解
4. **前端體驗**: 缺乏趨勢圖表和詳細指標說明

**建議優先執行高優先級項目（1-3）**，這些變更相對獨立、影響可控，且能顯著提升系統的稽核能力和可維護性。

---

*報告產生時間: 2026-05-21*  
*分支: feat/alert-monitoring-audit-20260521*
