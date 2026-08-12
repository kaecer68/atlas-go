package alertscanner

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestNarrativeSource_WarningPerTheme verifies a single high-severity
// narrative event surfaces as a WARNING AlertRecord with the correct
// DedupKey (narrative:<theme>) and rule (narrative_theme_detected).
func TestNarrativeSource_WarningPerTheme(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	src := NewNarrativeSource(bus, 0)
	if err := src.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer src.Stop()

	bus.PublishNarrativeEvent(
		"evt-1", "AI_capex_surge", "Global",
		0.8, 0.9, "model", "0.81", "inflow", "1w", "", "",
	)
	time.Sleep(200 * time.Millisecond)

	alerts, err := src.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Rule != "narrative_theme_detected" {
		t.Errorf("rule = %q, want narrative_theme_detected", a.Rule)
	}
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.DedupKey != "narrative:AI_capex_surge" {
		t.Errorf("dedup_key = %q, want narrative:AI_capex_surge", a.DedupKey)
	}
	if a.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestNarrativeSource_DedupSameTheme verifies repeated events for the same
// theme within the TTL window update the existing record instead of
// creating a duplicate.
func TestNarrativeSource_DedupSameTheme(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	src := NewNarrativeSource(bus, 0)
	if err := src.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer src.Stop()

	bus.PublishNarrativeEvent("evt-1", "US_rates_up", "US", 0.6, 0.8, "model", "0.7", "outflow", "1d", "", "")
	time.Sleep(150 * time.Millisecond)
	bus.PublishNarrativeEvent("evt-2", "US_rates_up", "US", 0.7, 0.85, "model", "0.72", "outflow", "1d", "", "")
	time.Sleep(200 * time.Millisecond)

	alerts, err := src.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 deduped alert, got %d", len(alerts))
	}
	if alerts[0].Count < 2 {
		t.Errorf("expected Count >= 2 after dedup, got %d", alerts[0].Count)
	}
}

// TestNarrativeSource_MultiThemeEscalates verifies ≥3 distinct themes
// firing within the dedup window escalate the newest alert to CRITICAL.
func TestNarrativeSource_MultiThemeEscalates(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	src := NewNarrativeSource(bus, 0)
	if err := src.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer src.Stop()

	bus.PublishNarrativeEvent("evt-1", "US_rates_up", "US", 0.5, 0.8, "model", "0.7", "outflow", "1d", "", "")
	bus.PublishNarrativeEvent("evt-2", "JPY_carry_unwind", "JP", 0.4, 0.7, "model", "0.6", "outflow", "1d", "", "")
	bus.PublishNarrativeEvent("evt-3", "geopolitical_risk_spike", "CN", 0.6, 0.9, "model", "0.8", "outflow", "7d", "", "")
	time.Sleep(250 * time.Millisecond)

	alerts, err := src.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts (one per theme), got %d", len(alerts))
	}
	foundCritical := false
	for _, a := range alerts {
		if a.Severity == "critical" {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Fatalf("expected a CRITICAL alert on multi-theme resonance, got %+v", alerts)
	}
}

// TestNarrativeSource_NilBusIsSafe verifies a nil bus is a no-op source.
func TestNarrativeSource_NilBusIsSafe(t *testing.T) {
	src := NewNarrativeSource(nil, 0)
	if err := src.Start(); err != nil {
		t.Fatalf("Start with nil bus should not error, got %v", err)
	}
	src.Stop()
	alerts, err := src.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts with nil bus, got %d", len(alerts))
	}
}
