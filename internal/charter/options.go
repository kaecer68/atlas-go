// Package charter — Phase C stepwise charter A/B controls and statistics.
//
// Phase C2 wired the charter layers behind the single ATLAS_CHARTER_MODE
// flag. Phase C3 (this package) adds per-arm control of the five charter
// switches so the backtest Runner can A/B each layer incrementally:
//
//	arm 1 PeriodOnly       → 7-period detection fills ExecutionContext.Period
//	arm 2 +StrategyFilter  → Advisor.AllowedStrategies gating of raw recs
//	arm 3 +MacroFlow       → macroflow conviction scaling
//	arm 4 +CashReserve     → period cash reserve in the sim engine
//	arm 5 +ConvictionFloor → periodized conviction floor in the sim engine
//
// The package also hosts the A/B statistics (paired t-test + BCa bootstrap,
// stats.go) and the per-day recommendation pipeline trace used for
// attribution (trace.go).
//
// Maturity: E (evolving) — new module, API may adjust.
package charter

import "github.com/kaecer68/atlas-go/internal/domain"

// Options are the five stepwise charter switches. Each switch activates one
// incremental charter layer; the C3 arms are cumulative (see StepwiseArms).
type Options struct {
	PeriodOnly      bool // period detection → ExecutionContext.Period
	StrategyFilter  bool // Advisor.AllowedStrategies gating of raw recs
	MacroFlow       bool // macroflow conviction scaling
	CashReserve     bool // period cash reserve (sim engine override)
	ConvictionFloor bool // periodized conviction floor (sim engine override)
}

// Enabled reports whether any charter switch is on.
func (o Options) Enabled() bool {
	return o.PeriodOnly || o.StrategyFilter || o.MacroFlow || o.CashReserve || o.ConvictionFloor
}

// Names returns the enabled switch names in arm order (used in reports).
func (o Options) Names() []string {
	all := []struct {
		on   bool
		name string
	}{
		{o.PeriodOnly, "PeriodOnly"},
		{o.StrategyFilter, "StrategyFilter"},
		{o.MacroFlow, "MacroFlow"},
		{o.CashReserve, "CashReserve"},
		{o.ConvictionFloor, "ConvictionFloor"},
	}
	names := make([]string, 0, len(all))
	for _, s := range all {
		if s.on {
			names = append(names, s.name)
		}
	}
	return names
}

// AllOn returns the full charter configuration — the behavior of
// ATLAS_CHARTER_MODE=true with no per-arm options (Phase C2 compatibility).
func AllOn() Options {
	return Options{
		PeriodOnly:      true,
		StrategyFilter:  true,
		MacroFlow:       true,
		CashReserve:     true,
		ConvictionFloor: true,
	}
}

// ArmNames returns the five C3 arm display names (stepwise).
func ArmNames() []string {
	return []string{
		"PeriodOnly",
		"+StrategyFilter",
		"+MacroFlow",
		"+CashReserve",
		"+ConvictionFloor",
	}
}

// StepwiseArms returns the five cumulative C3 arms. Each arm adds exactly one
// charter layer on top of the previous arm.
func StepwiseArms() []Options {
	return []Options{
		{PeriodOnly: true},
		{PeriodOnly: true, StrategyFilter: true},
		{PeriodOnly: true, StrategyFilter: true, MacroFlow: true},
		{PeriodOnly: true, StrategyFilter: true, MacroFlow: true, CashReserve: true},
		{PeriodOnly: true, StrategyFilter: true, MacroFlow: true, CashReserve: true, ConvictionFloor: true},
	}
}

// ConvictionFloorDelta returns the extra conviction floor (percentage points)
// applied on top of the base MinRecommendationConviction for a market period,
// per charter §C14/C17:
//
//	black_swan            → +20
//	RISK_OFF periods      → +10 (downturn, turnaround_down; black_swan uses +20)
//	all other periods     → 0
func ConvictionFloorDelta(period domain.MarketPeriod) int {
	switch period {
	case domain.PeriodBlackSwan:
		return 20
	case domain.PeriodDownturn, domain.PeriodTurnaroundDown:
		return 10
	default:
		return 0
	}
}
