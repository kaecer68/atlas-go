// Package narrative provides macro narrative event detection, causal chain
// derivation, and Taiwan stress index calibration for the atlas-go system.
//
// Data flow:
//
//	MacroIngestor (MarketData)
//	  → NarrativeEvent
//	  → KnowledgeBase (match DefaultTemplates)
//	  → CausalChain
//
// The Bundle API at /api/narrative/bundle exposes events, causal chains,
// models, and seasonality analysis to the frontend narrative page.
//
// News Sentiment source limitation: Finnhub News Sentiment API only covers
// US equities. Taiwan narrative analysis uses three workarounds: (1) US broad
// market as proxy (NASDAQ, S&P 500), (2) foreign capital flow inference
// (US down + VIX up → Taiwan outflow), (3) TWSE open data as local sentiment.
//
// NarrativeEvent fields:
//   - Theme (e.g. US_rates_up, AI_capex_surge)
//   - Confidence [0,1] + ConfidenceSource
//   - HitRate MUST come from hitRateForTheme() (never computed manually)
//   - SourceData (mandatory, original trigger values)
//   - Duration / ExpiresAt
//   - Severity (low/medium/high/critical → ±5/10/20/30% factor weight adjustment)
//   - Status: active → confirmed → faded → expired (managed by EventLifecycleManager)
//
// State transitions: active (confidence > threshold) → confirmed (2+
// independent sources) → faded (elapsed > Duration × 0.8) → expired (elapsed
// > ExpiresAt). Duplicates update existing active events' confidence rather
// than creating new ones.
//
// RegimeChange triggers: VIX > 30 (HighVol), VIX > 25 with downward trend
// (Bear), VIX < 15 with upward trend (Bull), critical severity events, or
// StressIndex > 80.
//
// SeasonalBridge implements NarrativeLinkageProvider with 5 built-in theme
// multipliers (oil_price_shock, AI_capex_surge, US_rates_up,
// JPY_carry_unwind, geopolitical_risk_spike). Without configuration it
// degrades to an empty list.
//
// Taiwan stress index uses hybrid signal max(|level_z|, |change_pct|).
// FactorBaseline stores only Mean+Count (z-score is computed on read).
// ValidateCalibration failures do not write new config (current day uses
// old). Calibration parameters come from ParametersConfig (no hardcode).
// calibration_enabled defaults to false; staging validation ≥ 30 days
// required before enabling.
//
// Maturity: stable
package narrative
