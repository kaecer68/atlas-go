# FinMind 402 / rate-limit Hotfix 方案 — Phase 4 HF-1 (設計文件)

> **狀態**: 方案草案,待審計 (issue #1465 強制 Gate: 方案 → 審計 → 才 PR 實作)
> **基於**: `docs/investigations/2026-08-06-finmind-quota-collision.md` (已 amended, 含 8 個 reviewer findings + P1.10 驗證)
> **日期**: 2026-08-06
> **範圍**: 讓 FinMind 402 / 本地 rate-limit 在 metric 可見,且不再誤報 `no_data`

---

## 0. 背景真相 (P1.10 實證, 2026-08-06 05:10 UTC)

`auto_cycle_update` 失敗有兩個獨立根因,metric 都顯示 `kind=no_data`,無法區分:

1. **server 402** (8/5 14:16:20 那輪): 3 task 同步 + auto_quote_backfill 首次 backfill 搶 token
2. **本地 rate limiter 錯配** (8/5 20:16:20 + 8/6 02:16:20 兩輪, 0 個 402):
   - `finmind_client.go:131` `rate.NewLimiter(rate.Every(6s), 60)` — 600/hr = 6s/token
   - `data_aggregator.go:184` `fetchRevenueYoY` ctx = **5s timeout**
   - **5s ctx < 6s token interval** → token 耗盡後 `rateLimiter.Wait(ctx)` 必失敗 (`ErrRateLimited`)
   - `fetchRevenueYoY` fallback 吞 error (data_aggregator.go:191-197 `if err != nil { month--; continue }`) → 3 個月全被 rate limit → 合成 `"no data in last 3 months"`
   - `AggregateIndustry` 收到合成 error → `recordIndustryFailure` 收到 `"no valid data for industry"` (data_aggregator.go:143) → `classifyFinMindError` → `no_data`

**error 傳遞鏈**: 底層 402/rate-limit → `fetchRevenueYoY` **吞掉** → 合成 no_data → `recordIndustryFailure` → `no_data`。**兩層被吞**。

---

## 1. 問題 (Problem)

| 症狀 | 根因 | 影響 |
|------|------|------|
| metric 恆為 `no_data` | error 兩層被吞 (fetchRevenueYoY + AggregateIndustry) | ops 無法區分 402 vs rate-limit vs 真沒資料 |
| rate limiter Wait 必失敗 | 5s ctx < 6s token interval | token 耗盡後整輪 fail 11 industry |
| 402 對 metric 不可見 | `fetchDataset` 只印 warning 不上 counter | 402 撞牆無監控告警 |

---

## 2. 方案 (Design)

### HF-1a: error 傳遞修復 (前置, finding 2)

**目標**: 底層 402 / `ErrRateLimited` 不再被 `fetchRevenueYoY` / `fetchProfitYoY` 吞掉。

**變更** `internal/industry/data_aggregator.go`:

```go
// fetchRevenueYoY / fetchProfitYoY 內:
current, err := a.finmind.GetMonthRevenue(ctx, symbol, year, month)
if err != nil {
    // 402 / rate-limit 直接透傳,不 fallback (避免 3 個月全被吞)
    if errors.Is(err, marketdata.ErrRateLimited) || isFinMind402(err) {
        return 0, err
    }
    // 其他錯誤 (no data 等) 維持原 fallback
    month--
    if month == 0 { month = 12; year-- }
    continue
}
```

**新增 helper** `isFinMind402(err error) bool` — 檢查 `"Requests reach the upper limit"` 字串 (server 402 body)。

### HF-1b: classifyFinMindError 加 402 識別 (HF-1 原案, 但需 HF-1a 才生效)

**變更** `internal/industry/data_aggregator.go` `classifyFinMindError`:

```go
if errors.Is(err, marketdata.ErrQuotaExhausted) { return "quota" }
if errors.Is(err, marketdata.ErrRateLimited)     { return "rate_limited" }
if isFinMind402(err)                              { return "quota" }  // server 402 = quota 類
// ... 其餘不變
```

**理由**: 402 是 server-side quota,與本地 `ErrQuotaExhausted` 同屬「quota 類」,alert 規則可共用。

### HF-1c: 5s ctx vs 6s token 錯配修復 (P1.10 根因)

**選項 A (建議)**: `fetchRevenueYoY` / `fetchProfitYoY` ctx timeout 從 5s 提到 **10s** (≥ 6s token interval + buffer)。
- 最小改動, 1 行
- 風險: 單 symbol 最壞等待時間增加 (但 rate limiter 是共享的, 實際等待被 token 釋放速率 bound)

**選項 B**: rate limiter 改細粒度 (如 600/hr 改 token bucket 每 6s 1 個 → 保持, 但 burst 提高)。
- 改動 rate 參數, 影響所有 FinMind caller
- 風險較大, 需單獨評估

**建議走 A** — 直接對應 P1.10 實證的根因, 影響面最小。

### HF-1d (optional): fetchDataset 402 上 metric

新增 `atlas_finmind_quota_402_total` counter — 讓 402 在 Prometheus 可見 (不只 warning log)。

**是否納入 HF-1**: 建議**不納入** (獨立 PR, 涉及 monitoring 框架), HF-1 聚焦 error 傳遞 + 分類正確。

---

## 3. 上游 / 下游 / 影響範圍

| 方向 | 內容 | 影響 |
|------|------|------|
| 上游 (callers of fetchRevenueYoY) | `AggregateIndustry` → `AggregateAllIndustries` | 行為改變: 402/rate-limit 時不再 fallback, 直接 fail industry — **但錯誤分類正確** |
| 上游 (callers of classifyFinMindError) | `recordIndustryFailure` → `monitoring.RecordDataAggregatorFailure` | metric kind 從 `no_data` 變為 `quota`/`rate_limited` — **dashboard 顯示改變** |
| 下游 (fetchRevenueYoY 內部) | fallback 迴圈 | 402/rate-limit 早退, 其他錯誤維持原行為 |
| 平行系統 | tsmc_revenue_provider (02:15:37 也撞 rate limit) | 不影響 (不同 code path), 但共享 rate limiter 受益於 10s ctx |

**相容性**: `classifyFinMindError` 新增分支向後相容 (新增 kind 值, 不改既有)。`isFinMind402` 為純函式。

---

## 4. 測試計畫

| 測試 | 位置 | 驗證 |
|------|------|------|
| `isFinMind402` 單元測試 | data_aggregator_test.go | 402 body 字串 → true; no-data 字串 → false |
| `classifyFinMindError` 402 分支 | coverage_push_test.go (既有表驅動) | 補 `"finmind: status 402, body: Requests reach the upper limit"` → `quota` |
| `fetchRevenueYoY` 透傳測試 | data_aggregator_test.go | mock GetMonthRevenue 回 `ErrRateLimited` → fetchRevenueYoY 回原 error (不合成 no_data) |
| ctx 10s 測試 | data_aggregator_test.go | 確認 timeout 常數改 10s (無行為測試, 常數驗證) |
| 既有測試回歸 | `go test ./internal/industry/ ./internal/marketdata/` | 無 breakage |

**production 驗證**: merge + rebuild 後等 1 輪 `auto_cycle_update` (6h), 確認:
- 若撞 rate limit: metric kind = `rate_limited` (不再 no_data)
- 若撞 402: metric kind = `quota`
- dashboard channel-health 顯示正確

---

## 5. 不做的事 (Non-goals)

- 不修 DailyQuotaTracker atomic rename (HF-4, 另案)
- 不修排程器錯開 3 task (HF-5, 另案)
- 不修 finmindDailyLimit 細粒度 (HF-6, 另案)
- 不加 402 Prometheus counter (HF-3, 另案)

---

## 6. 審計問題 (給 kaecer)

1. HF-1a + HF-1b + HF-1c 是否一個 PR? 還是拆開?
2. 402 歸類 `quota` (與 ErrQuotaExhausted 同類) 是否正確? 或該獨立 `server_quota` kind?
3. HF-1c 選項 A (10s ctx) 是否可接受? 或要選項 B (rate limiter 調整)?
4. HF-1d (402 metric) 是否要併入?

---

## 7. 參考

- `docs/investigations/2026-08-06-finmind-quota-collision.md` §5.1.1 (P1.10 根因)
- `docs/investigations/2026-08-06-industry-count-correction.md` (14/11/16)
- Issue #1465 (Phase 4 hotfix 候選)
