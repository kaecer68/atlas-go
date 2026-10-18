# 「未守 `Close>0`」價格→報酬／波動負責端權威盤點（FU-20260930-04，2026-10-16）

| 項目 | 內容 |
|---|---|
| 文件角色 | [FU-20260930-04](../operations/FOLLOWUPS.md) 要求的**權威盤點**（消費者 × 是否守門 × 壞列行為）＋本批修法與負向證明；同時是 FU-20260930-04 的處置紀錄 |
| 基準 | `origin/main` @ `bfac872d8`（2026-10-16）；下表行號為**基準版**行號（本 PR 改動後會位移） |
| 壞值來源 | [FU-20260930-03](../operations/FOLLOWUPS.md)：`quotes` 有 `close<=0` 109 列／32 檔（`Close=0` 表示停牌未表達缺值）、`5904.TW` 約 10 倍價、`2380.TW` 垃圾列 |
| 判準方法 | 機械式：`grep -rEn` 掃 `internal/**`＋`cmd/**` 非測試 `.go`，取「同時含價格識別字（`Close`/`Price`/`Last`/`prev`/`series`/`bars`）與除法」的行，再逐筆讀碼判定守門強度（**不靠印象**） |
| 守門強度記號 | ✓ 已守（含 `> 0` 或 `<= 0` 過濾）／◐ 只守 `== 0` 或只守單端（負值、NaN/Inf、另一端可穿透）／❌ 未守 |
| 本檔範圍 | **只含價格→報酬／波動／漲跌幅／回撤／標籤**的消費端。價格水準（SMA、`close` 特徵）與非價格數列（成交量、OI、出口值、CPI、匯率水位）不列入，僅於 §3 註記 |

---

## 1. 主要清單（儲存價格序列 → 報酬／波動／標籤）：本批修復

> 這些路徑吃的是**儲存的日收盤序列**（`replay` CSV／JSONL、`HistoricalPrices`、`[]domain.DailyBar`），也就是 FU-20260930-03 壞值實際流經的地方。共同選定語意：**非正或非有限收盤價＝缺值 ⇒ 跳過該觀測**，且「跳過後無可用觀測」時沿用該函式既有的「無資料」回傳值（`0`／`ok=false`）。

| # | 消費端（基準 `file:line`） | 修前守門 | 壞列行為（實測） | 本批處置 |
|---|---|---|---|---|
| 1 | `internal/portfolio/historical_prices.go:142` `MomentumReturn` | ❌ 只守 `past == 0`（latest 未守） | `latest=0` ⇒ **−1（−100%）**；`latest=NaN` ⇒ NaN | 修：兩端都要 `shared.IsUsablePrice` ⇒ 回 0 |
| 2 | `internal/portfolio/historical_prices.go:302` `Volatility` | ❌ 只守 `series[i-1] == 0` | `curr=0` ⇒ −100% 進入 mean／σ（fixture `[100,101,0,102]` 由 0 變 **0.505**） | 修：任一端不可用即跳過該日對 |
| 3 | `internal/portfolio/optimizer_math.go:92` `extractReturnMatrix` | ❌ 只守 `prev == 0` | `curr=0` ⇒ −1 進入共變異數矩陣與均值（Markowitz 期望報酬） | 修 |
| 4 | `internal/monitoring/risk_exclusion.go:318` `realizedVolatility` | ❌ 只守 `series[i-1] == 0` | 同上（fixture 由 0 變 **0.55**） | 修 |
| 5 | `internal/monitoring/risk_exclusion.go:349` `maxDrawdown` | ❌（`peak > 0` 只保護分母） | `close=0` ⇒ `(peak-0)/peak` = **100% 回撤** ⇒ 單獨超過 30% 門檻而**排除該檔** | 修：非可用價整筆略過 ⇒ 0.25 |
| 6 | `internal/feature/feature.go:26` `return_1d` | ❌ 只守 `prev > 0` | `curr=0` ⇒ **−1** | 修 |
| 7 | `internal/feature/feature.go:32` `return_5d` | ❌ 只守 `prev > 0` | 同上 | 修 |
| 8 | `internal/feature/feature.go:334` `return_autocorr` | ❌ 只守 `pos-1` | `curr=0` ⇒ −1 進入自相關（0.3699 vs 0.5432） | 修 |
| 9 | `internal/feature/feature.go:310` `price_position` | ◐ 只守 `== 0` | 負值穿透 ⇒ 符號翻轉（fixture **21.66**） | 修：改 `IsUsablePrice` |
| 10 | `internal/feature/feature.go:326` `quality_intra` | ◐ 只守 `== 0` | 負值穿透 ⇒ `1-(H-L)/close` = **1.8** | 修 |
| 11 | `internal/feature/feature.go:587` `ForwardReturnLabel` | ❌ 只守 `bars[i]` | 前向缺值 ⇒ 標籤 **−1** | 修 |
| 12 | `internal/replay/twse_csv.go:127` `Dataset.ForwardReturn` | ◐ 兩端只守 `== 0` | 負值／NaN 穿透 ⇒ `ok=true` 且回傳垃圾值 | 修 |
| 13 | `internal/orchestrator/ml_scorer.go:217` `forwardReturn`＋`Train` | ❌ 只守 `current.Close == 0` | 前向 `Close=0` ⇒ 標籤 **−1**；且 `open=close=0` 時 `extractFeatures` 產生 **NaN** 特徵 | 修：`Train` 直接跳過該樣本（含特徵） |
| 14 | `internal/scheduler/ml_retrain.go:237` `extractLabels` | ❌ **完全未守** | `Close=0` 分母 ⇒ **±Inf**；前向 0 ⇒ −1；且與 `extractFeatures` 產生**不同列數** | 修：兩者改用同一 `sampleIndexes` 清單 |
| 15 | `internal/stockpicker/backtest.go:189` 前向報酬 | ◐ 只守觸發日 | 前向 `Close=0` ⇒ 寫入 **−100% outcome** ⇒ 進入 win-rate 聚合 | 修 |
| 16 | `internal/calibration/load.go:117` 校準標籤 | ◐ 只守 `== 0` | 負值穿透 ⇒ **−1.5882**，且污染 `prevClose` 鏈（下一列也錯） | 修 |

## 2. 同族掃描：已守門（列舉、不加冗餘檢查）

| 消費端（`file:line`） | 守門 | 說明 |
|---|---|---|
| `internal/industry/correlation_loader.go:82-119` | ✓ | 先以 `bar.Close <= 0` 過濾，再以 `prevPrice > 0` 計 return |
| `cmd/calibrate-seasonal/main.go:297-330` | ✓ | 同型（`bar.Close <= 0` 過濾 + `prevPrice > 0`） |
| `internal/feature/feature.go:150` `amihud`、`:39` `hl_ratio`、`:217` `kurtosis_20d` | ✓ | 兩端 `> 0`（`kurtosis` 缺值以 0 補，為既有語意） |
| `internal/marketdata/taiwan_volatility_provider.go:229-241` | ✓ | 對數報酬兩端 `> 0`，且結果另做 NaN/Inf 檢查 |
| `internal/marketdata/official_series_actions.go:163-200` | ✓ | 先過濾 `Close <= 0` 才計比率 |
| `internal/portfolio/factor_engine_momentum.go:52`、`factor_engine_liquidity.go:21`、`etf_analysis.go:83` | ✓ | 即時報價路徑守 `Open == 0`／`Last <= 0` |
| `internal/reflexivity/concrete_rules.go:19,104` | ✓ | 守 `q.Open == 0` |
| `internal/orchestrator/executor_collection.go:239`、`regime_inference.go:49,72` | ✓ | 守 `Last/Open/High/Low > 0` |
| `internal/monitoring/service/session.go:206` | ✓ | 守 `bar.Open > 0` |
| `internal/narrative/ingestor.go:153,584,601` | ✓ | 守 `prev.Value != 0`（總經資料，非 `quotes`） |
| `cmd/backfill-macro-history/main.go:412-417` | ✓ | 守 `prevClose > 0` 且有跳空倍數閘（`maxDailyChangePct`） |
| `cmd/daily-replay-sync/main.go:500-506` | ✓ | 守 `prevClose > 0`，且只**告警**不產生數值 |
| `internal/domain/volume_divergence.go:116` | ✓ | `high<=0 || low<=0` 即拒 |
| `internal/marketdata/microstructure_provider.go:45` | ◐ | 守 `Last == 0`（負值可穿透，但 `Last` 來自即時報價契約；列清單不修） |

## 3. 未修：需要另外裁決（提案，非本 PR）

| 項 | 位置 | 為什麼只提案 |
|---|---|---|
| 跳空倍數（10 倍價）防護 | 全族 return 消費者 | 現行守門只處理「非正／非有限」。要擋 `5904.TW` 型 10 倍值需**跳空門檻**（例：日報酬 \|r\|>50% 視為壞值）；這會改變**既有指標數值語意**（正常但劇烈的真事件也會被丟掉）⇒ 屬值語意變更，需業主授權 |
| 單純「價格水準」消費者 | `historical_prices.go:126` `SMA`、`feature.go:44` `ma_ratio`、`feature.go:53` 等 | 不是報酬／波動；`close=0` 會拉低均價，但修它等於改變指標定義 ⇒ 只列清單 |
| 總經／即時報價 `(Last-Open)/Open` 未守 | `internal/orchestrator/system_narrative.go:63-87`、`internal/orchestrator/system.go:1416-1438` | 資料源與 `quotes` 不同；`Open=0` ⇒ ±Inf/NaN，但**可達性未證實**（BDI 空報價已由 provider 轉為 `ErrEmptyQuote`），且「缺值＝0 或略過」屬敘事層值語意 ⇒ 另案處理 |
| `mean` 除以固定 `Tret` | `optimizer_math.go:96` | 跳過樣本時均值分母未同步縮減（既有偏誤、與壞值無關）⇒ 另案 |
| 研究工具（`cmd/experimental/**`） | `jev-eval-sympanel/main.go:284,322`、`sbl-ic-study/main.go:224` `trailingReturn` | 同型未守（`curr=0` ⇒ −1），但為**非生產**研究 CLI，其輸出是研究結論而非生產數值 ⇒ 只列清單 |

## 4. 資料修復（`FU-20260930-03`）：只提案，屬 operator 動作

本 PR 不動任何資料。建議（與 FU-20260930-03 原敘述一致）：

1. 寫入端把 `close <= 0` 視為**缺值**（拒絕或標記），不要寫成 0。
2. `5904.TW` 2026-08-10、`2380.TW` 2026-06-29 修值或標缺。
3. 本 PR 已讓消費端不再把 `close<=0` 讀成 −100%，但**10 倍價仍會穿透**（見 §3）⇒ 修資料仍是唯一解。

---

## 5. 本批負向證明（修前必紅、修後符合語意）

- 修前（把 16 處原始碼還原成 `origin/main`、測試保留）：`internal/portfolio`／`monitoring`／`feature`／`replay`／`orchestrator`／`scheduler`／`stockpicker`／`calibration` **全部紅燈**，訊息逐列印出壞值（例：`MomentumReturn with latest close 0 = -1, want 0`、`maxDrawdown ... = 1, want 0.25`、`trained on 3 samples, want 1`、`return[1] = -1.588235294117647`）。
- 修後：同一組測試全綠。
- 還原方式：`cp` 備份 → 還原 → `sha256sum` 逐檔比對（9 個原始碼檔全數位元組相等）。
- 正常資料不變：各測試內含 clean-data 對照（硬編碼期望值），且受影響套件既有測試全數通過。

（逐字 log 見 PR body 與 `/tmp/fu20260930-04-*.log`。）

---

## 6. 後續落地：k3 決策 3-1／3-3／3-4（2026-10-17）

§3 的三項提案已裁決並落地（其餘仍只提案）。**動機**：§1 的守門只認得「非正／非有限」，而
`FU-20260930-03` 的第二類壞值是**倍數**（`5904.TW` 存成 720 而官方 72）——兩端都是可用價格，所以
§1 的守門不會擋，`+900%` 直接進入動能／波動／標籤／勝率。

### 6.1 3-1 跳空倍數門檻（`internal/pricegate`）

| 條件 | 實作 |
|---|---|
| `\|r\|` 超出門檻即壞值 | `shared.MaxPlausibleReturn(horizon)`：單日 = 50%（TW 漲跌停 10%／20% ⇒ 正常資料不可能）；長天期按 `1.5^h-1` 複利（5 日 = 659%，避免把合法的 `1.1^5-1` 61% 誤殺） |
| 新股上市首 5 日豁免 | `shared.InIPOExemptWindow(pos, horizon)`：窗口落在該檔前 5 個交易日內即豁免（截斷序列只會**放寬**，屬 fail-open） |
| 被拒列可審計 | `logging.Warn(component, "gap_multiple_rejected", symbol/pos/horizon/prev/curr/return/bound)`；豁免記 `Info(gap_multiple_ipo_exempt)`；**每 component 上限 100 行**＋1 行 `gap_multiple_rejected_suppressed` |
| 正常資料不變 | `pricegate.Check` 只回「可否使用」，**不負責除法**——各消費端保留原本算式（`(curr-prev)/prev` 或 `curr/prev-1`），故正常觀測逐位元組同值 |

接上的消費端是 §1 的 16 筆中的 **13 筆報酬消費者**（`maxDrawdown`／`price_position`／`quality_intra`
三筆是價格水準，跳空倍數門檻不適用）：`portfolio`（`MomentumReturn`／`Volatility`／
`extractReturnMatrix`）、`feature`（`return_1d`／`return_5d`／`return_autocorr`／`ForwardReturnLabel`）、
`monitoring.realizedVolatility`、`replay.Dataset.ForwardReturn`、`calibration.LoadReturnsFromCSV`、
`scheduler.extractLabels`（經 `sampleIndexes`）、`orchestrator.ml_scorer.Train`、`stockpicker` 前向報酬。
`optimizer_math.go` 的 `extractReturnMatrix` 同時吃到 3-1 與 3-4。

### 6.2 3-3 總經／即時報價 `(Last-Open)/Open`

`orchestrator.quoteChangePct` 只守**分母**：`Open` 非正／非有限 ⇒ 該觀測略過（欄位維持零值）並記
`Warn(quote_change_skipped)`。套用於 `QuotesToNarrativeData`（11 個 symbol 分支）與
`QuotesToMacroDataSnapshot`（7 個分支）。

### 6.3 3-4 `optimizer_math.go` 均值分母

`means[i] = sum / float64(Tret)` → `sum / float64(n)`（`n` = 實際累積的可用報酬數）；`n == 0` 時維持 0。
**值語意微變**：有缺值的資產，其 Markowitz 期望報酬會回到「可用樣本平均」（例如跳過 1 天時由 23/24 縮放
變回原值）。無缺值的資產不變。

### 6.4 本批負向證明（突變 ⇒ 必紅）

把 10 個消費端原始碼還原成 `HEAD`（測試保留、`internal/pricegate` 與 `shared/price_gate.go` 保留）：

| 套件 | 修前紅燈原文（節錄） |
|---|---|
| `portfolio` | `Volatility with a 10x stored close = 1.815170446542142, want 0`；`means[BAD.TW] = 0.33749999999999997`；`MomentumReturn(1) with a 10x latest close = 9` |
| `feature` | `return_1d with a 10x close = 9.091743119266056`；`label into a 10x close = 9.095238095238095` |
| `monitoring` | `realizedVolatility with a 10x stored close = 2.8774989139876315` |
| `replay` | `ForwardReturn reported success across a 10x stored close; want ok=false` |
| `calibration` | `got 8 returns [0 0 0 0 0 0 9 -0.9], want 6` |
| `scheduler` | `got 6 samples, want 4` |
| `orchestrator` | `trained on 8 samples, want 6` |
| `stockpicker` | `outcome stockpicker-foreign-3d-net-buy @ 2026-01-10 has ForwardReturn 9.476190476190476` |

還原方式：`cp` 備份 → `git checkout HEAD -- <10 檔>` → 紅燈 → `cp` 還原 → `sha256sum` 逐檔位元組相等 → 全綠。

### 6.5 已知限制（本批不動）

1. **`Last` 非正仍未被擋**（3-3 只守分母）：`Open>0 && Last<=0` 會得到 `-100%`。屬同一缺陷族，但決策只授權
   最小守門 ⇒ 留在 §3 待裁決。
2. **未調整的公司行為可超過門檻**：`MomentumReturn` 用的是**未調整**價格（見套件 doc）；減資／分割造成的
   帳面跳空若 `>50%` 會被本門檻當成壞值丟棄。這是「事後調整才有正確答案」的既有結構問題，非本門檻能解。
3. **`prevClose` 鏈汙染仍在**：`calibration` 與 `replay` 的壞值被拒後，下一列仍以壞值為分母；該列通常也落在
   門檻外而被拒（等效於跳過兩列），但沒有「跳過壞值改用前一可用值」的重建。
4. **跳過樣本後共變異數仍含 0 填充**（`optimizer_math.sampleCov` 對 `ret[t][i]==0` 視為 0 報酬）；3-4 只修均值
   分母。
5. **`cmd/experimental/**` 研究 CLI 未接門檻**（非生產路徑，§3 原就只提案）。

---

## 7. 後續落地：k3 決策 3-2 — 價格水準消費者補「非可用價＝缺值」守門（2026-10-18）

§3 的提案「單純『價格水準』消費者」已裁決**做**（低優先、獨立小 PR）。選定語意與 `FU-20260930-04`
完全相同：**非可用價（`shared.IsUsablePrice` 為 false ⇒ `<=0`／NaN／±Inf）＝缺值 ⇒ 跳過該觀測，平均除以
實際使用的筆數**；窗內若無任何可用值，沿用該函式既有的「無資料」回傳值（`0`／`1.0`）。
⇒ 只影響本來就是壞值的列，正常資料逐位元組不變（每個修動都附 clean-data 對照測試）。

那一節的另外三項（報酬類、`Open` 守門、optimizer 分母）已於 3-1／3-3／3-4 落地，**本 PR 不重做**。

### 7.1 機械式盤點（候選 → 判定）

掃描方式：`grep -rn 'Close' --include=*.go internal/ cmd/`（**1,116 行／292 檔**）→ 取同一行含價格識別字
與算術運算子（或 `+=`）者（39 行）＋ `sum|total +=` 全表（346 行）→ 逐筆讀碼判定「水準值（level）」
或「報酬值（return）」。判定**不靠印象**：每筆都讀到函式體。

| # | 位置（`origin/main` 行號） | 類別 | 判定 |
|---|---|---|---|
| 1 | `internal/portfolio/historical_prices.go:127` `SMA` | 水準（N 日均價） | **改** |
| 2 | `internal/feature/feature.go:58` `ma_ratio` | 水準（`close / MA20`） | **改**（決策點名） |
| 3 | `internal/feature/feature.go:193` `bb_pct_b` | 水準（通道中心＋寬度） | **改** |
| 4 | `internal/feature/feature.go:215` `hl_range_pct` | 水準（分母 `MA20`） | **改** |
| 5 | `internal/feature/feature.go:324` `price_position` | 水準（減 `MA20`） | **改**：分子已於 3-1 期守門，**均值未守**（窗內壞值會移動比較基準） |
| 6 | `internal/feature/feature.go:517` `emaClose`（`macd`／`macd_signal` 共用） | 水準（EMA） | **改** |
| 7 | `internal/scheduler/ml_retrain.go:214` `extractFeatures`（`:225` `meanC`／`stdC`、`:257` `ma5Close`） | 水準（z 分數基準／5 日均價） | **改** |
| 8 | `internal/stocktools/handler.go:411` `sma`（`sma20`／`sma50` 端點） | 水準 | **改** |
| 9 | `internal/feature/feature.go:19` `close` 特徵 | 水準（原值輸出） | **不改**：它輸出的就是原始觀測；樣本層已於 3-1 期跳過（`scheduler.sampleIndexes`、`ml_scorer.Train`）。改它等於把壞值偽裝成 0 特徵 |
| 10 | `internal/feature/feature.go:307` `obv` | 方向（`Close` 大小比較） | **不改**：非水準值（只取方向），屬報酬／方向類 ⇒ 另案 |
| 11 | `internal/feature/feature.go:176` `atr_14`、`:390` `rsi_14` | 區間／差分類 | **不改**：3-2 只授權水準類。`atr_14` 的 TR 用 `max()`，缺值不會放大；`rsi_14` 是差分收斂 ⇒ 屬 3-1 家族 |
| 12 | `internal/monitoring/service/session.go:226-241`（5 日高低標籤） | 水準（極值） | **不改**：已有 `b.Close > 0`／`low5 > 0` 前置；殘留邊界只有在**整個前 4 日窗皆壞值**時才出現且無實測可達性（見 §7.4） |
| 13 | `internal/domain/volume_divergence.go:107-113`（視窗高低） | 水準（極值） | **不改**：`high <= low \|\| high <= 0 \|\| low <= 0` ⇒ 回 `false`（等效缺值），壞值不會產生訊號 |
| 14 | `internal/portfolio/period_calculator.go:329` `computeSMA` | 水準（總經／指數） | **不改**：來源是 macro 快照（TAIEX／SOX／USD-TWD…），非 `quotes`；且已跳過 0 並以實際筆數為分母 |
| 15 | `internal/monitoring/risk_exclusion.go:324-341`（`maxDrawdown` 內同一模式）、`feature.go` 的 `price_position`／`quality_intra` 分子 | 水準 | **已由 3-1 期覆蓋**（`shared.IsUsablePrice`）⇒ 不重做 |
| 16 | `optimizer_math.go:96` 均值分母、`orchestrator.quoteChangePct` 的 `Open` 分母 | — | **已由 3-4／3-3 覆蓋** ⇒ 不動 |
| 17 | `cmd/experimental/**` 研究 CLI | 混合 | **不改**：非生產路徑（§3 原本就只提案） |

### 7.2 修法（8 處，逐處同 commit 補測試）

| 消費端 | 修前 | 修後 |
|---|---|---|
| `portfolio.SMA` | 窗內壞值全部加進分子，分母固定 `days` | 跳過非可用價，分母＝可用筆數；全窗無可用值 ⇒ `0` |
| `ma_ratio` | `sum += bars[j].Close`（含壞值）、`sum > 0` 才回傳，否則 `1.0` | 新增 `meanUsableClose(window)`；當前 `Close` 非可用 ⇒ `1.0`（既有中性值） |
| `bb_pct_b` | 均值與**離差平方和**都吃壞值 | 均值／`std` 只由可用值計算、分母＝可用筆數；當前 `Close` 非可用 ⇒ `0.0` |
| `hl_range_pct` | 分母是含壞值的 `MA20` | 分母改 `meanUsableClose` |
| `price_position` | 分子守門、均值未守 | 均值改 `meanUsableClose` |
| `emaClose` | 種子 SMA 與傳遞都吃壞值 | 種子＝可用值均值；傳遞跳過壞值（維持前值）；種子無可用值 ⇒ `0` |
| `scheduler.extractFeatures` | `meanC`／`stdC`／`ma5Close` 吃壞值 | `close` 的均值／標準差與 `ma5Close` 只用可用值、分母＝可用筆數；其餘欄位（`Open`／`High`／`Low`／`Volume`）維持原式 |
| `stocktools.sma` | 全部加總除以 `n` | 跳過非可用價、除以實際筆數；全窗無可用值 ⇒ `0` |

共用守門一律用**既有的** `shared.IsUsablePrice`（`internal/domain/shared/price.go`），未新增 predicate。

### 7.3 負向證明（修前必紅、修後符合語意）

把 4 個原始碼檔還原成 `origin/main`（測試保留），四個套件全紅：

| 套件 | 修前紅燈原文（節錄） |
|---|---|
| `portfolio` | `SMA(5) over [0 106 107 108 109] = 86, want 107.5`；`with a -5 close = 85`；`with a NaN close = NaN` |
| `feature` | `ma_ratio with a stored 0 close in the window = 1.1442307692307692, want 1.0870192307692308`；`ma_ratio with current close -5 = -0.0484027105517909, want 1.0`；`bb_pct_b ... = 0.3000379273290907, want 0.7957368009704224`；`hl_range_pct ... = 0.03836930455635491, want 0.03645083932853717`；`price_position ... = 0.12394957983193278, want 0.07784166298098182`；`macd on a 100-per-day series with one missing close = 2.415028855210764, want 0`（一個缺值造出負值趨勢訊號） |
| `scheduler` | `row[3] (close z-score) with a stored 0 close = 0.38615176976646903, want 0.7066562511750967`；`row[6] (close / 5-day mean level - 1) = 0.2735849056603774, want 0.018867924528301883` |
| `stocktools` | `sma over [100 101 0 103 104] = 81.6, want 102`；`[100 101 -5 103 104] = 80.6` |

修後：同一組測試全綠，且四套件既有測試（`go test ./internal/portfolio/ ./internal/feature/ ./internal/scheduler/
./internal/stocktools/`）全數通過 ⇒ clean-data 對照（每個修動都有一條 `..._CleanDataUnchanged`）證明
**正常資料數值不變**。

還原方式：`cp` 備份 → `git checkout origin/main -- <4 檔>` → 紅燈 → `cp` 還原 → `sha256sum` 逐檔位元組相等 → 全綠。

（逐字 log 在 PR body 與 `/tmp/f72-price-level-*.log`。）

### 7.4 已知限制（本批不動）

1. **`session.go` 的 5 日高低標籤**：只有「前 4 日窗全部壞值」時 `high5` 會停在初值而使任何正收盤被標成
   `創5日高`。需要同檔連續 4 個壞收盤才會出現，`FU-20260930-03` 的 109 列分佈在 32 檔、無連續證據
   ⇒ 只列入清理單，不在本 PR 猜修。
2. **`obv`／`rsi_14`／`atr_14` 仍吃壞值**（方向／差分／區間類，非水準類）⇒ 屬 3-1 家族的下一批，需另案裁決。
3. **`close` 原始特徵不變**：呼叫端若沒走 `sampleIndexes`／`Train` 的樣本過濾，特徵仍是壞值本身（設計如此）。
4. **10 倍價（`5904.TW` 型）在水準類仍會穿透**：本守門只認「非正／非有限」，倍數值仍是可用價格
   ⇒ 仍須 `FU-20260930-03` 的資料修復（operator 動作）。
5. **跳過後不均勻加權**：視窗內少 1 筆可用值時，平均值等於「其餘觀測的等權平均」，不是「往回多取 1 筆」
   ⇒ 與 3-4 的處理一致（分母＝實際樣本數），刻意不回溯補值。
