# 2026-08-05: `auto_cycle_update` 80+ 日卡 error — quota 結構性問題假設被推翻

> **結論先行**：「`auto_cycle_update` 卡 80+ 日是 quota + 月初結構性問題」這個假設**沒有任何 runtime 證據支持**。真實根因更可能是 (a) FinMind 對特定 symbol 沒月營收資料 / (b) 月初月營收尚未 publish。**PR-E（commit `2d90a401`）已修的 `endDate=31` bug 對 5 月（31 天）沒影響**，與 5/14 起持續出現的 error 不對應。
>
> **下一個 agent 請勿**僅憑 `finmind_client.go:35-48` 註解或 PR-E commit message 就推導出「quota 月初打爆」的結論。

---

## 問題陳述

`/api/dashboard/channel-health` 上的 `auto_cycle_update` channel 從 2026-05-14 起 `status=error`、`last_error="data_aggregator: no valid data for industry \"electronics\""`、`updated_at=2026-05-14T08:01:09+08:00`，距 2026-08-05 已 83 天。

初始假設（已推翻）：
1. PR-E 修了 `endDate=31` 的 bug，理論上應該解掉
2. 沒解掉 → quota 月初被 auto_cycle_update 打爆 → channel 沉默
3. 這是 quota + 月初的結構性問題，1-2 人天獨立 PR

## 三項驗證與結論

### 驗證 1：task 是否還在跑

**方法**：MCP `task_get` 拉 scheduler 狀態。

**證據**：
- `auto_cycle_update` task：`enabled=true`、`interval=21600000000000ns`（6h）、`last_run=2026-08-05T08:20:59Z`、`consecutive_failures=1`
- `next_run=2026-08-05T14:20:59Z`

**結論**：task **持續在跑**，每 6h 一次，今天 30 分鐘前才跑過。「卡 80+ 日」是 channel_health.json 的 record 沒被刷新，不是 task 沒跑。

### 驗證 2：5/14 error 根因

**方法**：
1. 追 `data_aggregator.go:113-115` — `no valid data for industry` �發條件 = `revCount==0 && profitCount==0`
2. 追 `fetchRevenueYoY`（line 144-188）— 3 個月 fallback × 每個 representative stock
3. 追 `defaults_narrative.go:1191` — electronics representative stocks = `2317.TW` (鴻海)、`2382.TW` (廣達)、`2357.TW` (華碩)
4. 從 MCP `task_get` 拿最新 `last_error`：`"data_aggregator: no valid data for industry \"leo_satellite\""`

**證據**：
- 最新一次失敗的是 `leo_satellite`（不是 `electronics`）— 表示每次跑失敗的 industry **不同**
- 鴻海 5/29 chips 有資料（透過 `stock_get_chips` 拉到 `date: "20260529"`）— 鴻海並非 FinMind 缺失
- FinMind token 還能用：`finmind` channel `last_success_at=2026-08-05T08:20:59Z`、status=ok、今天已用 3572 calls

**結論**：根因**不是 quota**，而是「特定 industry representative stocks 在 FinMind 拿不到月營收資料」。失敗 industry 隨 task run 而變（electronics 是 5/14 那次，leo_satellite 是最近一次）。

**PR-E 與 5/14 error 的關係**：PR-E 修了 `endDate=31` 對 30-day 月份的問題。**5 月有 31 天**，PR-E 不會影響 5 月的 request 構造。兩者**無因果關係**。

### 驗證 3：quota 月初瓶頸

**方法**：從 production `internal/marketdata/data/state/finmind_daily_quota.json` 讀當前 quota 狀態。

**證據**：
```
{"calls_today":3572,"last_reset":"2026-08-05T08:00:00+08:00"}
```
（`08:00:00+08:00` = UTC 午夜，無 bug）

- 當前用量 3572 / 上限 14400 = **24.8%**
- 預估 daily 用量 ~900 calls/day（從 8/5 累積推算）
- 離上限 14400 還有 **75% 餘裕**

**`auto_cycle_update` 自身用量估算**：
- 14 industries × 平均 2.5 stocks × (revenue 3m×2 + profit 3q×2 = 12 calls) = 420 calls per run
- 6h × 4 = 1680 calls/day from auto_cycle_update alone

加上 `channel_health_finmind` (24/day)、`auto_quote_backfill`、`tsmc_revenue` → 3572 是合理數字。

**結論**：沒有任何證據支持「quota 月初打爆」。真實瓶頸也不在月初。

---

## 推翻後的真實問題

`auto_cycle_update` task 持續在跑，但每次都失敗，失敗的 industry 隨 run 而變。可能根因（按概率排序）：

1. **月初月營收尚未 publish**：台灣 TWSE 月營收通常 10 號公告。5/14 雖然不是月初，但若 task 在 5 月某次 run 時 fetch「current=5 月」失敗，可能是 5 月前幾天資料還沒出。3 個月 fallback 對 5 月初 = 5/4/3 月，若 5 月初沒 publish 就 fail。

2. **FinMind 對邊緣個股沒資料**：`3426.TW` (台光電) 在 production 對 TWSE quote timeout（`stock_get_technical` 回 503 `insufficient historical quote data`），可能 FinMind 也沒完整收錄。

3. **FinMind `TaiwanStockMonthRevenue` 對某些 symbol 回空 array**：code `GetMonthRevenue` 在 `len(data)==0` 時回 `finmind: no month revenue data for {symbol} {year}-{month}`，aggregator 把這視為「沒資料」並 continue，最後落入 `revCount==0 && profitCount==0`。

4. **leo_satellite 是新興 industry**：`defaults_narrative.go:1198` 註明 `Weight: 0.02`、`RepresentativeStocks: []string{"6271.TW", "3426.TW"}`。新興 industry 收錄可能不完整。

---

## 給下一個 agent 的警示

**禁止僅憑以下 surface evidence 就推出「quota 結構性問題」的結論**：

| Surface evidence | 為何誤導 |
|---|---|
| `finmind_client.go:33-48` 的註解談 quota + month | 註解描述 FinMind 設計意圖，跟當前 `auto_cycle_update` failure 無直接因果 |
| PR-E commit `2d90a401` message 提「80+ day auto_cycle_update stale」 | PR-E 修的是 30-day/Feb 月份的 endDate bug；5 月（31 天）不受影響，與 5/14 起的 failure 無關 |
| `data_aggregator.go:115` `no valid data for industry` error 字串 | 觸發條件是「每個 stock 都拿不到資料」，不是 quota |
| channel_health.json `auto_cycle_update.last_success_at` 是空 | 表示從未成功，**但** task 確實在跑（驗證 1） |

**正確的診斷流程**：
1. 先用 MCP `task_get` 看 task 是不是真的沒在跑（不是 channel_health record frozen）
2. 看 task `last_error` 的當前內容（不是 channel_health.json 上的舊記錄）
3. 從 `defaults_narrative.go` 找 representative stocks 清單
4. 對 production quota state file 看實際用量

---

## 後續工作（2026-08-05 dispatch 規劃）

1. **error 分類 metric**：在 `internal/industry/data_aggregator.go` 區分 error type
   - quota exhausted（FinMind 回 402）
   - symbol not found（FinMind 回空 array 連續 N 次）
   - no data published yet（月營收月初尚未 publish）
2. **驗證 representative stocks**：對 14 industries 的全部 representative stocks 跑 `TaiwanStockInfo` 確認 symbol 存在
3. **fallback window 拉長**：從 3 個月拉到 4-6 個月，避開月初 publish lag
