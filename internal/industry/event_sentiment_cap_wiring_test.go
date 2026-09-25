package industry

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestComputeSentimentAdjustment_ConsumesConfigCap is the N-C3 wiring proof
// (#1944 Batch 4): industry.event_sentiment_cap used to be declared in config
// while the ±0.05 cap was hardcoded here, so changing the config changed
// nothing. With the knob wired, a different value must reach the computation.
func TestComputeSentimentAdjustment_ConsumesConfigCap(t *testing.T) {
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	evt := CalendarEvent{
		ID:         "test_event",
		Direction:  "bullish",
		BaseWeight: 1.0,
		DecayDays:  5,
		PeakDate:   now,
		StartDate:  now,
		EndDate:    now.AddDate(0, 0, 5),
	}

	// No config loaded → hardcoded default (equals the shipped value).
	noConfig := &EventCalendar{}
	if got := noConfig.computeSentimentAdjustment(evt, now); got != defaultEventSentimentCap {
		t.Errorf("no-config adjustment = %v, want %v", got, defaultEventSentimentCap)
	}

	// Shipped config value → same result (behaviour-neutral wiring).
	shipped := config.DefaultParametersConfig()
	shippedCal := &EventCalendar{config: shipped}
	shippedCap := shipped.Industry.EventSentimentCap.Value
	if got := shippedCal.computeSentimentAdjustment(evt, now); got != shippedCap {
		t.Errorf("shipped-config adjustment = %v, want %v", got, shippedCap)
	}

	// A tightened cap must actually tighten the result.
	tight := config.DefaultParametersConfig()
	tight.Industry.EventSentimentCap.Value = 0.01
	tightCal := &EventCalendar{config: tight}
	if got := tightCal.computeSentimentAdjustment(evt, now); got != 0.01 {
		t.Errorf("tightened-cap adjustment = %v, want 0.01 (config must be consumed)", got)
	}

	// A negative/bearish event must clamp at the same magnitude.
	bearish := evt
	bearish.Direction = "bearish"
	if got := tightCal.computeSentimentAdjustment(bearish, now); got != -0.01 {
		t.Errorf("tightened-cap bearish adjustment = %v, want -0.01", got)
	}
	if got := noConfig.computeSentimentAdjustment(bearish, now); got != -defaultEventSentimentCap {
		t.Errorf("default bearish adjustment = %v, want %v", got, -defaultEventSentimentCap)
	}

	// A non-positive config value must fall back, never zero out the cap.
	zeroed := config.DefaultParametersConfig()
	zeroed.Industry.EventSentimentCap.Value = 0
	zeroedCal := &EventCalendar{config: zeroed}
	if got := zeroedCal.computeSentimentAdjustment(evt, now); math.Abs(got-defaultEventSentimentCap) > 1e-9 {
		t.Errorf("zero config cap adjustment = %v, want fallback %v", got, defaultEventSentimentCap)
	}
}

// TestShippedEventSentimentCapMatchesHardcodedDefault documents that the wiring
// above is behaviour-neutral for the shipped config.
func TestShippedEventSentimentCapMatchesHardcodedDefault(t *testing.T) {
	shipped := config.DefaultParametersConfig().Industry.EventSentimentCap.Value
	if math.Abs(shipped-defaultEventSentimentCap) > 1e-9 {
		t.Fatalf("shipped industry.event_sentiment_cap = %v, hardcoded default = %v — the wiring is no longer behaviour-neutral; report it in the PR body",
			shipped, defaultEventSentimentCap)
	}
}
