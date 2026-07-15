// Package eventdriven — Stage 5 PR#3 EventType → TriggerTheme 動態對應
//
// Background:
//
//	The legacy eventTypeToThemes() in predictor.go maps TaiwanEventType
//	(industry calendar events) to a set of calendar-specific theme names
//	(msci_rebalance, monthly_revenue, etc.). Those names do NOT match the
//	24-template trigger themes in narrative/templates.go (US_rates_up,
//	earnings_surprise, etc.), so the downstream themeMatchesAny() check
//	in predictor.Predict never fires — the calendar themes have no
//	overlap with InvestmentModel.ActiveThemes.
//
//	PR#3 fixes this by adding EventTypeToTriggerThemes(), which bridges
//	the two systems:
//	- Maps TaiwanEventType → the subset of our 24 trigger themes that
//	  make semantic sense for that calendar event type
//	- Filters the result by what's actually registered in the given
//	  DetectorRegistry (nil registry = return all defaults)
//
// Backward compatibility:
//
//	The legacy eventTypeToThemes() is left UNTOUCHED. Existing callers
//	(narrative_inject_test.go: TestEventTypeToThemes) keep working.
//	PR#3 is purely additive — the new function is consumed by the
//	PR#4 scheduler to know which detectors to pre-warm per event type.
package eventdriven

import (
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// eventTypeToTriggerThemesTable is the canonical mapping from
// industry.TaiwanEventType to our 24-template trigger themes.
//
// Theme selection rationale:
//   - EventSpringFestival   → spring_festival_season (Feb 15 .. Mar 1 window)
//   - EventExDividend       → dividend_season (Jul-Aug ex-dividend rush)
//   - EventDividendPayout   → dividend_season (overlaps with ex-dividend)
//   - EventWindowDressing   → year_end_window_dressing (Dec effect)
//   - EventElection         → election_cycle (Jan / pre-vote windows)
//   - EventMonthlyRevenue   → earnings_surprise (data drives surprise)
//   - EventFinancialReport  → earnings_surprise (quarterly reports)
//
// EventTypes NOT in this table (EventMSCIRebalance, EventTaiwan50Rebalance,
// EventFuturesSettlement, EventShareholderMeeting, EventInvestorConf,
// EventLongHoliday, EventPositionBuilding) have no direct trigger theme in
// the 24 templates — they are informational calendar events that do not
// match a macro narrative detector. Stage 6+ may add
// market_structure_change / futures_settlement / shareholder_meeting
// themes if data sources warrant.
var eventTypeToTriggerThemesTable = map[industry.TaiwanEventType][]string{
	industry.EventSpringFestival:      {"spring_festival_season"},
	industry.EventExDividend:          {"dividend_season"},
	industry.EventDividendPayout:      {"dividend_season"},
	industry.EventWindowDressing:      {"year_end_window_dressing"},
	industry.EventElection:            {"election_cycle"},
	industry.EventMonthlyRevenue:      {"earnings_surprise"},
	industry.EventFinancialReport:     {"earnings_surprise"},
	industry.EventPositionBuilding:    {"year_end_window_dressing"},
	industry.EventShareholderMeeting:  {"earnings_blackout"},
	industry.EventFOMCMeeting:         {"US_rates_up", "US_rates_down"},
	industry.EventBOJRateDecision:     {"JPY_carry_unwind"},
	industry.EventOPECMeeting:         {"oil_price_shock"},
	industry.EventCPIRelease:          {"inflation_spike"},
	industry.EventChinaGDPRelease:     {"china_slowdown"},
	industry.EventTaiwanExportRelease: {"taiwan_export_boom"},
	industry.EventEarningsBlackout:    {"earnings_blackout"},
	industry.EventTariffAnnouncement:  {"tariff_shock"},
}

// EventTypeToTriggerThemes maps a TaiwanEventType (as a string) to the
// subset of trigger themes that should be considered when this event type
// becomes imminent. Results are filtered against the given
// DetectorRegistry — themes that are not registered are excluded.
//
// Pass registry = nil to receive all default themes without filtering
// (useful for tests and for callers that don't have a registry wired up).
//
// Returned slice is always a fresh copy; callers may mutate it without
// affecting the underlying table.
//
// EventTypes without a direct mapping return nil; the PR#4 scheduler will
// simply not pre-warm any detector for them.
func EventTypeToTriggerThemes(eventType string, registry *narrative.DetectorRegistry) []string {
	defaults := eventTypeToTriggerThemesTable[industry.TaiwanEventType(eventType)]
	if len(defaults) == 0 {
		return nil
	}

	if registry == nil {
		// Return a copy so callers can mutate without affecting the table.
		out := make([]string, len(defaults))
		copy(out, defaults)
		return out
	}

	out := make([]string, 0, len(defaults))
	for _, theme := range defaults {
		if _, ok := registry.Get(theme); ok {
			out = append(out, theme)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MappedEventTypes returns the set of TaiwanEventType values that have at
// least one trigger theme mapping. Useful for tests, docs, and a future
// MCP tool that lists "what event types the system reacts to".
func MappedEventTypes() []industry.TaiwanEventType {
	out := make([]industry.TaiwanEventType, 0, len(eventTypeToTriggerThemesTable))
	for et := range eventTypeToTriggerThemesTable {
		out = append(out, et)
	}
	return out
}
