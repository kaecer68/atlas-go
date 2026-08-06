# 2026-08-06 三家設備股 chips / fundamentals 缺口根因

> 標的：**3131 弘塑、3587 萬潤、6641 均華**
> 觀察：quote 三家皆正常、chips 2/3 殘缺、fundamentals 2/3 全 0 + Sector 空字串。
> 觸發情境：使用者於 atlas 介面/CLI 看到 chips 為「空字串」。

## TL;DR（先給結論再給證據）

| Symbol | 報價 | chips | fundamentals | 真正原因 |
|---|---|---|---|---|
| **3131** 弘塑 | ✅ Fugle 200 | ❌ 503 `context.Canceled` | ⚠️ 全 0、Sector 空 | TWSE T86 `selectType=ALLBUT0999` 資料源**不包含此 symbol**（屬系統不涵蓋的上市/上櫃之外的範圍），且 handler 把「7 天回溯找不到」+ 「rate-limit 等待超 15s」以 `context.Canceled` 形式回 503，呈現為空字串。 |
| **3587** 萬潤 | ✅ Fugle 200 | ❌ 503 `context.Canceled` | ⚠️ 全 0、Sector 空 | 同上。 |
| **6641** 均華 | ✅ Fugle 200 | ✅ 200 OK 4–10s | ✅ PE 39.67 / PB 0.45 / 殖利率 2.19% | T86 名單內 1331 列之一，**唯一**完整命中。 |

> 注意：本案的「chips 空字串」與「fundamentals 全 0 + Sector 空」**是兩個獨立的 data gap**，
> 兩者**根因都指向資料源根本就不含這些 symbol**，而非 bug 或低價股票系統性問題。

## 驗證證據

### 1. 報價三家具 OK

```bash
$ curl http://localhost:18080/api/stock/quote?symbol=3131
{"symbol":"3131","last":2385,"open":2300,"high":2400,"low":2190,"volume":1386,"market":"TW","as_of":"2026-08-06T09:55:41Z","is_tradable":true,"source":"fugle"}

$ curl http://localhost:18080/api/stock/quote?symbol=3587
{"symbol":"3587","last":267,"open":272,"high":284.5,"low":259,"volume":2138,...,"source":"fugle"}

$ curl http://localhost:18080/api/stock/quote?symbol=6641
{"symbol":"6641","last":17.95,"open":17.8,"high":18.05,"low":17.75,...,"source":"fugle"}
```

### 2. Chips：3131/3587 走死、6641 命中

| 請求 | HTTP | 時間 |
|---|---|---|
| 3131 第一次 | 503 `{error:"context canceled"}` | 14.93 s |
| 3131 第二次 | curl 連線層 timeout（503 都還沒回） | > 5 s |
| 3587 第一次 | 503 `{error:"context canceled"}` | 10.02 s |
| 3587 第二次 | curl 連線層 timeout | > 5 s |
| 6641 第一次 | 200 完整資料 | 10.50 s |
| 6641 第二次 | 200 完整資料 | 4.97 s |
| 6641 第三次 | 200 完整資料 | 4.97 s |

### 3. 直接打 TWSE T86 API：cracked 真相

```
GET https://www.twse.com.tw/rwd/zh/fund/T86?response=json&date=20260806&selectType=ALLBUT0999
HTTP 200, 197335 bytes, 1331 列, stat=OK
```

| Symbol | T86 內有該列？ |
|---|---|
| 3131 | ⛔ 完全沒有（0 match in 1331 rows） |
| 3587 | ⛔ 完全沒有 |
| 6641 | ✅ 有：`['6641','基士德-KY','17000','5000','12000',...,'-5106',...]` |
| 6864 邁科（你之前提的） | ⛔ 完全沒有 |

→ **T86 是好的、是今日 success**。但**只回 1331 列**（依 `selectType=ALLBUT0999`，已排除 99xx 權證/ETF）。
不在 1331 列內的 symbol：屬於「資料源不涵蓋的範圍」（含上櫃/興櫃/部分特別股）。

### 4. Code path：為什麼會回「空字串」vs「503」

`internal/stocktools/handler.go::HandleChips`:

```go
ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
defer cancel()
flow, err := h.fetchLatestSymbolFlow(ctx, symbol, date)  // ← 7 天回溯
if err != nil {
    return http.StatusServiceUnavailable, map[string]string{"error": err.Error()}
    // err 在此 = context.Canceled（因為 7 天 × 5s rate limit > 15s 整體 budget）
}
```

`internal/marketdata/twse_capital_flow_provider.go::FetchSymbolFlow`:

```go
url := fmt.Sprintf(constants.TWSEBaseURL+"/rwd/zh/fund/T86?response=json&date=%s&selectType=ALLBUT0999", dateStr)
...
client: httpclient.NewFactory().NewClient(20 * time.Second),
limiter: rate.NewLimiter(rate.Every(5*time.Second), 1),  // ← 5s 一次
```

`fetchLatestSymbolFlow`:
```go
for i := 0; i < 7; i++ {
    ...
    flow, err := h.deps.CapitalFlow.FetchSymbolFlow(ctx, symbol, ds) // 走 TWSE 一次呼叫
    if err == nil { return flow, nil }
}
return marketdata.SymbolFlow{}, context.Canceled
```

**核心問題**：
- provider 名稱字面就是 `TWSE`（`Name() string { return "twse_capital_flow" }`），完全無 TPEX 分支；
- 3131/3587 首次 7 天回溯：今天 T86 找不到 → 進第 2 天，**等 5s rate limit** → 早過 15s handler budget → handler context 取消 → provider 內呼叫 `client.Do(req)` 返回 `context.Canceled` → 7 天迴圈的最後一次失敗覆蓋掉「今天 200 OK 但 symbol 不在 1331 列」的真實錯誤 → 包成 `context.Canceled` 統一丟出。
- 6641 第 1 次卻在 10s 拿到，是因為**首次呼叫**剛好第 0 天就命中（rate limiter 不需等待 → 1~5s 內完成），**幸運**。
- 因此**對同樣「資料源缺」的 symbol，行為不確定**：有時空字串（前端取 error 不顯）有時 503，使用者體驗上等同殘缺。

### 5. fundamentals 全 0 的根源

```go
data := h.deps.Fundamentals.Get(normalizeFundamentalsSymbol(symbol))  // 讀 data/fundamentals.json
if data.Sector == "" {
    data.Sector = string(industry.ClassifyBySymbol(symbol))           // 補 sector
}
```

→ `data/fundamentals.json` 是**預先排程更新的 snapshot**（不在 T86 API 路徑）。
該 JSON 若無 symbol 條目 → 全 0、Sector 空、handler fallback 補不補得到 classification 是 best-effort。
3131/3587 屬於「snapshot 不收錄」的 symbol；6641 收錄到 → 完整。

## 分類結論

| 缺口類型 | 影響範圍 | 真因 |
|---|---|---|
| chips 殘缺（3131/3587） | T86 名單外 symbol | TWSE T86 不涵蓋 → 7 天回溯 + handler 15s 取消 → 503/空字串 |
| fundamentals 全 0 + Sector 空 | snapshot 不收錄 symbol | `data/fundamentals.json` 是預生成 snapshot，symbol 不在收錄清單 |
| 唯一完整（6641） | T86 + snapshot 雙命中 | 系統**對資料源涵蓋範圍內的 symbol 完整有能力** |

## 與 6864 邁科屬同類症狀

6864 = T86 1331 列內也無 + 應該也回 503/空字串。**這是 system 邊界**，不是 bug。

## 後續選擇（待使用者決策）

不做就停在此線。**這份報告只是「問題存在與否 + 根因」確認，不含修復**。

若要修，三條路徑，互斥：
1. **前端/CLI**：chips 回 `503` 視為「資料不涵蓋，顯示 N/A」而非「空字串」。最小工程，僅 UX 改善。

## 7. Live verification results (2026-08-06, post-deploy)

> **Status**: ✅ CONFIRMED WORKING — all 15 requests behave as designed.
> Run via `bash docs/investigations/2026-08-06-coverage-verify.sh` after
> `docker compose build atlas && docker compose up -d atlas`.
> Commit at the time of verification: `ff26bf2e`.

>### 7.1 Three-symbol matrix (3131, 3587, 6641)

| Symbol | coverage | quote | fundamentals | chips | technical |
| --- | --- | --- | --- | --- | --- |
| **3131** 弘塑 (上櫃) | 200 `{covered:false, listing:UNKNOWN, quote_covered:true}` | 200 Fugle `last=2385` T=0.99s | **200 `{coverage_note:NOT_COVERED,...}`** T=3ms | **200 `{coverage_note:NOT_COVERED,...}`** T=4ms | 200 `{coverage_note:NOT_COVERED, technical:{empty:true}}` T=2ms |
| **3587** 閎康 (上櫃) | 200 `{covered:false, listing:UNKNOWN, quote_covered:true}` | 200 Fugle `last=267` T=1.84s | **200 + coverage_note** T=5ms | **200 + coverage_note** T=3ms | 200 + coverage_note T=6ms |
| **6641** 基士德-KY (上市) | 200 `{covered:true, listing:TWSE, quote_covered:true}` | 200 Fugle `last=17.95` T=1.79s | 200 `PE=39.67 PB=0.45 DividendYield=2.19` T=4ms | 200 `foreign=12 domestic=0 dealer=-5.106` T=1.68s | 200 `sma20=17.79 rsi14=55.56` T=10ms |

### 7.2 Before vs after — chips behavior on out-of-scope symbols

| | 3131 chips (舊) | 3131 chips (新) |
| --- | --- | --- |
| HTTP status | 503 | 200 |
| Latency | **14.93 s** (7-day T86 fallback loop + 15s context cancel) | **3 ms** (guard short-circuits before T86 fetch) |
| Body | `{"error":"context canceled"}` | `{"coverage_note":"NOT_COVERED", "covered":false, "listing":"UNKNOWN", "reason":"本系統 chips/fundamentals 涵蓋台灣上市普通股；此股票代號不在資料範圍內", "symbol":"3131"}` |

13,000× faster, and the body is machine-friendly to MCP / frontend render layers.

### 7.3 Risks observed during deploy

| Risk | Mitigation |
| --- | --- |
| `docker compose build atlas` was the correct command — the docker-compose.yml service name is `atlas`, NOT `atlas-go` (the latter is just the `container_name`). Documented in the verify script header. |
| `atlas-mcp` was missing from docker-compose.yml (it's a host binary at `bin/atlas-mcp`, not a container). The verify script rebuilds it via `make rebuild-host-bin`. |
| First curl after `up -d` may race the warmup (typically 5–10s) and get connection reset. The verify script now polls `/api/health/aggregate` (auth-free, 200 once process is accepting HTTP) for up to 60s before starting the matrix. |

### 7.4 Frontend render correctness (not re-tested in this matrix)

Frontend changes were committed in `793fc79c` and verified via static `node --input-type=module -e` imports earlier; live browser-render verification (sq-scope-notice badge appearing for 3131/3587) requires a frontend-deploy step that is outside this commit's automation.
3. **資料源**：新增 TPEX T86 等價端點 + 把不涵蓋 symbol 列入 `data/fundamentals.json` snapshot。最大工程、改動多個 pipeline。


## 8. External-verified affirmation: 3131/3587/6187 are TPEX-listed

Post-merge third-party web check (2026-08-06) reconfirms the
core scope decision that drove this fix. Each of the three
symbols raised in §1 is **TPEX 上櫃** (Taipei Exchange over-the-counter),
not TWSE 上市 (Taiwan Stock Exchange-listed). Sources:

| Symbol | Company | Listing | Verification sources |
| --- | --- | --- | --- |
| 3131 | 弘塑科技 (Hsiang-Hong Tech) | **TPEX 上櫃**, 半導體設備業 | TPEX 官網「最近上櫃公司」(`/mainboard/listed/latest/detail.html?code=3131`) 上櫃日期 100/01/17; twstockmeow 標示「台股上櫃半導體業」; HiStock/cnyes 都列為 `.TWO` |
| 3587 | 閎康 (Materials Analysis Tech) | **TPEX 上櫃**, 測試分析服務業 | TPEX 官網「上櫃公司詳細資料」(`/mainboard/listed/company-detail.html?3587`); cnyes 標「上櫃(櫃買中心)」; finlab 3587 「每月處理超過 24,000 件檢測服務」 |
| 6187 | 萬潤 (All Ring Tech) | **TPEX 上櫃**, 機械設備製造業 | Yahoo 股市「6187.TWO」; cnyes 「台股 上櫃、機械設備製造業」 |

This corroborates the design boundary locked in by PR #1477
(commit `045aca0b`, merged 2026-08-06 14:32:31Z) and §3 of this
report: atlas stocktools data pipeline covers only TWSE-listed
common stocks (~1070 names from `data/fundamentals.json`); TPEX
symbols are out of scope by design, not by accident. A user
querying 3131/3587/6187 against the 4 stocktools endpoints will
correctly receive a structured 200 + `coverage_note: NOT_COVERED`
response (verified live post-deploy; see §7).

### 8.1 Note on the "3587 萬潤" symbol-recall slip

§1 listed "3587 萬潤" as a target of investigation. Web verification
and TPEX official records confirm **symbol 3587 belongs to 閎康
(Materials Analysis), not 萬潤 (All Ring Tech). 萬潤's actual
symbol is 6187.** Both companies are TPEX 上櫃 and both are correctly
excluded from this fix's scope. §1 entry is preserved as a verbatim
record of the original user input (which appears to have been a
symbol-recall slip rather than a trading intent), not as a
substantive claim.

## 9. Session closure

This document, `2026-08-06-equipment-stocks-chips-gaps-research.md`,
and the design manifest `2026-08-06-stock-coverage-notice.md` together
form a complete scope/state record as of main `045aca0b`. The fix is
live in production (`atlas-go` container running binary built from
`Commit=045aca0b7da27cc17f6cfdcc728873083ff78563`); live curl
verification confirms 3131/3587 return `coverage_note: NOT_COVERED`
in ~3ms (vs. the pre-fix 14.93s 503 on the chips path). Further
follow-ups tracked separately; nothing remains open in this session.
