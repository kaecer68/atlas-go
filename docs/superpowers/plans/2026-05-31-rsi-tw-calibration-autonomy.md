# RSI-tw 子指標校準與自主進化計劃

> **Branch**: `feat/rsi-tw-calibration-autonomy`  
> **Created**: 2026-05-31  
> **Status**: Phase 0 — Investigation Complete, Phase 1 — In Progress  
> **Module**: `internal/retail/` (Maturity: experimental, X-tier)  
> **Related**: PR #299 (hardcoded params partial fix)

---

## 0. 現狀診斷 (Root Cause Analysis)

### 0.1 用戶觀察到的問題

從前端【子指標明細】面板擷取的數值：

| 子指標 | 顯示值 | 問題 |
|--------|--------|------|
| 散戶期貨 OI (C1) | 0.500 | 硬編碼 fallback |
| 券商分點流向 (C2) | 0.500 | 硬編碼 fallback |
| ETF 申購分數 (C3) | 0.500 | 硬編碼 fallback |
| VIX 風險分數 (A4) | 0.300 | 硬編碼映射結果 |
| 週選擇權 PCR (A5) | 0.900 | 硬編碼 fallback |
| 零股交易失衡 (A6) | 0.500 | 硬編碼 fallback |
| 融資餘額 Z-score (A1) | 0.000 | 歷史不足 fallback |

### 0.2 根本原因

#### 原因 1：資料饋送不完整
- `GeopoliticalRisk` 在 `handlers.go:204` **硬編碼為 0**
- Gateway channels (`taifex_daily`, `twse_oddlot`, `twse_etf`) 若回傳錯誤，fetcher 回傳 nil，子指標全部 fallback 到 0.5
- **無監控機制**顯示哪些 fetcher 成功/失敗

#### 原因 2：參數全部為啟發式預設值
所有 `RSITwParameters` 都是 `SourceHeuristic`，附 TODO 校準標籤：
- `C1VeryBullishThreshold: 20` — TODO: "Calibrate from 2Y historical futures OI distribution"
- `C2NeutralMidpoint: 0.5` — TODO: "Calibrate from 2Y foreign/domestic fund net flow distribution"
- `C3VeryBullishThreshold: 1_000_000_000` — 無校準計劃
- 所有 Part D 參數也是 heuristic

#### 原因 3：Part A 權重/閾值寫死在程式碼中
以下值在 `rsi_tw_calculator.go` 中為 literal constants，無法透過 `parameters.json` 調整：

| 位置 | 硬編碼值 |
|------|----------|
| `computePartA()` line 126 | `0.40`, `0.25` (Part A/C 權重) |
| `subA1` | weight 0.25 |
| `subA2` | weight 0.20 |
| `subA3` | weight 0.20, Z-score formula `(p-0.5)*2` |
| `subA4` | weight 0.15 |
| `subA5` | weight 0.10, PCR thresholds `[1.5, 1.0, 0.8]`, scores `[0.9, 0.7, 0.5, 0.1]` |
| `subA6` | weight 0.10, odd-lot thresholds `[0.2, 0.1, -0.1, -0.2]`, scores `[0.85, 0.65, 0.5, 0.35, 0.15]` |
| `vixMap()` | thresholds `[15, 20, 25, 30, 35]`, scores `[0.1, 0.3, 0.5, 0.7, 0.85, 1.0]` |
| Part C 子權重 | `subC1` 0.40, `subC2` 0.35, `subC3` 0.25 |
| Factor 權重 | `factorD1` 0.85, `factorD2` 0.90, `factorD3` 0.80 |

#### 原因 4：無自主進化機制
- 無 feedback loop（計算出的 score vs. 實際市場行為）
- 無 Bayesian optimization / gradient-free optimization
- 無定期校準排程（`BackgroundTaskManager` 未註冊 RSI-tw 校準任務）
- 無 `CalibrationReport` 類似 `risk_gate_calibrate` 的機制

#### 原因 5：無下游消費
- RSI-tw 值僅在前端展示，未被 orchestrator、risk manager、或任何 executor 使用
- 無 `conviction_builder` 整合
- 無 `factor_engine` 整合
- 無 `RiskGate` 整合

#### 原因 6：模組合規性缺失
- `internal/retail/` 有 `doc.go`（Maturity: experimental）但**未列入 `internal/MATURITY.md`**
- 無 `internal/retail/AGENTS.md`（無模組陷阱記錄）
- 無測試檔案

---

## 1. 解決方案架構

### 1.1 設計原則

1. **遵循 atlas-go 憲法**：所有外部資料通過 Gateway channels 取得；所有參數透過 `ParametersConfig` 管理；所有定時任務透過 `BackgroundTaskManager` 排程
2. **逐步自主化**：Phase 1 先修復資料流（讓數值不再是 0.5），Phase 2 加入參數化（移除硬編碼），Phase 3 加入校準管道，Phase 4 加入自主進化閉環
3. **證據驅動**：每階段有明確的 before/after 對比，確保數值從 0.5 變成真實校準值
4. **下游整合**：Phase 5 將 RSI-tw 接入 orchestrator / risk manager 決策鏈

### 1.2 架構總覽

```
┌─────────────────────────────────────────────────────────┐
│                   Phase 5: 下游消費                       │
│  orchestrator → conviction_builder → RiskGate             │
│  factor_engine → portfolio allocation                     │
├─────────────────────────────────────────────────────────┤
│                   Phase 4: 自主進化閉環                    │
│  BackgroundTaskManager → CalibrateRSITw()                │
│  ┌──────────┐    ┌───────────┐    ┌──────────────────┐  │
│  │ 載入歷史  │ → │ Bayesian   │ → │ 產出              │  │
│  │ outcomes  │    │ Optimizer  │    │ CalibrationReport │  │
│  └──────────┘    └───────────┘    └──────────────────┘  │
├─────────────────────────────────────────────────────────┤
│                   Phase 3: 校準管道                       │
│  cmd/calibrate-rsi-tw: 從歷史回測數據計算最佳參數          │
│  data/state/rsi_tw_calibration.json: 校準結果持久化        │
├─────────────────────────────────────────────────────────┤
│                   Phase 2: 參數化                         │
│  parameters.json ← 所有權重、閾值、映射表                  │
│  移除 Part A 硬編碼，提升為 ParameterMetadata               │
├─────────────────────────────────────────────────────────┤
│                   Phase 1: 修復資料流                      │
│  GeopoliticalRisk ← NarrativeProvider                    │
│  Gateway channels 錯誤監控                                │
│  fetcher 成功率 metrics                                   │
├─────────────────────────────────────────────────────────┤
│                   Phase 0: 合規性                         │
│  MATURITY.md 更新、AGENTS.md 建立、測試覆蓋               │
└─────────────────────────────────────────────────────────┘
```

---

## 2. Phase 0 — 模組合規性 (Compliance)

### 2.1 更新 `internal/MATURITY.md`
- [ ] 新增 `internal/retail/` 條目，標記為 X-tier (experimental)
- [ ] 執行 `bash scripts/ci/check_maturity.sh` 確認通過

### 2.2 建立 `internal/retail/AGENTS.md`
- [ ] 記錄模組陷阱：資料饋送依賴、fallback 行為、權重硬編碼位置
- [ ] 記錄 `RSITwInput` 欄位的資料來源對照表
- [ ] 記錄 sub-indicator key name convention

### 2.3 建立測試基礎設施
- [ ] `internal/retail/rsi_tw_calculator_test.go`：單元測試覆蓋 `ComputeFinal()`
- [ ] 測試涵蓋：正常輸入、fallback 路徑、edge cases (零值、負值、極端值)

### 2.4 移除 `narrative.js` 前端暫存修復
- [x] `window.toggleSubIndicators` 已修復（當前分支已含此變更）

---

## 3. Phase 1 — 修復資料流 (Data Pipe)

### 3.1 GeopoliticalRisk 資料饋送
- [ ] `handlers.go:204`：`GeopoliticalRisk: 0` → 改為從 Narrative provider 讀取
- [ ] 實作 `GeopoliticalRiskFetcher` type（類似 DayTradingFetcher）
- [ ] 在 `dashboard_api.go` 中接線
- [ ] Gateway adapter 實作：從 `narrative.GeopoliticalRiskProvider` 讀取

### 3.2 Fetcher 監控
- [ ] 每次 API 請求記錄每個 fetcher 的成功/失敗狀態
- [ ] 新增 `fetcher_status` 欄位到 `RetailSentimentResponse`
- [ ] 前端顯示資料來源健康狀態（🟢/🟡/🔴）

### 3.3 Gateway Channel 驗證
- [ ] 確認 `taifex_daily` channel 已註冊且提供真實數據
- [ ] 確認 `twse_oddlot` channel 已註冊且提供真實數據
- [ ] 確認 `twse_etf` channel 已註冊且提供真實數據
- [ ] 若 Gateway channel 返回錯誤，記錄詳細錯誤原因（而非靜默 fallback）

### 3.4 `CreditTightening` 信號
- [ ] `RSITwInput.CreditTightening` 目前可能未正確填充
- [ ] 從 macro snapshot 或 narrative events 讀取信貸緊縮信號

---

## 4. Phase 2 — 參數化 (Parameterization)

### 4.1 Part A 權重參數化
- [ ] 新增以下 `ParameterMetadata[float64]` 到 `RSITwParameters`：
  - `A1Weight` (default 0.25), `A2Weight` (0.20), `A3Weight` (0.20)
  - `A4Weight` (0.15), `A5Weight` (0.10), `A6Weight` (0.10)
  - `PartAWeight` (0.40), `PartCWeight` (0.25)
- [ ] 重構 `computePartA()` 從 params 讀取權重
- [ ] 重構 `ComputeFinal()` 使用可配置的 Part A/C 權重

### 4.2 Part A 閾值參數化
- [ ] PCR 映射表參數化（thresholds + scores）
- [ ] Odd-lot 映射表參數化
- [ ] VIX 映射表參數化（thresholds + scores）
- [ ] A3 Z-score formula `(p-0.5)*2` 參數化（midpoint, scaling factor）

### 4.3 Part C 子權重參數化  
- [ ] 新增 `C1Weight` (default 0.40), `C2Weight` (0.35), `C3Weight` (0.25)

### 4.4 參數驗證
- [ ] 所有權重加總檢查（Part A sub-weights = 1.0, Part C sub-weights = 1.0）
- [ ] 閾值單調性檢查（bullish > neutral > bearish）
- [ ] `parameters_defaults.go` 更新 → `parameters.json` 自動同步

---

## 5. Phase 3 — 校準管道 (Calibration Pipeline)

### 5.1 CLI 校準工具
- [ ] 建立 `cmd/calibrate-rsi-tw/main.go`
- [ ] 輸入：歷史 macro snapshots (`data/state/macro/*.json`)
- [ ] 輸出：校準後的 `parameters.json` patch
- [ ] 支援 `--replay` 旗標（使用 replay data）

### 5.2 校準方法論

#### Part A 權重校準
- [ ] 目標：最小化 Part A score 與 TWSE 指數未來 N 日報酬的 RMSE
- [ ] 方法：Grid search 或 Bayesian optimization (scikit-optimize 風格)
- [ ] 約束：權重總和 = 1.0，每個權重 ∈ [0.05, 0.40]

#### Part C 閾值校準
- [ ] 目標：最大化 futures OI 信號對未來報酬的方向性預測準確率
- [ ] 方法：ROC 曲線最佳化（找 optimal threshold）
- [ ] 每個閾值獨立校準，使用 historical distribution

#### Part D 乘數校準
- [ ] 目標：事件發生後 N 日的實際報酬 vs. 預期報酬差異
- [ ] 方法：Event study methodology
- [ ] 輸出：每個事件類型的 empirical multiplier

### 5.3 校準證據追蹤
- [ ] 每個參數更新後記錄：
  - `last_calibrated` timestamp
  - `calibration_method`（grid_search / bayesian / event_study）
  - `calibration_sample_size`（使用的歷史數據點數）
  - `calibration_metric`（RMSE / accuracy / AUC）
  - `evidence_quality`（high / medium / low）

### 5.4 校準驗證
- [ ] Walk-forward validation（避免 look-ahead bias）
- [ ] Out-of-sample testing（最後 20% 數據）
- [ ] 與 baseline（全 0.5 fallback）的相對改善度報告

---

## 6. Phase 4 — 自主進化閉環 (Self-Evolution)

### 6.1 BackgroundTaskManager 註冊
- [ ] 在 `cmd/atlas/main.go` 註冊 `rsi_tw_calibrate` 任務
- [ ] 排程：每 24 小時（或每 N 個 session）
- [ ] 觸發條件：累積足夠新數據（≥ 30 個新 session）

### 6.2 自主校準邏輯

```
CalibrateRSITw():
  1. 載入最近 30-90 個 session 的 RSI-tw 計算結果
  2. 對比 forward return（session 日期 + N 日的實際報酬）
  3. 計算每個 sub-indicator 的預測能力（IC, rank IC）
  4. 若某 sub-indicator 的 IC 顯著下降（低於歷史平均 - 2σ），標記為退化
  5. Bayesian optimizer 搜尋新的最佳權重/閾值
  6. 套用新參數（若改善 > 最小閾值）
  7. 產出 CalibrationReport → 寫入參數檔案
```

### 6.3 CalibrationReport
- [ ] 結構定義：`internal/retail/calibration_report.go`
- [ ] 欄位：timestamp, parameters_before, parameters_after, improvement_metric, sample_size, confidence
- [ ] API 端點：`GET /api/dashboard/rsi-tw-calibration`
- [ ] 前端：校準歷史圖表

### 6.4 安全機制
- [ ] 參數變更幅度限制（單次調整 ≤ 20%）
- [ ] 回滚機制：保留最近 3 次校準快照
- [ ] 人工審查閘門：若 improvement < 最小閾值或 confidence < 0.6，僅記錄不套用
- [ ] 前台顯示參數來源（`heuristic` → `backtest_N_samples` → `auto_calibrated`）

---

## 7. Phase 5 — 下游消費整合 (Downstream Integration)

### 7.1 Orchestrator 整合
- [ ] 在 `SystemCore` 決策鏈中參考 RSI-tw score
- [ ] 極端讀數（frenzy ≥ 0.5, fear ≤ -0.5）作為 RiskGate 的附加過濾條件
- [ ] `conviction_builder` 根據散戶情緒調整 conviction 分數

### 7.2 Factor Engine 整合
- [ ] 將 RSI-tw score 作為 `SentimentFactor` 輸入 factor engine
- [ ] 定義 factor 權重和衰減曲線
- [ ] 在 `CalculateAllScoresWithBreakdown()` 中包含 sentiment factor

### 7.3 Portfolio 整合
- [ ] 散戶情緒過熱時（≥ 0.7）降低整體部位權重
- [ ] 散戶恐慌時（≤ -0.7）作為逆向指標增加部位

### 7.4 實驗整合
- [ ] RSI-tw score 可作為實驗 mutation brief 中的控制變數
- [ ] A/B test：有/無 RSI-tw 過濾的實驗組對比

---

## 8. 任務優先級與依賴

| Phase | 優先級 | 預估工作量 | 依賴 |
|-------|--------|-----------|------|
| Phase 0 | P0 | 2h | 無 |
| Phase 1 | P0 | 4h | Phase 0 |
| Phase 2 | P1 | 6h | Phase 0 |
| Phase 3 | P1 | 8h | Phase 2 |
| Phase 4 | P2 | 6h | Phase 3 |
| Phase 5 | P2 | 8h | Phase 1, 3 |

**Phase 0-1 可並行**；Phase 2-3 循序；Phase 4-5 可部分並行。

---

## 9. 驗證標準

每個 Phase 完成後必須滿足：

### Phase 0
- [ ] `check_maturity.sh` 通過
- [ ] `go test ./internal/retail/...` 覆蓋率 ≥ 40%
- [ ] `gofmt`, `go vet`, `staticcheck` 通過

### Phase 1
- [ ] 子指標明細中 C1/C2/C3 值不再是 0.500（有真實數據）
- [ ] GeopoliticalRisk 非 0（有真實敘事數據）
- [ ] Fetcher 狀態在前端可見
- [ ] `go build ./...` 通過

### Phase 2
- [ ] 所有 Part A 權重/閾值可透過 `parameters.json` 調整
- [ ] 權重總和驗證通過
- [ ] Before/after 計算結果一致（預設值 = 原硬編碼值）
- [ ] `parameters.json` 包含完整 `rsi_tw` 區段

### Phase 3
- [ ] `go run ./cmd/calibrate-rsi-tw` 成功產出校準報告
- [ ] 校準後的參數在 out-of-sample 測試中優於 baseline
- [ ] `evidence_quality` 欄位正確標記
- [ ] Walk-forward validation 無 look-ahead bias

### Phase 4
- [ ] `BackgroundTaskManager` 正確註冊並執行校準任務
- [ ] `CalibrationReport` API 端點正常回應
- [ ] 自主校準在 3+ 次迭代後參數趨於穩定
- [ ] 回滚機制可正常運作

### Phase 5
- [ ] Orchestrator 在極端散戶情緒時正確調整決策
- [ ] Factor engine 正確計算 sentiment factor
- [ ] A/B test 顯示有意義的績效差異

---

## 10. 風險與緩解

| 風險 | 影響 | 緩解 |
|------|------|------|
| Gateway channels 無真實數據 | Phase 1 無法取得真實值 | 先用 replay data 驗證；若 channels 不存在，優先建立 channels |
| 歷史數據不足以校準 | Phase 3-4 校準不穩定 | 設定最小樣本數門檻（≥ 30 sessions）；不足時維持 heuristic |
| 自主校準過度擬合 | Phase 4 參數退化解 | Walk-forward validation + out-of-sample testing + 變更幅度限制 |
| 下游整合破壞現有邏輯 | Phase 5 影響其他模組 | Feature flag 控制；漸進式 rollout；完整回歸測試 |
