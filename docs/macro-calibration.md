# Rolling 巨集觀校準框架

> **適用範圍**：台灣壓力指數（Taiwan Stress Index）與其底層的 macro factor calibration。
> **適用模組**：`internal/narrative`（主）+ `internal/scheduler`（排程）+ `internal/config`（參數）+ `internal/monitoring`（dashboard）。
> **取代對象**：原 Rolling 巨集觀校準計畫書（PR #756 已清理，內容已萃取至本檔；歷史路徑已刪除）。

---

## 一、為什麼需要這個框架

### 問題陳述

台股壓力指數（Taiwan Stress Index）由八個 macro factor 組成：DXY、US10Y、VIX、JPY、Oil、Gold、ForeignFlow、Geopolitical。

在 2024–2025 的回測中發現一個**靜默錯誤**：
- 當日 DXY、JPY、Oil、Gold 報價為**平穩日（±0.0%）**時，原版的 `ChangePct`-based signal 會得到 **0.0**
- 八個因子全部回報 0.0 → 壓力指數回報接近 0 → 風險警訊漏報
- 漏報的後果：系統以為「無風險」→ 倉位滿載 → 真實衝擊到來時來不及反應

### 根因

`ChangePct` 是**事件導向**指標（事件日 = 變化日），但 macro factor 有**狀態導向**特性（level 偏離常態就代表壓力，不需「事件」）。

例如：
- DXY 從 100 升至 110（連續 5 日 +1%）：change-pct 訊號弱，但 level 偏離歷史均值已達 2σ → 應觸發警示
- 油價在 $80 盤整 30 日：change-pct 訊號全為 0，但若歷史均值為 $70，level 偏離 1σ 仍有意義

### 解法：Hybrid Signal

對每個 macro factor 並用兩種訊號：
- **ChangePct**（變化量）：捕捉**活躍日**的事件型衝擊
- **Level z-score**（位準偏離）：捕捉**平穩日**的狀態型壓力

最終 signal 取 `max(|level_z|, |change_pct|)`，保守地以兩種訊號中較強者為準。

---

## 二、五層架構

### 整體流程

```
1. Baseline   → 為每個 factor 計算 rolling mean & stddev
2. Hybrid     → 對每個 snapshot 計算 level z-score + change-pct
3. Scale      → 自動調整係數，使各因子對壓力指數的貢獻相當
4. Regime     → 根據 VIX 切換 bull/normal/bear/crisis 配置
5. Validate   → 80/20 train/validation，hit-rate 退化則跳過 export
6. Schedule   → Maturity-gated 每日觸發
```

### 1. Baseline（`calibration_baseline.go`）

**目的**：在 rolling window 上計算每個 factor 的統計基線。

**關鍵設計**：
- `FactorBaseline` 只存 `Mean` 與 `Count`（**不存 Baseline 欄位**），z-score 在使用時即時計算
  - 理由：避免 baseline 值被靜默正規化（Darwinian 權重 `[0.3, 2.5]` 範圍陷阱），Mean 是 raw 統計量
- `BaselineConfig` 用 `map[string]FactorBaseline`（**非 8 個固定欄位**）
  - 理由：新增 factor 不需修改 BaselineConfig 結構（OCP 開放封閉原則）
- rolling window 預設 60 個交易日（約 3 個日曆月）

### 2. Scale Calibration（`calibration_scales.go`）

**目的**：自動調整每個 factor 的 scale 係數，使各因子對壓力指數的貢獻相當。

**演算法**：
1. 在最近 N 個 snapshots 上，計算各 factor 的 `Mean(component_contribution)`
2. 計算全域 `target_median = 20.0`（points per factor under normal conditions）
3. 每個 factor 的 scale = `target_median / factor_median`
4. 套用至 `ApplyCalibratedScales`

**為何 median 而非 mean**：macro factor 分布有長尾（crisis 時的極端值），median 較不受離群值影響。

### 3. Regime-Aware Weights（`calibration_regime.go`）

**目的**：依 VIX 切換配置，避免用牛市權重解讀熊市訊號。

**Regime 分類**：

| Regime | VIX 範圍 | 權重傾向 |
|--------|----------|----------|
| `bull` | VIX < 15 | 降低 Geopolitical、提高 ForeignFlow |
| `normal` | 15 ≤ VIX < 25 | 中性 baseline |
| `bear` | 25 ≤ VIX < 35 | 提高 VIX、Geopolitical 權重 |
| `crisis` | VIX ≥ 35 | 全面提高波動率因子，限縮 ForeignFlow |

**關鍵設計**：`RegimeCalibrator.CalibrateByRegime` 在每個 regime 內**獨立**校準權重，避免牛市資料主導危機配置。

### 4. Validation Gate（`calibration_validation.go`）

**目的**：用 out-of-sample 測試確保新校準不會**退化**。

**演算法**（80/20 split）：
1. 將 records 依時間排序，前 80% 為 train set，後 20% 為 holdout
2. 在 train set 上計算 `NewConfig`（校準後）
3. 在 holdout 上比較 `OldConfig` 與 `NewConfig` 的 hit-rate
4. 若 `NewConfig` hit-rate 顯著低於 `OldConfig`（degradation），**跳過 export**

**API**：`ValidateCalibration(records, oldCfg, newCfg) CalibrationValidation`
- 是**獨立函式**（非 WeightCalibrationEngine 的 method）
- 理由：validation 邏輯與 engine 鬆耦合，可獨立測試、可在其他場合複用

### 5. Maturity-Gated Scheduler（`internal/scheduler/auto_calibration.go`）

**目的**：依系統成熟度決定是否執行校準。

**規則**：

| 系統成熟度 | 行為 | 理由 |
|----------|------|------|
| `BURN_IN` | log status、**完全跳過** | 系統剛啟動，資料量不足，校準結果不可信（避免假信號） |
| `CALIBRATING` | 執行校準 + validation gate | 累積足夠資料，可開始自動校準 |
| `FULL_AUTO` | 執行校準 + validation gate | 成熟系統，校準常駐運行 |

**關鍵設計**：
- `BackgroundCalibrationScheduler.RunDaily` 是 `BackgroundTaskFunc`（`func(ctx) error`），由 `BackgroundTaskManager` 排程
- `CalibrationTask` 本身**沒有** goroutine、`Start/Stop/Status` 方法
  - 理由：避免重複的 scheduler 狀態管理；AGENTS.md 架構禁令要求背景任務一律經 `BackgroundTaskManager` 註冊啟動
- Maturity tracker 為 `nil` 時 scheduler 安全降級（不 panic，記 log skip）

---

## 三、配置參數

所有參數透過 `ParametersConfig` 管理（禁止 hardcode magic number）。

| 參數 | 預設值 | 用途 | 來源 |
|------|--------|------|------|
| `calibration_baseline_window` | `60` | Rolling window（交易日），約 3 個月 | Heuristic |
| `calibration_target_median` | `20.0` | Scale calibration 目標中位數 | Heuristic |
| `calibration_validation_pct` | `0.2` | 80/20 train/validation split | Heuristic |
| `calibration_min_records` | `10` | 最低資料量門檻 | Heuristic |
| `calibration_enabled` | **`false`** | 是否啟用自動校準 | Heuristic（預設關閉） |

**重要**：`calibration_enabled` 預設為 `false`。啟用前需：
1. 累積至少 6 個月 live data
2. 驗證校準後的 hit-rate 優於 baseline
3. 在 staging 環境至少測試 1 個月

---

## 四、檔案地圖

| 檔案 | 角色 |
|------|------|
| `internal/narrative/calibration/calibration_baseline.go` | FactorBaseline + ComputeBaselines + Hybrid signal |
| `internal/narrative/calibration/calibration_baseline_test.go` | 8 個單元測試 |
| `internal/narrative/calibration/calibration_scales.go` | ScaleCalibrator + AutoCalibrateScales |
| `internal/narrative/calibration/calibration_scales_test.go` | 6 個單元測試 |
| `internal/narrative/calibration/calibration_regime.go` | MarketRegime + ClassifyRegime + RegimeCalibrator |
| `internal/narrative/calibration/calibration_regime_test.go` | 8 個單元測試 |
| `internal/narrative/calibration/calibration_validation.go` | ValidateCalibration 獨立函式 |
| `internal/narrative/calibration/calibration_validation_test.go` | 5 個單元測試 |
| `internal/narrative/calibration/weight_calibration.go` | CalibrationTask + 既有 engine |
| `internal/narrative/calibration/stress_index_config.go` | StressIndexWeights/Scaling/Thresholds 型別與常數（新集中定義） |
| `internal/narrative/calibration/load_weights_config.go` | 從參數系統載入 stress-index 權重 |
| `internal/narrative/calibration/helpers.go` | calibration 子套件內部 helper（e.g. `mean`） |
| `internal/narrative/calibration_facade.go` | 重新匯出 calibration 公開 API，維持舊 import 路徑相容 |
| `internal/narrative/taiwan_stress_index.go` | 整合所有 layers 的 orchestrator |
| `internal/scheduler/auto_calibration.go` | BackgroundCalibrationScheduler（maturity-gated） |
| `internal/config/parameters.go` | 5 個 `ParameterMetadata[T]` 欄位 |
| `internal/config/parameters_defaults.go` | 5 個預設值（含 rationale 與 source） |
| `internal/monitoring/dashboard_api.go` | SetCalibrationTask + RunCalibration API |
| `cmd/atlas/main.go` | 註冊 `calibration_cycle` 背景任務（24h interval） |

---

## 五、執行流程

### 啟動時
1. `cmd/atlas/main.go` 啟動 `BackgroundTaskManager`
2. 若 `maturityTracker != nil`：
   - 建立 `CalibrationTask`（持有 `workDir` 與 `CalibrationValidation` 輸出）
   - 建立 `BackgroundCalibrationScheduler(tracker)` 並註冊 task
   - 呼叫 `dashboard.SetCalibrationTask(calTask)`
   - 註冊排程：24h interval、30min jitter、依 `calibration_enabled` 啟用

### 每日觸發時
1. `BackgroundCalibrationScheduler.RunDaily` 被呼叫
2. 檢查 maturity：
   - BURN_IN → log `burn_in_skip` → return nil（不執行）
   - CALIBRATING/FULL_AUTO → 繼續
3. 呼叫 `calTask.RunCalibrationCycle()`：
   - 載入最近 60d snapshots
   - 計算 `NewConfig`（含 baseline、scale、regime-aware）
   - 載入既有 `OldConfig`
   - 呼叫 `ValidateCalibration`：若退化，跳過 export
   - 持久化 `NewConfig` 到 `data/state/calibration/`
4. 後續 `TaiwanStressIndex.Calculate` 自動使用新 config

### 退化情境處理
- 若 validation 標記 `IsDegradation: true`：
  - 不寫入新 config
  - 保留舊 config
  - log warning 到 monitoring
  - 不影響當日 `Calculate` 結果（用舊 config）

---

## 六、與既有架構的整合

### AGENTS.md 架構禁令遵守
- ✅ **業務邏輯參數一律取自 `ParametersConfig`**：5 個 calibration 參數都透過 `config.GetParametersConfig()` 取得
- ✅ **背景任務一律經 `BackgroundTaskManager` 註冊**：`calibration_cycle` 透過 `taskMgr.Register(&ScheduledTask{...})` 註冊
- ✅ **禁止全域可變狀態進行執行期協調**：`BackgroundCalibrationScheduler` 是 struct 實例，狀態封裝
- ✅ **domain 型別留在 `internal/domain`**：maturity 判定用 `domain.SystemMaturity`，未污染 narrative 模組

### 與 MacroIngestor 的關係
- `MacroIngestor` 負責**事件偵測**（偵測 US_rates_up、JPY_carry_unwind 等）
- Calibration framework 負責**壓力指數的數值校準**（不涉及事件偵測）
- 兩者獨立運行，但 `TaiwanStressCalculator` 同時消費兩者輸出

### 與 Seasonal Calibration 的關係
- **完全獨立**。`internal/scheduler/seasonal_task.go`（季節性校準）是不同框架
- 兩者透過各自的 `BackgroundTaskManager.Register` 排程，互不干擾

---

## 七、測試策略

### 單元測試
- 每個新檔案都有對應 `*_test.go`
- 涵蓋 happy path、edge cases（空 records、單一 record、退化情境）
- 全部 76 個 packages 的既有測試在 framework 整合後仍全數通過

### 整合測試
- `taiwan_stress_index_test.go`：驗證 `Calculate` 在不同 regime 下的輸出
- `weight_calibration_test.go`：驗證 `CalibrationTask.RunCalibrationCycle` 端到端流程

### 驗證閘門（人工）
- 啟用 `calibration_enabled = true` 前，必須在 staging 環境執行至少 30 日
- 比較啟用前後的 stress index 與實際波動率的相關性

---

## 八、後續 TODO

- [ ] 累積 6 個月 live data 後，重新評估 baseline window（目前 60d）
- [ ] 增加 cross-factor correlation matrix 校準（避免雙重計算）
- [ ] ML-based 校準（將 5 個參數改為動態學習）
- [ ] Dashboard UI 顯示 calibration status 與最近一次 validation 結果

---

*文件版本: 1.0*
*最後更新: 2026-06-05*
*對應計畫書：原 Rolling 校準計畫書已在 PR #756 清理並萃取內容後刪除。
