---
title: 期貨日行情（first-party）資料層規格 — 階段 0
status: draft
updated: 2026-09-28
owner: marketdata / ledger
related:
  - docs/specs/event-calendar-date-invariants-spec.md（thirdWednesday 既有實作）
  - docs/reference/traps.md
  - internal/apigateway/CONSTITUTION.md（數據源憲法）
issue: "#2111（階段 0–1：期貨當訊號）；#2110（階段 2：可下單，本檔不涉及）"
---

# 期貨日行情（first-party）資料層規格 — 階段 0

> **本規格的目的**：讓系統能回答「**某日、某契約（TX/MTX）的 OHLC ＋ 成交量 ＋ 未平倉**」，
> 且可回溯多年、來源為 first-party、儲存 backend-aware。
>
> **本規格不授權**任何模擬／策略／配置引擎的變更，也不含任何下單、放空或槓桿能力（那是 #2110）。

| 項目 | 內容 |
|---|---|
| 文件角色 | 期貨日行情資料層的**權威規格**（上游契約 ＋ 領域模型 ＋ 儲存 ＋ 連續契約 ＋ CLI ＋ 測試） |
| 基準 | `origin/main` @ `4800e8ea`（2026-09-28） |
| 實測日期 | 2026-09-28（台北時間）；最近交易日 = 2026-09-24（09-25 中秋節、09-28 教師節休市） |
| 前置驗證 | 上游端點已以實跑 curl／httpx 證明（含**負對照**），證據見 §12 |
| 現況缺口 | `internal/marketdata` 目前只有 OI／口數（`taifex_provider.go` 的 `/PutCallRatio`、`/OpenInterestOfLargeTradersFutures`；`taifex_institutional.go` 三大法人契約明細）⇒ **沒有任何期貨成交價序列** |

---

## 0. 範圍（規範）

**在本階段（DO）**

1. 新增 first-party 期貨日行情 provider（OpenAPI 為主、官網 CSV 為歷史回補／fallback）。
2. 新增**獨立**領域型別（期貨 bar／契約規格），**不改動**既有股票型別。
3. 新增 backend-aware 的期貨 bar 儲存（新表 `futures_bars`；SQLite ＋ Postgres 兩後端）。
4. 提供連續契約（rollover back-adjust）純函式 ＋ 調整法規格。
5. 新增 backfill CLI `cmd/backfill-futures-bars`（`-start`／`-end`，預設寫入生產該寫的後端）。
6. 測試（含突變釘子）＋ 本規格。

**不在本階段（DON'T）**

- 不改 `internal/sim`、`internal/sectorallocation`、策略／配置引擎、任何下單或風控參數。
- 不加放空／槓桿（#2110）。
- 不做「一般 ＋ 盤後」合併 bar（資料雙軌入庫即可，合併留待有需求時再做）。
- 不新增 required CI check。

---

## 1. 上游契約（規範性；全部經實測）

### 1.1 來源 A — TAIFEX OpenAPI（JSON，最新交易日）

| 項目 | 值 |
|---|---|
| Method / URL | `GET https://openapi.taifex.com.tw/v1/DailyMarketReportFut` |
| 名稱 | 期貨每日交易行情（swagger `summary` 原文） |
| 回應 | HTTP 200；`Content-Type: application/octet-stream`；body 為 UTF-8 JSON array |
| 日期參數 | **無**。永遠只回**最新一個交易日**的全契約資料 |
| 用途 | **每日增量**（1 次請求取得當日全市場契約） |
| 注意 | 舊名 `/DailyMarketReportFutures` **不存在**（回 302 轉址到站首）⇒ 302 是「端點名錯」的訊號，不是資料問題 |

實測樣本（2026-09-28 抓取，最新日 `20260924`）：

```json
[{"Date":"20260924","Contract":"ZFF","ContractMonth(Week)":"202610","Open":"3605.6","High":"3615.6",
  "Low":"3584.8","Last":"3586.4","Change":"-25.8","%":"-0.71%","Volume":"219","SettlementPrice":"3586.8",
  "OpenInterest":"329","BestBid":"3585.4","BestAsk":"3590.6","HistoricalHigh":"3700","HistoricalLow":"3134",
  "TradingHalt":"","TradingSession":"一般","Volume(ExecutionsAmongSpreadOrderAndSingleOrderOnly)":""}]
```

### 1.2 來源 B — TAIFEX 官網 CSV（POST，可指定日期區間）

| 項目 | 值 |
|---|---|
| Method / URL | `POST https://www.taifex.com.tw/cht/3/futDataDown` |
| 表單 | `down_type=1`、`commodity_id={all\|TX\|MTX}`、`queryStartDate=YYYY/MM/DD`、`queryEndDate=YYYY/MM/DD` |
| 回應 | HTTP 200；`Content-Type: text/html;charset=MS950`；**body 是 CSV**（不是 HTML） |
| 編碼 | **MS950／Big5** ⇒ 必須轉碼後才可解析（沿用既有 `golang.org/x/text` 慣例，見 §3.3） |
| 用途 | **歷史回補**（多年）＋ OpenAPI 不可用時的 fallback |
| `commodity_id=""` | 回 404（HTML）⇒ 一定要給值 |
| `commodity_id=all` | 有效；單月（全契約）約 **4.3 MB** ⇒ 本階段只取 `TX`／`MTX` 以縮量 |

CSV 表頭（19 欄，實測原文）：

```
交易日期,契約,到期月份(週別),開盤價,最高價,最低價,收盤價,漲跌價,漲跌%,成交量,結算價,未沖銷契約數,最後最佳買價,最後最佳賣價,歷史最高價,歷史最低價,是否因訊息面暫停交易,交易時段,價差對單式委託成交量
```

欄位索引（**程式必須以此為準**；此索引即突變釘子的對象，見 §9）：

| idx | 欄位 | 用途 | 缺值樣態 |
|---|---|---|---|
| 0 | 交易日期 | `TradeDate` | — |
| 1 | 契約 | `Contract` | — |
| 2 | 到期月份(週別) | `ContractMonth`（可能 `202610`、`202609W5`、`202610/202611`） | — |
| 3–6 | 開盤價／最高價／最低價／收盤價 | `Open/High/Low/Close` | `-` |
| 7 | 漲跌價 | 不用（可由序列推導） | `-` |
| 8 | 漲跌% | 不用 | `-` |
| 9 | **成交量** | `Volume` | 缺值出現為 `0` |
| 10 | **結算價** | `SettlementPrice` | `-` 或 `NULL` |
| 11 | **未沖銷契約數** | `OpenInterest` | `-`（**盤後列固定為 `-`**，見 §2.3） |
| 12–15 | 最後最佳買價／賣價／歷史最高價／歷史最低價 | 本階段不用 | `-` |
| 16 | 是否因訊息面暫停交易 | 本階段不用（非空代表當日暫停交易） | 空 |
| 17 | **交易時段** | `Session`（`一般` / `盤後`） | — |
| 18 | 價差對單式委託成交量 | 本階段不用 | 空 |

實測樣本（2026-09-21 起 4 日、`commodity_id=TX`，共 94 行）：

```
2026/09/21,TX,202610  ,47607,48091,47496,48077,649,1.37%,37750,48053,101502,48078,48082,48091,39852,,一般,,
2026/09/21,TX,202610  ,47494,47584,47208,47405,-23,-0.05%,21514,-,-,47404,47415,47780,39852,,盤後,,
2026/09/21,MTX,202610  ,47590,48088,47495,48080,652,1.37%,80457,48053,31108,48080,48086,48088,39800,,一般,,
2026/09/22,TX,202610  ,48888,48946,48034,48243,190,0.40%,41691,48225,102946,48243,48247,48946,39852,,一般,,
```

### 1.3 判定規則：**表頭哨兵**（強制；#2107 教訓）

官網端點的失敗**不會**反映在 HTTP status（永遠 200），因此**禁止只看 status code 判定成功**。
正規化流程：

1. 轉碼（MS950 → UTF-8）。
2. **哨兵**：body 第一列（去除 BOM 與 `\r`）必須以 `交易日期` 開頭。否則：
   - 是 → 進入 CSV 解析。
   - 否 → **typed error `ErrTAIFEXSchema`**（不得靜默回空）。實測失敗樣態是 JS 轉址頁：
     `<script>alert("日期時間錯誤");window.location.replace("futDailyMarketView");</script>`
3. 表頭欄數必須 ≥ 19，且 `idx 1/2/3..6/9/10/11/17` 的欄名逐一比對（表頭驅動對映，不硬編索引）。
4. **表頭通過但 0 資料列** ⇒ typed **`ErrNoData`**（例假、或契約尚未上市）。
   實測：`1998/07/20`（TX 上市前一日）與 `2026/09/25`／`2026/09/28`（中秋／教師節）皆為「**只有表頭、0 資料列**」。

> 「只有表頭」與「JS 錯誤頁」是**兩種不同的世界**，必須分別對應 `ErrNoData` 與 `ErrSchema`。

### 1.4 分段規則：單次查詢區間 ≤ 31 天

實測（`commodity_id=TX`，end 固定 `2026/09/24`）：

| 起日 | 相差 | 結果 |
|---|---|---|
| 2026/08/24 | 31 天 | ✅ CSV，有資料 |
| 2026/08/23 | 32 天 | ❌ HTTP 200 ＋ JS 錯誤頁（`日期時間錯誤`） |
| 2026/06/01（3.8 個月） | 115 天 | ❌ 同上 |
| 2025/09/01（1 年） | 388 天 | ❌ 同上 |

⇒ **回補邏輯必須自行分段**。安全切法：以**日曆月**為單位（同月 1 日 → 末日；最大相差 30–31 天），
或「起日 ≤ end−31 天」。分段後每段獨立套用 §1.3 哨兵；區塊失敗只影響該區塊，不中斷整輪。

### 1.5 契約月格式：三種形態必須分類

| 形態 | 例 | 意義 | 本階段處置 |
|---|---|---|---|
| 月契約 | `202610` | 標準月契約 | **入庫**（`^\d{6}$`） |
| 週契約 | `202609W5`、`202610W1` | 加掛週契約（MTX 等） | 入庫但標記（連續契約不採用） |
| 價差組合 | `202706/202709` | 跨月價差委託 | **丟棄**（含 `/`；非單一契約，OHLC 為 `-`） |

連續契約序列只取 `^\d{6}$` 月契約。

### 1.6 資料起點（實測）

| 契約 | 本層可取得之最早資料日 | 佐證 |
|---|---|---|
| TX（臺股期貨） | **1998-07-21** | 該日有資料列；前一日 1998-07-20 為「只有表頭」 |
| MTX（小型臺指） | **2001-04-09** | 該日有資料列；前一日 2001-04-08 無（且為休市日） |

⇒ 驗收準則「可回溯多年」成立（TX 近 28 年）。

### 1.7 swagger 內與期貨（非選擇權）相關的 path

`https://openapi.taifex.com.tw/swagger.json` 共 135 paths（已存清單）。與本階段相關者：

| path | 說明 | 本階段 |
|---|---|---|
| `/DailyMarketReportFut` | 期貨每日交易行情（**本階段主來源**） | ✅ 採用 |
| `/TimeAndSalesData` | 每日期貨每筆成交資料（tick） | 未採用（階段 1 若需盤中再評估） |
| `/FinalSettlementPrice` 系列 | 最後結算價 | 未採用（與 bar 的 `SettlementPrice` 重疊） |
| `/OpenInterestOfLargeTradersFutures` | 大額交易人未沖銷結構 | 已由現有 channel 消費 |
| `/MarketDataOfMajorInstitutionalTradersDetails*Futures*` | 三大法人期貨契約明細 | 已由 `taifex_institutional.go` 消費 |
| `/Daily_FUT`（期貨商交易量日報表） | 期貨商別交易量，**非行情價** | 未採用（易誤用，特此標註） |

---

## 2. 語意規則（規範）

### 2.1 canonical 日 bar =「一般」交易時段列

每個 `(契約, 到期月, 交易日)` 在來源中**最多兩列**：`交易時段=一般` 與 `交易時段=盤後`。

**canonical 日 bar 一律取 `一般`（13:45 收盤）列**，理由：

1. **未平倉（OI）只在「一般」列有值**；「盤後」列的 `未沖銷契約數` 固定為 `-`
   ⇒ 若以盤後列為 canonical，日序列會**沒有 OI**，而 OI 是期貨訊號的主要輸入。
2. 一般列收盤（13:45）與**現貨 13:30 收盤同日對齊** ⇒ 基差（futures − spot）不會跨日錯位。
3. 盤後時段跨午夜（15:00 → 次日 05:00），其「交易日」歸屬語意與現貨不同，混用會污染日頻對齊。

`盤後` 列**照常入庫**（雙軌，`session` 欄區分），本階段**不合併**。未來若要「全日盤合併 bar」，資料已在庫，無需重抓。

### 2.2 缺值必須可區分「沒有值」與「值為 0」

來源以 `-`／`NULL`／空字串表示缺值。**禁止**把缺值映射為 `0`（#2107 同型陷阱）。
領域模型以指標型別表示可空欄位（§4）；`-` ⇒ `nil`，`0` ⇒ `0`。
判別「該列是否有實際成交」的規則：**`Close == nil` 或 `Volume == nil || *Volume == 0` ⇒ 無成交**（不可以 `Close==0` 判）。

### 2.3 每列的 `Session` 正規化

| 來源字串 | 領域值 | 說明 |
|---|---|---|
| `一般` | `SessionRegular` | 08:45–13:45（最後交易日 08:45–13:30） |
| `盤後` | `SessionAfterHours` | 15:00–次日 05:00 |
| 其他／空 | `SessionUnknown` | 不猜測；入庫並記錄 warn（不得靜默歸類為一般） |

---

## 3. 契約參考表（first-party 引文）

以下為期交所官網契約規格頁（2026-09-28 抓取，原文引述；HTML 存於 §12）：

| 項目 | TX（臺股期貨） | MTX（小型臺指期貨） |
|---|---|---|
| 契約頁 | `https://www.taifex.com.tw/cht/2/tX` | `https://www.taifex.com.tw/cht/2/mTX` |
| 契約價值（乘數） | 「臺股期貨指數乘上新臺幣**200 元**」 | 「小型臺指期貨指數乘上新臺幣**50 元**」 |
| 最小升降單位 | 指數 1 點（＝新臺幣 200 元） | 指數 1 點（＝新臺幣 50 元） |
| 一般交易時段 | 08:45–13:45；**最後交易日 08:45–13:30** | 同 TX |
| 盤後交易時段 | 15:00–次日 05:00；**最後交易日無盤後時段** | 同 TX |
| 最後交易日 | 「各契約的最後交易日為各該契約交割月份**第三個星期三**」 | 「各月份契約的最後交易日為各該契約交割月份**第 3 個星期三**；交易當週星期三加掛之契約，其最後交易日為掛牌日次二週之星期三」 |
| 最後結算日 | 「最後結算日**同最後交易日**」 | 同 TX |
| 最後結算價 | 最後結算日證交所交易時間**收盤前三十分鐘**內標的指數之**簡單算術平均價** | 同 TX |
| 交割方式 | 現金交割（依最後結算價差額淨額收付） | 同 TX |
| 到期月份規則 | 自交易當月起連續 3 個月份 ＋ 3／6／9／12 月中 3 個接續季月 | 同 TX，**另加掛週契約**（除每月第 1 個星期三外，於交易當週星期三加掛次二週之星期三到期契約） |
| 假日順延 | 「最後交易日若為假日或因不可抗力因素未能進行交易時，以其**最近之次一營業日**為最後交易日」 | 同 TX |

> **實作對應**：乘數 → `domain.FuturesContractSpec.Multiplier`；
> 第三個星期三 → 沿用 `internal/industry/event_calendar.go::thirdWednesday` 的既有邏輯
> （新程式碼需以**相同語意**實作或抽出共用；本階段先在期貨套件內自持一份以避免跨模組耦合，
> 並以測試鎖定「與 `industry` 版本同值」）。
> **假日順延**須以實際資料為準（本層以「該月有 OI 資料的最後一個交易日」驗證契約最後交易日，見 §6.2）。

---

## 4. 領域模型（獨立小結構；不動既有股票型別）

**禁止**改動 `domain.DailyBar` 或新增 `AssetClass` 到共用型別。
新增**獨立**型別（同檔或新檔 `internal/domain/futures.go`）：

```go
// FuturesSession 是期貨交易時段。
type FuturesSession string

const (
    SessionRegular     FuturesSession = "regular"     // 一般（08:45–13:45）
    SessionAfterHours  FuturesSession = "after_hours" // 盤後（15:00–次日 05:00）
    SessionUnknown     FuturesSession = "unknown"
)

// FuturesBar 是單一契約、單一到期月、單一交易日、單一時段的一根日 bar。
// 可空欄位用指標：來源的 '-'/'NULL'/'' 一律 nil，禁止以 0 代表缺值。
type FuturesBar struct {
    Contract        string         `json:"contract"`          // "TX" / "MTX"
    ContractMonth   string         `json:"contract_month"`    // "202610" / "202609W5"
    TradeDate       time.Time      `json:"trade_date"`        // Asia/Taipei 日曆日
    Session         FuturesSession `json:"session"`
    Open            *float64       `json:"open,omitempty"`
    High            *float64       `json:"high,omitempty"`
    Low             *float64       `json:"low,omitempty"`
    Close           *float64       `json:"close,omitempty"`
    Volume          *int64         `json:"volume,omitempty"`
    SettlementPrice *float64       `json:"settlement_price,omitempty"`
    OpenInterest    *int64         `json:"open_interest,omitempty"`
    Source          string         `json:"source"`            // "taifex_openapi" / "taifex_csv"
}

// FuturesContractSpec 是契約參考表（本階段只填 TX/MTX）。
type FuturesContractSpec struct {
    Code         string  // "TX" / "MTX"
    Name         string  // 臺股期貨 / 小型臺指期貨
    Multiplier   float64 // 200 / 50（每點新臺幣元）
    TickSize     float64 // 1 點
    HasWeeklies  bool    // MTX true
}
```

其他規範：

- `TradeDate` 一律以 **Asia/Taipei** 日曆日；時間部分歸零（`00:00`）。
- `Traded()` helper：`Close != nil && Volume != nil && *Volume > 0`。
- 契約規格表以常數 / `FuturesContractSpecFor(code)` 提供，**未知契約回 `ok=false`**（不給預設乘數）。

---

## 5. 儲存（backend-aware；#2107 教訓）

### 5.1 硬性規則

1. 後端一律由 **`ATLAS_STORE_BACKEND`（`config.Config.StoreBackend`）** 或顯式 flag 決定，
   經既有 `ledger.ResolveStoreBackend()` 正規化。
2. **嚴禁**「預設 sqlite、Postgres 需明示 flag」的形狀。若組態為 `postgres` 但連線池未注入 ⇒ **回錯誤並中止**，
   不得靜默降級（沿用 `NewFullStore` 的既有行為）。
3. **嚴禁**硬編碼 `data/state/atlas.db` 路徑（traps.md：production 為 Postgres-first）。

**釘子測試（強制）**：`TestNewFuturesBarStore_EnvPostgresNeverFallsBackToSQLite`
—— 環境宣告 `postgres` 但未注入連線池時，**即使 `ATLAS_SQLITE_PATH` 指向一個可寫檔**，
工廠也必須回錯誤；測試同時證明「同環境下的 sqlite 後端是可用的」，
以排除「其實是環境壞掉才報錯」的假陽性（`internal/ledger/futures_bar_store_test.go`）。

### 5.2 後端解析表

| `ATLAS_STORE_BACKEND` | 實作 | 備註 |
|---|---|---|
| `postgres`（**生產**） | `PostgresFuturesBarStore`（pgxpool） | 需先 `ledger.SetPostgresPool()`；未注入 ⇒ error |
| `sqlite` | `SQLiteFuturesBarStore`（`config.SQLitePath`） | 本機開發 |
| `jsonl`（未設定的舊預設） | `JSONLFuturesBarStore`（`config.LedgerDir` 下 `futures_bars.jsonl`） | 保留舊預設語意，不改變既有行為 |
| 其他 | **error** | `ResolveStoreBackend` 既有行為 |

> 與 quotes 的差異：**期貨 bar 不寫入 `quotes` 表**（避免 symbol 命名空間污染與 schema 誤用）。

### 5.3 Schema（SQLite 與 Postgres 同構）

```sql
-- sql/migrations/000025_futures_bars.up.sql（Postgres）
CREATE TABLE IF NOT EXISTS futures_bars (
    contract         TEXT        NOT NULL,
    contract_month   TEXT        NOT NULL,
    trade_date       DATE        NOT NULL,
    session          TEXT        NOT NULL,   -- regular | after_hours | unknown
    open             DOUBLE PRECISION NULL,
    high             DOUBLE PRECISION NULL,
    low              DOUBLE PRECISION NULL,
    close            DOUBLE PRECISION NULL,
    volume           BIGINT      NULL,
    settlement_price DOUBLE PRECISION NULL,
    open_interest    BIGINT      NULL,
    source           TEXT        NOT NULL,
    fetched_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (contract, contract_month, trade_date, session)
);
CREATE INDEX IF NOT EXISTS idx_futures_bars_contract_date ON futures_bars (contract, trade_date);

-- 連續契約 splice 錨點（§6.2 推導結果的落地，供 SQL 消費者與稽核）
CREATE TABLE IF NOT EXISTS futures_rollovers (
    contract         TEXT        NOT NULL,
    roll_date        DATE        NOT NULL,   -- 近月契約最後交易日（一般時段）
    from_month       TEXT        NOT NULL,
    to_month         TEXT        NOT NULL,
    price_diff       DOUBLE PRECISION NULL,  -- close(to, roll_date) − close(from, roll_date)
    computed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (contract, roll_date, from_month, to_month)
);
```

**冪等性**：寫入一律 `INSERT ... ON CONFLICT (PK) DO UPDATE`（SQLite：`INSERT OR REPLACE`）。
**缺值語意**：`nil` → SQL `NULL`（不寫 0）。

### 5.4 介面

```go
type FuturesBarStore interface {
    RecordFuturesBars(ctx context.Context, bars []domain.FuturesBar) (int, error)
    LoadFuturesBars(ctx context.Context, contract, contractMonth string, start, end time.Time, session domain.FuturesSession) ([]domain.FuturesBar, error)
    LoadLatestFuturesBars(ctx context.Context, contract string) ([]domain.FuturesBar, error) // 每到期月/時段最新一根
}
```

工廠：`ledger.NewFuturesBarStore(cfg config.Config) (FuturesBarStore, error)`（依 §5.2 分派）。

---

## 6. 連續契約（rollover back-adjust）

### 6.1 名詞

- **前月（front month）**：任一交易日中，到期月最近且**尚未到期**的月契約。
- **換倉日（roll date, R）**：某一月契約的**最後交易日**（表定＝該月第三個星期三；遇假順延）。
  該日近月與次月**同時有報價** ⇒ 可無縫計算價差。
- **段（segment）**：某一契約作為前月的連續交易日區間。

### 6.2 換倉日推導（以資料為準，不純靠日曆）

1. 日曆候選：`thirdWednesday(year, month)`；若該日無交易資料，順延至**次一有資料的交易日**（表定規則的假日順延）。
2. 驗證：`roll_date` 必須符合「該到期月在此日之後不再出現於資料中」（即為該月最後一個有資料日）；
   不符 ⇒ 記 warn 並以**資料實際最後一日**為準（資料優先）。
3. `price_diff(R) = close_to(R) − close_from(R)`，兩者皆取 **canonical（一般時段）** 收盤。
   任一為 `nil` ⇒ 該次 splice **不成立**（不補值、不插值），並記 warn。

### 6.3 調整法：**價差調整（back-adjust；older segments shifted to current level）**

**採用**：加法價差調整（difference / back-adjust），錨定於**最新段的原始價位**。

對某契約的連續序列，令 splice 點為 `R_1 < R_2 < … < R_n`（`diff_k = price_diff(R_k)`）：

```
adjusted(t) = raw_{segment(t)}(t) + Σ_{k : R_k > t} diff_k
```

- 最新段（`t > R_n`）不加調整 ⇒ 與現行市場價位一致。
- 越舊的段被平移越多 ⇒ 歷史段可能出現與當年真實指數不同的絕對值（長序列甚至可能為負）。
- **只調整價格**（OHLC 全部平移同一 `diff`）；**量與 OI 不調整**（調整後的口數無意義）。

**替代方案與為何不選**（規範性記錄）：

| 方案 | 定義 | 優點 | 為何不選（本階段） |
|---|---|---|---|
| 比率調整（ratio / panama） | 各段乘以累積比率 | 保留百分比報酬；不會出現負價 | 會**扭曲絕對點數**；階段 1 的用途是點數層級的門檻／基差／z-score，比率法使絕對門檻失去意義 |
| 不調整（raw splice） | 直接串接 | 0 加工、可完全回溯真實成交 | 換倉日有價差跳空，會被誤讀成單日大漲跌（污染波動率／動能） |
| 換倉日前 N 日提前換倉 | 提前到流動性高峰 | 貼近實務交易 | 引入未經實證的 N；本階段先用「最後交易日」這個**可稽核**的確定規則 |
| 只存原始、不建連續序列 | — | 最保守 | 階段 1 需要即時可用的連續序列；純函式可重算，不落庫調整值 |

**落地方式**：本階段以**純函式**實作，型別放在 `domain`（避免 `ledger → marketdata` 的反向依賴）：

```go
// internal/domain/futures_continuous.go
domain.FuturesRollover        // splice 事件（含 PriceDiff *float64）
domain.FuturesContinuousBar   // 調整後 bar（同時保留原始值與 CumulativeDiff）
domain.AdjustNone | domain.AdjustPriceDiff

// internal/marketdata/futures_continuous.go
marketdata.BuildContinuousSeries(bars []domain.FuturesBar, method domain.AdjustMethod)
    ([]domain.FuturesContinuousBar, []domain.FuturesRollover, error)
```

- **golden test**（`futures_continuous_test.go`）：合成 3 段、已知 splice（diff 8 與 7）⇒
  逐根斷言 `AdjustedClose` 與 `CumulativeDiff`（115/116/117、119/120、121），
  並斷言量與 OI **未被調整**。任何人改動調整法或換倉推導，這個測試必紅。
- **splice 事件落庫**：`futures_rollovers` 表（`FuturesBarStore.RecordFuturesRollovers`），
  由 CLI 於每次回補後推導並寫入（`-rollovers`，預設開）。
- **不落庫**整條調整後序列（可重算；避免與原始 bar 不一致的副本漂移）。

### 6.4 重算契約（可執行步驟；規範）

任何人都必須能用「原始 bar ＋ splice 事件」重算出同一條連續序列。步驟：

1. 取原始 bar：`SELECT contract, contract_month, trade_date, session, open, high, low, close, volume, open_interest
   FROM futures_bars WHERE contract = :c ORDER BY trade_date`。
2. 過濾：只留 `session='regular'`（canonical）且 `contract_month` 符合 `^\d{6}$`（排除週契約與價差組合）。
3. 決定每日前月：對交易日 `t`，前月 = 「該月契約的最後一個有資料日 ≥ `t`」的月份中**最小**者。
4. 取 splice 事件：`SELECT roll_date, from_month, to_month, price_diff FROM futures_rollovers
   WHERE contract = :c ORDER BY roll_date`。
5. 計算累積平移：對第 `j` 段（第 j 個前月區間）的任一根，
   `cum_j = Σ_{k>=j} price_diff[k]`（`price_diff IS NULL` 的 splice 以 0 計入，並應被視為缺口而告警）。
6. `adjusted_price = raw_price + cum_j`；`volume`、`open_interest` **不變**。
7. 驗證：在每個換倉日 `R_k`，`close(from, R_k) + cum_k` 必須等於 `close(to, R_k) + cum_{k+1}`（同日兩契約的調整後收盤相等）。

> 步驟 7 就是「序列連續」的可執行定義；若不成立，代表 splice 或前月推導有誤。

### 6.5 回溯性陷阱（規範）

- 連續序列**重算即變**：新增換倉日會平移所有更舊的段 ⇒ 調整後的值**不得**與原始 bar 混存於同一張表。
- 任何下游客戶端若快取調整值，必須以 `(contract, computed_at)` 失效。

---

## 7. Backfill CLI：`cmd/backfill-futures-bars`

沿用 `cmd/backfill-taifex-oi-*` 慣例（flag 解析 ＋ `run(config)` 可測 ＋ 摘要 ＋ pacing）。

| flag | 預設 | 說明 |
|---|---|---|
| `-contracts` | `TX,MTX` | 逗號分隔 |
| `-start` | `2001-01-01` | `YYYY-MM-DD`（Asia/Taipei） |
| `-end` | 今天（Asia/Taipei） | `YYYY-MM-DD` |
| `-backend` | `""` ⇒ `ATLAS_STORE_BACKEND` | 空字串代表沿用環境組態（**不得**硬編 sqlite） |
| `-source` | `auto` | `auto`（OpenAPI 優先、CSV 補歷史）／`csv`／`openapi` |
| `-pacing` | `1500`（ms） | 每請求最小間隔（下限強制） |
| `-max-retries` | `3` | 每段重試次數 |
| `-dry-run` | `false` | 只印不寫 |
| `-force` | `false` | 覆寫既有列（預設仍為 upsert，此 flag 保留給「拒絕覆寫」策略的未來擴充） |

行為：

1. 依 §1.4 分段（日曆月）；每段 `POST futDataDown`。
2. 套用 §1.3 哨兵；`ErrSchema` ⇒ 該段失敗但不中斷整輪（收集進摘要）；`ErrNoData` ⇒ 正常跳過。
3. 過濾：僅保留 `commodity_id` 指定契約、`^\d{6}$` 或 `W` 格式到期月、丟棄含 `/` 的價差組合。
4. 解析每列 → `domain.FuturesBar`（雙時段都存）。
5. 依 §5.2 解析後端並 upsert。
6. 輸出摘要（段數／列數／寫入列數／失敗段清單）並以非零 exit code 表示「有失敗段」。

---

## 8. 錯誤分類與可觀測性

沿用 `internal/marketdata/errors.go` 的三向分類（既有語意，不新增 sentinel）：

| 情況 | sentinel | breaker |
|---|---|---|
| 表頭哨兵失敗（JS 錯誤頁／HTML） | `ErrTAIFEXSchema`（`ErrSchema`） | 記失敗 |
| 表頭正常但 0 資料列（例假／未上市） | `ErrNoData` | **不記失敗** |
| transport／非 2xx／timeout | `ErrUpstream` | 記失敗 |
| 契約不存在於回應（過濾後為空） | `ErrNoData` | 不記失敗 |

**metrics**：實測 `internal/marketdata` 目前**沒有任何 prometheus 指標**（grep 0 命中），
因此本階段不強行發明新指標；改為 **Observer seam**：
（a）provider 提供 `FuturesBarsObserver` 介面（預設 no-op），**由 `cmd/backfill-futures-bars` 實作並注入**
（生產消費者，非只有測試在用）；未來要接 `internal/monitoring` 的 `GaugeSink` 只需換一個實作；
（b）breaker 狀態既有 `BreakerInfo()` 可觀測；
（c）backfill CLI 的摘要即為該次回補的稽核紀錄。

> **本階段決策（業主 2026-09-28 定案）**：**不做真 prometheus 指標**。
> 真指標要動 `internal/monitoring` ＋ rules ＋ 測試（且涉及「新增 required check 需先問」），
> 屬**獨立工作流**。期貨資料新鮮度指標 ＋ 告警規則（promtool 模式）已列為 **#2111 待辦**，
> 由 Observer seam 承接（見 §11）。

---

## 9. 測試（含突變釘子）

| 測試 | 內容 | 釘住什麼 |
|---|---|---|
| `TestParseFutDataDownCSV_Sample` | 以 §12 的真實 CSV fixture（2026-09-21~24、TX＋MTX）解析 | 欄位索引與型別（idx 9 量、10 結算、11 OI、17 時段） |
| `TestFetchBarsRange_HeaderSentinel_RejectsAlertPage` | 餵 §12 的 `negative_range_limit.html` | **200-but-error**：必須回 `ErrSchema`，**不得**回空 slice ＋ nil |
| `TestParseCSV_HeaderOnlyIsNoData` | 只有表頭的 CSV（1998/07/20 實測樣態） | `ErrNoData`（≠ `ErrSchema`） |
| `TestParseCSV_SkipsSpreadCombos` | 含 `202706/202709` 的列 | 價差組合不得入庫 |
| `TestParseCSV_WeeklyContracts` | 含 `202609W5` | 週契約可入庫、但連續序列排除 |
| `TestCanonicalSessionIsRegular` | 同日兩列 | canonical＝`一般`；OI 只在一般列 |
| `TestMissingValuesAreNilNotZero` | `-` / `NULL` | 缺值 → `nil`（**不得** → 0） |
| `TestChunkDateRange_MaxThirtyOneDays` | 分段函式 | 每段相差 ≤ 31 天 |
| `TestThirdWednesdayMatchesIndustry` | 期貨套件的第三個星期三 | 與 `internal/industry` 的既有實作**同值** |
| `TestBuildContinuousSeries_BackAdjust` | 合成 3 段、已知 splice | 調整值等於手算期望值；OI 不變 |
| `TestFuturesBarStore_BackendResolution` | `postgres` 未注入 pool | **必須 error**（不得降級 sqlite） |
| `TestSQLiteFuturesBarStore_RoundTrip` | upsert → load | PK 冪等、缺值以 NULL 存取 |

**突變釘子（mutation pin）要求**：每個「關鍵欄位」都要有一個**專門斷言其值**的測試，
且該測試必須在**人為改壞對應程式碼後變紅**。本階段至少執行下列 3 個突變並記錄結果：

| # | 突變 | 期望 |
|---|---|---|
| M1 | 把成交量索引 `9` 改成 `10`（結算價） | `TestParseFutDataDownCSV_Sample` 紅 |
| M2 | 移除表頭哨兵（改為直接解析） | `TestFetchBarsRange_HeaderSentinel_RejectsAlertPage` 紅 |
| M3 | 把 `-`（缺值）改成 `0` | `TestMissingValuesAreNilNotZero` 紅 |

> 實作階段必須**實際執行**這 3 個突變並把「紅燈證據」寫進 PR 的 Verification 段，不可只寫在規格裡。

---

## 10. 護欄（強制）

1. 不改 `internal/sim`、`internal/sectorallocation`、策略／配置引擎、風控參數。
2. 不加放空／槓桿／下單（#2110）。
3. store 必須 backend-aware；生產為 Postgres-first。
4. 2026-09-29 06:00Z 前不重啟 `atlas-go`、不動生產環境（本階段可在 repo 內完成全部工作）。
5. 不新增 required CI check。
6. 不改 detector 數量敘述。
7. 分支 `feat/20260928-futures-bars`，以 worktree 作業，不動主 clone HEAD。
8. 證據檔（§12）不進版控（`session-artifacts/` 已在 `.gitignore`）。

---

## 11. 未解問題 / 後續

1. **[#2111 待辦｜已定案延後] 期貨資料新鮮度指標 ＋ 告警規則**：以 promtool 單元測試模式
   （`promtool test rules`）撰寫「futures_bars 最新交易日 vs 交易日曆」的告警，
   並把 `FuturesBarsObserver` 接上 `internal/monitoring` 的 metrics bridge。
   **本階段不做**（業主 2026-09-28 定案）：需動 `internal/monitoring` ＋ rules ＋ 測試，
   屬獨立工作流，且涉及「新增 required CI check 需先問」。目前由 §8 的 Observer seam 取代。
2. **Tick 級資料**（`/TimeAndSalesData`）本階段未採用；若階段 1 需要盤中訊號再評估（含 payload 體積與 rate limit）。
3. **盤後合併 bar** 未定義（雙軌已入庫，未來可加）。
4. **跨商品擴充**（TXO、個股期貨）需先擴 §3 契約表；本階段只 TX/MTX。
5. `commodity_id=all` 的單月 4.3 MB 是否值得（一次性全市場回補）——本階段不做。
6. **連續序列落庫**：現行決策為不落庫（§6.3）。若未來有 SQL 消費者需要，另開表，
   但必須附「重算契約」（§6.4）的一致性測試，避免副本漂移。

---

## 12. 證據（不進版控）

路徑：`session-artifacts/futures-phase0-evidence/`（`.gitignore` 第 276 行已排除）

| 檔 | 內容 |
|---|---|
| `openapi-paths.txt` | swagger.json 的 135 個 path |
| `openapi-dailymarketreportfut.json` | `/DailyMarketReportFut` 實測 JSON（前 20 KB） |
| `futDataDown_2026-09-21_24_TX.csv` | CSV 實測（TX，4 日，94 行） |
| `futDataDown_2026-09-21_24_MTX.csv` | CSV 實測（MTX，含週契約） |
| `futDataDown_2001-01-02_TX.csv`、`futDataDown_2005-01-03_TX.csv`、`futDataDown_2012-01-16_TX.csv` | 多年回溯實測 |
| `negative_range_limit.html` | **負對照**：區間 >31 天 ⇒ HTTP 200 ＋ JS `alert("日期時間錯誤")` |
| `taifex-contract-spec-TX.html`、`taifex-contract-spec-MTX.html`、`taifex-contract-spec-extract.txt` | §3 契約規格原文與擷取 |
