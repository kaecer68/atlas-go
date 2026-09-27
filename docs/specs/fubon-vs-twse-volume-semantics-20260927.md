# Fubon / Fugle intraday `Volume` 與 TWSE 日收盤成交股數的語意契約：實證與建議（2026-09-27）

> **本文是「成交量語意契約」的實證與建議，不是 provider 缺陷報告。**
> 量測結論：`Fubon`／`Fugle` `intraday/quote` 的 `Volume` 與 `TWSE STOCK_DAY_ALL`（及 `MI_INDEX` 每日收盤行情）
> 的「成交股數」是**兩種不同的量**（口徑不同），不是同一種量少算 6–11%。

| 項目 | 內容 |
|---|---|
| 文件角色 | 語意契約（範圍語意）的**權威實證**與**修法建議**；對應 issue [#2018](https://github.com/kaecer68/atlas-go/issues/2018)、祖先 [#1987](https://github.com/kaecer68/atlas-go/issues/1987)（單位統一，已由 PR #2015 交付） |
| 基準 | `origin/main` @ `5094abc8`（2026-09-27） |
| 量測日期 | 最近交易日 **2026-09-24（1150924）**；該週 09-25 中秋、09-28 教師節休市 ⇒ 量測在休市狀態下進行（見 §9 邊界） |
| 樣本 | 上市全母體：**1,364／1,380 檔**（2026-09-24 日收盤 ∩ Fubon 報價；見 §9） |
| 本檔範圍 | **只讀量測 ＋ 書面建議**。本檔不授權、也不包含任何 provider／門檻／參數／契約的程式變更 |
| 相關文件 | [universe-quote-reliability-spec.md](universe-quote-reliability-spec.md)、[traps.md](../reference/traps.md)、[universe-scoring-ranked-zero-20260925.md](../operations/universe-scoring-ranked-zero-20260925.md) |

---

## 1. 結論（規範性敘述）

**C1（語意差異，非缺陷）.** 同標的同日下，`Fubon`（經 `services/fubon-proxy/main.py` 透傳 Fubon neo `total.tradeVolume`）
與 `Fugle`（`internal/marketdata/fugle_client.go` 的 `intraday/quote`）回報的 `Volume` 是
**「整股（regular lot）當日累計成交量」**：涵蓋 09:00–13:30 逐筆成交與 09:00 開盤、13:30 收盤的**集合競價**，
**不含**盤中零股、盤後零股、盤後定價交易、鉅額交易。

**C2（官方日收盤的範圍）.** `TWSE STOCK_DAY_ALL` 與 `MI_INDEX` 每日收盤行情的「成交股數」是
**「整股 ＋ 盤中零股 ＋ 盤後零股 ＋ 盤後定價 ＋ 鉅額」**（實測 1,380 檔兩者逐檔完全相同，見 §3）。

**C3（可分解到零）.** 因此兩者差額可被 TWSE 公開拆項**逐檔完整解釋**，無殘餘謎團：

| 項目（0050，2026-09-24，單位＝股） | 值 | 來源 |
|---|---|---|
| 整股當日累計（＝ Fubon ＝ Fugle `intraday/quote`） | 66,000,000 | `http://127.0.0.1:18081/quote/0050`（生產機唯讀）；`api.fugle.tw/.../intraday/quote/0050` |
| ＋ 盤中零股 | 3,997,147 | TWSE `exchangeReport/TWTC7U?date=20260924` |
| ＋ 盤後零股 | 147,792 | TWSE `exchangeReport/TWT53U?date=20260924` |
| ＋ 盤後定價 | 343,000 | TWSE `exchangeReport/BFT41U`／FinMind 逐筆 tick 14:30 print |
| ＋ 鉅額（單一證券） | 0 | TWSE `rwd/zh/block/BFIAUU?date=20260924` |
| **＝ TWSE 日收盤成交股數** | **70,487,939** | TWSE `exchangeReport/STOCK_DAY_ALL`／`MI_INDEX` ✅ 逐位相符 |

**C4（跨供應商驗證）.** Fubon `volume × domain.SharesPerLot` 與 **FinMind 逐筆成交（`TaiwanStockPriceTick`）的 `<14:00` 累計**在抽驗的 **9/9 檔完全相等**（不同供應商、不同傳輸管道，見 §2.2）。

**C5（issue #2018 敘述的更正）.** 「Fubon 比 TWSE 少 6–11%」是**口徑差，不是少算**；
且 6–11% 只是**零股占比高的 ETF 的 p90–p95 段**（全體樣本 gap 中位數僅 **1.59%**，見 §4）。

**C6（真風險）.** 真正的系統性風險不是這個比例，而是**同一輪母體／風控內兩種語意並存**：
生產 `scoring_filters` 記錄 `lots_converted=1120 / quotes_returned=1301` ⇒ 約 86% 報價是 Fubon（整股）語意、
其餘來自 TWSE/TPEx（全口徑）語意，卻套用同一個門檻與同一組排名權重（見 §6）。

---

## 2. 逐項分解與交叉驗證

### 2.1 九檔逐檔精確等式（【實測】）

`TWSE 日收盤 = 整股(Fubon) + 盤中零股 + 盤後零股 + 鉅額 + 盤後定價`（單位：股）

| 代號 | TWSE 日收盤 | 整股（Fubon＝tick<14:00） | 盤中零股 | 盤後零股 | 鉅額 | 盤後定價（14:30 print） |
|---|---|---|---|---|---|---|
| 0050 | 70,487,939 | 66,000,000 | 3,997,147 | 147,792 | 0 | 343,000 |
| 00685L | 136,663,568 | 134,019,000 | 640,136 | 63,432 | 0 | 1,941,000 |
| 00919 | 86,095,342 | 81,595,000 | 3,399,177 | 70,165 | 0 | 1,031,000 † |
| 3481 | 101,964,433 | 100,995,000 | 580,548 | 36,885 | 0 | 352,000 |
| 1303 | 67,158,368 | 65,983,000 | 1,005,228 | 26,140 | 0 | 144,000 |
| 3189 | 44,137,311 | 41,273,000 | 1,363,837 | 10,474 | 1,434,000 | 56,000 |
| 3231 | 31,269,892 | 27,147,000 | 438,129 | 16,763 | 3,605,000 | 63,000 |
| 2885 | 13,758,876 | 12,243,000 | 382,411 | 18,465 | 1,107,000 | 8,000 |
| 2496 | 12,765 | 3,000 | 765 | 0 | 0 | 9,000 |

† `00919` 的拆項和比 TWSE 日收盤多 3 張（0.003%）＝ tick 聚合粒度差，非結構性差異。

### 2.2 跨供應商驗證（【實測】，9/9 完全相等）

`Fubon volume`（生產機 `127.0.0.1:18081/quote/<sym>`，原始單位＝張）與
`FinMind TaiwanStockPriceTick`（逐筆，單位＝張）在 **`Time < 14:00` 的累計量逐檔相等**：
0050 66,000／00685L 134,019／00919 81,595／3481 100,995／1303 65,983／3189 41,273／3231 27,147／2885 12,243／2496 3。
⇒ 「整股當日累計」的定義不是 Fubon 單一供應商的實作細節。

### 2.3 同一供應商兩種端點，兩種定義（【實測】，最強單點證據）

| 端點（同一天 2026-09-24、同一檔 2330） | 值 | 對應語意 |
|---|---|---|
| Fugle `intraday/quote/2330` → `total.tradeVolume` | **12,989 張** | 整股當日累計（＝ Fubon） |
| Fugle `historical/candles/2330?from=&to=` → `volume` | **14,557,662 股** | 全口徑（＝ TWSE 日收盤，逐位相同） |
| TWSE `STOCK_DAY_ALL` 2330 | 14,557,662 股 | 全口徑 |

⇒ **差異在端點語意，不在供應商能力**；把 `intraday/quote` 當「日量」使用是消費者端的語意誤用。

---

## 3. 官方日收盤的範圍與拆項來源

| 拆項 | 端點 | 備註 |
|---|---|---|
| 全口徑日收盤 | `exchangeReport/STOCK_DAY_ALL`（2026-06-30 起為 CSV）、`exchangeReport/MI_INDEX?type=ALLBUT0999&date=YYYYMMDD` | **兩者逐檔完全相同**（1,380 檔，0 差異）⇒ 可互為替代；`MI_INDEX` 可帶日期、單次請求覆蓋全市場 |
| 盤中零股 | `exchangeReport/TWTC7U?date=YYYYMMDD` | 上市、逐檔、`成交股數`（股） |
| 盤後零股 | `exchangeReport/TWT53U?date=YYYYMMDD` | 上市、逐檔、`成交股數`（股） |
| 鉅額（單一證券） | `rwd/zh/block/BFIAUU?date=YYYYMMDD` | 逐筆列示後自行彙總；`總計` 列與逐檔和一致 |
| 盤後定價 | `exchangeReport/BFT41U?date=YYYYMMDD&selectType=NN`（NN=01..34） | **不含 ETF**（實測 1,033 檔皆為股票），且對部分個股與逐筆 print 不一致（見 §9）⇒ **不可當逐檔權威** |

---

## 4. 分布（【實測】；2026-09-24，**上市全母體**；n=1,364／1,380 ＝ 98.8%）

`gap% = 1 − Fubon_shares / TWSE_shares`（股對股；Fubon 已乘 `domain.SharesPerLot`）。
下表為**全覆蓋樣本**（第一批掃描因 fubon-proxy 節流失敗的 696 檔已全部重試成功，見 §9）；
括號內為第一批子樣本（n=675）的值，兩者同量級 ⇒ 分布不依賴抽樣。

| 統計 | 全體 n=1,361 † | ETF n=235 | 普通股 n=1,079 | 特別股 n=25 | 權證 n=8 |
|---|---|---|---|---|---|
| p50 | **1.70%**（1.59%） | 1.94% | 1.73% | 0.64% | 0.00% |
| p75 | 3.59% | — | — | — | — |
| p90 | 7.79% | 8.72% | 7.64% | 7.13% | 0.00% |
| p95 | **11.23%**（11.13%） | — | — | — | 0.00% |
| p99 | 26.83% | — | — | — | — |
| max | **76.50%**（2496 卓越） | — | — | — | — |
| gap=0（完全相同） | 44 檔（3.2%） | 5 | 21 | 6 | **8/8** |

† 自 n=1,364 排除 3 檔外幣計價 ETF（`00636K`／`00657K`／`00668K`，見 §9）。

- **權證 8/8 完全相同**：權證無零股市場 ⇒ 直接佐證「差額＝零股／盤後／鉅額類別」。
- ETF 與普通股的**中位數差距不大（1.94% vs 1.73%）**；ETF 的尾部較重（第一批子樣本的 ETF p5 = 19.6%）。
  但**這不是 ETF 專屬問題**：普通股 `2496` 卓越 gap 76.50%。
- **上櫃（TPEx `openapi/v1/tpex_mainboard_daily_close_quotes` 對照，n=11，`TradingShares`）**：
  median **4.60%**、範圍 0.61%（1569 濱川）～11.60%（5274 信驊）⇒ 同一語意差在上市／上櫃都存在，
  **不是**「上市與上櫃走不同來源」造成的。
- 樣本限制：見 §9。

---

## 5. 五項否證（issue #2018 列的候選成因）

| 候選成因 | 判定 | 證據 |
|---|---|---|
| (a) 取樣時間點（盤中快照 vs 收盤） | **否證**（對本例） | Fubon `is_close=true`、`lastUpdated=1790227800000000µs` ＝ **2026-09-24 13:30:00.000000 CST**；OHLC 112.00／112.45／111.75／112.40 與 TWSE 該日逐項相同；TWSE 端為 1150924 日收盤 ⇒ 兩者皆為收盤後最終值。休市日重查仍回 09-24 最終值 |
| (b) 未計入的成交類別 | **成立，且可逐項量化** | §2 九檔等式 ＋ **1,364 檔合計**：gap 110,887,697 股 ＝ 盤中零股 76,261,996（68.8%）＋盤後零股 2,113,688（1.9%）＋鉅額 12,305,413（11.1%）＋殘差 20,206,600（18.2%）；殘差＝盤後定價（其中 12,145,600 屬 ETF：`BFT41U` 不收 ETF，已逐檔以 FinMind 14:30 print 驗證；8,061,000 屬股票，與 `BFT41U`／逐筆 print 相符）。**787/1,364（57.7%）殘差恰為 0**。交叉驗證：`BFIAUU` 逐檔加總＝其「總計」列＝**12,305,413 股**，與本表鉅額欄逐位相同 |
| (c) 市場別（上市 vs 上櫃不同來源） | **否證**（不是成因） | Fubon 對上櫃同樣有值且同樣短少 0.61–11.60% ⇒ 語意差與市場別無關。但**生產確實混源**：TWSE 寬臂只涵蓋上市（`STOCK_DAY_ALL` 在 1,599 檔母體中覆蓋 904），上櫃由 TPEx 日收盤補齊（`cmd/atlas/bootstrap_helpers.go`） |
| (d) ETF 特殊處理 | **否證**（ETF 只是放大鏡） | 見 §4；ETF 專屬 provider（`internal/marketdata/fubon_etf_provider.go`）處理的是 PCF／NAV，與量能無關 |
| (e) Fubon 官方文件明示 | **查不到** | Fubon neo 內嵌 Fugle 規格；`services/fubon-proxy/main.py::convert_quote` 直接透傳 `total.tradeVolume`。官方只述「累計成交量」，**無任何文件**明示「不含零股／盤後／鉅額」⇒ 本契約的範圍語意是**反推＋實測**得出（§2.3 為其反證） |

---

## 6. 影響量化與真風險

### 6.1 門檻翻面（【實測】；用各檔自身 `Last` 計 turnover）

| 門檻（現行值） | 危險帶檔數 | 翻面檔數 | 清單（代號） |
|---|---|---|---|
| 母體 `volume_floor_twd = 10,000,000` | 29（[10.0M, 11.5M)） | **6** | 0057、00875、2417、2442、3338、9917 |
| 風控 `min_daily_amount_twd = 5,000,000` | 24 | **3** | 2762、4104、6585 |
| `shouldReducePosition`（`internal/orchestrator/plugin_sector.go`，`Volume < 1,000,000` 股） | 28（[1.0M, 1.15M)） | **4** | 2485、2636、3704、4551 |

- turnover 加權低估（**1,364 檔全樣本**）：TWSE 合計 **0.7769 兆** TWD vs Fubon **0.7376 兆** ⇒ **5.06%**
  （第一批子樣本 n=675 為 4.21%；兩者同量級）。
- 方向性：Fubon 來源一律**偏低** ⇒ 門檻偏向**排除**（`below_turnover_floor`／`symbols_excluded(liquidity)` 略增）。
- 翻面比例約 **0.44%**（6/1,364）；對 1,599 檔（上市＋上櫃）母體是同量級（≈7 檔），**不可**當精確預測。

### 6.2 真風險：同一輪內兩種語意並存（【推論】，基於生產 log ＋ §4 分布）

生產 `scoring_filters`（2026-09-25 手動觸發，`../operations/universe-scoring-ranked-zero-20260925.md`）：
`input=1599 no_quote=298 lots_converted=1120 survivors=652`；`quotes_returned=1301`。

- 約 **86%（1120/1301）** 報價來自 Fubon／Fugle（**整股**語意），其餘來自 TWSE／TPEx（**全口徑**語意）。
- 兩組被同一個門檻（`volume_floor_twd`／`min_daily_amount_twd`）與同一組排名權重比較 ⇒
  **跨股不可比、且同一檔在不同輪可能換來源**（arm 順序 `fubon → finmind → fugle → twse` 疊加負載與斷路器）
  ⇒ 排名／納入的**可重現性**受損，且偏誤方向固定（Fubon 側偏低 ⇒ 偏排除）。
- 這比「6–11%」本身更嚴重：它是**判定系統的語意不變量**被破壞，而不是單一數值誤差。

---

## 7. 建議（書面；本檔不實作）

1. **日量權威＝官方日收盤**（TWSE `MI_INDEX`／`STOCK_DAY_ALL`、TPEx 上櫃日收盤）。`intraday/quote` 只作
   「即時價／盤中判斷」；**不得**作為日量門檻的輸入。實測 `MI_INDEX?type=ALLBUT0999` 單次請求即覆蓋整個上市市場
   （1,380 檔與 `STOCK_DAY_ALL` 0 差異），上櫃再一次請求 ⇒ 成本比逐檔 arm 更低。
2. **契約層標註範圍語意**（列為後續票，**不在本輪**）：於 `domain.Quote`（`internal/domain/shared/shared.go`）之
   `Volume` 明示範圍（例如新增 `VolumeScope`：`session_regular` vs `day_all_categories`），或在 provider 邊界留下
   不可誤用的註解。**這是跨消費者變更**，需在量能路徑凍結期（09-29 決定性驗收）之後獨立進行。
3. **母體／風控回到單一口徑**：門檻與排名只在同一範圍語意內比較（建議 1 的來源）；若必須混源，至少把「來源」
   帶進快照與 log，使混比可觀測。
4. **不做數值補償**（例如一律加 1.6%）：差額逐檔不同（0%–76.5%），任何常數修正都會製造新的偏誤；本檔的分布即為其否證。
5. **驗收紀律（09-29）**：
   - 不要把「語意差」誤記為「#2015 單位修正未完成」；驗收時同時看 `quotes_returned`／`chunks_failed`／來源混比。
   - 預期 Fubon 來源標的額外偏低 **0.5–11%（中位 1.6%）** ⇒ `below_turnover_floor`／`symbols_excluded(liquidity)`
     會比「全官方口徑」略高，且**上櫃 cohort 與上市 cohort 不可直接互比**（上櫃中位 4.60%）。

---

## 8. 可重現指令（全部唯讀）

```bash
# 日收盤（全口徑）與其替代
curl -s 'https://www.twse.com.tw/exchangeReport/STOCK_DAY_ALL?response=json'              # date 欄＝1150924（CSV）
curl -s 'https://www.twse.com.tw/exchangeReport/MI_INDEX?type=ALLBUT0999&date=20260924&response=json'  # tables[8]
# 拆項
curl -s 'https://www.twse.com.tw/exchangeReport/TWTC7U?response=json&date=20260924'       # 盤中零股
curl -s 'https://www.twse.com.tw/exchangeReport/TWT53U?response=json&date=20260924'       # 盤後零股
curl -s 'https://www.twse.com.tw/rwd/zh/block/BFIAUU?date=20260924&response=json'         # 鉅額（單一證券）
curl -s 'https://www.twse.com.tw/exchangeReport/BFT41U?response=json&date=20260924&selectType=01'   # 盤後定價（NN=01..34；不含 ETF）
# 上櫃日收盤（成交股數＝TradingShares）
curl -s 'https://www.tpex.org.tw/openapi/v1/tpex_mainboard_daily_close_quotes'
# Fubon 原始值（張）— 生產機唯讀
ssh kmacmini "curl -s http://127.0.0.1:18081/quote/0050"
# 同供應商兩端點對照（Fugle）
curl -s -H "X-API-KEY: $FUGLE_API_KEY" 'https://api.fugle.tw/marketdata/v1.0/stock/intraday/quote/2330'
curl -s -H "X-API-KEY: $FUGLE_API_KEY" 'https://api.fugle.tw/marketdata/v1.0/stock/historical/candles/2330?from=2026-09-24&to=2026-09-24&fields=close,volume'
# 逐筆 tick（<14:00 累計 ≡ Fubon；14:30 print ＝ 盤後定價）
curl -s -H "Authorization: Bearer $FINMIND_API_KEY" \
  'https://api.finmindtrade.com/api/v4/data?dataset=TaiwanStockPriceTick&data_id=0050&start_date=2026-09-24&end_date=2026-09-24'
# 09-29 待驗：pipeline 14:00 觸發時，日收盤端點的日期是否已 rollover
curl -s 'https://www.twse.com.tw/exchangeReport/MI_INDEX?type=ALLBUT0999&date=$(date +%Y%m%d)&response=json' | head -c 300
```

---

## 9. 量測邊界（【誠實列表】）

- **樣本與節流（兩批合併 ⇒ 全覆蓋）**：上市 1,380 檔取 Fubon 報價 —— 第一批以 ~10 req/s 掃描時 **696 檔回 HTTP 500
  ＝ fubon-proxy 節流**（SDK `intradayLimit ≈ 30/min`），重試即恢復（2330 → 12,989 張、2317 → 32,929 張）
  ⇒ 屬**掃描副作用，不是符號缺資料**。第二批以 1.5 s/檔重試，**696/696 全部成功** ⇒ 合併後可用配對 **n=1,364**
  （1,380 檔的 98.8%；其餘 16 檔為當日無成交之無報價列）。§4／§5(b)／§6.1 的正式數字採全樣本，括號內保留第一批子樣本值。
- **休市量測**：09-24 為最近交易日（09-25 中秋、09-28 教師節）⇒ 「盤中即時值行為」與「TWSE 日收盤 rollover 時點」
  **未量測**。
- **【待驗證，09-29 必查】** 母體 pipeline 觸發時間為 **14:00 台北**（`internal/monitoring/universe_scheduler.go` 的
  `universeTriggerHourTW`，註解自述「收盤後、跑在前一交易日資料上」）；`STOCK_DAY_ALL` **無日期參數**（latest-only）。
  若 14:00 時 Fubon 已是「當日」而 TWSE 仍是「前一交易日」，同一輪內會混到**不同交易日**的量——
  這比 6–11% 的類別差**更大**，應納入修復票範圍。查法見 §8 最後一行。
- **`BFT41U` 不含 ETF**（1,033 檔皆為股票），且對 3481／3189／3231 的值恰為逐筆 14:30 print 的 **2 倍**
  （1303／2885／2496 則相符）⇒ 逐檔權威請用 FinMind 14:30 print；本檔 §2.1 以此驗 9 檔。
- **外幣計價 ETF**：`00636K`／`00657K`／`00668K` 反向（Fubon ＝ TWSE × 10：200 股 vs 2,000 股），
  未列入 §4 分布，需另案判定（可能是外幣交易單位的聚合差異）。
- 本次未變更任何 provider／門檻／參數／config；未寫入生產狀態（僅對生產機 loopback 與公開端點做 HTTP GET）。

---

## 10. 修訂紀錄

| 日期 | 變更 |
|---|---|
| 2026-09-27 | 建立：issue #2018 的唯讀量測（分解式、9/9 跨供應商驗證、分布、五項否證、影響量化與建議）（PR #2097） |
| 2026-09-27 | 分布與影響量化改採**上市全母體樣本 n=1,364**（第一批節流失敗的 696 檔重試 696/696 成功）；第一批 n=675 的值以括號保留。結論與否證不變 |
