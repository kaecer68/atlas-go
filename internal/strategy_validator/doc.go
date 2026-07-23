// Package strategy_validator 提供策略歷史回測驗證與績效指標計算。
//
// Validator 接受策略的每日報酬序列，計算年化報酬、最大回撤、Sharpe 比率、
// 勝率以及與加權指數的相關係數（Pearson），輸出結構化的策略驗證報告。
//
// 驗證報告整合自下列來源：
//   - strategy.ComparisonEngine（現有交易紀錄與初步指標）
//   - domain/shared.ComputeSharpe（年化 Sharpe，使用 TWSE 交易日頻率 sqrt(243)）
//   - 自行計算的年化報酬換算與 Pearson 相關係數
//
// 使用範例：
//
//	v := strategy_validator.NewValidator()
//	report := v.Validate("momentum", dailyReturns, taiexReturns)
//	// report.AnnualizedReturn, report.SharpeRatio, report.TaiexCorrelation ...
//
// Maturity: evolving
package strategy_validator
