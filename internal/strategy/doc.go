// Package strategy provides dynamic strategy selection and risk-parity allocation.
//
// Core components:
//
//	Registry          — Built-in strategies: all_weather, growth, value, defensive, momentum
//	Selector          — Regime-aware strategy picker with cooling-off period
//	ComparisonEngine  — Per-strategy Sharpe / drawdown / win-rate tracking
//	StrategyAllocator — Risk-parity weighting (inverse volatility) with [5%, 50%] bounds
//
// Selector behavior:
//   - shouldSwitch() enforces MinSwitchInterval to prevent churn
//   - No regime match → falls back to all_weather; missing all_weather → "fallback"
//   - ComparisonEngine scoring: Sharpe*0.4 + DailyReturn*30*0.3 + WinRate*0.3
//     (returns 0.5 when sample history < configured days)
//   - estimateVolatility() returns 0.20 (annualized default) when < 5 samples
//
// --- 策略賺錢邏輯與證實假設 ---
//
// all_weather（全天候）
//
//	賺錢假設：台股長期存在正風險溢價（equity risk premium），分散持有可
//	穿越牛熊。不擇時也不擇股，靠資產配置的風險平價本質獲取基礎回報。
//	適用宏觀狀態：RISK_ON / RISK_OFF / NEUTRAL（全狀態）
//	失效條件：系統性風險事件（全球金融危機）使 β 趨近零的極端狀況；
//	長期盤整市（∼0% 年化）無 alpha 可捕獲。
//	風險等級：Balanced
//
// growth（成長動能）
//
//	賺錢假設：台股存在最多 6 個月的中期動能效應（momentum factor），
//	配合 AI 供應鏈結構性成長（NVDA/TSMC 資本支出週期）形成雙因子溢酬。
//	適用宏觀狀態：RISK_ON（資金流入成長股）、NEUTRAL（結構性成長仍可表現）
//	失效條件：盤勢急轉為 RISK_OFF 時動能因子大幅回撤；AI 泡沫破裂
//	（如 2022 年的估值修正）會令成長股雙殺。
//	風險等級：Aggressive
//
// value（價值投資）
//
//	賺錢假設：台股存在約 1.5~2 年的價值均值回歸（value mean-reversion）；
//	低本益比/高殖利率標的在估值修復過程中有超額報酬。
//	Quality 因子提供下檔保護（篩選 ROE >15% 且現金流穩健者）。
//	適用宏觀狀態：RISK_ON（估值擴張期）、NEUTRAL（防禦性價值輪動）
//	失效條件：長期負利率環境使價值因子失效（成長股永遠溢價）；
//	台股結構性改變（如半導體主導導致傳統產業長年折價）。
//	風險等級：Conservative
//
// defensive（防禦型）
//
//	賺錢假設：RISK_OFF 環境中高品質低波動股具備抗跌優勢（downside
//	protection），透過選股避開高 β / 高負債股，以相對收益為目標。
//	適用宏觀狀態：RISK_OFF（下跌市）、NEUTRAL（防禦性配置保護）
//	失效條件：全市場齊跌（系統性殺盤）無法以選股避險；
//	V 型反轉時防禦股的落後幅度會顯著擴大。
//	風險等級：Conservative
//
// momentum（純動能）
//
//	賺錢假設：Jegadeesh & Titman (1993) 的 cross-sectional momentum 在
//	台股略約 3~6 個月最有效；以過去 6 個月扣除最近 1 個月（skip-month）
//	的報酬排序，買強賣弱。
//	適用宏觀狀態：RISK_ON（動能溢酬最顯著）
//	失效條件：盤勢 V 型反轉（momentum crash）；高度波動的整理盤
//	（whipsaw）會使動能信號頻繁反轉；交易成本（滑價/稅費）侵蝕淨報酬。
//	風險等級：Aggressive
//
// Maturity: evolving
package strategy
