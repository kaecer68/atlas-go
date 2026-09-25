# Per-stock 產業欄位基質（per-stock industry substrate）規格

> **狀態**：已實作（2026-09-25，issue #1943 剩餘瓶頸）
> **範圍**：`symbol_industry` 第一方 channel、`symbol_industry` DB 欄位、`industry.substrate_from_symbol_industry_enabled` gate 與其兩個生產消費端。
> **上游契約（只讀，不修改）**：`internal/sectormap/twse_industry_code.go`（namespace `twse_industry_code`，36 個 2 位數字碼，22 mapped / 14 unmapped+reason，覆蓋 20/20 canonical L1；#1958）。

## 1. 問題陳述

#1951 統一了產業命名空間、#1958 補上「上游產業碼 → canonical L1」的權威詞彙，但
**#1943 真正的瓶頸是資料母體**：

- DB 沒有任何 per-stock 產業欄位。
- `symbol → canonical L1` 只能由硬編碼代表股清單回答：
  `internal/industry/representative_stocks.go`（20 L1 / ~96 檔）與
  classification tree 的 `representative_stocks`（生產 27 檔，`universe_snapshot.json`
  實證 `symbols_built=27`）。
- 27 檔約等於全體上市櫃的 **1.4%** ⇒ 任何產業層統計（命中率、配置、預測）都不顯著。

因此本規格交付的是**母體**，不是新的分類法。

## 2. 資料流（單向、可稽核）

```
TWSE opendata/t187ap03_L  (上市, 產業別)
TPEx mopsfin_t187ap03_O   (上櫃, SecuritiesIndustryCode)
        │
        ▼  marketdata.SymbolIndustryProvider.FetchAll
data/state/symbol_industry.json          ← channel 的持久化產物（單一真相檔案）
        │  （entries 依代號排序；atomic tmp+rename；免 key、零 FinMind 配額）
        ▼  cmd/atlas auto_symbol_industry 任務（5 分鐘 tick + 每日 gate）
symbol_industry 表（Postgres SSoT / CLI-dev 走 job-local SQLite）
        │  internal/symbolindustry.Store（backend-aware, 禁硬編碼 SQLite 路徑）
        ▼  cmd/atlas storeSymbolIndustrySubstrate（gate on 才安裝）
industry.SymbolIndustrySubstrate
        ├── composition.Root.SymbolL1Mapper      → sector exposure / 配置
        └── monitoring.UniverseBuilderDeps        → SmartUniverse 母體 + canonical L1
```

## 3. 契約

### 3.1 channel `symbol_industry`

| 欄位 | 值 | 理由 |
|---|---|---|
| `SourcePriority` | `["TWSE", "TPEx"]` | 兩家交易所官方，第一方；無付費來源 |
| `ExpectedRefresh` | 24h | 上游公司產業碼表以「出表日期」標記，至多每日變動 |
| `HealthSource` | `file_state` | 健康＝磁碟上有可用快照，不是「上游 ping 得到」 |
| `SuccessCriteria` | `value_nonzero` | 檔存在不足以證明有母體 |
| `DegradedOnEmpty` | `true` | #1953 規矩：空/缺檔一律 degraded，不得回 `ok` |

快照形狀（`data/state/symbol_industry.json`）：
`channel`（必為 `symbol_industry`）、`updated_at`、`sources`、`counts`
（`total/mapped/unmapped/unknown/canonical_l1`）、`l1_counts`、
`unmapped_codes`、`unknown_codes`、`entries[]`。
`entries[]` 每列：`symbol/company_name/market/industry_code/industry_name_zh/canonical_l1/mapping_status/mapping_reason/source/as_of`。

**映射紀律**：逐碼走 `sectormap.Resolve(NamespaceTWSESIndustryCode, code)`。
`mapped` 才填 `canonical_l1`；`unmapped`（14 碼）與 `unknown`（未宣告碼，實測 `91`＝TDR）
一律 `canonical_l1=""` 並帶 reason，**不猜、不用 candidates 自動補**。

### 3.2 DB 欄位

migration `000024_symbol_industry`（dual-dialect：SQLite 端由 store 的 `ensureTable` 建立）。
讀取一律經 `symbolindustry.NewStore(ctx, cfg.StoreBackend, pool, workDir)`：
`postgres` + 非 nil pool ⇒ Postgres（production SSoT）；其餘 ⇒ job-local SQLite。
**禁令**：不得在 CLI/job 內硬編碼 `data/state/atlas.db`。

### 3.3 gate 與消費端

```
configs/parameters.json → industry.substrate_from_symbol_industry_enabled   (2026-09-25 起預設 true；issue #1971 業主簽核，觀察窗 20 sessions，違反不變式即回滾為 false)
```

- **off（預設）**：`newSymbolIndustrySubstrate` 回 `nil`；`NewSubstrateIndustryMapper(nil)`
  回傳原 mapper；`gatherAllSymbols` 完全不看 substrate；`SymbolL1Mapper.ResolveL1`
  走原表。⇒ 與改動前逐位元等同。
- **on**：substrate 於 **兩個**生產消費端生效：
  1. `composition.Root.SymbolL1Mapper`（`ResolveL1`；`ResolveL1WithSource` 可回報來源）
     —— sector exposure／配置。
  2. `SmartUniverseBuilder`：`gatherAllSymbols` 以 `substrate.Symbols()` 為母體；
     `SubstrateIndustryMapper.GetClassification` 以 canonical L1 回答。
- **fallback 而非取代**：substrate 沒有的代號仍由代表股表回答 ⇒ 覆蓋率只增不減。
- **fail-closed**：載入失敗或表為空時 `ResolveL1` 回 `ok=false`，consumer 落回原表；
  不會出現「為了填滿市場而猜產業」。

### 3.4 消費證據（#1944 教訓）

`industry.RegisterSymbolIndustryConsumer(label)` / `ResetSymbolIndustryConsumers()` /
`SymbolIndustryConsumerWired()`。wiring 在安裝 substrate 時註冊
（`cmd/atlas.sector_exposure`、`cmd/atlas.universe_builder`、`cmd/atlas.build-universe`）。
測試另以 substrate 的 `lookups` 計數釘住「真的被呼叫」，避免「安裝了但沒人問」的 inert 寫入。

## 4. 覆蓋率（2026-09-24/25 上游實測）

| 指標 | 值 |
|---|---|
| 上市（TWSE t187ap03_L） | 1095 |
| 上櫃（TPEx mopsfin_t187ap03_O） | 893 |
| 去重後母體 | **1988**（0 重疊） |
| 可達 canonical L1 | **1599** |
| unmapped（14 碼） | 379 |
| unknown（未宣告碼 `91`＝TDR） | 10 |
| canonical L1 觸及 | **20 / 20** |

L1 分佈（檔數）：electronics 319、semiconductor 207、biotech 159、machinery 117、
optoelectronics 116、other_electronics 95、telecom 92、construction 88、textiles 52、
tourism 51、steel 48、auto 44、chemicals 42、financials 40、shipping 34、food 33、
plastics 25、retail 18、energy 12、cement 7。

母體變化：**27 → 1599**（gate on；gate off 維持 27）。

## 5. 未覆蓋項（明示，不回報不修）

- 14 個 unmapped 碼（玻璃陶瓷、造紙、橡膠、綜合、其他業、電子通路、資訊服務、文創、
  農技、綠能、數位雲端、運動休閒、居家生活、legacy 電子工業）在 canonical taxonomy
  沒有可辯護的單一節點；依 #1943/#1958 決議維持 unmapped，由 `unmapped_codes` 呈現。
- `91`（TDR）不在 namespace K 的 36 碼內，屬**上游漂移**：僅回報，不映射（若需納入，
  應另開 issue 更新 namespace 表）。
- 本 PR 只放大母體，**不改** `symbol_industry` 之外任何產業欄位；命中率
  (`stocktools.ResolveCanonicalL1Industry`) 仍走原 `ClassifyBySymbol` 路徑（後續工作）。

## 6. 驗證方式

```bash
# gate off：既有測試全綠、輸出與合併前一致
make ci-gate
go test ./internal/{marketdata,apigateway,symbolindustry,industry,monitoring,config}/... ./cmd/atlas/... -count=1

# gate on：母體變大（端到端）
#   internal/monitoring TestBuildUniverse_SubstrateGrowsPopulation
#   cmd/atlas        TestNewSymbolIndustrySubstrate_GateOnLoadsPerStockField（印出 27 → N）

# 真實上游覆蓋率
go run ./cmd/experimental/industry-namespace-audit   # namespace K unmapped 仍為 14 且有 reason
```
