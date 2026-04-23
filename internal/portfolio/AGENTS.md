# internal/portfolio AGENTS.md

## OVERVIEW
`internal/portfolio` 負責台股投資組合的權重管理與因子計算，是系統「模擬優先」與「稽核導向」的核心。

---

## KEY CONCEPTS

### 1. Darwinian Weights (達爾文權重管理)
- **範圍限制**：權重強制夾制於 `[0.3, 2.5]`。`0.3` 為 whisper (低語)，`2.5` 為 shout (高喊)。
- **動態調整**：`PerformDailyAdjustment` 依據 **Rolling Sharpe Ratio** (20天) 進行分層調整：
    - **Top 1/3**: 權重提升 (`TopQuartileMultiplier=1.05`) + Performance Bonus。
    - **Bottom 1/3**: 權重調降 (`BottomQuartileMultiplier=0.95`) + Risk Penalty (若波幅過高)。
- **套用機制**：`ApplyDarwinianWeights` 將 Agent 推薦的 Conviction 乘上權重，結果限制在 `[1, 250]`。

### 2. FactorEngine (多因子計算引擎)
- **因子類型**：計算 Momentum (動能)、Value (價值)、Quality (品質) 與 Agent (代理人) 四類因子。
- **透明決策鏈 (Audit Trail)**：回傳 `FactorScoreBreakdown` 包含：
    - `Formula`: 實際計算公式字串。
    - `RawInputs`: 原始輸入數值 (如 P/E, P/B, 20d Volatility)。
    - `IsFallback`: 標記是否因資料缺失而使用猜測值。
- **Fallback 行為**：當歷史資料不足時，Momentum 會回退至 intraday return；Value/Quality 則回退至固定常數 (`0.1`/`0.05`)。

---

## ANTI-PATTERNS (高危陷阱)

- **Silent Clamping (靜默夾制)**：權重調整在 `constrainWeight` 中靜默完成，外部調用者若不檢查 `adjustments` 回傳值將無法得知是否觸碰邊界。
- **Ignoring IsFallback (忽略回退標記)**：在進行實驗評價 (Judge) 或 決策鏈審查時，必須檢查 `IsFallback`，否則會誤信低品質的估算數據。
- **Mutable Slice Reuse (切片重用)**：`ApplyDarwinianWeights` 會生成新的 `Recommendation` 切片，切勿直接修改傳入的原切片。
