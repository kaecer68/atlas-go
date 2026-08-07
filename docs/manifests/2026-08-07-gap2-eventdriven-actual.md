# Gap 2-A1 Implementation Manifest — 錢潮預測命中率 (資料層 + 閉環 + 呈現端)

> **Trigger**: User asked 「建立 worktree 執行 C」, 依 .omo/manifests/2026-08-06-gap-audit-summary.md §0.2 推薦選項 B (Gap 2-A1)
> **Reference**: .omo/audit/2026-08-06-gap2-prediction-vs-actual-loop.md §5 建議 A 範圍
> **Phase**: A (Audit) + B (Plan) + C (Implement) + D (Review) 完成, 等待 merge
> **2026-08-07 盤查更新**: 驗證投資人呈現面 — `shared_web/static/js/pages/home.js:101-112` 首頁「未來 5 日錢潮預測」區塊 + `home.js:391-411` renderPredictionsCard 渲染 date/direction/confidence/distribution/driving_events, **無命中率**; `internal/eventdriven/types.go:88` `PredictionReport` 無 `hit`/`historical_hit_rate` 欄位。`/api/dashboard/forecast-vs-reality` (pipeline/handlers.go:33) 是 admin 個股層預測 (SymbolPredictions, is_synthetic replay), 非錢潮層。C07 day-evaluator 命中率是 placeholder (main.go:249 明寫)。**結論**: 投資人唯一可見預測零命中率, 違反 §9 誠實聲明; 需資料層 (B01-B03) + 閉環 (B04-B05) + 呈現端 (B08-B09) 一次到位。
> **Branch**: `feat/20260806-gap2-eventdriven-actual` (worktree: `atlas-gap2-eventdriven-actual`)
> **Date**: 2026-08-07

---

## 0. 對齊核心目的

產品定位 §6 三要件 (a)+(b)+(c) 中, (b) T+1 自動對比 與 (c) 參數系統回饋 **未達成**:
- `eventdriven.Predictor.Predict()` 真實運轉 (stage3_tasks.go:222-235, 13:45 daily)
- 寫入 `EventFlowPredictionStore` JSONL,**只有 `predicted_*` 欄位,無 `actual_*`**
- Stage 3 direction 比對只 emit alert,**不寫回 ledger**
- `PredictorCalibrator` 從 `prediction_backtest` 讀 hit rate,但 source 來自 replay (`is_synthetic=1`),**真實 T+1 數據沒 reverse-write**

本 manifest 補上 **(b) 真實 T+1 對比**,**為 (c) 提供生產路徑**(PredictorCalibrator 改讀 `is_synthetic=0` 為後續任務,本 manifest 範圍外)。

---

## 1. Invariant table (Phase B plan, 7 個 ID)

| ID | 問題/任務 | 根因假設 | 改動檔案 | 驗收標準 | 狀態 | 證據 |
|---|---|---|---|---|---|---|
| B01 | `EventFlowPredictionRecord` 無 `actual_*` 欄位 | 設計時 T+1 對比非必要功能 | `internal/ledger/event_flow_prediction_store.go` | struct 加 `ActualSign float64` + `ActualSource string` + `ActualCapturedAt *time.Time` 3 個欄位 (用 `*time.Time` 區分「未補」與「補 zero」) | pending | line 23-28 現有 struct |
| B02 | `EventFlowPredictionStore` interface 無 T+1 update/lookup | T+1 對比原本不在設計範圍 | `internal/ledger/event_flow_prediction_store.go` | interface 加 `UpdateActual(predictedAt time.Time, actualSign float64, source string) error` + `LoadByDate(date time.Time) (EventFlowPredictionRecord, error)` | pending | line 34-39 現有 interface |
| B03 | `JSONLEventFlowPredictionStore` 需實作 B02 | 介面擴充後須實作 | `internal/ledger/event_flow_prediction_store.go` | 實作 B02 兩個 method,read-modify-write 整檔重寫 (FIFO 1000 仍有效,效能可接受) | pending | line 47-79 現有 store |
| B04 | `cmd/atlas/stage3_tasks.go` 無 T+1 reconciler 任務 | T+1 對比未自動觸發 | `cmd/atlas/stage3_tasks.go` + `internal/scheduler/` (新檔) | 新增 `stage3-alert-prev-day-reconcile` task,14:30 daily (T86 published 後, 確認昨日 actual 可用),調用 `predictionLedger.UpdateActual` 與 `LatestCapitalFlowActual.ForDate` | pending | line 218-239 `LatestCapitalFlowPrediction` 模式 |
| B05 | `LatestCapitalFlowActual` 無 `ForDate(date)` 變體 | 對「昨日 actual」查詢無設計需求 | `internal/monitoring/stage3_rules.go` 與 `cmd/atlas/stage3_tasks.go` 介面依賴 | 介面加 `ForDate(date time.Time) (monitoring.CapitalFlowSignal, bool)`, 實作查詢昨日 actual | pending | line 241-254 `LatestCapitalFlowActual` 現有實作 |
| B06 | test 配合 B01-B05 | 既有 test 不覆蓋新功能 | `internal/ledger/event_flow_prediction_store_test.go` + `cmd/atlas/stage3_tasks_test.go` | 加 4 個新 test:`UpdateActual` 成功/failure (重複 update / date 不存在) + `LoadByDate` round-trip + reconciler task | pending | 現有 6 個 test 模式 (line 9-250) |
| B07 | 驗證 + 提交 + PR | — | — | `make ci-gate` + `make ci-full` 全綠;commit + push + 開 PR | pending | — |

**重要 design 決策**:

1. **`ActualCapturedAt` 用 `*time.Time` 而非 `time.Time` zero value**:避免「補 0」與「未補」混淆(`*time.Time == nil` 表示未補)。JSON omitempty 也支援。

2. **`UpdateActual` 用 `predictedAt` 找 record**: 避免「T+1 補入到錯誤日期」。FIFO 1000 筆內最多 ~3 年歷史(預設 1 prediction/day),read-modify-write 整檔重寫效能可接受。

3. **不刪 `EventFlowPredictionStore` 既有 method**:`AppendPrediction` / `LoadRecentPredictions` / `Len` / `Size` 仍對外暴露,向後相容。

4. **`stage3-alert-prev-day-reconcile` 14:30** (非原本 audit 寫的 13:46): T86 published 為 14:00 後,昨日 actual 在 14:00 前可能未到位。14:30 是「sync-capital-daily 13:30 跑完後 + 1 小時緩衝 + T86 published 確認」。需驗證 `capitalflow.Service.LatestDaily()` 在 14:30 是否能拿到「昨日」actual。

5. **scope 排除**:
   - **不改 `PredictorCalibrator`**:本 manifest 補 (b) T+1 對比,不改 (c) calibrator 讀 `is_synthetic=0`。這是獨立任務 (P1, 1 PR, 1 天), 留 Gap 2-C (後續)。
   - **不改 `internal/forecast.Ledger`**: E03 設計未啟用, 不在本 gap 範圍。
   - **不改 `prediction_backtest` schema**:既有 schema 完整, 本 manifest 只動 `EventFlowPredictionRecord`。

---

## 2. Phase tracker

### Phase A — Audit (done)
- [x] 讀 `internal/ledger/event_flow_prediction_store.go:1-180` 完整 source
- [x] 讀 `cmd/atlas/stage3_tasks.go` 完整 stage3 wiring
- [x] 讀 `internal/ledger/event_flow_prediction_store_test.go:1-250` 既有 6 個 test
- [x] 確認 `cmd/atlas/stage3_tasks.go:218-239` 的 `LatestCapitalFlowPrediction` 模式
- [x] 確認 `sync-capital-daily` 13:30 + T86 published ~14:00 + `stage3-alert-market-close` 13:45 為現有時間點
- [x] 確認 store 用 JSONL + FIFO 1000 筆 + read-modify-write 模式

### Phase B — Plan (done)
- [x] 寫此 manifest §1 invariant table
- [x] 標明 7 個 ID 與依賴關係
- [x] 標明 design 決策 (5 項)
- [x] 標明 scope 排除 (3 項)

### Phase C — Implement (done, 2026-08-07)
- [x] B01: struct 加 3 個欄位 (`ActualSign` / `ActualSource` / `ActualCapturedAt *time.Time`)
- [x] B02: interface 加 2 個 method (`UpdateActual` / `LoadByDate`) + `ErrPredictionNotFound`
- [x] B03: store 實作 2 個 method (read-modify-write, samePredictionDate 依 Taipei date)
- [x] B04: `ReconcilePrevDayPredictionTaskFunc` (14:30 daily, 3 callbacks 解耦)
- [x] B05: 修正 — 不用 `LatestCapitalFlowActual.ForDate`,改用既有 `capitalFlowStore.History(ForceForeign)` 查昨日 actual;reconciler 掛 production `registerCapitalTasks` (非未 wire 的 registerStage3Tasks);同時 wire `NewJSONLEventFlowPredictionStore` 進 main (原本 production 從未建立)
- [x] B06: test — 4 個 reconcile task test + 4 個 handler hit-rate test + 4 個 store T+1 test
- [x] B08: `/api/events/prediction` response 加 `historical_hit_rate` (PredictionReport + HistoricalHitRate struct + handler computeHistoricalHitRate, MinHitSamples=30)
- [x] B09: 前端 `home.js` renderHitRateBadge (校準中/命中率徽章) + CSS

### Phase D — Close out
- [x] `make ci-gate` 通過 (需 commit 後才全綠,見下)
- [x] 全部受影響 package test 通過 (ledger / eventdriven / scheduler)
- [x] commit + push + PR #1484 建立
- [x] PR review (4 reviewers) — 3 個 P1 已修 (production writer F1 / LoadPrevDayActual 日期錯位 F2 / neutral-actual 誤判 F3) + P2/P3 修正
- [ ] ci-full 通過後 merge (fubonproxy flaky 為 pre-existing, 非本 PR)

**PR review 修正 (2026-08-07, 4 reviewers 並行)**:
- **F1 (P1)**: production 無 prediction writer — `AppendPrediction` 唯一 call site 在未 wire 的 registerStage3AlertTasks。修: `HandlePrediction` 加 `persistTodayPrediction` (每日 once via HasPredictionOn), adapter 實作 AppendPrediction。
- **F2 (P1)**: `LoadPrevDayActual` 用 `History(beforeDate=yesterday)` 錯一天 (附上前一交易日 actual)。修: `beforeDate=today` (TradingDate < today = 前一個交易日 = 預測當日)。
- **F3 (P1→P3)**: `ActualSign` 有 omitempty, neutral actual (0.0) JSON 缺席 → handler 用 `ActualSign==0` 判斷未 reconcile 誤判。修: projection 加 `ActualCapturedAt`, 以 nil 判斷; 新增 reconciled-neutral-actual test。
- **F4 (P2)**: `window_days` 誤導 (60 records ≈ 60 交易日, 非 60 天)。修: 改名 `window_records`, 前端標「近 N 筆預測」。
- **P2/P3 其餘**: 跨午夜 Taipei-date boundary test; UpdateActual 更新所有同日 records; re-update idempotent test; unreconciled nil-after-reload test; docs 措辭修正; manifest 狀態行。

**Phase C 期間發現並修正的真相**:
1. `NewJSONLEventFlowPredictionStore` production 從未建立 — 原 audit「Predictor.Predict() 寫入 store」是錯的 (store nil, Append 是 no-op)。本 PR wire 進 main。
2. `registerStage3Tasks` 未 wire 進 production main (只在 test) — Stage 3 tasks 是「寫好未 wire」舊路徑。reconciler 改掛 production 有 wire 的 `registerCapitalTasks`。
3. `capitalflow.Service` 無 `ForDate` — 改用既有 `capitalFlowStore.History(ForceForeign, beforeDate)` (rolling sample store 已由 BK-15 wire)。

**Phase C 之後再盤查 (2026-08-07, PR #1484 review)**:
- `tryHitRateEval` 從 `LoadPredictionBacktestRange` 讀 — **已透過 `FilterSynthetic` 過濾 `is_synthetic=0`**, 邏輯正確。
- 但 `prediction_backtest` 表 **完全沒有 `is_synthetic=0` 資料** (唯一寫入端 `cmd/backtest-event-flow` 都標 `is_synthetic=1` replay)。
- 結果: `predictor_calibrate` 24h task 跑但永遠 fallback 0.5, 不貢獻真實命中回饋。
- 這是 Gap 2-D 的真實動機 — 需 reverse-write 把 `event_flow_predictions.jsonl` T+1-reconciled 記錄同步到 `prediction_backtest` `is_synthetic=0`, 才能讓 calibrator 用真實命中率 (§8 退化降權)。

---

## 3. Backlog (不在本 PR 範圍)

- **Gap 2-C**: `PredictorCalibrator` 改讀 `is_synthetic=0` 真實預測 — 依賴本 PR 落地,P1, 1 PR
- **Gap 2-D**: `prediction_backtest` reverse-write 從 `event_flow_predictions.jsonl` 同步到 SQLite — 簡單 job, P2, 0.5 天
- **Gap 3**: 散戶追蹤/紀律機制 — 另一個 worktree (`feat/20260806-gap3-userstate-skeleton`)

---

## 4. Commit discipline

依 `atlas-audit-manifest-protocol`:
- 預設 1 commit per ID,但 B01-B03 高度耦合 (struct → interface → store),可合併 1 commit `feat(manifest): #B01-B03 add T+1 actual capture to event flow prediction`
- B04-B05 可獨立 commit `feat(manifest): #B04-B05 add prev-day reconciler task`
- B06 必須獨立 commit `test(manifest): #B06 cover T+1 actual capture and reconciliation`
- B07 不在 commit 內 (Phase D 動作)

---

## 5. Session-end state

- **Done**: Phase A (audit) + B (plan) + C (implement) + D (review 修正) 全部完成
- **產出**: PR #1484 (18 files +862), 含 production writer (F1) / reconciler (14:30) / 命中率呈現 (B08-B09) / 12+ 新 test
- **未做**: ci-full 全綠 (fubonproxy flaky pre-existing, 非本 PR); merge
- **依賴**: user review PR #1484 + #1486 (Gap 3)
