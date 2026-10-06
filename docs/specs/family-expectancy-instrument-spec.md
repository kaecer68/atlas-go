# 訊號家族淨期望值與體制分層儀器規格（唯讀）

| 項目 | 內容 |
|---|---|
| 文件角色 | 「訊號家族（source）淨期望值 + 體制分層」儀器的**口徑 SSOT**：定義、視窗口徑、資料品質處理、錯誤契約 |
| 狀態 | v1（2026-10-06，純新增唯讀面） |
| Source-of-truth | 定義：本檔；數學：`internal/stockpicker/winrate.go`（`NetHit` / `WinRate` / `WilsonScoreInterval` / `CalibrationStatusFor`）＋ `internal/stockpicker/family_expectancy.go`（`NetExpectancyT`）；參數：`configs/parameters.json` → `stockpicker.*` |
| 上層口徑 | [`industry-hitrate-metric-spec.md`](industry-hitrate-metric-spec.md)（canonical 命中率定義） |
| 實作 | `internal/stockpicker/family_expectancy.go`、`internal/stocktools/family_expectancy.go`、`internal/stocktools/handler.go` |

> **一句話**：同一個 canonical 扣成本口徑（`net_forward_return > 0`、固定 5 交易日、成本率 0.585%、Wilson 95% CI），聚合鍵換成 **`source`（家族）**，再加上**淨期望值 t 統計量**、**體制（regime）分層**與**近 60 個交易日子窗**，讓 owner 一眼看出「哪個家族還活著」。
>
> **本儀器只做量測揭露，不構成投資建議。** 不改任何既有計算、不改 Darwinian 權重、不動任何既有數值。

---

## §1 口徑（與產業命中率規格同一套）

| 要素 | 值 | 依據 |
|---|---|---|
| 命中判定 | `forward_return − cost_rate > 0`（嚴格大於，打平不算） | `winrate.go` `NetHit` |
| 淨報酬 | `forward_return − cost_rate`（每列重算，採**現行參數**成本率，非讀取已存的 `net_forward_return` 欄位） | 同 `industry_winrate.go` |
| 持有期 | 5 交易日（既有固定持有期） | `daily_update.go` `DefaultForwardDays` |
| 成本率 | 0.585%（`stockpicker.costs.round_trip_pct`） | `configs/parameters.json` |
| 最小樣本 | 30（未達 = `calibrating`） | `stockpicker.calibration.min_samples` |
| 信賴區間 | Wilson，95% | `winrate.go` `WilsonScoreInterval` |
| 家族 | `source` 全字串（含 `stockpicker-` 前綴）；`condition_id` = 去前綴，`direction` 由 `IsAvoidCondition` 決定 | `conditions.go` |
| t 統計量 | `mean / (sampleSD / sqrt(n))`（**唯一實作**：`NetExpectancyT`） | `family_expectancy.go` |
| 資料源 | `stock_signal_outcomes`（job-local ledger，`mode=ro` 開啟）；**全期 = 表內全部 rows，不做 rolling_window 過濾** | `signal_outcome_store.go` |

## §2 輸出（`GET /api/stock/family-expectancy`）

報告層：`generated_at`、`trailing_trading_days`、`trailing_window_start/end`、`total_observations`、`invalid_trigger_dates`、`non_trading_trigger_dates`、`unattributed_observations`、`families[]`（依 `source` 升冪）。

每列家族（`families[]`）：

| 欄位 | 說明 |
|---|---|
| `source` / `condition_id` / `direction` | 家族識別；`direction` = `buy` 或 `avoid`（反轉語義沿用） |
| `n` / `hits` / `net_hit_rate` | 全期樣本、命中數、命中率（＝ `WinRate`） |
| `wilson_lower` / `wilson_upper` / `confidence` | Wilson 95% CI |
| `calibration_status` | `CalibrationStatusFor(n, 30)` |
| `net_cost_rate` | 本次讀取使用的成本率（0.585%） |
| `symbols` | 相異 symbol 數 |
| `avg_net_forward_return` | 全期`淨報酬`平均 |
| `net_expectancy_t` | 全期淨期望值 t 統計量 |
| `data_start` / `data_end` | 有效 `trigger_date` 的最小/最大值（全期） |
| `trailing_n` / `trailing_avg_net_forward_return` / `trailing_net_expectancy_t` | **近 60 個交易日**子窗的同義指標 |
| `trailing_window_start` / `trailing_window_end` | 實際使用的子窗邊界（`trailing_n = 0` 時仍回填） |
| `by_regime[]` | 體制分層：`regime` / `n` / `net_hit_rate` / `wilson_lower` / `wilson_upper` / `avg_net_forward_return` / `net_expectancy_t`；排序 = 樣本數遞減，再 `regime` 升冪 |
| `generated_at` | 報告 `generated_at` 的回音（呼叫端注入時鐘，測試可決定性） |

## §3 子窗（trailing）口徑 —— 這是本儀器存在的理由

- 子窗 = **本次讀取中最近 60 個「相異 `trigger_date`」**（去重後排序，取最大 60 個；跨全部家族一起取，非各家族自算）。`trailing_days` 可覆寫（預設 60）。
- 錨定在全域日期（而非該家族自己的日期）：若用家族自己的最後 60 天，一個**半年前就停止觸發**的家族仍會看起來健康；用全域視窗它會得到 `trailing_n = 0`，正是「還活著嗎」的答案。
- 去重是對**日期**：同一天多筆（多 symbol）算同一交易日；`trailing_n` 是落在子窗內的**列數**。
- 沒有交易日曆依賴：子窗邊界由資料自身決定，因此「無資料」與「無此窗」可分辨（`trailing_n = 0` 且邊界仍在）。

## §4 邊界與異常處理（不得靜默丟棄）

| 情境 | 處理 |
|---|---|
| `regime` 欄為空（目前多數 rows） | 歸 `unknown`（`stockpicker.UnknownRegime`）；**不補值、不猜**，也不併入任何已標記分層 |
| `trigger_date` 格式異常（非 `YYYY-MM-DD`，含空字串） | 計入 `invalid_trigger_dates`；**該列仍計入 `n` 與全期統計**，但不參與日期運算（`data_start/end`、子窗）——因無法定位到時間軸 |
| 週末日期（週六/週日） | 計入 `non_trading_trigger_dates`；**不移除**、仍占子窗一格。**已知限制**：不查證交所行事曆，國定假日無法辨識 |
| `source` 為空 | 無法歸戶；計入 `unattributed_observations`，不併入任何家族（持久層本就拒收空 `source`，僅記憶體呼叫端可達） |
| 樣本 0 / 1 或零離散（全同值） | 不 panic；`net_expectancy_t`（含 trailing）回 **0 哨兵值** ＝「無法定義」，**不代表無效果**，須與 `n`、`avg_net_forward_return` 合讀 |
| `trailing_days <= 0` | 子窗為空（邊界留空、`trailing_n = 0`），全期統計不受影響；端點對此回 400 |

## §5 錯誤契約與端點

- 路徑：`GET /api/stock/family-expectancy`（唯讀；`/api/stock/` 前綴在 `shared.AuthFreePrefixPaths` 內，無需 API key）。
- 參數：`trailing_days`（選填，正整數，預設 60）。**無必填參數**：家族之間不合併，故不需 `condition_id`。
- 回應：`found`（＝至少一個可歸戶家族）＋ `message`（`found=false` 時）＋ 報告本體。
- 錯誤：`400`（`trailing_days` 非法：非整數 / ≤ 0）；`503`（provider 未注入或 ledger 不可讀）；空 ledger → `200` + `found=false`。
- 已知限制（沿用同族端點）：`*_web/.../field_types.ts` 的產生器**不展開 embedded struct**，故 TS 型別只有 `found` / `message`。

## §6 測試（同 commit）

| 檔案 | 覆蓋 |
|---|---|
| `internal/stockpicker/family_expectancy_test.go` | t 統計量手算值與哨兵、共用 helper（Wilson/命中率）與既有口徑的**指紋固定值**（SignalWinRate / ConditionWinRate / IndustryWinRate 不回歸）、子窗 60 交易日去重選取與「死亡家族 `trailing_n=0`」、空 regime → `unknown` 不可補值、樣本 0 不 panic、格式異常與週末揭露、`trailing_days<=0`、排序決定性 |
| `internal/stocktools/family_expectancy_test.go` | 端點 happy path（含 t、strata、子窗、`generated_at`）、`trailing_days` 覆寫與 400、無 provider 503、provider 錯誤 503、空 ledger 200 + `found=false`、路由表**加法式註冊**、**既有回應位元組不變**（相容性） |

## §7 呈現紀律

1. 本儀器的數字屬 canonical 口徑（與 `industry-hitrate-metric-spec.md` 的 H1/H2/H2b **同判定式、同期間、同成本**），可與 symbol / condition / industry 三層直接比較。
2. 一律附 `n`、`wilson_lower/upper`、`calibration_status`；`n < 30` 不得下結論。
3. `net_expectancy_t = 0` 讀作「無法定義」，不得讀作「不顯著」。
4. **只做量測揭露，不構成投資建議**；不得由本儀器推導下單、權重或配置決策（權重公式本票未觸碰）。
