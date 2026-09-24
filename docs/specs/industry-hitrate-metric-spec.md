# 產業級命中率口徑規格（canonical）

| 項目 | 內容 |
|---|---|
| 文件角色 | 「產業命中率」的**單一口徑 SSOT**：定義、聚合鍵、覆蓋率政策、以及與其他 8 套「命中」判定（非 canonical）的關係 |
| 狀態 | v1（2026-09-24，issue [#1942](https://github.com/kaecer68/atlas-go/issues/1942)） |
| 放置選擇 | 放 `docs/specs/`（主題技術規格，與 `dashboard-metrics-ssot-spec.md` 同類）；`docs/reference/` 保留給跨模組索引（`traps`/`parameter-system`/`tool-catalog`）。本檔是「一個指標的定義」，屬前者 |
| Source-of-truth | 定義：本檔；數學：`internal/stockpicker/winrate.go`；參數：`configs/parameters.json` → `stockpicker.*`；產業鍵：`internal/industry/sector.go` |
| 相關票 | [#1942](https://github.com/kaecer68/atlas-go/issues/1942)（本票，唯讀新介面 + 文件）；[#1943](https://github.com/kaecer68/atlas-go/issues/1943)（二套產業命名空間，**不在本票範圍**） |

> **一句話**：產業命中率 = 以**唯一扣成本**的 stockpicker 口徑（`net_forward_return > 0`、固定 5 交易日、成本率 0.585%、≥30 樣本 + Wilson 95% CI），把聚合鍵換成 **canonical L1 產業**（`industry_id × source × rolling_window`）。
> 其餘判定式**全部降級為「指標」**，不得再以「命中率」對外呈現（見 §2）。
>
> 計數定義：本票之前既有 **10 套**判定式（§2 的 H1–H10），其中只有 **H1/H2** 是扣成本的 canonical 口徑；本票新增產業級 **H2b**（同口徑、只換聚合鍵）；剩下的 **H3–H10 共 8 套**一律降級為指標。

---

## §0 為什麼需要這份規格（#1942 症狀）

- 全 repo 至少 **10 套並存的「命中」判定式**（§2），期間與成本假設互斥 ⇒ 同一個策略在不同口徑下的勝率**不可比**，卻都被叫「命中率」。
- **沒有產業聚合鍵**：`stock_signal_outcomes` / `stock_win_rate` 只有 `(symbol, source, rolling_window)`、`source`、`agent_id`（agent 層 outcome 結構甚至沒有 industry 欄位）⇒「某產業命中率＝？」**以前無法查**。
- 產業層只有四条**間接**形式（narrative 相對強弱、季節 pattern、週期羅盤 5 層、12 個產業桌 agent 的 scorecard），且多數閉環 inert。
- 後果：任何「提升產業命中率」的評估**不可證偽**（MATURITY/docs 查無產業命中率實測）。

本規格與實作**只新增唯讀表面**：不改任何既有計算、不改 Darwinian 權重、不動任何既有數值。

---

## §1 Canonical 定義（唯一認可）

| 要素 | 值 | SSOT |
|---|---|---|
| 命中判定 | `net_forward_return > 0`，即 `forward_return − cost_rate > 0`（**嚴格大於**，打平不算命中） | `internal/stockpicker/winrate.go:98 NetHit` |
| 持有期 | **5 交易日**（stockpicker 既有固定持有期） | `internal/stockpicker/daily_update.go:39 DefaultForwardDays = 5` |
| 成本率 | **0.585%**（台股來回：手續費 0.1425%×2 + 證交稅 0.3%；**不含滑價**） | `configs/parameters.json` → `stockpicker.costs.round_trip_pct = 0.00585` |
| 最小樣本 | **30**（未達標 → `calibration_status = calibrating`，僅供觀察） | `configs/parameters.json` → `stockpicker.calibration.min_samples = 30` |
| 信賴區間 | **Wilson score interval，95%**（下界 `wilson_lower` / 上界 `wilson_upper`） | `internal/stockpicker/winrate.go:71 WilsonScoreInterval` |
| 聚合鍵 | **`industry_id × source × rolling_window`**（可選 `regime` 分層） | 本檔 §3 |
| 產業鍵 | **canonical L1 `industry.SectorID`**（20 個；`internal/industry/sector.go`，映射表 `internal/industry/representative_stocks.go ClassifyBySymbol`） | `internal/industry/sector.go` |
| 反轉語義 | `source = stockpicker-price-volume-top-divergence`（頂背離，avoid）⇒ 回傳 `direction = "avoid"`，**低勝率＝訊號有效** | `internal/stockpicker/conditions.go:212 IsAvoidCondition` |

### 1.1 數學必須共用、不得另寫一份

產業層使用 `WinRate` / `WilsonScoreInterval` / `CalibrationStatusFor` / `NetHit` **同一組函式**（`internal/stockpicker/industry_winrate.go` 呼叫既有 helper）。因此 symbol → condition → industry 三層的數字**可直接互相比較**，不會出現第三套數學。

### 1.2 覆蓋率與未映射政策（不可靜默 drop）

產業歸屬依 canonical L1；**對不到 canonical L1 的 symbol 必須被回報，且不得灌進 `unknown` 桶**：

- 未映射 outcome **不進任何產業列**（放進 unknown 會回答另一個問題並掩蓋缺口）。
- 回應必附 `coverage` 區塊：`total_observations` / `mapped_observations` / `unmapped_observations`、`total_symbols` / `mapped_symbols`、`coverage_pct`（以 observation 計）、`symbol_coverage_pct`（以相異 symbol 計）、`unmapped_symbols[]`（含每個 symbol 的 observation 數，依量遞減排序）。
- 每列另附 `coverage_pct` = 該列 observations ÷ **本次查詢實際納入的** observations（分母含未映射）⇒ 單列不會高估自己掌握的證據量。未指定 `regime` 時分母 = 該 source 在此視窗的全部 observations；指定 `regime` 時分母 = 該分層的 observations。
- 因每列獨立四捨五入到 2 位小數，**所有列相加 ≈ `coverage.coverage_pct`**（誤差 ≤ 0.005 × 列數），不是精確等於。
- 報告級 `coverage` 描述的是**該 source × window（× regime）的讀取**，與 `industry_id` 過濾無關：用 `industry_id` 只回一列時，`coverage` 仍是整個 source 的覆蓋率。
- `coverage` 在 `found=false` 時**仍會回填**，包含兩種空集合：
  - **完全無資料**：該 source 在此視窗沒有 outcomes → `coverage` 全 0，訊息為 `no stored outcomes for condition X (window W)`。
  - **分層為空**：指定了 `regime` 但沒有 outcome 帶該標記（未標記的舊 rows 不可歸層，見 §3）→ `found=false`、`industries` 為空，但 `coverage` 回填**未分層**的讀取，訊息明確說明「無此 regime 的資料，其他 regime 有 N 筆」。呼叫者必須能分辨「沒有資料」與「不在這一層」，且**不得**把其他 regime 的列混進來回答。

### 1.3 明確的不變式

- 不改 `net_forward_return` / `hit` 的計算（`internal/stockpicker/daily_update.go`、`backtest.go` 不動）。
- 不改 Darwinian 權重公式與任何既有數值；本票**不**新增回饋閉環讀取（見 §5）。
- 產業列**不跨 source 合併**：`condition_id` 必填。把異質條件池化成一個「產業命中率」正是本票要消滅的口徑混亂。

---

## §2 與其他口徑的關係（裁決表）

**只有 H1/H2/H2b 可稱「命中率」**（= 本規格，扣成本）；其餘 8 套（H3–H10）一律稱**「指標」**，且對外呈現必須標示其實際期間與成本假設（§7）。

| # | 主體 | 判定式 | 期間 | 成本 | 裁決 |
|---|---|---|---|---|---|
| **H1** | 個股 × 條件（canonical 底層） | `net_forward_return > 0` | 5 交易日 | **0.585%** | ✅ **命中率（canonical）** |
| **H2** | 條件級跨股（`ConditionWinRate`） | 同 H1，以 `source` 聚合 | 5 交易日 | 0.585% | ✅ **命中率（canonical，同口徑）** |
| **H2b** | **產業級（本票新增）** | 同 H1，以 `industry_id × source × window` 聚合 | 5 交易日 | 0.585% | ✅ **命中率（canonical，同口徑）** |
| H3 | 退化判定 `IsDegraded` | 近期段 Wilson 上界 < 全期 Wilson 下界 | 依 `degraded_recent_fraction` | — | ⚙️ **校準旗標**（不是命中率；寫入 `calibration_status`） |
| **H4** | **agent 層（含 12 產業桌）** | `Hit = forwardReturn > 0` | **1 交易日** | **無成本、無 benchmark** | ⚠️ **指標**。與 canonical 差 **5×持有期 + 0.585% 成本** ⇒ 兩者**不可比**；UI/文件必須標示「1 日 / 未扣成本」 |
| H5 | 績效報告 | `ForwardReturn > 0.002`（0.2% 近似成本） | 1 交易日 | 0.2% 近似 | ⚠️ **指標**（近似成本 ≠ 0.585%） |
| H6 | 事件流方向命中 | 預測 sign == T+1 對帳實際 sign | T+1 | 無 | ⚠️ **指標**（方向命中率，非報酬命中率） |
| H7 | narrative 投資模型 | favored 產業平均前瞻報酬 > avoided | `holdWindow=5` | 無 | ⚠️ **指標**（產業**相對強弱**，非單一產業命中率；且結果僅在記憶體） |
| H8 | 季節 pattern | favored 產業期間累積報酬 > avoided（逐年 1 次） | 逐年 | 無 | ⚠️ **指標**（年度、樣本極少） |
| H9 | 心法 L1–L5 | 條件觸發後 **TAIEX** 方向 == frame 方向 | `forwardLookback` | 無 | ⚠️ **指標**（基準是 TAIEX，不是個股報酬，且非全部 frame 可命中） |
| H10 | 外資現貨預測校準 | 預測方向 == T+1 實際方向（±門檻分級） | T+1 | 無 | ⚠️ **指標**（方向命中率，樣本門檻 90 / hitRate ≥0.55） |

實作位置（現行 HEAD）：

| # | 位置 |
|---|---|
| H1/H2 | `internal/stockpicker/winrate.go:52/71/88/98/108/195` |
| H2b | `internal/stockpicker/industry_winrate.go`、`internal/stocktools/industry_winrate.go` |
| H3 | `internal/stockpicker/winrate.go:250 IsDegraded`、`internal/stockpicker/aggregate.go` |
| H4 | `internal/orchestrator/system.go:1252,1305`（`ds.ForwardReturn(..., 1)` 於 `:1283`）；聚合 `internal/portfolio/darwinian_weights.go:326,333`；同一口徑的另一個對外表面（agent scorecard `hit_rate`）在 `internal/ledger/ledger.go:514`（`ratio(entry.hits, n)`，hits 來自 `outcome.Hit`） |
| H5 | `internal/reporting/performance.go:1099 defaultWinRateThreshold = 0.002` |
| H6 | `internal/eventdriven/handler.go:134 hitRateWindow = 60`、`:186 computeHistoricalHitRate`；`internal/eventdriven/types.go:9 MinHitSamples = 30` |
| H7 | `internal/narrative/knowledge_base.go:585-586` |
| H8 | `internal/industry/seasonal_calibrator.go:333-339` |
| H9 | `internal/strategy_techniques/evaluator.go:155-159`（判定式；`:30-31` 只是 `EvalResult` 的 `TotalHits`/`HitRate` 欄位） |
| H10 | `internal/forecast/foreign_forecast.go:33/36`（90 樣本、hitRate ≥0.55） |

**成本假設不一致的具體後果**（H1/H2 0.585% vs H4 0% vs H5 0.2%）：同一批交易在「agent 層」與「stockpicker 層」會得到不同勝率；引用數字時必須同時引用口徑。

---

## §3 實作對照（唯讀）

| 層 | 位置 |
|---|---|
| 純聚合（無 IO、resolver 注入） | `internal/stockpicker/industry_winrate.go`：`IndustryWinRate`、`IndustryWinRateFor`、`IndustryWinRateSummary`、`IndustryCoverage` |
| 服務層（讀 DB、注入 canonical L1 resolver、填 zh 名稱/window） | `internal/stocktools/industry_winrate.go`：`SQLiteWinRateProvider.LoadIndustryWinRate`、`ResolveCanonicalL1Industry`、`CanonicalL1IndustryID` |
| HTTP | `GET /api/stock/industry_winrate`（`internal/stocktools/handler.go`，與 `win_rate` / `condition_winrate` 同契約：`found=false` + `message`，非錯誤） |
| MCP | `stock_get_industry_winrate`（`cmd/atlas-mcp/server/tools_stock.go`，轉呼上述端點） |
| 資料源 | `stock_signal_outcomes`（job-local SQLite ledger，`mode=ro` 開啟）——**只聚合既有 raw rows，不重算回測**。與 `/api/stock/win_rate`、`/api/stock/condition_winrate` 同源；production 為 Postgres-first，此端點可見的資料取決於該 job-local ledger 是否為最新 backfill（**既有風險，本票不改變**） |

查詢參數：

| 參數 | 必填 | 說明 |
|---|---|---|
| `condition_id` | ✅ | 例如 `momentum-20d-positive`、`price-volume-top-divergence`（多個條件請分別呼叫） |
| `industry_id` | ✖ | canonical L1 id 或中文標籤（`semiconductor` / `半導體` / `金融` 皆可）。省略 = 回傳該 source 的**全部** L1 列（排名視圖） |
| `rolling_window` | ✖ | 預設 `120d` |
| `regime` | ✖ | 例如 `RISK_ON`；只納入觸發時已標記 regime 的 rows（未標記者排除，不池化） |

回傳（節錄）：`found`、`message`、`source`、`condition_id`、`direction`、`rolling_window`、`regime`、`industries[]`（`industry_id`、`industry_name_zh`、`observations`、`symbols`、`hits`、`win_rate`、`wilson_lower/upper`、`confidence`、`calibration_status`、`net_cost_rate`、`avg_forward_return`、`avg_net_forward_return`、`coverage_pct`、`data_start/end`）、`coverage`（§1.2）。

**錯誤契約**（與既有同族端點一致）：

| 情境 | 回應 |
|---|---|
| 缺 `condition_id` | `400` |
| `industry_id` 非法（未知 / L2 sub-industry） | `400` |
| `industry_id` 合法但該 source/視窗（或分層）無該產業的資料 | `200` + `found=false` + `message`（附覆蓋率） |
| provider 未注入 / ledger 不可讀 / `rolling_window` 格式非法（如 `abc`） | `503` + `error`（**沿用** `win_rate` / `condition_winrate` 既有契約，未在本票改為 400） |

**Auth/tier**：與其他 `/api/stock/*` 相同，目前在 `cmd/atlas/main.go isPublicPath` 為 public path（不需 API key）。**未新增任何寫入面**。

**兩個已知限制（沿用既有生成器/端點特性，非本票缺失）**：

1. `*_web/static/js/shared/field_types.ts` 的 `IndustryWinRateResponse` 只有自有欄位（`found`/`message`）：field-contract 生成器**不展開 embedded struct**，因此 `industries` / `coverage` 不在 TS 型別中（`ConditionWinRateResponse` 同此行為）。前端若要用這兩個欄位需自行宣告。
2. `coverage.unmapped_symbols` 是**完整清單**（dev ledger 實測 755 筆），不截斷、不抽樣——這是「不可靜默 drop」的直接後果；只要摘要的呼叫者請用 `unmapped_observations` / `symbol_coverage_pct`。

---

## §4 覆蓋率實測（本機 dev ledger，2026-09-17 快照）

> 本節數字取自**開發機 job-local ledger artifact**（`data/state/atlas.db`），**不代表 production**（production 為 Postgres-first）。重跑方式：以 `rolling_window=3650d` 呼叫新端點，或讀同一張表以 `representative_stocks.go` 的映射重算（兩條路徑實測一致）。

資料源：`data/state/atlas.db` 的 `stock_signal_outcomes`（36,917 rows、3 個 source、851 個相異 symbol）；以本票新增的唯讀聚合（`rolling_window=3650d`）實測：

| condition（source） | 產業列 | observations | 已映射 | 未映射 | 覆蓋率（obs） | 已映射 symbol / 全部 |
|---|---|---|---|---|---|---|
| `momentum-20d-positive` | 20 | 28,457 | 4,637 | 23,820 | **16.29%** | 96 / 851（11.28%） |
| `price-volume-top-divergence` | 20 | 2,726 | 525 | 2,201 | **19.26%** | 82 / 529（15.50%） |
| `price-volume-bottom-divergence` | 20 | 5,734 | 550 | 5,184 | **9.59%** | 83 / 740（11.22%） |

- 20 個 canonical L1 全部出現（代表股映射涵蓋每個產業，但每產業僅 2–12 檔）。
- 三個 source 的未映射相異 symbol **聯集 = 755 個**（= `momentum` 的 851 − 96；另兩個 source 的未映射集合為其子集：447 與 657 個）。其中含大量 ETF（`0050/0056/00713/00878/00919/00940…`）與小型股，前幾名為 `00713`（81 筆）、`0051`、`0056`、`00878`、`00940`。
- **結論**：現行 canonical L1 代表股映射（96 檔）不足以支撐產業級統計 ⇒ 目前任何「某產業命中率」都必須附覆蓋率，且**不應**用於跨產業排名裁決。要提升覆蓋率需接上完整 industry 欄位（見 §5 / #1943）。

---

## §5 不在本票範圍（相鄰問題，需另開票）

1. **#1943 二套產業命名空間**：canonical L1（20）與 config `classification_tree`（16 L1，僅 5 個與 canonical 交集）不一致；本票**只**用 canonical L1，未動 tree mapper。⇒ 覆蓋率會隨該票結論改變。
2. **DB 無個股產業欄位**：本票不改 schema（未新增 industry 欄位、未新增表），聚合在記憶體完成。
3. **回饋閉環**：Darwinian 權重與 `sectorallocation` 仍**不讀**產業命中率（本票不觸碰權重公式）。要「至少一個閉環改讀 canonical 口徑」需另票（含風險評估）。
4. **H4 agent 層改口徑**（1 日/無成本 → 5 日/扣成本）：屬行為變更，需另票，且會改動既有 scorecard 數字。
5. **其餘 9 套口徑的 UI/文件標示**：本規格先定義「不得稱命中率」，逐處改字屬後續清理。

---

## §6 測試與驗收

| 測試 | 覆蓋 |
|---|---|
| `internal/stockpicker/industry_winrate_test.go` | 分組與排序、Wilson 邊界（單筆、全命中）、coverage 計算與四捨五入、未映射不被池化、跨 source 跳過、avoid 語義、minSamples 門檻、nil resolver、`IndustryWinRateFor` |
| `internal/stocktools/industry_winrate_test.go` | 端點 happy path（含 `industry_name_zh`）、industry 過濾（id 與中文）、無列時 `found=false` + coverage 保留、非法 industry_id（未知 / L2）400、缺 `condition_id` 400、無 provider 503、空資料 200、regime 分層 |
| `internal/stocktools/win_rate_test.go`（既有） | per-symbol 與 condition 級**不得回歸** |
| `cmd/atlas-mcp/server/tools_stock_test.go` | MCP tool 路徑/參數轉送 |
| `cmd/atlas-mcp/server/tools_canary_test.go` | canary 路由條目 |

驗收（#1942）：可用**一個查詢**回答「某產業在某期間的命中率＝？」（例：`GET /api/stock/industry_winrate?condition_id=momentum-20d-positive&industry_id=semiconductor&rolling_window=120d`），且回覆必附 samples / Wilson CI / 覆蓋率。

---

## §7 呈現紀律（強制）

1. 只有 H1/H2/H2b 的數字可標為「命中率」。
2. 引用其他口徑時，必須同時寫出「判定式 + 期間 + 成本假設」，且用語為「指標」或「方向命中率」。
3. 產業命中率一律附 `observations`、`wilson_lower/upper`、`calibration_status`（< 30 樣本 = calibrating，不得下結論）與 `coverage_pct`。
4. 不得把「未映射」的 observations 併入任何產業數字，也不得省略未映射清單。
