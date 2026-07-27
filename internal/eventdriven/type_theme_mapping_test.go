// Package eventdriven — Stage 5 PR#3 tests for EventTypeToTriggerThemes.
package eventdriven

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// minimalDetector is a stub satisfying narrative.Detector for tests that
// only need the registry to know about a theme (not actual detection).
type minimalDetector struct {
	theme   string
	enabled bool
}

func (d *minimalDetector) Theme() string                                   { return d.theme }
func (d *minimalDetector) Enabled() bool                                   { return d.enabled }
func (d *minimalDetector) SetEnabled(b bool)                               { d.enabled = b }
func (d *minimalDetector) PeriodWeight(period domain.MarketPeriod) float64 { return 1.0 }
func (d *minimalDetector) Detect(_ context.Context, _ narrative.DetectorInput) (*narrative.DetectionResult, error) {
	return nil, nil
}

// TestEventTypeToTriggerThemes_NilRegistry_ReturnsDefaults verifies the
// canonical mapping for every EventType that has a known trigger theme.
func TestEventTypeToTriggerThemes_NilRegistry_ReturnsDefaults(t *testing.T) {
	cases := []struct {
		eventType industry.TaiwanEventType
		want      []string
	}{
		{industry.EventSpringFestival, []string{"spring_festival_season"}},
		{industry.EventExDividend, []string{"dividend_season"}},
		{industry.EventDividendPayout, []string{"dividend_season"}},
		{industry.EventWindowDressing, []string{"year_end_window_dressing"}},
		{industry.EventElection, []string{"election_cycle"}},
		{industry.EventMonthlyRevenue, []string{"earnings_surprise"}},
		{industry.EventFinancialReport, []string{"earnings_surprise"}},
		{industry.EventPositionBuilding, []string{"year_end_window_dressing"}},
		{industry.EventShareholderMeeting, []string{"earnings_blackout"}},
		{industry.EventFOMCMeeting, []string{"US_rates_up", "US_rates_down"}},
		{industry.EventBOJRateDecision, []string{"JPY_carry_unwind"}},
		{industry.EventOPECMeeting, []string{"oil_price_shock"}},
		{industry.EventCPIRelease, []string{"inflation_spike"}},
		{industry.EventChinaGDPRelease, []string{"china_slowdown"}},
		{industry.EventTaiwanExportRelease, []string{"taiwan_export_boom"}},
		{industry.EventEarningsBlackout, []string{"earnings_blackout"}},
		{industry.EventTariffAnnouncement, []string{"tariff_shock"}},
		{industry.EventTSMCRevenueSurge, []string{"AI_capex_surge"}},
		{industry.EventRSSGeoEvent, []string{"geopolitical_risk_spike"}},
		{industry.EventUSDTWDVolatility, []string{"USD_TWD_volatility"}},
		{industry.EventMarginDivergence, []string{"retail_institutional_divergence"}},
		{industry.EventBDIShippingSpike, []string{"shipping_rate_spike"}},
		{industry.EventTWSEIndexDrop, []string{"semiconductor_downturn"}},
		{industry.EventTechConference, []string{"tech_peak_season"}},
	}
	for _, tc := range cases {
		got := EventTypeToTriggerThemes(string(tc.eventType), nil)
		if !stringSliceEqual(got, tc.want) {
			t.Errorf("EventTypeToTriggerThemes(%q, nil) = %v, want %v",
				tc.eventType, got, tc.want)
		}
	}
}

// TestEventTypeToTriggerThemes_UnknownEventType_ReturnsNil verifies that
// EventTypes without a trigger theme mapping (and unknown strings) return
// nil rather than empty slice. This is the contract the PR#4 scheduler
// relies on when iterating upcoming events.
func TestEventTypeToTriggerThemes_UnknownEventType_ReturnsNil(t *testing.T) {
	cases := []industry.TaiwanEventType{
		industry.EventMSCIRebalance,     // no template yet
		industry.EventTaiwan50Rebalance, // no template yet
		industry.EventFuturesSettlement, // no template yet
		industry.EventInvestorConf,
		industry.EventLongHoliday,
	}
	for _, et := range cases {
		got := EventTypeToTriggerThemes(string(et), nil)
		if got != nil {
			t.Errorf("EventTypeToTriggerThemes(%q, nil) = %v, want nil", et, got)
		}
	}

	// Also verify an arbitrary unknown string returns nil.
	if got := EventTypeToTriggerThemes("totally_unknown_type", nil); got != nil {
		t.Errorf("EventTypeToTriggerThemes(\"totally_unknown_type\", nil) = %v, want nil", got)
	}
}

// TestEventTypeToTriggerThemes_EmptyRegistry_FiltersAllOut verifies that
// when the registry is non-nil but has zero detectors, every default
// theme is filtered out (no themes are "registered" → all return nil).
func TestEventTypeToTriggerThemes_EmptyRegistry_FiltersAllOut(t *testing.T) {
	empty := narrative.NewDetectorRegistry()

	cases := []industry.TaiwanEventType{
		industry.EventSpringFestival,
		industry.EventExDividend,
		industry.EventMonthlyRevenue,
		industry.EventFinancialReport,
	}
	for _, et := range cases {
		got := EventTypeToTriggerThemes(string(et), empty)
		if got != nil {
			t.Errorf("EventTypeToTriggerThemes(%q, empty registry) = %v, want nil", et, got)
		}
	}
}

// TestEventTypeToTriggerThemes_PartialRegistry_FiltersUnregistered
// verifies that themes NOT in the registry are filtered out, but themes
// that ARE in the registry pass through.
func TestEventTypeToTriggerThemes_PartialRegistry_FiltersUnregistered(t *testing.T) {
	// Register only spring_festival_season, not dividend_season.
	partial := narrative.NewDetectorRegistry()
	if err := partial.Register(&minimalDetector{theme: "spring_festival_season", enabled: true}); err != nil {
		t.Fatalf("Register spring_festival_season: %v", err)
	}

	// EventSpringFestival: spring_festival_season IS registered → returned.
	got := EventTypeToTriggerThemes(string(industry.EventSpringFestival), partial)
	if !stringSliceEqual(got, []string{"spring_festival_season"}) {
		t.Errorf("EventSpringFestival with partial registry = %v, want [spring_festival_season]", got)
	}

	// EventExDividend: dividend_season NOT registered → filtered out → nil.
	got = EventTypeToTriggerThemes(string(industry.EventExDividend), partial)
	if got != nil {
		t.Errorf("EventExDividend with partial registry = %v, want nil (dividend_season not registered)", got)
	}

	// EventFinancialReport: earnings_surprise NOT registered → filtered → nil.
	got = EventTypeToTriggerThemes(string(industry.EventFinancialReport), partial)
	if got != nil {
		t.Errorf("EventFinancialReport with partial registry = %v, want nil (earnings_surprise not registered)", got)
	}
}

// TestEventTypeToTriggerThemes_FullRegistry_ReturnsAllDefaults verifies
// that when the full NewDefaultDetectorRegistry() is used (all 24 themes
// registered), every mapped EventType returns its expected theme list.
func TestEventTypeToTriggerThemes_FullRegistry_ReturnsAllDefaults(t *testing.T) {
	full := narrative.NewDefaultDetectorRegistry()

	cases := []struct {
		eventType industry.TaiwanEventType
		want      []string
	}{
		{industry.EventSpringFestival, []string{"spring_festival_season"}},
		{industry.EventExDividend, []string{"dividend_season"}},
		{industry.EventDividendPayout, []string{"dividend_season"}},
		{industry.EventWindowDressing, []string{"year_end_window_dressing"}},
		{industry.EventElection, []string{"election_cycle"}},
		{industry.EventMonthlyRevenue, []string{"earnings_surprise"}},
		{industry.EventFinancialReport, []string{"earnings_surprise"}},
		{industry.EventPositionBuilding, []string{"year_end_window_dressing"}},
		{industry.EventShareholderMeeting, []string{"earnings_blackout"}},
		{industry.EventFOMCMeeting, []string{"US_rates_up", "US_rates_down"}},
		{industry.EventBOJRateDecision, []string{"JPY_carry_unwind"}},
		{industry.EventOPECMeeting, []string{"oil_price_shock"}},
		{industry.EventCPIRelease, []string{"inflation_spike"}},
		{industry.EventChinaGDPRelease, []string{"china_slowdown"}},
		{industry.EventTaiwanExportRelease, []string{"taiwan_export_boom"}},
		{industry.EventEarningsBlackout, []string{"earnings_blackout"}},
		{industry.EventTariffAnnouncement, []string{"tariff_shock"}},
		{industry.EventTSMCRevenueSurge, []string{"AI_capex_surge"}},
		{industry.EventRSSGeoEvent, []string{"geopolitical_risk_spike"}},
		{industry.EventUSDTWDVolatility, []string{"USD_TWD_volatility"}},
		{industry.EventMarginDivergence, []string{"retail_institutional_divergence"}},
		{industry.EventBDIShippingSpike, []string{"shipping_rate_spike"}},
		{industry.EventTWSEIndexDrop, []string{"semiconductor_downturn"}},
		{industry.EventTechConference, []string{"tech_peak_season"}},
	}
	for _, tc := range cases {
		got := EventTypeToTriggerThemes(string(tc.eventType), full)
		if !stringSliceEqual(got, tc.want) {
			t.Errorf("EventTypeToTriggerThemes(%q, full) = %v, want %v",
				tc.eventType, got, tc.want)
		}
	}
}

// TestEventTypeToTriggerThemes_NilRegistryReturnsCopy verifies that the
// returned slice is a copy (callers can mutate without affecting the
// underlying table or other callers).
func TestEventTypeToTriggerThemes_NilRegistryReturnsCopy(t *testing.T) {
	first := EventTypeToTriggerThemes(string(industry.EventExDividend), nil)
	if len(first) == 0 {
		t.Fatal("expected non-empty result for EventExDividend")
	}

	// Mutate the returned slice
	first[0] = "corrupted"

	// Second call must NOT see the mutation
	second := EventTypeToTriggerThemes(string(industry.EventExDividend), nil)
	if len(second) == 0 || second[0] != "dividend_season" {
		t.Errorf("after mutating first call, second call = %v, want [dividend_season]", second)
	}
}

// TestEventTypeToTriggerThemes_DisabledDetectorStillCounts verifies the
// filtering is based on registry.Get() (presence), not Enabled(). Disabled
// detectors are still returned so the scheduler can decide what to do —
// the registry's RunAll() respects the Enabled flag at execution time.
func TestEventTypeToTriggerThemes_DisabledDetectorStillCounts(t *testing.T) {
	reg := narrative.NewDetectorRegistry()
	if err := reg.Register(&minimalDetector{theme: "earnings_surprise", enabled: false}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := EventTypeToTriggerThemes(string(industry.EventMonthlyRevenue), reg)
	if !stringSliceEqual(got, []string{"earnings_surprise"}) {
		t.Errorf("EventTypeToTriggerThemes with disabled detector = %v, want [earnings_surprise]", got)
	}
}

// TestMappedEventTypes verifies the MappedEventTypes() helper returns
// exactly the set of TaiwanEventType values present in the mapping table.
func TestMappedEventTypes(t *testing.T) {
	got := MappedEventTypes()

	want := map[industry.TaiwanEventType]bool{
		industry.EventSpringFestival:      true,
		industry.EventExDividend:          true,
		industry.EventDividendPayout:      true,
		industry.EventWindowDressing:      true,
		industry.EventElection:            true,
		industry.EventMonthlyRevenue:      true,
		industry.EventFinancialReport:     true,
		industry.EventPositionBuilding:    true,
		industry.EventShareholderMeeting:  true,
		industry.EventFOMCMeeting:         true,
		industry.EventBOJRateDecision:     true,
		industry.EventOPECMeeting:         true,
		industry.EventCPIRelease:          true,
		industry.EventChinaGDPRelease:     true,
		industry.EventTaiwanExportRelease: true,
		industry.EventEarningsBlackout:    true,
		industry.EventTariffAnnouncement:  true,
		industry.EventTSMCRevenueSurge:    true,
		industry.EventRSSGeoEvent:         true,
		industry.EventUSDTWDVolatility:    true,
		industry.EventMarginDivergence:    true,
		industry.EventBDIShippingSpike:    true,
		industry.EventTWSEIndexDrop:       true,
		industry.EventTechConference:      true,
	}

	if len(got) != len(want) {
		t.Errorf("MappedEventTypes returned %d types, want %d (got=%v)", len(got), len(want), got)
	}
	for _, et := range got {
		if !want[et] {
			t.Errorf("MappedEventTypes returned unexpected %q", et)
		}
	}
}
