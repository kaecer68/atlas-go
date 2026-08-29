package industry

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventquality"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// TaiwanEventType is a canonical string enum for Taiwan market event types.
type TaiwanEventType string

const (
	EventSpringFestival     TaiwanEventType = "spring_festival"
	EventExDividend         TaiwanEventType = "ex_dividend"
	EventShareholderMeeting TaiwanEventType = "shareholder_meeting"
	EventWindowDressing     TaiwanEventType = "window_dressing"
	EventElection           TaiwanEventType = "election"
	EventMSCIRebalance      TaiwanEventType = "msci_rebalance"
	EventFinancialReport    TaiwanEventType = "financial_report"
	EventInvestorConf       TaiwanEventType = "investor_conference"
	EventMonthlyRevenue     TaiwanEventType = "monthly_revenue"
	EventLongHoliday        TaiwanEventType = "long_holiday"
	EventDividendPayout     TaiwanEventType = "dividend_payout"
	EventTaiwan50Rebalance  TaiwanEventType = "taiwan50_rebalance"
	EventFuturesSettlement  TaiwanEventType = "futures_settlement"
	EventPositionBuilding   TaiwanEventType = "position_building"

	// MacroEventType values (G-05) for themes that are not driven by
	// Taiwan calendar events. These are scheduled externally (FOMC,
	// BOJ, OPEC, etc.) and trigger trigger themes via the same
	// EventTypeToTriggerThemes mapping. Data-source ingestion is a
	// separate workstream.
	EventFOMCMeeting         TaiwanEventType = "fomc_meeting"
	EventBOJRateDecision     TaiwanEventType = "boj_rate_decision"
	EventOPECMeeting         TaiwanEventType = "opec_meeting"
	EventCPIRelease          TaiwanEventType = "cpi_release"
	EventChinaGDPRelease     TaiwanEventType = "china_gdp_release"
	EventTaiwanExportRelease TaiwanEventType = "taiwan_export_release"
	EventEarningsBlackout    TaiwanEventType = "earnings_blackout"
	EventTariffAnnouncement  TaiwanEventType = "tariff_announcement"

	// G-05 follow-up: market-data-driven event types for the 7 themes
	// that have no clean calendar event. Each is triggered by a data
	// threshold rather than a scheduled date. Data-source ingestion
	// populates these; the events.go ingestion layer maps raw data to
	// the corresponding trigger.
	EventTSMCRevenueSurge TaiwanEventType = "tsmc_revenue_surge"
	EventRSSGeoEvent      TaiwanEventType = "rss_geo_event"
	EventUSDTWDVolatility TaiwanEventType = "usd_twd_volatility"
	EventMarginDivergence TaiwanEventType = "margin_divergence"
	EventBDIShippingSpike TaiwanEventType = "bdi_shipping_spike"
	EventTWSEIndexDrop    TaiwanEventType = "twse_index_drop"
	EventTechConference   TaiwanEventType = "tech_conference"
)

// EventDataSource tracks the provenance of calendar event data.
type EventDataSource string

const (
	DataSourceDefaultRules EventDataSource = "default_rules"    // 硬編碼規則生成
	DataSourceTWSE         EventDataSource = "twse_provider"    // TWSE OpenAPI
	DataSourceFinMind      EventDataSource = "finmind_provider" // FinMind API
	DataSourceMOPS         EventDataSource = "mops_provider"    // 公開資訊觀測站
)

// EventEvidence tracks the evidentiary support level for calendar event data.
type EventEvidence string

const (
	EvidenceBacktested EventEvidence = "backtested" // 經過回測驗證
	EvidenceEstimated  EventEvidence = "estimated"  // 基於歷史經驗估算
	EvidenceUnverified EventEvidence = "unverified" // 未經任何驗證
	EvidenceRealTime   EventEvidence = "realtime"   // 即時 API 資料
)

// CalendarEvent represents a concrete calendar event with computed dates.
type CalendarEvent struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	NameEN              string    `json:"name_en"`
	EventType           string    `json:"event_type"`
	Description         string    `json:"description"`
	Direction           string    `json:"direction"` // "bullish" | "bearish" | "mixed" | "neutral"
	BaseWeight          float64   `json:"base_weight"`
	Active              bool      `json:"active"`
	StartDate           time.Time `json:"start_date"`
	EndDate             time.Time `json:"end_date"`
	PeakDate            time.Time `json:"peak_date"`
	DecayDays           int       `json:"decay_days"`
	AffectedIndustries  []string  `json:"affected_industries"`
	SentimentAdjustment float64   `json:"sentiment_adjustment"`
	// DataSource tracks the provenance of this event (default_rules, twse_provider, etc.).
	DataSource EventDataSource `json:"data_source"`
	// EvidenceQuality indicates the level of evidentiary support for this event.
	EvidenceQuality EventEvidence `json:"evidence_quality"`
	// GeneratedAt records when this event was created, allowing freshness checks.
	GeneratedAt time.Time `json:"generated_at"`
	// Backfilled is true when this event was retroactively added (its effective
	// date is in the past at ingestion time). The predictor discounts backfilled
	// events to 0.7x weight (Stage 2.2c).
	Backfilled bool `json:"backfilled"`
	// CrossSourceStatus tracks cross-source verification (Stage 2.2b).
	// "confirmed" when ≥2 distinct sources report the same composite key;
	// "pending" with only 1 source; empty when not evaluated.
	CrossSourceStatus string `json:"cross_source_status,omitempty"`
}

// String returns a human-readable summary of the event.
func (e CalendarEvent) String() string {
	return fmt.Sprintf(
		"%s (%s): %s, Direction=%s, Weight=%.2f, Active=%v",
		e.Name, e.NameEN,
		e.PeakDate.Format("2006-01-02"),
		e.Direction, e.BaseWeight, e.Active,
	)
}

// EventRule defines how to compute event dates dynamically for a given year.
type EventRule struct {
	EventType          string
	Name               string
	ComputeStartDate   func(year int) time.Time
	ComputeEndDate     func(year int) time.Time
	ComputePeakDate    func(year int) time.Time
	Direction          string
	BaseWeight         float64
	DecayDays          int
	AffectedIndustries []string
}

// EventCalendar is the engine that manages Taiwan market calendar events.
// Follows the same structural pattern as SeasonalEngine.
type EventCalendar struct {
	events      []CalendarEvent
	mu          sync.RWMutex
	config      *config.ParametersConfig
	annualRules map[string]EventRule
	generatedAt time.Time

	// Stage 2 quality gate hooks (PR #1119 + #1120, #1122). All optional; when nil
	// the calendar falls back to the legacy "accept everything" path so
	// existing callers don't need to wire a validator.
	validator        eventquality.EventValidator
	qualityLog       *eventquality.QualityLog
	crossSourceStore *eventquality.CrossSourceStore
}

// NewEventCalendar creates a new EventCalendar with default event rules.
// Follows the same constructor pattern as NewSeasonalEngine.
func NewEventCalendar() *EventCalendar {
	cfg := config.GetParametersConfig()
	tec := &EventCalendar{
		config:      cfg,
		annualRules: defaultEventRules(),
	}
	return tec
}

// WithValidator attaches a Stage 2 eventquality.EventValidator. Events that
// fail validation are dropped and (if WithQualityLog is also wired) recorded
// in the quality log. Returns the receiver for chaining.
func (tec *EventCalendar) WithValidator(v eventquality.EventValidator) *EventCalendar {
	tec.mu.Lock()
	tec.validator = v
	tec.mu.Unlock()
	return tec
}

// WithQualityLog attaches a Stage 2 eventquality.QualityLog destination for
// rejected events. No-op when the validator is nil.
func (tec *EventCalendar) WithQualityLog(l *eventquality.QualityLog) *EventCalendar {
	tec.mu.Lock()
	tec.qualityLog = l
	tec.mu.Unlock()
	return tec
}

// WithCrossSourceStore attaches a Stage 2 cross-source verification store.
// When wired, UpdateFromProvider records each provider's source in the store
// and tags events with their cross-source status (confirmed / pending).
func (tec *EventCalendar) WithCrossSourceStore(s *eventquality.CrossSourceStore) *EventCalendar {
	tec.mu.Lock()
	tec.crossSourceStore = s
	tec.mu.Unlock()
	return tec
}

// gateEvent runs the quality validator (if wired) and returns (true, "") to
// accept the event, (false, reason) to reject. Rejection reason is also
// recorded to the quality log when wired.
func (tec *EventCalendar) gateEvent(raw eventquality.RawEvent) (bool, string) {
	if tec.validator == nil {
		return true, ""
	}
	res := tec.validator.Validate(raw)
	if res.Accepted {
		return true, ""
	}
	if tec.qualityLog != nil {
		if err := tec.qualityLog.Record(res); err != nil {
			logging.Warn(
				"event_calendar", "quality_log_write_failed",
				logging.FStr("event_id", res.EventID),
				logging.Err(err),
			)
		}
	}
	return false, res.Reason
}

// toRawEvent converts a CalendarEvent into the wire-agnostic RawEvent fed to
// the Stage 2 validator. Confidence is mapped from the provenance Evidence
// field; trigger_theme is the event_type (the calendar has no separate
// trigger_theme column).
func toRawEvent(e CalendarEvent) eventquality.RawEvent {
	conf := 0.5
	switch e.EvidenceQuality {
	case EvidenceRealTime:
		conf = 1.0
	case EvidenceEstimated:
		conf = 0.7
	case EvidenceUnverified:
		conf = 0.3
	}
	symbol := ""
	if len(e.AffectedIndustries) > 0 {
		symbol = e.AffectedIndustries[0]
	} else if e.EventType != "" {
		symbol = e.EventType
	}
	src := string(e.DataSource)
	if src == "" {
		src = "calendar"
	}
	ingestedAt := e.GeneratedAt
	if ingestedAt.IsZero() {
		ingestedAt = time.Now()
	}
	return eventquality.RawEvent{
		EventID:        e.ID,
		EventType:      e.EventType,
		EffectiveDate:  e.PeakDate,
		SymbolOrSector: symbol,
		Title:          e.Name,
		TriggerTheme:   e.EventType,
		Source:         src,
		Confidence:     conf,
		IngestedAt:     ingestedAt,
	}
}

// filterByQualityGate runs gateEvent on each event in events and returns the
// subset that passes validation. When the validator is nil the function is a
// pass-through (returns the input slice).
func (tec *EventCalendar) filterByQualityGate(events []CalendarEvent) []CalendarEvent {
	if tec.validator == nil {
		return events
	}
	out := make([]CalendarEvent, 0, len(events))
	for _, evt := range events {
		if ok, _ := tec.gateEvent(toRawEvent(evt)); ok {
			out = append(out, evt)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Lunar calendar lookup tables
// ---------------------------------------------------------------------------
//
// P1-8: these maps are DERIVED from the single-source internal/taiwanholidays
// package (verified 2023-2040). They exist as package-level maps so existing
// callers (event rules, tests) keep indexing by year; the canonical data lives
// in taiwanholidays and cannot drift. Out-of-range years fall back to the
// conventional dates below (same conventions as before).

var (
	// lunarNewYearDates maps year to lunar new year (春節) date in Asia/Taipei.
	lunarNewYearDates = taiwanholidays.LunarNewYearDates()
	// lunarDragonBoatDates maps year to 端午節 date (lunar 5/5).
	lunarDragonBoatDates = taiwanholidays.LunarDragonBoatDates()
	// lunarMidAutumnDates maps year to 中秋節 date (lunar 8/15).
	lunarMidAutumnDates = taiwanholidays.LunarMidAutumnDates()
	// tombSweepingDates maps year to 清明節 date.
	tombSweepingDates = taiwanholidays.TombSweepingDates()
)

// GetLunarCoverageYears returns the effective coverage range of the lunar calendar
// system. Since ST-8 (lunar automation), the range is effectively unbounded;
// the returned values indicate the verified hardcoded cache range (2023-2030).
// Callers should treat any year as computable.
func GetLunarCoverageYears() (int, int) {
	return taiwanholidays.CoverageYears()
}

// taiwanHoliday is a fixed-date or lookup-based Taiwan public holiday.
type taiwanHoliday struct {
	Name    string
	Compute func(year int) time.Time // nil for fixed-date holidays
	Month   int                      // used when Compute is nil
	Day     int                      // used when Compute is nil
}

var taiwanPublicHolidays = []taiwanHoliday{
	{Name: "元旦", Month: 1, Day: 1},
	{Name: "春節", Compute: func(y int) time.Time {
		if d, ok := lunarNewYearDates[y]; ok {
			return d
		}
		return time.Date(y, 2, 1, 0, 0, 0, 0, time.UTC)
	}},
	{Name: "228和平紀念日", Month: 2, Day: 28},
	{Name: "清明節", Compute: func(y int) time.Time {
		if d, ok := tombSweepingDates[y]; ok {
			return d
		}
		return time.Date(y, 4, 5, 0, 0, 0, 0, time.UTC)
	}},
	{Name: "勞動節", Month: 5, Day: 1},
	{Name: "端午節", Compute: func(y int) time.Time {
		if d, ok := lunarDragonBoatDates[y]; ok {
			return d
		}
		return time.Date(y, 6, 10, 0, 0, 0, 0, time.UTC)
	}},
	{Name: "中秋節", Compute: func(y int) time.Time {
		if d, ok := lunarMidAutumnDates[y]; ok {
			return d
		}
		return time.Date(y, 9, 20, 0, 0, 0, 0, time.UTC)
	}},
	{Name: "國慶日", Month: 10, Day: 10},
}

// ---------------------------------------------------------------------------
// Date computation helpers
// ---------------------------------------------------------------------------

// nthWeekdayOfMonth returns the nth occurrence of a weekday in a month.
// weekday: time.Sunday=0 … time.Saturday=6; n: 1-based (1=first, 3=third).
func nthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	daysUntil := (int(weekday) - int(first.Weekday()) + 7) % 7
	day := 1 + daysUntil + (n-1)*7
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// thirdFriday returns the 3rd Friday of the given month.
func thirdFriday(year int, month time.Month) time.Time {
	return nthWeekdayOfMonth(year, month, time.Friday, 3)
}

// thirdWednesday returns the 3rd Wednesday of the given month.
func thirdWednesday(year int, month time.Month) time.Time {
	return nthWeekdayOfMonth(year, month, time.Wednesday, 3)
}

// lastBusinessDay returns the last business day of the given month.
func lastBusinessDay(year int, month time.Month) time.Time {
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}
	lastDay := time.Date(nextYear, nextMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)

	for lastDay.Weekday() == time.Saturday || lastDay.Weekday() == time.Sunday {
		lastDay = lastDay.AddDate(0, 0, -1)
	}
	return lastDay
}

// lastTwoWeekStart returns the start date of the last two weeks of a month.
func lastTwoWeekStart(year int, month time.Month) time.Time {
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}
	lastDay := time.Date(nextYear, nextMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return lastDay.AddDate(0, 0, -13)
}

// lastWeekStart returns the start date of the last week of a month.
func lastWeekStart(year int, month time.Month) time.Time {
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}
	lastDay := time.Date(nextYear, nextMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return lastDay.AddDate(0, 0, -6)
}

// dateInRange checks if a date falls within a range (inclusive).
func dateInRange(t, start, end time.Time) bool {
	return !t.Before(start) && !t.After(end)
}

// ---------------------------------------------------------------------------
// Default event rules
// ---------------------------------------------------------------------------

func defaultEventRules() map[string]EventRule {
	rules := make(map[string]EventRule)

	// Spring Festival (春節前後) — bullish before and after
	rules["spring_festival"] = EventRule{
		EventType: "spring_festival",
		Name:      "春節前後",
		ComputePeakDate: func(year int) time.Time {
			if d, ok := lunarNewYearDates[year]; ok {
				return d
			}
			return time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC)
		},
		ComputeStartDate: func(year int) time.Time {
			if d, ok := lunarNewYearDates[year]; ok {
				return d.AddDate(0, 0, -5)
			}
			return time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -5)
		},
		ComputeEndDate: func(year int) time.Time {
			if d, ok := lunarNewYearDates[year]; ok {
				return d.AddDate(0, 0, 10)
			}
			return time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 10)
		},
		Direction:          "bullish",
		BaseWeight:         0.60,
		DecayDays:          5,
		AffectedIndustries: []string{"financials", "consumer"},
	}

	// Ex-Dividend Season (除權息旺季) — June through August
	rules["ex_dividend"] = EventRule{
		EventType:          "ex_dividend",
		Name:               "除權息旺季",
		ComputeStartDate:   func(year int) time.Time { return time.Date(year, 6, 15, 0, 0, 0, 0, time.UTC) },
		ComputeEndDate:     func(year int) time.Time { return time.Date(year, 8, 31, 0, 0, 0, 0, time.UTC) },
		ComputePeakDate:    func(year int) time.Time { return time.Date(year, 7, 15, 0, 0, 0, 0, time.UTC) },
		Direction:          "mixed",
		BaseWeight:         0.70,
		DecayDays:          7,
		AffectedIndustries: []string{"financials", "consumer"},
	}

	// Shareholder Meeting Season (股東會密集期) — late May through June
	rules["shareholder_meeting"] = EventRule{
		EventType:          "shareholder_meeting",
		Name:               "股東會密集期",
		ComputeStartDate:   func(year int) time.Time { return time.Date(year, 5, 20, 0, 0, 0, 0, time.UTC) },
		ComputeEndDate:     func(year int) time.Time { return time.Date(year, 6, 30, 0, 0, 0, 0, time.UTC) },
		ComputePeakDate:    func(year int) time.Time { return time.Date(year, 6, 10, 0, 0, 0, 0, time.UTC) },
		Direction:          "bullish",
		BaseWeight:         0.50,
		DecayDays:          5,
		AffectedIndustries: []string{"financials", "electronics"},
	}

	// Window Dressing (季底作帳行情) — last 2 weeks of Mar/Jun/Sep/Dec
	rules["window_dressing"] = EventRule{
		EventType: "window_dressing",
		Name:      "季底作帳行情",
		ComputeStartDate: func(year int) time.Time {
			return lastTwoWeekStart(year, time.March)
		},
		ComputeEndDate: func(year int) time.Time {
			return time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
		},
		ComputePeakDate: func(year int) time.Time {
			return lastBusinessDay(year, time.June)
		},
		Direction:          "bullish",
		BaseWeight:         0.80,
		DecayDays:          14,
		AffectedIndustries: []string{"financials", "semiconductor", "electronics"},
	}

	// Election (選舉行情) — even-numbered years (Nov); presidential years (Jan)
	rules["election"] = EventRule{
		EventType: "election",
		Name:      "選舉行情",
		ComputeStartDate: func(year int) time.Time {
			if year%4 == 0 {
				return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			if year%2 == 0 {
				return time.Date(year, 11, 1, 0, 0, 0, 0, time.UTC)
			}
			return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC) // won't be active
		},
		ComputeEndDate: func(year int) time.Time {
			if year%4 == 0 {
				return time.Date(year, 1, 31, 0, 0, 0, 0, time.UTC)
			}
			if year%2 == 0 {
				return time.Date(year, 11, 30, 0, 0, 0, 0, time.UTC)
			}
			return time.Date(year, 1, 2, 0, 0, 0, 0, time.UTC) // won't be active
		},
		ComputePeakDate: func(year int) time.Time {
			if year%4 == 0 {
				return time.Date(year, 1, 15, 0, 0, 0, 0, time.UTC)
			}
			if year%2 == 0 {
				return time.Date(year, 11, 15, 0, 0, 0, 0, time.UTC)
			}
			return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC) // won't be active
		},
		Direction:          "bullish",
		BaseWeight:         0.60,
		DecayDays:          5,
		AffectedIndustries: []string{"financials", "construction", "electronics"},
	}

	// MSCI Rebalance (MSCI季度調整) — Feb, May, Aug, Nov (last business day)
	rules["msci_rebalance"] = EventRule{
		EventType: "msci_rebalance",
		Name:      "MSCI季度調整",
		ComputeStartDate: func(year int) time.Time {
			return lastBusinessDay(year, time.February).AddDate(0, 0, -3)
		},
		ComputeEndDate: func(year int) time.Time {
			return lastBusinessDay(year, time.November).AddDate(0, 0, 3)
		},
		ComputePeakDate: func(year int) time.Time {
			return lastBusinessDay(year, time.May)
		},
		Direction:          "neutral",
		BaseWeight:         0.90,
		DecayDays:          3,
		AffectedIndustries: []string{"semiconductor", "electronics", "financials"},
	}

	// Financial Report Deadlines (財報公布密集期) — 3/31, 5/15, 8/14, 11/14 ± 5 days
	rules["financial_report"] = EventRule{
		EventType: "financial_report",
		Name:      "財報公布密集期",
		ComputeStartDate: func(year int) time.Time {
			return time.Date(year, 3, 26, 0, 0, 0, 0, time.UTC)
		},
		ComputeEndDate: func(year int) time.Time {
			return time.Date(year, 11, 19, 0, 0, 0, 0, time.UTC)
		},
		ComputePeakDate: func(year int) time.Time {
			return time.Date(year, 5, 15, 0, 0, 0, 0, time.UTC)
		},
		Direction:          "mixed",
		BaseWeight:         0.70,
		DecayDays:          5,
		AffectedIndustries: []string{"semiconductor", "ai_supply_chain", "electronics", "financials"},
	}

	// Investor Conference Season (法說會旺季) — Jan, Apr, Jul, Oct
	rules["investor_conference"] = EventRule{
		EventType: "investor_conference",
		Name:      "法說會旺季",
		ComputeStartDate: func(year int) time.Time {
			return time.Date(year, time.January, 10, 0, 0, 0, 0, time.UTC)
		},
		ComputeEndDate: func(year int) time.Time {
			return time.Date(year, time.October, 25, 0, 0, 0, 0, time.UTC)
		},
		ComputePeakDate: func(year int) time.Time {
			return time.Date(year, time.July, 15, 0, 0, 0, 0, time.UTC)
		},
		Direction:          "bullish",
		BaseWeight:         0.60,
		DecayDays:          14,
		AffectedIndustries: []string{"semiconductor", "ai_supply_chain", "electronics"},
	}

	// Monthly Revenue Publication (營收公布高峰) — 10th of each month ± 3 days
	rules["monthly_revenue"] = EventRule{
		EventType: "monthly_revenue",
		Name:      "營收公布高峰",
		ComputeStartDate: func(year int) time.Time {
			return time.Date(year, 1, 7, 0, 0, 0, 0, time.UTC)
		},
		ComputeEndDate: func(year int) time.Time {
			return time.Date(year, 12, 13, 0, 0, 0, 0, time.UTC)
		},
		ComputePeakDate: func(year int) time.Time {
			return time.Date(year, 6, 10, 0, 0, 0, 0, time.UTC)
		},
		Direction:          "mixed",
		BaseWeight:         0.40,
		DecayDays:          3,
		AffectedIndustries: []string{},
	}

	// Long Holidays (連假前後) — Taiwan public holidays
	rules["long_holiday"] = EventRule{
		EventType: "long_holiday",
		Name:      "連假前後",
		ComputeStartDate: func(year int) time.Time {
			return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		},
		ComputeEndDate: func(year int) time.Time {
			return time.Date(year, 10, 10, 0, 0, 0, 0, time.UTC)
		},
		ComputePeakDate: func(year int) time.Time {
			return time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC)
		},
		Direction:          "bearish",
		BaseWeight:         0.50,
		DecayDays:          2,
		AffectedIndustries: []string{"consumer", "shipping"},
	}

	// Dividend Payout Cash Return (配息資金回流) — July through September
	rules["dividend_payout"] = EventRule{
		EventType:          "dividend_payout",
		Name:               "配息資金回流",
		ComputeStartDate:   func(year int) time.Time { return time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC) },
		ComputeEndDate:     func(year int) time.Time { return time.Date(year, 9, 30, 0, 0, 0, 0, time.UTC) },
		ComputePeakDate:    func(year int) time.Time { return time.Date(year, 8, 15, 0, 0, 0, 0, time.UTC) },
		Direction:          "bullish",
		BaseWeight:         0.50,
		DecayDays:          14,
		AffectedIndustries: []string{"financials", "consumer"},
	}

	// Taiwan 50 Rebalance (台灣50季度調整) — Mar, Jun, Sep, Dec (3rd Friday)
	rules["taiwan50_rebalance"] = EventRule{
		EventType: "taiwan50_rebalance",
		Name:      "台灣50季度調整",
		ComputeStartDate: func(year int) time.Time {
			return thirdFriday(year, time.March).AddDate(0, 0, -3)
		},
		ComputeEndDate: func(year int) time.Time {
			return thirdFriday(year, time.December).AddDate(0, 0, 3)
		},
		ComputePeakDate: func(year int) time.Time {
			return thirdFriday(year, time.June)
		},
		Direction:          "neutral",
		BaseWeight:         0.70,
		DecayDays:          3,
		AffectedIndustries: []string{"semiconductor", "electronics", "financials"},
	}

	// Futures Settlement (期貨結算日) — 3rd Wednesday of each month
	rules["futures_settlement"] = EventRule{
		EventType: "futures_settlement",
		Name:      "期貨結算日",
		ComputeStartDate: func(year int) time.Time {
			return thirdWednesday(year, time.January).AddDate(0, 0, -2)
		},
		ComputeEndDate: func(year int) time.Time {
			return thirdWednesday(year, time.December).AddDate(0, 0, 2)
		},
		ComputePeakDate: func(year int) time.Time {
			return thirdWednesday(year, time.June)
		},
		Direction:          "bearish",
		BaseWeight:         0.60,
		DecayDays:          2,
		AffectedIndustries: []string{"financials", "electronics"},
	}

	// Position Building (卡位行情) — last week before each window_dressing
	rules["position_building"] = EventRule{
		EventType: "position_building",
		Name:      "卡位行情",
		ComputeStartDate: func(year int) time.Time {
			return lastWeekStart(year, time.March)
		},
		ComputeEndDate: func(year int) time.Time {
			return time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
		},
		ComputePeakDate: func(year int) time.Time {
			return lastWeekStart(year, time.June)
		},
		Direction:          "bullish",
		BaseWeight:         0.50,
		DecayDays:          7,
		AffectedIndustries: []string{"financials", "electronics"},
	}

	return rules
}

// ---------------------------------------------------------------------------
// EventCalendar methods
// ---------------------------------------------------------------------------

// RefreshEvents regenerates all events for the year of the given time.
func (tec *EventCalendar) RefreshEvents(now time.Time) {
	tec.mu.Lock()
	defer tec.mu.Unlock()

	tec.generatedAt = time.Now()
	year := now.Year()
	var allEvents []CalendarEvent

	for _, rule := range tec.annualRules {
		// Handle window_dressing as 4 separate events per year
		if rule.EventType == "window_dressing" {
			for _, m := range []time.Month{time.March, time.June, time.September, time.December} {
				evt := tec.buildEventFromRule(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle monthly events (futures_settlement)
		if rule.EventType == "futures_settlement" {
			for m := time.January; m <= time.December; m++ {
				evt := tec.buildMonthlyEvent(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle long holidays — one event per public holiday
		if rule.EventType == "long_holiday" {
			for _, h := range taiwanPublicHolidays {
				evt := tec.buildHolidayEvent(rule, h, year)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle position_building as 4 separate events per year
		if rule.EventType == "position_building" {
			for _, m := range []time.Month{time.March, time.June, time.September, time.December} {
				evt := tec.buildPositionBuildingEvent(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle election — generate for both presidential and local election years
		if rule.EventType == "election" {
			evt := tec.buildElectionEvent(rule, year)
			if evt.StartDate.Before(evt.EndDate) { // only add if valid
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle MSCI rebalance as 4 events per year
		if rule.EventType == "msci_rebalance" {
			for _, m := range []time.Month{time.February, time.May, time.August, time.November} {
				evt := tec.buildMSCIEvent(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle financial_report as 4 events per year
		if rule.EventType == "financial_report" {
			deadlines := []struct {
				d    time.Time
				name string
			}{
				{time.Date(year, 3, 31, 0, 0, 0, 0, time.UTC), "Q4年報"},
				{time.Date(year, 5, 15, 0, 0, 0, 0, time.UTC), "Q1季報"},
				{time.Date(year, 8, 14, 0, 0, 0, 0, time.UTC), "Q2季報"},
				{time.Date(year, 11, 14, 0, 0, 0, 0, time.UTC), "Q3季報"},
			}
			for _, dl := range deadlines {
				evt := tec.buildReportEvent(rule, dl.d, dl.name, year)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle investor_conference as 4 events per year
		if rule.EventType == "investor_conference" {
			for _, m := range []time.Month{time.January, time.April, time.July, time.October} {
				evt := tec.buildMonthlyEvent(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle taiwan50_rebalance as 4 events per year
		if rule.EventType == "taiwan50_rebalance" {
			for _, m := range []time.Month{time.March, time.June, time.September, time.December} {
				evt := tec.buildTW50Event(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Handle monthly_revenue as 12 events per year
		if rule.EventType == "monthly_revenue" {
			for m := time.January; m <= time.December; m++ {
				evt := tec.buildRevenueEvent(rule, year, m)
				allEvents = append(allEvents, evt)
			}
			continue
		}

		// Default: single event for the year
		evt := tec.buildSingleEvent(rule, year)
		allEvents = append(allEvents, evt)
	}

	allEvents = tec.filterByQualityGate(allEvents)
	tec.events = allEvents
}

// DetectActiveEvents returns all events active at the given time.
func (tec *EventCalendar) DetectActiveEvents(now time.Time) []CalendarEvent {
	tec.mu.RLock()
	defer tec.mu.RUnlock()

	var active []CalendarEvent
	for _, evt := range tec.events {
		if dateInRange(now, evt.StartDate, evt.EndDate) {
			evt.Active = true
			evt.SentimentAdjustment = tec.computeSentimentAdjustment(evt, now)
			active = append(active, evt)
		}
	}
	return active
}

// GetEventAdjustment returns the combined event adjustment for a given industry.
func (tec *EventCalendar) GetEventAdjustment(industryID string, now time.Time) float64 {
	active := tec.DetectActiveEvents(now)
	if len(active) == 0 {
		return 0.0
	}

	total := 0.0
	count := 0.0
	for _, evt := range active {
		adj := evt.SentimentAdjustment
		// If industry matches, give full weight; otherwise partial
		relevant := len(evt.AffectedIndustries) == 0
		if slices.Contains(evt.AffectedIndustries, industryID) {
			relevant = true
		}
		if relevant {
			total += adj
			count++
		} else {
			// Cross-industry spillover at 30% weight
			total += adj * 0.3
			count += 0.3
		}
	}
	if count == 0 {
		return 0.0
	}
	return total / count
}

// GetEventTimeline returns all events within the next N days from now.
func (tec *EventCalendar) GetEventTimeline(now time.Time, days int) []CalendarEvent {
	tec.mu.RLock()
	defer tec.mu.RUnlock()

	endDate := now.AddDate(0, 0, days)
	var timeline []CalendarEvent
	for _, evt := range tec.events {
		// Event overlaps with [now, now+days] window
		if evt.StartDate.Before(endDate) && evt.EndDate.After(now) {
			evt.Active = dateInRange(now, evt.StartDate, evt.EndDate)
			evt.SentimentAdjustment = tec.computeSentimentAdjustment(evt, now)
			timeline = append(timeline, evt)
		}
	}
	return timeline
}

func (tec *EventCalendar) GetEventsForDate(date time.Time) []CalendarEvent {
	return tec.DetectActiveEvents(date)
}

// IsTaiwanTradingDay reports whether `date` is a weekday outside every Taiwan
// public-holiday window (long_holiday event). Used by alert rules to suppress
// spurious "no events" signals on weekends and lunar/fixed-date holidays.
func (tec *EventCalendar) IsTaiwanTradingDay(date time.Time) bool {
	wd := date.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	for _, evt := range tec.GetEventsForDate(date) {
		if evt.EventType == string(EventLongHoliday) {
			return false
		}
	}
	return true
}

// GetAllActiveEventNames returns the names of currently active events.
func (tec *EventCalendar) GetAllActiveEventNames(now time.Time) []string {
	active := tec.DetectActiveEvents(now)
	names := make([]string, len(active))
	for i, evt := range active {
		names[i] = evt.Name
	}
	return names
}

// GetCompositeEventSentiment returns an aggregated sentiment score in range [0.8, 1.2].
// This is the equivalent of GetPatternAdjustment from SeasonalEngine but expressed as
// a sentiment multiplier rather than a factor adjustment.
func (tec *EventCalendar) GetCompositeEventSentiment(now time.Time) float64 {
	active := tec.DetectActiveEvents(now)
	if len(active) == 0 {
		return 1.0
	}

	totalSentiment := 0.0
	for _, evt := range active {
		totalSentiment += evt.SentimentAdjustment
	}

	// Scale: sum of sentiment adjustments mapped to [0.8, 1.2] range
	// Each event contributes at most ±0.05, so 4 events at max = ±0.20
	composite := 1.0 + totalSentiment

	// Clamp to [0.8, 1.2]
	if composite < 0.8 {
		composite = 0.8
	}
	if composite > 1.2 {
		composite = 1.2
	}
	return math.Round(composite*1000) / 1000
}

// GetAllEvents returns all generated events for the current year.
func (tec *EventCalendar) GetAllEvents() []CalendarEvent {
	tec.mu.RLock()
	defer tec.mu.RUnlock()

	result := make([]CalendarEvent, len(tec.events))
	copy(result, tec.events)
	return result
}

// ---------------------------------------------------------------------------
// Event construction helpers
// ---------------------------------------------------------------------------

func (tec *EventCalendar) buildEventFromRule(rule EventRule, year int, month time.Month) CalendarEvent {
	startDate := lastTwoWeekStart(year, month)
	endDate := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	if month == time.December {
		endDate = time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	}

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("季底作帳 - %s", month.String()),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           startDate,
		EndDate:             endDate,
		PeakDate:            endDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildMonthlyEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	peakDate := rule.ComputePeakDate(year)
	// For generic monthly events, set peak mid-month
	startDate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	if month == time.December {
		endDate = time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	}

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("%s - %s", rule.Name, month.String()),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           startDate,
		EndDate:             endDate,
		PeakDate:            peakDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildHolidayEvent(rule EventRule, h taiwanHoliday, year int) CalendarEvent {
	var holidayDate time.Time
	if h.Compute != nil {
		holidayDate = h.Compute(year)
	} else {
		holidayDate = time.Date(year, time.Month(h.Month), h.Day, 0, 0, 0, 0, time.UTC)
	}

	startDate := holidayDate.AddDate(0, 0, -3)
	endDate := holidayDate.AddDate(0, 0, 2)

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%s_%d", rule.EventType, h.Name, year),
		Name:                fmt.Sprintf("連假 - %s", h.Name),
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("%s前後交易淡季", h.Name),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           startDate,
		EndDate:             endDate,
		PeakDate:            holidayDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildPositionBuildingEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	startDate := lastWeekStart(year, month)
	windowStart := lastTwoWeekStart(year, month)

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("卡位行情 - %s", month.String()),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           startDate,
		EndDate:             windowStart.AddDate(0, 0, -1),
		PeakDate:            startDate.AddDate(0, 0, 2),
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildElectionEvent(rule EventRule, year int) CalendarEvent {
	id := fmt.Sprintf("%s_%d", rule.EventType, year)
	desc := "非選舉年"
	startDate := rule.ComputeStartDate(year)
	endDate := rule.ComputeEndDate(year)
	peakDate := rule.ComputePeakDate(year)

	if year%4 == 0 {
		desc = "總統大選行情"
		id = fmt.Sprintf("%s_presidential_%d", rule.EventType, year)
	} else if year%2 == 0 {
		desc = "地方選舉行情"
		id = fmt.Sprintf("%s_local_%d", rule.EventType, year)
	}

	return CalendarEvent{
		ID:                  id,
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         desc,
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           startDate,
		EndDate:             endDate,
		PeakDate:            peakDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildMSCIEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	settlementDate := lastBusinessDay(year, month)

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("MSCI季度調整 - %s", month.String()),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           settlementDate.AddDate(0, 0, -3),
		EndDate:             settlementDate.AddDate(0, 0, 3),
		PeakDate:            settlementDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildReportEvent(rule EventRule, deadline time.Time, label string, _ int) CalendarEvent {
	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%s", rule.EventType, deadline.Format("2006-01-02")),
		Name:                fmt.Sprintf("%s - %s", rule.Name, label),
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("%s deadline %s", label, deadline.Format("2006-01-02")),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           deadline.AddDate(0, 0, -5),
		EndDate:             deadline.AddDate(0, 0, 5),
		PeakDate:            deadline,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildTW50Event(rule EventRule, year int, month time.Month) CalendarEvent {
	settlementDate := thirdFriday(year, month)

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("台灣50季度調整 - %s", month.String()),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           settlementDate.AddDate(0, 0, -3),
		EndDate:             settlementDate.AddDate(0, 0, 3),
		PeakDate:            settlementDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildRevenueEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	revenueDate := time.Date(year, month, 10, 0, 0, 0, 0, time.UTC)

	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         fmt.Sprintf("%s monthly revenue", month.String()),
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           revenueDate.AddDate(0, 0, -3),
		EndDate:             revenueDate.AddDate(0, 0, 3),
		PeakDate:            revenueDate,
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

func (tec *EventCalendar) buildSingleEvent(rule EventRule, year int) CalendarEvent {
	return CalendarEvent{
		ID:                  fmt.Sprintf("%s_%d", rule.EventType, year),
		Name:                rule.Name,
		NameEN:              "",
		EventType:           rule.EventType,
		Description:         rule.Name,
		Direction:           rule.Direction,
		BaseWeight:          rule.BaseWeight,
		Active:              false,
		StartDate:           rule.ComputeStartDate(year),
		EndDate:             rule.ComputeEndDate(year),
		PeakDate:            rule.ComputePeakDate(year),
		DecayDays:           rule.DecayDays,
		AffectedIndustries:  rule.AffectedIndustries,
		SentimentAdjustment: 0.0,
	}
}

// ---------------------------------------------------------------------------
// Sentiment computation
// ---------------------------------------------------------------------------

// computeSentimentAdjustment calculates the sentiment adjustment for an event
// given the current time. The adjustment decays linearly from the peak date
// and is capped at ±0.05.
func (tec *EventCalendar) computeSentimentAdjustment(evt CalendarEvent, now time.Time) float64 {
	// Direction multiplier
	dirMul := 1.0
	switch evt.Direction {
	case "bullish":
		dirMul = 1.0
	case "bearish":
		dirMul = -1.0
	case "mixed":
		dirMul = 0.0
	case "neutral":
		dirMul = 0.0
	}

	// Decay factor: 1.0 at peak, 0.0 after decayDays past peak/end
	daysFromPeak := now.Sub(evt.PeakDate).Hours() / 24.0
	if daysFromPeak < 0 {
		daysFromPeak = -daysFromPeak
	}
	decayFactor := 0.0
	if evt.DecayDays > 0 {
		decayFactor = math.Max(0, 1.0-daysFromPeak/float64(evt.DecayDays))
	}

	// Base weight * direction * decay, capped at ±0.05
	adjustment := evt.BaseWeight * dirMul * decayFactor * 0.05
	if adjustment > 0.05 {
		adjustment = 0.05
	}
	if adjustment < -0.05 {
		adjustment = -0.05
	}
	return math.Round(adjustment*10000) / 10000
}

// UpdateFromProvider fetches events from an external provider and merges them
// into the calendar. Provider events are appended alongside default-rule events;
// the EventDataSource and EventEvidence fields distinguish their provenance.
// Safe for concurrent use via the calendar's internal RWMutex.
func (tec *EventCalendar) UpdateFromProvider(ctx context.Context, provider marketdata.CalendarEventProvider) {
	if provider == nil {
		return
	}
	year := tec.generatedAt.Year()
	if year == 0 {
		year = time.Now().Year()
	}
	events, err := provider.FetchEvents(ctx, year)
	if err != nil {
		logging.Warn(
			"event_calendar", "provider_fetch_failed",
			logging.FStr("provider", provider.Name()),
			logging.Err(err),
		)
		return
	}
	if len(events) == 0 {
		return
	}

	// Convert provider data to CalendarEvent using the TWSE provider data source.
	ds := DataSourceTWSE
	if provider.Name() == "finmind" {
		ds = DataSourceFinMind
	}

	tec.mu.Lock()
	defer tec.mu.Unlock()
	var newEvents []CalendarEvent
	for _, pd := range events {
		startDate, err := time.Parse("2006-01-02", pd.Date)
		if err != nil {
			logging.Warn(
				"event_calendar", "parse_date_failed",
				logging.FStr("provider", provider.Name()),
				logging.FStr("date", pd.Date),
				logging.Err(err),
			)
			continue
		}
		evt := CalendarEvent{
			ID:                fmt.Sprintf("%s_%s_%s", ds, pd.EventType, pd.Date),
			Name:              pd.Name,
			NameEN:            pd.Name,
			EventType:         pd.EventType,
			Description:       pd.Description,
			Direction:         pd.Direction,
			BaseWeight:        pd.Weight,
			Active:            true,
			StartDate:         startDate,
			EndDate:           startDate.AddDate(0, 0, 1), // default 1-day event
			PeakDate:          startDate,
			DecayDays:         7,
			DataSource:        ds,
			EvidenceQuality:   EvidenceRealTime,
			GeneratedAt:       time.Now(),
			CrossSourceStatus: string(eventquality.StatusPending),
		}
		// Stage 2.2c: mark events whose effective date is in the past as backfilled.
		if startDate.Before(time.Now().Truncate(24 * time.Hour)) {
			evt.Backfilled = true
		}
		// Stage 2.2b: cross-source verification — record the provider source
		// and tag the event with its cross-source status.
		if tec.crossSourceStore != nil {
			theme := pd.EventType
			symbol := pd.Symbol
			status := tec.crossSourceStore.Record(provider.Name(), theme, symbol, startDate)
			evt.CrossSourceStatus = string(status)
		}
		newEvents = append(newEvents, evt)
	}
	accepted := tec.filterByQualityGate(newEvents)
	tec.events = append(tec.events, accepted...)
	logging.Info(
		"event_calendar", "provider_events_added",
		logging.FStr("provider", provider.Name()),
		logging.FInt("added_events", len(events)),
		logging.FInt("accepted_events", len(accepted)),
	)
}

// NewEventCalendarWithProvider 是「wired」版 EventCalendar factory，
// 與 NewEventCalendar() 的差異在於會同步呼叫 RefreshEvents(time.Now()) 載入當年預設事件。
//
// 設計理由（Stage 1 缺口補齊 PR#1）：
//   - 舊的 NewEventCalendar() 只載入 annualRules，events slice 為空，
//     必須另呼叫 RefreshEvents 才會有資料。
//   - 過往各 caller 各自記得呼叫 RefreshEvents，容易遺漏（PR#1 root cause）。
//   - 此 factory 把「載入預設事件」內建為不可分割的一步，杜絕漏呼叫。
//
// 注意：provider 為 nil 時只載入預設事件。如需從外部 provider 拉資料（例如 TWSE 營收公布日），
// 請在 caller 端啟動背景 goroutine，以 app context 驅動週期性呼叫 UpdateFromProvider；
// 不建議在 factory 內同步呼叫 provider.FetchEvents（會 block API startup）。
//
// Maturity: stable（v0.0.0.33+ 公開 API）。
func NewEventCalendarWithProvider(provider marketdata.CalendarEventProvider) *EventCalendar {
	ec := NewEventCalendar()
	ec.RefreshEvents(time.Now())
	_ = provider // provider 為非 nil 時，caller 須自行啟動背景 refresh。
	return ec
}
