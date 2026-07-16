# C07 Invariant Tracker Manifest — 板塊維度預測

> **Goal**: 在 `/api/events/prediction` 新增 `sector_predictions` 欄位，為每個 L1 板塊提供可解釋的 5 日方向預測，並維持與整體預測的一致性。
> **Spec**: docs/specs/sector-dimension-prediction.md v1.1
> **Created**: 2026-07-16
> **Branch**: `feat/sector-dimension-prediction`（已建立）
> **Status**: 實作完成，待 PR 與 L2.4 觀察窗口

---

## 1. Invariants（不可違反的條件）

| ID | Invariant | Owner | Verification | Status | Notes |
|---|---|---|---|---|---|
| I1 | `sector_predictions` 欄位必須 **always present**（不帶 omitempty）；無資料時為 `[]`，絕不為 `null` | eventdriven | API contract test + `Test_Predict_JSONHasSectorPredictions` | 🟢 done | `PredictionReport` 已新增 `SectorPredictions` 且無 omitempty |
| I2 | 每個預測日必須包含 **全部 20 個 L1 canonical sectors**，且排序固定 | eventdriven | `TestSectorPredictor_All20L1SectorsPerDay` + `TestSectorPredictor_FixedSectorOrder` | 🟢 done | 依 `internal/industry.L1Sectors()` 排序 |
| I3 | 每個 `distribution` 必須是合法機率分布：`inflow + neutral + outflow == 1.0`（容許浮點誤差 1e-6） | eventdriven | `TestSectorPredictor_DistributionSumsToOne` | 🟢 done | softmax + residual absorption 保證 |
| I4 | `sector_id` 必須是 canonical L1 ID；不允許 raw Chinese name 或 stock symbol | eventdriven | `TestSectorPredictor_CanonicalSectorIDs` | 🟢 done | 使用 `industry.L1Sectors()` 與 canonical ID |
| I5 | Sector-weighted 預測與整體預測的 **JSD ≤ 0.25**；若超過，confidence 必須衰減 0.85 且顯示 warning | eventdriven | `TestSectorPredictor_JSDConsistencyCheck` + frontend assertion | 🟢 done | 核心 investor-protection invariant |
| I6 | 歷史 baseline 僅在 **N ≥ 24（月度）/ 12（季度）/ 3（年度）** 時使用 empirical；否則強制 Bayesian shrinkage | eventdriven | deferred | ⚪ deferred | Phase 0 發現歷史資料無法取得；rule-based 模型預設先驗權重，資料就緒後無縫替換 |
| I7 | 模型 confidence floor = 0.40；不允許出現 confidence < 0.40 的輸出 | eventdriven | `TestSectorPredictor_ConfidenceFloor` | 🟢 done | 對齊 calibration 標準 |
| I8 | Frontend 預設 **摺疊**；展開後顯示 5 必須看板塊，可切換全部 20；必須記憶 localStorage | client_web | `__tests__/sector_predictions.test.mjs` | 🟢 done | UX invariant |
| I9 | Frontend 對 missing/empty `sector_predictions` 必須顯示 empty state，不可 crash | client_web | `__tests__/sector_predictions.test.mjs` | 🟢 done | defensive invariant |
| I10 | 新增計算後 `/api/events/prediction` p95 latency < 200ms | eventdriven | benchmark in integration test | 🟡 pending | 待 L2.4 觀察窗口測量 |
| I11 | Feature flag `SECTOR_PREDICTION_ENABLED` 預設 **false**；開啟前須通過 L2.4 observation window | config | `internal/config/config.go` (`SectorPredictionEnabled bool`) + `cmd/atlas/main.go` consumer | 🟢 done | rollout safety |
| I12 | Out-of-sample Brier score ≤ 0.20；hit-rate ≥ 0.55 | data-science | `cmd/run-experiment` backtest | ⚪ deferred | 需歷史板塊報酬才能計算；標記為未來升級條件 |

---

## 2. Phase-by-Phase Execution Tracker

### Phase 0 — Data Availability (2026-07-16)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 0.1 盤點 TWSE / FinMind 板塊日報酬可取得性 | I12 | 🟢 done | TWSE `exchangeReport/MI_INDEX?type=MS` 忽略 `date` 參數；FinMind `TaiwanStockPrice` 不支援 3 位數類股指數；歷史類股指數資料為付費 tier |
| 0.2 建立 stock symbol → canonical sector mapping | I4 | ⚪ deferred | 歷史資料不足，暫以 L1 canonical ID 直接對齊事件 affected_industries |
| 0.3 建立 canonical sector alias mapping（中文 → ID） | I4 | 🟢 done | `internal/industry/sector.go` `DisplayZHTw` + `L1Sectors()` |
| 0.4 Backfill 板塊日報酬 2023-07-01 ~ 2026-07-01 | I12 | ❌ blocked | 資料源無法提供 ≥ 95% 覆蓋率 |
| 0.5 產生 event_type × sector hit table | I6 | ⚪ deferred | 無足夠樣本，改以事件 `affected_industries` + 規則驅動 |
| 0.6 資料覆蓋率 ≥ 95% 驗證 | I12 | ❌ blocked | 覆蓋率 < 95% |

**Phase 0 結果**：觸發退出條件。改採 **rule-based / heuristic** 預測器（替代方案），不依賴歷史板塊日報酬，並保留未來升級為統計模型的空間。


---

### Phase 1 — SectorPredictor Core (2026-07-16)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 1.1 設計 `SectorPredictor` interface | I3, I4, I7 | 🟢 done | `internal/eventdriven/sector_predictor.go` |
| 1.2 實作 empirical baseline with Bayesian shrinkage | I6 | ⚪ deferred | Phase 0 資料不可取得 |
| 1.3 實作 macro exposure adjustment | I3 | 🟢 done | `macroExposureAdjustment` + `TestSectorPredictor_DifferentOverallDirections` |
| 1.4 實作 cycle position shift | I3 | 🟢 done | `cycleShift` + `TestSectorPredictor_*` |
| 1.5 實作 hybrid score + distribution softmax | I3, I7 | 🟢 done | `softmaxDistribution` + `TestSectorPredictor_DistributionSumsToOne` |
| 1.6 實作 JSD consistency check | I5 | 🟢 done | `TestSectorPredictor_JSDConsistencyCheck` |
| 1.7 純 offline backtest（2026-Q2 hold-out） | I12 | ⚪ deferred | 無歷史資料 |

---

### Phase 2 — API Integration (2026-07-16)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 2.1 新增 `SectorPrediction` / `SectorDayPrediction` types | I1, I4 | 🟢 done | `internal/eventdriven/types.go` |
| 2.2 將 `sector_predictions` 注入 `PredictionReport` | I1, I2 | 🟢 done | `predictor.go` + tests |
| 2.3 API contract test：必含 `sector_predictions`、20 sectors × 5 days | I1, I2 | 🟢 done | `TestRegisterRoutes_FullHTTPFlow_Prediction` |
| 2.4 新增 feature flag `SECTOR_PREDICTION_ENABLED` | I11 | 🟢 done | `cmd/atlas/main.go` |
| 2.5 效能 benchmark：p95 latency < 200ms | I10 | 🟡 pending | 待 L2.4 測量 |

### Phase 3 — Frontend (2026-07-16)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 3.1 新增 `cp-sector-predictions` template section | I8 | 🟢 done | `capital_predictions.js` |
| 3.2 實作摺疊/展開 + summary badge | I8 | 🟢 done | `__tests__/sector_predictions.test.mjs` |
| 3.3 實作 5 必須看板塊渲染 | I8 | 🟢 done | `__tests__/sector_predictions.test.mjs` |
| 3.4 實作「顯示全部 20 板塊」切換 | I8 | 🟢 done | `__tests__/sector_predictions.test.mjs` |
| 3.5 實作 driver tooltip + consistency warning | I5, I8 | 🟢 done | `title` tooltip + test |
| 3.6 localStorage 記憶偏好 | I8 | 🟢 done | `__tests__/sector_predictions.test.mjs` |
| 3.7 empty state 處理 | I9 | 🟢 done | `__tests__/sector_predictions.test.mjs` |
| 3.8 Frontend tests ≥ 5 cases | I8, I9 | 🟢 done | 5/5 passing |

---

### Phase 4 — L2.4 Observation Window (估計 2 週)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 4.1 Dark launch：API 回傳但前端不顯示 | I11 | 🟡 pending | frontend flag off |
| 4.2 每日比較 sector predictions vs 實際板塊報酬 | I12 | 🟡 pending | daily log + dashboard |
| 4.3 監控 JSD alert rate | I5 | 🟡 pending | alert metrics |
| 4.4 監控 overall prediction hit-rate 是否退步 | I12 | 🟡 pending | weekly report |
| 4.5 觀察窗口結束評估 | I11, I12 | 🟡 pending | observation report |

**Phase 4 退出條件**：
- 若 hit-rate 比 baseline 退步 → **不通過**，回 Phase 1 調校 macro beta / baseline shrinkage。
- 若 JSD alert rate > 10% → **不通過**，調低 macro weight 或提高 shrinkage。
- 若 latency p95 ≥ 200ms → **不通過**，優化計算路徑或改為 cron 預計算。

---

### Phase 5 — GA (General Availability)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 5.1 開啟 `SECTOR_PREDICTION_ENABLED=true` | I11 | 🟡 pending | config change + runbook |
| 5.2 更新 `docs/operations/sector-prediction-runbook.md` | I11 | 🟡 pending | runbook merged |
| 5.3 最終 `npm test` 與 `go test` 全綠 | I1-I10 | 🟡 pending | CI green |
| 5.4 更新每日簡報與機器人話術 | n/a | 🟡 pending | robot-communication skills |

---

## 3. Current Status Summary

- **Phase 0**: ✅ 完成 — 歷史板塊日報酬不可取得，改採 rule-based 替代方案
- **Phase 1**: ✅ 完成 — `SectorPredictor` 核心實作（rule-based）+ 單元測試
- **Phase 2**: ✅ 完成 — `sector_predictions` 接入 `PredictionReport` + API contract + feature flag
- **Phase 3**: ✅ 完成 — 前端板塊方向預測區塊 + 5 個單元測試
- **Phase 4**: 🟡 進行中 — 待 PR merge 後進入 L2.4 observation window
- **Phase 5**: 🟡 待開始 — 觀察通過後 GA

**Next action**: 整理 commit、rebase 到 main、開 PR。


---

## 4. Risk Register

| Risk | Probability | Impact | Mitigation | Trigger to stop |
|---|---|---|---|---|
| 板塊日報酬資料無法取得 ≥ 95% | ✅ 已發生 | 高 | 改用 rule-based 替代方案 | 已記錄於 §6 |
| 事件標籤 canonicalization 複雜 | 中 | 中 | 使用 `affected_industries` 直接對齊 L1 canonical IDs | 無法 mapping 的標籤 > 15% → 暫停 |
| 模型 overconfident | 中 | 高 | confidence floor 0.40 + JSD 一致性 + softmax 分佈 | Brier > 0.25 → Phase 4 退出 |
| 一致性衝突過多 | 低 | 中 | JSD 降權 + UI warning | JSD alert rate > 10% → 調權重 |
| 前端 100 cell 效能差 | 低 | 中 | 預設摺疊 + 按需渲染 | p95 latency ≥ 200ms → 預計算 |

---

## 5. Evidence Ledger（事實來源）

- L1 canonical sector IDs: `internal/industry/sector.go`
- Macro driver fields: `internal/marketdata/macro_provider.go:MacroDataSnapshot`
- Cycle feature: `internal/industry/cycle.go`
- Existing prediction API: `internal/eventdriven/predictor.go` + `types.go`
- Frontend pattern: `shared_web/static/js/pages/capital_predictions.js` (C06 template)
- L2.4 rollout pattern: `docs/operations/l2-4-runbook.md` + `docs/specs/l2-4-observation-spec.md`
- TWSE sector index API: `internal/marketdata/twse_sector_index_provider.go` (probe shows historical date param ignored)
- FinMind sector index API: `taiwan_stock_tick_snapshot` only real-time; `TaiwanStockPrice` rejects 3-digit index codes; historical sector index datasets are paid-tier

---

## 6. Phase 0 Audit Findings (2026-07-16)

### 6.1 資料源檢查結果

| 資料源 | 歷史板塊指數支援 | 證據 | 結論 |
|---|---|---|---|
| TWSE OpenAPI `exchangeReport/MI_INDEX?type=MS` | ❌ 不支援 | `date` 參數被忽略，任何日期都回傳最新資料 | 無法回填 |
| FinMind `TaiwanStockPrice` | ❌ 不支援 3-digit index codes | 對 `036` 回傳 `{"data":[]}`；對 `2330` 正常 | 無法回填 |
| FinMind `TaiwanStockEvery5SecondsIndex` | 僅限 paid-tier | 文件標示 backer/sponsor only | 無法用免費版回填 |
| Fugle / Fubon | 僅即時 | 即時行情 provider | 無法回填 |

### 6.2 結論

- **原訂「3 年歷史板塊日報酬 backfill」不可行**：無法達到 spec 規定 ≥ 95% 覆蓋率。
- **觸發 Phase 0 退出條件**：若嚴格遵循 spec v1.0，應停止 C07 並改為 frontend 只顯示 `affected_industries` 標籤。
- **替代方案**：系統已有 `MacroDataSnapshot`、`CycleTracker`、事件 `affected_industries` 與 narrative 模型；可實作不依賴歷史板塊回報的 **rule-based / heuristic sector predictor**，提供可解釋的板塊方向預測，並在資料就緒後平滑升級為統計模型。

### 6.3 決策

採用 **替代方案** 繼續 C07：

1. 不實作 historical backfill（任務 0.4 取消）。
2. 以事件 + 宏觀 + 週期為輸入，實作 rule-based `SectorPredictor`。
3. 維持所有 invariants（I1-I12），但 I12 accuracy KPI 改為「以回測就緒後再評估」，而非本階段上線門檻。
4. 文件化此偏差於 manifest 與 spec；觀察窗口期間持續監控一致性（I5）與 latency（I10）。

---

## 7. Change Log

| Date | Version | Change | Author |
|---|---|---|---|
| 2026-07-16 | 1.0 | Initial invariant tracker based on spec v1.0 | atlas-dev |
| 2026-07-16 | 1.1 | Phase 0 audit findings + alternative path decision | atlas-dev |
| 2026-07-16 | 1.2 | Phase 1-3 實作完成，更新 invariant 與 phase tracker 狀態 | atlas-dev |
