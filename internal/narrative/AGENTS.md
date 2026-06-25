# AGENTS.md — internal/narrative

本目錄負責**巨集觀敘事（Macro Narrative）**事件偵測與因果鏈（Causal Chain）推導。

---

## OVERVIEW

`internal/narrative` 透過監控全球總經指標（美債、匯率、VIX、原油、黃金）與特定產業情緒，將原始數據轉化為具備信心度與命中率的領域事件，並藉由因果範本推論對台股各板塊的潛在影響。

### 核心資料流
`MacroIngestor (MarketData) → NarrativeEvent → KnowledgeBase (Match Template) → CausalChain`

### Bundle API 實時數據流
前端【宏觀敘事】頁面透過 `/api/narrative/bundle` 取得事件、因果鏈、模型與季節性分析。該 endpoint 已接入 `marketdata.MacroDataProvider`。

### News Sentiment 數據策略
**重要限制**：Finnhub News Sentiment API **僅支援美股公司**，台股無法直接使用。實作策略：
1. 美股大盤作為代理指標（NASDAQ、S&P 500 新聞情緒）
2. 外資流向推斷：美股下跌 + VIX 上升 → 外資可能撤離台股
3. 台股本地數據：TWSE 開放資料作為本地情緒代理指標

---

## EVENT TYPES

### NarrativeEvent 結構
- `Theme`: 事件主題標籤（如 `US_rates_up`, `AI_capex_surge`）。
- `Confidence`: 信心度 `[0.0, 1.0]`；`ConfidenceSource`: 來源標記。
- `HitRate`: 歷史回測命中率；`SourceData`: 觸發時原始數值快照。
- `Duration` / `ExpiresAt`: 影響持續時間與過期時間點。
- `Severity`: `low`/`medium`/`high`/`critical`；`Status`: `active`/`confirmed`/`faded`/`expired`。

### Event 狀態機
```
active → confirmed → faded → expired
  ↓
（可直接跳轉）
```

| 狀態 | 轉換條件 |
|------|----------|
| `active` | 信心度 > 閾值 |
| `confirmed` | 2+ 獨立數據源確認 |
| `faded` | 時間経過 > Duration × 0.8 |
| `expired` | 時間経過 > ExpiresAt |

### Severity 等級
| 等級 | 因子權重調整 |
|------|--------------|
| `low` | ±5% |
| `medium` | ±10% |
| `high` | ±20% |
| `critical` | ±30%，立即觸發 RegimeChange |

### 內建主題與命中率
所有主題命中率由 `DefaultTemplates()` 提供，`hitRateForTheme()` 自動查表。完整主題列表與持續時間定義見 `internal/narrative/templates.go`（或對應實作檔）。

---

## EVENT LIFECYCLE MANAGEMENT

1. 每個 `NarrativeEvent` 建立時根據 Theme 自動設定 `Duration` 與 `ExpiresAt`。
2. `EventLifecycleManager` 追蹤活躍事件，定期更新狀態。
3. 事件從 `active` 轉為 `faded` 時，`FactorWeightEngine` 收到通知，開始漸進式回調權重。
4. 事件過期時，從 `FactorWeightEngine` 活躍事件列表移除。

### RegimeChange 觸發條件
- VIX 突破 30（HighVol）
- VIX 突破 25 且趨勢向下（Bear）
- VIX 跌破 15 且趨勢向上（Bull）
- `critical` 等級事件觸發
- StressIndex 突破 80

---

## ANTI-PATTERNS

- **手動計算 HitRate**: `HitRate` 必須透過 `hitRateForTheme()` 從 `DefaultTemplates` 取得，不可在 detector 中硬編碼。
- **遺漏 SourceData**: 每個 `NarrativeEvent` 必須包含觸發時的 `SourceData`。
- **無視 Region 限制**: 因果鏈匹配時會檢查 `RequiredRegion`。
- **直接修改模型權重**: `InvestmentModel` 權重更新應由 `UpdateModelWeights` 統一處理。
- **手動設定 Status**: Status 轉換應由 `EventLifecycleManager` 統一管理，detector 僅初始化為 `active`。
- **事件重複偵測**: 相同 Theme 在 active 狀態時應更新現有事件 Confidence，而非建立新事件。
- **遺漏 causal chain 方向性**: `CausalChain` 已新增 `FavoredSectors` 與 `AvoidedSectors`，前端應使用這兩個欄位。

---

## KEY TYPES

| 結構體 | 檔案 | 用途 |
|--------|------|------|
| `NarrativeEvent` | types.go | 領域事件結構 |
| `KnowledgeBase` | knowledge_base.go | 因果範本匹配 |
| `CausalChain` | types.go | 因果鏈推導（含方向性板塊分類） |
| `MacroIngestor` | ingestor.go | 巨集觀數據攝入 |
| `EventLifecycleManager` | lifecycle.go | 事件生命週期管理 |
| `TaiwanStressIndex` | taiwan_stress_index.go | 台灣壓力指數計算 |
| `SeasonalBridge` | seasonal_bridge.go | 橋接敘事主題至產業供應鏈連動 |
| `MarketNarrativeDataFromSnapshot` | snapshot_converter.go | 將 `MacroDataSnapshot` 轉換為 `MarketNarrativeData` |

## 敘事感知相關矩陣調變

`SeasonalBridge` 實作 `NarrativeLinkageProvider` 介面，使巨集觀敘事主題能動態調整產業間相關矩陣。內建 5 個主題乘數（`oil_price_shock`、`AI_capex_surge`、`US_rates_up`、`JPY_carry_unwind`、`geopolitical_risk_spike`），無配置時自動降級為空列表。

---

## ROLLING CALIBRATION FRAMEWORK

台灣壓力指數自動校準框架，詳見 `docs/MACRO_CALIBRATION.md`。

### 五層架構
1. **Baseline**（`calibration_baseline.go`）：60d rolling Mean/Count 統計
2. **Scale**（`calibration_scales.go`）：自動調整使各因子貢獻相當
3. **Regime**（`calibration_regime.go`）：VIX-based 切換 bull/normal/bear/crisis
4. **Validation**（`calibration_validation.go`）：80/20 split，hit-rate 退化則跳過 export
5. **Scheduler**（`internal/scheduler/auto_calibration.go`）：Maturity-gated 每日觸發

### 核心規則
- **Hybrid Signal 是預設**：`TaiwanStressIndex.Calculate` 使用 `max(|level_z|, |change_pct|)`。
- **`FactorBaseline` 不存 Baseline 欄位**：只存 `Mean` 與 `Count`，z-score 使用時即時計算。
- **`BaselineConfig` 是 map**：新增 factor 不需修改 struct 定義。
- **`ValidateCalibration` 是獨立函式**：鬆耦合、可獨立測試。
- **`CalibrationTask` 沒有 goroutine**：背景排程一律交給 `BackgroundCalibrationScheduler.RunDaily`（由 `BackgroundTaskManager` 註冊）。
- **校準參數一律取自 `ParametersConfig`**：禁止 hardcode。
- **`calibration_enabled` 預設 false**：啟用前需在 staging 驗證至少 30 日。

### Maturity-Gated 行為

| 系統成熟度 | 行為 |
|----------|------|
| `BURN_IN` | log `burn_in_skip`、完全跳過校準 |
| `CALIBRATING` | 執行校準 + validation gate |
| `FULL_AUTO` | 執行校準 + validation gate |

### 驗證失敗（Degradation）處理
`ValidateCalibration` 回傳 `IsDegradation: true` 時：不寫入新 config、log warning、當日 `Calculate` 仍用舊 config。

---

## 測試與驗證

```bash
go test -v ./internal/narrative/...
go test -v ./internal/narrative/calibration_baseline_test.go
```
