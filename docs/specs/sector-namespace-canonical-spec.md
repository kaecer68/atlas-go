# 產業命名空間統一規格（canonical sector taxonomy + 顯式映射）

| 項目 | 內容 |
|---|---|
| 對應 issue | #1943（產業分類六套命名空間統一） |
| 落地套件 | `internal/sectormap`（leaf，零專案相依）、`internal/industry`（typed 權威）、`internal/sectorallocation`、`internal/marketdata` |
| 稽核工具 | `go run ./cmd/experimental/industry-namespace-audit` |
| 漂移守門測試 | `internal/sectormap/drift_test.go`、`internal/industry/namespace_bridge_test.go`、`internal/industry/data_conflict_test.go`、`internal/marketdata/namespace_canonical_test.go`、`internal/marketdata/finmind_sector_index_provider_test.go`、`internal/sectorallocation/legacy_gics_namespace_test.go` |

## 1. 問題陳述

atlas-go 的「產業」在生產程式碼裡同時存在 **6 套以上互不相容的 key 空間**。同一個
symbol、同一個 TWSE 指數名稱，在不同程式路徑會得到不同的「產業 ID」；多數消費端
根本無法消費另一端的輸出。實證（2026-09-24 唯讀盤查，main@6fed8b8，
`~/workspace/atlas-notes/03-system-health/2026-09-24-industry-hitrate-survey.md` §1.1）：

- `monitoring.TreeBasedMapper` / `industry.SymbolL1Mapper` 只認「ID 剛好是 canonical L1」的樹節點，
  29 個 segment 有 **19 個被靜默跳過**；生產唯一路徑母體只有 **27 支** symbol，`symbols_ranked=0`。
  （`SymbolL1Mapper` 實跑真實 config 只解出 **18 支 / 5 個產業**。）
- `sectorallocation` 的 legacy `ComputeWeights` 走 `base_weights`（GICS 式 11+1 key），
  產物**永遠不可能**通過 `ValidateL1FinalTarget`（要求恰好 20 個 canonical L1 key）。
- `marketdata` 同一份 TWSE 名稱有**兩份映射且互相矛盾**：
  `mapIndustryName` 把「電腦及週邊設備類」映到 `ai_supply_chain`、「電機機械類」映到
  `robotics`；`canonicalL1SectorID` 卻映到 `electronics` / `machinery`。
- `SectorIndexReader.canonicalSectorIDs` 手抄 18 個 ID，缺 `chemicals`、`tourism`。

## 2. Canonical taxonomy（唯一權威）

**權威檔案**：`internal/industry/sector.go`（typed 定義）、`internal/sectormap/canonical.go`（跨套件共用的 ID 清單）。

- **L1 = 20 個**：`auto, biotech, cement, chemicals, construction, electronics, energy,
  financials, food, machinery, optoelectronics, other_electronics, plastics, retail,
  semiconductor, shipping, steel, telecom, textiles, tourism`
- **L2 = 18 個**：`ai_supply_chain, consumer, cooling, copper_industry, etf_rotation, foundry,
  ground_equipment, industrial, laser_communication, leo_satellite, metal_processing, mining,
  precious_metals_recycling, rare_earth_specialty, robotics, satellite_pcb,
  satellite_rf_components, server_assembly`
- 中文標籤只存在於 `DisplayZHTw` / `SubIndustryDisplayZHTw`，**永不當 key**。
- `SectorIDFromString` 可解析 canonical ID、中文全名、legacy 中文別名（`DisplayZHAliases`）。

### 2.1 L2 → L1 父層表（本 PR 新增）

canonical taxonomy 原本只宣告「ID 集合 + 層級」（`IsL1`/`IsL2`），**沒有父子關係**，
所以「這個 symbol 屬於哪個 L1」對 L2-only segment 無解。父層關係改為顯式表
（`internal/sectormap/canonical.go` → `l2ParentL1`），並遵守一條硬規則：

> **凡 config 分類樹對某個 L2 segment 宣告了父鏈，本表必須與它一致。**
> 否則同一個 key 會因為「來自樹 segment」或「來自原始 key」而解析到不同 L1 —— 正是本
> PR 要消滅的缺陷類型。`TestDrift_L2ParentL1AgreesWithAuthoredTree` 強制此規則。

| L2 | L1 父層 | 依據 |
|---|---|---|
| cooling, server_assembly, foundry | semiconductor | 樹的 `parent_id`（`foundry` 亦符合語意） |
| ai_supply_chain | electronics | 樹 L1 root（無父層可依）；本表為唯一宣告 |
| satellite_pcb, satellite_rf_components, laser_communication, ground_equipment, leo_satellite | telecom | 樹的父鏈（`leo_satellite`） |
| industrial, robotics | machinery | 樹 L1 root（無父層可依） |
| mining, copper_industry, precious_metals_recycling, rare_earth_specialty, metal_processing | steel | 樹的父鏈（`mining`）；TWSE 鋼鐵類涵蓋礦業與金屬加工 |
| consumer | retail | 樹 L1 root；TWSE 消費 → 百貨零售 |
| etf_rotation | **無父層** | 資產類別輪動桶，不是權益產業；`ParentL1Of` 回 `("", false)` |

## 3. 命名空間清單與處置

「命名空間」= 一套可作為產業 key 的字串集合。共 **12 套**已宣告，每套都在
`internal/sectormap/tables.go` 有逐 key 的顯式處置（數字為稽核 CLI 實際輸出）。

| # | namespace | 來源 | key 數 | canonical | mapped | unmapped | 覆蓋 L1 | 處置 |
|---|---|---|---|---|---|---|---|---|
| A | `canonical_l1_l2` | `internal/industry/sector.go` | 38 | 38 | 0 | 0 | 20/20 | **保留（權威）** |
| C | `industry_representative_stocks` | `internal/industry/representative_stocks.go` | 20 | 20 | 0 | 0 | 20/20 | **保留**（key 已是 canonical L1） |
| B | `config_classification_tree` | `configs/parameters/industry.json` → `classification_tree` | 29 | 23 | 0 | 6 | 9/20 | **保留 + 顯式映射** |
| D | `config_sector_symbols` | `configs/sector_symbols.json` | 22 | 16 | 0 | 6 | 10/20 | **保留 + 顯式映射** |
| F | `config_industry_default_metrics` | `industry.json` → `default_metrics` | 23 | 23 | 0 | 0 | 9/20 | **保留**（全為 canonical ID） |
| F′ | `config_industry_cycle_thresholds` | `industry.json` → `cycle_thresholds` | 10 | 10 | 0 | 0 | 8/20 | **保留** |
| E | `config_sector_allocation_base_weights` | `configs/parameters/sector_allocation.json` → `base_weights` | 12 | 5 | 2 | 5 | 7/20 | **標記 legacy**：經 `ProjectLegacyGICSWeights` 投影，5 個 key 需人工決策 |
| G | `twse_sector_index_name` | TWSE `MI_INDEX` 指數名 | 39 | 0 | 24 | 15 | 20/20 | **收斂為單一表**（原兩份矛盾映射刪除）；見 §3.2 |
| G′ | `twse_sector_index_name_legacy` | 同上（舊 8-key 子集） | 8 | 0 | 8 | 0 | 7/20 | **標記 deprecated**；與 G 同一表推導，永不互相矛盾 |
| H | `marketdata_sector_index_reader_ids` | `SectorIndexReader` | 22 | 20 | 2 | 0 | 20/20 | **保留**：20 canonical L1 + 2 個 legacy 讀取相容 ID |
| I | `finmind_sector_series` | `marketdata.finmindSectorSeries` | 18 | 0 | 18 | 0 | 18/20 | **保留**：缺 `chemicals`、`tourism`（`觀光` 上游存在但被丟棄） |
| J | `strategy_technique_sectors` | `data/seeds/strategy_techniques.json` → `sectors` | 20 | 0 | 5 | 15 | 3/20 | **保留 + 標記混用**：15 個是 size/style/asset-class 桶 |

### 3.1 未映射（unmapped）清單（必須回報，不得靜默丟棄）

| namespace | unmapped keys | 類別 | candidates（僅供人工決策） |
|---|---|---|---|
| B / D | `defensive` | strategy_bucket | cement, food, telecom, energy |
| B / D | `high_dividend` | strategy_bucket | — |
| B / D | `small_cap` | strategy_bucket | retail, textiles, construction |
| B / D | `tech` | research_theme | electronics, semiconductor, other_electronics, optoelectronics, telecom |
| B / D | `pcb` | research_theme | electronics, other_electronics |
| B / D | `thermal` | research_theme | electronics, other_electronics, cooling |
| E | `industrials` | GICS 橫跨多 L1 | machinery, construction, shipping, steel |
| E | `materials` | GICS 橫跨多 L1 | chemicals, plastics, cement, steel |
| E | `real_estate` | canonical 無此 L1 | construction |
| E | `utilities` | canonical 無此 L1 | energy |
| E | `_cash_reserve` | asset_class | —（不得給權益 candidate） |
| G | `電子工業類指數`、`電子通路類指數`、`資訊服務類指數`、`數位雲端類指數`、`綠能環保類指數`、`化學生技醫療類指數`、`塑膠化工類指數`、`玻璃陶瓷類指數`、`水泥窯製類指數`、`造紙類指數`、`橡膠類指數`、`機電類指數`、`運動休閒類指數`、`居家生活類指數`、`其他類指數` | 複合指數／canonical 無節點／殘差桶 | 逐 key 列於表中 |
| J | `PCB`、`電源`、`內需`、`出口股`、`出口導向股`、`外銷股`、`中小型股`、`權值股`、`高股息`、`防禦性資產`、`題材股`、`科技股`、`加權指數`、`黃金`、`ETF 標的` | research_theme / strategy_bucket / asset_class | 逐 key 列於表中 |

`internal/sectormap` 對每個 unmapped key 都必須提供 `Reason`；有機器可讀 candidate 者列
`Candidates`（**不會自動套用**）。未知 key（不在表內）回 `StatusUnknown` = drift，與
`StatusUnmapped` 明確區分。

### 3.2 TWSE 名稱與 live API 的落差（2026-09-24 實測）

TWSE `exchangeReport/MI_INDEX` 實際回傳 **37 個** `*類指數` 名稱。本表：

- **對映 20 個**（兩組塌縮：「電腦及週邊設備類」＋「電子零組件類」→ `electronics`；
  「電機機械類」＋「電器電纜類」→ `machinery`），使 20 個 canonical L1 全數可達。
- **宣告 15 個為 unmapped**（複合指數如「化學生技醫療類指數」、canonical 無對應節點者如
  「造紙類指數」、殘差桶「其他類指數」）。provider 原本就丟棄它們，現在這份丟棄是**具名可稽核**的。
- **2 個歷史別名**：「化學工業類指數」「觀光類指數」**不存在於 live 回應**（本 PR 前的表只有這兩個名稱
  → `chemicals`/`tourism` 實際上永遠拿不到資料，不只是被讀取端過濾）。live 名稱為
  「化學類指數」「觀光餐旅類指數」，已補入表中；舊名保留以便快取檔案仍可解析。

## 4. 映射規則（強制）

1. **禁止字串相似／模糊比對**。key 只能有「顯式對映」或「顯式未映射」兩種狀態。
2. **禁止隱式 alias**。舊 ID 若仍須讀取，必須在表中顯式宣告（例：`
   marketdata_sector_index_reader_ids` 的 `ai_supply_chain`/`robotics`）。
3. **未映射必須回報**：回傳 `StatusUnmapped` + `Reason`，呼叫端不得自行猜測。
4. **1:many 映射必須帶權重**（`Targets` 加總 = 1）。目前**沒有任何** 1:many 條目（`dist()` 未被使用）；
   GICS 的複合類別一律走 unmapped + candidates，避免發明權重。
5. **漂移必須測試**：上游 config/程式變動而未更新表 → 測試紅燈（§7）。

### 4.1 樹結構 vs 宣告表的優先序（`SymbolL1Mapper` / `SymbolIndustryMapper`）

| rank | 規則 | 例 |
|---|---|---|
| 0（structural） | 樹路徑（root→自身）中**最近**一個 canonical L1 節點 | `cooling` 掛在 `semiconductor` → semiconductor |
| 1（declared） | 無 canonical L1 祖先時，由 `internal/sectormap` 宣告表換算（segment 自身優先，再往外找祖先） | `robotics` → machinery；`ai_supply_chain` → electronics |

§2.1 的規則確保 rank 0 與 rank 1 對同一個 key 給出同一個 L1，因此兩條路徑等價。

同 symbol 被多個 segment 認領時，依 (rank 升冪 → 深度降冪（越特定優先）→ segment ID 字典序)
決定，並記錄為 `SymbolL1Conflict` 供人稽核。**唯一會回 error 的情況**：同一 symbol 被
兩個「自身 ID 即 canonical L1」的 segment 宣告（樹的作者錯誤，無法用品味解決）。
真實 config 下有 **1 筆** conflict（`2356`：`server_assembly`(rank 0→semiconductor) vs `robotics`(rank 1→machinery)）。

## 5. 覆蓋率指標（#1943 item 4）

兩個數字必須分開講，不可混用：

| 指標 | 定義 | 現值（真實 config） |
|---|---|---|
| **映射覆蓋率（declared）** | 系統自己宣告的 representative stock 中，能映到 canonical L1 的比例 | L1 segments：**27 / 27**（修正前 18/27）；全樹 L1+L2：**44 / 44**（修正前 18/44） |
| **可達 canonical L1 數** | 樹/宣告表能觸及的 L1 種類 | **9**（修正前 5；22 個 aggregatable segment 過去有 19 個被跳過） |
| **母體覆蓋率（universe）** | 給定 symbol 母體（如 TWSE 上市清單、`universe_snapshot`）中，能映到 canonical L1 的比例 | 由呼叫端提供母體後計算：`industry.ComputeCanonicalCoverage(symbols, mapper)`；生產 `universe_snapshot.json` 2026-09-17 記錄 `symbols_built=27 / symbols_ranked=0`，相對 TWSE 上市普通股母體（約 854 檔）約 **27/854 ≈ 3.2%** |

> ⚠️ 母體覆蓋率低是**資料覆蓋**問題（代表股只列 27 支），**不是**映射問題。修好命名空間
> 不會改變 27/854；要提升必須增加 per-symbol 產業來源（DB 目前**完全沒有**個股產業欄位）。

查詢方式：

```bash
# 全命名空間稽核 + 宣告覆蓋率
go run ./cmd/experimental/industry-namespace-audit -json=false

# 指定母體（一行一個 symbol，# 為註解）→ 母體覆蓋率
go run ./cmd/experimental/industry-namespace-audit -universe /tmp/twse_symbols.txt -json=false
```

## 6. 已知缺口與已具名資料衝突（另開票，不在本 PR 範圍）

| # | 缺口 | 影響 | 建議 |
|---|---|---|---|
| 1 | **DB 無 per-stock 產業欄位**（`sql/migrations/` 46 檔對 `industry` 零命中） | 母體覆蓋率上限被綁在代表股清單 | 新增 `symbol_industry` 表 + FinMind/TWSE 匯入 job |
| 2 | FinMind 系列缺 `chemicals`、`tourism`（`觀光` 在上游存在但被 provider 丟棄） | `finmind_sector_series` 只覆蓋 18/20 L1 | 向 FinMind 確認系列英文名後補表（**不得猜測名稱**） |
| 3 | TWSE 一日多系列塌縮：同映 `electronics` 的兩個名稱在 `fetchSingleDay` 以 canonical ID 為 map key → 當日後者覆蓋前者 | 資訊損失（修正前即存在） | 改為同日同產業多系列聚合 |
| 4 | `SectorIndexReader` 對同一 (date, industry) 多次寫入時為 last-wins（map iteration 順序隨機） | 讀取結果可能非決定性 | Phase 1（native）也改為顯式聚合 |
| 5 | GICS `industrials`/`materials`/`real_estate`/`utilities`/`_cash_reserve` 未定案（blocked weight 0.25） | legacy `ComputeWeights` 無法產生合規 L1 向量 | operator 決定投影權重，或停用 legacy 路徑 |
| 6 | `monitoring.TreeBasedMapper` 仍只吃 `GetLevel1()`，且 `universe_builder` 對未知 symbol 靜默丟棄 | 智慧母體管線覆蓋率失真 | 改用 `industry.SymbolL1Mapper` + `UnmappedSegments()` 回報 |
| 7 | 前端 mirror 漂移：`shared_web/static/js/shared/sector-display.js` 的 `SUB_INDUSTRY_DISPLAY_ZH` 只有 10 個 L2（後端 18） | UI 顯示退回 snake_case | 補齊並加測試 |
| 8 | **編譯內建 fallback 與 JSON 不一致**：`DefaultParametersConfig().Industry.CycleThresholds` 有 13 個 key（含 `_default`；實測見 `TestDefaultCycleThresholdsVocabularyIsCanonical` 的 log），JSON 只有 10 個（多出 `etf_rotation`、`leo_satellite`） | config 檔遺失時的有效詞彙不同 | 對齊兩者；已加測試確保 fallback 詞彙全為 canonical ID |
| 9 | **同一個 key 在不同命名空間的代表股互不相容**：`consumer` 在樹的代表股是 2912/9927（零售），在 `sector_symbols.json` 卻是 1301/1303/1326/1216（3 檔在 fallback 表屬 `plastics`、1 檔屬 `food`） | 同一個「消費」桶在敘事模型與樹路徑指向不同產業 | 需 per-symbol 產業來源（同缺口 1） |
| 9 | **4 筆 tree vs fallback 表的 symbol 分類衝突**（`TestKnownSymbolClassificationConflicts` 釘住） | 同一 symbol 在兩份 authored 來源屬不同 L1 | 需 per-symbol 產業來源（同缺口 1） |

| # | symbol | tree（經宣告表） | `DefaultRepresentativeStocks()` |
|---|---|---|---|
| 1 | `1707` | steel（樹置於 mining） | chemicals |
| 2 | `2324` | semiconductor（樹置於 server_assembly） | electronics |
| 3 | `2356` | semiconductor（同上；且被兩個 segment 認領） | electronics |
| 4 | `3661` | electronics（樹置於 ai_supply_chain） | semiconductor |

## 7. 漂移守門（測試即規格）

| 測試 | 守什麼 |
|---|---|
| `internal/sectormap/drift_test.go` | 表的 key 集合 = config/seed/程式實際 key 集合（B/D/E/F/G/J）；TWSE 兩表對同一名稱必須一致；**宣告的 L2→L1 父層必須等於樹的父鏈**；TWSE 表必須仍能觸及 `chemicals`/`tourism` |
| `internal/industry/namespace_bridge_test.go` | `sectormap` 清單 = `industry.L1Sectors()`/`SubIndustryDisplayZHTw`；真實 config 下**所有**宣告 symbol 都映得到 L1；未映射 segment 集合固定；fallback cycle_thresholds 詞彙全為 canonical ID；TWSE 表同時存在 mapped 與 declared-unmapped |
| `internal/industry/symbol_l1_mapper_test.go` | rank 0/1 解析、未映射 segment 回報、L2 無祖先改用宣告表、canonical L1 重複仍回 error |
| `internal/industry/data_conflict_test.go` | tree vs fallback 表的 4 筆 symbol 衝突集合固定（修好或退化都會亮） |
| `internal/sectorallocation/legacy_gics_namespace_test.go` | GICS key 漂移；未映射 key 帶權重時必須回 `ErrLegacyGICSUnmapped`；legacy key 空間不可能通過 `ValidateL1FinalTarget` |
| `internal/marketdata/twse_sector_index_provider_test.go` | 兩個 TWSE entry point 解析同一表；legacy ID 不再被寫出 |
| `internal/marketdata/finmind_sector_index_provider_test.go` | 18 系列 = 宣告表；缺的兩個 canonical L1 固定為 `chemicals`/`tourism` |
| `internal/marketdata/namespace_canonical_test.go` | segment canonical 化（含樹結構優先、宣告表 fallback、bucket 回報理由） |

## 8. 相容性

- **JSON/API 欄位語意**：`MapperIndustryClassification` 僅**新增** `canonical_sector_id` /
  `canonical_l1` / `canonical_reason`（`omitempty`），既有欄位不動；已重跑 `go run ./cmd/gentags`
  更新 3 份前端 generated 檔。
- **`SymbolL1Mapper`**：`NewSymbolL1Mapper` 簽名不變；新增 `Len/Symbols/L1Counts/UnmappedSegments/Conflicts`。
  既有錯誤契約保留（同 symbol 被兩個 canonical L1 segment 宣告 → error）。
- **TWSE sector_index 檔案**：新寫出的檔案改用 canonical L1 ID（`ai_supply_chain`→`electronics`、
  `robotics`→`machinery`，讀取端本來就做同樣的 alias），因此**讀取結果不變**；舊檔仍可讀。
  另外新增 live 名稱（`化學類指數`、`觀光餐旅類指數`）→ 新檔會多出 `chemicals`/`tourism` 兩個產業
  （過去永遠拿不到的資料，屬修正）。副作用：新檔最多 20 個 key（舊檔最多 8），會落進
  `SectorIndexReader` 的 native（`>=18`）分支。
- **`SectorIndexReader` 接受的 ID 集合**由 18 擴為 20（補 `chemicals`、`tourism`）。
