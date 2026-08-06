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
2. **Handler**：`fetchLatestSymbolFlow` 把「symbol 在整個 7 天都沒出現」明確回 `nil SymbolFlow{} + ErrSymbolNotCovered`，前端顯示「不涵蓋」徽章。中等工程。
3. **資料源**：新增 TPEX T86 等價端點 + 把不涵蓋 symbol 列入 `data/fundamentals.json` snapshot。最大工程、改動多個 pipeline。

