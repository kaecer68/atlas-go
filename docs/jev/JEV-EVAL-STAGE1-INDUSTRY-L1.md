# JEV-EVAL-STAGE1-INDUSTRY-L1 — Jev 對預測是否有用：Stage 1（canonical L1 產業層 E0）

| 項目 | 內容 |
|---|---|
| 問題 | 「Jev 的判斷，對 **canonical L1 產業層**的 5 交易日、扣成本後 forward 方向，是否攜帶資訊？相對平台既有產業層訊號是否有增量？」 |
| 階段 | Stage 1（E0：**shadow 評估，不接任何生產決策路徑**）。E2（接線）不在本階段範圍 |
| 框架 | [`JEV-EVAL-FRAMEWORK.md`](JEV-EVAL-FRAMEWORK.md)（可重用評估框架本體） |
| Issue | [#1968](https://github.com/kaecer68/atlas-go/issues/1968) |
| 日期 | 2026-09-25 |
| 模型 | `jev-1.13.0`（pin 版本 ID，非 alias） |
| **分級結論** | **未驗證（no statistically distinguishable signal）** |
| 樣本數 | 3240 cases（18 個 canonical L1 × 180 交易日）；held-out 1296 cases（72 日） |
| 成本 | **$0.0456**（180 requests、1,085,311 input tokens @ $0.042/Mtok）；洩漏探針 +$0.0067；repeat=3 複製實驗 +$0.137 |

---

## §0 先驗與假設（為什麼要做這個 E0）

本專案對 Jev 的前次實測（`experiments/008-jev-systemone/RESULTS-5-USECASES.md`，2026-09-23）在「不適合」清單最後一項寫著：

> 「時序預測本身（atlas 股價/錢潮『預測』不能靠它；它只能對『既有訊號/說法』打分或判斷屬性）」

E0 是對該先驗的**可證偽檢定**，而不是要把 Jev 推上預測位置：

- **H0（先驗）**：Jev 對產業層 forward 方向沒有可用資訊。
- **H1**：Jev 在此任務上 AUC 顯著 > 0.5，且優於基於同一資料的既有 baseline。

同時 §4.4 的教訓（「輸入無資訊」會製造假陰性）要求在設計前先確認 state 是否**真的含判斷所需資訊**，因此 state 刻意放入 baseline 所用的同一批特徵（trailing returns / 波動 / 距高點），讓檢定是「Jev 能否在同樣資訊下做得更好」，而不是「Jev 能否從空 state 猜市場」。

---

## §1 資料：GT 用什麼、以及**排除了什麼**

### 1.1 採用（真實資料）

| 項目 | 內容 |
|---|---|
| 來源 | `data/state/sector_index/` 的 canonical 產業指數序列，經 `internal/marketdata.SectorIndexReader`（`ReadRange` 做 canonical L1 正規化；18 產業 native schema 優先） |
| 真實性 | 2021 年檔由 `cmd/backfill-sector-index -source finmind`（FinMind `TaiwanStockEvery5SecondsIndex`）回填；交易日分布符合台股 2021 行事曆（例：農曆年 2021-02-05 → 02-17 休市），244 個交易日 |
| 視窗 | `2021-04-09 .. 2021-12-23`（前 60 個交易日作為 trailing 歷史；末端 5 個交易日保留給 forward truth） |
| 母體 | 18 個 canonical L1（`auto, biotech, cement, construction, electronics, energy, financials, food, machinery, optoelectronics, other_electronics, plastics, retail, semiconductor, shipping, steel, telecom, textiles`） |
| 覆蓋率 | 100%（3240/3240 observations、18/18 industries、全部 `calibration_status=eligible`，每產業 180 樣本 ≫ min_samples 30） |

### 1.2 **排除**（本地 2026 資料不可用作 GT）

本機 `data/state/sector_index/` 另有 2026-06-03 .. 2026-09-16 的 89 個日檔。**不得**當成近期真實窗口，證據：

- 檔名/內容落在**週六與週日**且 return_pct 非零：`sector_indices_20260606_20260606.json`（2026-06-06 = 週六）`ai_supply_chain.return_pct = -3.63`。
- 只有 **8** 個產業（真實 18 產業 native schema），且 `2026-09-12`（週六）為空檔 0 產業。

→ 判定為**開發環境合成資料**；用它當 GT 等於量測合成訊號，會製造假結論。真實近期窗口需要生產機（Mac Mini）的 sector_index 資料，列為 Stage 1.1。

---

## §2 GT 口徑（引用既有程式路徑，不另立）

| 要素 | 值 | 既有路徑 |
|---|---|---|
| 命中判定 | `forward_return − cost_rate > 0`（嚴格大於） | `internal/stockpicker/winrate.go` `NetHit` |
| 持有期 | 5 交易日 | `internal/stockpicker/daily_update.go` `DefaultForwardDays = 5` |
| 成本率 | 0.585% | `configs/parameters.json` → `stockpicker.costs.round_trip_pct`（由 CLI 讀入，不硬編） |
| 最小樣本 | 30 | `configs/parameters.json` → `stockpicker.calibration.min_samples` |
| 信賴區間 | Wilson 95% | `internal/stockpicker/winrate.go` `WilsonScoreInterval` |
| 聚合 + 覆蓋率 | `industry_id × source × window` + `coverage` 區塊 | `internal/stockpicker/industry_winrate.go` `IndustryWinRate`（spec：`docs/specs/industry-hitrate-metric-spec.md` §1 的 H2b） |

**一行指令重生成 GT**（含 PIT 特徵與 forward 真值）：

```bash
go run ./cmd/experimental/jev-eval-panel \
  -dir data/state/sector_index -start 2021-01-04 -end 2021-12-30 -min-history 60 \
  -out /tmp/panel.jsonl -summary-out /tmp/panel_summary.json
```

輸出（實測）：`3240 rows, 18 industries, 2021-04-09..2021-12-23`，caliber `hold=5 sessions, cost=0.00585, min_samples=30, confidence=0.95`；每產業 180 obs、win_rate 0.344–0.522（`eligible`）。**整體 base rate（扣成本後看多的比例）≈ 42.8%** —— 這是任何 lift 宣稱的地板，也是「一律看多」策略的 precision。

> 口徑註記（誠實揭露）：GT 的 forward return 是**產業指數報酬**（TWSE 類股指數序列），不是「成分股等權平均報酬」。兩者都是產業層，但不可互相引用為同一數列；扣成本仍沿用 canonical 0.585%（可與 H2b 同尺度比較）。**未**另外報告未扣成本變體：那會是另一種口徑的「指標」，不屬本次預先指定的指標集。

---

## §3 Baseline（具名）與其可得性

| baseline | 路徑 | 本次 2021 視窗可得？ |
|---|---|---|
| `industry_momentum_20d` | 同一 canonical 產業指數序列的 20 交易日 trailing 報酬 > 0（資料路徑 `internal/marketdata/sector_index_reader.go`；**這是重建的持續性 baseline，不是已接線的生產決策路徑**） | ✅ 可得 |
| `industry_momentum_5d` | 同上，5 交易日（與持有期對齊） | ✅ 可得 |
| `platform_industry_hitrate_tilt` | **平台自己的產業層訊號**：`internal/sectorallocation/industry_hitrate_consume.go` + `industry_hitrate_assessment_decorator.go`（#1959），tilt = `WilsonLower − 0.5`、只吃 `eligible` 的 canonical L1 列 | ❌ **不可得**（見下） |

**為什麼第 3 條不可得（這是 Stage 2/3 的真缺口，不是偷懶）**：該 tilt 讀的是**已持久化的產業命中率列**，其來源 `stock_signal_outcomes` 在本機最早只到 `2026-05-11`（36,917 列、3 個 source、851 檔）。2021 年**沒有任何 PIT 的產業命中率列**，所以無法在 2021 視窗重建該 baseline。框架的處理是**明文 `unavailable`**（`metrics.evaluation.baselines[*].available=false`），而不是用 momentum 冒充它。

---

## §4 State 與題目設計

- **單位**：一題 = 一個（交易日 × canonical L1）；**一天的 18 個產業合成一個 request**（fan-out，官方與本方實測皆顯示批次遠優於逐題）。
- **State（僅 PIT）**：`as_of`、持有期、成本率，以及每產業的 `trailing_return_{5,20,60}_sessions_pct`、`realised_vol_20_sessions_pct`、`below_60_session_high_pct`、`session_return_pct`。全部只由 **date ≤ t** 的報酬推導（`main_test.go::TestComputeFeatures_PointInTime` 守住；forward 區塊**不在** state 內）。
- **題目**（`noul`）：`over the NEXT 5 trading sessions …, will this industry index return be > 0 after subtracting the 0.585% round-trip transaction cost?`，並附 `criteria{true,false}` 把邊界寫死（§2.1：一題一個窄判斷）。
- **GT 不進 state**：`CaseView` 結構上不含 `gt`（自檢守住）。
- **洩漏探針**（另一支 spec mode）：同標的、同日期，但問「**結束於** as-of 日的過去 5 個交易日是否上漲」，且 state **完全不含價格特徵**（只有產業 id/名稱與日期）→ 只能靠先驗知識回答。

---

## §5 結果（`jev-1.13.0`，2021 視窗）

### 5.1 分級結論

> **未驗證** — held-out pooled AUC CI `[0.4147, 0.5014]` 包含 0.5：沒有可與隨機區分的訊號。

- 樣本：held-out **1296 cases**（72 個交易日）· base rate **41.44%** · answered fraction 1.00
- 成本：**$0.0456**（180 requests / 1,085,311 input tokens）
- 洩漏探針：**未偵測到記憶訊號**（§5.4）

### 5.1.1 複製實驗（repeat=3，看到 repeat=1 結果後才跑 → 只作敏感性分析，不改主要結論）

| run | AUC（pooled） | 分級 |
|---|---|---|
| **主要**：repeat=1（首次回答） | 0.4584 [0.4147, 0.5014] | 未驗證 |
| 複製：repeat=3，首次回答 | 0.4562 [0.4131, 0.4990] | 已更正（CI 上界 < 0.5，邊界） |
| 複製：repeat=3，平均回答 | 0.4562 [0.4128, 0.4978] | 已更正（同上） |
| 複製：同日排名（repeat=3 mean） | 0.4579 [0.4244, 0.4909] | — |

- 兩次獨立抽樣的**點估計一致（0.456–0.458）**，差異只在 CI 上界是否剛好跨過 0.5。
- 因此：**「正向訊號」在任何一次抽樣都不成立**；「顯著差於隨機」只在第二次抽樣成立且屬**邊界**（約 1.7–1.9 SE），效果量極小（AUC ≈ 0.456）。
- **不**把這解讀成「Jev 是反指標」：AUC 0.456 在單一窗口上不具交易意義。主要分級仍為 `未驗證`（預先指定、repeat=1）。

### 5.2 判別力與操作點

| 指標 | 值 |
|---|---|
| AUC（pooled，**主要**） | **0.4584** [0.4147, 0.5014] |
| AUC（同日跨產業排名，次要） | **0.4582** [0.4246, 0.4907]（可用交易日 69） |
| 校準門檻（校準集 1944 cases / 108 日，base rate 43.72%） | **0.69**（目標 min_precision 0.55 → 校準集 precision 0.647、recall 0.013） |
| 操作點 | up calls **1** / 1296 → precision 1.0000、recall 0.0019、coverage 0.08% → **標記為不可用（< 30 呼叫）** |
| Brier / ECE | 0.2851 / 0.1790 |

兩個獨立的口徑（pooled、同日排名）都指向同一結論：**點估計略低於隨機，CI 含 0.5**。

### 5.3 相對 baseline 的增量

| baseline | AUC | AUC(同日) | ΔAUC [CI] | ΔAUC(同日) [CI] |
|---|---|---|---|---|
| `industry_momentum_20d` | 0.4538 | 0.4874 | +0.0047 [−0.0467, +0.0546] | −0.0292 [−0.0664, +0.0074] |
| `industry_momentum_5d` | 0.4631 | 0.4563 | −0.0047 [−0.0352, +0.0267] | +0.0018 [−0.0255, +0.0262] |
| `platform_industry_hitrate_tilt` | — | — | unavailable（§3） | unavailable |

- **沒有任何一條 baseline 被顯著超越**，兩條 momentum baseline 自己也 ≈ 0.45–0.46（在同一視窗上同樣無訊號）。
- `Δprecision` / `filter lift` 因為操作點只有 1 個呼叫，報告中一律標 `n/a*`（框架的可靠性閘門）：**不得**把「precision 1.0」當成證據。

### 5.4 洩漏探針（記憶 vs 預測）

| 指標 | 值 |
|---|---|
| 探針 AUC（pooled） | 0.5719 [0.4617, 0.6751] |
| 探針 AUC（同日排名） | 0.5789 [0.4959, 0.6569] |
| 樣本 | 648 cases / 36 requests（每 5 個交易日抽 1）；held-out 252 |
| 成本 | $0.0067 |

- CI 皆含 0.5 → **未達可偵測的記憶**；主結論不能歸因於「模型記得 2021 的走勢」。
- 但點估計在 0.5 之上（0.57），**不能**宣稱「完全沒有記憶」；此不確定性保留在結論中。若要更強的主張，需要更晚（接近或晚於模型釋出日 2026-09-10）的視窗。

### 5.5 校準表（reliability，held-out）

| bin | n | mean score | accuracy | gap |
|---|---|---|---|---|
| [0.1,0.2) | 405 | 0.155 | 0.477 | −0.322 |
| [0.2,0.3) | 349 | 0.241 | 0.375 | −0.135 |
| [0.3,0.4) | 192 | 0.343 | 0.396 | −0.053 |
| [0.4,0.5) | 143 | 0.445 | 0.378 | +0.068 |
| [0.5,0.6) | 149 | 0.540 | 0.396 | +0.144 |
| [0.6,0.7) | 57 | 0.629 | 0.404 | +0.225 |
| [0.7,0.8) | 1 | 0.710 | 1.000 | −0.290 |

兩個已驗證的事實（與 §4 契約的歷史觀察一致）：

1. **Jev 在此任務上系統性保守**：機率集中在 0.1–0.4，而實際命中率約 0.38–0.48 → 門檻**必須**用自家資料校準（照抄 0.5 會得到 coverage ≈ 16% 且 precision ≈ 0.40，低於 base rate）。
2. **機率尺度在中段沒有單調性**：bin 0.2–0.7 的實際命中率扁平在 0.375–0.405，卻在 0.1–0.2 反而有 0.477 —— 這是「分數不可當機率用」的直接證據。

---

## §6 非決定性與成本

| 項目 | 實測 |
|---|---|
| 呼叫非決定性（20 交易日抽樣） | 完全相同僅 **5.8%**；分數差 均值 0.022 / p50 0.02 / p95 0.05 / 最大 0.08 |
| 呼叫非決定性（全窗口 180 日 × 3240 cases） | 完全相同 **9.35%**；分數差 均值 0.019 / p50 0.02 / p95 0.04 / 最大 0.09 |
| 主要結果 | 用**首次**回答（`--repeat-aggregate first`）；重複只作為敏感性分析，不混入主要數字 |
| 總成本（含探針與 repeat=3） | ≈ **$0.189**（0.0456 + 0.0067 + 0.137） |
| Wall time | judge 180 requests @ concurrency 6 ≈ 26 秒；repeat=3（540 requests）≈ 88 秒 |

---

## §7 分級清單（`已驗證 / 已更正 / 未驗證`）

| 分級 | 項目 |
|---|---|
| ✅ **已驗證** | (1) 框架可跑、可離線重算、可重生成 GT：同一 panel 重跑 → 同 fingerprint、同 cases（自檢）。 (2) GT 覆蓋率 100%（3240/3240、18/18 產業、全 `eligible`）。 (3) Jev 在此任務上**系統性保守**且機率中段不具單調性（reliability table）。 (4) 成本量測：3240 cases = $0.0456。 (5) 呼叫非決定性量級 0.02（p95 0.05）。 |
| ⚠️ **已更正** | 「用 min_precision 0.55 校準就能得到可用的操作點」是錯的：校準確實找到門檻 0.69，但 held-out 只有 1 個呼叫 → 校準集的 precision 0.647 完全沒有轉移價值。也再次更正「Jev 可用於時序預測」的想法（與 008 的先驗一致）。 |
| ❓ **未驗證** | **headline**：Jev 對 canonical L1 產業層 5 日扣成本 forward 方向**是否攜帶資訊**。實測 pooled AUC 0.4584（CI 含 0.5）、同日排名 0.4582（CI 含 0.5）、相對兩條 baseline 的 ΔAUC CI 皆含 0 → **無統計可辨識的訊號**；但因 CI 含 0.5，正確說法是「未驗證」而非「已證實無效」。複製實驗（repeat=3）點估計相同但 CI 上界剛好跨到 0.5 以下（0.4990 / 0.4978）→ 也只能說「**沒有任何正向訊號**」（§5.1.1）。另外：`platform_industry_hitrate_tilt` baseline 在此視窗完全無法計算（§3）。 |

---

## §8 Stage 2 需求（個股層）— 由本次學到的東西導出

### 8.1 GT（可重生成）

- **定義**：per-symbol，`NetHit(forward_return, 0.00585)`，5 交易日（= H1 canonical）。**既有程式路徑**：`internal/stockpicker/backtest.go::RunBacktest`（PIT panel、`ForwardDays`、`CostRate`）→ `signal_outcome_store.go` 持久化；或直接以 `stockpicker.NetHit` 對 quote panel 計算並匯出（與 Stage 1 相同做法）。
- **既有資料（實測）**：本機 `data/state/atlas.db` → `stock_signal_outcomes`：**36,917 列 / 851 symbols / 3 sources / 2026-05-11..2026-09-09 / cost_rate 全為 0.00585**。這已足夠當**部分** GT 來源。
- **一行指令重生成**：沿用 Stage 1 的 panel exporter 模式新增 `cmd/experimental/jev-eval-panel-symbol`（或 `-level=symbol`），**但必須讀得到 quote panel**。

### 8.2 缺什麼（依阻塞程度排序）

| # | 缺口 | 現況證據 | 影響 |
|---|---|---|---|
| 1 | **quote 母體不可得** | 本機 `data/state/atlas.db` 的 `quotes` 表 = **0 列**；production 是 Postgres-first（`ATLAS_STORE_BACKEND=postgres`），quote 在生產機 | **阻塞**：沒有 quote 就只能退回讀 `stock_signal_outcomes`（只有已觸發條件的子集，母體偏誤） |
| 2 | **`I25` 母體 quote provider 寫死 nil → `symbols_ranked=0`** | 待該票驗證 | **阻塞**：symbol 層 baseline 無法產生 |
| 3 | **#1935 var-returns 正確性** | 待該票驗證 | **致命**：GT 是報酬的函數；var-returns 錯 → GT 錯，且不會報錯 |
| 4 | **#1943 產業母體覆蓋** | 產業層統計母體曾只有 **27 檔**（子票 `symbol-industry-substrate` 進行中） | symbol→industry 聯結會繼承該缺口；Stage 2 若要在產業層彙總必須先確認覆蓋率 |
| 5 | per-symbol PIT 特徵 | 需 bars + T86 flows（`data/state/stock_flows/<symbol>.json`）+ 融資券 | 中：可由既有 provider 組裝，但是工作量主體 |

### 8.3 Baseline（具名）

- **(a) 條件本身**：`stockpicker-momentum-20d-positive`（`internal/stockpicker/conditions.go`；同時是 #1959 產業命中率列的上游 source）→ 觸發/未觸發即方向。
- **(b) 條件命中率列**：`internal/stockpicker/winrate.go::ConditionWinRate` 的 `wilson_lower`（同樣以 `> 0.5` 當看多訊號），即 #1959 消費鏈的輸入端。
- **(c) 產業 tilt**：`platform_industry_hitrate_tilt`（Stage 1 已實作介面；一旦有 PIT 命中率列即可啟用）。

### 8.4 工作量估

| 工作 | 估時 |
|---|---|
| symbol-level panel exporter（沿用 Stage 1 的 exporter 骨架 + 讀 QuoteStore/`stock_signal_outcomes`） | 0.5–1 天（**前提**：quote 可得） |
| `industry_l1` 之外的 `symbol_l1` spec（state 建構：個股 PIT 特徵） | 1–2 天（特徵組裝是主體） |
| baseline (b) 的 PIT 列生成（rolling window，僅用 ≤ t 的 outcomes） | 0.5 天 |
| **前置（阻塞）** | quote 母體 + I25 + var-returns 三項釐清，屬他票 |

---

## §9 Stage 3 需求（事件 / 新聞層）— 由本次學到的東西導出

### 9.1 既有可用的東西（實測）

| 資產 | 內容 |
|---|---|
| `internal/industry/event_calendar.go` | canonical 事件日曆：**22 種事件型別**（含 `fomc_meeting`、`cpi_release`、`taiwan_export_release` 等 macro 型別）、**日期規則可程式重生成**（含農曆與台股行事曆）、`RefreshEvents` / `DetectActiveEvents` / `GetEventAdjustment(industryID, now)`、品質閘門 `eventquality.EventValidator` + `CrossSourceStore` |
| `event_calendar_history`（本機 DB） | **1,276 列，2000-03-16 .. 2026-11-30** → 事件日期**已經是可重生成的 GT 骨架** |
| `internal/eventdriven` | `FlowPrediction`（direction/confidence/distribution/driving_events）+ **T+1 對帳**（`actual_sign` / `actual_source=twse_t86`）；本機 `data/state/event_flow_predictions.jsonl` 30 列、其中 16 列有 `actual_sign` |
| H6 口徑 | 「預測 sign == T+1 實際 sign」= **指標**（`docs/specs/industry-hitrate-metric-spec.md` §2 明列 H6 不是命中率，無成本、T+1） |

### 9.2 缺什麼

| # | 缺口 | 說明 | 可否可程式重生成 |
|---|---|---|---|
| 1 | **事件窗口的扣成本 forward return GT** | 現有只有「T+1 資金流方向命中」（H6），**沒有** canonical 扣成本 forward return。Stage 3 需要 `(event_type, industry_id, event_date) → forward_return(H) → NetHit` | ✅ 可以（事件日期已決定性 + 產業指數序列可得；照 Stage 1 的 exporter 模式） |
| 2 | **事件層 baseline** | `GetEventAdjustment` 回傳的是**啟發式 adjustment**，不是 outcome；沒有「事件日 + 方向」的既有可評估訊號 | ⚠️ 需先指定語意（用 adjustment 當 baseline 是可行的，但必須明說它是 heuristic） |
| 3 | **新聞/RSS 事件標註** | `rss_geo_event` 只是**門檻觸發型別**；沒有「新聞 → 事件標籤」的已驗證管線，故新聞層目前**沒有可重生成的標註** | ❌ 需要新建（且屬 §4.1 高風險：標註必須可自我驗證） |
| 4 | 事件視窗的樣本量 | 事件日稀疏（1,276 列 / 26 年），單一事件型別 × 產業的樣本很快低於 min_samples 30 | 需先做樣本量盤點再定視窗 |

### 9.3 工作量估

| 工作 | 估時 |
|---|---|
| 事件窗口 GT exporter（`event_calendar_history` × canonical L1 × forward H，扣成本、走 `NetHit`）→ 一行指令 | **0.5–1 天** |
| `event_l1` spec（state：事件型別/方向/衰減 + 該產業 PIT 特徵；baseline = `GetEventAdjustment` heuristic） | 1 天 |
| 新聞 → 事件標註管線（若要 Stage 3 的「新聞」面） | 3–5 天，且需要獨立的標註驗證設計 |

> **由 Stage 1 導出的排序建議**：**Stage 3 的「排程事件層」比 Stage 2 便宜**，因為事件日期已是決定性 GT 骨架、不需要 quote 母體（產業指數序列即可）；Stage 2 則被 quote 母體 / I25 / var-returns 三項外部缺口阻塞。建議 Stage 1.1 → Stage 3（排程事件）→ Stage 2（個股）。

---

## §10 Stage 1.1（下一步，本階段未做）

1. **近期真實窗口**：把 `data/state/sector_index/` 換成生產機（Mac Mini）的真實資料，跑 `2026-09` 之後的日期 → 取得**晚於模型釋出日（2026-09-10）**的乾淨樣本；forward 需要 5 個交易日，因此最早可在 5 個交易日後成立。
2. **啟用 `platform_industry_hitrate_tilt` baseline**：以 `stock_signal_outcomes` 產生 PIT 命中率列（僅用 ≤ t 的 outcomes），即可量測「Jev 相對**平台現有產業層訊號**」的增量——這才是 Stage 1 真正想回答的問題。
3. **提高統計力**：多窗口（2021、2026）合併 + `--repeat 3` 平均，讓 AUC CI 收窄到 ±0.03 以內。
4. **不做**：接線（E2）。本次產物只有檔案，無任何生產路徑依賴。
