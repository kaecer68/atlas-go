// Package methodology bridges the ATLAS_METHODOLOGY.md constitution with
// runtime strategy selection. The Advisor loads methodology_rules.yaml and
// answers period→strategy mapping queries.
//
// Maturity: E (evolving)
package methodology

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// Advisor wraps MethodologyRules and provides period-aware strategy queries.
// Zero-value is invalid; use NewAdvisor() or NewAdvisorFromPath().
type Advisor struct {
	rules *config.MethodologyRules
}

// NewAdvisor creates an Advisor from already-loaded rules.
func NewAdvisor(rules *config.MethodologyRules) *Advisor {
	if rules == nil {
		rules = config.TryLoadMethodologyRules("configs/methodology_rules.yaml")
	}
	return &Advisor{rules: rules}
}

// NewAdvisorFromPath loads methodology rules from a YAML file path.
func NewAdvisorFromPath(path string) *Advisor {
	return &Advisor{
		rules: config.TryLoadMethodologyRules(path),
	}
}

// AllowedStrategies returns the strategy IDs allowed for a given MarketPeriod,
// combining primary and secondary. Returns nil for unknown periods.
func (a *Advisor) AllowedStrategies(period domain.MarketPeriod) []string {
	return a.rules.GetAllowedStrategies(string(period))
}

// IsStrategyAllowed checks if a strategy ID is allowed in a given period.
func (a *Advisor) IsStrategyAllowed(period domain.MarketPeriod, strategyID string) bool {
	return a.rules.IsStrategyAllowed(string(period), strategyID)
}

// CashReserve returns the recommended cash reserve percentage for a period.
func (a *Advisor) CashReserve(period domain.MarketPeriod) float64 {
	return a.rules.GetCashReserve(string(period))
}

// MacroflowRiskLevel returns the macroflow risk level string for a period.
func (a *Advisor) MacroflowRiskLevel(period domain.MarketPeriod) string {
	return a.rules.GetMacroflowRiskLevel(string(period))
}

// StrategyCategory returns the category for a strategy ID.
func (a *Advisor) StrategyCategory(strategyID string) string {
	return a.rules.GetStrategyCategory(strategyID)
}

// FilterStrategies filters a list of strategy IDs, keeping only those
// allowed in the given period. Order is preserved.
func (a *Advisor) FilterStrategies(period domain.MarketPeriod, strategyIDs []string) []string {
	allowed := a.AllowedStrategies(period)
	if allowed == nil {
		return strategyIDs // unknown period → pass through
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = true
	}
	result := make([]string, 0, len(strategyIDs))
	for _, id := range strategyIDs {
		if allowedSet[id] {
			result = append(result, id)
		}
	}
	return result
}

// ─── Regime string → MarketPeriod mapping ─────────────────────────────

// RegimeToPeriod maps a three-state Regime string (RISK_ON/RISK_OFF/NEUTRAL)
// to the most likely MarketPeriod. This is a best-effort bridge for systems
// that only have access to the 3-state Regime. When a PeriodDetector is
// available, prefer using the detected MarketPeriod directly.
//
// Mapping:
//
//	RISK_ON  → bull (most likely during risk-on)
//	RISK_OFF → downturn (conservative default for risk-off)
//	NEUTRAL  → consolidation
func RegimeToPeriod(regime domain.Regime) domain.MarketPeriod {
	switch regime {
	case domain.RegimeRiskOn:
		return domain.PeriodBull
	case domain.RegimeRiskOff:
		return domain.PeriodDownturn
	default:
		return domain.PeriodConsolidation
	}
}
