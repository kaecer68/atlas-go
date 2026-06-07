package industry

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helper: create a SiliconCycleTracker at a specific phase by feeding
// appropriate indicator values through DetectPhase.
// ---------------------------------------------------------------------------

func newSiliconTrackerAtPhase(t *testing.T, phase SiliconCyclePhase) *SiliconCycleTracker {
	t.Helper()
	st := NewSiliconCycleTracker()
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	// Feed indicators to reach the desired phase.
	var ind SiliconIndicators
	switch phase {
	case PhaseBottomRecovery:
		// engine starts here, no transition needed
		ind = SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.05,
			GlobalSemiconductorBillingsYoY: -0.02,
			DRAMSpotPriceTrend:             -0.01,
		}
	case PhaseExpansionConfirmed:
		ind = SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.20,
			GlobalSemiconductorBillingsYoY: 0.15,
			DRAMSpotPriceTrend:             0.05,
		}
	case PhaseOverheat:
		// First go to Expansion, then to Overheat
		st.DetectPhase(now, SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.20,
			GlobalSemiconductorBillingsYoY: 0.15,
			DRAMSpotPriceTrend:             0.05,
		})
		ind = SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.20,
			GlobalSemiconductorBillingsYoY: 0.15,
			DRAMSpotPriceTrend:             0.05,
			TaiwanSemiconductorIndexMA:     0.25,
			PhiladelphiaSOXIndexYoY:        0.50,
		}
	case PhaseContraction:
		// Expansion → Contraction via capex cut
		st.DetectPhase(now, SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.20,
			GlobalSemiconductorBillingsYoY: 0.15,
			DRAMSpotPriceTrend:             0.05,
		})
		ind = SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.20,
			GlobalSemiconductorBillingsYoY: 0.15,
			DRAMSpotPriceTrend:             0.05,
			TSMCCapexGuidance:              -0.15,
		}
	}
	got := st.DetectPhase(now, ind)
	if got != phase {
		t.Fatalf("expected phase %d (%s), got %d (%s)", phase, GetPhaseName(phase), got, GetPhaseName(got))
	}
	return st
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBuildCard_Smoke(t *testing.T) {
	st := newSiliconTrackerAtPhase(t, PhaseExpansionConfirmed)
	ct := NewCycleTracker()
	se := NewSeasonalEngine()
	ec := NewEventCalendar()
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	builder := NewCycleStatusCardBuilder(st, ct, se, ec, nil)

	card, err := builder.BuildCard(now, "semiconductor")
	if err != nil {
		t.Fatalf("BuildCard failed: %v", err)
	}

	if card.SiliconPhase != int(PhaseExpansionConfirmed) {
		t.Errorf("expected silicon phase %d, got %d", PhaseExpansionConfirmed, card.SiliconPhase)
	}
	if card.SiliconPhaseName != "擴張確認" {
		t.Errorf("expected phase name '擴張確認', got '%s'", card.SiliconPhaseName)
	}
	if card.SiliconScore <= 0.9 {
		t.Errorf("expected silicon score > 0.9 for expansion, got %.3f", card.SiliconScore)
	}
	if card.SiliconIndicators == nil {
		t.Error("expected silicon indicators to be non-nil")
	}
	if card.CompositeCoefficient < 0.8 || card.CompositeCoefficient > 1.2 {
		t.Errorf("composite coefficient %.3f out of [0.8, 1.2]", card.CompositeCoefficient)
	}
	if len(card.Breakdown) != 5 {
		t.Errorf("expected 5 breakdown layers, got %d", len(card.Breakdown))
	}
	if card.Breakdown[0].Layer != "silicon" {
		t.Errorf("expected first breakdown layer 'silicon', got '%s'", card.Breakdown[0].Layer)
	}
	if card.Breakdown[4].Layer != "supply_chain" {
		t.Errorf("expected last breakdown layer 'supply_chain', got '%s'", card.Breakdown[4].Layer)
	}
}

// TestBuildCard_NoPhaseTransition_PopulatesIndicators is a regression test
// for the silicon clock 0.00 bug. When a DetectPhase call does not trigger a
// phase transition (history is empty), resolveSiliconLayer must still populate
// card.SiliconIndicators from the tracker's latest snapshot — otherwise the
// frontend renders all six indicators as 0.
func TestBuildCard_NoPhaseTransition_PopulatesIndicators(t *testing.T) {
	st := NewSiliconCycleTracker()
	ct := NewCycleTracker()
	se := NewSeasonalEngine()
	ec := NewEventCalendar()
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	// Feed indicators that keep us in PhaseBottomRecovery (no transition).
	// All six fields populated with non-zero values so we can detect a 0.00 regression.
	live := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.12,
		GlobalSemiconductorBillingsYoY: 0.08,
		DRAMSpotPriceTrend:             0.03,
		TaiwanSemiconductorIndexMA:     0.04,
		TSMCCapexGuidance:              0.05,
		PhiladelphiaSOXIndexYoY:        0.18,
	}
	if phase := st.DetectPhase(now, live); phase != PhaseBottomRecovery {
		t.Fatalf("expected to stay in PhaseBottomRecovery, got %s", GetPhaseName(phase))
	}
	if n := st.GetTransitionCount(); n != 0 {
		t.Fatalf("expected 0 transitions, got %d (test premise broken)", n)
	}

	builder := NewCycleStatusCardBuilder(st, ct, se, ec, nil)
	card, err := builder.BuildCard(now, "semiconductor")
	if err != nil {
		t.Fatalf("BuildCard failed: %v", err)
	}

	if card.SiliconIndicators == nil {
		t.Fatal("regression: SiliconIndicators is nil despite DetectPhase having stored live values")
	}
	got := card.SiliconIndicators
	if got.TSMCMonthlyRevenueYoY != live.TSMCMonthlyRevenueYoY {
		t.Errorf("TSMCMonthlyRevenueYoY = %.4f, want %.4f", got.TSMCMonthlyRevenueYoY, live.TSMCMonthlyRevenueYoY)
	}
	if got.GlobalSemiconductorBillingsYoY != live.GlobalSemiconductorBillingsYoY {
		t.Errorf("GlobalSemiconductorBillingsYoY = %.4f, want %.4f", got.GlobalSemiconductorBillingsYoY, live.GlobalSemiconductorBillingsYoY)
	}
	if got.DRAMSpotPriceTrend != live.DRAMSpotPriceTrend {
		t.Errorf("DRAMSpotPriceTrend = %.4f, want %.4f", got.DRAMSpotPriceTrend, live.DRAMSpotPriceTrend)
	}
	if got.TaiwanSemiconductorIndexMA != live.TaiwanSemiconductorIndexMA {
		t.Errorf("TaiwanSemiconductorIndexMA = %.4f, want %.4f", got.TaiwanSemiconductorIndexMA, live.TaiwanSemiconductorIndexMA)
	}
	if got.TSMCCapexGuidance != live.TSMCCapexGuidance {
		t.Errorf("TSMCCapexGuidance = %.4f, want %.4f", got.TSMCCapexGuidance, live.TSMCCapexGuidance)
	}
	if got.PhiladelphiaSOXIndexYoY != live.PhiladelphiaSOXIndexYoY {
		t.Errorf("PhiladelphiaSOXIndexYoY = %.4f, want %.4f", got.PhiladelphiaSOXIndexYoY, live.PhiladelphiaSOXIndexYoY)
	}
}

// TestGetLatestIndicators covers the new public method on SiliconCycleTracker:
// it must return the most recent indicators (true) after DetectPhase, and
// (zero, false) before any DetectPhase call.
func TestGetLatestIndicators(t *testing.T) {
	st := NewSiliconCycleTracker()

	if _, ok := st.GetLatestIndicators(); ok {
		t.Error("GetLatestIndicators on fresh tracker should return ok=false")
	}

	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	ind := SiliconIndicators{
		TSMCMonthlyRevenueYoY: 0.05,
		DRAMSpotPriceTrend:    0.01,
	}
	st.DetectPhase(now, ind)

	got, ok := st.GetLatestIndicators()
	if !ok {
		t.Fatal("GetLatestIndicators after DetectPhase should return ok=true")
	}
	if got.TSMCMonthlyRevenueYoY != ind.TSMCMonthlyRevenueYoY {
		t.Errorf("TSMCMonthlyRevenueYoY = %.4f, want %.4f", got.TSMCMonthlyRevenueYoY, ind.TSMCMonthlyRevenueYoY)
	}

	// Reset must clear the latest snapshot.
	st.Reset()
	if _, ok := st.GetLatestIndicators(); ok {
		t.Error("GetLatestIndicators after Reset should return ok=false")
	}
}

func TestCompositeCoefficient_Clamped(t *testing.T) {
	// All-extreme bullish: full silicon score, full confidence, high seasonal, high events → should hit 1.2 cap
	cfg := defaultCardConfig()
	coeff := computeCompositeCoefficient(1.0, 1.0, 1.5, 1.5, 0.5, cfg)
	if coeff > cfg.ClampMax {
		t.Errorf("expected coefficient <= %.2f, got %.3f", cfg.ClampMax, coeff)
	}
	if coeff != cfg.ClampMax {
		t.Logf("coefficient clamped to %.3f (max=%.2f)", coeff, cfg.ClampMax)
	}

	// All-extreme bearish: zero silicon score, zero confidence → should hit 0.8 floor
	coeff = computeCompositeCoefficient(0.0, 0.0, 0.5, 0.5, -0.5, cfg)
	if coeff < cfg.ClampMin {
		t.Errorf("expected coefficient >= %.2f, got %.3f", cfg.ClampMin, coeff)
	}
	if coeff != cfg.ClampMin {
		t.Logf("coefficient clamped to %.3f (min=%.2f)", coeff, cfg.ClampMin)
	}

	// Neutral inputs: 0.5 score, 0.5 confidence, 1.0 seasonal, 1.0 events, 0.0 supply → exactly 1.0
	coeff = computeCompositeCoefficient(0.5, 0.5, 1.0, 1.0, 0.0, cfg)
	if coeff != 1.0 {
		t.Errorf("expected neutral coefficient 1.0, got %.3f", coeff)
	}
}

func TestSentimentLabelMapping(t *testing.T) {
	cfg := defaultCardConfig()
	tests := []struct {
		coeff float64
		label string
	}{
		{1.15, "強烈看多"},
		{1.10, "強烈看多"},
		{1.07, "偏多"},
		{1.05, "偏多"},
		{1.02, "中性"},
		{1.00, "中性"},
		{0.97, "中性"},
		{0.95, "中性"},
		{0.92, "偏空"},
		{0.90, "偏空"},
		{0.85, "強烈看空"},
		{0.80, "強烈看空"},
	}
	for _, tc := range tests {
		label := computeSentimentLabel(tc.coeff, cfg)
		if label != tc.label {
			t.Errorf("coefficient=%.2f: expected '%s', got '%s'", tc.coeff, tc.label, label)
		}
	}
}

func TestBuildCard_AllPhases(t *testing.T) {
	phases := []SiliconCyclePhase{
		PhaseBottomRecovery,
		PhaseExpansionConfirmed,
		PhaseOverheat,
		PhaseContraction,
	}
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	for _, phase := range phases {
		t.Run(GetPhaseName(phase), func(t *testing.T) {
			st := newSiliconTrackerAtPhase(t, phase)
			ct := NewCycleTracker()
			se := NewSeasonalEngine()
			ec := NewEventCalendar()

			builder := NewCycleStatusCardBuilder(st, ct, se, ec, nil)
			card, err := builder.BuildCard(now, "semiconductor")
			if err != nil {
				t.Fatalf("BuildCard failed for phase %s: %v", GetPhaseName(phase), err)
			}

			if card.SiliconPhase != int(phase) {
				t.Errorf("expected phase %d, got %d", phase, card.SiliconPhase)
			}
			if card.SiliconPhaseName != GetPhaseName(phase) {
				t.Errorf("expected name '%s', got '%s'", GetPhaseName(phase), card.SiliconPhaseName)
			}
			if card.CompositeCoefficient < 0.8 || card.CompositeCoefficient > 1.2 {
				t.Errorf("coefficient %.3f out of range", card.CompositeCoefficient)
			}
			t.Logf("phase=%s score=%.3f coefficient=%.3f label=%s",
				card.SiliconPhaseName, card.SiliconScore, card.CompositeCoefficient, card.SentimentLabel)
		})
	}
}

func TestEmptyEvents(t *testing.T) {
	st := newSiliconTrackerAtPhase(t, PhaseExpansionConfirmed)
	ct := NewCycleTracker()
	se := NewSeasonalEngine()
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	// Builder with nil event calendar: events layer should be neutral (1.0).
	builder := NewCycleStatusCardBuilder(st, ct, se, nil, nil)
	card, err := builder.BuildCard(now, "semiconductor")
	if err != nil {
		t.Fatalf("BuildCard failed: %v", err)
	}
	if len(card.ActiveEvents) != 0 {
		t.Errorf("expected 0 active events with nil calendar, got %d", len(card.ActiveEvents))
	}
	if card.EventSentiment != 1.0 {
		t.Errorf("expected event sentiment 1.0 with nil calendar, got %.3f", card.EventSentiment)
	}

	// Find the events breakdown layer and verify contribution is 0.
	var eventsAdj *LayerAdjustment
	for i := range card.Breakdown {
		if card.Breakdown[i].Layer == "events" {
			eventsAdj = &card.Breakdown[i]
			break
		}
	}
	if eventsAdj == nil {
		t.Fatal("missing events breakdown layer")
	}
	if eventsAdj.Contribution != 0.0 {
		t.Errorf("expected 0 contribution for nil events, got %.4f", eventsAdj.Contribution)
	}
}

func TestBuildCard_AllNilEngines(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	builder := NewCycleStatusCardBuilder(nil, nil, nil, nil, nil)
	card, err := builder.BuildCard(now, "semiconductor")
	if err != nil {
		t.Fatalf("BuildCard with all nil engines failed: %v", err)
	}

	if card.SiliconScore != 0.5 {
		t.Errorf("expected neutral silicon score 0.5 with nil engine, got %.3f", card.SiliconScore)
	}
	if card.CycleConfidence != 0.5 {
		t.Errorf("expected neutral cycle confidence 0.5 with nil tracker, got %.3f", card.CycleConfidence)
	}
	if card.SeasonalAdjustment != 1.0 {
		t.Errorf("expected seasonal adjustment 1.0 with nil engine, got %.3f", card.SeasonalAdjustment)
	}
	if card.EventSentiment != 1.0 {
		t.Errorf("expected event sentiment 1.0 with nil calendar, got %.3f", card.EventSentiment)
	}
	if card.SupplyChainSignal != 0.0 {
		t.Errorf("expected supply chain signal 0.0 with nil linkage, got %.3f", card.SupplyChainSignal)
	}
	if card.CompositeCoefficient != 1.0 {
		t.Errorf("expected neutral composite 1.0 with all nil, got %.3f", card.CompositeCoefficient)
	}
}

func TestBuildCompositeCard(t *testing.T) {
	st := newSiliconTrackerAtPhase(t, PhaseExpansionConfirmed)
	ct := NewCycleTracker()
	se := NewSeasonalEngine()
	ec := NewEventCalendar()
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	builder := NewCycleStatusCardBuilder(st, ct, se, ec, nil)
	card, err := builder.BuildCompositeCard(now)
	if err != nil {
		t.Fatalf("BuildCompositeCard failed: %v", err)
	}

	if card.SiliconPhase != int(PhaseExpansionConfirmed) {
		t.Errorf("expected phase %d, got %d", PhaseExpansionConfirmed, card.SiliconPhase)
	}
	if card.CompositeCoefficient < 0.8 || card.CompositeCoefficient > 1.2 {
		t.Errorf("coefficient %.3f out of range", card.CompositeCoefficient)
	}
	if card.SentimentLabel == "" {
		t.Error("expected non-empty sentiment label")
	}
	if len(card.Breakdown) != 5 {
		t.Errorf("expected 5 breakdown layers, got %d", len(card.Breakdown))
	}
	t.Logf("composite card: coefficient=%.3f label=%s", card.CompositeCoefficient, card.SentimentLabel)
}

func TestBuildAdj(t *testing.T) {
	tests := []struct {
		name   string
		layer  string
		raw    float64
		weight float64
		exp    float64
	}{
		{"silicon above neutral", "silicon", 0.8, 0.25, 0.0750},
		{"silicon below neutral", "silicon", 0.3, 0.25, -0.0500},
		{"cycle above neutral", "business_cycle", 0.7, 0.20, 0.0400},
		{"seasonal strong", "seasonal", 1.3, 0.15, 0.0450},
		{"events bearish", "events", 0.9, 0.15, -0.0150},
		{"supply chain positive", "supply_chain", 0.3, 0.10, 0.0300},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adj := buildAdj(tc.layer, tc.raw, tc.weight, "test")
			// Use approximate comparison for rounded values
			diff := adj.Contribution - tc.exp
			if diff > 0.0001 || diff < -0.0001 {
				t.Errorf("expected contribution %.4f, got %.4f (raw=%.2f, weight=%.2f)",
					tc.exp, adj.Contribution, tc.raw, tc.weight)
			}
		})
	}
}
