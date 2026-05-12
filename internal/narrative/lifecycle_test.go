package narrative

import (
	"testing"
	"time"
)

func TestNewEventLifecycleManager(t *testing.T) {
	lm := NewEventLifecycleManager()
	if lm == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(lm.events) != 0 {
		t.Fatalf("expected empty events, got %d", len(lm.events))
	}
}

func TestAddEventSetsDefaults(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()

	ev := &NarrativeEvent{
		ID:    "test-1",
		Theme: "AI_capex_surge",
	}
	ev.Timestamp = now
	lm.AddEvent(ev)

	if ev.Duration != 90*24*time.Hour {
		t.Fatalf("expected 90d duration for AI_capex_surge, got %v", ev.Duration)
	}
	if ev.ExpiresAt.IsZero() {
		t.Fatal("expected non-zero ExpiresAt")
	}
	if ev.Status != "active" {
		t.Fatalf("expected status active, got %s", ev.Status)
	}
}

func TestAddEventFallsBackToSevenDays(t *testing.T) {
	lm := NewEventLifecycleManager()
	ev := &NarrativeEvent{
		ID:    "test-unknown",
		Theme: "unknown_theme",
	}
	ev.Timestamp = time.Now()
	lm.AddEvent(ev)

	if ev.Duration != 7*24*time.Hour {
		t.Fatalf("expected 7d fallback duration, got %v", ev.Duration)
	}
}

func TestIsThemeActive(t *testing.T) {
	lm := NewEventLifecycleManager()
	ev := &NarrativeEvent{
		ID:    "test-1",
		Theme: "AI_capex_surge",
		Status: "active",
	}
	ev.Timestamp = time.Now()
	lm.AddEvent(ev)

	if !lm.IsThemeActive("AI_capex_surge") {
		t.Fatal("expected AI_capex_surge to be active")
	}
	if lm.IsThemeActive("non_existent") {
		t.Fatal("expected non_existent to not be active")
	}
}

func TestIsThemeActiveConfirmed(t *testing.T) {
	lm := NewEventLifecycleManager()
	ev := &NarrativeEvent{
		ID:    "test-1",
		Theme: "US_rates_up",
		Status: "confirmed",
	}
	ev.Timestamp = time.Now()
	lm.AddEvent(ev)

	if !lm.IsThemeActive("US_rates_up") {
		t.Fatal("expected confirmed event to be considered active")
	}
}

func TestIsThemeActiveExpired(t *testing.T) {
	lm := NewEventLifecycleManager()
	ev := &NarrativeEvent{
		ID:    "test-1",
		Theme: "oil_price_shock",
		Status: "expired",
	}
	ev.Timestamp = time.Now()
	lm.AddEvent(ev)

	if lm.IsThemeActive("oil_price_shock") {
		t.Fatal("expected expired event to NOT be active")
	}
}

func TestGetActiveByTheme(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-a", Theme: "AI_capex_surge", Status: "active", Timestamp: now,
	})
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-b", Theme: "US_rates_up", Status: "expired", Timestamp: now,
	})

	ev := lm.GetActiveByTheme("AI_capex_surge")
	if ev == nil {
		t.Fatal("expected active event")
	}
	if ev.ID != "evt-a" {
		t.Fatalf("expected evt-a, got %s", ev.ID)
	}

	if got := lm.GetActiveByTheme("US_rates_up"); got != nil {
		t.Fatal("expected nil for expired event")
	}
	if got := lm.GetActiveByTheme("non_existent"); got != nil {
		t.Fatal("expected nil for unknown theme")
	}
}

func TestUpdateConfidence(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-a", Theme: "AI_capex_surge", Status: "active", Confidence: 0.5, Timestamp: now,
	})

	lm.UpdateConfidence("evt-a", 0.85)
	ev := lm.GetActiveByTheme("AI_capex_surge")
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.Confidence != 0.85 {
		t.Fatalf("expected confidence 0.85, got %f", ev.Confidence)
	}
}

func TestUpdateConfidenceNonExistent(t *testing.T) {
	lm := NewEventLifecycleManager()
	lm.UpdateConfidence("non-existent", 0.9)
}

func TestGetActiveEvents(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-a", Theme: "AI_capex_surge", Status: "active", Timestamp: now,
	})
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-b", Theme: "US_rates_up", Status: "confirmed", Timestamp: now,
	})
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-c", Theme: "oil_price_shock", Status: "faded", Timestamp: now,
	})
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-d", Theme: "JPY_carry_unwind", Status: "expired", Timestamp: now,
	})

	active := lm.GetActiveEvents()
	if len(active) != 2 {
		t.Fatalf("expected 2 active events (active+confirmed), got %d", len(active))
	}
}

func TestUpdateStatusesToFaded(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	ev := &NarrativeEvent{
		ID: "evt-a", Theme: "US_rates_up", Status: "active",
	}
	ev.Timestamp = now.Add(-6 * 24 * time.Hour)
	ev.Duration = 7 * 24 * time.Hour
	lm.AddEvent(ev)

	lm.UpdateStatuses()
	if ev.Status != "faded" {
		t.Fatalf("expected faded (%.1f%% of duration elapsed), got %s", 
			float64(time.Since(ev.Timestamp))/float64(ev.Duration)*100, ev.Status)
	}
}

func TestUpdateStatusesToExpired(t *testing.T) {
	lm := NewEventLifecycleManager()
	ev := &NarrativeEvent{
		ID: "evt-a", Theme: "US_rates_up", Status: "active",
	}
	ev.Timestamp = time.Now().Add(-10 * 24 * time.Hour)
	ev.Duration = 7 * 24 * time.Hour
	lm.AddEvent(ev)

	lm.UpdateStatuses()
	if ev.Status != "expired" {
		t.Fatalf("expected expired, got %s", ev.Status)
	}
}

func TestConfirmEvent(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-a", Theme: "AI_capex_surge", Status: "active", Timestamp: now,
	})
	lm.ConfirmEvent("evt-a")

	ev := lm.GetActiveByTheme("AI_capex_surge")
	if ev == nil || ev.Status != "confirmed" {
		t.Fatalf("expected confirmed status, got %v", ev)
	}
}

func TestExpireEvent(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{
		ID: "evt-a", Theme: "AI_capex_surge", Status: "active", Timestamp: now,
	})
	lm.ExpireEvent("evt-a")

	if lm.IsThemeActive("AI_capex_surge") {
		t.Fatal("expected event to be inactive after ExpireEvent")
	}
}

func TestClear(t *testing.T) {
	lm := NewEventLifecycleManager()
	now := time.Now()
	lm.AddEvent(&NarrativeEvent{ID: "evt-a", Theme: "AI_capex_surge", Status: "active", Timestamp: now})
	lm.AddEvent(&NarrativeEvent{ID: "evt-b", Theme: "US_rates_up", Status: "active", Timestamp: now})

	lm.Clear()
	if len(lm.events) != 0 {
		t.Fatalf("expected empty after Clear, got %d events", len(lm.events))
	}
}
