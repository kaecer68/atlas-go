// Package eventquality implements the event data quality gate for atlas-go.
//
// Stage 2 (~/workspace/atlas-notes/05-decisions/2026-07-13-stage-2-data-quality.md)
// mandates that any event ingested through a public API or background task must
// pass EventValidator.Validate() before being written to the event calendar.
// Rejected events are recorded in the QualityLog for downstream audit and
// debugging. The package is intentionally decoupled from internal/industry and
// internal/eventdriven so existing data structures (CalendarEvent,
// EventCalendarItem) can keep their current schema while gaining a quality
// gate on the ingestion boundary.
package eventquality

import (
	"fmt"
	"sync"
	"time"
)

// RawEvent is the ingestion input fed to the validator. It mirrors the fields
// required by the Stage 2 spec and intentionally does NOT depend on any
// downstream type (CalendarEvent, EventCalendarItem) so the validator can be
// unit-tested in isolation.
type RawEvent struct {
	EventID        string
	EventType      string
	EffectiveDate  time.Time
	SymbolOrSector string
	Title          string
	TriggerTheme   string
	Source         string
	Confidence     float64
	IngestedAt     time.Time
	IsBackfill     bool
}

// dedupKey is the composite (trigger_theme, symbol, date) key used for the
// dedup rule. Date is normalised to UTC midnight so two ingestion events
// from the same source on the same trading day collapse to one key.
func (e RawEvent) dedupKey() string {
	return fmt.Sprintf("%s|%s|%s",
		e.TriggerTheme, e.SymbolOrSector, e.EffectiveDate.UTC().Format("2006-01-02"))
}

// ValidationResult is the outcome of a single Validate() call. Accepted=true
// means the event may proceed to the calendar. Accepted=false means the event
// must be rejected; Rule, Field and Reason describe the first failure.
type ValidationResult struct {
	EventID    string    `json:"event_id"`
	Accepted   bool      `json:"accepted"`
	Rule       string    `json:"rule,omitempty"`
	Field      string    `json:"field,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
	IsBackfill bool      `json:"is_backfill,omitempty"`
}

// EventValidator validates RawEvent instances against the 5 Stage 2 rules.
// Implementations must be safe for concurrent use (the validator is called
// from background ingestion tasks and from request handlers).
type EventValidator interface {
	Validate(event RawEvent) ValidationResult
}

// DateRange configures the date-sanity rule. PastBound and FutureBound are
// expressed as time.Duration values (negative for past). DefaultConfig() returns
// a validator with the spec defaults: 30 days past, 90 days future.
type DateRange struct {
	PastBound   time.Duration // e.g. -30 * 24 * time.Hour
	FutureBound time.Duration // e.g.  90 * 24 * time.Hour
}

// Validator is the concrete EventValidator implementation. It tracks recent
// (trigger_theme, symbol, date) tuples in an in-memory ring keyed by the
// dedup key; entries expire after dedupTTL so the map size stays bounded.
type Validator struct {
	mu        sync.Mutex
	seen      map[string]time.Time
	dedupTTL  time.Duration
	dateRange DateRange
	now       func() time.Time // injectable clock for tests
}

// NewValidator returns a Validator configured per spec. Pass dedupTTL = 0
// to get the default 7-day window. Pass dateRange = DateRange{} for spec
// defaults (30d / 90d).
func NewValidator(dateRange DateRange, dedupTTL time.Duration) *Validator {
	if dedupTTL == 0 {
		dedupTTL = 7 * 24 * time.Hour
	}
	if dateRange.PastBound == 0 {
		dateRange.PastBound = -30 * 24 * time.Hour
	}
	if dateRange.FutureBound == 0 {
		dateRange.FutureBound = 90 * 24 * time.Hour
	}
	return &Validator{
		seen:      make(map[string]time.Time),
		dedupTTL:  dedupTTL,
		dateRange: dateRange,
		now:       time.Now,
	}
}

// SetClock replaces the clock used for date checks; tests use it to make
// the date-range rule deterministic.
func (v *Validator) SetClock(now func() time.Time) {
	v.mu.Lock()
	v.now = now
	v.mu.Unlock()
}

// Validate runs the 5 rules in spec order and returns the first failure.
// Rules are checked top-to-bottom so the most fundamental problems (missing
// required fields) surface first.
func (v *Validator) Validate(event RawEvent) ValidationResult {
	now := v.now()

	if r := v.validateRequiredFields(event, now); !r.Accepted {
		return r
	}
	if r := v.validateSourceMarking(event, now); !r.Accepted {
		return r
	}
	if r := v.validateDateRange(event, now); !r.Accepted {
		return r
	}
	if r := v.validateConfidence(event, now); !r.Accepted {
		return r
	}
	if r := v.validateDedup(event, now); !r.Accepted {
		return r
	}
	return ValidationResult{EventID: event.EventID, Accepted: true, CheckedAt: now, IsBackfill: event.IsBackfill}
}

func (v *Validator) validateRequiredFields(e RawEvent, now time.Time) ValidationResult {
	if e.EventID == "" {
		return reject(e, now, "required_fields", "event_id", "event_id is required")
	}
	if e.EventType == "" {
		return reject(e, now, "required_fields", "event_type", "event_type is required")
	}
	if e.SymbolOrSector == "" {
		return reject(e, now, "required_fields", "symbol_or_sector", "symbol_or_sector is required")
	}
	if e.EffectiveDate.IsZero() {
		return reject(e, now, "required_fields", "effective_date", "effective_date is required")
	}
	return ValidationResult{EventID: e.EventID, Accepted: true, CheckedAt: now}
}

func (v *Validator) validateSourceMarking(e RawEvent, now time.Time) ValidationResult {
	if e.Source == "" {
		return reject(e, now, "source_marking", "source", "source is required for traceability")
	}
	if e.IngestedAt.IsZero() {
		return reject(e, now, "source_marking", "ingested_at", "ingested_at is required for traceability")
	}
	return ValidationResult{EventID: e.EventID, Accepted: true, CheckedAt: now}
}

func (v *Validator) validateDateRange(e RawEvent, now time.Time) ValidationResult {
	cutoff := now.Add(v.dateRange.PastBound)
	if e.EffectiveDate.Before(cutoff) {
		return reject(e, now, "date_range", "effective_date",
			fmt.Sprintf("effective_date %s is more than %s in the past",
				e.EffectiveDate.Format("2006-01-02"), v.dateRange.PastBound))
	}
	futureCutoff := now.Add(v.dateRange.FutureBound)
	if e.EffectiveDate.After(futureCutoff) {
		return reject(e, now, "date_range", "effective_date",
			fmt.Sprintf("effective_date %s is more than %s in the future",
				e.EffectiveDate.Format("2006-01-02"), v.dateRange.FutureBound))
	}
	return ValidationResult{EventID: e.EventID, Accepted: true, CheckedAt: now}
}

func (v *Validator) validateConfidence(e RawEvent, now time.Time) ValidationResult {
	if e.Confidence < 0 || e.Confidence > 1 {
		return reject(e, now, "confidence", "confidence",
			fmt.Sprintf("confidence must be in [0, 1], got %f", e.Confidence))
	}
	return ValidationResult{EventID: e.EventID, Accepted: true, CheckedAt: now}
}

func (v *Validator) validateDedup(e RawEvent, now time.Time) ValidationResult {
	key := e.dedupKey()
	v.mu.Lock()
	defer v.mu.Unlock()

	v.gcLocked(now)
	if prev, ok := v.seen[key]; ok {
		return reject(e, now, "dedup", "trigger_theme+symbol+date",
			fmt.Sprintf("duplicate event for key %s (previous ingested at %s)",
				key, prev.Format(time.RFC3339)))
	}
	v.seen[key] = e.IngestedAt
	return ValidationResult{EventID: e.EventID, Accepted: true, CheckedAt: now}
}

func (v *Validator) gcLocked(now time.Time) {
	cutoff := now.Add(-v.dedupTTL)
	for k, ts := range v.seen {
		if ts.Before(cutoff) {
			delete(v.seen, k)
		}
	}
}

func reject(e RawEvent, now time.Time, rule, field, reason string) ValidationResult {
	return ValidationResult{
		EventID:   e.EventID,
		Accepted:  false,
		Rule:      rule,
		Field:     field,
		Reason:    reason,
		CheckedAt: now,
	}
}
