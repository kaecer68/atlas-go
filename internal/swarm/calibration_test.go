package swarm

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/stress"
)

func TestEstimateVolatilityFromStress(t *testing.T) {
	t.Run("uses VIX when available", func(t *testing.T) {
		s := stress.Scenario{
			Quotes: []domain.Quote{
				{Symbol: "VIX", Last: 32.0, IsTradable: true},
				{Symbol: "DXY", Last: 105.0, Open: 102.0, IsTradable: true},
			},
		}
		vol := estimateVolatilityFromStress(s)
		if math.Abs(vol-0.32) > 1e-9 {
			t.Errorf("expected vol 0.32 from VIX, got %.4f", vol)
		}
	})

	t.Run("falls back to price range", func(t *testing.T) {
		s := stress.Scenario{
			Quotes: []domain.Quote{
				{Symbol: "DXY", Last: 105.0, Open: 100.0, IsTradable: true},
			},
		}
		vol := estimateVolatilityFromStress(s)
		expected := 0.05 * math.Sqrt(252.0)
		if math.Abs(vol-expected) > 1e-9 {
			t.Errorf("expected vol %.4f from price range, got %.4f", expected, vol)
		}
	})

	t.Run("default when no useful quotes", func(t *testing.T) {
		s := stress.Scenario{Quotes: []domain.Quote{}}
		vol := estimateVolatilityFromStress(s)
		if vol != 0.20 {
			t.Errorf("expected default vol 0.20, got %.4f", vol)
		}
	})
}

func TestMapStressRegimeToSwarm(t *testing.T) {
	cases := []struct {
		input    domain.Regime
		expected string
	}{
		{domain.RegimeRiskOn, "risk_on"},
		{domain.RegimeRiskOff, "risk_off"},
		{domain.RegimeNeutral, "complacent"},
		{domain.Regime("unknown"), "transition"},
	}
	for _, c := range cases {
		got := mapStressRegimeToSwarm(c.input)
		if got != c.expected {
			t.Errorf("mapStressRegimeToSwarm(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestEstimateTrendFromStress(t *testing.T) {
	t.Run("positive gap yields small positive trend", func(t *testing.T) {
		s := stress.Scenario{
			Quotes: []domain.Quote{
				{Symbol: "A", Last: 105.0, Open: 100.0, IsTradable: true},
			},
		}
		trend := estimateTrendFromStress(s)
		if trend <= 0 {
			t.Errorf("expected positive trend, got %.6f", trend)
		}
		if trend > 0.005 {
			t.Errorf("trend %.6f exceeds max cap 0.005", trend)
		}
	})

	t.Run("negative gap yields small negative trend", func(t *testing.T) {
		s := stress.Scenario{
			Quotes: []domain.Quote{
				{Symbol: "A", Last: 90.0, Open: 100.0, IsTradable: true},
			},
		}
		trend := estimateTrendFromStress(s)
		if trend >= 0 {
			t.Errorf("expected negative trend, got %.6f", trend)
		}
		if trend < -0.005 {
			t.Errorf("trend %.6f below min cap -0.005", trend)
		}
	})

	t.Run("no quotes yields zero trend", func(t *testing.T) {
		s := stress.Scenario{Quotes: []domain.Quote{}}
		trend := estimateTrendFromStress(s)
		if trend != 0.0 {
			t.Errorf("expected zero trend, got %.6f", trend)
		}
	})
}

func TestBuildEventsFromStress(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("crash keyword maps to flash_crash", func(t *testing.T) {
		s := stress.Scenario{
			Description: "Global market crash and liquidity freeze",
			Date:        baseTime.Add(48 * time.Hour),
		}
		events := buildEventsFromStress(s, baseTime)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != "flash_crash" {
			t.Errorf("expected type flash_crash, got %s", events[0].Type)
		}
		if math.Abs(events[0].Magnitude-0.05) > 1e-9 {
			t.Errorf("expected magnitude 0.05, got %.4f", events[0].Magnitude)
		}
		if !events[0].Time.Equal(s.Date) {
			t.Errorf("expected event time %v, got %v", s.Date, events[0].Time)
		}
	})

	t.Run("rally keyword maps to rally", func(t *testing.T) {
		s := stress.Scenario{
			Description: "Tech rally and earnings surge",
			Date:        time.Time{},
		}
		events := buildEventsFromStress(s, baseTime)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != "rally" {
			t.Errorf("expected type rally, got %s", events[0].Type)
		}
		if math.Abs(events[0].Magnitude-0.03) > 1e-9 {
			t.Errorf("expected magnitude 0.03, got %.4f", events[0].Magnitude)
		}
		expectedTime := baseTime.Add(24 * time.Hour)
		if !events[0].Time.Equal(expectedTime) {
			t.Errorf("expected event time %v, got %v", expectedTime, events[0].Time)
		}
	})

	t.Run("default maps to earnings_surprise", func(t *testing.T) {
		s := stress.Scenario{Description: "Unexpected macro release"}
		events := buildEventsFromStress(s, baseTime)
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != "earnings_surprise" {
			t.Errorf("expected type earnings_surprise, got %s", events[0].Type)
		}
		if math.Abs(events[0].Magnitude-0.02) > 1e-9 {
			t.Errorf("expected magnitude 0.02, got %.4f", events[0].Magnitude)
		}
	})

	t.Run("empty description returns nil", func(t *testing.T) {
		s := stress.Scenario{Description: ""}
		events := buildEventsFromStress(s, baseTime)
		if events != nil {
			t.Errorf("expected nil events, got %v", events)
		}
	})
}

func TestInitializeScenariosFromStress(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	baseState := MarketState{
		Timestamp: baseTime,
		Prices:    map[string]float64{"TEST": 100.0},
		Volumes:   map[string]float64{"TEST": 1000000},
	}

	stressScenarios := []stress.Scenario{
		{
			ID:          "test_bull",
			Name:        "Test Bull",
			Description: "AI surge rally",
			WindowDays:  20,
			Regime:      domain.RegimeRiskOn,
			Quotes: []domain.Quote{
				{Symbol: "VIX", Last: 14.0, IsTradable: true},
				{Symbol: "TEST", Last: 105.0, Open: 100.0, IsTradable: true},
			},
		},
		{
			ID:          "test_bear",
			Name:        "Test Bear",
			Description: "Market crash",
			WindowDays:  20,
			Regime:      domain.RegimeRiskOff,
			Quotes: []domain.Quote{
				{Symbol: "VIX", Last: 32.0, IsTradable: true},
				{Symbol: "TEST", Last: 90.0, Open: 100.0, IsTradable: true},
			},
		},
	}

	config := DefaultSwarmConfig()
	config.FishCount = 10
	sw := NewMiroFishSwarm(config)
	sw.InitializeScenariosFromStress(baseState, stressScenarios)

	if len(sw.scenarios) != 2 {
		t.Errorf("expected 2 scenarios, got %d", len(sw.scenarios))
	}

	if len(sw.fish) != config.FishCount {
		t.Errorf("expected %d fish, got %d", config.FishCount, len(sw.fish))
	}

	for _, s := range sw.scenarios {
		if s.ID == "" || s.Name == "" {
			t.Errorf("scenario missing id or name: %+v", s)
		}
		if s.Volatility <= 0 {
			t.Errorf("expected positive volatility for %s, got %.4f", s.ID, s.Volatility)
		}
		if s.Duration <= 0 {
			t.Errorf("expected positive duration for %s, got %v", s.ID, s.Duration)
		}
	}

	t.Run("empty scenarios logs warning and creates no fish", func(t *testing.T) {
		sw2 := NewMiroFishSwarm(config)
		sw2.InitializeScenariosFromStress(baseState, nil)
		if len(sw2.scenarios) != 0 {
			t.Errorf("expected 0 scenarios, got %d", len(sw2.scenarios))
		}
		if len(sw2.fish) != 0 {
			t.Errorf("expected 0 fish with no scenarios, got %d", len(sw2.fish))
		}
	})
}

func TestComputeStatsFromFish(t *testing.T) {
	baseTime := time.Now()
	fish := []*MiroFish{
		{
			ID: "f1",
			History: []MarketState{
				{Timestamp: baseTime, Prices: map[string]float64{"A": 100.0}},
				{Timestamp: baseTime.Add(time.Hour), Prices: map[string]float64{"A": 101.0}},
				{Timestamp: baseTime.Add(2 * time.Hour), Prices: map[string]float64{"A": 102.0}},
				{Timestamp: baseTime.Add(3 * time.Hour), Prices: map[string]float64{"A": 101.0}},
			},
		},
		{
			ID: "f2",
			History: []MarketState{
				{Timestamp: baseTime, Prices: map[string]float64{"A": 100.0}},
				{Timestamp: baseTime.Add(time.Hour), Prices: map[string]float64{"A": 99.0}},
				{Timestamp: baseTime.Add(2 * time.Hour), Prices: map[string]float64{"A": 98.0}},
				{Timestamp: baseTime.Add(3 * time.Hour), Prices: map[string]float64{"A": 99.0}},
			},
		},
	}

	stats := computeStatsFromFish(fish)
	if stats.Volatility <= 0 {
		t.Errorf("expected positive volatility, got %.6f", stats.Volatility)
	}
	if stats.MeanReturn == 0 {
		t.Error("expected non-zero mean return")
	}
	if stats.CorrelationMatrix == nil {
		t.Error("expected correlation matrix with one symbol")
	}
	if stats.SharpeRatio == 0 {
		t.Error("expected non-zero Sharpe ratio")
	}

	t.Run("Sharpe computed via shared function", func(t *testing.T) {
		// Verify shared.ComputeSharpe produced a valid (non-NaN, non-zero) Sharpe ratio.
		if math.IsNaN(stats.SharpeRatio) {
			t.Error("expected valid Sharpe ratio from shared.ComputeSharpe, got NaN")
		}
		if stats.SharpeRatio == 0 {
			t.Error("expected non-zero Sharpe ratio from shared.ComputeSharpe")
		}
	})

	t.Run("empty fish returns zero stats", func(t *testing.T) {
		stats := computeStatsFromFish(nil)
		if stats.Volatility != 0 {
			t.Errorf("expected zero volatility, got %.6f", stats.Volatility)
		}
	})
}

func TestRelativeError(t *testing.T) {
	t.Run("normal relative error", func(t *testing.T) {
		err := relativeError(0.20, 0.10)
		if math.Abs(err-0.5) > 1e-9 {
			t.Errorf("expected 0.5, got %.6f", err)
		}
	})

	t.Run("near-zero target clamps to [-1,1]", func(t *testing.T) {
		// Without clamping this would produce a huge number.
		err := relativeError(0.0, 0.5)
		if err > 1.0 || err < -1.0 {
			t.Errorf("expected clamped error in [-1,1], got %.6f", err)
		}
	})

	t.Run("small diff near zero returned raw", func(t *testing.T) {
		err := relativeError(0.0, 0.3)
		if math.Abs(err+0.3) > 1e-9 {
			t.Errorf("expected -0.3, got %.6f", err)
		}
	})
}

func TestCalibrateParameters(t *testing.T) {
	sim := SimulationStatistics{
		Volatility:  0.10,
		MeanReturn:  0.0001,
		Skewness:    0.0,
		Kurtosis:    2.5,
		MaxDrawdown: 0.05,
	}
	target := SimulationStatistics{
		Volatility:  0.20,
		MeanReturn:  0.0003,
		Skewness:    -0.5,
		Kurtosis:    3.5,
		MaxDrawdown: 0.10,
	}

	report := CalibrateParameters(sim, target)

	if report.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if report.CalibrationError <= 0 {
		t.Errorf("expected positive calibration error, got %.6f", report.CalibrationError)
	}

	keys := []string{"garch_omega", "garch_alpha", "garch_beta", "jump_lambda", "jump_mu", "jump_sigma", "trend"}
	for _, k := range keys {
		if _, ok := report.ParameterAdjustments[k]; !ok {
			t.Errorf("missing adjustment key %s", k)
		}
	}

	// Simulated vol is lower than target -> omega should be positive.
	if report.ParameterAdjustments["garch_omega"] <= 0 {
		t.Errorf("expected positive garch_omega adjustment when sim vol < target, got %.6f", report.ParameterAdjustments["garch_omega"])
	}
}

func TestCalibrateAgainstTarget(t *testing.T) {
	baseTime := time.Now()
	config := DefaultSwarmConfig()
	config.FishCount = 10
	config.SimulationHorizon = 4 * time.Hour
	config.TimeStep = time.Hour

	sw := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: baseTime,
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState)
	sw.Start()

	target := SimulationStatistics{
		Volatility:  0.20,
		MeanReturn:  0.0005,
		Skewness:    -0.2,
		Kurtosis:    3.0,
		MaxDrawdown: 0.10,
	}

	report := sw.CalibrateAgainstTarget(target)
	if report.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp in report")
	}
	if report.CalibrationError < 0 {
		t.Errorf("expected non-negative calibration error, got %.6f", report.CalibrationError)
	}
	if len(report.ParameterAdjustments) == 0 {
		t.Error("expected non-empty parameter adjustments")
	}
}
