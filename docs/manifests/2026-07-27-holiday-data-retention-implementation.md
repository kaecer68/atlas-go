# Implementation Manifest: 假日資料保留 (方案 C)

> **Branch**: `fix/holiday-data-retention`
> **Audit**: `docs/manifests/2026-07-26-holiday-data-retention-audit.md`
> **Approach**: Option C — 提取共用 utility + 修復 us_yahoo + dram_spot_price
> **Created**: 2026-07-27
> **Status**: in-progress

---

## Phase 1 — 提取共用 `previousBusinessDay`

**現狀**: `previousBusinessDay` 出現在 3 處:
- `internal/apigateway/adapter_government_broker.go:106-115` (週末 only)
- `internal/marketdata/frankfurter_provider.go:141-145` (週末 only)
- `internal/industry/event_calendar.go:396-402` (`lastBusinessDay`, 月結算用)

**做法**: 提取為共用 utility → `internal/marketdata/calendar.go`

**介面**:
```go
// PreviousBusinessDay returns the most recent Mon-Fri date ≤ (now - daysBack).
// On weekends, rolls to the prior Friday.
func PreviousBusinessDay(now time.Time, daysBack int) time.Time

// PreviousTradingDay returns the most recent non-weekend date ≤ (now - daysBack).
// On weekends, rolls to the prior Friday.
// This is intentionally simple: TW/US holiday detection is an upstream concern
// (Yahoo Finance already returns the last close containing non-zero data).
func PreviousTradingDay(now time.Time, daysBack int) time.Time
```

**變更檔案**:
| File | Change |
|------|--------|
| `internal/marketdata/calendar.go` | **NEW** — 共用 utility |
| `internal/marketdata/calendar_test.go` | **NEW** — unit test |
| `internal/apigateway/adapter_government_broker.go` | 改用 `marketdata.PreviousBusinessDay` |
| `internal/marketdata/frankfurter_provider.go` | 改用 `marketdata.PreviousBusinessDay` |

**驗收**:
```
go test ./internal/marketdata/... -run TestPreviousBusinessDay -count=1
go test ./internal/apigateway/... -run TestPreviousTradingDay -count=1
go build ./...
```

---

## Phase 2 — 修正 `yahoo_macro_provider` 零值 fallback

**現狀**: `fetchIndicator()` 在 `latest == 0` 時直接 `return error`[yahoo_macro_provider.go:176-178]。
這是 DXY 在休市時顯示「資料獲取失敗」的根因: Yahoo 回傳 `closes: [0.0, ..., 0.0]` 但更早的歷史資料中有上一個交易日的有效收盤價。

**TAIEX 已運作的模式**[taiex_index_provider.go:67-78]:
```go
latest := closes[len(closes)-1]
if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
    return error
}
prev := closes[len(closes)-2] // 前一個非零值
```

**做法**: 零值時從 `closes` 陣列末尾往回找上一個非零值:
- 找到: 用該非零值替代 `latest`，`changePct` 改用該值 vs 前一值計算
- 找不到(全部為零): 才回 error

**實作位置**: `internal/marketdata/yahoo_macro_provider.go:167-178` (`fetchIndicator`)

**變更後邏輯**:
```go
if latest == 0 {
    // Walk backwards through closes to find the last valid (non-zero) price.
    // Yahoo Finance returns [0.0, ..., 0.0] during US market off-hours for
    // certain forex/commodity tickers (DX-Y.NYB, CL=F, etc.). The range=1mo
    // parameter ensures enough historical data to find the prior trading day's
    // close. Only reject when ALL closes in the array are zero or NaN.
    latest, prev = findLastValidClose(closes)
    if latest == 0 {
        return MacroDataPoint{}, fmt.Errorf("zero latest price for %s (all closes zero)", ticker)
    }
}

// findLastValidClose walks closes backwards to find the last non-zero,
// non-NaN value. Returns (0, 0) if none found.
func findLastValidClose(closes []float64) (latest, prev float64) {
    for i := len(closes) - 1; i >= 0; i-- {
        v := closes[i]
        if !math.IsNaN(v) && !math.IsInf(v, 0) && v != 0 {
            if latest == 0 {
                latest = v
            } else {
                prev = v
                return
            }
        }
    }
    return 0, 0
}
```

**變更檔案**: `internal/marketdata/yahoo_macro_provider.go`

**驗收**:
```
go test ./internal/marketdata/... -run TestFetchIndicator -count=1
go test ./internal/marketdata/... -run TestFindLastValidClose -count=1
```

---

## Phase 3 — 修正 `adapter_yahoo_macro` PartialSuccess 判斷

**現狀**: `Fetch()` 只用 `RecordedAt > 0` 判 partial success[adapter_yahoo_macro.go:30-41]。
`RecordedAt` 在 `FetchSnapshot()` 第一行就設定了[yahoo_macro_provider.go:64]，全失敗時仍 > 0。

**做法**: 改用 `snapshotHasAnySymbol(snap)` — 至少一個 macro 指標有有效 Symbol。

**實作位置**: `internal/apigateway/adapter_yahoo_macro.go:30-41`

```go
func (a *YahooMacroChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
    snap, err := a.provider.FetchSnapshot(ctx)
    if err != nil {
        if snap.RecordedAt > 0 && snapshotHasAnySymbol(snap) {
            logging.Warn("apigateway", "yahoo_macro_partial_fetch",
                "error", err.Error())
            // partial success: some indicators have data
        } else {
            return nil, fmt.Errorf("yahoo macro fetch: %w", err)
        }
    }
    ...
}

func snapshotHasAnySymbol(snap marketdata.MacroDataSnapshot) bool {
    return snap.US10Y.Symbol != "" || snap.DXY.Symbol != "" ||
        snap.VIX.Symbol != "" || snap.Oil.Symbol != "" ||
        snap.Gold.Symbol != "" || snap.USD_TWD.Symbol != "" ||
        snap.Silver.Symbol != "" || snap.Copper.Symbol != ""
}
```

**變更檔案**:
| File | Change |
|------|--------|
| `internal/apigateway/adapter_yahoo_macro.go` | 加 `snapshotHasAnySymbol`，改 `Fetch` 判斷 |
| `internal/apigateway/adapter_yahoo_macro_test.go` | 加 `TestFetch_AllIndicatorsFailed` 測試 |

**驗收**:
```
go test ./internal/apigateway/... -run TestYahooMacro -count=1
```

---

## Phase 4 — 套用相同模式至 `dram_spot_price_provider`

**做法**: 同 Phase 2，`fetchLatest()` 或 equivalent 中：
- 零值時 walk backwards through closes array
- 全零才回 error

**變更檔案**: `internal/marketdata/dram_spot_price_provider.go`

**驗收**:
```
go test ./internal/marketdata/... -run TestDRAM -count=1
```

---

## Phase 5 — 整合測試 + Deploy 驗證

**步驟**:
1. `make ci-gate` → 通過
2. `make rebuild-all` (fresh binaries)
3. 部署到容器
4. `curl /api/cross-market/status` → DXY 休市時顯示上個交易日收盤價,非 Symbol=''
5. `curl /api/dashboard/correlation-matrix` → labels 全中文
6. 瀏覽器驗證: crossmarket 頁面 DXY/US10Y 都顯示數值(休市時為上一個收盤價)

---

## Summary

| Phase | 改動檔案 | 行數 | 風險 |
|-------|---------|------|:---:|
| 1 | `calendar.go`(new) + 2 adapter 調整 | ~40 | 低 |
| 2 | `yahoo_macro_provider.go` | ~25 | 低 |
| 3 | `adapter_yahoo_macro.go` + test | ~20 | 低 |
| 4 | `dram_spot_price_provider.go` | ~15 | 低 |
| 5 | deploy + browser verify | 0 | 低 |
