// Command jev-eval-events exports the deterministic Taiwan market event
// calendar as the event-layer ground-truth skeleton consumed by the Jev
// shadow-evaluation framework (Stage 3 / E0, issue #1968).
//
// Why this exists
//
//	The event layer needs a ground truth that is (a) regenerable by one
//	command and (b) expressed in the platform's own rules instead of a new
//	caliber. Both constraints are satisfied by exporting the calendar in Go:
//	the dates come from internal/industry's rule engine
//	(industry.EventCalendar.RefreshEvents -> defaultEventRules), so the export
//	cannot drift away from the calendar the platform itself consumes.
//
//	This command deliberately exports NO price data. The other half of the
//	event-layer ground truth — the cost-adjusted forward return at the same
//	(date x canonical L1 industry) grain — is already exported by
//	cmd/experimental/jev-eval-panel, which owns the canonical caliber
//	(stockpicker.NetHit with the cost rate read from configs/parameters.json).
//	The Python task spec (scripts/jev_eval/specs/event_calendar.py) joins the
//	two files on the anchor date, so no return arithmetic is re-implemented
//	here and both halves stay auditable in isolation.
//
// Output (JSONL, one row per event occurrence whose anchor falls in the window)
//
//	Each row is one *scheduled* event occurrence: the window it covers, the
//	platform's own direction/weight prior for it, the industries it declares,
//	and the anchor session the evaluation is measured from. Every field is
//	derivable from the year alone, so the file is byte-reproducible.
//
// Anchor policy
//
//	The evaluation measures the forward return from the CLOSE of the anchor
//	session, so the anchor has to be a session that the canonical price
//	universe actually contains. The anchor is therefore the first session on or
//	after the nominal date (-anchor peak|start|end, default peak) inside the
//	universe read from -dir. A nominal date that is a weekend or a holiday
//	rolls forward; `anchor_shift_sessions` records how many sessions were
//	skipped, and `nominal_anchor_date` keeps the rolled-from date visible.
//
// Flags:
//
//	-start/-end    anchor window (YYYY-MM-DD, required)
//	-anchor        nominal anchor date: peak|start|end (default peak)
//	-dir           sector_index directory: the session universe (default data/state/sector_index)
//	-out             events JSONL path (required)
//	-adjustments-out  optional JSONL of the platform's OWN event-layer decision
//	                  variable (industry.GetEventAdjustment) per session x
//	                  canonical L1 industry, so a baseline can be measured
//	                  against the number the platform actually computes instead
//	                  of a re-implementation of it
//	-summary-out     JSON summary path (optional)
//	-quiet           suppress the human summary on stdout
//
// Read-only: it never writes config, state or production data other than the
// two output files it is asked for.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	anchorPeak  = "peak"
	anchorStart = "start"
	anchorEnd   = "end"

	dateLayout = "2006-01-02"

	// defaultSource labels the exported skeleton rows. It is deliberately a
	// distinct label from any production condition, so a reader can tell an
	// evaluation artifact from a live input.
	defaultSource = "jev-eval-events"
)

// EventRow is one exported scheduled event occurrence.
//
// It carries no price data and no outcome: it is the *question side* of the
// event layer. The matching outcome lives at (anchor_date, industry_id) in the
// panel produced by cmd/experimental/jev-eval-panel.
type EventRow struct {
	EventID     string  `json:"event_id"`
	EventType   string  `json:"event_type"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Direction   string  `json:"direction"`
	BaseWeight  float64 `json:"base_weight"`
	DecayDays   int     `json:"decay_days"`
	// AffectedIndustries are the canonical L1 ids the event rule declares. The
	// platform's GetEventAdjustment gives those full weight and everything else
	// 30% spillover, so the list is part of the prior, not decoration.
	AffectedIndustries []string `json:"affected_industries"`
	// WindowStart/PeakDate/WindowEnd are the event's own active window.
	WindowStart string `json:"window_start"`
	PeakDate    string `json:"peak_date"`
	WindowEnd   string `json:"window_end"`
	// AnchorKind records which of the three dates the evaluation is measured
	// from; NominalAnchorDate is that raw date and AnchorDate is it rolled into
	// the session universe.
	AnchorKind        string `json:"anchor_kind"`
	NominalAnchorDate string `json:"nominal_anchor_date"`
	AnchorDate        string `json:"anchor_date"`
	// AnchorShiftDays is how many calendar days the nominal date was rolled
	// forward to reach the anchor; 0 means the nominal date was itself a session
	// with price data, so 0 is unambiguous (a Saturday nominal date reports 2,
	// not 0).
	AnchorShiftDays int `json:"anchor_shift_days"`
	// QualityNotes records every data-quality exclusion this occurrence hit
	// (empty when the gate passed). Rows are never dropped silently: the reason
	// is either written here or counted in the summary, never both.
	QualityNotes []string `json:"quality_notes,omitempty"`
	// SessionsFromWindowStart / SessionsAnchorToWindowEnd place the anchor inside
	// the event window in sessions. Both are known in advance (they are calendar
	// arithmetic), so they are safe state inputs.
	SessionsFromWindowStart   int `json:"sessions_from_window_start"`
	SessionsAnchorToWindowEnd int `json:"sessions_anchor_to_window_end"`
	// CalendarYear is the year of the rule instantiation.
	CalendarYear int `json:"calendar_year"`
}

// AdjustmentRow is the platform's OWN event-layer decision variable at one
// (session, canonical L1 industry), read through the platform's function
// (industry.EventCalendar.GetEventAdjustment) instead of being recomputed here.
//
// It exists so the evaluation can measure "does Jev add anything on top of the
// number the platform already computes?" against that exact number. The value
// depends only on the session (all active events, their direction, base weight,
// decay and the 30% cross-industry spillover), so it is exported per session
// rather than per event occurrence.
type AdjustmentRow struct {
	Date             string   `json:"date"`
	IndustryID       string   `json:"industry_id"`
	IndustryNameZH   string   `json:"industry_name_zh,omitempty"`
	EventAdjustment  float64  `json:"event_adjustment"`
	ActiveEventTypes []string `json:"active_event_types"`
}

// eventsSummary is the reproducible aggregate of one export.
type eventsSummary struct {
	Source      string `json:"source"`
	Caliber     string `json:"caliber"`
	AnchorKind  string `json:"anchor_kind"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	// Session universe the anchor was rolled into.
	Sessions       int    `json:"sessions"`
	SessionFirst   string `json:"session_first"`
	SessionLast    string `json:"session_last"`
	Years          []int  `json:"years"`
	EventsTotal    int    `json:"events_total"`
	EventsExported int    `json:"events_exported"`
	// Drop counters make the coverage of the skeleton explicit instead of
	// letting missing events disappear silently.
	QualityGate               bool           `json:"quality_gate"`
	LunarCalendarVerifiedFrom int            `json:"lunar_calendar_verified_from"`
	DroppedPeakOutsideWindow  int            `json:"dropped_peak_outside_own_window"`
	DroppedInvertedWindow     int            `json:"dropped_inverted_window"`
	DroppedUnverifiedLunar    int            `json:"dropped_unverified_lunar_calendar"`
	DroppedAnchorBeforeWindow int            `json:"dropped_anchor_before_window"`
	DroppedAnchorAfterWindow  int            `json:"dropped_anchor_after_window"`
	DroppedNoSessionAtOrAfter int            `json:"dropped_no_session_at_or_after_nominal"`
	RolledAnchors             int            `json:"rolled_anchors"`
	MaxAnchorShiftDays        int            `json:"max_anchor_shift_days"`
	ByEventType               map[string]int `json:"by_event_type"`
	ByDirection               map[string]int `json:"by_direction"`
	// AdjustmentRows counts the exported platform-adjustment rows (0 when
	// -adjustments-out was not requested).
	AdjustmentRows int `json:"adjustment_rows"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "jev-eval-events: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("jev-eval-events", flag.ContinueOnError)
	startStr := fs.String("start", "", "anchor window start date YYYY-MM-DD (required)")
	endStr := fs.String("end", "", "anchor window end date YYYY-MM-DD (required)")
	anchorKind := fs.String("anchor", anchorPeak, "nominal anchor date: peak|start|end")
	qualityGate := fs.Bool("quality-gate", true,
		"drop occurrences whose anchor cannot be trusted (year-fixed peak, unverified lunar calendar)")
	dir := fs.String("dir", "data/state/sector_index", "sector_index directory (session universe)")
	outPath := fs.String("out", "", "events JSONL path (required)")
	adjPath := fs.String("adjustments-out", "", "platform GetEventAdjustment JSONL path (optional)")
	summaryPath := fs.String("summary-out", "", "JSON summary path (optional)")
	quiet := fs.Bool("quiet", false, "suppress the human summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outPath == "" {
		return fmt.Errorf("-out is required")
	}
	if *startStr == "" || *endStr == "" {
		return fmt.Errorf("-start and -end are required")
	}
	start, err := time.Parse(dateLayout, *startStr)
	if err != nil {
		return fmt.Errorf("parse -start: %w", err)
	}
	end, err := time.Parse(dateLayout, *endStr)
	if err != nil {
		return fmt.Errorf("parse -end: %w", err)
	}
	if end.Before(start) {
		return fmt.Errorf("-end %s before -start %s", *endStr, *startStr)
	}
	if *anchorKind != anchorPeak && *anchorKind != anchorStart && *anchorKind != anchorEnd {
		return fmt.Errorf("-anchor must be %s|%s|%s, got %q", anchorPeak, anchorStart, anchorEnd, *anchorKind)
	}

	reader := marketdata.NewSectorIndexReader(*dir)
	allDates, err := reader.AvailableDates()
	if err != nil {
		return fmt.Errorf("read sector index dates: %w", err)
	}
	sessions := uniqueSortedDates(allDates)
	if len(sessions) == 0 {
		return fmt.Errorf("no sector index dates under %s: the anchor session universe is empty", *dir)
	}
	index := make(map[string]int, len(sessions))
	for i, d := range sessions {
		index[d] = i
	}

	rows := make([]EventRow, 0, 256)
	sum := eventsSummary{
		Source:       defaultSource,
		Caliber:      "internal/industry/event_calendar.go determinism (defaultEventRules) + first session on/after the nominal date in internal/marketdata.SectorIndexReader",
		AnchorKind:   *anchorKind,
		WindowStart:  *startStr,
		WindowEnd:    *endStr,
		Sessions:     len(sessions),
		SessionFirst: sessions[0],
		SessionLast:  sessions[len(sessions)-1],
		ByEventType:  map[string]int{},
		ByDirection:  map[string]int{},
	}

	lunarFrom, _ := industry.GetLunarCoverageYears()
	sum.QualityGate = *qualityGate
	sum.LunarCalendarVerifiedFrom = lunarFrom

	for year := start.Year(); year <= end.Year(); year++ {
		// A fresh calendar per year: RefreshEvents regenerates *the year of the
		// argument* and replaces the event set, so one instance per year keeps
		// the export independent of iteration order.
		cal := industry.NewEventCalendar()
		cal.RefreshEvents(time.Date(year, time.June, 1, 0, 0, 0, 0, time.UTC))
		events := cal.GetAllEvents()
		sum.Years = append(sum.Years, year)
		sum.EventsTotal += len(events)

		for _, evt := range events {
			if *qualityGate {
				if reason, drop := qualityExclusion(evt, year, lunarFrom, *anchorKind); drop {
					switch reason {
					case reasonPeakOutsideWindow:
						sum.DroppedPeakOutsideWindow++
					case reasonUnverifiedLunar:
						sum.DroppedUnverifiedLunar++
					case reasonInvertedWindow:
						sum.DroppedInvertedWindow++
					}
					continue
				}
			}
			nominal := nominalAnchor(evt, *anchorKind)
			if nominal.IsZero() {
				continue
			}
			nominalStr := nominal.Format(dateLayout)
			anchorDate, shift, ok := firstSessionAtOrAfter(sessions, nominalStr)
			if !ok {
				sum.DroppedNoSessionAtOrAfter++
				continue
			}
			if anchorDate < *startStr {
				sum.DroppedAnchorBeforeWindow++
				continue
			}
			if anchorDate > *endStr {
				sum.DroppedAnchorAfterWindow++
				continue
			}
			if shift > 0 {
				sum.RolledAnchors++
			}
			if shift > sum.MaxAnchorShiftDays {
				sum.MaxAnchorShiftDays = shift
			}
			rows = append(rows, EventRow{
				EventID:                   evt.ID,
				EventType:                 evt.EventType,
				Name:                      evt.Name,
				Description:               evt.Description,
				Direction:                 evt.Direction,
				BaseWeight:                evt.BaseWeight,
				DecayDays:                 evt.DecayDays,
				AffectedIndustries:        append([]string(nil), evt.AffectedIndustries...),
				WindowStart:               evt.StartDate.Format(dateLayout),
				PeakDate:                  evt.PeakDate.Format(dateLayout),
				WindowEnd:                 evt.EndDate.Format(dateLayout),
				AnchorKind:                *anchorKind,
				NominalAnchorDate:         nominalStr,
				AnchorDate:                anchorDate,
				AnchorShiftDays:           shift,
				SessionsFromWindowStart:   sessionsFromWindowStart(sessions, index, anchorDate, evt.StartDate.Format(dateLayout)),
				SessionsAnchorToWindowEnd: sessionsToWindowEnd(sessions, index, anchorDate, evt.EndDate.Format(dateLayout)),
				CalendarYear:              year,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AnchorDate != rows[j].AnchorDate {
			return rows[i].AnchorDate < rows[j].AnchorDate
		}
		return rows[i].EventID < rows[j].EventID
	})
	if len(rows) == 0 {
		return fmt.Errorf("no event occurrence has an anchor session in [%s, %s] under %s", *startStr, *endStr, *dir)
	}
	sum.EventsExported = len(rows)
	for _, r := range rows {
		sum.ByEventType[r.EventType]++
		sum.ByDirection[r.Direction]++
	}

	if err := writeJSONL(*outPath, rows); err != nil {
		return err
	}
	if *adjPath != "" {
		adj, err := exportAdjustments(reader, sessions, *startStr, *endStr)
		if err != nil {
			return err
		}
		sum.AdjustmentRows = len(adj)
		if err := writeJSONL(*adjPath, adj); err != nil {
			return err
		}
	}
	if *summaryPath != "" {
		if err := writeJSON(*summaryPath, sum); err != nil {
			return err
		}
	}
	if !*quiet {
		printSummary(stdout, sum)
	}
	return nil
}

// Data-quality gate reasons. Both are hard evidence that the occurrence's own
// dates are not usable ground truth, so the occurrence is dropped rather than
// evaluated with a label that is known to be wrong.
const (
	// reasonPeakOutsideWindow: industry.EventCalendar.buildMonthlyEvent sets
	// PeakDate from rule.ComputePeakDate(year), which for the two year-anchored
	// monthly rules (futures_settlement, investor_conference) is a single fixed
	// date for every month. 11 of 12 futures_settlement and 3 of 4
	// investor_conference occurrences therefore carry a "peak" that is not
	// inside their own window, i.e. not their peak at all.
	reasonPeakOutsideWindow = "peak_outside_own_window"
	// reasonInvertedWindow: the occurrence's own window is malformed
	// (StartDate after EndDate), so industry.EventCalendar.DetectActiveEvents can
	// never report it active. 4 of 4 position_building occurrences in 2021 have
	// this shape (their EndDate is derived from a different week helper than
	// their StartDate), so the occurrence is not evaluable at all.
	reasonInvertedWindow = "inverted_window"
	// reasonUnverifiedLunar: the lunar tables behind the moving Taiwan holidays
	// (春節 / 清明 / 端午 / 中秋) are only verified from
	// industry.GetLunarCoverageYears(); before that year the calendar falls back
	// to conventional placeholders, so a `long_holiday` occurrence in such a
	// year is not the real holiday.
	reasonUnverifiedLunar = "unverified_lunar_calendar"
)

// qualityExclusion reports whether an occurrence must be dropped before any
// anchor is derived from it. `anchorKind` matters: only a peak anchor depends on
// PeakDate, so the window check is scoped to it.
func qualityExclusion(evt industry.CalendarEvent, year, lunarFrom int, anchorKind string) (string, bool) {
	if evt.EndDate.Before(evt.StartDate) {
		return reasonInvertedWindow, true
	}
	if evt.EventType == string(industry.EventLongHoliday) && year < lunarFrom {
		return reasonUnverifiedLunar, true
	}
	if anchorKind == anchorPeak && !dateWithin(evt.PeakDate, evt.StartDate, evt.EndDate) {
		return reasonPeakOutsideWindow, true
	}
	return "", false
}

// dateWithin reports t in [start, end].
func dateWithin(t, start, end time.Time) bool {
	return !t.Before(start) && !t.After(end)
}

// nominalAnchor picks the date the evaluation is measured from, before the
// session roll. A zero result means the event has no usable nominal date.
func nominalAnchor(evt industry.CalendarEvent, kind string) time.Time {
	switch kind {
	case anchorStart:
		return evt.StartDate
	case anchorEnd:
		return evt.EndDate
	default:
		return evt.PeakDate
	}
}

// firstSessionAtOrAfter returns the first session >= nominal and how many
// calendar days it had to move. It reports false when the universe ends before
// the nominal date.
func firstSessionAtOrAfter(sessions []string, nominal string) (string, int, bool) {
	i := sort.SearchStrings(sessions, nominal)
	if i >= len(sessions) {
		return "", 0, false
	}
	if sessions[i] == nominal {
		return sessions[i], 0, true
	}
	from, err1 := time.Parse(dateLayout, nominal)
	to, err2 := time.Parse(dateLayout, sessions[i])
	if err1 != nil || err2 != nil {
		return sessions[i], 0, true
	}
	return sessions[i], int(to.Sub(from).Hours() / 24), true
}

// sessionsFromWindowStart counts sessions from the first session on or after the
// window start up to the anchor (0 when the anchor is at or before the start).
func sessionsFromWindowStart(sessions []string, index map[string]int, anchor, windowStart string) int {
	a, ok := index[anchor]
	if !ok {
		return 0
	}
	s := sort.SearchStrings(sessions, windowStart)
	if s >= len(sessions) || s > a {
		return 0
	}
	return a - s
}

// sessionsToWindowEnd counts sessions from the anchor to the last session on or
// before the window end (0 when the window already closed).
func sessionsToWindowEnd(sessions []string, index map[string]int, anchor, windowEnd string) int {
	a, ok := index[anchor]
	if !ok {
		return 0
	}
	e := sort.SearchStrings(sessions, windowEnd)
	if e >= len(sessions) || sessions[e] > windowEnd {
		e--
	}
	if e < a {
		return 0
	}
	return e - a
}

func uniqueSortedDates(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, d := range in {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// exportAdjustments emits the platform's own event adjustment for every
// (session in [start, end], canonical L1 industry). One calendar instance per
// year is used because RefreshEvents regenerates the year of its argument; a
// session is only evaluated against the calendar of its own year.
func exportAdjustments(reader *marketdata.SectorIndexReader, sessions []string, start, end string) ([]AdjustmentRow, error) {
	// The range read is a guard: a window the reader cannot serve must fail
	// loudly here rather than yield an all-zero adjustment file downstream. Its
	// keys are also the population: the platform's sector id universe (20) is
	// wider than the canonical price universe (18 - no chemicals, no tourism),
	// and the baseline must cover exactly the rows the panel can score.
	ranged, err := reader.ReadRange(mustParseDate(start), mustParseDate(end))
	if err != nil {
		return nil, fmt.Errorf("read sector index range for adjustments: %w", err)
	}
	seenIDs := map[string]struct{}{}
	for _, byIndustry := range ranged {
		for id := range byIndustry {
			seenIDs[id] = struct{}{}
		}
	}
	if len(seenIDs) == 0 {
		return nil, fmt.Errorf("sector index range [%s, %s] has no industry: cannot export adjustments", start, end)
	}
	ids := make([]string, 0, len(seenIDs))
	for id := range seenIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	inWindow := make([]string, 0, len(sessions))
	for _, d := range sessions {
		if d >= start && d <= end {
			inWindow = append(inWindow, d)
		}
	}
	if len(inWindow) == 0 {
		return nil, fmt.Errorf("no session in [%s, %s] for -adjustments-out", start, end)
	}

	cal := industry.NewEventCalendar()
	currentYear := -1
	rows := make([]AdjustmentRow, 0, len(inWindow)*len(ids))
	for _, d := range inWindow {
		day := mustParseDate(d)
		if day.Year() != currentYear {
			cal.RefreshEvents(day)
			currentYear = day.Year()
		}
		active := make([]string, 0, 4)
		for _, evt := range cal.GetEventsForDate(day) {
			active = append(active, evt.EventType)
		}
		sort.Strings(active)
		for _, id := range ids {
			name := ""
			if sid, ok := industry.SectorIDFromString(id); ok {
				name = industry.DisplayZHTw[sid]
			}
			rows = append(rows, AdjustmentRow{
				Date:             d,
				IndustryID:       id,
				IndustryNameZH:   name,
				EventAdjustment:  cal.GetEventAdjustment(id, day),
				ActiveEventTypes: active,
			})
		}
	}
	return rows, nil
}

// mustParseDate parses a date that was already validated by the flag parser.
func mustParseDate(d string) time.Time {
	t, err := time.Parse(dateLayout, d)
	if err != nil {
		return time.Time{}
	}
	return t
}

func writeJSONL[T any](path string, rows []T) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	// Read-only resource release: the project convention drops Close errors on
	// read paths, and the explicit flush below reports a failed write first.
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("encode %s: %w", path, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush %s: %w", path, err)
	}
	return f.Close()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func printSummary(w io.Writer, s eventsSummary) {
	emit := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	emit("jev-eval-events: %d events, anchor=%s, %s..%s\n", s.EventsExported, s.AnchorKind, s.WindowStart, s.WindowEnd)
	emit("session universe: %d sessions, %s..%s\n", s.Sessions, s.SessionFirst, s.SessionLast)
	emit("generated=%d exported=%d rolled=%d dropped(before=%d after=%d no_session=%d peak_outside=%d inverted=%d lunar=%d)\n",
		s.EventsTotal, s.EventsExported, s.RolledAnchors,
		s.DroppedAnchorBeforeWindow, s.DroppedAnchorAfterWindow, s.DroppedNoSessionAtOrAfter,
		s.DroppedPeakOutsideWindow, s.DroppedInvertedWindow, s.DroppedUnverifiedLunar)
	types := make([]string, 0, len(s.ByEventType))
	for t := range s.ByEventType {
		types = append(types, t)
	}
	sort.Strings(types)
	emit("%-24s %6s\n", "event_type", "count")
	for _, t := range types {
		emit("%-24s %6d\n", t, s.ByEventType[t])
	}
	emit("%-24s %6d\n", "TOTAL", s.EventsExported)
}
