# JEV-EVAL-STAGE3-EVENTS — Jev 對預測是否有用：Stage 3（事件層 E0）

| 項目 | 內容 |
|---|---|
| 問題 | 「Jev 的判斷，對 **台股排程事件（事件層）**發生後的 5 交易日、扣成本後 forward 方向，是否攜帶資訊？相對平台既有的**事件先驗**是否有增量？」 |
| 階段 | Stage 3（E0：**shadow 評估，不接任何生產決策路徑**）。E2（接線）不在本階段範圍 |
| 框架 | [`JEV-EVAL-FRAMEWORK.md`](JEV-EVAL-FRAMEWORK.md)（可重用評估框架；本階段**未修改框架本體**） |
| 前階段 | [`JEV-EVAL-STAGE1-INDUSTRY-L1.md`](JEV-EVAL-STAGE1-INDUSTRY-L1.md)（產業層；結論 **未驗證**） |
| Issue | [#1968](https://github.com/kaecer68/atlas-go/issues/1968)（本階段追蹤）；日曆缺陷另開 [#1973](https://github.com/kaecer68/atlas-go/issues/1973)（見 §1.3） |
| 日期 | 2026-09-25 |
| 模型 | `jev-1.13.0`（pin 版本 ID，非 alias） |
| **分級結論** | **未驗證（pooled AUC CI 排除 0.5 且方向為負，但被洩漏上限降級）** |
| 樣本數 | 1656 cases（92 個 event-anchor × 18 個 canonical L1）· held-out 630 cases（25 個交易日）· 62 個 cluster |
| 成本 | **$0.0297**（92 requests、707,999 input tokens @ $0.042/Mtok，H=5 主配置）；本階段含廢棄迭代**實際支出 $0.1346** |

---

## §0 動機：為什麼事件層，以及為什麼**不是**「事件窗口內的每一天」

Stage 1 導出「事件層比個股層便宜」的理由是「事件層已有現成骨架」。實際動手後，這個骨架有兩件事必須先處理，否則評估會變成無效實驗：

1. **骨架必須是可程式重生成的真值，不是快照。** 本機 `event_calendar_history`（1,276 列）**全部是 `is_synthetic=1` 的回填列、且 2021 年 0 列**（見 §1.3）。因此事件日期 GT **不取自該表**，而是取自 `internal/industry/event_calendar.go` 的**決定性年份規則**（`RefreshEvents` 只依賴年份，不用網路/DB），由一支新的 Go 匯出器吐成 JSONL。一行指令即可重建。

2. **「事件窗口內的每一天」在本平台上等於「每一天」。** 以 `DetectActiveEvents` 的語意（`dateInRange(now, StartDate, EndDate)`）計算，2021 視窗內 **180/180 個交易日全部落在至少一個事件窗口內**——因為 `buildMonthlyEvent` 給 `futures_settlement` / `investor_conference` 的窗口是**整個月**（`start=當月1日, end=當月末日`）。用「窗口內日」當 case 單位，條件化就是空的（futures_settlement 一項就覆蓋 180/180 天）。

   因此本階段的 case 單位是 **事件錨點**：一個事件有 ≤3 個**排程上明確、事前已知**的決策日——窗口開啟日（`start`）、事件高峰日（`peak`）、窗口結束日（`end`）。這也直接對應題目的「**事件後** [0,5]／[0,20] 交易日的 forward return」。

> **預先指定（在看到任何結果之前寫定）**：主要指標為 **pooled AUC**（全部 anchor 種類合併），次要為 **同日跨產業排名 AUC**，並**同時**報告三種 anchor 種類的分解，以暴露同一事件三個錨點之間的非獨立性。**不得**在看到結果後改選指標。

---

## §1 資料：GT 用什麼、以及**排除了什麼**

### 1.1 事件骨架（可程式重生成的一行指令）

```bash
go run ./cmd/experimental/jev-eval-events \
  -dir data/state/sector_index -start 2021-01-04 -end 2021-12-30 \
  -anchor peak -out /tmp/events_peak.jsonl -adjustments-out /tmp/adjustments.jsonl \
  -summary-out /tmp/events_peak_summary.json
```

- **日期來源**：`industry.NewEventCalendar()` + `RefreshEvents(<year>)` → `defaultEventRules()`（14 條規則、22 種 `TaiwanEventType`）。逐年份實例化，因此輸出與迭代順序無關，且**同輸入 → 同 bytes**（`TestRun_ExportsDeterministicSkeleton` 守住）。
- **錨點政策**：`anchor_date` = 標稱日（`-anchor peak|start|end`）**當天或之後的第一個「價格宇宙存在的交易日」**。理由：forward return 從錨點收盤起算，錨點必須是可交易且有資料的交易日；標稱日落在週末/假日時往後滾動，`anchor_shift_days` 記錄滾動的**日曆天數**（0 = 標稱日本身就是交易日），`nominal_anchor_date` 保留原標稱日以供稽核。
- **匯出內容**（每列一個 event occurrence）：事件 id／型別／名稱／方向／基礎權重／衰減天數／受影響產業、窗口三日期、錨點三欄位、窗口內位置（`sessions_from_window_start`、`sessions_anchor_to_window_end`）、年份。
- **2021 實測**：每年產生 61 個 occurrence，品質閘門後匯出 `start` 33／`peak` 25／`end` 34 列（**2021 視窗內**，見 §1.3 閘門）。

另一份產物（同一支匯出器的 `-adjustments-out`）是**平台自己的事件層決策變數**：對每個（交易日 × canonical L1）呼叫 `industry.EventCalendar.GetEventAdjustment(industryID, t)`，不是複製一份演算法。它的產業母體**刻意取價格宇宙的 18 個 id**（平台的 sector id 宇宙是 20 個，含 `chemicals`/`tourism`，這兩個在 2021 沒有任何價格列）。

### 1.2 forward return GT（口徑**沿用 Stage 1 的既有權威**，未另立）

事件層**不重新實作報酬計算**。另一半 GT 就是 Stage 1 的 canonical L1 panel，在同一個（日期 × canonical L1）粒度上：

```bash
go run ./cmd/experimental/jev-eval-panel \
  -dir data/state/sector_index -start 2021-01-04 -end 2021-12-30 -min-history 60 \
  -forward-days 5  -out /tmp/panel5.jsonl  -summary-out /tmp/panel5_summary.json   # 主配置
  -forward-days 20 -out /tmp/panel20.jsonl -summary-out /tmp/panel20_summary.json  # 敏感度
```

Python task spec（`scripts/jev_eval/specs/event_calendar.py`）把兩份檔案**以錨點日期 join**：

| GT 要素 | 值 | 既有路徑（口徑權威） |
|---|---|---|
| 命中判定 | `forward_return − cost_rate > 0`（嚴格大於） | `internal/stockpicker/winrate.go` `NetHit` |
| 持有期 | 5 交易日（主）／20 交易日（敏感度） | `internal/stockpicker` `DefaultForwardDays = 5` |
| 成本率 | 0.585% | `configs/parameters.json` → `stockpicker.costs.round_trip_pct`（CLI 讀入，不硬編） |
| 聚合／CI | Wilson 95% / min_samples 30 | `internal/stockpicker/industry_winrate.go`（`docs/specs/industry-hitrate-metric-spec.md` §1） |
| 標的序列 | canonical L1 產業指數 | `internal/marketdata/sector_index_reader.go`（18 產業 native schema 優先） |

- **主配置（H=5）**：`panel5.jsonl` 3240 列 / 180 交易日 / 18 產業，與 Stage 1 完全同一份（同指令、同視窗）。join 後：**92 個 anchor → 1656 cases**，錨點日期 2021-04-12 … 2021-12-20。
- **敏感度（H=20）**：`panel20.jsonl` 2970 列 / 165 交易日；join 後 **84 個 anchor → 1512 cases**。
- **join 落空（誠實紀錄）**：H=5 有 55 個 occurrence-anchor 的錨點日不在 panel 視窗內（前 60 個交易日的 trailing 歷史與末端 forward 需求造成），已記錄在 spec `meta.dropped`。

> **口徑註記（誠實揭露）**：與 Stage 1 相同，GT 的 forward return 是**產業指數報酬**，不是成分股等權平均；event anchor 只是把「哪一天」條件化，序列本身沒有改變。因此本層測的是「**同一條產業指數序列在事件錨點上的方向**」，不是「事件對個股的衝擊」。

### 1.3 **排除與品質閘門**（沿用 Stage 1「先排除並記錄」的紀律）

匯出器內建 `-quality-gate`（預設開；`-quality-gate=false` 可關以便稽核被丟掉什麼），計數一律寫進 summary。2021 年被閘門擋下的 26 個 occurrence：

| 閘門 | 2021 擋掉 | 證據 |
|---|---|---|
| `peak_outside_own_window` | 14（futures_settlement 11、investor_conference 3） | `buildMonthlyEvent`（`internal/industry/event_calendar.go:982`）取 `rule.ComputePeakDate(year)`——**年份固定**而非該月。於是 12 個 `futures_settlement` occurrence 的 `PeakDate` **全部是 2021-06-16**（其中 11 個落在自己窗口之外），4 個 `investor_conference` 全部是 2021-07-15。**peak 錨點因此不可用**（同一日期被重複計 12 次會嚴重扭曲 pooled 樣本）。 |
| `inverted_window` | 4（position_building） | `buildPositionBuildingEvent`：`StartDate=lastWeekStart(month)`、`EndDate=lastTwoWeekStart(month)−1`，2021 產生 `StartDate > EndDate`（例：`2021-06-24 .. 2021-06-16`）。`DetectActiveEvents` 永遠不會回報它 active ⇒ 該 occurrence 整體不可評估。 |
| `unverified_lunar_calendar` | 8（全部 `long_holiday`） | 農曆表只驗證到 `industry.GetLunarCoverageYears() = 2023..2040`；2021 走**慣例 placeholder**（春節→2/1、清明→4/5、端午→6/10、中秋→9/20）。實測 2021 春節真值 2/12、端午 6/14、中秋 9/21 ⇒ 移動型假日日期不可信。因無法從套件外區分「農曆推導」與「固定日期」假日，**保守起見整型別排除**；固定日期者（元旦/228/勞動節/國慶日）為附帶損失，代價僅 4 列。 |

被排除的**資料源**：

| 來源 | 排除理由 |
|---|---|
| `data/state/sector_index/` 2026 段（89 檔、8 產業、含 24 個週末日且報酬非零） | **不作為 GT**。註記：本階段獨立稽核**修正 Stage 1 的判定用詞**——這段的**數值本身是真的**（半導體水位 58/61 筆與 `macro` 的 `taiwan_semi_index` 逐筆相同、level corr 0.9667），壞掉的是**日期軸**（週末列、重複日期、缺 12 個平日）。結論不變（排除），但原因由「合成資料」更正為「日期軸污染的不得用資料」。 |
| `data/state/sector_index/` 多日檔 `sector_indices_20260701_20260710.json` | 10 個日期共用同一組數值（162/162 筆零變化卻有非零報酬）⇒ 明確偽造。 |
| `event_calendar_history`（本地 SQLite） | 1,276 列**全部 `is_synthetic=1`**，`captured_at` 2026-08-24/25，全部 event id 帶 `_backfill_`，**2021 年 0 列** ⇒ 只能當交叉檢查，**不可**當 2021 事件日期真值。 |
| `taiwan_index_history.json`、`macro/`、`sessions/`、`capital_flow/`、`sbl/`、`tdcc/`、`quotes`、`live/` | 無 2021 覆蓋（`taiwan_index_history.json` 只有 21 筆 2026 值；`quotes` 表 0 列）。 |
| `data/replay/*`（`tw_combined`、`atlas_combined`、`tw_main_dataset`） | `source` 標記為 `simulated` / `generated_extended_data`。 |

**唯一可用的 2021 指數序列**：`sector_index` 的 244 個交易日（2021-01-04..2021-12-30，18 產業，0 個週末列；15 個缺席平日全部是 2021 台股假日）。已用四條獨立資料血緣（margin / taifex_oi / finmind replay / sector_index）交叉驗證日期集合一致。

---

## §2 Baseline（具名）與可得性

| baseline | 路徑 | 2021 可得？ |
|---|---|---|
| `unconditional_always_up` | 無條件參考：常數分數（0.5）。在框架的 baseline 決策規則（`score > 0`）下這**就是**「一律看多」策略，故其 precision = base rate、AUC 恆為 0.5 | ✅ |
| `event_direction_prior` | **平台自己的事件方向先驗**：`CalendarEvent.Direction` 經 `computeSentimentAdjustment` 的符號表（bullish +1／bearish −1），再乘 `GetEventAdjustment` 的相關性權重（受影響產業 1.0、其餘 0.3 外溢）。`mixed`/`neutral` 在平台上算出的就是 0，因此先驗**棄權（無值）**而不是硬編一個方向 | ✅ |
| `platform_event_adjustment` | **平台自己的事件層決策變數**，直接呼叫 `industry.EventCalendar.GetEventAdjustment(industryID, t)` 匯出（非重寫）：`mean(active events) of BaseWeight × dirMul × 線性衰減，clamp ±0.05`。峰值窗（±`DecayDays`）之外為 0 ⇒ 遠離峰值的錨點上此 baseline「沒有看法」（0），這是平台自己的立場而非缺值 | ✅ |
| `industry_momentum_20d` / `_5d` | 同一條 canonical 產業指數序列的 20／5 交易日 trailing 報酬（重建的持續性 baseline，非已接線的生產決策路徑） | ✅ |
| `platform_eventdriven_t1_capital_flow_direction` | `internal/eventdriven` 的 T+1 資金流方向預測。它以 **H6 指標**（`docs/specs/industry-hitrate-metric-spec.md:79`「事件流方向命中」；實作 `internal/eventdriven/handler.go` `computeHistoricalHitRate`，`hitRateWindow=60`、`MinHitSamples=30`）評分，**不是命中率**，因此本來就不可與 AUC 直接同尺度比較；且 **2021 沒有任何 PIT 預測序列**（`data/state/event_flow_predictions.jsonl` 僅 30 列、2026-07-14..2026-09-18；`prediction_backtest` 表 0 列） | ❌ **unavailable**（框架明文報 `available:false`，**不用近似品冒充**） |

> 同一次稽核確認 Stage 1 的 `platform_industry_hitrate_tilt` 在 2021 同樣不可得（其輸入 `stock_signal_outcomes` 最早 2026-05-11，2021 年 0 列），本階段未重跑該 baseline。
>
> `unconditional_always_up` 的 AUC **恆為 0.5**，故 `ΔAUC vs 它` 就等於 `AUC − 0.5`；它同時讓 `filter` 區塊退化為「Jev 自己在全樣本上的篩選」，可讀性有限，主要用途是**明確標定無資訊基準線**。

---

## §3 State 與題目設計（PIT）

- **單位**：一題 = 一個（event occurrence × canonical L1）；**一個 occurrence 的 18 個產業合成一個 request**（fan-out）。`group_id` = **錨點交易日**（跨標的、跨事件同日同 cluster）。
- **State（僅 PIT）**：
  - `event` 區塊：id／型別／名稱／描述／direction／base_weight／decay_days／受影響產業、窗口三日期、`anchor_kind` 與標稱日、`anchor_shift_days`、窗口內位置。**全部是排程資訊**——`RefreshEvents` 只依賴年份（唯一 wall-clock 用法是 `generatedAt` 這個不入 state 的欄位），事件日期事前已知，不含任何事件後資料。
  - `industries` 區塊：每產業 `trailing_return_{5,20,60}_sessions_pct`、`realised_vol_20_sessions_pct`、`below_60_session_high_pct`、`session_return_pct`——與 Stage 1 **同一批特徵、同一份 panel**，全部只由 date ≤ t 的報酬推導。
- **題目**（`noul`，一題一個窄判斷）：`over the NEXT 5 trading sessions …, will this industry index return be > 0 after subtracting the 0.585% round-trip transaction cost?`，並附 `criteria{true,false}` 把邊界寫死。
- **GT 不進 state**：`CaseView` 結構上不含 `gt`；新增自檢 `test_event_state_hides_ground_truth` 對 state 的 JSON 文字直接斷言不含 `forward`/`backward`/`hit`/`net_return`。
- **洩漏探針**（`mode=leakage`）：同標的、同錨點，但問「**結束於**錨點的過去 N 個交易日是否上漲」，且 state **既不含價格、也不含事件細節**（只有產業 id/名稱與日期）——事件細節會給出方向提示，會把「記得序列」與「照先驗猜」混在一起，故刻意移除。
- **門檻**：只用校準集（時間序前 60% 交易日，共 39 個）呼叫 `jevkit.calibrate_threshold`。

---

## §4 結果（`jev-1.13.0`，2021 視窗）

### 4.1 分級結論（主配置 H=5，pooled）

> **未驗證** — pooled AUC **0.3995 [0.3322, 0.4774]** 的 CI **排除 0.5 且方向為負**（未加洩漏上限時框架判 `已更正`），但洩漏探針下限 > 0.5 觸發 §5.2 第 4 條的洩漏上限，結論降為 **未驗證**。

- 樣本：**held-out 630 cases（25 個交易日）** · base rate **45.71%** · answered fraction 1.00 · cluster 62
- 主要指標 Jev pooled AUC **0.3995 [0.3322, 0.4774]**；次要指標同日排名 AUC **0.4268 [0.3726, 0.4834]**（25 個可用日）
- 校準門檻 **0.67**（min_precision 0.55，校準集 base rate 35.19%）在 held-out 只有 **5 個正向呼叫** → 操作點**不可用**（`n/a*`）
- 成本：**$0.0297**（92 requests / 707,999 input tokens）；洩漏探針 +$0.0088
- 洩漏探針：AUC **0.571 [0.5004, 0.6206]**（288 cases／31 cluster）→ 下限**僅超出 0.5 約 0.0004**，屬**邊界案例**（見 §4.5）

### 4.2 分級結論（敏感度 H=20，pooled）

> **未驗證** — AUC **0.3899 [0.3168, 0.4832]**（未加洩漏上限時同為 `已更正`），洩漏探針 **0.5722 [0.5064, 0.6186]** 同樣觸發降級。

- 樣本：held-out 558 cases · base rate 44.44% · 校準門檻 0.67 → held-out **2 個**正向呼叫（操作點不可用）
- 兩個持有期**方向一致且皆顯著為負**（ΔAUC vs `unconditional_always_up`：H=5 −0.1005 [−0.1678, −0.0226]；H=20 −0.1101 [−0.1832, −0.0168]）

### 4.3 相對 baseline 的 lift（主配置 H=5，held-out 630 cases）

| baseline | n | AUC | AUC(同日) | **ΔAUC [CI]** | ΔAUC(同日) [CI] |
|---|---|---|---|---|---|
| Jev（主要） | 630 | **0.3995** | 0.4268 | — | — |
| `unconditional_always_up` | 630 | 0.5000 | 0.5000 | **−0.1005 [−0.1678, −0.0226]** | **−0.0732 [−0.1274, −0.0166]** |
| `event_direction_prior` | 234 | 0.3964 | 0.4849 | +0.0031 [−0.1219, 0.1081] | −0.0581 [−0.1172, 0.0088] |
| `platform_event_adjustment` | 630 | 0.4200 | 0.4976 | −0.0205 [−0.1538, 0.0988] | **−0.0708 [−0.1253, −0.0126]** |
| `industry_momentum_20d` | 630 | 0.4741 | 0.5177 | −0.0746 [−0.1778, 0.0180] | **−0.0909 [−0.1474, −0.0370]** |
| `industry_momentum_5d` | 630 | 0.4355 | 0.4442 | −0.0361 [−0.1015, 0.0361] | −0.0173 [−0.0630, 0.0324] |
| `platform_eventdriven_t1_capital_flow_direction` | – | – | – | unavailable | – |

**讀法（不誇大）**：

- Jev **沒有**在事件層帶來任何正向增量。相對「無條件」參考其 ΔAUC 顯著為負；相對平台自己的事件先驗（`event_direction_prior`）與平台自己的事件調整量（`platform_event_adjustment`）**都未勝出**（ΔAUC 點估計皆為負或約 0，CI 含 0 或全負）。
- **但**：「Jev 顯著比隨機差」與「Jev 已證實無效」是**不同**命題。本階段只能說**未偵測到正向訊號**，且**未加洩漏上限時的讀法是「顯著差於隨機」**。不得寫成「已證實 Jev 無效」（§4.5 分級紀律）。
- `event_direction_prior` 只有 n=234：`mixed`/`neutral` 事件（`ex_dividend`、`monthly_revenue`、`msci_rebalance`、`tw50_rebalance`、`financial_report`……）依設計棄權，故該 baseline 只在有方向宣告的 case 上有值；這些 case 上 Jev 與它為平手（ΔAUC +0.0031）。

### 4.4 Anchor 種類分解（**事前指定**的次要分析；暴露同一事件三個錨點的非獨立性）

| anchor | cluster | held-out cases | Jev AUC [CI] | 同日 AUC | 洩漏探針 [CI] | 分級 |
|---|---|---|---|---|---|---|
| `start`（窗口開啟） | 28 | 216 | **0.3754 [0.2815, 0.4793]** | 0.4255 | 0.5183 [0.3394, 0.6478] | **已更正**（CI 上界 < 0.5、探針未偵測記憶、且通過 200 case 覆蓋閘門） |
| `peak`（事件高峰） | 22 | 162 | 0.4267 [0.3000, 0.5808] | 0.4181 | 0.5679 [n/a, n/a] | 未驗證（**覆蓋閘門未過**：held-out 162 < 200） |
| `end`（窗口結束） | 26 | 234 | 0.4451 [0.3290, 0.5568] | 0.4284 | 0.5988 [0.4556, 0.6025] | 未驗證（CI 含 0.5） |

- 三者**方向一致**（全部 < 0.5），但只有 `start` 單獨通過覆蓋閘門並給出 `已更正`。
- ⚠ **統計限制（必須讀）**：同一事件的 `start`/`peak`/`end` 三個錨點共用同一個事件、且部分 forward 窗重疊（例：`msci_rebalance` 的窗口只有 ±3 天）。**以交易日為 cluster 的 bootstrap 不會捕捉這種「同事件內」的相關性**，因此 pooled 的 CI 偏窄、有效自由度被高估。分解表就是這個風險的暴露方式；若要求嚴格獨立性，只能看 `peak` 一欄（而它單獨的樣本不足以過閘門）。
- `peak` 的洩漏探針 CI 退化為 `[None, None]`（該子集的 bootstrap 重抽多數樣本統計量為 `None`），故其分級理由只能是覆蓋閘門，不是「沒有記憶」。

### 4.5 洩漏探針（backtest 特有風險）

| 探針 | cases / cluster | base rate | AUC [CI] | 判讀 |
|---|---|---|---|---|
| H=5 pooled | 288 / 31 | 36.46% | **0.571 [0.5004, 0.6206]** | 下限 > 0.5 → **觸發洩漏上限** |
| H=20 pooled | 270 / 28 | 48.89% | **0.5722 [0.5064, 0.6186]** | 同上 |

- Jev 為 2026-09 釋出、評估視窗為 2021 ⇒ 回測天然有「記得該序列」的風險。本探針問的是**過去** N 日的方向，state 既無價格也無事件細節，唯一答題來源是**先驗知識**。
- **誠實揭露**：H=5 探針的 CI 下限只比 0.5 高 **0.0004**，這是**邊界案例**。框架規則（§5.2 第 4 條：探針 CI 下限 > 0.5 即降級）已照套；但「模型真的記得 2021 這批產業序列」的證據強度**弱**。合併另一個事實——pooled AUC 是**負的**（0.3995），而探針是正的話——兩者指向的不是同一種偏誤，這也支持「降級」而非「把負向結果當成乾淨結論」。
- 因此本階段的立場是：**既不宣稱正向訊號，也不宣稱已證實無效**，而是記錄**未驗證**並把兩個讀法一併寫出來。

---

## §5 成本（實際支出，含廢棄迭代）

| 迭代 | requests | input tokens | 成本 | 狀態 |
|---|---|---|---|---|
| H=5 判定（第 1 版） | 125 | 961,495 | $0.0404 | **廢棄**（匯出器欄位語意修正導致 state 變更） |
| H=5 洩漏探針（第 1 版） | 65 | 294,840 | $0.0124 | **廢棄**（同上） |
| H=5 判定（第 2 版，欄位修正後） | 125 | 961,495 | $0.0404 | 其中 92 列以 JSONL 快取被最終版重用 |
| H=5 判定（最終版，品質閘門後） | 92 | 707,999 | $0.0000（全數命中快取） | 主配置 |
| H=5 洩漏探針（最終版） | 46（30 新打） | 136,080（新打） | $0.0057 | 主配置 |
| H=20 判定 + 探針 | 84 + 43 | 652,580 + 197,413 | $0.0357 | 敏感度 |
| **合計** | – | **3,203,903** | **$0.1346** | – |

- **主配置（H=5）的邊際成本 = $0.0354**（92 requests / 707,999 tokens 判定 + 46 requests / 208,656 tokens 探針，後者含 16 列快取）。
- **流程檢討（可稽核）**：$0.0528 花在一個**匯出器欄位語意修正**（`anchor_shift_sessions` → `anchor_shift_days`）之後被重打的 requests 上。教訓：**先凍結 GT 骨架 schema，再開始花錢**；本次先行跑了 125 requests 才發現欄位語意有歧義。重跑完全靠框架的「JSONL 即快取」機制（`(request_id, fingerprint)`）救回，第 3 版判定 0 次呼叫。
- 單價：$0.042/Mtok（`experiments/008-jev-systemone` 實測值；僅計 input token）。

---

## §6 交付物

| 檔案 | 角色 |
|---|---|
| `cmd/experimental/jev-eval-events/`（`main.go` + `main_test.go`） | 事件骨架匯出器（決定性、無價格資料）＋平台事件調整量匯出＋品質閘門；單元測試含決定性、anchor 滾動、窗口位置算術、端到端 |
| `scripts/jev_eval/specs/event_calendar.py` | 事件層 task spec（join 兩份 Go 產物；PIT state；6 個具名 baseline；judge/leakage 兩模式） |
| `scripts/jev_eval/selfcheck.py` | 新增 7 項事件層自檢（GT 不進 state、決定性、探針 state 無價格也無事件細節、探針 GT 取自 backward 窗、方向先驗在 neutral/mixed 棄權、常數無資訊 baseline、不可得 baseline 報 null 而非替身） |
| `scripts/jev_eval/cli.py` | 註冊 `event_calendar` spec |
| `scripts/ci/check_jev_eval.sh` | CI 閘門加入 `go test ./cmd/experimental/jev-eval-events/` |
| 本文件 | 階段報告 |

**不在本階段範圍**：不接生產決策路徑（E2 另議）、不改任何 config 預設值、不動 production、未合併 PR（交 root 驗收）。

---

## §7 一行指令重建 GT（端到端可重跑）

```bash
# 1) 事件骨架（事件日期 GT；決定性、無價格資料）
go run ./cmd/experimental/jev-eval-events -dir data/state/sector_index \
  -start 2021-01-04 -end 2021-12-30 -anchor start \
  -out /tmp/events_start.jsonl -adjustments-out /tmp/adjustments.jsonl
# …對 peak / end 各跑一次（-anchor peak / -anchor end）

# 2) forward return GT（canonical 口徑；沿用 Stage 1 匯出器，不變）
go run ./cmd/experimental/jev-eval-panel -dir data/state/sector_index \
  -start 2021-01-04 -end 2021-12-30 -min-history 60 -forward-days 5 -out /tmp/panel5.jsonl

# 3) 收集（會呼叫 Jev；shadow-only）→ 評分 → 報告（後兩者零成本、可離線重算）
python3 scripts/jev_eval/cli.py all --spec event_calendar --out-dir /tmp/run \
  --spec-arg events=/tmp/events_start.jsonl,/tmp/events_peak.jsonl,/tmp/events_end.jsonl \
  --spec-arg panel=/tmp/panel5.jsonl --spec-arg adjustments=/tmp/adjustments.jsonl \
  --leakage-run-dir /tmp/leak
```

---

## §8 未完成與未驗證項（誠實清單）

1. **`已更正` 只在未加洩漏上限時成立**。框架的降級規則把最終分級定為 `未驗證`；兩者並存於本文件，**不得**只引用其中一個。
2. **洩漏探針是邊界案例**（下限 0.5004）。未做的是：把探針換成「問 2021 之外的年份」或「問合成序列」來建立對照；也未做探針的非決定性重複（`--repeat`）。
3. **同一事件的三個錨點不獨立**，日期 cluster bootstrap 不捕捉此事；因此 pooled CI 偏窄。未做的是：以 event occurrence 為 cluster 的替代 bootstrap（會違反框架「同日同 cluster」的規定，需先修規範）。
4. **`peak` 錨點單獨樣本不足**（held-out 162 < 200 覆蓋閘門），因此「事件高峰當天」這個最貼近交易直覺的時點**沒有**獨立可用的結論。
5. **只評估了 2021**。本地沒有 2022–2025 的 canonical 指數序列（`quotes` 表 0 列、`sector_index` 缺該段），所以無法做跨年度穩健性檢驗；這是**資料缺口**，不是設計選擇。
6. **`platform_eventdriven_t1_capital_flow_direction` 未量測**（2021 無 PIT 序列）。要量它必須先有 `prediction_backtest` / `event_flow_predictions` 的歷史，屬另一個工作流。
7. **`internal/industry` 的三個日曆缺陷未修**（year-fixed peak、inverted window、2023 前的農曆 placeholder）→ 已開 **#1973** 記錄並附重現指令。本階段只做**排除＋記錄＋開 issue**，不動 `internal/industry`（避免與正在併入的 #1965 分支衝突）。任何消費 `EventCalendar.PeakDate`（含 `toRawEvent` 的 `EffectiveDate`）或 `GetEventAdjustment` 的程式都可能受影響。
8. **未做 non-degeneracy 檢查**：Jev 在事件層的 ECE 0.2166、且分數集中在低區間，表示其機率分佈在此任務上系統性偏移；門檻因此被校準到 0.67 而覆蓋率僅 0.8%。這與 Stage 1 的觀察一致（Jev 不適合時序預測本身），但**不是**本階段的結論，只是觀察。
