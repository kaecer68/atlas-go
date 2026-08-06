# Gap Audit 1 — 預測命中率可見化

> **盤查日期**: 2026-08-06
> **盤查者**: atlas AI (scout subagent, 100-request budget 限制下 partial)
> **範圍**: READ-ONLY 盤查,未動任何 code
> **ACI 工具鏈**: `gitnexus_query` (hit rate tracking)、`codebase-memory_explore` (FeedbackStore)、`grep` (572 行 hit_rate / 599 行 accuracy / 245 行 命中率)、`read` (handlers / ranker / types / evaluator / main.go)

---

## 1. 背景

atlas-go 產品定位 §8 明言「任何外部經驗法則 (如『外資連買 5 天是信號』) 不得直接硬編碼為規則,必須經過:

```
假設登錄 → 歷史資料校準(命中率/報酬統計) → 寫回 parameters.json → 持續追蹤命中率 → 退化自動降權
```

問題:這條閉環的 **可見化** 端是否真的存在? 散戶登入 `/client/` web dashboard,能否看到「過去 30 天,某 L1-L5 訊號對隔日大盤方向的命中率」?

---

## 2. 現況發現

### 2.1 已實作的部分

| 元件 | 位置 | 狀態 |
|---|---|---|
| `ConditionEvaluator` 訊號端 | `internal/strategy_techniques/evaluator.go` | ✅ 對 macro snapshot 算策略命中率 |
| `FeedbackStore` 持久化 | `data/state/strategy_feedback/` (per strategy JSONL) | ✅ 介面存在,目錄存在但**無檔案**(未實際累積) |
| `prediction_backtest` SQLite | `internal/ledger/sqlite_core.go` | ✅ Schema 完整: `(predicted_direction, predicted_confidence, actual_direction, actual_capital_flow_change, hit, model_version, captured_at, is_synthetic)` |
| `RecommendationOutcome` struct | `internal/domain/recommendation/recommendation.go:40-110` | ✅ `Hit bool` + `HitRate float64` 欄位 |
| `/api/strategies` 端點 | `internal/monitoring/api/strategies/handlers.go:301-495` | ✅ 回傳單點 hit_rate |
| `/api/strategies/{id}/summary` | 同上 | ✅ 回傳單點 hit_rate |
| `darwinian/trend` 端點 | `internal/monitoring/api/pipeline/handlers.go:530-595` | ⚠️ 顯示 darwinian 趨勢但不是逐日 hit_rate |
| `client_web` strategies 頁 | `client_web/static/js/main.js:172` 引用 `./pages/strategies.js` | ✅ esbuild plugin fallback 到 `shared_web/static/js/pages/strategies.js` (line 300-613 含 hit_rate 渲染) |

### 2.2 缺失的部分

| 元件 | 證據 | 缺口 |
|---|---|---|
| **過去 N 天逐日 hit_rate 時間序列** | `grep` `historical_hit_rate` / `rolling.*hit_rate` / `hit_rate.*history` 全 codebase 0 命中 | 沒有任何端點回傳時間序列 |
| **`/api/strategies/{id}/history`** | 0 命中 `HandleStrategiesHistory` / `strategies_history` / `skill_history` | 端點不存在 |
| **`/api/prediction-backtest`** | 0 命中 `HandlePredictionBacktest` / `prediction_backtest_range` | 端點不存在,SQLite 表對 dashboard 不可見 |
| **client_web 滾動命中率圖** | `shared_web/static/js/pages/strategies.js` 只渲染「策略卡片 + 單點 hit_rate」 | 無時間序列圖表 (e.g. Chart.js / ASCII sparkline) |
| **跨策略命中率排名** | `internal/strategy_ranker/handler.go` 只回傳靜態排名 | 不顯示「哪個策略最近 30 天最好」 |

### 2.3 死資料狀態

- `data/state/strategy_feedback/` 目錄存在但**無任何 JSONL 檔案** — 從未累積過 validation event
- `prediction_backtest` 表存在但**僅由 `cmd/backtest-event-flow` replay 90 天填入** (`is_synthetic=1`),**production code path 完全沒人寫入**

---

## 3. 缺口判定

> **部分缺口 (Partial Gap)**。訊號→outcome 對應資料在後台運轉,但散戶介面**只能看單點 hit rate**,沒有時間序列 dashboard。

不是「完全沒有」,而是「**資料在,可見化是單點**」。

---

## 4. 對齊核心目的程度

| 核心目的 | 對齊程度 |
|---|---|
| 散戶能看到命中率 | ⚠️ 中 — 單點可見,時間序列不可見 |
| §8 校準哲學 (持續追蹤 + 退化降權) | ⚠️ 中低 — `PredictorCalibrator` 從 `prediction_backtest` 讀,但 source 來自 replay 而非真實 T+1 (見 Gap 2) |
| 散戶做投資判斷時的依據 | ⚠️ 中 — 只能看「現在命中率」,不能看「過去 30 天趨勢」 |

整體對齊 ~50-60%。

---

## 5. 建議

**最小可行範圍 (若要做)**:

1. 新增 `/api/strategies/{id}/history?days=N` 端點,回傳時間序列
2. 改 `shared_web/static/js/pages/strategies.js`,加 ASCII sparkline 渲染過去 30 天 hit_rate
3. 同步補 `data/state/strategy_feedback/` 真實 validation 寫入路徑(目前是手動或 backtest 觸發,production 沒有)

**前置依賴**:
- Gap 2 (eventdriven prediction 補 actual) 必須先做,否則 `prediction_backtest` 表沒有 `is_synthetic=0` 的真實資料,時間序列也只是 replay 數據

**優先級**: 中 (P1)。**不建議在 Gap 2 之前單獨做** — 否則暴露的時間序列是假的(replay 而非真實),反而誤導散戶。

---

## Summary

- **findings_summary**: 已部分實現單點 hit rate 可見化 (client_web strategies 頁),缺時間序列 dashboard 與 prediction_backtest 端點
- **is_real_gap**: true (partial)
- **value_to_core_mission**: medium
- **recommended_action**: 暫不做;等 Gap 2 (eventdriven actual) 補完後,再做時間序列暴露 (P1,1-2 天工作量)
