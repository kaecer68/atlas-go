# Decision Chain: Financial Investment Strategy Review

## 1. 盤勢判定 (Regime Detection)
**機制**: 多證據源加權評分（macro events + narrative themes）
**評估**: ✅ 合理。使用 ensemble 方式綜合多個來源，每個證據源有自己的信心度權重。這是業界常見的 regime classification 手法。

**建議**: 可加入 regime 轉換的 hysteresis（遲滯區間）避免頻繁切換。

## 2. 代理推薦 (Agent Recommendations)
**機制**: Executor plugins 各自產出 conviction + factor scores
**評估**: ✅ 合理。多策略並行、各自評分，類似 fund of funds 的概念。

**注意**: Conviction (0-100) 是唯一的信心度量。現實中應考慮各策略的歷史 Sharpe、最大回撤、當前勝率來動態調整 conviction 可信度。DarwinianWeightManager 有做一部分（rolling Sharpe 調整權重），但未反映在 reasoning trace 中。

## 3. 控制層過濾 (CRO/CIO)
**機制**: 
- CRO: Z-score 標準化基於 MAE + conviction floor
- CIO: 加權聚合、去重合併 unique symbols

**評估**: ✅ 合理。使用 MAE (Median Absolute Error) 做 Z-score 標準化比標準差更穩健（對 outlier 不敏感）。Conviction floor 確保低信念 recommendation 不會消耗計算資源。

**潛在問題**: 
- Z-score 標準化假設常態分布，但推薦信念值通常是離散且偏態的
- CIO 的去重邏輯（merge by symbol）沒有考慮到同 symbol 但不同 agent 的 conviction variance

## 4. 組合構建 (Portfolio Build)
**機制**: Capital allocator + sizer (Kelly/volatility/ATR/liquidity/anti-correlation)
**評估**: ✅ 合理。涵蓋了 Kelly 公式、波動率調整、ATR 停損、流動性限制——業界標準做法。

**不足**: 
- 推理鏈 (reasoning trace) 中 portfolio_build 階段**沒有具體的決策軌跡**——看不到每個標的的倉位計算過程
- 雖然 sizer 有完整的計算邏輯，但 reasoning trace 只顯示到 CIO 過濾，沒有 portfolio_build 的 trace 事件

## 總結

| 階段 | 合理性 | 風險 |
|------|--------|------|
| 盤勢判定 | ✅ 合理 | Regime hysteresis 不足 |
| 代理推薦 | ✅ 合理 | Conviction 未考慮歷史績效權重 |
| 控制層過濾 | ✅ 合理 | Z-score 常態假設 |
| 組合構建 | ✅ 合理 | Trace 缺倉位計算細節 |

**整體**: 決策鏈設計在金融投資策略上是合理的，涵蓋了從宏觀判定到個股推薦、風險控制、組合構建的完整流程。主要缺口是 reasoning trace 在 portfolio_build 階段缺乏具體計算軌跡，以及 synthetic mode 下 forward return 的可靠性問題。
