package industry

import (
	"testing"
	"time"
)

func TestNewSiliconCycleTracker(t *testing.T) {
	e := NewSiliconCycleTracker()
	if e == nil {
		t.Fatal("expected non-nil SiliconCycleTracker")
	}
	if e.currentPhase != PhaseBottomRecovery {
		t.Errorf("expected initial phase BottomRecovery(0), got %d", e.currentPhase)
	}
	if e.history == nil {
		t.Error("expected history slice to be initialized")
	}
	if len(e.history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(e.history))
	}
	if phase := e.GetCurrentPhase(); phase != PhaseBottomRecovery {
		t.Errorf("GetCurrentPhase() = %d, want PhaseBottomRecovery(0)", phase)
	}
}

func TestDetectPhase_AllPhases(t *testing.T) {
	now := time.Now()

	// Helper: indicators that trigger 0→1 (bottom recovery → expansion).
	expansionIndicators := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     0.10,
		PhiladelphiaSOXIndexYoY:        0.25,
		TSMCCapexGuidance:              0.0,
	}

	// Helper: indicators that trigger 1→2 (expansion → overheat) via index deviation.
	overheatIndexIndicators := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     0.25,
		PhiladelphiaSOXIndexYoY:        0.25,
		TSMCCapexGuidance:              0.0,
	}

	// Helper: indicators that trigger 1→2 via SOX extreme.
	overheatSOXIndicators := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     0.10,
		PhiladelphiaSOXIndexYoY:        0.50,
		TSMCCapexGuidance:              0.0,
	}

	// Helper: indicators that trigger 2→3 (overheat → contraction) via capex cut.
	contractionIndicators := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.25,
		GlobalSemiconductorBillingsYoY: 0.15,
		DRAMSpotPriceTrend:             -0.05,
		TaiwanSemiconductorIndexMA:     0.30,
		PhiladelphiaSOXIndexYoY:        0.35,
		TSMCCapexGuidance:              -0.15,
	}

	// Test: phase 0 → stays at 0 (indicators below recovery thresholds).
	t.Run("phase0_stays_when_weak", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		phase := e.DetectPhase(now, SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.05,
			GlobalSemiconductorBillingsYoY: 0.02,
			DRAMSpotPriceTrend:             -0.10,
		})
		if phase != PhaseBottomRecovery {
			t.Errorf("got %s, want PhaseBottomRecovery", GetPhaseName(phase))
		}
	})

	// Test: phase 0 → 1 (recovery → expansion).
	t.Run("phase0_to_1_expansion", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		phase := e.DetectPhase(now, expansionIndicators)
		if phase != PhaseExpansionConfirmed {
			t.Errorf("got %s, want PhaseExpansionConfirmed", GetPhaseName(phase))
		}
	})

	// Test: phase 1 → 2 via index above MA threshold.
	t.Run("phase1_to_2_overheat_index", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		e.DetectPhase(now, expansionIndicators) // 0→1 first
		now = now.Add(24 * time.Hour)
		phase := e.DetectPhase(now, overheatIndexIndicators) // 1→2
		if phase != PhaseOverheat {
			t.Errorf("got %s, want PhaseOverheat", GetPhaseName(phase))
		}
	})

	// Test: phase 1 → 2 via SOX extreme threshold.
	t.Run("phase1_to_2_overheat_sox", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		e.DetectPhase(now, expansionIndicators) // 0→1 first
		now = now.Add(24 * time.Hour)
		phase := e.DetectPhase(now, overheatSOXIndicators) // 1→2
		if phase != PhaseOverheat {
			t.Errorf("got %s, want PhaseOverheat", GetPhaseName(phase))
		}
	})

	// Test: phase 2 → 3 (overheat → contraction via capex cut).
	t.Run("phase2_to_3_contraction", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		e.DetectPhase(now, expansionIndicators) // 0→1
		now = now.Add(24 * time.Hour)
		e.DetectPhase(now, overheatIndexIndicators) // 1→2
		now = now.Add(24 * time.Hour)
		phase := e.DetectPhase(now, contractionIndicators) // 2→3
		if phase != PhaseContraction {
			t.Errorf("got %s, want PhaseContraction", GetPhaseName(phase))
		}
	})

	// Test: phase 3 → 0 (contraction → recovery via billings stabilization + DRAM bottoming).
	t.Run("phase3_to_0_recovery", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		e.DetectPhase(now, expansionIndicators) // 0→1
		now = now.Add(24 * time.Hour)
		e.DetectPhase(now, overheatIndexIndicators) // 1→2
		now = now.Add(24 * time.Hour)
		e.DetectPhase(now, contractionIndicators) // 2→3
		now = now.Add(60 * 24 * time.Hour)
		phase := e.DetectPhase(now, SiliconIndicators{
			GlobalSemiconductorBillingsYoY: 0.02,
			DRAMSpotPriceTrend:             0.01,
		})
		if phase != PhaseBottomRecovery {
			t.Errorf("got %s, want PhaseBottomRecovery", GetPhaseName(phase))
		}
	})

	// Test: phase 3 stays in contraction (indicators still weak).
	t.Run("phase3_stays_when_weak", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		e.DetectPhase(now, expansionIndicators) // 0→1
		now = now.Add(24 * time.Hour)
		e.DetectPhase(now, overheatIndexIndicators) // 1→2
		now = now.Add(24 * time.Hour)
		e.DetectPhase(now, contractionIndicators) // 2→3
		now = now.Add(24 * time.Hour)
		phase := e.DetectPhase(now, SiliconIndicators{
			TSMCMonthlyRevenueYoY:          -0.10,
			GlobalSemiconductorBillingsYoY: -0.15,
			DRAMSpotPriceTrend:             -0.20,
		})
		if phase != PhaseContraction {
			t.Errorf("got %s, want PhaseContraction", GetPhaseName(phase))
		}
	})

	// Test: phase 1 expands directly to 3 via capex cut (contraction signal skips overheat).
	t.Run("phase1_expands_direct_to_3", func(t *testing.T) {
		e := NewSiliconCycleTracker()
		e.DetectPhase(now, expansionIndicators) // 0→1
		now = now.Add(24 * time.Hour)
		phase := e.DetectPhase(now, SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.30,
			GlobalSemiconductorBillingsYoY: 0.20,
			DRAMSpotPriceTrend:             0.05,
			TaiwanSemiconductorIndexMA:     0.10,
			PhiladelphiaSOXIndexYoY:        0.25,
			TSMCCapexGuidance:              -0.15,
		})
		if phase != PhaseContraction {
			t.Errorf("got %s, want PhaseContraction", GetPhaseName(phase))
		}
	})
}

func TestPhaseTransitions(t *testing.T) {
	now := time.Now()
	e := NewSiliconCycleTracker()

	// Start at phase 0 (bottom recovery)
	if e.GetCurrentPhase() != PhaseBottomRecovery {
		t.Fatalf("expected initial phase 0, got %d", e.GetCurrentPhase())
	}

	// Transition 0 → 1: expansion signal
	expSignal := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     0.10,
		PhiladelphiaSOXIndexYoY:        0.25,
		TSMCCapexGuidance:              0.0,
	}
	phase := e.DetectPhase(now, expSignal)
	if phase != PhaseExpansionConfirmed {
		t.Errorf("0→1: expected PhaseExpansionConfirmed, got %s", GetPhaseName(phase))
	}
	if e.GetTransitionCount() != 1 {
		t.Errorf("0→1: expected 1 transition, got %d", e.GetTransitionCount())
	}

	// Transition 1 → 2: overheat signal (index above MA)
	now = now.Add(30 * 24 * time.Hour)
	overheatSignal := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     0.30,
		PhiladelphiaSOXIndexYoY:        0.25,
		TSMCCapexGuidance:              0.0,
	}
	phase = e.DetectPhase(now, overheatSignal)
	if phase != PhaseOverheat {
		t.Errorf("1→2: expected PhaseOverheat, got %s", GetPhaseName(phase))
	}
	if e.GetTransitionCount() != 2 {
		t.Errorf("1→2: expected 2 transitions, got %d", e.GetTransitionCount())
	}

	// Transition 2 → 3: capex cut
	now = now.Add(30 * 24 * time.Hour)
	contractionSignal := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.25,
		GlobalSemiconductorBillingsYoY: 0.15,
		DRAMSpotPriceTrend:             -0.05,
		TaiwanSemiconductorIndexMA:     0.30,
		PhiladelphiaSOXIndexYoY:        0.35,
		TSMCCapexGuidance:              -0.15,
	}
	phase = e.DetectPhase(now, contractionSignal)
	if phase != PhaseContraction {
		t.Errorf("2→3: expected PhaseContraction, got %s", GetPhaseName(phase))
	}
	if e.GetTransitionCount() != 3 {
		t.Errorf("2→3: expected 3 transitions, got %d", e.GetTransitionCount())
	}

	// Transition 3 → 0: recovery signal
	now = now.Add(60 * 24 * time.Hour)
	recoverySignal := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.05,
		GlobalSemiconductorBillingsYoY: 0.02,
		DRAMSpotPriceTrend:             0.01,
		TaiwanSemiconductorIndexMA:     -0.05,
		PhiladelphiaSOXIndexYoY:        -0.05,
		TSMCCapexGuidance:              -0.05,
	}
	phase = e.DetectPhase(now, recoverySignal)
	if phase != PhaseBottomRecovery {
		t.Errorf("3→0: expected PhaseBottomRecovery, got %s", GetPhaseName(phase))
	}
	if e.GetTransitionCount() != 4 {
		t.Errorf("3→0: expected 4 transitions, got %d", e.GetTransitionCount())
	}
}

func TestPhaseWeightMultiplier(t *testing.T) {
	tests := []struct {
		phase          SiliconCyclePhase
		wantMultiplier float64
	}{
		{PhaseBottomRecovery, 1.05},
		{PhaseExpansionConfirmed, 1.10},
		{PhaseOverheat, 0.90},
		{PhaseContraction, 0.85},
	}

	for _, tt := range tests {
		got := GetPhaseWeightMultiplier(tt.phase)
		if got != tt.wantMultiplier {
			t.Errorf("GetPhaseWeightMultiplier(%d) = %.2f, want %.2f",
				tt.phase, got, tt.wantMultiplier)
		}
	}
}

func TestPhaseScore(t *testing.T) {
	tests := []struct {
		phase     SiliconCyclePhase
		wantScore float64
	}{
		{PhaseExpansionConfirmed, 1.0},
		{PhaseBottomRecovery, 0.65},
		{PhaseOverheat, 0.40},
		{PhaseContraction, 0.15},
	}

	for _, tt := range tests {
		got := GetPhaseScore(tt.phase)
		if got != tt.wantScore {
			t.Errorf("GetPhaseScore(%d) = %.2f, want %.2f",
				tt.phase, got, tt.wantScore)
		}
	}

	// Verify score ordering: expansion > recovery > overheat > contraction
	scores := []float64{
		GetPhaseScore(PhaseExpansionConfirmed),
		GetPhaseScore(PhaseBottomRecovery),
		GetPhaseScore(PhaseOverheat),
		GetPhaseScore(PhaseContraction),
	}
	for i := 1; i < len(scores); i++ {
		if scores[i] >= scores[i-1] {
			t.Errorf("score ordering broken at positions %d→%d: %.2f should be > %.2f",
				i, i+1, scores[i-1], scores[i])
		}
	}
}

func TestGetPhaseName(t *testing.T) {
	tests := []struct {
		phase SiliconCyclePhase
		want  string
	}{
		{PhaseBottomRecovery, "谷底復甦"},
		{PhaseExpansionConfirmed, "擴張確認"},
		{PhaseOverheat, "過熱期"},
		{PhaseContraction, "收縮期"},
		{SiliconCyclePhase(99), "未知"},
	}

	for _, tt := range tests {
		got := GetPhaseName(tt.phase)
		if got != tt.want {
			t.Errorf("GetPhaseName(%d) = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestGetPhaseDescription(t *testing.T) {
	// Verify all known phases have non-empty, distinct descriptions.
	phases := []SiliconCyclePhase{
		PhaseBottomRecovery,
		PhaseExpansionConfirmed,
		PhaseOverheat,
		PhaseContraction,
	}

	seen := make(map[string]bool)
	for _, p := range phases {
		desc := GetPhaseDescription(p)
		if desc == "" {
			t.Errorf("GetPhaseDescription(%d) returned empty string", p)
		}
		if seen[desc] {
			t.Errorf("GetPhaseDescription(%d) returned duplicate description: %q", p, desc)
		}
		seen[desc] = true
	}

	// Unknown phase returns non-empty description.
	if desc := GetPhaseDescription(SiliconCyclePhase(99)); desc == "" {
		t.Error("GetPhaseDescription(unknown) returned empty string")
	}
}

func TestGetTypicalDuration(t *testing.T) {
	tests := []struct {
		phase SiliconCyclePhase
		want  int
	}{
		{PhaseBottomRecovery, 90},
		{PhaseExpansionConfirmed, 360},
		{PhaseOverheat, 120},
		{PhaseContraction, 180},
	}

	for _, tt := range tests {
		got := GetTypicalDuration(tt.phase)
		if got != tt.want {
			t.Errorf("GetTypicalDuration(%d) = %d, want %d",
				tt.phase, got, tt.want)
		}
	}

	// Unknown phase returns 0
	if got := GetTypicalDuration(SiliconCyclePhase(99)); got != 0 {
		t.Errorf("GetTypicalDuration(unknown) = %d, want 0", got)
	}
}

func TestIsFavorable(t *testing.T) {
	now := time.Now()

	// Phase 0 (recovery) is favorable.
	e := NewSiliconCycleTracker()
	if !e.IsFavorable() {
		t.Error("PhaseBottomRecovery should be favorable")
	}

	// Transition to expansion (should be favorable).
	e.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
	})
	if !e.IsFavorable() {
		t.Error("PhaseExpansionConfirmed should be favorable")
	}

	// Transition to overheat (should NOT be favorable).
	now = now.Add(30 * 24 * time.Hour)
	e.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     0.30,
	})
	if e.IsFavorable() {
		t.Error("PhaseOverheat should NOT be favorable")
	}

	// Transition to contraction (should NOT be favorable).
	now = now.Add(30 * 24 * time.Hour)
	e.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.25,
		GlobalSemiconductorBillingsYoY: 0.15,
		DRAMSpotPriceTrend:             -0.05,
		TaiwanSemiconductorIndexMA:     0.30,
		TSMCCapexGuidance:              -0.15,
	})
	if e.IsFavorable() {
		t.Error("PhaseContraction should NOT be favorable")
	}
}

func TestHistoryRetention(t *testing.T) {
	now := time.Now()
	e := NewSiliconCycleTracker()

	// Trigger many transitions to test history window retention.
	params := defaultSiliconCycleParams()

	for i := 0; i < params.HistoryWindowSize+20; i++ {
		now = now.Add(24 * time.Hour)
		e.DetectPhase(now, SiliconIndicators{
			TSMCMonthlyRevenueYoY:          0.30,
			GlobalSemiconductorBillingsYoY: 0.20,
			DRAMSpotPriceTrend:             0.05,
			TaiwanSemiconductorIndexMA:     0.30,
		})
	}

	if count := e.GetTransitionCount(); count > params.HistoryWindowSize {
		t.Errorf("history size %d exceeds window size %d", count, params.HistoryWindowSize)
	}
}

func TestDaysInCurrentPhase(t *testing.T) {
	baseTime := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	e := NewSiliconCycleTracker()

	// No transitions yet — should be 0.
	if days := e.DaysInCurrentPhase(baseTime); days != 0 {
		t.Errorf("DaysInCurrentPhase with no history: got %d, want 0", days)
	}

	// Trigger a transition.
	e.DetectPhase(baseTime, SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
	})

	// 10 days later.
	later := baseTime.Add(10 * 24 * time.Hour)
	days := e.DaysInCurrentPhase(later)
	if days < 9 || days > 11 {
		t.Errorf("DaysInCurrentPhase after 10 days: got %d, want ~10", days)
	}
}

func TestDefaultSiliconCycleParams(t *testing.T) {
	params := defaultSiliconCycleParams()

	if params.RevenueYoYThreshold <= 0 {
		t.Error("RevenueYoYThreshold should be positive")
	}
	if params.BillingsYoYThreshold <= 0 {
		t.Error("BillingsYoYThreshold should be positive")
	}
	if params.IndexMAPercentThreshold <= 0 {
		t.Error("IndexMAPercentThreshold should be positive")
	}
	if params.SOXExtremeThreshold <= 0 {
		t.Error("SOXExtremeThreshold should be positive")
	}
	if params.CapexCutThreshold <= 0 {
		t.Error("CapexCutThreshold should be positive")
	}
	if params.MinConfidence <= 0 || params.MinConfidence > 1 {
		t.Errorf("MinConfidence = %.2f, want (0, 1]", params.MinConfidence)
	}
	if params.HistoryWindowSize <= 0 {
		t.Error("HistoryWindowSize should be positive")
	}
}

func TestString(t *testing.T) {
	e := NewSiliconCycleTracker()
	s := e.String()
	if s == "" {
		t.Error("String() returned empty string")
	}
}

func TestReset(t *testing.T) {
	now := time.Now()
	e := NewSiliconCycleTracker()

	// Trigger a transition.
	e.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
	})
	if e.GetTransitionCount() != 1 {
		t.Fatalf("expected 1 transition after detect, got %d", e.GetTransitionCount())
	}

	// Reset.
	e.Reset()
	if e.GetCurrentPhase() != PhaseBottomRecovery {
		t.Errorf("after Reset: expected PhaseBottomRecovery, got %s", GetPhaseName(e.GetCurrentPhase()))
	}
	if e.GetTransitionCount() != 0 {
		t.Errorf("after Reset: expected 0 transitions, got %d", e.GetTransitionCount())
	}
}

func TestNoTransitionWhenSamePhase(t *testing.T) {
	now := time.Now()
	e := NewSiliconCycleTracker()

	// Apply indicators that should keep us in PhaseBottomRecovery.
	indicators := SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.05,
		GlobalSemiconductorBillingsYoY: 0.02,
		DRAMSpotPriceTrend:             -0.10,
	}
	phase := e.DetectPhase(now, indicators)
	if phase != PhaseBottomRecovery {
		t.Fatalf("expected PhaseBottomRecovery, got %s", GetPhaseName(phase))
	}
	if e.GetTransitionCount() != 0 {
		t.Errorf("expected 0 transitions, got %d", e.GetTransitionCount())
	}
}

func TestSiliconGetHistory(t *testing.T) {
	now := time.Now()
	e := NewSiliconCycleTracker()

	// Trigger a transition.
	e.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.30,
		GlobalSemiconductorBillingsYoY: 0.20,
		DRAMSpotPriceTrend:             0.05,
	})

	history := e.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry := history[0]
	if entry.FromPhase != PhaseBottomRecovery {
		t.Errorf("expected FromPhase=0, got %d", entry.FromPhase)
	}
	if entry.ToPhase != PhaseExpansionConfirmed {
		t.Errorf("expected ToPhase=1, got %d", entry.ToPhase)
	}

	// Verify the returned slice is a copy — modifying it should not affect engine.
	history[0] = PhaseTransition{FromPhase: PhaseContraction}
	history2 := e.GetHistory()
	if history2[0].FromPhase != PhaseBottomRecovery {
		t.Error("GetHistory should return a copy, not a reference to internal state")
	}
}
