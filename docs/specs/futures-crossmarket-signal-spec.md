---
title: 期貨跨市場訊號規格（階段 1：影子評估，不接線）
status: draft
updated: 2026-09-28
owner: futures / marketdata
related:
  - docs/specs/futures-bars-firstparty-spec.md（階段 0：期貨日行情資料層）
  - docs/reference/product-positioning.md §8（heuristic 一律經過驗證管道）
issue: "#2111（階段 0–1）；#2110（階段 2：可下單／接線）"
---

# 期貨跨市場訊號規格（階段 1：影子評估，不接線）

> **一句話**：本階段把期貨市場的跨市場特徵算出來、對「隔一交易日報酬符號」做**影子量測**，
> 寫進**獨立的**影子儲存並產出命中率。**它不產生交易決策，也不接進任何引擎路徑。**

| 項目 | 內容 |
|---|---|
| 文件角色 | 階段 1 的權威規格（特徵定義、標籤定義、係數紀律、影子儲存、測試與突變釘子） |
| 基準 | `origin/main` @ `1572cef6`（階段 0 已併） |
| 交付 | `internal/futures`（純函式）、`internal/ledger`（影子儲存）、`cmd/futures-shadow-eval`、本規格 |
| 前置 | 階段 0 的 `futures_bars`（provider／store／backfill CLI） |

---

## 0. 硬性邊界（規範；違反即退回）

1. **不得接線**：不得把本階段的訊號接進部位規模／風控／歸因（`WeightFor`／`ApplySignal` 那條橋）。
   引擎側審計判定 **R3（long-biased 歸因會反向演化）必須先修**；接線屬階段 2（#2110）。
2. **不得改動** `internal/{sim,sectorallocation,portfolio,strategy,risk,tax,live}` 任何檔。
3. **不得在生產寫入任何資料**（含期貨 bars 的生產回補，仍等 09-29 06:00Z 之後的統一部屬窗口）。
4. **不得硬編外部經驗法則**（`product-positioning.md` §8）：每個係數都要能對散戶解釋，且必經
   「假設登錄 → 歷史校準 → 寫回 parameters.json → 追蹤 → 退化降權」。
5. **不新增 required CI check**。

---

## 1. 為什麼只做影子（問題陳述）

階段 0 交付了期貨日行情，但**沒有任何機制量測它的預測力**。若直接把它接進決策，
會在三處同時造成不可觀測的風險：

- **歸因**：R3 已知會讓 long-biased 歸因反向演化 ⇒ 訊號的貢獻會被錯誤歸因。
- **校準**：型別正確但未校準的門檻會變成「看起來有依據」的隱藏常數。
- **驗收**：沒有前置的命中率量測，就無法分辨「訊號有效」與「訊號雜訊」。

因此本階段只做**可量測的影子路徑**：算出特徵、產生影子預測、量測命中率、落庫稽核。
**量測結果不會回流到任何決策**。

---

## 2. 特徵（規範）

四個特徵全部是**純函式**：輸入 bars／外部輸入，輸出數值。不碰 DB、不打網路。

| 代號 | 特徵 | 來源 | 是否自足 | 實作 |
|---|---|---|---|---|
| **F1** | 跨月價差結構（近月 vs 遠月） | 期貨 bars（同一交易日兩個月契約的一般時段收盤） | **自足**（階段 0 已交付） | `futures.CalendarSpreadAt` |
| **F2** | OI 日變化（近月 ＋ 所有月契約合計） | 期貨 bars（`open_interest`） | **自足** | `futures.OIChangeAt` |
| **F3** | 三大法人期貨淨部位（外資／投信／自營） | `marketdata.FetchInstitutionalFuturesDaily`（TAIFEX OpenAPI） | 需外部輸入 | `futures.InstitutionalPositionFrom` |
| **F4** | PCR（put/call 量比與 OI 比） | `marketdata.FetchPCR`（TAIFEX OpenAPI `/PutCallRatio`） | 需外部輸入 | `futures.PCRFeatureFrom` |

定義細節：

- **F1**：前月＝該日 canonical（`session=regular`）**月契約**（`^\d{6}$`）中年份月份最小者；
  遠月＝次小者。`SpreadPoints = far − near`；`SpreadBP = SpreadPoints / near × 10000`；
  `|SpreadPoints| <= FlatSpreadPoints`（**參數**）⇒ `flat`；否則 `contango`／`backwardation`。
  缺近月或遠月 ⇒ `unavailable`（**不插值、不猜測**）。
  週契約（`202609W5`）與盤後列**一律排除**。
- **F2**：近月 OI＝上述前月的 `open_interest`；合計 OI＝該日所有月契約 `open_interest` 之和。
  前一日＝**上一個有資料的交易日**。首日（無前一日）⇒ `unavailable`，**不得以 0 當前值**。
  `open_interest = NULL`（上游缺值）⇒ **不計入合計**，也不得當成 0。
  趨勢標籤 `up/down/flat` 的門檻是參數（`OIFlatChangePct`）。
- **F3**：`ThreePartyNet = 外資 + 投信 + 自營`（口）；`ForeignOIChange` 需要前一日輸入，
  缺前一日時**保持 0 且不假造前值**。
- **F4**：`PutCallVolumeRatio = PutVolume / CallVolume`；`PutCallOIRatio = PutOI / CallOI`；
  比值變化需前一日輸入，缺則 `HasPrev=false` 且變化欄位保持 0。

### 2.1 本階段**不做**：基差（futures − spot）

基差需要**現貨**日序列。實查結果（2026-09-28）：

- `internal/marketdata/taiwan_index_history.go` 是**檔案型 rolling window**（`RecentCloses(n)`），
  為 tw_vol 的 Yahoo fallback 而設計，**不是可回溯多年的權威序列**；
- `internal/ledger`、`internal/repository` **沒有**大盤指數日序列表。

⇒ 基差列為**獨立後續票**（需另立 spot 序列的 scope）。若日後確認某既有序列深度足夠，
也只能作為**選用特徵**，且必須先回報**實測樣本深度**，不得假設。

---

## 3. 標籤（規範）

- **標籤＝同一契約月的 T+1 報酬符號**（自足：連續契約不需要現貨）。
  `label_return_pct = (close_m(t+1) − close_m(t)) / close_m(t) × 100`，`m` ＝ t 日的前月。
- **不得跨契約取標籤**：換倉日（前月契約隔日不再有報價）的樣本**沒有標籤**。
  理由：若用換倉後的新契約算報酬，**換倉價差會被誤讀成報酬**。
- `|label| == 0` ⇒ `neutral`（由下游略過，與 repo 既有校準器語意一致）。
- **多視野（T+5／T+20）不在本階段**：既有校準框架是 T+1 符號一致，多視野屬新機制 ⇒ 後續票。
- 標籤與預測**同向 ⇒ hit**；任一方 neutral ⇒ `hit = NULL`（不是 false）。
  `NULL` 與 `false` 的區別很重要：false 是「猜錯」，NULL 是「沒有可比較的樣本」。

---

## 4. 影子規則（透明、可對散戶解釋）

分數＝各項假設的加權和，權重**全部由呼叫端注入**：

| 項 | 假設方向 | 對散戶的一句話解釋 |
|---|---|---|
| `s1` 跨月價差 | backwardation ⇒ **+1**；contango ⇒ **−1**；flat/unavailable ⇒ 不計入 | 「遠月比近月便宜，代表市場願意為**現在**多付錢 ⇒ 偏緊、偏多」 |
| `s2` OI 變化 × 近月報酬 | `sign(ΔOI) × sign(near_return)` | 「未平倉增加且價格順向 ⇒ 有新倉進場的動能」 |
| `s3` 外資淨部位變化 | `sign(Δ外資淨部位)` | 「外資淨多增加 ⇒ 偏多」 |
| `s4` PCR OI 比變化 | `−sign(Δ PutCallOIRatio)` | 「買權賣權避險比上升 ⇒ 偏空（避險需求增加）」 |

`score = Σ wᵢ·sᵢ / Σ|wᵢ|`（**只計入可用項**，未提供的輸入不補 0）⇒ 值域 `[−1, 1]`；
`confidence = |score|`；`|score| > DirectionThreshold` ⇒ `up`／`down`，否則 `neutral`。

**這些權重與門檻都是「待校準的假設」，不是已驗證的事實。** 本階段不提供任何預設值
（見 §6），量測結果才是下一步校準的輸入。

---

## 5. 資料可得性（實測；決定哪些特徵能回溯）

| 輸入 | 可得性 | 證據 |
|---|---|---|
| 期貨 bars（F1/F2） | 可回溯多年，但**生產尚未回補**（held） | 階段 0 規格；`futures_bars` |
| PCR（F4） | **只有滾動約 19 個交易日** | 2026-09-28 實測 `GET /v1/PutCallRatio` ⇒ 19 列，`20260831..20260924`，無日期參數 |
| 三大法人期貨部位（F3） | OpenAPI **僅最新交易日**（無日期參數） | `internal/marketdata/taifex_institutional.go` 註解 |
| 三大法人期貨**歷史** | 官網 CSV（`futContractsDateDown`）可逐日抓，repo 既有 CLI 覆蓋 **2024-07 起** | `cmd/backfill-taifex-oi-v2` 檔頭註解（**本次未重新實測**，僅引用 repo 既有敘述） |

⇒ 因此 `cmd/futures-shadow-eval` 本版**只接 F1/F2**（自足），F3/F4 以 `nil` 輸入傳入
（`TermsUsed` 會如實反映未計入），並在本表記錄「要接線前必須先解決歷史深度」。
**這是刻意的誠實邊界**：不為了讓影子評估「看起來更豐富」而餵入沒有歷史的輸入。

---

## 6. 係數紀律（規範）

1. **零隱藏係數**：`internal/futures` 內**沒有任何預設門檻／權重常數**；
   `FeatureParams` / `ShadowParams` 由呼叫端注入，且**不提供 `Default*()`**。
2. **CLI 沒有預設值**：`cmd/futures-shadow-eval` 的 7 個門檻／權重 flag 全部**必填**，
   少給任一個 ⇒ 直接錯誤（訊息明示「no hidden coefficients」）。測試 `TestParseArgs_RequiresEveryCoefficient`
   會**逐一拿掉**每個 flag 並要求失敗。
3. **參數登錄不在本階段**：本階段不動 `parameters.json`（避免觸動參數治理／`validate-parameters`）；
   登錄＋校準屬後續步驟（`product-positioning.md` §8 的完整管道）。
4. 量測用的門檻（例如命中率門檻）可以寫在**測試與本規格**，但**不得**成為產品邏輯的隱藏常數。

---

## 7. 影子儲存：**獨立命名空間**（規範）

**結論：影子列寫入 `futures_shadow_predictions`（獨立表），不寫 `prediction_backtest`。**

理由（以程式碼為證，2026-09-28）：

```
internal/calibration/predictor_calibrator.go:63
    store.LoadPredictionBacktestRange(context.Background(), "", "", 90)
internal/ledger/historical_store.go:797-802（SQLite 版）
    SELECT ... FROM prediction_backtest
    WHERE (? = '' OR date >= ?) AND (? = '' OR date <= ?) AND is_synthetic = 0
    ORDER BY date ASC LIMIT ?
```

- 該查詢的兩個空字串是 **startDate／endDate**（不是 model version）；
  **SQL 完全不過濾 `model_version`**，`tryHitRateEval` 接著把所有列混算成**單一命中率**。
- ⇒ 任何外來 `ModelVersion` 的列都會**直接改變** `predictor_*` 的貝氏校準分數。
- ⇒ 因此在本階段，影子列**必須**離開該表。要合流的**前置條件**是：
  ① 校準器確實按 `ModelVersion` 分組／過濾（程式碼證據）；② 證明不可能污染；
  兩者都成立後才回來討論合流。

### 7.1 另一條可行路徑：`is_synthetic`（評估後不採用）

repo **原本就有**一個隔離機制：`internal/ledger/historical_store.go:26-27` 的
`FilterSynthetic = true` / `IncludeSynthetic = false` ⇒ 預設查詢**排除 `is_synthetic = 1`** 的列
（`LoadPredictionBacktestRange` 的 SQL 即帶 `AND is_synthetic = 0`）。
因此理論上也可以把影子列寫進共用表並標記 `is_synthetic = 1`。

**為何仍選獨立表**：

1. **語意**：`is_synthetic` 是「這列不是真實生產預測」的旗標，語意與「這是另一個模型的影子實驗」不同；混用會讓未來讀者難以分辨。
2. **不依賴他人記得過濾**：任何新查詢只要忘了帶 `is_synthetic`（或使用 `IncludeSynthetic` 變體）就會把影子列吃進來；
   獨立表在**schema 層**排除這個可能，不靠呼叫端自律。
3. **欄位自由度**：影子列需要的欄位（`terms_used`、各特徵值）與 `prediction_backtest` 的預測語意不同，
   硬塞會讓該表承載兩種模型。

⇒ 結論不變：**獨立表**。此節保留紀錄，供未來若真的需要合流時重新評估。

儲存形狀（backend-aware；`ATLAS_STORE_BACKEND` 決定，postgres 未注入 pool ⇒ **錯誤，不降級**）：

| 後端 | 實作 |
|---|---|
| `postgres`（生產） | `PostgresFuturesShadowStore` |
| `sqlite` | `SQLiteFuturesShadowStore` |
| `jsonl` | `JSONLFuturesShadowStore`（`futures_shadow_predictions.jsonl`） |

- 表：`futures_shadow_predictions`（migration `000026`；SQLite schema 在 `ledger.InitSchema`）。
- 冪等鍵：`(contract, trade_date, model_version)`。
- **`model_version` 不可為空**（否則無法與 live 校準區隔）⇒ store 層拒絕。
- 無標籤列：`label_return_pct`／`actual_direction`／`hit` 為 **SQL NULL**（不得寫 0／`neutral`）。
- 外部輸入缺值：`foreign_oi_change`／`pcr_oi_ratio` 為 **NULL**。

---

## 8. 資料閘門（規範）

`cmd/futures-shadow-eval` 在指定區間內**沒有 futures bars** 時：

- **no-op**：不報錯、不寫任何列、不產生假訊號，輸出說明原因，exit code 0。

生產在回補完成前就是這個狀態（階段 0 的回補 held 至 09-29 06:00Z 之後）。
測試 `TestRunWith_DataGateIsNoOp` 釘住此行為。

---

## 9. 測試與突變釘子

| 測試 | 釘住什麼 |
|---|---|
| `TestCalendarSpreadAt_ContangoGolden` | F1 數值（10 點、1000 bp、contango） |
| `TestCalendarSpreadAt_BackwardationAndFlat` | 方向與**門檻參數**（門檻 12 ⇒ flat） |
| `TestCalendarSpreadAt_IgnoresWeeklyAndAfterHours` | 週契約與盤後列不得成為近月 |
| `TestCalendarSpreadAt_UnavailableWithoutFarMonth` | 缺遠月 ⇒ unavailable（不合成） |
| `TestOIChangeAt_Golden` | F2 數值（近月 100 口／合計 120 口）與趨勢標籤；首日 unavailable |
| `TestOIChangeAt_MissingOINotZero` | `NULL` OI **不計入**合計（也不當 0） |
| `TestFeatureParams_Validate` | 負值／NaN 門檻被拒（函式層也驗證） |
| `TestInstitutionalPositionFrom_NoFabricatedPrev` / `TestPCRFeatureFrom_NoFabricatedPrev` | 缺前一日 ⇒ 變化保持 0、`HasPrev=false`（不假造） |
| `TestBuildShadowSeries_Golden` | 影子分數／方向／標籤／hit 的手算期望值（含 `Skipped` 計數） |
| `TestBuildShadowSeries_RollDayHasNoLabel` | 換倉日**沒有標籤** |
| `TestNextDaySameMonthReturn_DoesNotCrossContracts` | 函式級：「標籤不得跨契約」＋ 反向對照（同契約有下一日必須算得出來） |
| `TestBuildShadowSeries_WeightsAreInjected` | 方向由**注入權重**決定；門檻 1.5 ⇒ 全部 neutral |
| `TestBuildShadowSeries_MissingExternalInputsAreNotFabricated` / `_ExternalInputsAddTerms` | 缺外部輸入 ⇒ 欄位 nil 且不計入；有輸入 ⇒ `TermsUsed=4` 且值正確 |
| `TestSummarizeHitRate` | 命中率與 `Skipped` 的計算（neutral／無標籤略過） |
| `TestNewFuturesShadowStore_*` | 後端判定（#2107 形狀）：宣告 postgres 無 pool ⇒ 錯誤不降級；同環境 sqlite 可用（排除假陽性） |
| `TestSQLiteFuturesShadowStore_RoundTrip` / `TestJSONLFuturesShadowStore_RoundTrip` | 冪等、NULL 語意、版本過濾 |
| `TestFuturesShadowStore_RejectsEmptyModelVersion` | 空版本被拒 |
| `TestShadowEvalDoesNotWriteLiveCalibrationTable` | **隔離釘子**：跑完影子評估後 `futures_shadow_predictions` 有列、`prediction_backtest` **必須為 0 列** |
| `TestRunWith_DataGateIsNoOp` / `_DryRunDoesNotWrite` / `_ReaderErrorPropagates` / `_ValidatesParams` | CLI 行為 |
| `TestParseArgs_RequiresEveryCoefficient` | **零隱藏係數**：逐一拿掉每個 flag 都必須失敗 |

### 9.1 實際施作的突變（紅燈原文）

| # | 突變 | 目標測試 | 結果 |
|---|---|---|---|
| M1 | 跨月價差結構方向反轉（contango 給 +1） | `TestBuildShadowSeries_Golden` | 🔴 `d1 = 2026-01-05/up, want 2026-01-05/down` |
| M2 | 前一天缺資料時以當日充當前一日 | `TestOIChangeAt_Golden` | 🔴 `first day trend = flat, want unavailable` |
| M3 | 標籤跨契約（移除 month 過濾） | 序列級 ＋ 函式級 | 單點：**GREEN（等價突變）**；兩處同時移除：🔴 |
| M4 | 影子列**同時**寫進 live 表 `prediction_backtest` | `TestShadowEvalDoesNotWriteLiveCalibrationTable` | 🔴 `shadow evaluation must NOT write the live calibration table, found 1 rows` |

> **M3 的教訓（比紅燈更有價值）**：單獨移除 `nextDaySameMonthReturn` 的 month 過濾**不會**改變行為，
> 因為 `closeFor` 內還有第二道 month 過濾；反之亦然。這是**等價突變（equivalent mutant）**，
> 不是釘子空轉：性質由**兩道獨立防線**共同保證。
> 處置：(a) 補一條**函式級**測試把契約寫明（`TestNextDaySameMonthReturn_DoesNotCrossContracts`，
> 含反向對照以免「永遠 nil」的假綠）；(b) 以**組合突變**證明釘子會咬
> （`label=0x…` 非 nil ⇒ roll-day 測試紅）。
> **通則：突變全綠時，先分辨「釘子空轉」與「等價突變」——前者要修測試，後者要補契約級測試。**

---

## 10. 後續（不在本階段）

1. **基差（futures − spot）**：需要現貨日序列（見 §2.1）⇒ 獨立票。
2. **多視野標籤**（T+5／T+20）：需新機制（既有校準器為 T+1 符號一致）⇒ 獨立票。
3. **F3/F4 的歷史輸入**：需先解決歷史深度（§5）⇒ 獨立票。
4. **接線**（把影子訊號接進決策／部位規模）：**#2110**，且 R3 必須先修。
5. **係數登錄與校準**：`product-positioning.md` §8 的完整管道（登錄 → 校準 → `parameters.json` → 追蹤 → 降權）。
6. **影子表的新鮮度指標／告警**：同階段 0 §11.1，屬 monitoring 工作流。

---

## 11. 護欄與驗收

- 驗收：能用 `cmd/futures-shadow-eval`（自足特徵）產出影子列與命中率；
  影子列**只**落在 `futures_shadow_predictions`；無資料時 no-op；所有係數必須明確提供。
- 護欄：§0 五條。特別是本階段產物**不得**被任何決策路徑引用。
