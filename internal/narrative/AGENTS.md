# AGENTS.md — internal/narrative

本目錄負責**巨集觀敘事（Macro Narrative）**事件偵測與因果鏈（Causal Chain）推導。

---

## OVERVIEW

`internal/narrative` 透過監控全球總經指標（美債、匯率、VIX、原油、黃金）與特定產業情緒，將原始數據轉化為具備信心度與命中率的領域事件，並藉由因果範本推論對台股各板塊的潛在影響。

### 核心資料流
`MacroIngestor (MarketData) → NarrativeEvent → KnowledgeBase (Match Template) → CausalChain`

### Bundle API 實時數據流
前端【宏觀敘事】頁面透過 `/api/narrative/bundle` 取得事件、因果鏈、模型與季節性分析。該 endpoint 已接入 `marketdata.MacroDataProvider`：
```
MacroDataProvider.FetchSnapshot() → MarketNarrativeDataFromSnapshot()
  → DetectEvents() → MatchChains() / ActiveModels()
```
未在 snapshot 中的欄位（GeopoliticalGPR、RetailInstitutionalDivergence、MarginZScore）可透過 query param 手動覆蓋。

### News Sentiment 數據策略
**重要限制**：Finnhub News Sentiment API **僅支援美股公司**，台股無法直接使用。

**實作策略**：
1. **美股大盤作為代理指標**：使用 Finnhub 取得美股（NASDAQ、S&P 500）的新聞情緒與大盤漲跌
2. **外资流向推斷**：當美股下跌且 VIX 上升時，推測外资可能從台股撤離
3. **台股本地數據**：使用 TWSE 開放資料作為台股本地情緒的代理指標（公告數量、討論度）

**因果關係**：
```
美股大跌 + VIX飆升  →  外資撤離台股  →  台股壓力上升  →  RegimeChange
     ↑                                          ↑
  Finnhub API                              TWSE 開放資料
  (新聞情緒)                               (本地代理指標)
```

---

## EVENT TYPES

### NarrativeEvent 結構（擴展版）
- `Theme`: 事件主題標籤（如 `US_rates_up`, `AI_capex_surge`）。
- `Confidence`: 偵測演算法對事件成立的信心度 `[0.0, 1.0]`。
- `ConfidenceSource`: 信心度來源（預設 `heuristic_fixed_v1`）。
- `HitRate`: 該主題在歷史中的回測命中率。
- `SourceData`: 觸發事件的原始數值快照。
- `Duration`: 事件影響持續時間（time.Duration）。
- `ExpiresAt`: 事件過期時間點（*time.Time）。
- `Severity`: 事件嚴重程度（`low`/`medium`/`high`/`critical`）。
- `Status`: 事件狀態（`active`/`confirmed`/`faded`/`expired`）。

### Event 狀態機
```
active → confirmed → faded → expired
  ↓
  （可直接跳轉）
```

| 狀態 | 說明 | 轉換條件 |
|------|------|----------|
| `active` | 事件初始偵測 | 信心度 > 閾值 |
| `confirmed` | 事件已確認（多個數據源驗證） | 2+ 獨立數據源確認 |
| `faded` | 事件影響減弱 | 時間経過 > Duration × 0.8 |
| `expired` | 事件完全過期 | 時間経過 > ExpiresAt |

### Severity 等級
| 等級 | 說明 | 因子權重調整 |
|------|------|--------------|
| `low` | 輕微影響 | ±5% |
| `medium` | 中度影響 | ±10% |
| `high` | 高度影響 | ±20% |
| `critical` | 極度影響（緊急） | ±30%，立即觸發 RegimeChange |

### 內建主題與命中率 (Built-in Hit Rates)
所有主題的命中率統一由 `DefaultTemplates()` 提供，`hitRateForTheme()` 自動查表。回傳 0 表示該主題無模板定義。

| Theme | Hit Rate | 偵測指標 |
|-------|----------|---------|
| `AI_capex_surge` | **0.81** | AI 資本支出展望、CoWoS 需求 |
| `US_rates_up` | **0.72** | 美債 10Y 殖利率、美元指數 (DXY) |
| `JPY_carry_unwind` | **0.68** | 日圓匯率、VIX 波動率 |
| `geopolitical_risk_spike` | **0.65** | 黃金、VIX、地緣政治指數 (GPR) |
| `oil_price_shock` | **0.58** | 原油價格劇烈波動 |
| `taiwan_political_risk` | **0.65** | 兩岸緊張、軍事演習 |
| `semiconductor_downturn` | **0.60** | VIX+DXY+AI 情緒多因子 |
| `USD_TWD_volatility` | **0.62** | 台幣日波動 > 1% |
| `retail_institutional_divergence` | **0.60** | 融資 Z-Score + 散戶機構分歧 |
| `gold_rally` | **0.65** | 黃金漲幅突破閾值 |
| `dollar_surge` | **0.70** | DXY 急升突破閾值 |
| `inflation_spike` | **0.62** | VIX+DXY 通膨重新定價 |
| `earnings_surprise` | **0.60** | TSMC 財報驚喜 / AI 情緒代理 |
| `spring_festival_season` | **0.70** | 農曆年前後季節性 |
| `election_cycle` | **0.65** | 選舉週期不確定性 |
| `earnings_blackout` | **0.65** | 財報空窗期 |
| `tech_peak_season` | **0.70** | Q3-Q4 科技出貨旺季 |
| `year_end_window_dressing` | **0.68** | 年底作帳行情 |

### 內建事件持續時間 (Event Durations)
所有持續時間定義於 `DefaultThemeDurations()`，為 `EventLifecycleManager` 與 KB detector 的唯一權威來源。

| Theme | 價格發現期 | 影響持續期 | Severity 預設 |
|-------|-----------|------------|---------------|
| `AI_capex_surge` | 1-30 日 | 90 日 | `high` |
| `US_rates_up` | 1-4 小時 | 7 日 | `medium` |
| `JPY_carry_unwind` | 即時 | 14 日 | `medium` |
| `geopolitical_risk_spike` | 即時 | 30 日 | `high` |
| `oil_price_shock` | 即時 | 15 日 | `medium` |
| `Fed_emergency_cut` | 即時 | 3 日 | `critical` |
| `earnings_surprise` | 1-3 日 | 10 日 | `high` |
| `taiwan_political_risk` | 即時 | 30 日 | `high` |
| `semiconductor_downturn` | 1-7 日 | 90 日 | `high` |
| `USD_TWD_volatility` | 即時 | 7 日 | `medium` |
| `retail_institutional_divergence` | 即時 | 7 日 | `medium` |
| `gold_rally` | 即時 | 7 日 | `medium` |
| `dollar_surge` | 即時 | 7 日 | `medium` |
| `inflation_spike` | 即時 | 15 日 | `medium` |
| `spring_festival_season` | 日曆 | 30 日 | `low` |
| `election_cycle` | 日曆 | 30 日 | `low` |
| `earnings_blackout` | 日曆 | 30 日 | `low` |
| `tech_peak_season` | 日曆 | 60 日 | `low` |
| `year_end_window_dressing` | 日曆 | 60 日 | `low` |

---

## EVENT LIFECYCLE MANAGEMENT

### 事件過期機制
1. 每個 `NarrativeEvent` 在建立時根據 Theme 自動設定 `Duration` 與 `ExpiresAt`。
2. `EventLifecycleManager` 負責追蹤所有活躍事件，定期更新狀態。
3. 當事件從 `active` 轉為 `faded` 時，FactorWeightEngine 收到通知，開始漸進式回調權重。
4. 當事件過期時，從 FactorWeightEngine 的活躍事件列表移除。

### RegimeChange 觸發條件
以下情況觸發 RegimeChange：
- VIX 突破 30（進入 HighVol Regime）
- VIX 突破 25 且趨勢向下（進入 Bear Regime）
- VIX 跌破 15 且趨勢向上（進入 Bull Regime）
- `critical` 等級事件觸發
- StressIndex 突破 80

---

## ANTI-PATTERNS

- **手動計算 HitRate**: `NarrativeEvent` 的 `HitRate` 必須透過 `hitRateForTheme()` 從 `DefaultTemplates` 取得，不可在 detector 中硬編碼。
- **遺漏 SourceData**: 每個 `NarrativeEvent` 必須包含觸發時的 `SourceData`（如 bps 變化或百分比），以利後續決策鏈透明化追蹤。
- **無視 Region 限制**: 因果鏈匹配時會檢查 `RequiredRegion`（如 `US_rates_up` 需為 `US`），擴充偵測邏輯時須確保地域屬性正確。
- **直接修改模型權重**: `InvestmentModel` 的權重更新應由 `UpdateModelWeights` (Inverse-error + 40% 單一模型上限) 統一處理，避免手動干預造成權重失衡。
- **忽略 Duration/ExpiresAt/Severity/Status**: 所有 detector（含 shared builder）必須在建立事件時設定這四個欄位。`EventLifecycleManager.AddEvent` 雖會補上預設值，但 detector 顯式設定可確保一致性。
- **手動設定 Status**: Status 的狀態轉換應由 `EventLifecycleManager` 統一管理，不应由 detector 直接設定（detector 僅負責初始化為 `active`）。
- **事件重複偵測**: 相同 Theme 的事件在 active 狀態時不應重複偵測，應更新現有事件的 Confidence 而非建立新事件。
- **不一致的 confidence 計算**: 所有 detector 應統一使用 `computeDeviationConfidence(observed, threshold, base, ceiling)`，避免手寫公式導致跨事件不可比。
- **遺漏 causal chain 方向性**: `CausalChain` 已新增 `FavoredSectors` 與 `AvoidedSectors`，由 `classifySectorsByImpact` 根據 `Steps[].Impact` 正負號自動分類。前端「敘事看多/看空板塊」應使用這兩個欄位，而非 `AffectedSectors`。

---

## KEY TYPES (public 結構體)

| 結構體 | 檔案 | 用途 |
|--------|------|------|
| `NarrativeEvent` | types.go | 領域事件結構 |
| `KnowledgeBase` | knowledge_base.go | 因果範本匹配；`MatchChains` 產出帶 `FavoredSectors`/`AvoidedSectors` 的 `CausalChain` |
| `CausalChain` | types.go | 因果鏈推導（含方向性板塊分類） |
| `MacroIngestor` | ingestor.go | 巨集觀數據攝入 |
| `EventLifecycleManager` | lifecycle.go | 事件生命週期管理；`defaultDurations` 改為引用 `DefaultThemeDurations()` |
| `TaiwanStressIndex` | taiwan_stress_index.go | 台灣壓力指數計算 |
| `SeasonalBridge` | seasonal_bridge.go | 橋接敘事主題至產業供應鏈連動，提供 `ActiveThemes()` 與 `CorrelationMultiplier()` 供 `ShockPropagation` 與 `SeasonalEngine` 使用。 |
| `MarketNarrativeDataFromSnapshot` | snapshot_converter.go | 將 `MacroDataSnapshot` 轉換為 `MarketNarrativeData`，供 Bundle API 使用實時數據。 |

## 敘事感知相關矩陣調變 (Narrative-aware Correlation)

`SeasonalBridge` 實作 `NarrativeLinkageProvider` 介面（定義於 `internal/industry/linkage.go`），使巨集觀敘事主題能動態調整產業間相關矩陣：
- **5 個內建主題乘數**：`oil_price_shock`（油↔運 1.25）、`AI_capex_surge`（AI↔電 1.20）、`US_rates_up`（金↔消 0.80）、`JPY_carry_unwind`（全產業 0.85）、`geopolitical_risk_spike`（油↔金 1.30、科技 0.85）
- **無配置時降級**：`ActiveThemes()` 在 `narrativeEngine` 為 nil 時自動回退至空列表，不影響計算。
- **測試涵蓋**：`seasonal_bridge_test.go` 包含 20 個 `TestCorrelationMultiplier` 測試案例，涵蓋所有主題的成對匹配與對稱性驗證。

---

## ROLLING CALIBRATION FRAMEWORK

台灣壓力指數的自動校準框架。詳見 `docs/MACRO_CALIBRATION.md`。

### 五層架構
1. **Baseline**（`calibration_baseline.go`）：60d rolling Mean/Count 統計
2. **Scale**（`calibration_scales.go`）：自動調整使各因子貢獻相當
3. **Regime**（`calibration_regime.go`）：VIX-based 切換 bull/normal/bear/crisis
4. **Validation**（`calibration_validation.go`）：80/20 split，hit-rate 退化則跳過 export
5. **Scheduler**（`internal/scheduler/auto_calibration.go`）：Maturity-gated 每日觸發

### 核心規則

- **Hybrid Signal 是預設**：`TaiwanStressIndex.Calculate` 使用 `max(|level_z|, |change_pct|)`，不可降級為單一 change-pct。
- **`FactorBaseline` 不存 Baseline 欄位**：只存 `Mean` 與 `Count`，z-score 使用時即時計算（避免 Darwinian 權重靜默正規化）。
- **`BaselineConfig` 是 map**：新增 factor 不需修改 struct 定義（OCP 開放封閉原則）。
- **`ValidateCalibration` 是獨立函式**：非 `WeightCalibrationEngine` method，鬆耦合、可獨立測試。
- **`CalibrationTask` 沒有 goroutine**：背景排程一律交給 `BackgroundCalibrationScheduler.RunDaily`（由 `BackgroundTaskManager` 註冊）。不可在 narrative 模組內啟動長期 goroutine。
- **校準參數一律取自 `ParametersConfig`**：透過 `config.GetParametersConfig()` 取得，禁止 hardcode。
- **`calibration_enabled` 預設 false**：啟用前需在 staging 環境驗證至少 30 日。

### Maturity-Gated 行為

| 系統成熟度 | 行為 |
|----------|------|
| `BURN_IN` | log `burn_in_skip`、完全跳過校準 |
| `CALIBRATING` | 執行校準 + validation gate |
| `FULL_AUTO` | 執行校準 + validation gate |

### 驗證失敗（Degradation）處理

`ValidateCalibration` 回傳 `IsDegradation: true` 時：
- 不寫入新 config（保留舊 config）
- log warning 到 monitoring
- 當日 `Calculate` 仍用舊 config（不中斷服務）

### ANTI-PATTERNS（校準專屬）

- **手寫校準腳本**：不可在 `cmd/` 下新增 ad-hoc 校準工具，必須透過 `CalibrationTask` 框架。
- **跳過 validation gate**：不可直接寫入新 config 而不通過 `ValidateCalibration`。
- **在 narrative 模組啟動 goroutine**：校準相關的 background goroutine 一律在 `internal/scheduler` 內。
- **Hardcode 校準參數**：所有閾值（window、target_median、min_records 等）必須從 `ParametersConfig` 取得。

### 測試

```bash
# Baseline + Hybrid Signal
go test -v ./internal/narrative/calibration_baseline_test.go
# Scale Calibration
go test -v ./internal/narrative/calibration_scales_test.go
# Regime-Aware Weights
go test -v ./internal/narrative/calibration_regime_test.go
# Validation Gate
go test -v ./internal/narrative/calibration_validation_test.go
# 整合：TaiwanStressIndex 端到端
go test -v ./internal/narrative/taiwan_stress_index_test.go
# Scheduler 整合
go test -v ./internal/scheduler/auto_calibration_test.go
```

---

## 測試與驗證

- 偵測邏輯驗證：`go test -v ./internal/narrative/ingestor_test.go`
- 模板匹配驗證：`go test -v ./internal/narrative/narrative_test.go`
- 事件生命週期驗證：`go test -v ./internal/narrative/lifecycle_test.go`
- StressIndex 驗證：`go test -v ./internal/narrative/taiwan_stress_index_test.go`
