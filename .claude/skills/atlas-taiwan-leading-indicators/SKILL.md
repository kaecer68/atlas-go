---
name: atlas-taiwan-leading-indicators
description: "Use when working with the 4 core leading indicators for short-term Taiwan stock direction (1-3 day). Triggers: ForeignInvestorNet, TSMADR, NVDA, DXY, data source tracing for short-term judgment, MacroDataSnapshot field usage."
---

> **核心組合**：4 個最精簡的觀察窗口，對短線 1~3 日台股方向判斷效果極佳
> **設計原則**：資金（外資）+ 產業（台積電/輝達）+ 流動性（DXY）三軸交叉驗證
> **資料完整性**：4 個指標全部已就緒於 `marketdata.MacroDataSnapshot`

## 描述

**4 核心短線指標** — 涵蓋絕大多數由資金與產業驅動的台股盤勢變化，是實戰中相當高勝率的觀盤基礎。

## 任務觸發

當 AI 代理需要：
- 為心法庫設計短線判斷條件
- 在 MacroDataSnapshot 中查找 4 指標欄位
- 排除單一指標的誤判（如 NVDA 跌但 TSMADR 漲）
- 設計「核心 4 指標即時條帶」UI

## 4 核心指標詳細規格

### 1. 外資現貨買賣超（ForeignInvestorNet）

- **數據源**：TWSE T86 法人買賣超日報
- **Channel ID**：`twse_capital_flow`
- **Provider**：`TWSECapitalFlowProvider`（`internal/marketdata/`）
- **MacroDataSnapshot 欄位**：`ForeignInvestorNet.Value`（單位：新台幣元；建議 UI 換算為「億元」）
- **更新頻率**：每日收盤後 ~18:00 CST
- **判斷邏輯**：
  - 連 3 日買超 → 短線偏多確立
  - 連 3 日賣超 → 短線偏空確立
  - 單日大於 ±100 億 → 訊號強度極高
- **陷阱**：
  - 月底、季底會有「作帳」效應，須注意
  - 除權息旺季（6~8 月）外資買賣超會被股息影響
  - 央行匯市干預時，外資買超可能假性放大

### 2. 台積電 ADR（TSMADR）

- **數據源**：Yahoo Finance TSM（US 上市 ADR）
- **Channel ID**：`tsm_adr`
- **Provider**：`YahooMacroProvider`（macro batch）
- **MacroDataSnapshot 欄位**：`TSMADR.Value`（美元股價）、`TSMADR.ChangePct`（每日漲跌幅，daily change — 非年增率）
- **更新頻率**：美股盤中即時（9:30-16:00 EST = 22:30-05:00 CST）
- **判斷邏輯**：
  - TSMADR 漲 > 0.3% → 台股開盤多半高開
  - TSMADR 跌 > 0.5% → 開盤低開機率高
- **陷阱**：
  - ADR 漲跌以美元計價，未考慮匯率；新台幣升值時 ADR 漲幅可能高於現貨
  - 美股盤後（after-hours）不計入，但現貨開盤前會參考

### 3. 輝達股價（NVDA）

- **數據源**：Yahoo Finance NVDA
- **Channel ID**：`us_nvda`
- **Provider**：`YahooMacroProvider`
- **MacroDataSnapshot 欄位**：`NVDA.Value`（美元股價）、`NVDA.ChangePct`（漲跌幅）
- **更新頻率**：美股盤中即時
- **判斷邏輯**：
  - NVDA 漲 > 0.5% → 台灣 AI 供應鏈（散熱、PCB、電源、組裝）正面訊號
  - NVDA 跌 > 1% + SPX 同步跌 → 系統性風險
  - NVDA 財報公布日 → 衝擊波極大
- **陷阱**：
  - 與 SOX 指數高相關但不等同，須兩者交叉確認
  - NVDA 個股事件（如 Blackwell 出貨延遲）可能與大盤脫鉤

### 4. 美元指數（DXY）

- **數據源**：Yahoo Finance DX-Y.NYB
- **Channel ID**：`us_yahoo`（macro batch）
- **Provider**：`YahooMacroProvider`
- **MacroDataSnapshot 欄位**：`DXY.Value`（指數值）、`DXY.ChangePct`
- **更新頻率**：美股盤中即時
- **判斷邏輯**：
  - DXY < 105 且連 2 日下行 → 美元轉弱，新台幣升值壓力，外資匯入買超
  - DXY > 106 且連 2 日上行 → 美元走強，外資可能抽離
- **陷阱**：
  - DXY 與 10Y 美債殖利率並非完全正相關（QE/QT 期間有背離）
  - 日圓急升時 DXY 漲幅可能有限（避險貨幣輪動）

## 4 指標組合心法

### 高勝率同步觸發

| 外資 | TSMADR | NVDA | DXY | 機率方向 |
|------|--------|------|-----|---------|
| 連 3 買 | 漲 | 漲 | < 105 ↓ | **85% ↑**（強多訊號）|
| 連 3 賣 | 跌 | 跌 | > 106 ↑ | **85% ↓**（強空訊號）|
| 中性 | 漲 | 漲 | 中性 | **60% ↑**（產業訊號主導）|
| 中性 | 跌 | 跌 | 中性 | **60% ↓**（產業訊號主導）|

### 矛盾訊號（警告）

- 外資買超但 DXY 強勢 → 短期續漲但注意美元轉強後外資抽離風險
- NVDA 漲但 TSMADR 跌 → NVDA 個股事件，非系統性，看其他科技股
- DXY 弱但 SPX 跌 → 風險情緒差，看 VIX 是否 > 25

## 與 MacroDataSnapshot 對應

```go
// 4 核心指標在 MacroDataSnapshot 中的欄位
type MacroDataSnapshot struct {
    // ... 其他 28 個欄位
    ForeignInvestorNet MacroDataPoint  // 外資現貨
    TSMADR             MacroDataPoint  // 台積電 ADR
    NVDA               MacroDataPoint  // 輝達
    DXY                MacroDataPoint  // 美元指數
    // ... 其他 24 個欄位
}

// MacroDataPoint 結構
type MacroDataPoint struct {
    Value     float64
    ChangePct float64
    Timestamp int64
}
```

## 與 strategy_techniques 整合

**核心 4 指標的 Condition 設計範例**（已實作於 production seeds）：

```json
{
  "id": "nvidia-tsmadr-confirm",
  "layer": "L3",
  "conditions": [
    {"field": "NVDA.ChangePct", "operator": "gt", "value": 0.5, "timeframe": "1D"},
    {"field": "TSMADR.ChangePct", "operator": "gt", "value": 0.3, "timeframe": "1D"}
  ]
}
```

**前端 4 指標條帶設計**（`strategies.js` + `decision-chain.js`）：

```javascript
const coreIndicators = {
  foreign_capital_net_twd: snap.ForeignInvestorNet.Value,
  tsm_adr_pct:             snap.TSMADR.ChangePct,
  nvda_pct:               snap.NVDA.ChangePct,
  dxy_pct:                snap.DXY.ChangePct,
};
```

## 4 指標的限制

1. **短線為主**：對 1~3 日台股方向判斷佳，中長期（週、月）須加入其他因子
2. **單一指標不可靠**：4 指標同步變動才具高勝率；單獨使用任一指標誤判率高
3. **黑天鵝失效**：地緣政治重大事件（如台海軍演）會使 4 指標暫時失效
4. **數據延遲**：除美股盤中外，台股收盤後到隔日開盤前有 12+ 小時空窗

## 擴充性

- 4 指標可加為 5、6 指標：人民幣、銅、費半、20Y 美債等（見 `atlas-macro-narrative` 第八節 Rolling Calibration）
- 但**核心 4 指標**為設計上的最小集，加更多會降低判斷速度
- 若需擴充，須保留「核心 4」為主視覺，新指標放次層

## 與其他技能整合

- `atlas-strategy-techniques`：4 指標是 L1~L5 心法的觸發條件
- `atlas-macro-narrative`：4 指標的更宏觀脈絡（外資流向模型、Carry Trade 等）
- `atlas-risk-management`：4 指標為 RiskGate 的輸入之一
- `atlas-event-driven-weights`：4 指標變化觸發因子權重調整

## 驗證要求

```bash
go test ./internal/marketdata/...                   # Provider 測試
go test ./internal/strategy_techniques/...         # 心法命中測試
go test ./internal/strategy_techniques/... -run TestEval     # 4 指標在 NumericFields 中
```

## 設計原則

1. **核心 4 指標不變**：可加不可減；新增指標放次層
2. **同步變動優先**：單一指標變動 < 4 指標同步變動
3. **Dot notation 統一**：`ForeignInvestorNet.Value` 而非 `ForeignInvestorNetValue`
4. **Value vs ChangePct**：短線看 ChangePct，中長期看 Value 走勢
5. **不取代基本面**：4 指標為短線判斷工具，個股基本面仍須獨立分析

---

*技能版本: 1.0*
*最後更新: 2026-06-11*
*適用對象: Atlas-Go AI Agent*
