package narrative

import "slices"

// RegimeVocabularyMapping describes the cross-walk between the two regime
// vocabularies that have ended up sharing the SQLite stress_index_history.regime
// and regime_history.regime columns:
//
//   - "Stress vocabulary" (live TaiwanStressCalculator): low / alert / high / crisis
//     — written by TaiwanStressCalculator.Calculate() based on the composite score's
//     relation to configurable thresholds (calibration.StressThresholdAlert/High/Crisis).
//
//   - "Regime vocabulary" (Janus multi-factor engine): RISK_ON / RISK_OFF / NEUTRAL /
//     TRANSITIONAL — written by report_generator.go and janus regime detection,
//     persisted via DashboardAPI.persistRegime (or its historical equivalents).
//
// The two systems measure different things: the stress vocabulary is a severity
// bucket of a single composite score, while the regime vocabulary is a categorical
// market mood derived from multiple macro factors. They are NOT 1:1 equivalent.
// The mapping below is the best-effort approximation we recommend to consumers
// who need to align data written by both systems; the Source column on
// TaiwanStressIndex and RegimeRow tells consumers which vocabulary a given row
// actually uses, and consumers who need exact equivalence should treat the two
// vocabularies as orthogonal rather than applying this mapping blindly.
//
// The mapping is intentionally explicit (not derived from score thresholds) so
// that future changes to either vocabulary can be reflected in one place.
var RegimeVocabularyMapping = map[string]string{
	// Stress → Regime
	"low":    "RISK_ON",
	"alert":  "NEUTRAL",
	"high":   "RISK_OFF",
	"crisis": "RISK_OFF",

	// Regime → Stress
	"RISK_ON":      "low",
	"RISK_OFF":     "high",
	"NEUTRAL":      "alert",
	"TRANSITIONAL": "alert",
}

// NormalizeRegime accepts any string from either vocabulary and returns its
// canonical (Regime vocabulary) form. Regime-vocabulary inputs are returned
// as-is; stress-vocabulary inputs are mapped to their regime equivalent.
// Unknown values pass through unchanged so future vocabulary additions don't
// get silently rewritten to a wrong bucket.
//
// Use this when joining data across the two systems into a single downstream
// pipeline that expects a single canonical vocabulary. Use the Source field
// to detect whether the row was originally a Stress or Regime vocabulary
// token if the original vocabulary matters.
func NormalizeRegime(in string) string {
	// Already canonical — no rewrite needed.
	if slices.Contains(RegimeVocabulary, in) {
		return in
	}
	if mapped, ok := RegimeVocabularyMapping[in]; ok {
		return mapped
	}
	return in
}

// StressVocabulary enumerates the four canonical stress-index regime labels.
var StressVocabulary = []string{"low", "alert", "high", "crisis"}

// RegimeVocabulary enumerates the four canonical regime-detector labels.
var RegimeVocabulary = []string{"RISK_ON", "RISK_OFF", "NEUTRAL", "TRANSITIONAL"}
