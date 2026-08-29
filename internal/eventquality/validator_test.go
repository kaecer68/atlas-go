package eventquality

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedNow returns a clock pinned to 2026-07-13T00:00:00Z so the date-range
// rule is deterministic across test runs and machines.
func fixedNow() time.Time {
	return time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
}

// validRawEvent returns a fully-valid RawEvent anchored at fixedNow().
func validRawEvent() RawEvent {
	now := fixedNow()
	return RawEvent{
		EventID:        "ev-001",
		EventType:      "earnings",
		EffectiveDate:  now.Add(5 * 24 * time.Hour), // 5 days in future
		SymbolOrSector: "2330",
		Title:          "TSMC Q2 earnings call",
		TriggerTheme:   "earnings_release",
		Source:         "twse_provider",
		Confidence:     0.85,
		IngestedAt:     now,
	}
}

func TestValidator_AcceptsValidEvent(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)

	r := v.Validate(validRawEvent())
	if !r.Accepted {
		t.Errorf("expected accepted, got rejected: rule=%s field=%s reason=%s", r.Rule, r.Field, r.Reason)
	}
}

// --- Rule 1: required fields ---

func TestValidator_RejectsMissingEventID(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)

	e := validRawEvent()
	e.EventID = ""
	r := v.Validate(e)
	if r.Accepted {
		t.Fatal("expected rejected")
	}
	if r.Rule != "required_fields" || r.Field != "event_id" {
		t.Errorf("got rule=%s field=%s, want required_fields/event_id", r.Rule, r.Field)
	}
}

func TestValidator_RejectsMissingEventType(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.EventType = ""
	r := v.Validate(e)
	if r.Accepted || r.Field != "event_type" {
		t.Errorf("got %+v, want rejected with field=event_type", r)
	}
}

func TestValidator_RejectsMissingSymbolOrSector(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.SymbolOrSector = ""
	r := v.Validate(e)
	if r.Accepted || r.Field != "symbol_or_sector" {
		t.Errorf("got %+v, want rejected with field=symbol_or_sector", r)
	}
}

func TestValidator_RejectsZeroEffectiveDate(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.EffectiveDate = time.Time{}
	r := v.Validate(e)
	if r.Accepted || r.Field != "effective_date" {
		t.Errorf("got %+v, want rejected with field=effective_date", r)
	}
}

// --- Rule 2: source marking ---

func TestValidator_RejectsEmptySource(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.Source = ""
	r := v.Validate(e)
	if r.Accepted || r.Rule != "source_marking" || r.Field != "source" {
		t.Errorf("got %+v, want source_marking/source", r)
	}
}

func TestValidator_RejectsZeroIngestedAt(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.IngestedAt = time.Time{}
	r := v.Validate(e)
	if r.Accepted || r.Field != "ingested_at" {
		t.Errorf("got %+v, want rejected with field=ingested_at", r)
	}
}

// --- Rule 3: date range ---

func TestValidator_RejectsDateTooOld(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.EffectiveDate = fixedNow().AddDate(0, 0, -31) // 31 days ago
	r := v.Validate(e)
	if r.Accepted || r.Rule != "date_range" {
		t.Errorf("got %+v, want rejected with date_range", r)
	}
}

func TestValidator_RejectsDateTooFar(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.EffectiveDate = fixedNow().AddDate(0, 0, 91) // 91 days future
	r := v.Validate(e)
	if r.Accepted || r.Rule != "date_range" {
		t.Errorf("got %+v, want rejected with date_range", r)
	}
}

func TestValidator_AcceptsDateAtBoundary(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.EffectiveDate = fixedNow().AddDate(0, 0, 30) // exactly 30 days future
	if r := v.Validate(e); !r.Accepted {
		t.Errorf("30-day future should be accepted, got %+v", r)
	}
}

// --- Rule 4: confidence ---

func TestValidator_RejectsNegativeConfidence(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.Confidence = -0.1
	r := v.Validate(e)
	if r.Accepted || r.Rule != "confidence" {
		t.Errorf("got %+v, want rejected with confidence", r)
	}
}

func TestValidator_RejectsOverOneConfidence(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.Confidence = 1.5
	r := v.Validate(e)
	if r.Accepted || r.Rule != "confidence" {
		t.Errorf("got %+v, want rejected with confidence", r)
	}
}

func TestValidator_AcceptsHumanLabeledConfidence(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.Confidence = 1.0
	if r := v.Validate(e); !r.Accepted {
		t.Errorf("confidence=1.0 (human-labeled) should be accepted, got %+v", r)
	}
}

// --- Rule 5: dedup ---

func TestValidator_RejectsDuplicateEvent(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	if r := v.Validate(e); !r.Accepted {
		t.Fatal("first event should be accepted")
	}
	r := v.Validate(e)
	if r.Accepted || r.Rule != "dedup" {
		t.Errorf("duplicate should be rejected with dedup, got %+v", r)
	}
}

func TestValidator_DedupKeyIsScopedByDate(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e1 := validRawEvent()
	e1.EffectiveDate = fixedNow().Add(48 * time.Hour)
	e2 := validRawEvent()
	e2.EffectiveDate = fixedNow().Add(72 * time.Hour) // different date
	if r := v.Validate(e1); !r.Accepted {
		t.Fatal("e1 should be accepted")
	}
	if r := v.Validate(e2); !r.Accepted {
		t.Errorf("e2 different date should be accepted, got %+v", r)
	}
}

func TestValidator_DedupKeyIsScopedBySymbolAndTheme(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e1 := validRawEvent()
	if r := v.Validate(e1); !r.Accepted {
		t.Fatal("e1 should be accepted")
	}
	e2 := validRawEvent()
	e2.SymbolOrSector = "2454" // different symbol
	if r := v.Validate(e2); !r.Accepted {
		t.Errorf("different symbol should be accepted, got %+v", r)
	}
	e3 := validRawEvent()
	e3.SymbolOrSector = "2330"
	e3.TriggerTheme = "msci_rebalance" // different theme
	if r := v.Validate(e3); !r.Accepted {
		t.Errorf("different theme should be accepted, got %+v", r)
	}
}

func TestValidator_DedupTTLExpiry(t *testing.T) {
	v := NewValidator(DateRange{}, 0) // dedupTTL = 7 days
	clock := fixedNow()
	v.SetClock(func() time.Time { return clock })

	e := validRawEvent()
	if r := v.Validate(e); !r.Accepted {
		t.Fatal("first should be accepted")
	}

	clock = clock.Add(8 * 24 * time.Hour) // 8 days later
	if r := v.Validate(e); !r.Accepted {
		t.Errorf("after TTL expiry, should be accepted, got %+v", r)
	}
}

// --- Rule ordering: required fields surfaces first ---

func TestValidator_ReportsFirstFailingRule(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.EventID = ""   // required fields
	e.Source = ""    // source marking
	e.Confidence = 5 // confidence
	r := v.Validate(e)
	if r.Accepted {
		t.Fatal("expected rejected")
	}
	if r.Rule != "required_fields" {
		t.Errorf("expected required_fields to be reported first, got %s", r.Rule)
	}
}

// --- QualityLog ---

func TestQualityLog_WritesJSONL(t *testing.T) {
	var buf bytes.Buffer
	log := NewQualityLog(&buf)
	r := ValidationResult{
		EventID:   "ev-1",
		Accepted:  false,
		Rule:      "dedup",
		Field:     "trigger_theme+symbol+date",
		Reason:    "duplicate",
		CheckedAt: fixedNow(),
	}
	if err := log.Record(r); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
		t.Errorf("expected JSONL, got %q", line)
	}
	var got ValidationResult
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if got != r {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, r)
	}
}

func TestQualityLog_ConcurrentSafe(t *testing.T) {
	var buf bytes.Buffer
	log := NewQualityLog(&buf)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = log.Record(ValidationResult{
				EventID:   "ev",
				Accepted:  i%2 == 0,
				CheckedAt: fixedNow(),
			})
		}(i)
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 50 {
		t.Errorf("expected 50 lines, got %d", len(lines))
	}
}

// --- dedup key format ---

func TestDedupKeyFormat(t *testing.T) {
	e := RawEvent{
		TriggerTheme:   "earnings_release",
		SymbolOrSector: "2330",
		EffectiveDate:  time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC),
	}
	got := e.dedupKey()
	want := "earnings_release|2330|2026-07-13"
	if got != want {
		t.Errorf("dedupKey = %q, want %q", got, want)
	}
}

func TestValidator_BackfilledEventPassesValidation(t *testing.T) {
	v := NewValidator(DateRange{}, 0)
	v.SetClock(fixedNow)
	e := validRawEvent()
	e.IsBackfill = true
	r := v.Validate(e)
	if !r.Accepted {
		t.Errorf("backfilled event should pass validation, got rejected: rule=%s reason=%s", r.Rule, r.Reason)
	}
	if !r.IsBackfill {
		t.Error("IsBackfill flag must propagate to ValidationResult for downstream predictors")
	}
}

func TestValidationResult_JSONRoundTrip_BackfillFlag(t *testing.T) {
	r := ValidationResult{
		EventID:    "ev-001",
		Accepted:   true,
		CheckedAt:  fixedNow(),
		IsBackfill: true,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(data, []byte(`"is_backfill":true`)) {
		t.Errorf("expected is_backfill:true in JSON, got %s", data)
	}
	var back ValidationResult
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.IsBackfill {
		t.Error("IsBackfill lost across JSON round-trip")
	}
}
