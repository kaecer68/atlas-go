# Quote Warmup 覆蓋率盤查（2026-07-24）

## 問題
1. Quote warmup 只存 46（實際 1）/132 筆
2. SMA50=2359.7 但只有 46 筆資料
3. Regime burn-in live pipeline 功能確認

## 根因

### Quote warmup 只存 1/132 筆

**檔案**：`internal/monitoring/dashboard_api.go:787`
```go
url := "https://api.fugle.tw/marketdata/v1.0/stock/historical/candles/2330?..."
```

`warmupQuotes()` 只 hardcode 抓 **2330.TW** 一支股票。全 universe 有 ~96 檔代表股（`DefaultRepresentativeStocks()`），加上 fundamentals.json 共約 132 檔。只有 1 檔有 quote 資料。

SQLite 驗證：
```
SELECT COUNT(DISTINCT symbol), COUNT(*) FROM quotes;
→ 1 symbol, 133 rows (all 2330.TW)
```

`cron-quote-backfill` 獨立 binary 存在但**未排程為背景任務**，需手動執行。

### SMA50=2359.7 但只有 46 筆

SMA50 計算（`internal/stocktools/handler.go:226-234`）：
```go
func sma(values []float64, n int) float64 {
    if len(values) < n { return 0 }  // 不足 n 筆回 0
    ...
}
```

**2330.TW 有 133 筆日線**，SMA50 可正常計算（133 ≥ 50）。SMA50=2359.7 是正確值。

若查詢其他無 quote 資料的股票，`sma(closes, 50)` 因 `len(closes) < 50` 會回傳 0。

「46 筆」可能來自：
- Fugle 免費 API 的回傳上限（實測是完整的 ~133 筆）
- 或是查了非 2330 的股票（0 筆資料）

### Regime burn-in live pipeline

**已確認正常**：
- SQLite `regime_history` 有從 6/16 起的每日連續資料
- `source` 從 `synthetic` → `snapshot_backfill` → `macro_ingest`（7/24 最新）
- macro_ingest 每 5 分鐘執行、成功寫入
- US market refresh 正常執行

Burn-in 剩餘天數會自然遞減（`system_get_maturity` → `days_until_calibrating`）。

## 修復

1. **擴充 `warmupQuotes()`**：從只抓 2330 → 抓全部 `DefaultRepresentativeStocks()` 的 ~96 檔代表股
2. 新增 `fetchFugleCandles()` helper：單檔 Fugle API 呼叫
3. Rate limit：每檔間隔 2 秒（~30 req/min，Fugle 免費 tier 上限）
4. Timeout：300 秒（96 * 2s ≈ 192s + buffer）
5. 失敗 graceful：跳過繼續下一檔

## 後續建議

- `cron-quote-backfill` 可排程為每日背景任務，補充新上市股及補缺
- Fugle API 有付費 tier 可加速（concurrency > 1）
- 可考慮用 TWSE OpenAPI 作為免費 fallback（但資料欄位不同）
