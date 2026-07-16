# C07 Invariant Tracker Manifest — 板塊維度預測

> **Goal**: 在 `/api/events/prediction` 新增 `sector_predictions` 欄位，為每個 L1 板塊提供可解釋的 5 日方向預測，並維持與整體預測的一致性。
> **Spec**: docs/specs/sector-dimension-prediction.md v1.0
> **Created**: 2026-07-16
> **Branch**: `feat/sector-dimension-prediction`（待建立）

---

## 1. Invariants（不可違反的條件）

| ID | Invariant | Owner | Verification | Status | Notes |
|---|---|---|---|---|---|
| I1 | `sector_predictions` 欄位必須 **always present**（不帶 omitempty）；無資料時為 `[]`，絕不為 `null` | eventdriven | API contract test + `Test_Predict_JSONMarshal_HasSectorPredictions` | 🟡 pending | 需修改 `PredictionReport` struct |
| I2 | 每個預測日必須包含 **全部 20 個 L1 canonical sectors**，且排序固定 | eventdriven | `Test_SectorPredictions_Exactly20L1Sectors` | 🟡 pending | 依 `internal/industry/sector.go` canonical IDs |
| I3 | 每個 `distribution` 必須是合法機率分布：`inflow + neutral + outflow == 1.0`（容許浮點誤差 1e-6） | eventdriven | `Test_SectorPrediction_DistributionSumsToOne` | 🟡 pending | 純函式保證 |
| I4 | `sector_id` 必須是 canonical L1 ID；不允許 raw Chinese name 或 stock symbol | eventdriven | `Test_SectorPrediction_IDCanonical` | 🟡 pending | 使用 `industry.NormalizeSectorID()` |
| I5 | Sector-weighted 預測與整體預測的 **JSD ≤ 0.25**；若超過，confidence 必須衰減 0.85 且顯示 warning | eventdriven | `Test_SectorPredictions_ConsistencyJSD` + frontend assertion | 🟡 pending | 核心 investor-protection invariant |
| I6 | 歷史 baseline 僅在 **N ≥ 24（月度）/ 12（季度）/ 3（年度）** 時使用 empirical；否則強制 Bayesian shrinkage | eventdriven | `Test_SectorPredictor_BaselineShrinkage` | 🟡 pending | 避免 overconfidence |
| I7 | 模型 confidence floor = 0.40；不允許出現 confidence < 0.40 的輸出 | eventdriven | `Test_SectorPredictor_ConfidenceFloor` | 🟡 pending | 對齊 calibration 標準 |
| I8 | Frontend 預設 **摺疊**；展開後顯示 5 必須看板塊，可切換全部 20；必須記憶 localStorage | client_web | `__tests__/sector_predictions.test.mjs` | 🟡 pending | UX invariant |
| I9 | Frontend 對 missing/empty `sector_predictions` 必須顯示 empty state，不可 crash | client_web | `__tests__/sector_predictions.test.mjs` | 🟡 pending | defensive invariant |
| I10 | 新增計算後 `/api/events/prediction` p95 latency < 200ms | eventdriven | benchmark in integration test | 🟡 pending | 效能 invariant |
| I11 | Feature flag `SECTOR_PREDICTION_ENABLED` 預設 **false**；開啟前須通過 L2.4 observation window | config | `docs/reference/parameters.md` + runbook | 🟡 pending | rollout safety |
| I12 | Out-of-sample Brier score ≤ 0.20；hit-rate ≥ 0.55 | data-science | `cmd/run-experiment` backtest | 🟡 pending | accuracy invariant |

---

## 2. Phase-by-Phase Execution Tracker

### Phase 0 — Data Availability (估計 1 週)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 0.1 盤點 TWSE / FinMind 板塊日報酬可取得性 | I12 | 🟡 pending | 資料覆蓋率報告 |
| 0.2 建立 stock symbol → canonical sector mapping | I4 | 🟡 pending | `internal/industry/stock_sector_map.go` + tests |
| 0.3 建立 canonical sector alias mapping（中文 → ID） | I4 | 🟡 pending | mapping 表 |
| 0.4 Backfill 板塊日報酬 2023-07-01 ~ 2026-07-01 | I12 | 🟡 pending | `data/sector_data/sector_returns.jsonl` |
| 0.5 產生 event_type × sector hit table | I6 | 🟡 pending | lookup table + N 統計 |
| 0.6 資料覆蓋率 ≥ 95% 驗證 | I12 | 🟡 pending | coverage report |

**Phase 0 退出條件**：若 0.1 發現板塊日報酬無法取得 ≥ 95% 覆蓋率 → **停止 C07**，改為 frontend 只顯示 `affected_industries` 標籤，不聲稱預測。

---

### Phase 1 — SectorPredictor Core (估計 1 週)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 1.1 設計 `SectorPredictor` interface | I3, I4, I7 | 🟡 pending | `internal/eventdriven/sector_predictor.go` |
| 1.2 實作 empirical baseline with Bayesian shrinkage | I6 | 🟡 pending | `Test_SectorPredictor_BaselineShrinkage` |
| 1.3 實作 macro exposure adjustment | I3 | 🟡 pending | beta lookup + tests |
| 1.4 實作 cycle position shift | I3 | 🟡 pending | cycle.go integration tests |
| 1.5 實作 hybrid score + distribution softmax | I3, I7 | 🟡 pending | deterministic tests |
| 1.6 實作 JSD consistency check | I5 | 🟡 pending | `Test_SectorPredictions_ConsistencyJSD` |
| 1.7 純 offline backtest（2026-Q2 hold-out） | I12 | 🟡 pending | hit-rate / Brier report |

---

### Phase 2 — API Integration (估計 1 週)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 2.1 新增 `SectorPrediction` / `SectorDayPrediction` types | I1, I4 | 🟡 pending | `internal/eventdriven/types.go` |
| 2.2 將 `sector_predictions` 注入 `PredictionReport` | I1, I2 | 🟡 pending | `predictor.go` + tests |
| 2.3 API contract test：必含 `sector_predictions`、20 sectors × 5 days | I1, I2 | 🟡 pending | integration test |
| 2.4 新增 feature flag `SECTOR_PREDICTION_ENABLED` | I11 | 🟡 pending | `docs/reference/parameters.md` |
| 2.5 效能 benchmark：p95 latency < 200ms | I10 | 🟡 pending | benchmark result |

---

### Phase 3 — Frontend (估計 1 週)

| Task | Invariant(s) | Status | Evidence |
|---|---|---|---|
| 3.1 新增 `cp-sector-predictions` template section | I8 | 🟡 pending | `capital_predictions.js` |
| 3.2 實作摺疊/展開 + summary badge | I8 | 🟡 pending | unit test |
| 3.3 實作 5 必須看板塊渲染 | I8 | 🟡 pending | unit test |
| 3.4 實作「顯示全部 20 板塊」切換 | I8 | 🟡 pending | unit test |
| 3.5 實作 driver tooltip + consistency warning | I5, I8 | 🟡 pending | unit test |
| 3.6 localStorage 記憶偏好 | I8 | 🟡 pending | unit test |
| 3.7 empty state 處理 | I9 | 🟡 pending | unit test |
| 3.8 Frontend tests ≥ 15 cases | I8, I9 | 🟡 pending | `npm test` pass |

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

- **Phase 0**: 尚未開始
- **Phase 1**: 尚未開始
- **Phase 2**: 尚未開始
- **Phase 3**: 尚未開始
- **Phase 4**: 尚未開始
- **Phase 5**: 尚未開始

**Next action**: 進入 Phase 0 Task 0.1 — 盤點板塊日報酬資料可取得性。

---

## 4. Risk Register

| Risk | Probability | Impact | Mitigation | Trigger to stop |
|---|---|---|---|---|
| 板塊日報酬資料無法取得 ≥ 95% | 中 | 高 | 優先 TWSE，fallback FinMind | 覆蓋率 < 95% → Phase 0 退出 |
| 事件標籤 canonicalization 複雜 | 中 | 中 | 建立 alias mapping + fuzzymatch | 無法 mapping 的標籤 > 15% → 暫停 |
| 模型 overconfident | 中 | 高 | Bayesian shrinkage + JSD + floor 0.40 | Brier > 0.25 → Phase 4 退出 |
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

---

## 6. Change Log

| Date | Version | Change | Author |
|---|---|---|---|
| 2026-07-16 | 1.0 | Initial invariant tracker based on spec v1.0 | atlas-dev |
