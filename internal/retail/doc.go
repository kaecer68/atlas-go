// Package retail provides RSI-tw (Retail Sentiment Index — Taiwan), a composite retail
// investor sentiment calculation engine for Taiwan equity markets.
//
// It computes a final sentiment score from three weighted components:
//   - Part A (40%): Margin imbalance, day-trading ratio, VIX mapping, and proxies
//   - Part C (25%): Institutional net flow, futures OI, and ETF subscription proxies
//   - Part D: Adjustment factor (0.8–1.2) from geopolitical risk, VIX spikes, and credit events
//
// Maturity: evolving
package retail
