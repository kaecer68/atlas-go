# 2026-08-06: `auto_cycle_update` 撞 FinMind 402 quota 卡牆 — 真實根因盤查

> **給 AI agent 的導引**：本檔延續 `2026-08-05-auto-cycle-update-quota-misconception.md` 的盤查。該 doc 在 2026-08-05 PR-#1461 階段推翻「quota 結構性問題」假設,提出「個別 symbol 沒月營收資料」為更真實根因。本檔**進一步**盤查 2026-08-05 14:16:20 UTC production 觀察到的第二條真實根因:**FinMind server-side 402 quota 撞牆**,且**仍然沒被 trap 框架吸收**。
>
> **結論先行 (2026-08-06 amended)**: 「`auto_cycle_update` 失敗」有**兩個獨立根因**:
> 1. **8/5 14:16:20 UTC 那輪**: FinMind server-side 402 「Requests reach the upper limit」 — 3 task 同步 + auto_quote_backfill 首次 backfill 搶 token
> 2. **8/5 20:16:20 與 8/6 02:16:20 兩輪 (0 個 402)**: **本地 rate limiter 設計缺陷** — `fetchRevenueYoY` 5s ctx timeout < rate limiter 6s token interval, token 耗盡後 `Wait` 必失敗;error 兩層被吞 (`fetchRevenueYoY` fallback → `AggregateIndustry` 合成 error),metric 顯示 `no_data` 無法區分 (見 §5.1.1 P1.10 實證)
>
> 原稿僅歸因 402, 經 8 個 reviewer findings + P1.10 production 驗證修正為雙根因。**這條根因 20+ 次 FinMind PR 修補循環從未被識別**。
>
> **下一個 agent 必須**先讀本檔再對 `auto_cycle_update` 失敗做診斷,並把本檔連結加進 `docs/reference/traps.md` 的「FinMind 相關」trap 群組。

---

## 問題陳述

`auto_cycle_update` 2026-08-05 14:16:20 UTC 跑,5 秒內失敗 11 個 industries (`ai_supply_chain` / `consumer` / `electronics` / `energy` / `financials` / `industrial` / `leo_satellite` / `mining` / `robotics` / `semiconductor` / `shipping`),`consecutive_failures=1`。

PR-#1462 hotfix (merge 2026-08-05 14:09 UTC) 把 metric label 從 `kind=unknown` 修正為 `kind=no_data`。**但沒修真實的根因**。

同一個時間段, `channel_health_finmind` 也撞 402, `auto_quote_backfill` 也撞 402 — **3 個 background task 同步在 14:16:20-25 5 秒內對 FinMind token 撞牆**。

---

## 真實時序 (2026-08-05 14:16:20-25 UTC)

從 docker logs 抽出的時序:

| 時間 | 事件 |
|------|------|
| 14:16:20.488 | `channel_health_finmind` (1h) + `auto_quote_backfill` (24h) background task 註冊 |
| 14:16:20.501 | `channel_health_finmind` task started |
| 14:16:20.522 | `auto_cycle_update` task started |
| 14:16:20.833 | 第一個 channel_health_summary (推估 internal sub-task 触發) |
| 14:16:22.349 | **第一個 402** (TaiwanStockPrice data_id=6164) |
| 14:16:22-25.943 | 87 個 402 log 行 (TaiwanStockPrice 跟 TaiwanStockMonthRevenue) |
| 14:16:25.550 | 第一個 `revenue_fetch_failed` (2383.TW, kind=no_data) |
| 14:16:25.943 | 11 個 industry 全部 `industry_aggregate_failed` |

**8/5 02:16:20 UTC 跟 08:16:20 UTC 之前 24h 內 0 個 402**。**只在 14:16:20-25 5 秒內爆發 87 個 402**。

### 對比 02:16:20 跟 08:16:20 UTC 兩輪

- 02:16:20 UTC 8/5 (= 10:16 local) — `auto_cycle_update` 跑過, 沒 402
- 08:16:20 UTC 8/5 (= 16:16 local) — `auto_cycle_update` 跑過, 沒 402
- 14:16:20 UTC 8/5 (= 22:16 local) — **撞 402** (本檔分析)
- 20:16:20 UTC 8/5 (= 04:16 local 8/6) — **0 個 402, 但仍 fail 11 industry (kind=no_data)** — finding 4 + P1.10 實證

**14:16:20 唯一不同**:`auto_quote_backfill` 24h 排程**首次重 backfill** (上次跑是 8/4 14:16:20)。**3 個 task 同步觸發搶 token 是撞牆的引爆點**。

**但 (reviewer finding 4 + 5.1.1)「3 task 同步觸發」不是唯一根因**: 20:16:20 與 02:16:20 兩輪無 402 仍 fail 11 — 持續性根因是本地 rate limiter 5s ctx vs 6s token 錯配 (詳見 §5.1.1)。

---

## 1. server-side 402 catch chain (todo 3.3)

### 1.1 HTTP response 處理 (finmind_client.go:215-227)

```go
if resp.StatusCode != http.StatusOK {
    bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
    bodyStr := strings.TrimSpace(string(bodyBytes))
    logging.Warn("finmind", "fetch_non_2xx", "status", resp.StatusCode, "body", bodyStr, ...)
    return nil, fmt.Errorf("finmind: status %d, body: %s", resp.StatusCode, bodyStr)
}
```

**402 訊息路徑**: server 回 `{"msg":"Requests reach the upper limit. https://finmindtrade.com/","status":402}` → `fetchDataset` 印 `fetch_non_2xx` warning → return `fmt.Errorf("finmind: status 402, body: Requests reach the upper limit...")` 

**注意`: `fmt.Errorf` 沒 wrap `ErrQuotaExhausted` sentinel** — 這跟本地 quota tracker 撞 14400 的 `ErrQuotaExhausted` 是**兩個不同的 error**。

### 1.2 三層 quota 錯誤混淆

| 錯誤來源 | sentinel | 觸發條件 |
|----------|----------|----------|
| `c.rateLimiter.Wait(ctx)` 失敗 | `ErrRateLimited` | 本地 6s pacing 等不到 |
| `c.quotaTracker.AllowCall()` 撞 14400 | `ErrQuotaExhausted` | atlas-go 內 daily counter 撞上限 |
| HTTP 402 server 回 | `fmt.Errorf("finmind: status 402, body: ...")` | 實際 server-side FinMind 限額 |

**`finmindDailyLimit = 14400`** 寫死 (finmind_client.go:41) — **跟 FinMind server 真正 600/hr / 14400/day 完全一致,但 bucket 沒對應 fine-grain 600/hr**。**這跟 FinMind 一天 quota 撞牆有 24 倍 cushion**,本地 counter 永遠不會撞牆。**只有 server 會撞 402**。

### 1.3 classifyFinMindError 對 402 走到哪

(8/5 之後 PR-#1462 hotfix 修後, `internal/industry/data_aggregator.go:370-394`):

```go
if errors.Is(err, marketdata.ErrQuotaExhausted) { return "quota" }
if errors.Is(err, marketdata.ErrRateLimited) { return "rate_limited" }
switch {
case strings.Contains(msg, "no month revenue data"),
    strings.Contains(msg, "no data"),
    strings.Contains(msg, "no valid data"):
    return "no_data"
case strings.Contains(msg, "cannot parse"), strings.Contains(msg, "decode"): return "parse_error"
case strings.Contains(msg, "http request"), ..., strings.Contains(msg, "no such host"): return "transport"
default: return "unknown"
}
```

**402 訊息走 default → "unknown"**。**但 production 14:16:25 看到的 `kind=no_data` 來源不是這條** — 是 `industry_aggregate_failed` 層 (data_aggregator.go:145) 內 `kind=no_data` (revCount==0 && profitCount==0 觸發),**跟 classifyFinMindError 是不同層**。

**結論**: **PR-#1462 修的 metric label 「no_data」是 industry aggregate 層的「沒資料」**,**不是 402 撞牆**。**402 撞牆對 Prometheus metric 仍不可見**。

### 1.4 402 訊息怎麼 emit 到 log

`fetchDataset` 內 `logging.Warn("finmind", "fetch_non_2xx", "status", 402, "body", "Requests reach the upper limit...")` 印 warning,不進 metric pipeline。

但 **`data_aggregator.go:121-126` `revenue_fetch_failed` warning 印的不是 raw 402** — 它印的是 `fetchRevenueYoY` 內 fallback 迴圈吞掉底層 error 後合成的 `"finmind revenue: no data for %s in last %d months"` (data_aggregator.go:226)。402 原始 error 在 `fetchRevenueYoY` 的 `if err != nil { month--; continue }` 處被丟棄,**從未到達 `revenue_fetch_failed`**。

**修正 (reviewer finding 1)**: 原稿寫「revenue_fetch_failed 印 raw err.Error() = 完整 402 訊息 string」是**錯誤描述** — 實證 (8/5 20:16:20 與 8/6 02:16:20 兩輪 production log) 顯示 `revenue_fetch_failed` 的 err 恆為 `"finmind revenue: no data for X in last 3 months"` 合成字串,**即使底層是 rate limit 或 402**。

**真實路徑**: 402 → `fetchRevenueYoY` fallback 吞掉 → 合成「no data」error → `revenue_fetch_failed` 印合成字串 → `AggregateIndustry` revCount==0 → `recordIndustryFailure` 收「no valid data for industry」→ `classifyFinMindError` 分類 `no_data`。**402 與 rate limit 對 metric 完全不可見,且原始 error 在兩層被吞**。

---

## 2. 14 vs 11 industries 差異 (todo 3.1)

### 2.1 「14」哪來的

`docs/investigations/2026-08-05-auto-cycle-update-quota-misconception.md` line 64 寫:
> `auto_cycle_update` 自身用量估算: 14 industries × 平均 2.5 stocks × (revenue 3m×2 + profit 3q×2 = 12 calls) = 420 calls per run

**這個 14 是 PR-#1461 dispatch 階段的估算**,**沒驗證過**。

### 2.2 真實數字

**修正 (reviewer finding 3)**: 原稿寫「12 個 L1 industries」是**錯誤**。實際 `configs/parameters.json` `industry.classification_tree.value.segments` 是 **29 個 segments = 16 L1 + 13 L2**:

16 個 L1 industries:
- **11 個有 representative_stocks**: ai_supply_chain / consumer / electronics / energy / financials / industrial / leo_satellite / mining / robotics / semiconductor / shipping
- **5 個無 stocks (TotalStocks=0)**: etf_rotation / defensive / high_dividend / small_cap / tech

13 個 L2 (有 parent, 全部無 stocks): pcb / thermal (→electronics), foundry / server_assembly / cooling (→semiconductor), precious_metals_recycling / copper_industry / rare_earth_specialty / metal_processing (→mining), satellite_rf_components / satellite_pcb / ground_equipment / laser_communication (→leo_satellite)

Production 14:16:25 UTC 11 個有 stocks 的 L1 全部 fail (5 個無 stocks + 13 個 L2 不參與 auto_cycle_update)。

**真實 fail 數 = 11**,**不是 14**。

### 2.3 為什麼這是個歷史污染

「14」這個數字從 PR-#1461 開始散播:
- PR-#1461 investigation doc (line 64)
- PR-#1462 hotfix commit message 引用 14
- 我的 hotfix 驗收報告也寫 14

**沒人回去從 production log 數真實 unique industry count**。**這是個「記憶污染」**。

### 2.4 給下一個 agent 的警示

**禁止** 僅憑 PR-#1461 / #1462 任何 commit message 內的「14」來推導 fail industry 數。**驗證方法**: `docker logs atlas-go --since 1h | grep "industry_aggregate_failed" | awk -F'industry=' '{print $2}' | awk '{print $1}' | sort -u` 應得 11 個 unique industry。**數字校驗**: 16 L1 中 11 個有 stocks (auto_cycle_update 只跑有 stocks 的 L1), 5 個無 stocks (etf_rotation / defensive / high_dividend / small_cap / tech) 不參與。

---

## 3. PR history 完整路徑 (todo 3.2)

50+ 個 FinMind 相關 PR (`gh pr list --search "finmind" --state all --limit 50` — 2026-08-06 實查 50 個被 limit 截斷,表格列直接相關者;reviewer finding 8 補 #1290/#1094/#955):

| # | PR | 標題 | 修什麼 / 沒修什麼 |
|---|-----|------|-------------------|
| #1463 | OPEN | test(industry): live FinMind symbol coverage 驗證 | 改盤查工具, 沒修根因 |
| #1462 | MERGED | fix(industry): classifyFinMindError 識別「no valid data」 | 修 metric label, 沒修 402 撞牆 |
| #1461 | MERGED | feat(industry): auto_cycle_update 失敗 metric + validator | 修 metric 框架, 沒修 402 撞牆 |
| #1456 | MERGED | fix(marketdata): FinMind endDate 從 last day of month 算 | 修 30-day 月份 endDate 400, **沒修 402** |
| #1452 | MERGED | fix(marketdata): capture FinMind API error body in channel health | 修 402 訊息 visibility, **沒修 402 撞牆原因** |
| #1451 | MERGED | fix(apigateway,marketdata,monitoring): unified quota management | 自稱 unified quota, **沒解 server-side 402** |
| #1454 | MERGED | feat(monitoring): known-issue badge for long-stale channels | 改 dashboard surfacing, 沒修 FinMind |
| #1457 | MERGED | fix(monitoring): clear stale crossmarket degraded | 沒修 FinMind |
| #1458 | MERGED | fix(monitoring): known-issue badge for dead taifex-daily | 沒修 FinMind |
| #1338 | MERGED | refactor(backfill): remove rogue FinMind backfill CLIs | 移除 rogue CLIs, **沒解 quota 機制問題** |
| #1344 | MERGED | feat(route-e): gap-detection metrics | 改 metrics, 沒修 FinMind |
| #1290 | CLOSED | feat(channel): register tw_vol — TAIEX 20d volatility | 提 FinMind 為 quote 備援原則, 沒修 FinMind (reviewer finding 8 補) |
| #1225 | MERGED | fix: background task health + BK-12 BK-14 | 修排程器, 沒修 FinMind |
| #1114 | MERGED | fix(mcp): prefers real engine over synthesized fallback | 沒修 FinMind |
| #1094 | MERGED | refactor(docs): normalize 11 docs filenames (含 finmind_alternatives) | 文件重命名, 沒修 (reviewer finding 8 補) |
| #1061 | MERGED | fix(cron-quote-backfill): shared rate-limit | 改 cron rate-limit, 沒修 402 撞牆 |
| #1059 | MERGED | chore(deploy): cron-quote-backfill docker service | 部署, 沒修 |
| #1051 | MERGED | feat(cron): cron-quote-backfill binary | 加 binary, 沒修 |
| #955 | MERGED | refactor(constants): add paths.go + urls.go — 集中 FinMind URL | 消除重複 const, 沒修 quota (reviewer finding 8 補) |
| #678 | MERGED | fix(marketdata): P1 FinMind trading-day guard | 改週末 guard, 沒修 402 |
| #654 | MERGED | refactor(cmd/atlas): extract data-sync tasks | 沒修 FinMind |
| #630 | MERGED | feat(monitoring): SmartUniverseBuilder + finmind infrastructure | 加基礎, 沒修 402 |
| #561 | MERGED | chore(workspace-cleanup): ghost-agent monitor + finmind calibration | 沒修 |

### 3.1 修補鏈 pattern (真實 20+ 次 loop)

```
#1451 (8/4 16:32) "unified quota"
    → 8/5 02:16:20 沒撞 402
    → 8/5 08:16:20 沒撞 402
    → 8/5 14:16:20 ★ 撞 402 ★
        → #1456 (8/5 03:44) 修 endDate 31 — 沒修 402
        → #1461 (8/5 ~12:00) 加 metric 框架 — 沒修 402
        → #1462 (8/5 ~14:00) 修 classifyFinMindError — 沒修 402
        → #1463 (8/5 22:41) 加 coverage validator — 沒修 402
```

**#1451 自稱「unified quota」但 8/5 14:16:20 還撞 402** — **這是 20+ 次 loop 的真實 anchor**。

### 3.2 每個 PR 修了什麼 / 沒修什麼

| PR | 修了 | 沒修 |
|----|------|------|
| #1451 | atlas-go 內部 quota tracker unified | 沒對應 server 600/hr 細粒度 |
| #1452 | 402 body 訊息進入 channel health | 沒分類 / 沒上 metric |
| #1456 | endDate 計算用 lastDayOfMonth | 沒修 quota 撞牆 |
| #1461 | metric 框架 + symbol coverage validator | 沒修 402 撞牆 |
| #1462 | classifyFinMindError 加「no valid data」 | 沒加 402 對應 "quota" |
| #1463 | live test 工具 + coverage 100% 確認 | 沒查 402 真實路徑 |

---

## 4. 跨 channel 互動 (todo 3.4)

### 4.1 FinMind 11 個 caller 完整盤查

直接打 FinMindClient:
1. `internal/industry/data_aggregator.go` — 月營收 / 財報季度 (auto_cycle_update)
2. `internal/marketdata/finmind_dividend_provider.go` — 配息 (ad-hoc, 有 cache)
3. `internal/marketdata/hybrid_provider.go` — Hybrid (Fugle 主, FinMind 備)
4. `internal/marketdata/odm_provider.go` — ODM 個股
5. `internal/marketdata/finmind_intraday.go` — 5秒指數
6. `internal/marketdata/symbol_industry_mapper.go` — 收錄清單 (TaiwanStockInfo)
7. `internal/marketdata/tsmc_revenue_provider.go` — TSMC 月營收
8. `internal/monitoring/dashboard_api.go` — 健康檢查 (channel_health_finmind)
9. `internal/monitoring/quote_backfill_task.go` — 報價回補 (auto_quote_backfill)

透過 gateway 走 FinMind:
10. `internal/apigateway/adapter_finmind.go` — "finmind" channel
11. `internal/apigateway/adapter_tsmc_revenue.go` — "tsmc_revenue" channel (雙源: FinMind + TWSE)

### 4.2 撞 402 那 5 秒內的 3 個 task

| task | 排程 | 14:16:20 attempt |
|------|------|------------------|
| `auto_cycle_update` | 6h | 14:16:20.522 started (`data_aggregator.go`) |
| `auto_quote_backfill` | 24h | 14:16:20.488 registered (**24h 整點首次重 backfill**) |
| `channel_health_finmind` | 1h | 14:16:20.501 started |

**3 個 task 同時觸發 `*FinMindClient.GetStockInfo/GetMonthRevenue/etc.`** → **同一個 shared token** → **撞牆**。

### 4.3 兩層 rate limiter 沒互鎖

- `c.rateLimiter` (finmind_client.go 內部, 6s/call, burst 60) — 跟 server 600/hr 對應不上
- gateway `rate.NewLimiter(FinMindFreeRate, FinMindFreeBurst)` (limits.go:122) — 在 `apigateway/adapter_finmind.go` 註冊

**這兩層 rate limiter 沒互鎖**。**在哪個層 abort 不知道**。

### 4.4 跟 Fugle / TWSE 互動

- HybridProvider 先試 Fugle, fail 走 TWSE, 不 fallback FinMind (cache 內)
- tsmc_revenue 是雙源 (FinMind + TWSE), 14:21:16 撞 402 那次是 FinMind 撞, TWSE 接手
- 也就是 **大部份 routine call 不依賴 FinMind**, FinMind 是「備援」 — 但 14:16:20-25 撞牆影響 `auto_cycle_update` 跟 `auto_quote_backfill` 這兩條**主路徑**

---

## 5. 開放問題 + reviewer 待辦 (todo 3.5)

### 5.1 還沒驗證的問題

- [x] **02:16:20 UTC 8/6 (= 10:16 local) auto_cycle_update 那一輪是否恢復?** — **沒有 402,但仍然 fail 11 industry (kind=no_data)**。P1.10 實證 (2026-08-06 05:10 UTC 檢查): 02:16:20.667-.669 全部 symbol `revenue_fetch_failed err="finmind revenue: no data for X in last 3 months"`,0 個 `fetch_non_2xx status=402`,quota counter 3080/14400 未撞牆。**證實「402 撞牆不是 fail 的必要條件」** — 02:16:20 那輪的 fail 是 **rate limiter Wait 本地失敗** (tsmc_revenue_provider 02:15:37 同輪 log `fetch_failed_falling_back_to_cache err="finmind: rate limit wait: rate limited"` 佐證)。
- [ ] `*FinMindClient.GetStockInfo` 是否還在 production 跑? (理論上 auto_quote_backfill 24h 撞牆後不再跑)
- [ ] 8/5 整天 `finmind_daily_quota.json` 累積真實數字? (docker 內 json 顯示 153 跟 14400 矛盾, 推測 concurrent write race)
- [ ] `DailyQuotaTracker.AllowCall()` 跟 `save()` 之間是否有 lock? (line 47-67 看起來用 `t.mu.Lock()` 但 save 跟 AllowCall 是同一個 lock 內的整段, 應該沒 race。但若 t.callsToday 過期重置跟 save 跨 process 則有 race)
- [ ] `finmindDailyLimit` 14400 跟 server 600/hr 為什麼會對應一致? (其實是巧合, 14400/day = 600/hr × 24h, 但 server 應該不是這麼算)

### 5.1.1 P1.10 驗證發現的更深根因 (2026-08-06 05:10 UTC)

**02:16:20 UTC 8/6 那輪的 fail 根因是 `rateLimiter.Wait(ctx)` 本地失敗,不是 server 402**:

- `finmind_client.go:131`: `rate.NewLimiter(rate.Every(time.Hour/finmindRateLimit), finmindBurst)` = **600/hr = 每 6s 1 token**, burst 60
- `data_aggregator.go:184`: `fetchRevenueYoY` 用 `context.WithTimeout(ctx, 5*time.Second)`
- **5s ctx timeout < 6s token interval** → burst 耗盡後 (`channel_health_finmind` 1h + `tsmc_revenue_provider` + `auto_quote_backfill` 24h 同步搶 token),`rateLimiter.Wait(ctx)` 在 5s 內等不到 token → 回 `ErrRateLimited`
- `fetchRevenueYoY` 把這個 error 當作「該月沒資料」**吞掉 fallback 到下個月** (data_aggregator.go:191-197 `if err != nil { month--; continue }`) → 3 個月全被 rate limit → 合成 `"no data in last 3 months"`
- **result**: metric kind=no_data,完全看不出是 rate limit。**rate limiter 本地失敗與 server 402 在 metric 層無法區分**。

**這改寫本 doc 的核心結論**: 14:16:20 那輪是 server 402 撞牆 (有 87 個 402 log 為證);但 20:16:20 與 02:16:20 兩輪 (0 個 402) 的 fail 是 **本地 rate limiter 設計缺陷** — 6s token interval 對 5s ctx timeout 的錯配,加上 error 被兩層吞掉。**「3 task 同步觸發」只是加速 token 耗盡的催化劑,不是唯一根因**。

### 5.2 候選 hotfix (待 Phase 4 完整 plan)

> **reviewer finding 5 (2026-08-06)**: Phase 4/5 plan 檔案位於 `/tmp/phase4-hotfix-plan-2026-08-06.md` 與 `/tmp/phase5-index-mockup-2026-08-06.md` — **ephemeral staging, 不進 source tree**。本 doc 不內嵌其內容, 僅列候選方向; 實際方案以 repo 內 design doc 為準。

(這段**不**給 PR body, 只列候選方向)

1. **修 classifyFinMindError 對 402 識別** — 加 `strings.Contains(msg, "Requests reach the upper limit")` 進入 `quota` 分類。**但 (reviewer finding 2) 這單獨無效**: `recordIndustryFailure` (data_aggregator.go:170) 收到的 err 是 `AggregateIndustry` 合成的 `"data_aggregator: no valid data for industry X"` (data_aggregator.go:143),不是底層 402/rate-limit error — **原始 error 在 fetchRevenueYoY 就被吞**。需先修 error 傳遞 (見 5.1.1)。
2. **修 fetchRevenueYoY / fetchProfitYoY 對 402/rate-limit 早 abort** — 撞 402 或 `ErrRateLimited` 不再 fallback 3 個月,直接透傳 error (不吞)。**這是 finding 2 的前置**。
3. **修 fetchDataset 內 402 上 metric** — 新增 `atlas_finmind_quota_402_total` counter
4. **修 DailyQuotaTracker save() 加 atomic rename** — 避免 concurrent write race
5. **修排程器錯開 auto_cycle_update / auto_quote_backfill / channel_health_finmind 同時 trigger** — 14:16:20 巧合同步應避免;**且 5.1.1 證明非唯一根因**
6. **修 finmindDailyLimit 細粒度** — 改成 600/hr 跟 server 對應, 或加 token-bucket 600/hr 內部 token bucket
7. **[新] 修 5s ctx vs 6s token 錯配** — `fetchRevenueYoY`/`fetchProfitYoY` 的 ctx timeout 需 ≥ rate limiter interval (或 rate limiter 改用較小粒度),否則 token 耗盡後 Wait 必然失敗 (5.1.1 根因)

### 5.3 給 reviewer (kaecer) 的問題

- 5.2 這 7 個 hotfix 方向, 哪幾個走 PR? 哪幾個單獨 PR?
- 5.1 5 個開放問題, 哪些需要先驗證才能動 5.2?
- 之前 PR-#1451 的「unified quota」PR body 寫了什麼? 為什麼沒對應 server 600/hr 細粒度?

---

## 6. 給下一個 agent 的警示

**禁止** 僅憑以下就推導 「auto_cycle_update 失敗」的原因:

| Surface evidence | 為何誤導 |
|---|---|
| `data_aggregator.go:115` `no valid data for industry` | 觸發條件是「每個 stock 都拿不到資料」, 沒區分 402 撞牆 vs 沒 publish vs **本地 rate limit** |
| `finmind_daily_quota.json` 顯示 1 call (= host stale json) | 這是 host 測試用的 stale 檔, 真的 counter 在 docker 內 |
| PR-#1462 commit message 提「14 個 industries」 | 14 是歷史污染, 真實 11 個 (16 L1 中 5 個無 stocks 不參與) |
| `fetchDataset` 印 `fetch_non_2xx status=402` | 402 沒上 metric, 從 metric 看不出有 402 撞牆 |
| **`revenue_fetch_failed err="no data in last 3 months"`** | **最誤導**: 這可能是 402、本地 rate limit、或真的沒 publish — 原始 error 在 `fetchRevenueYoY` fallback 被吞, 合成字串無法區分 (5.1.1 實證) |
| `auto_cycle_update` task **continues to run** | task 沒卡, 但每 6h fail, 持續累積 consecutive_failures (02:16/08:16 兩輪 0 個 402 仍 fail 11) |

**正確診斷流程**:
1. 看 `docker logs atlas-go --since 1h | grep "fetch_non_2xx status=402"` 確認 402 是否有 (0 個 ≠ 沒問題, 見 5.1.1 rate limit 路徑)
2. 看 `docker logs atlas-go --since 1h | grep "rate limit wait"` 確認是否本地 rate limiter 撞牆 (tsmc_revenue_provider 02:15:37 有前例)
3. 看 `*ErrQuotaExhausted` log 確認是否撞本地 14400
4. 從 `docker exec atlas-go cat /app/data/state/finmind_daily_quota.json` 確認真實 counter (非 host 檔)
5. 從 `cmd/atlas/main.go` 註冊 tasks 確認哪幾個 14:16:20-25 同步觸發
6. 對 production 14:16:20-25 5 秒 sort by timestamp, 確認 402 撞牆順序

---

## 7. 對 traps.md 的影響

**必須新增 (traps.md 目前無任何 FinMind trap — grep finmind 0 結果)**:
- 「FinMind server 402 ≠ `ErrQuotaExhausted`」 — 跟 line 18 (JSONL ledger mutex) 同 mental model
- 「rate limiter 本地失敗會被 fallback 吞掉」 — 5.1.1 根因: 5s ctx < 6s token, error 兩層被吞, metric 顯示 no_data (跟 line 67 Dockerfile healthcheck 同「避免 silent failure」主題)
- 「排程器 24h 整點多 task 同步觸發」 — 跟 line 41 (BackgroundTaskManager 唯一註冊) 互補

**現有 traps.md 與本檔的關聯 (reviewer finding 7 修正)**:
- line 18 (JSONL ledger append mutex) — 補: DailyQuotaTracker 雖然有 lock, 但跨 process 寫沒 lock
- line 41 (BackgroundTaskManager) — 補: 子任務 interval 對齊時, burst 會撞牆
- **原稿寫「現有 traps.md 引用本檔」是錯誤** — traps.md 目前**沒有**引用本檔, 需要**新增** FinMind trap 群組而非「引用現有」

**本檔應該**:
- 加連結 `> 2026-08-06-finmind-quota-collision.md` 在 `traps.md` 開頭索引
- traps.md 內 cross-ref 用 anchor

---

## 8. 跟 2026-08-05 doc 的關係

`2026-08-05-auto-cycle-update-quota-misconception.md` 推翻「quota 結構性問題」假設, 提出「個別 symbol 沒資料」為更真實根因。

**本檔進一步說明**:
- 14:16:20-25 真正撞牆是 **server-side 402** (不是 quota 結構性)
- 14:16:20 之前 02:16 + 08:16 兩輪沒撞 — 14:16 唯一不同是 `auto_quote_backfill` 24h 整點 first run
- **但 20:16:20 (8/5) 與 02:16:20 (8/6) 兩輪 0 個 402 仍 fail 11** (P1.10 實證) — fail 的**持續性根因是本地 rate limiter 5s ctx vs 6s token 錯配**, 402 只是 14:16 那一輪的疊加因素
- 「個別 symbol 沒資料」這條是次要問題, 真實主要問題是 402 撞牆 + rate limiter 錯配雙軌

**任何對「auto_cycle_update 失敗」做診斷的 agent 必須讀兩個 doc**:
1. `2026-08-05-auto-cycle-update-quota-misconception.md` — 學「不要僅憑 doc 內所有 surface evidence 推導結論」
2. `2026-08-06-finmind-quota-collision.md` (本檔) — 學「402 撞牆 + rate limiter 錯配是兩個獨立根因, 且 error 兩層被吞」

---

## 9. 寫作時間

盤查 + 寫: 2026-08-06 (~03:00-08:00 UTC)
驗證 02:16:20 UTC 8/6 那一輪: **✅ 已驗證 (2026-08-06 05:10 UTC)** — 0 個 402, quota 3080/14400, fail 11 industry, 根因 = rate limiter Wait 本地失敗 (詳見 5.1.1)
狀態: **✅ amended 2026-08-06 (8 個 reviewer findings 已逐項驗證修正 + P1.10 完成)**
