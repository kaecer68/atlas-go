// Package realtime provides sub-second market data streaming and real-time
// regime detection, with dynamic Agent weight adjustment based on detected
// regime.
//
// Core components:
//
//	RealTimeAdapter   — Master controller: data ingestion, regime detection, weight adjustment
//	RegimeDetector    — 7-type fine-grained regime classification (calm, volatile, trending_up, ...)
//	MarketDataPoint   — Single market observation (price, volume, bid/ask, timestamp)
//	RegimeType        — Local fine-grained enum (distinct from domain.Regime's 3-type coarse enum)
//	RealTimeStats     — Monitoring stats: symbol count, agent count, regime distribution, avg confidence
//
// Detection rules:
//   - DataWindowSize defaults to 60; DetectRegime() needs >= 30 points, otherwise
//     returns RegimeCalm
//   - Weight changes are bounded: |delta| <= MaxWeightChange (default 0.5) and
//     floor at MinWeight (default 0.1)
//   - Regime confidence = 1 - volatility/threshold (inverse relationship)
//
// ApplyToRecommendation multiplies the original conviction by the realtime
// weight and clamps the result to [1, 100]. GetAgentWeight() returns the
// default 1.0 for unregistered agent-symbol pairs without erroring.
//
// Maturity: evolving
package realtime
