package industry

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestIndustryEcosystemIntegration verifies that the four PRs (parameterized
// classification tree, per-industry cycle thresholds, risk aggregation, and
// lunar-automated event calendar) work together as a cohesive ecosystem.
func TestIndustryEcosystemIntegration(t *testing.T) {
	cfg := config.GetParametersConfig()
	if cfg == nil {
		t.Fatal("ParametersConfig not loaded")
	}

	// 1. Classification tree loads from config (PR-1)
	tree := DefaultClassification()
	if tree == nil {
		t.Fatal("DefaultClassification returned nil")
	}
	l1 := tree.GetLevel1()
	if len(l1) < 10 {
		t.Errorf("expected at least 10 L1 industries, got %d", len(l1))
	}

	// 2. Cycle tracker uses per-industry thresholds via config (PR-2)
	tracker := NewCycleTracker()
	// Provide some metrics to trigger phase detection
	tracker.UpdatePosition("financials", IndustryMetrics{
		RevenueGrowthYoY:    0.15,
		ProfitGrowthYoY:     0.20,
		CapacityUtilization: 0.75,
		InventoryTurnover:   5.0,
		PE:                  15.0,
		PB:                  1.5,
	})
	phase, ok := tracker.GetPhase("financials")
	if !ok {
		t.Log("financials phase not yet determinable (needs more data points)")
	} else {
		t.Logf("financials phase: %v", phase)
	}

	// 3. Seasonality engine (PR-2)
	seasonal := NewSeasonalEngine()
	if seasonal == nil {
		t.Fatal("NewSeasonalEngine returned nil")
	}

	// 4. Risk monitor (PR-2)
	rm := NewRiskMonitor()
	risks := rm.GetAllRisks("2330", "semiconductor", 0.05, 1.0)
	if risks == nil {
		t.Error("GetAllRisks should return a slice (possibly empty)")
	}

	// 5. Correlation matrix loads from config (PR-2)
	matrix := DefaultCorrelationMatrix()
	if matrix == nil {
		t.Fatal("DefaultCorrelationMatrix returned nil")
	}
	allCorr := matrix.GetAllCorrelations()
	if len(allCorr) == 0 {
		t.Error("DefaultCorrelationMatrix should load correlations from config")
	}

	// 6. Event calendar with lunar automation (PR-4, ST-8)
	cal := NewEventCalendar()
	if cal == nil {
		t.Fatal("NewEventCalendar returned nil")
	}

	// Refresh for a year inside the hardcoded cache
	now2026 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	cal.RefreshEvents(now2026)
	events2026 := cal.GetAllEvents()
	if len(events2026) == 0 {
		t.Error("expected events for 2026")
	}

	// Refresh for a year outside the original hardcoded range
	now2049 := time.Date(2049, 2, 10, 0, 0, 0, 0, time.UTC)
	cal.RefreshEvents(now2049)
	events2049 := cal.GetAllEvents()
	if len(events2049) == 0 {
		t.Error("expected events for 2049 (lunar auto-computation)")
	}

	// Verify spring festival is active around lunar new year 2049
	active := cal.DetectActiveEvents(now2049)
	hasSpring := false
	for _, e := range active {
		if len(e.ID) >= len("spring_festival") && e.ID[:len("spring_festival")] == "spring_festival" {
			hasSpring = true
			break
		}
	}
	if !hasSpring {
		t.Error("expected spring_festival active near 2049 lunar new year")
	}

	// 7. End-to-end: event adjustment for a known industry
	adj := cal.GetEventAdjustment("financials", now2026)
	if adj < -1.0 || adj > 1.0 {
		t.Errorf("event adjustment %.4f out of reasonable range", adj)
	}

	// 8. Composite sentiment is bounded
	sentiment := cal.GetCompositeEventSentiment(now2026)
	if sentiment < 0.8 || sentiment > 1.2 {
		t.Errorf("composite sentiment %.4f out of [0.8, 1.2]", sentiment)
	}
}

// TestClassificationTreeAndCycleCoherence verifies that every L1 industry
// defined in the classification tree can be fed into the cycle tracker
// without panic, and the tracker produces a position entry.
func TestClassificationTreeAndCycleCoherence(t *testing.T) {
	tree := DefaultClassification()
	tracker := NewCycleTracker()

	for _, seg := range tree.GetLevel1() {
		// UpdatePosition should never panic and should create a history entry
		tracker.UpdatePosition(seg.ID, IndustryMetrics{
			RevenueGrowthYoY:    0.10,
			ProfitGrowthYoY:     0.15,
			CapacityUtilization: 0.70,
			InventoryTurnover:   4.0,
			PE:                  12.0,
			PB:                  1.2,
		})
		history := tracker.GetHistory(seg.ID)
		if len(history) == 0 {
			t.Errorf("industry %q: no history after UpdatePosition", seg.ID)
		}
	}
}
