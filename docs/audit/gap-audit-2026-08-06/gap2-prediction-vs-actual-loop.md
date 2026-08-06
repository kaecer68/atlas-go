# Gap Audit 2 — 預測 vs 實際閉環

> **盤查日期**: 2026-08-06
> **盤查者**: atlas AI (scout subagent, 3m36s 完成)
> **範圍**: READ-ONLY 盤查,未動任何 code
> **ACI 工具鏈**: `codegraph_explore` (Ledger / Predictor / store / stage3 / calibrator)、`grep` (AppendPrediction / EventFlowPredictionRecord / predictionLedger / DirectionSign)、`read` (Ledger 主檔 + 11 個相關檔案)

---

## 1. 背景

產品定位 §6「預測可信的三要件」其中之一:**「預測 vs 實際的同單位誤差追蹤」**。
產品定位 §8「持續追蹤命中率 → 退化自動降權」。

問題:`eventdriven.Predictor` / `forecast.Score` / 任何 prediction module 是否有:
- (a) 把 prediction 結果存進 ledger / DB
- (b) T+1 之後自動跟實際結果對比
- (c) 把誤差回饋給 calibration / parameter 系統

---

## 2. 現況發現 (細部證據)

### 2.1 真實運轉的部分 (微弱閉環)

```
[Predictor.Predict()]  →  JSONLEventFlowPredictionStore.AppendPrediction (predicted direction only)
   ↓
[13:45 cron: stage3-alert-market-close]
   ↓
[Stage3AlertEvaluator.evaluatePredictionDrift]
   ├─ LatestCapitalFlowPrediction()  → live predictor.Predict() (today)
   ├─ LatestCapitalFlowActual()      → capitalflow.Service.LatestDaily() (today)
   └─ emit "prediction-drift" alert (direction 比對,不寫 ledger)
```

### 2.2 斷點 (未真正運轉的部分)

| 斷點 | 證據 | 影響 |
|---|---|---|
| JSONL ledger 沒有 `actual_*` 欄位 | `internal/ledger/event_flow_prediction_store.go:13-28` `EventFlowPredictionRecord` 只有 `DirectionSign` + `Confidence` + `Direction` | T+1 沒地方寫 actual |
| `prediction_backtest` SQLite 雖有完整 schema | `internal/ledger/historical_store.go:75-110` schema 含 `ActualDirection` / `ActualCapitalFlowChange` / `Hit` / `is_synthetic` | 但 production path **沒人寫入** |
| `internal/forecast.Ledger` 是完整 T+1 infra | `internal/forecast/foreign_forecast.go:97-254` (Ledger / Write / Load / List / Judge / Calibrate) | grep 不到 `forecast.NewLedger` 在 cmd/ 內呼叫 |
| `PredictorCalibrator` 從 `prediction_backtest` 讀 | `internal/calibration/predictor_calibrator.go` (NewPredictorEvaluator) | 餵 Bayesian optimizer,但 source 來自 `cmd/backtest-event-flow` replay (`is_synthetic=1`),**真實 T+1 數據沒有 reverse-write** |
| Stage 3 alert 只 emit 不回饋 | `internal/monitoring/stage3_rules.go` `evaluatePredictionDrift` | 命中/未命中只 emit alert,**不寫 ledger、不回饋 calibration** |
| `cmd/atlas/stage3_tasks.go` 無 T+1 對比任務 | 全文 grep 不到 T+1 / actual_direction / hit 寫入 | **沒有 schedule 寫 `actual_direction` 或 `hit` 到 predictionLedger** |

### 2.3 重要修正 — `internal/forecast` 不是「死碼」

最初盤查的 subagent 報告將 `internal/forecast` 描述為「100% 完整 T+1 infra 但 production 未被任何 main wired (dead code)」。

**深入 git 歷史查證後的修正**:
- `internal/forecast/foreign_forecast.go` 來自 commit `9f71933e` — **「feat(forecast): 外資方向推估 v1 scorecard + ledger + 校準門檻 — #E03」**
- 對應 spec: `docs/specs/foreign-flow-forecast-spec.md` §6 明確寫「**首次啟用後至少需連續運行 90 個交易日才能驗證**」、§9 寫「**累積 ≥ 90 個交易日後,啟用對外展示**」
- `internal/forecast/AGENTS.md` 完整記錄此設計意圖,且**未標示為 deprecated**
- 對比: `internal/forecast_bridge/` (外部套件) 已在 commit `3e5808f4` 移除,理由是「Phase 3.5 M4 PoC shipped but never consumed by runtime. TradeSignal conversion now handled by `strategy.DirectionalTradeLayer` directly」

**修正結論**: `internal/forecast` 是 E03 manifest 的**完整設計 + 90 天手動 warm-up 計畫**,**不是「過度設計的無用代碼」**,而是**「依設計意圖尚未啟用」**。
- 不能僅因為「production 沒自動呼叫」就推論它是 dead code
- 若要啟用,需先寫一份「E03 啟用 plan」決定:實驗期、owner、退出條件、是否在 staging 跑

---

## 3. 缺口定位

| 閉環環節 | 狀態 |
|---|---|
| (a) prediction 寫入 ledger | ✅ 有 (`event_flow_predictions.jsonl` + `prediction_backtest` schema) |
| (b) T+1 自動對比 | ⚠️ 形式上有 (Stage 3 direction 標籤比對 emit alert),**但沒有把對比結果寫回 ledger** + 粒度只到標籤不是數值同單位誤差 |
| (c) 回饋給 calibration/parameter | ⚠️ 形式上有 (`PredictorCalibrator` 讀 `prediction_backtest`),**但真實 T+1 數據因為 (b) 斷也沒有被餵進這條路**;參數系統消費的是「replay 90 天」而非「真實營運 90 天」 |

---

## 4. 對齊核心目的程度

| 核心目的 | 對齊程度 |
|---|---|
| §6「同單位誤差追蹤」 | ❌ **不符合** — 只有「方向標籤」對比,僅當 direction mismatch 才 emit alert,沒有追蹤 confidence prediction error / capital flow 數值誤差的 closed-loop alerting |
| §8「持續追蹤命中率 → 退化自動降權」 | ❌ **不符合** — `predictor_calibrator` 雖然會調參數,但 feed 來源是 replay,iterator 沒有寫回「真實 T+1 命中率」到 parameter 系統的最近 N 天窗口 |
| 對 eventdriven.Predictor | ❌ **Real gap** — 預測有存,actual 從未補 |
| 對 forecast.ForeignForecast | ⚠️ **不是 gap,是「未啟用」** — 依 E03 設計意圖,90 天手動 warm-up |

整體對齊 ~40%。

---

## 5. 建議

**A. 對 `eventdriven.Predictor` 真實可做的 wire-up (P0)**:

1. `EventFlowPredictionRecord` 加 `ActualSign float64` + `ActualSource string` + `CapturedActualAt time.Time` 3 個欄位
2. `JSONLEventFlowPredictionStore` 加 `UpdateActual(date string, actualSign float64, source string) error` + `LoadByDate(date string) (EventFlowPredictionRecord, error)` 2 個 method
3. 新增 `stage3-alert-prev-day-reconcile` task,13:46 (market-close 後 1 分鐘),取昨日 prediction 與昨日 actual 寫回
4. 改 `LatestCapitalFlowActual` 加 `ForDate(date string)` 變體,允許查指定日期 actual
5. 改 `event_flow_prediction_store_test.go` + `stage3_tasks_test.go` 配合新欄位

範圍: 1 struct + 1 store + 1 task + 1 test 更新。**1 PR, 1 天工作量**。

**B. 對 `forecast.ForeignForecast` 不建議在沒有「E03 啟用 plan」的情況下動**:

- 這是**架構決策**,不是實作任務
- 需要 owner 確認:是否要在 production 啟用 90 天實驗、若啟用,資料源從哪來 (T86 published time? replay 加速?)、失敗退出條件
- 若無此 plan,動 `internal/forecast` 是把「依設計未啟用」當成「bug 要修」,違反設計意圖

**C. 對 `prediction_backtest` reverse-write 是獨立任務**:

- 把 eventdriven 補完 actual 後,還需另一個任務:讓 `PredictorCalibrator` 讀 `is_synthetic=0` 的真實資料(目前只讀 replay)
- 此任務依賴 A 完成,**應為第二個 PR**

---

## Summary

- **findings_summary**: eventdriven.Predictor 真實有 predicted 但無 actual (real gap);forecast.ForeignForecast 是 E03 未啟用設計 (非 gap);predictor_calibrator 讀的是 replay 不是真實營運
- **is_real_gap**: true (限 eventdriven 部分;forecast 部分是「未啟用」)
- **value_to_core_mission**: medium-high
- **recommended_action**: 對 eventdriven 做 A 範圍 wire-up (1 PR, 1 天);對 forecast 不動,等 owner 提供 E03 啟用 plan
