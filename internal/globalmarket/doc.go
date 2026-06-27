// Package globalmarket provides global macroeconomic data management.
// Only Taiwan market is enabled by default; EnableMarket auto-spawns regional Agents.
//
// Key contracts:
//   - Default correlation = 0.5 (GetCrossMarketCorrelation / GetCorrelation)
//   - Symbol inference: .TW → TW, no suffix uppercase → US, .T → JP, .HK/.KS → Asia
//   - Exposure limits are logged as LimitBreach but do NOT block trading
//   - Diversification score = 1 - avg correlation
//
// Maturity: evolving
package globalmarket
