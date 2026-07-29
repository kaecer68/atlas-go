# B5 偵察報告：PeriodIndicators 死欄位補齊評估

> **狀態**：唯讀偵察（禁止修改任何檔案）
> **日期**：2026-07-29
> **背景**：B2 確認 PeriodIndicators 37 欄位中 adapter 僅填 12，25 個死輸入以零值參與求值。B2 已在 API 標記 `input_available`。

---

## Q1. 死欄位總表（25 欄位，非 B2 宣稱的 18）

> **注意**：PeriodIndicators struct 實際有 **37 個欄位**（非文件所述 ~30）。Adapter 填充 **12 個**，**25 個死欄位**。B2 報告稱 30 欄位 / 18 死，與程式碼現況不符（推測 struct 在報告後擴充了）。

### 已填充欄位（12）— 來源：`SnapshotToPeriodIndicators` + `snapshotToPeriodIndicators`

| 欄位 | 來源 | MacroDataSnapshot 欄位 |
|------|------|----------------------|
| `VIX` | snapshot | `VIX.Value` |
| `DXY` | snapshot | `DXY.Value` |
| `US10Y` | snapshot | `US10Y.Value` |
| `SOXPrice` | snapshot | `SOXIndex.Value` |
| `TSMADRPrice` | snapshot | `TSMADR.Value` |
| `TAIEXPrice` | snapshot | `TAIEX.Value` |
| `ForeignSingleDayNet` | snapshot | `ForeignInvestorNet.Value` |
| `ForeignFuturesOI` | snapshot | `ForeignFuturesOINet.Value` |
| `MarginBalance` | snapshot | `RetailMarginBalance.Value` |
| `MarginMaintenanceRatio` | snapshot | `MarginMaintenanceRatio.Value` |
| `MarketVolume` | snapshot | `MarketVolume.Value` |
| `DayTradeRatio` | snapshot | `DayTradeRatio.Value` |

### 死欄位（25）— 按分類

| # | 欄位 | 語義 | 需要輸入 | 影響的時期 / 偵測條件 |
|---|------|------|---------|---------------------|
| 1 | `SOXMA50` | SOX 50 日均線 | SOX 過去 50 日收盤價序列 | turnaround_down (price<MA50), turnaround_up (break above MA50) |
| 2 | `SOXMA20` | SOX 20 日均線 | SOX 過去 20 日收盤價序列 | turnaround_up (20 crosses above 50) |
| 3 | `TSMADRHigh5` | TSM ADR 近 5 日高點 | TSM ADR 過去 5 日 OHLC | turnaround_up (price>5d high + >2%) |
| 4 | `ForeignNet5DayAvg` | 外資近 5 日均值 | ForeignInvestorNet 過去 5 日序列 | downturn (sell slowing ratio), plateau (3d/10d ratio) |
| 5 | `ForeignNet10DayAvg` | 外資近 10 日均值 | ForeignInvestorNet 過去 10 日序列 | plateau (3d < 50% of 10d) |
| 6 | `ForeignNetPeakSell` | 前波賣超峰值 | 外資過去 N 日最大賣超值 | downturn (5d avg / peak ratio) |
| 7 | `ForeignBuyDays10` | 近 10 日買超天數 | 外資每日淨買賣超正負號 | bull (7+/10), consolidation (both >3) |
| 8 | `ForeignSellDays10` | 近 10 日賣超天數 | 同 #7 | consolidation (both buy & sell > 3) |
| 9 | `ForeignConsecBuyDays` | 連續買超天數 | 外資每日淨買賣超正負號 | turnaround_up (≥3 days or single day >100 億) |
| 10 | `ForeignConsecSellDays` | 連續賣超天數 | 同 #9 | turnaround_down (≥3 days + heavy sell >150 億) |
| 11 | `ForeignFuturesOIPrev` | 前期貨未平倉 | 昨日期貨 OI | turnaround_down (delta), turnaround_up (delta) |
| 12 | `ForeignFuturesOIDelta3` | 期貨連續增減天數 | 期貨 OI 過去 3 日增減方向 | plateau (declining 3+ days) |
| 13 | `TWDChange1D` | 新台幣單日變動% | USD/TWD 昨日 vs 今日 | black_swan (depreciation >X%), turnaround_up (appreciation > X%), turnaround_down |
| 14 | `TWDChange3D` | 新台幣 3 日變動% | USD/TWD 過去 3 日序列 | turnaround_up (>0.5% appreciation) |
| 15 | `TWDChange5D` | 新台幣 5 日變動% | USD/TWD 過去 5 日序列 | consolidation (range-bound ±0.5%) |
| 16 | `TWDMA20` | 新台幣 20 日均線 | USD/TWD 過去 20 日均值 | turnaround_down (TWD weaker than MA20), consolidation |
| 17 | `TAIEXMA5` | 加權 5 日線 | TAIEX 過去 5 日收盤價 | downturn (price>MA5 & <MA20) |
| 18 | `TAIEXMA20` | 加權 20 日線 | TAIEX 過去 20 日收盤價 | black_swan (crash from MA20), downturn, bull, plateau |
| 19 | `TAIEXMA20Slope` | 20 日線斜率 | TAIEX MA20 近期趨勢 | bull (positive slope required) |
| 20 | `MarketVolumeMA20` | 20 日均量 | 成交量過去 20 日均值 | consolidation (volume 70-100% of MA20) |
| 21 | `MarginBalancePeak` | 融資高峰 | 融資餘額歷史最高點 | downturn (decline >15% from peak) |
| 22 | `MarginBalanceChange5D` | 融資近 5 日變動% | 融資餘額過去 5 日變化 | bull (mild increase <1%/day) |
| 23 | `PublicBankConsecBuyDays` | 公股連續買超天數 | 公股券商每日買賣超正負號 | downturn (≥5 consecutive days) |
| 24 | `SectorRotationFlag` | 類股輪動旗標 | 產業指數過去 5 日 return_pct 前 3 vs 前 5 日前 3 比較（18 個產業，`data/state/sector_index/`，56 天歷史） | plateau, consolidation |
| 25 | `NationalFundActive` | 國安基金進場 | 政策性事件宣告 | black_swan (any trigger fires) |

---

## Q2. 歷史資料基礎設施盤點

### 2.1 Macro Snapshot 每日歷史

| 屬性 | 值 |
|------|-----|
| 儲存位置 | `data/state/macro/YYYY-MM-DD.json`（86 個檔案） |
| 起始日 | 2026-04-25（~3 個月） |
| 可查詢介面 | `MacroService.ListSnapshotsInRange()` — `internal/monitoring/service/macro.go:169` |
| 結構 | 完整 `MacroDataSnapshot` JSON，含 TAIEX/SOX/TSMADR/DXY/USD_TWD/ForeignInvestorNet/MarketVolume 等 |
| 上限 | 365 天（CF-MS-02） |

**正宗路徑結論**：對 TAIEX/SOX/TSMADR/DXY/USD_TWD/ForeignInvestorNet 等 macro snapshot 欄位的歷史計算，**標準路徑**為 `ListSnapshotsInRange() → 逐日讀取對應欄位 → 計算均線/變化率**。

### 2.2 Capital Flow 日序列

| 屬性 | 值 |
|------|-----|
| 儲存位置 | `data/state/capital_flow/YYYYMMDD.json`（每日個檔） |
| 滾動窗口 | `capital_flow_rolling.json`（`FileRollingSampleStore`） |
| 可查詢介面 | `RollingSampleStore.History()` — `internal/capitalflow/rolling_store.go:107` |
| 容量 | 252 日（per-dimension） |
| 可用維度 | 7 個 force dimensions（foreign/institutional/dealer/government/insurance/retail_margin/retail_short） |
| 資料起始日 | 2025-05-19 起（現存 300+ 日檔案） |

**正宗路徑結論**：外資買賣超的歷史均值/買賣天數/峰值計算應優先使用 rolling store（252 日 window，比 macro snapshot 歷史更長）。macro snapshot 的 `ForeignInvestorNet.Value` 同樣可用，但歷史較短（86 天）。

### 2.3 Rolling Sample Store（Z-score）

| 屬性 | 值 |
|------|-----|
| 儲存位置 | `capital_flow_rolling.json` |
| 可查詢介面 | `RollingSampleStore.History(dimension, beforeDate, limit)` |
| 結構 | per-dimension `[]RollingSample{TradingDate, RawValue, ZScore}` |
| 容量 | 252 天 |

### 2.4 Margin Balance 日歷史

| 屬性 | 值 |
|------|-----|
| 儲存位置 | `data/state/margin/YYYYMMDD_margin.json` |
| 可查詢介面 | `LoadMarginHistory()` — `internal/narrative/margin_history_loader.go:35` |
| 結構 | `MarginHistoryEntry{MarginBalance, ShortBalance, MaintenanceRatio, ChangePct}` |
| 最低天數 | 30 天（`marginHistoryAvailable()`） |
| 計算函數 | `ComputeRollingPercentile()`, `ComputeRollingAcceleration()` |

### 2.5 跨市場指數價格歷史

| 資產 | 日歷史來源 | 狀態 |
|------|----------|------|
| TAIEX | `data/state/macro/YYYY-MM-DD.json` 中的 `TAIEX.Value` | ✅ 86 天 |
| SOX | `data/state/macro/YYYY-MM-DD.json` 中的 `SOXIndex.Value` | ✅ 86 天 |
| TSM ADR | `data/state/macro/YYYY-MM-DD.json` 中的 `TSMADR.Value` | ✅ 86 天 |
| NVDA | `data/state/macro/YYYY-MM-DD.json` 中的 `NVDA.Value` | ✅ 86 天 |
| DXY | `data/state/macro/YYYY-MM-DD.json` 中的 `DXY.Value` | ✅ 86 天 |
| USD/TWD | `data/state/macro/YYYY-MM-DD.json` 中的 `USD_TWD.Value` | ✅ 86 天 |

> **無專用 QuoteStore**：`QuoteStore`（`internal/ledger/quote_store.go`）僅儲存個股 daily bars，**不含指數**。Indices 的歷史必須從 macro snapshot 檔案讀取。

### 2.6 公股買賣超日序列

| 屬性 | 值 |
|------|-----|
| 儲存位置 | `data/state/government_flow/YYYYMMDD.json` |
| 資料粒度 | 僅 `total_net`（每日總淨額），**無每日 per-broker 正負號** |
| 檔案數 | 7 個（2026-07-21 起） |
| Broker aggregator | `GovernmentBrokerAggregator.Run()` 產出單一 `total_net` |
| 連續買超天數計算 | **目前不可行** — 需擴充 aggregator 寫入 per-broker buy/sell 旗標 |

---

## Q3. 逐欄位裁決

### A 類：可從現有歷史計算（19 欄位）

| 欄位 | 計算方式 | 資料來源 | 起始日 | 備註 |
|------|---------|---------|--------|------|
| `SOXMA50` | SOXIndex.Value 過去 50 日均值 | macro snapshots | 需累積 50 個交易日（~2.5 月），現有 86 天足夠 | |
| `SOXMA20` | SOXIndex.Value 過去 20 日均值 | macro snapshots | 需累積 20 個交易日，現已滿足 | |
| `TSMADRHigh5` | TSMADR.Value 過去 5 日最高值 | macro snapshots | 立即可用 | |
| `ForeignNet5DayAvg` | ForeignInvestorNet.Value 過去 5 日均值 | macro snapshots 或 rolling store | 立即可用 | 優先用 rolling store（更長歷史） |
| `ForeignNet10DayAvg` | ForeignInvestorNet.Value 過去 10 日均值 | 同上 | 立即可用 | |
| `ForeignNetPeakSell` | ForeignInvestorNet.Value 滾動窗口最小值 | rolling store (252 天) 或 macro snapshots (86 天) | 立即可用 | 取負值中絕對值最大者 |
| `ForeignBuyDays10` | 近 10 日 ForeignInvestorNet.Value > 0 的天數 | macro snapshots | 立即可用 | |
| `ForeignSellDays10` | 近 10 日 ForeignInvestorNet.Value < 0 的天數 | macro snapshots | 立即可用 | |
| `ForeignConsecBuyDays` | 從今日往前數連續 >0 的天數 | macro snapshots | 立即可用 | |
| `ForeignConsecSellDays` | 從今日往前數連續 <0 的天數 | macro snapshots | 立即可用 | |
| `ForeignFuturesOIPrev` | 昨日 ForeignFuturesOINet.Value | macro snapshots | 立即可用 | 取 dated snapshot 前一天 |
| `ForeignFuturesOIDelta3` | 期貨 OI 過去 3 日增減方向（正負號） | macro snapshots | 需 3 天歷史 | |
| `TWDChange1D` | (今日 USD_TWD - 昨日) / 昨日 × 100 | macro snapshots | 立即可用 | 正=貶值，負=升值 |
| `TWDChange3D` | (今日 USD_TWD - 3 日前) / 3 日前 × 100 | macro snapshots | 立即可用 | |
| `TWDChange5D` | (今日 USD_TWD - 5 日前) / 5 日前 × 100 | macro snapshots | 立即可用 | |
| `TWDMA20` | USD_TWD.Value 過去 20 日均值 | macro snapshots | 20 天後可用 | |
| `TAIEXMA5` | TAIEX.Value 過去 5 日均值 | macro snapshots | 立即可用 | |
| `TAIEXMA20` | TAIEX.Value 過去 20 日均值 | macro snapshots | 現已滿足 | |
| `TAIEXMA20Slope` | MA20 近 5 日線性回歸斜率 | macro snapshots | 需 MA20 history + 5 天 | 簡單線性回歸 |
| `MarketVolumeMA20` | MarketVolume.Value 過去 20 日均值 | macro snapshots | 現已滿足 | |
| `MarginBalancePeak` | 融資餘額歷史最高值 | `LoadMarginHistory()` | 30 天後可用 | 若歷史不足，使用 rolling 最大值 |
| `MarginBalanceChange5D` | (今日 - 5 日前) / 5 日前 × 100 | `LoadMarginHistory()` | 立即可用 | margin_history 已有 ChangePct 欄位 |
| `SectorRotationFlag` | 近 5 日 return_pct 前 3 產業 vs 前 5 日的前 3 是否不同 | `data/state/sector_index/sector_indices_*.json`（18 個產業，56 天歷史） | 需 10 天 | 現有 sector_index 資料由 `industry.SiliconTracker` 週期更新 |

> **總計 A 類：23 欄位**（含 SectorRotationFlag）

### B 類：Provider 有能力但未接入（1 欄位）

| 欄位 | 現有能力 | 最小接入點 | 備註 |
|------|---------|----------|------|
| `PublicBankConsecBuyDays` | `GovernmentBrokerAggregator` 已取得 TWSE broker-level data | Aggregator 需擴充輸出 per-broker daily buy/sell flag | 目前僅產出 `total_net`，需改 aggregator 記錄八大公股行庫每日買賣超方向 |

### C 類：完全無源（1 欄位）

| 欄位 | 說明 | 查證過程 |
|------|------|---------|
| `NationalFundActive` | 國安基金宣布進場 | 政策性事件，無結構化資料源。可能的接入方式：RSS 新聞監控（類似 geopolitical channel）或 operator 手動標記。目前不建議自動化。 |

### 裁決總結

| 分類 | 欄位數 | 佔比 |
|------|--------|------|
| A（可計算） | 23 | 92% |
| B（Provider 擴充） | 1 | 4% |
| C（無源） | 1 | 4% |

---

## Q4. 填充點與降級設計

### 4.1 填充點位置

**現有 adapter 位置**（兩個，需同步修改）：
- `internal/monitoring/dashboard_api.go:1695` — `SnapshotToPeriodIndicators()`（dashboard 路徑）
- `internal/orchestrator/executor_pipeline.go:240` — `snapshotToPeriodIndicators()`（simulation 路徑）

**架構建議**：A 類欄位的計算**不應放在 adapter 內**。應建立獨立的 `PeriodIndicatorsCalculator`：

```go
// internal/portfolio/period_calculator.go（新建）
type PeriodIndicatorsCalculator struct {
    macroSvc     *service.MacroService       // for ListSnapshotsInRange
    marginLoader func(string) ([]MarginHistoryEntry, error)  // LoadMarginHistory
}

// Compute enriches a base PeriodIndicators (already populated with 12 single-day fields)
// with rolling-window A-class fields.
func (c *PeriodIndicatorsCalculator) Compute(ctx context.Context, base PeriodIndicators, tradingDate string) PeriodIndicators
```

**理由**：
1. 兩個 adapter 已有重複邏輯 — 合併到 calculator 消除重複
2. `ListSnapshotsInRange` 是 I/O 操作 — 獨立 component 方便測試與 mock
3. 降級邏輯（歷史不足 N 日）應集中在 calculator，避免散落兩處
4. 符合現有架構慣例：`divergence_detector.go` 有 `LoadMarginHistory()` 獨立函數，`rolling_store.go` 有獨立 store

### 4.2 降級設計

| 情境 | 策略 | 理由 |
|------|------|------|
| 歷史不足 N 日（如 MA20 需 20 天，但只有 10 天） | 欄位維持零值 → `input_available=false` | 不允許「短均線充數」— 10 日均值 ≠ 20 日均值，會讓 downturn/turnaround_down 條件假性觸發或假性未觸發 |
| 歷史剛好夠 N 日（如第 20 天） | 正常計算 MA20 | 資料品質與更長歷史的 MA20 等效（只是樣本較小） |
| 歷史超過 N 日 | 使用 rolling window（最近 N 個交易日） | 標準 rolling MA |
| 非交易日（週末/假日） | 跳過非交易日，僅計交易日 | macro snapshots 不含非交易日檔案，自然跳過 |
| `NationalFundActive` 永遠零值 | 永遠 `input_available=false` | C 類欄位 — 不影響其他條件，僅 black_swan 兩個 trigger 中的一個不可用 |

**降級對 `input_available` 的語義**：
- 目前 B2 標記 `input_available=false` 是因為 25/37 欄位為零值
- 補齊 A 類後，`input_available` 應分欄位標記（非全域布林）— 每個欄位有自己的「歷史天數滿足」狀態
- 建議 `PeriodIndicators` 增加 `FieldAvailable map[string]bool` 或每個欄位配 `*float64`（nil=不可用）
- **本 PR 不建議改 struct schema** — 先補齊 A 類計算，C 類維持零值，在 API 回應層標記

### 4.3 行為變更影響評估

補齊 A 類欄位後，以下時期的判定**可能改變**：

| 時期 | 影響的條件 | 變更方向 |
|------|----------|---------|
| **black_swan** | TWDChange1D（台幣重貶）、TAIEXMA20（偏離 20 日線>5%） | 新增更多觸發路徑，正確性提升（目前 TAIEXMA20=0 時條件被跳過） |
| **turnaround_down** | SOXMA50、ForeignConsecSellDays、ForeignFuturesOIPrev、TWDMA20、TWDChange1D | 從「永遠不滿足」→「有機會滿足」，「全條件必須通過 3+」的門檻現在可以被真實觸發 |
| **downturn** | ForeignNet5DayAvg、ForeignNetPeakSell、MarginBalancePeak、TAIEXMA5/MA20、PublicBankConsecBuyDays | 同上 |
| **turnaround_up** | ForeignConsecBuyDays、TWDChange1D/3D、SOXMA20/MA50、TSMADRHigh5、ForeignFuturesOIPrev | 同上 |
| **bull** | ForeignBuyDays10、MarginBalanceChange5D、TAIEXMA20/Slope | 同上 |
| **plateau** | ForeignNet5DayAvg/10DayAvg、ForeignFuturesOIDelta3、TAIEXMA20、SectorRotationFlag | 同上 |
| **consolidation** | ForeignBuyDays10、ForeignSellDays10、TWDChange5D、TWDMA20、MarketVolumeMA20、SectorRotationFlag | 同上 |

**本質變更**：目前 7 個時期中有 6 個**實際上幾乎不可能觸發**（因為關鍵條件依賴死欄位，而 N-of-M 門檻需要 2-3 個條件同時滿足）。補齊後所有時期變為可觸發，**這是行為變更，不是 bugfix**。

**黃金測試策略**：
1. 用現有 macro snapshot 歷史（86 天）對已知市場事件日期回測：
   - 2026-04-25 附近（歷史起始，有限數據）
   - 2026-05 月（關稅衝擊期，預期 downturn 或 turnaround_down）
   - 2026-06 月（反彈期，預期 turnaround_up 或 bull）
2. 與現有 production `PeriodHistory`（已存在於 `period_history` DB 表）對比：補齊後的判定是否比現有更合理？
3. 建議：補齊後以 `input_available` flag 作為 rollout gate — 先在 dashboard 顯示新舊兩套結果，確認合理後再切換

---

## Q5. 施工排序建議

### Batch 1：核心價格均線（影響 6/7 時期，低成本）

| 欄位 | 依賴 | 預估改動 |
|------|------|---------|
| `TAIEXMA5`, `TAIEXMA20`, `TAIEXMA20Slope` | macro snapshots (20 天) | `PeriodIndicatorsCalculator` + 兩個 adapter 改為呼叫 calculator |
| `SOXMA50`, `SOXMA20` | macro snapshots (50 天) | 同上 |
| `TSMADRHigh5` | macro snapshots (5 天) | 同上 |
| `MarketVolumeMA20` | macro snapshots (20 天) | 同上 |
| `TWDChange1D`, `TWDChange3D`, `TWDChange5D` | macro snapshots (5 天) | 同上 |
| `TWDMA20` | macro snapshots (20 天) | 同上 |

**改動檔案**：
- `internal/portfolio/period_calculator.go`（新建）
- `internal/monitoring/dashboard_api.go:1695`（改為呼叫 calculator）
- `internal/orchestrator/executor_pipeline.go:240`（同上）

**成本**：~150 行（calculator）+ 兩處 adapter 各 ~5 行變更 + 測試

**生效日**：2026-08-15（TAIEX/SOX 歷史累積 110+ 天，遠超 MA50 所需）

### Batch 2：外資行為指標（影響 5/7 時期，中成本）

| 欄位 | 依賴 | 預估改動 |
|------|------|---------|
| `ForeignNet5DayAvg`, `ForeignNet10DayAvg` | macro snapshots 或 rolling store | calculator |
| `ForeignNetPeakSell` | rolling store (252 天) | calculator |
| `ForeignBuyDays10`, `ForeignSellDays10` | macro snapshots (10 天) | calculator |
| `ForeignConsecBuyDays`, `ForeignConsecSellDays` | macro snapshots | calculator |
| `ForeignFuturesOIPrev` | macro snapshots (1 天) | calculator |
| `ForeignFuturesOIDelta3` | macro snapshots (3 天) | calculator |
| `MarginBalancePeak` | `LoadMarginHistory()` | calculator |
| `MarginBalanceChange5D` | `LoadMarginHistory()` | calculator |

**改動檔案**：同 Batch 1 + `internal/narrative/margin_history_loader.go`（可能需要匯出更多型別）

**成本**：~100 行 calculator 擴充 + 測試

**優先使用 rolling store**（252 天 > 86 天 macro snapshot），macro snapshot 作為 fallback。

### Batch 3：特殊接入（1-2 欄位，高成本/需外部依賴）

| 欄位 | 方案 | 預估改動 |
|------|------|---------|
| `SectorRotationFlag` | 讀取 `data/state/sector_index/` 18 產業每日 return_pct，計算近 5 vs 前 5 top-3 差異 | calculator ~30 行 + sector_index reader |
| `PublicBankConsecBuyDays` | 擴充 `GovernmentBrokerAggregator` 輸出 per-broker 買賣超方向 | `government_broker_aggregator.go` + calculator |
| `NationalFundActive` | C 類，不建議自動化 | 無 |

> **修正**：`SectorRotationFlag` 從 C 類重新分類為 A 類 — `data/state/sector_index/` 有 18 產業每日 `return_pct`，56 天歷史（2026-06-03 起），可計算輪動旗標。

## 總結

| 指標 | 值 |
|------|-----|
| 死欄位總數 | 25（非 B2 報告的 18） |
| A 類（可從現有歷史計算） | 23（92%） |
| B 類（Provider 擴充） | 1（4%） |
| C 類（無源） | 1（4%） |
| 推薦施工批次 | 3 batch，優先 Batch 1（均線） |
| 行為變更風險 | 高 — 6/7 時期從「幾乎永遠不觸發」變為「可觸發」 |
| 需要黃金測試 | 是 — 建議對已知歷史事件回測 |
| 推薦 rollout | `input_available` per-field flag + 新舊兩套並行顯示 |
