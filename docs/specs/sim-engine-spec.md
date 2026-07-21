# Sim Engine 模擬引擎規格

> **文件角色**：atlas-go 投資組合模擬引擎規格。
> **取代對象**：原 internal/sim/AGENTS.md（已遷移至此）。

`internal/sim` 執行投資組合模擬引擎：給定報價與建議，執行委託單、計算成交價、維持部位狀態。

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **`RunWithState()` 就地變異狀態** | `state *domain.SimulationState` 引數會就地修改。多次透過相同狀態呼叫 `RunWithState` 會疊加影響。每次模擬前**必須**呼叫 `domain.NewSimulationState()`。參見根 `AGENTS.md`「高危陷阱」第二項。 |
| **無 SlippageModel = 固定 SlippageBPS** | 若不呼叫 `WithSlippageModel()`，引擎使用 `constraints.SlippageBPS`。此靜態值對高成交量股票過於寬鬆，對低流動性股票過於嚴格。 |
| **Nil TaxCalculator 靜默跳過稅務** | 若 `taxCalc` 為 nil，引擎記錄警告並跳過稅務計算。最終 `SimulationResult` 的 PnL 為未稅，`FallbackEvents` 附 `"tax: nil calculator, skipping"`。 |
| **股息資料耦合** | 稅務計算器需要 `dividends` map（透過 `WithDividends()` 設定）。若遺漏，稅務調整使用 0 股息，導致低估稅務責任。 |
| **反身性規則就地變異 recs** | `reflexivity.Rule.Apply(recs, *state, ...)` 修改 `recs` slice 內容。多次模擬之間**絕對不能**共用同一份 `recs` slice。 |
| **動態閾值減少部位** | `DynamicThresholdEngine` 對高度相關的訊號套用相關性過濾，可能移除被判斷為冗餘的建議，導致實際持倉少於預期。 |
| **買入執行順序（legacy 路徑）** | `executeLegacyBuys` 對建議排序：`Conviction` 降冪 → `Symbol` → `Agent` → `Reason`。此順序是確定性的；optimizer 路徑則由 optimizer 決定。 |
| **`DynamicThresholdEngine` 重複符號過濾** | 若 `thresholdEngine` 已設定，`RunDay` 會過濾重複符號的建議，保留信心度最高者。若預期同符號有不同方向的建議，信心度較低者會被跳過。 |
| **無 `MarketImpactModel` = 忽略大單衝擊** | 若不呼叫 `WithMarketImpactModel()`，大額訂單相對於 ADV 仍會以收盤價成交，導致回測績效過度樂觀。建議在流動性較差的標的上啟用。 |

---

## 核心執行流程

```
Engine.Run(regime, quotes, recs)
  → dayResult := Engine.RunDay(state, time, regime, quotes, recs)
    1. 套用反身性規則（就地變異 recs）
    2. 現有部位依市價評估
    3. 透過 thresholdEngine 過濾重複符號
    4. 依信心度與可用現金執行買入
    5. 依退出訊號執行賣出
    6. 計算成交價與滑價
    7. 套用稅務（若已設定）
  → SimulationResult { Orders, Trades, Positions, EndingCash, ... }
```

**關鍵**：`Run()` 建立新的 `SimulationState`；`RunWithState()` 使用現有狀態。多日期回測一律使用 `RunWithState`；單次執行使用 `Run`。
