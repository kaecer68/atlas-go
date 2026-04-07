package reflexivity

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestReflexivityEngine(t *testing.T) {
	t.Run("NewReflexivityEngine", func(t *testing.T) {
		engine := NewReflexivityEngine()
		if engine == nil {
			t.Fatal("Expected non-nil engine")
		}
	})

	t.Run("RegisterBias", func(t *testing.T) {
		engine := NewReflexivityEngine()

		bias := &MarketBias{
			ID:         "bias_001",
			Type:       TrendFollowing,
			Target:     "2330.TW",
			Magnitude:  0.75,
			Confidence: 0.8,
			Source:     []string{"test-agent-01"},
			Timestamp:  time.Now(),
		}

		err := engine.RegisterBias(bias)
		if err != nil {
			t.Errorf("Failed to register bias: %v", err)
		}

		// Verify bias was registered
		if len(engine.biases) == 0 {
			t.Error("Expected bias to be registered")
		}
	})

	t.Run("UpdateReality", func(t *testing.T) {
		engine := NewReflexivityEngine()

		reality := &MarketReality{
			ID:         "reality_001",
			Target:     "2330.TW",
			Price:      850.0,
			Trend:      0.02,
			Volatility: 0.18,
		}

		engine.UpdateReality(reality)

		if len(engine.realities) == 0 {
			t.Error("Expected reality to be registered")
		}

		if engine.realities["2330.TW"] == nil {
			t.Error("Reality should be retrievable by target")
		}
	})

	t.Run("FeedbackLoopDetection", func(t *testing.T) {
		engine := NewReflexivityEngine()

		// Register a strong bias
		bias := &MarketBias{
			ID:         "bias_002",
			Type:       TrendFollowing,
			Target:     "TEST",
			Magnitude:  0.9,
			Confidence: 0.85,
		}
		engine.RegisterBias(bias)

		// Register matching reality with confirming trend (creates positive feedback)
		reality := &MarketReality{
			ID:         "real_002",
			Target:     "TEST",
			Price:      100.0,
			Trend:      0.5, // Strong positive trend aligned with bias
			Volatility: 0.15,
		}
		engine.UpdateReality(reality)

		// GetActiveLoops should find emerging loops
		loops := engine.GetActiveLoops()
		t.Logf("Detected %d active feedback loops", len(loops))

		// Also check by target
		targetLoops := engine.GetLoopsByTarget("TEST")
		t.Logf("Loops for TEST: %d", len(targetLoops))

		if len(targetLoops) > 0 {
			for _, loop := range targetLoops {
				if loop.Direction == PositiveFeedback {
					t.Log("Found positive feedback loop as expected")
				}
				if loop.Strength <= 0 {
					t.Error("Loop strength should be positive")
				}
			}
		}
	})

	t.Run("BiasTypes", func(t *testing.T) {
		biasTypes := []BiasType{
			TrendFollowing,
			Contrarian,
			Anchoring,
			Recency,
			Confirmation,
			Herding,
			Overconfidence,
			FearGreed,
		}

		// Verify all bias types are distinct
		seen := make(map[BiasType]bool)
		for _, bt := range biasTypes {
			if seen[bt] {
				t.Errorf("Duplicate bias type: %d", bt)
			}
			seen[bt] = true
		}
	})

	t.Run("ApplyReflexivityAdjustment", func(t *testing.T) {
		engine := NewReflexivityEngine()

		// Set up biased environment
		bias := &MarketBias{
			ID:         "bias_003",
			Type:       TrendFollowing,
			Target:     "2330.TW",
			Magnitude:  0.8,
			Confidence: 0.75,
		}
		engine.RegisterBias(bias)

		reality := &MarketReality{
			ID:         "real_003",
			Target:     "2330.TW",
			Price:      850.0,
			Trend:      0.5, // Strong enough to create feedback loop
			Volatility: 0.16,
		}
		engine.UpdateReality(reality)

		// Create recommendations
		recs := []domain.Recommendation{
			{
				Agent:      "agent_001",
				Symbol:     "2330.TW",
				Side:       domain.SideBuy,
				Conviction: 80,
			},
		}

		// Process through reflexivity
		adjusted := engine.ApplyReflexivityAdjustment(recs)

		if len(adjusted) != len(recs) {
			t.Errorf("Expected %d recommendations, got %d", len(recs), len(adjusted))
		}

		t.Logf("Original conviction: 80, Adjusted conviction: %d", adjusted[0].Conviction)
	})

	t.Run("ProcessRecommendations", func(t *testing.T) {
		engine := NewReflexivityEngine()

		recs := []domain.Recommendation{
			{Agent: "agent_001", Symbol: "TEST", Side: domain.SideBuy, Conviction: 70},
			{Agent: "agent_002", Symbol: "TEST", Side: domain.SideSell, Conviction: 60},
		}

		// Should not panic
		engine.ProcessRecommendations(recs)

		// After processing, biases should be registered
		if len(engine.biases) == 0 {
			t.Error("Expected biases to be registered after processing recommendations")
		}
	})

	t.Run("PredictLoopOutcome", func(t *testing.T) {
		engine := NewReflexivityEngine()

		// Create a feedback loop
		bias := &MarketBias{
			ID:         "bubble_bias",
			Type:       Herding,
			Target:     "BUBBLE",
			Magnitude:  0.95,
			Confidence: 0.9,
		}
		engine.RegisterBias(bias)

		reality := &MarketReality{
			ID:         "bubble_reality",
			Target:     "BUBBLE",
			Price:      100.0,
			Trend:      0.5,
			Volatility: 0.25,
		}
		engine.UpdateReality(reality)

		loops := engine.GetLoopsByTarget("BUBBLE")
		if len(loops) > 0 {
			prediction, confidence := engine.PredictLoopOutcome(loops[0].ID)
			t.Logf("Loop prediction: %s (confidence: %.2f)", prediction, confidence)
		}
	})

	t.Run("GetReflexivityReport", func(t *testing.T) {
		engine := NewReflexivityEngine()

		bias := &MarketBias{
			ID:         "r_bias",
			Type:       TrendFollowing,
			Target:     "RPT",
			Magnitude:  0.8,
			Confidence: 0.7,
			Source:     []string{"test-agent-02"},
			Timestamp:  time.Now(),
		}
		engine.RegisterBias(bias)

		reality := &MarketReality{
			ID:         "r_real",
			Target:     "RPT",
			Price:      100,
			Trend:      0.5,
			Volatility: 0.2,
			Timestamp:  time.Now(),
		}
		engine.UpdateReality(reality)

		report := engine.GetReflexivityReport()
		if report == nil {
			t.Fatal("Expected non-nil report")
		}
		if report.BiasCount == 0 {
			t.Error("Expected non-zero bias count in report")
		}
		t.Logf("Report: %d total loops, %d active", report.TotalLoops, report.ActiveLoops)
	})
}

func TestFeedbackLoop(t *testing.T) {
	t.Run("LoopDirectionTypes", func(t *testing.T) {
		directions := []LoopDirection{
			PositiveFeedback,
			NegativeFeedback,
		}

		seen := make(map[LoopDirection]bool)
		for _, d := range directions {
			if seen[d] {
				t.Errorf("Duplicate direction: %d", d)
			}
			seen[d] = true
		}
	})

	t.Run("LoopStatusTypes", func(t *testing.T) {
		statuses := []LoopStatus{
			LoopEmerging,
			LoopActive,
			LoopMaturing,
			LoopExhausting,
			LoopCompleted,
		}

		seen := make(map[LoopStatus]bool)
		for _, s := range statuses {
			if seen[s] {
				t.Errorf("Duplicate status: %d", s)
			}
			seen[s] = true
		}
	})

	t.Run("UpdateLoopStatus", func(t *testing.T) {
		engine := NewReflexivityEngine()

		// Create a loop
		bias := &MarketBias{
			ID: "st_bias", Type: TrendFollowing, Target: "ST",
			Magnitude: 0.9, Confidence: 0.8,
		}
		engine.RegisterBias(bias)

		reality := &MarketReality{
			ID: "st_real", Target: "ST", Price: 100, Trend: 0.5,
		}
		engine.UpdateReality(reality)

		loops := engine.GetLoopsByTarget("ST")
		if len(loops) > 0 {
			engine.UpdateLoopStatus(loops[0].ID, LoopActive)

			updated := engine.GetActiveLoops()
			found := false
			for _, l := range updated {
				if l.ID == loops[0].ID && l.Status == LoopActive {
					found = true
				}
			}
			if !found {
				t.Error("Expected loop to be updated to Active status")
			}
		}
	})
}
