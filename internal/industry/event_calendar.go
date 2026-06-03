package industry

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
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
	events         []CalendarEvent
	providerEvents []CalendarEvent // 外部 provider 事件，RefreshEvents 不會覆蓋
	mu             sync.RWMutex
	config         *config.ParametersConfig
	annualRules    map[string]EventRule
	generatedAt    time.Time // 當前批次的生成時間戳，避免 per-event time.Now()
}

// NewEventCalendar creates a new EventCalendar with default event rules.
// If ParametersConfig.Industry.EventCalendarRules is populated, its entries
// overlay BaseWeight/DecayDays/Direction onto the matching default rules (ST-1).
func NewEventCalendar() *EventCalendar {
	cfg := config.GetParametersConfig()
	tec := &EventCalendar{
		config:      cfg,
		annualRules: defaultEventRules(),
	}
	tec.applyConfigOverrides()
	return tec
}

// applyConfigOverrides reads EventCalendarRules from ParametersConfig and overlays
// tunable scalar fields (BaseWeight, DecayDays, Direction) onto the hardcoded
// annualRules map. Date computation functions and AffectedIndustries are NOT
// overridable from JSON (they require Go code). Entries without a matching
// EventType in annualRules are silently skipped for forward compatibility.
func (tec *EventCalendar) applyConfigOverrides() {
	cfg := tec.config
	if cfg == nil {
		return
	}
	rules := cfg.Industry.EventCalendarRules.Value
	if len(rules) == 0 {
		return
	}
	for _, cr := range rules {
		// Match by EventType first; fall back to Name for backward compatibility.
		key := cr.EventType
		if key == "" {
			key = cr.Name
		}
		rule, ok := tec.annualRules[key]
		if !ok {
			continue
		}
		// Overlay tunable fields only when the config value is non-zero.
		if cr.BaseWeight > 0 {
			rule.BaseWeight = cr.BaseWeight
		}
		if cr.DecayDays > 0 {
			rule.DecayDays = cr.DecayDays
		}
		if cr.Direction != "" {
			rule.Direction = cr.Direction
		}
		tec.annualRules[key] = rule
	}
	logging.Debug("event_calendar", "config_overrides_applied",
		logging.FInt("rule_count", len(rules)),
	)
}

// UpdateFromProvider fetches calendar events from an external provider and merges
// them into the provider event list. Provider events are stored separately in
// tec.providerEvents so that RefreshEvents() does not overwrite them.
// Deduplication is by (EventType, Date, Symbol).
//
// This method is safe for concurrent use via the EventCalendar's internal mutex.
func (tec *EventCalendar) UpdateFromProvider(ctx context.Context, provider marketdata.CalendarEventProvider) {
	if provider == nil {
		return
	}

	now := time.Now()
	year := now.Year()
	providerEvents, err := provider.FetchEvents(ctx, year)
	if err != nil {
		logging.Warn("event_calendar", "provider_fetch_failed",
			logging.FStr("provider", provider.Name()),
			logging.Err(err),
		)
		return
	}

	if len(providerEvents) == 0 {
		return
	}

	tec.generatedAt = time.Now()

	// Determine data source from provider name.
	source := DataSourceTWSE
	switch provider.Name() {
	case "twse_calendar":
		source = DataSourceTWSE
	case "finmind_calendar":
		source = DataSourceFinMind
	default:
		source = DataSourceTWSE
	}

	tec.mu.Lock()
	defer tec.mu.Unlock()

	// Dedup against both default events and existing provider events.
	// Key format: EventType + "|" + Date + "|" + Symbol (consistent across all sources).
	seen := make(map[string]bool)
	for _, evt := range tec.events {
		key := evt.EventType + "|" + evt.PeakDate.Format("2006-01-02") + "|"
		seen[key] = true
	}
	for _, evt := range tec.providerEvents {
		key := evt.EventType + "|" + evt.PeakDate.Format("2006-01-02") + "|" + evt.ID
		seen[key] = true
	}

	var added int
	var rejected int
	for _, pe := range providerEvents {
		// ST-5: Validate provider event data sanity.
		if valErr := validateProviderEvent(pe); valErr != nil {
			rejected++
			if rejected <= 5 {
				logging.Warn("event_calendar", "provider_event_rejected",
					logging.FStr("provider", provider.Name()),
					logging.FStr("reason", valErr.Error()),
					logging.FStr("symbol", pe.Symbol),
					logging.FStr("date", pe.Date),
				)
			}
			continue
		}

		key := pe.EventType + "|" + pe.Date + "|" + pe.Symbol
		if seen[key] {
			continue
		}
		seen[key] = true

		eventDate, parseErr := time.Parse("2006-01-02", pe.Date)
		if parseErr != nil {
			rejected++
			logging.Warn("event_calendar", "parse_date_failed",
				logging.FStr("date", pe.Date),
				logging.Err(parseErr),
			)
			continue
		}

		evt := tec.newProviderEvent(source)
		evt.ID = fmt.Sprintf("%s_%s_%s", pe.EventType, pe.Date, pe.Symbol)
		evt.Name = pe.Name
		evt.EventType = pe.EventType
		evt.Description = pe.Description
		evt.Direction = pe.Direction
		evt.BaseWeight = pe.Weight
		evt.StartDate = eventDate.AddDate(0, 0, -3)
		evt.EndDate = eventDate.AddDate(0, 0, 3)
		evt.PeakDate = eventDate
		evt.DecayDays = 3

		tec.providerEvents = append(tec.providerEvents, evt)
		added++
	}

	logging.Info("event_calendar", "provider_merged",
		logging.FStr("provider", provider.Name()),
		logging.FInt("added", added),
		logging.FInt("rejected", rejected),
		logging.FInt("total_provider_events", len(tec.providerEvents)),
	)
}

// validateProviderEvent performs sanity checks on a provider event (ST-5).
// Returns nil if valid, or a descriptive error if invalid.
func validateProviderEvent(pe marketdata.CalendarProviderData) error {
	if pe.Symbol == "" {
		return fmt.Errorf("empty symbol")
	}
	if pe.EventType == "" {
		return fmt.Errorf("empty event_type")
	}
	if pe.Date == "" {
		return fmt.Errorf("empty date")
	}
	if pe.Weight < 0.0 || pe.Weight > 1.0 {
		return fmt.Errorf("weight %.2f out of [0.0, 1.0] range", pe.Weight)
	}
	switch pe.Direction {
	case "bullish", "bearish", "mixed", "neutral":
		// valid
	default:
		return fmt.Errorf("invalid direction: %q", pe.Direction)
	}
	// Validate date is parseable and within reasonable range.
	t, err := time.Parse("2006-01-02", pe.Date)
	if err != nil {
		return fmt.Errorf("unparseable date %q: %w", pe.Date, err)
	}
	if t.Year() < 2010 || t.Year() > 2040 {
		return fmt.Errorf("date year %d out of [2010, 2040] range", t.Year())
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lunar calendar lookup tables (2024-2028)
// ---------------------------------------------------------------------------

// lunarNewYearDates maps year to lunar new year (春節) date in Asia/Taipei.
// Coverage: 2023-2030. Beyond this range, getLunarDate logs a warning and
// falls back to an approximate date.
var lunarNewYearDates = map[int]time.Time{
	2023: time.Date(2023, 1, 22, 0, 0, 0, 0, time.UTC),
	2024: time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 1, 29, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 2, 6, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 1, 26, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 2, 13, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 2, 3, 0, 0, 0, 0, time.UTC),
}

// lunarDragonBoatDates maps year to 端午節 date (lunar 5/5).
var lunarDragonBoatDates = map[int]time.Time{
	2023: time.Date(2023, 6, 22, 0, 0, 0, 0, time.UTC),
	2024: time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 6, 9, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 5, 28, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 6, 16, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 6, 5, 0, 0, 0, 0, time.UTC),
}

// lunarMidAutumnDates maps year to 中秋節 date (lunar 8/15).
var lunarMidAutumnDates = map[int]time.Time{
	2023: time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC),
	2024: time.Date(2024, 9, 17, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 10, 6, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 9, 15, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 10, 3, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 9, 22, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 9, 12, 0, 0, 0, 0, time.UTC),
}

// tombSweepingDates maps year to 清明節 date.
var tombSweepingDates = map[int]time.Time{
	2023: time.Date(2023, 4, 5, 0, 0, 0, 0, time.UTC),
	2024: time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 4, 5, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 4, 4, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 4, 4, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 4, 5, 0, 0, 0, 0, time.UTC),
}

// getLunarDate looks up a date from a lunar calendar table. If the year is not
// covered, it logs a warning and returns the fallback date. This makes fallback
// usage visible rather than silently producing inaccurate dates.
func getLunarDate(year int, table map[int]time.Time, fallback time.Time, holidayName string) time.Time {
	if d, ok := table[year]; ok {
		return d
	}
	logging.Warn("event_calendar", "lunar_date_fallback",
		logging.FStr("holiday", holidayName),
		logging.FInt("year", year),
		logging.FStr("fallback", fallback.Format("2006-01-02")),
	)
	return fallback
}

// GetLunarCoverageYears returns the min and max year covered by the lunar
// calendar lookup tables. Years outside this range use approximate fallbacks.
func GetLunarCoverageYears() (int, int) {
	return 2023, 2030
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
		return getLunarDate(y, lunarNewYearDates, time.Date(y, 2, 1, 0, 0, 0, 0, time.UTC), "春節")
	}},
	{Name: "228和平紀念日", Month: 2, Day: 28},
	{Name: "清明節", Compute: func(y int) time.Time {
		return getLunarDate(y, tombSweepingDates, time.Date(y, 4, 5, 0, 0, 0, 0, time.UTC), "清明節")
	}},
	{Name: "勞動節", Month: 5, Day: 1},
	{Name: "端午節", Compute: func(y int) time.Time {
		return getLunarDate(y, lunarDragonBoatDates, time.Date(y, 6, 10, 0, 0, 0, 0, time.UTC), "端午節")
	}},
	{Name: "中秋節", Compute: func(y int) time.Time {
		return getLunarDate(y, lunarMidAutumnDates, time.Date(y, 9, 20, 0, 0, 0, 0, time.UTC), "中秋節")
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
			return getLunarDate(year, lunarNewYearDates, time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC), "春節")
		},
		ComputeStartDate: func(year int) time.Time {
			d := getLunarDate(year, lunarNewYearDates, time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC), "春節")
			return d.AddDate(0, 0, -5)
		},
		ComputeEndDate: func(year int) time.Time {
			d := getLunarDate(year, lunarNewYearDates, time.Date(year, 2, 1, 0, 0, 0, 0, time.UTC), "春節")
			return d.AddDate(0, 0, 10)
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
// newDefaultEvent creates a CalendarEvent with default evidence markers for
// hardcoded-rule-generated events. All build*Event methods should use this.
// Uses tec.generatedAt which is set once per RefreshEvents/UpdateFromProvider call
// to avoid calling time.Now() per-event during bulk generation.
func (tec *EventCalendar) newDefaultEvent() CalendarEvent {
	return CalendarEvent{
		DataSource:      DataSourceDefaultRules,
		EvidenceQuality: EvidenceUnverified,
		GeneratedAt:     tec.generatedAt,
	}
}

// newProviderEvent creates a CalendarEvent with evidence markers for
// externally-sourced provider events.
func (tec *EventCalendar) newProviderEvent(source EventDataSource) CalendarEvent {
	return CalendarEvent{
		DataSource:      source,
		EvidenceQuality: EvidenceEstimated,
		GeneratedAt:     tec.generatedAt,
	}
}

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

	// ST-2 fix: Preserve provider events across RefreshEvents calls.
	// Provider events are stored separately and appended after regeneration
	// to prevent RefreshEvents from discarding TWSE/external data.
	tec.events = append(allEvents, tec.providerEvents...)
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
		for _, id := range evt.AffectedIndustries {
			if id == industryID {
				relevant = true
				break
			}
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

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("季底作帳 - %s", month.String())
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = startDate
	evt.EndDate = endDate
	evt.PeakDate = endDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildMonthlyEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	peakDate := rule.ComputePeakDate(year)
	// For generic monthly events, set peak mid-month
	startDate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	if month == time.December {
		endDate = time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	}

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("%s - %s", rule.Name, month.String())
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = startDate
	evt.EndDate = endDate
	evt.PeakDate = peakDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
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

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%s_%d", rule.EventType, h.Name, year)
	evt.Name = fmt.Sprintf("連假 - %s", h.Name)
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("%s前後交易淡季", h.Name)
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = startDate
	evt.EndDate = endDate
	evt.PeakDate = holidayDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildPositionBuildingEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	startDate := lastWeekStart(year, month)
	windowStart := lastTwoWeekStart(year, month)

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("卡位行情 - %s", month.String())
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = startDate
	evt.EndDate = windowStart.AddDate(0, 0, -1)
	evt.PeakDate = startDate.AddDate(0, 0, 2)
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
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

	evt := tec.newDefaultEvent()
	evt.ID = id
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = desc
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = startDate
	evt.EndDate = endDate
	evt.PeakDate = peakDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildMSCIEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	settlementDate := lastBusinessDay(year, month)

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("MSCI季度調整 - %s", month.String())
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = settlementDate.AddDate(0, 0, -3)
	evt.EndDate = settlementDate.AddDate(0, 0, 3)
	evt.PeakDate = settlementDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildReportEvent(rule EventRule, deadline time.Time, label string, _ int) CalendarEvent {
	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%s", rule.EventType, deadline.Format("2006-01-02"))
	evt.Name = fmt.Sprintf("%s - %s", rule.Name, label)
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("%s deadline %s", label, deadline.Format("2006-01-02"))
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = deadline.AddDate(0, 0, -5)
	evt.EndDate = deadline.AddDate(0, 0, 5)
	evt.PeakDate = deadline
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildTW50Event(rule EventRule, year int, month time.Month) CalendarEvent {
	settlementDate := thirdFriday(year, month)

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("台灣50季度調整 - %s", month.String())
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = settlementDate.AddDate(0, 0, -3)
	evt.EndDate = settlementDate.AddDate(0, 0, 3)
	evt.PeakDate = settlementDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildRevenueEvent(rule EventRule, year int, month time.Month) CalendarEvent {
	revenueDate := time.Date(year, month, 10, 0, 0, 0, 0, time.UTC)

	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d_%02d", rule.EventType, year, month)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = fmt.Sprintf("%s monthly revenue", month.String())
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = revenueDate.AddDate(0, 0, -3)
	evt.EndDate = revenueDate.AddDate(0, 0, 3)
	evt.PeakDate = revenueDate
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

func (tec *EventCalendar) buildSingleEvent(rule EventRule, year int) CalendarEvent {
	evt := tec.newDefaultEvent()
	evt.ID = fmt.Sprintf("%s_%d", rule.EventType, year)
	evt.Name = rule.Name
	evt.EventType = rule.EventType
	evt.Description = rule.Name
	evt.Direction = rule.Direction
	evt.BaseWeight = rule.BaseWeight
	evt.StartDate = rule.ComputeStartDate(year)
	evt.EndDate = rule.ComputeEndDate(year)
	evt.PeakDate = rule.ComputePeakDate(year)
	evt.DecayDays = rule.DecayDays
	evt.AffectedIndustries = rule.AffectedIndustries
	return evt
}

// ---------------------------------------------------------------------------
// Sentiment computation
// ---------------------------------------------------------------------------

// getSentimentCap returns the per-event sentiment cap from ParametersConfig,
// falling back to the default of 0.05 when config is unavailable.
func (tec *EventCalendar) getSentimentCap() float64 {
	cfg := config.GetParametersConfig()
	if cfg != nil && cfg.Industry.EventSentimentCap.Value != 0 {
		return cfg.Industry.EventSentimentCap.Value
	}
	return 0.05
}

// computeSentimentAdjustment calculates the sentiment adjustment for an event
// given the current time. The adjustment decays linearly from the peak date
// and is capped by the per-event sentiment cap from ParametersConfig
// (default ±0.05, configurable via Industry.EventSentimentCap).
func (tec *EventCalendar) computeSentimentAdjustment(evt CalendarEvent, now time.Time) float64 {
	// Direction multiplier.
	// "bullish" = net positive sentiment for affected industries.
	// "bearish" = net negative sentiment for affected industries.
	// "mixed"   = asymmetric impact (bullish for some industries, bearish for others).
	//             Per-industry resolution happens upstream in GetEventAdjustment via
	//             the AffectedIndustries matching logic. Net composite direction is zero
	//             because positive and negative effects cancel in aggregate.
	// "neutral" = the event exists and may cause volatility, but has no directional bias
	//             for any industry (e.g., MSCI rebalance = flow rotation, not direction).
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

	cap := tec.getSentimentCap()
	adjustment := evt.BaseWeight * dirMul * decayFactor * cap
	if adjustment > cap {
		adjustment = cap
	}
	if adjustment < -cap {
		adjustment = -cap
	}
	return math.Round(adjustment*10000) / 10000
}
