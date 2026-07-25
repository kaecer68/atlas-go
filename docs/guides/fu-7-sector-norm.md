# FU-7 Sector Normalization: Taiwan 產業分類正規化指南

> 文件角色:本指南是 atlas-go「FU-7 Sector Normalization」專案的權威定錨文件,涵蓋六個 Phase PR (#1159 到 #1164) 之間的關係、canonical source of truth、MCP 暴露、breaking change 評估與 trade-off 取捨,供 `internal/industry` 模組、`cmd/atlas-mcp` 與 `shared_web` 前端維護者查閱。
>
> 範圍對象:L1 (20 sectors) + L2 (18 sub-industries) 合計 38 個 canonical identifier,以及它們的繁體中文 display label、legacy alias 對映、代表性個股清單。
>
> 對應合併基準:main HEAD = `d1f53135` (六個 PR 全部已合併;此 SHA 是穩定 reference point,不會隨後續 commit 改動)。
>
> 相關文件: [`../architecture.md`](../architecture.md) § Industry Ecosystem + Layer 2 Sector Desks; [`../reference/traps.md`](../reference/traps.md) § 跨模組陷阱 JSON tag 大小寫; [`../../internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) (為何 sector 常數不走 Gateway)。

## 1. 總覽

FU-7 把散落在 ~290 個 backend Go 檔與 ~13 個 frontend JS 檔的「Taiwan 產業 sector 字串識別」,收斂成單一 canonical 資源:`internal/industry/sector.go` 中的 `SectorID` enum + `DisplayZHTw` 繁體中文 label map + `DisplayZHAliases` legacy alias 反查 + L1/L2 分層判斷,並透過 `cmd/atlas-mcp/server/tools_industry_sector.go` 提供兩個 read-only MCP 工具 (`industry_sector_list`、`industry_sector_lookup`) 讓外部 LLM agent 直接查詢。整個專案採 additive migration 策略,六個 Phase 各自只加新東西或鎖單一 contract,任意階段都可單獨 revert,風險低於一次 25 檔案大改。

## 2. 動機

促成 FU-7 的五個痛點,各自對應一個 Phase 的解法:

- **String-based sector ID 散落**:`~290` 個 backend `.go` 檔用 `"半導體"`、`"金融"` 等 Chinese 字串當 map key 與 struct field;`~13` 個 frontend `.js` 檔混用三種中文表示法 (canonical 中文名、truncated 前綴、TWSE-style 後綴 `類`),沒有 single source of truth。Phase A 透過 `SectorID` enum 根治。
- **沒有 L2 sub-industry 支援**:`internal/industry/cycle.go` 早就需要 `ai_supply_chain`、`cooling`、`satellite_*` 等 L2 細分類 ID,但沒有完整 enum,只能用 string literal 飄。Phase D 把 18 個 L2 加入 canonical。
- **Code ↔ doc drift 高風險**:backend `representative_stocks.go` Chinese keys 與 frontend `demo-data.js` truncated alias 不一致,rebuild 文件或 PR merge 後誰忘記對齊都很容易。Phase B + Phase E 各自動一邊,並由 `sector-display.test.mjs` 7 條 contract 鎖死 mirror。
- **Chinese label 當 map key / JSON key**:把 `"半導體"` 直接用於 wire protocol、JSON field、SQL query 都脆弱,改名時容易 break 跨組件解析。Phase A 把 Chinese 降級為 `DisplayZHTw` 的 value,canonical key 永遠是 snake_case English。
- **MCP 沒辦法查 sector**:沒有 MCP 工具讓 LLM agent 直接查詢台股的 sector taxonomy;過去得用字串比對、demo-data.js regex、或 reflection 自寫 helper。Phase F 新增 `industry_sector_list` / `industry_sector_lookup` 兩個 read-only tool。

## 3. Phase 拆解

六個 Phase 採 additive 設計,任意 PR 都能單獨 revert。實際合併順序受 GitHub merge queue 影響,並非嚴格 A→B→C→D→E→F,但各 PR 彼此獨立,可分批 review。

| Phase | PR | 主要變更 | 程式檔案 |
|-------|----|---------|----------|
| A - Canonical resource | [#1159](https://github.com/kaecer68/atlas-go/pull/1159) | 新增 `SectorID` enum、`DisplayZHTw` map、`DisplayZHAliases` legacy alias 解析、`SectorIDFromString`、`AllSectors`、`DisplayZH` | `internal/industry/sector.go` (新增 265 行) + `internal/industry/sector_test.go` (新增 187 行,15 條 contract test) |
| B - Representative stocks migrate | [#1163](https://github.com/kaecer68/atlas-go/pull/1163) | `DefaultRepresentativeStocks()` 回傳型別 `map[string][]string` → `map[SectorID][]string`;`ClassifyBySymbol` 回傳 `SectorID` 而非 Chinese string | `internal/industry/representative_stocks.go` (re-key 28 行,+0 net diff) |
| C - TWSE provider doc + drift guard | [#1162](https://github.com/kaecer68/atlas-go/pull/1162) | `twse_sector_index_provider.go` docstring 改寫為「canonical SectorID strings」,新增 `Test_TWSEMapping_AllValuesAreValidSectorIDs` 防 silent rename | `internal/marketdata/twse_sector_index_provider.go` (+1/-1) + `internal/industry/sector_test.go` (+20 行) |
| D - L2 + cycle.go migrate | [#1161](https://github.com/kaecer68/atlas-go/pull/1161) | 18 個 `SubIndustryXxx` L2 常數 + `Layer() / IsL1() / IsL2()` helper;`cycle.go` `defaultSeedMetrics` 改 `map[SectorID]IndustryMetrics` (5 L1 + 18 L2 = 23 entries) | `internal/industry/sector.go` (+114/-20) + `internal/industry/cycle.go` (+23/-25) + 8 條新 test |
| E - Frontend alignment | [#1160](https://github.com/kaecer68/atlas-go/pull/1160) | frontend `sector: '金融'` 改 `'金融保險'` 對齊 backend canonical;新增 `sector-display.js` `SECTOR_IDS` / `SECTOR_DISPLAY_ZH` / `displayZH()` mirror + 7 條 contract test | `shared_web/static/js/services/demo-data.js` (-1/+1) + `shared_web/static/js/shared/sector-display.js` (新增 75 行) + `__tests__/sector-display.test.mjs` (新增 92 行) |
| F - MCP tools | [#1164](https://github.com/kaecer68/atlas-go/pull/1164) | 新增 2 個 read-only MCP 工具,直接 import `internal/industry` 而非 REST proxy;tool 計數 assert 範圍 108-110 → **110-112** | `cmd/atlas-mcp/server/tools_industry_sector.go` (新增 132 行) + `server.go` / `tools.go` / `tools_transport_sse_test.go` bounds + `docs/reference/tool-catalog.md` 新增 section |

> [!NOTE]
> Phase 合併時間序列跟編號序不一定一致。最終 snapshot 是 main HEAD = `d1f53135`。Phase F 內文末段直接標註 `Part of FU-7 Sector Normalization` 並 cross-link 回本指南,這是本指南存在的最直接理由。

### Phase A→F 數據流

```text
                       ┌──────────────────────────────────────┐
   Phase A              │ internal/industry/sector.go          │
   (foundation)        │ 20 L1 SectorID + 18 L2 SubIndustry   │
   PR #1159            │ DisplayZHTw + DisplayZHAliases        │
                       │ AllSectors / SectorIDFromString       │
                       └───────────────────┬──────────────────┘
                                           │
                  ┌────────────────────────┼─────────────────────────┐
                  ▼                        ▼                         ▼
       Phase B                    Phase C                   Phase D
   PR #1163                  PR #1162                   PR #1161
   representative_           twse_sector_              cycle.go
   stocks 改 map              index_provider           defaultSeedMetrics
   [SectorID][]string        docstring + drift        map[SectorID]Industry-
                              guard test                Metrics (23 entries)
                                                        + L2 const
                                                        + Layer()/IsL1/2()
                  │
                  ▼
       ┌──────────────────────────────────┐
       │ canonical functions exposed:     │
       │ DefaultRepresentativeStocks()    │
       │ ClassifyBySymbol()               │
       │ AllSectors() / DisplayZH()       │
       └────────────────┬──────────────────┘
                        │
            ┌───────────┴────────────┐
            ▼                        ▼
   Phase E                    Phase F
   PR #1160                  PR #1164
   frontend demo-data.js     MCP tools
   + sector-display.js         industry_sector_list
   (mirrors canonical)         industry_sector_lookup
                               (MCP tool count 110→112)
```

## 4. Canonical Source of Truth

下面這張表告訴維護者「改 X 之前先看 Y」:

| 資源 | 唯一權威位置 | 對應衍生位置 (必須 mirror) |
|------|------------|--------------------------|
| 20 個 L1 sector ID 常數 | `internal/industry/sector.go` const block (line 36-57) | 無 |
| 18 個 L2 sub-industry ID 常數 | `internal/industry/sector.go` SubIndustryXxx const block (line 59-78) | 無 |
| L1 繁體中文 full label 對映 | `DisplayZHTw` map in `internal/industry/sector.go` (line 82-103) | frontend `SECTOR_DISPLAY_ZH` in `shared_web/static/js/shared/sector-display.js` (mirror) |
| L2 中文 label 對映 | `SubIndustryDisplayZHTw` map in `internal/industry/sector.go:156` | 無 (預設空 map,L2 沿用 ID 字串) |
| Legacy Chinese alias → canonical | `DisplayZHAliases` map in `internal/industry/sector.go` (line 113-147) | 無 |
| L1 representative symbols | `DefaultRepresentativeStocks()` in `internal/industry/representative_stocks.go` | 無 |
| Symbol → SectorID fallback 推斷 | `ClassifyBySymbol()` in `internal/industry/representative_stocks.go` | 無 |
| TWSE 8 個 index 對映 | `mapIndustryName` in `internal/marketdata/twse_sector_index_provider.go` (line 186-202) | 無 |
| Cycle tracker seed metrics | `defaultSeedMetrics()` in `internal/industry/cycle.go` | 無 |

### Enum layout 概念

```text
                    internal/industry/sector.go
                            │
        ┌───────────────────┴────────────────────┐
        ▼                                        ▼
 20 L1 SectorXxx                          18 L2 SubIndustryXxx
   SectorSemiconductor (="semiconductor")     SubIndustryAISupplyChain
   SectorFinancials      (="financials")       SubIndustryRobotics
   ...20 entries                              ...18 entries
        │                                        │
        ├──────────► DisplayZHTw[L1] = "中文"    │
        │                                        │
        ├──────────► DisplayZHAliases[legacy]    │
        │              = "金融" → SectorFinancials
        │                                        ▼
        │                              subIndustryIDs (runtime set)
        │                                        │
        └────────► Layer() returns "L1" / "L2"
                                            │
                                            ▼
                            DisplayZH(L2) returns Chinese label
```

### Helper API 一覽

| 函式 | 回傳 | 何時使用 |
|------|----|---------|
| `SectorID.IsValid()` | `bool` | 判斷此 SectorID 是否為合法註冊常數 (L1 + L2 皆算合法) |
| `SectorID.Layer()` | `"L1"` / `"L2"` / `"unknown"` | 區分父子層級,用於 UI 顯示或策略選擇 |
| `SectorID.IsL1()` / `SectorID.IsL2()` | `bool` | `Layer()` 的 shorthand |
| `SectorIDFromString(s)` | `(SectorID, bool)` | 解析任意輸入 (canonical ID / 中文 full label / legacy alias),miss 回 `("", false)` |
| `AllSectors()` | `[]SectorID` | 列出所有 canonical ID (L1 + L2),sorted ascending |
| `DisplayZH(id)` | `string` | L1 回中文 full label;L2 回中文 label（原為 ID passthrough，P3-2 補齊中文映射） |
| `DisplayZHTw[id]` | `string` | L1 繁中 label 對映 (read-only map) |
| `DisplayZHAliases[alias]` | `SectorID` | Legacy alias 反查 (read-only map) |

> [!NOTE]
> `SectorIDFromString` 解析順序固定為「exact canonical match → DisplayZHAliases 反查」。miss 一律回 `("", false)`,呼叫端需自行處理 unknown sentinel。

## 5. API / MCP exposure

Phase F 新增兩個 read-only MCP 工具,直接從 `internal/industry` import 計算 sector metadata,**不**走 REST proxy。

### Tool count 影響

依照 `cmd/atlas-mcp/server/AGENTS.md` 「工具計數」規範,任何 tool 新增都要 bump `server.Run()` 的 `RegisteredToolCount` assert 範圍:

| 變數 | Phase F merge 前 | Phase F merge 後 |
|------|---------------|---------------|
| 已註冊業務 tool handler 數 | 104 | 104 |
| `SamplingEnabled` / `ElicitationEnabled` 各 +1 | ≤ 106 | ≤ 106 |
| `registerAuditTools()` 額外 +4 | +4 | +4 |
| **`RegisteredToolCount` assert 範圍** | **108-110** | **110-112** |

兩個新 tool 掛進來後,`tools.go` 的 `countedAddTool` 自動累加,但 `server.go` 與 `tools_transport_sse_test.go` 的 upper bound 必須手動 bump,否則啟動時 assert fail。同期更新 `docs/reference/tool-catalog.md`,新增「Sector Canonical (2 tools)」section。

### industry_sector_list

無輸入參數,列出 38 個 sectors (20 L1 + 18 L2):

```jsonc
// Request
{"name": "industry_sector_list", "arguments": {}}
```

```json
// Response
{
  "sectors": [
    {"id": "semiconductor",   "display_zh": "半導體",          "stock_symbols": ["2330","2303","2454","3034","2379","3443","3661","2337","2344","8299","8081","6239"]},
    {"id": "electronics",     "display_zh": "電子零組件",       "stock_symbols": ["2317","2382","2324","2356","2357","2376","2377","2395","2308","3037"]},
    {"id": "financials",      "display_zh": "金融保險",         "stock_symbols": ["2881","2882","2886","2884","2891","2885","2892","5880","2883","2887"]},
    {"id": "ai_supply_chain", "display_zh": "ai_supply_chain",  "stock_symbols": []},
    {"id": "leo_satellite",   "display_zh": "leo_satellite",    "stock_symbols": []},
    {"id": "energy",          "display_zh": "油電燃氣",         "stock_symbols": ["6505","9933"]}
  ]
}
```

注意 L2 ID 的 `display_zh` 欄位直接等於 `id` 字串 (見 § 7 Trade-off (c))。

### industry_sector_lookup

兩種呼叫模式,可擇一或並用:

#### Mode 1 - 用 stock symbol 反查

```jsonc
// Request
{"name": "industry_sector_lookup", "arguments": {"symbol": "2330"}}
```

```json
// Response
{"found": true, "sector": {"id": "semiconductor", "display_zh": "半導體", "stock_symbols": [...]}}
```

#### Mode 2 - 用 sector 名稱 / canonical ID / legacy alias

```jsonc
// Request
{"name": "industry_sector_lookup", "arguments": {"sector": "金融"}}
```

```json
// Response
{"found": true, "sector": {"id": "financials", "display_zh": "金融保險", "stock_symbols": ["2881", ...]}}
```

`sector` 欄位三種輸入都可接受:canonical ID (`semiconductor`)、中文 full label (`半導體`)、legacy alias (`金融`、`金融保險類` 等)。

#### Miss 行為

```jsonc
{"name": "industry_sector_lookup", "arguments": {"symbol": "9999"}}
```

```json
{"found": false, "warning": "Symbol \"9999\" not found in representative stocks. Use industry_sector_list to see all sectors."}
```

空輸入 (symbol 與 sector 都缺) 同樣回 `found: false` + actionable warning,不 throw。

### 呼叫端選擇指引

| 情境 | 推薦呼叫 |
|------|---------|
| 「2330 是什麼產業?」 | `industry_sector_lookup({symbol: "2330"})` |
| 「金融保險類有哪些代表性個股?」 | `industry_sector_lookup({sector: "金融保險"})` |
| 「半導體相關 sub-industry 有哪些?」 | `industry_sector_list()`,前端過濾 `id.startsWith("ai_supply_chain")` 等 |
| 「列出所有中文 sector 名稱給 UI dropdown 用?」 | `industry_sector_list()`,前端 map 出 `{id → display_zh}` |

## 6. Breaking Changes / Migration

Phase A 採 additive 設計,**零 breaking change**。Phase B 改了 `DefaultRepresentativeStocks()` 與 `ClassifyBySymbol()` 的回傳型別,但兩者原本只被 `symbol_industry_mapper.go` 的內部文件 reference,production 沒有 active call site,因此屬於 effectively safe 但技術上是 type-level break (譯者:任何下游 import 此兩函式的程式必須跟著改回傳型別)。

### 仍用 Chinese string 的下游檔案 (legacy,可逐步收斂)

| 檔案 | 行 | 字串 | 建議動作 |
|------|----|----|---------|
| `internal/narrative/knowledge_base.go` | 51,55,59,66 | `"金融"`、`"半導體"` 等 alias 反查 | 已在 `DisplayZHAliases` 收下,但本檔仍硬編;考慮 v0.0.0.33+ 改 `industry.SectorIDFromString` lookup |
| `internal/narrative/templates.go` | 多處 | `"半導體"`、`"AI供應鏈"`、`"PCB"`、`"散熱"` 等 narrative theme 字串 | 屬 narrative **theme** namespace (與 sector canonical 同字但語意不同),**非 breaking** |
| `internal/sectorallocation/model_test.go` | 79,139,140,206 | `"半導體"`、`"金融"` 為 fixture name | 測試 fixture,可改可不改 |
| `shared_web/static/js/services/demo-data.js` | 25 (+ 其他 sector) | 修正後已對齊 canonical | 已於 Phase E 修正,後續若新增 demo symbol 應直接用 backend canonical 名 |

### 給外部 contributor 的 Safe Migration Path

```go
// ❌ Before:硬編 Chinese string 當 map key,symbol lookup 也是字串比對
sectors := map[string][]string{
    "半導體":   {"2330", "2454"},
    "金融保險":  {"2881", "2882"},
}
lookupStr := "金融"  // 不知會解析到哪個 SectorID

// ✅ After:canonical SectorID 為主,API 層只用 enum
sectors := industry.DefaultRepresentativeStocks()
secID, ok := industry.SectorIDFromString("金融保險")  // ok=true, secID=SectorFinancials
sym := industry.ClassifyBySymbol("2330")             // sym=SectorSemiconductor
label := industry.DisplayZH(secID)                    // "金融保險"
```

```js
// ❌ Before:frontend truncated alias 不對齊 backend canonical
{ symbol: '2881', name: '富邦金', sector: '金融' }

// ✅ After:對齊 backend DisplayZHTw 裡的全名
{ symbol: '2881', name: '富邦金', sector: '金融保險' }
```

```js
// 取得顯示標籤 (推薦取代 demo-data 內的 hardcoded sector string)
import { displayZH } from '../shared/sector-display.js';
const label = displayZH('financials');  // "金融保險"
```

### 檢查 migration 是否完整

```bash
# 1. 前端 mirror 與 backend canonical 一致 (sector-display.test.mjs 7 條 contract 鎖死)
node --test shared_web/static/js/__tests__/sector-display.test.mjs

# 2. canonical 數量正確 (L1 = 20, L2 = 18,合計 38)
go test -count=1 -v -run "Test_AllSectors_CanonicalCount|Test_L1Count|Test_L2Count" ./internal/industry/...

# 3. TWSE provider 8 個 entry 都能對到合法 SectorID (Phase C drift guard)
go test -count=1 -run "Test_TWSEMapping_AllValuesAreValidSectorIDs" ./internal/industry/...

# 4. MCP tool 數範圍仍正確 (Phase F bump 後 110-112)
go test -count=1 ./cmd/atlas-mcp/server/...
```

### 為何 Cycle.go 沒有先 migrate 就跑 Phase D

`cycle.go` 的 `defaultSeedMetrics` 在 Phase A 之前就已經用 snake_case English ID 字串 (例如 `"ai_supply_chain"`),這是 manual convention 而非 type-level contract。Phase D 把它升級為 `map[SectorID]IndustryMetrics`,把 type-level 保護補上;同時新增 L2 常數。如果未來 cycle.go 又冒出新的 L2 ID,test 會 fail,迫使 contributor 加進 `SubIndustryXxx` const block,避免 silent drift。

## 7. Trade-offs

三個有意選擇的設計取捨,給未來想改的人看:

### (a) 為什麼 MCP tools 直接 import `internal/industry`,不走 REST endpoint

sector taxonomy 是 **compile-time 常數** (`AllSectors()`、`ClassifyBySymbol()`、`DisplayZH()` 全是 in-memory function),不是 DB 查詢結果。走 REST 會多一圈 handler → HTTP → API router → handler → JSON → HTTP → unmarshal,latency 浪費,auditing 雜訊也增加。

但 canonical 仍透過 `Gateway` 規範 (`internal/apigateway/CONSTITUTION.md` 第 1 條):外部 API 必須走 `gateway.Fetch()`。sector 常數沒違反此條,因為它不是「外部資料源」,而是 Go process 內的 enum。`internal/apigateway/CONSTITUTION.md` 第一條只對外部 HTTP 資料抓取生效,純 in-memory 計算不受限。

### (b) 為什麼是 38 個 sectors (20 L1 + 18 L2),而不是 50 個或 100 個

20 L1 對齊 `DefaultRepresentativeStocks()` 既有的 20 個 inventory,且已經跟 TWSE 8 個 index + production display 對齊過。18 L2 從 `cycle.go` 既有的 sub-industry seed metrics 提煉,具體清單為:

```text
ai_supply_chain, robotics, consumer, industrial, foundry,
server_assembly, cooling, leo_satellite, satellite_rf_components,
satellite_pcb, ground_equipment, laser_communication, mining,
precious_metals_recycling, copper_industry, rare_earth_specialty,
metal_processing, etf_rotation
```

任意 bump number 都是 **deliberate taxonomy change**,必須走 PR + test bump + frontend mirror update,不能 silently 改。bump 之前先確認:

1. 新 L1 是否已經在 `DefaultRepresentativeStocks` 與 TWSE 兩條 path 出現?
2. 新 L2 是否已經在 `cycle.go` seed metrics 出現?
3. frontend `sector-display.js` mirror 是否要跟著加對應 entry?

### (c) 為什麼 L2 沒有 Chinese label (`DisplayZH` 直接回 ID 字串作為 fallback)

L2 sub-industries 是 **research / strategy 內部語意** (例如 `ai_supply_chain` 用於 cycle.go seed metrics、L2 cycle phase tracker),不是直接對投資人顯示的 sector 名稱。若有 caller 想顯示中文,得在 `SubIndustryDisplayZHTw` map 顯式 override;目前刻意保持空 map。

前端 `sector-display.js` mirror 也守住這個 L1-only 原則 (frontend 暫無 L2 顯示需求)。新增 L2 時建議先確認「是否確實是 internal-only 概念」,若對外要顯示中文化,再擴充 `SubIndustryDisplayZHTw` + frontend mirror。

## 8. References

### 合併的 PR

- [PR #1159](https://github.com/kaecer68/atlas-go/pull/1159) - Phase A (canonical foundation)
- [PR #1160](https://github.com/kaecer68/atlas-go/pull/1160) - Phase E (frontend alignment)
- [PR #1161](https://github.com/kaecer68/atlas-go/pull/1161) - Phase D (L2 + cycle.go)
- [PR #1162](https://github.com/kaecer68/atlas-go/pull/1162) - Phase C (TWSE provider doc + drift guard)
- [PR #1163](https://github.com/kaecer68/atlas-go/pull/1163) - Phase B (representative stocks migrate)
- [PR #1164](https://github.com/kaecer68/atlas-go/pull/1164) - Phase F (MCP tools)

### Canonical 程式檔案

- [`internal/industry/sector.go`](../../internal/industry/sector.go) - `SectorID` enum + `DisplayZHTw` + alias + L1/L2 helper (265 行)
- [`internal/industry/representative_stocks.go`](../../internal/industry/representative_stocks.go) - `DefaultRepresentativeStocks` + `ClassifyBySymbol` (170 行)
- [`internal/industry/cycle.go`](../../internal/industry/cycle.go) - L2 cycle tracker (`defaultSeedMetrics` map[SectorID])
- [`internal/marketdata/twse_sector_index_provider.go`](../../internal/marketdata/twse_sector_index_provider.go) - TWSE index 對映 (8 entries)
- [`cmd/atlas-mcp/server/tools_industry_sector.go`](../../cmd/atlas-mcp/server/tools_industry_sector.go) - MCP tools (132 行)
- [`shared_web/static/js/shared/sector-display.js`](../../shared_web/static/js/shared/sector-display.js) - frontend mirror

### 規範與指引

- [`../architecture.md`](../architecture.md) § Industry Ecosystem + Layer 2 Sector Desks
- [`../reference/traps.md`](../reference/traps.md) § 跨模組陷阱 - JSON tag 大小寫、Replay 格式
- [`../../internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) - 為什麼 sector 常數不走 Gateway (它不是外部資料源)
- [`../../cmd/atlas-mcp/server/AGENTS.md`](../../cmd/atlas-mcp/server/AGENTS.md) - MCP 工具計數規範
- [`../reference/tool-catalog.md`](../reference/tool-catalog.md) - MCP tool 公開名單 (含 Phase F 新增 2 個)
- [`../../.github/instructions/go-core.instructions.md`](../../.github/instructions/go-core.instructions.md) - Go coding rules (enum 用 string type,介面小而聚焦)

### 同主題相關文件

- [`adding-sector-agents.md`](adding-sector-agents.md) - 新增 sector agent 的做法 (Layer 2 結構)
- [`../specs/llm-sector-agent.md`](../specs/llm-sector-agent-spec.md) - L2.3 PoC LLM-driven sector agent spec
