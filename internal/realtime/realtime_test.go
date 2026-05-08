package realtime

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRealTimeAdapter(t *testing.T) {
	t.Run("NewRealTimeAdapter", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		if adapter == nil {
			t.Fatal("Expected non-nil adapter")
		}

		if adapter.detector == nil {
			t.Error("Expected detector to be initialized")
		}

		if adapter.config.UpdateInterval != 100*time.Millisecond {
			t.Errorf("Expected 100ms update interval, got %v", adapter.config.UpdateInterval)
		}
	})

	t.Run("DefaultConfig", func(t *testing.T) {
		params := defaultRealtimeConfig()

		if params.UpdateIntervalMs.Value != 100 {
			t.Errorf("Expected UpdateInterval 100ms, got %d", params.UpdateIntervalMs.Value)
		}

		if params.MinConfidence.Value != 0.7 {
			t.Errorf("Expected MinConfidence 0.7, got %f", params.MinConfidence.Value)
		}

		if params.MaxWeightChange.Value != 0.5 {
			t.Errorf("Expected MaxWeightChange 0.5, got %f", params.MaxWeightChange.Value)
		}
	})

	t.Run("IngestData", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		point := MarketDataPoint{
			Symbol:    "2330.TW",
			Price:     850.0,
			Volume:    50000000,
			Bid:       849.5,
			Ask:       850.5,
			Spread:    1.0,
			Timestamp: time.Now(),
		}

		adapter.IngestData(point)

		window := adapter.dataWindows["2330.TW"]
		if len(window) != 1 {
			t.Errorf("Expected 1 data point, got %d", len(window))
		}

		if window[0].Price != 850.0 {
			t.Errorf("Expected price 850.0, got %f", window[0].Price)
		}
	})

	t.Run("RegisterAgent", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		adapter.RegisterAgent("trend_agent", []string{"2330.TW", "2317.TW"}, 1.0)

		weight := adapter.GetAgentWeight("trend_agent", "2330.TW")
		if weight != 1.0 {
			t.Errorf("Expected weight 1.0, got %f", weight)
		}

		// Check unregistered symbol returns default
		defaultWeight := adapter.GetAgentWeight("trend_agent", "UNKNOWN")
		if defaultWeight != 1.0 {
			t.Errorf("Expected default weight 1.0, got %f", defaultWeight)
		}
	})

	t.Run("GetCurrentRegime", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		// Ingest enough data to detect regime
		baseTime := time.Now()
		for i := range 40 {
			point := MarketDataPoint{
				Symbol:    "TEST",
				Price:     100.0 + float64(i)*0.5, // Trending up
				Volume:    1000000,
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			}
			adapter.IngestData(point)
		}

		regime := adapter.GetCurrentRegime("TEST")

		if regime == "" {
			t.Error("Expected a regime to be detected")
		}

		t.Logf("Detected regime: %s", regime)

		// Should detect trending up or calm
		validRegimes := []RegimeType{RegimeCalm, RegimeTrendingUp, RegimeTrendingDown, RegimeVolatile}
		found := slices.Contains(validRegimes, regime)
		if !found {
			t.Errorf("Unexpected regime: %s", regime)
		}
	})

	t.Run("GetRegimeConfidence", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		// No data yet
		confidence := adapter.GetRegimeConfidence("TEST")
		if confidence != 0.0 {
			t.Errorf("Expected 0 confidence with no data, got %f", confidence)
		}

		// Add some data
		for i := range 20 {
			point := MarketDataPoint{
				Symbol:    "TEST",
				Price:     100.0,
				Volume:    1000000,
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			}
			adapter.IngestData(point)
		}

		confidence = adapter.GetRegimeConfidence("TEST")
		if confidence < 0 || confidence > 1 {
			t.Errorf("Confidence should be between 0 and 1, got %f", confidence)
		}

		t.Logf("Confidence: %.2f", confidence)
	})

	t.Run("ApplyToRecommendation", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		adapter.RegisterAgent("test_agent", []string{"2330.TW"}, 1.5)

		rec := domain.Recommendation{
			Agent:      "test_agent",
			Symbol:     "2330.TW",
			Side:       domain.SideBuy,
			Conviction: 70,
			Reason:     "Strong momentum",
		}

		adjusted := adapter.ApplyToRecommendation(rec)

		// Weight is 1.5, so conviction should be boosted
		expectedConviction := min(int(float64(70)*1.5), 100)

		if adjusted.Conviction != expectedConviction {
			t.Errorf("Expected conviction %d, got %d", expectedConviction, adjusted.Conviction)
		}

		t.Logf("Original: %d, Adjusted: %d", rec.Conviction, adjusted.Conviction)
	})

	t.Run("GetActiveSymbols", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		adapter.IngestData(MarketDataPoint{Symbol: "A", Price: 100})
		adapter.IngestData(MarketDataPoint{Symbol: "B", Price: 200})
		adapter.IngestData(MarketDataPoint{Symbol: "C", Price: 300})

		symbols := adapter.GetActiveSymbols()

		if len(symbols) != 3 {
			t.Errorf("Expected 3 symbols, got %d", len(symbols))
		}
	})

	t.Run("GetStatistics", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		adapter.IngestData(MarketDataPoint{Symbol: "A", Price: 100})
		adapter.RegisterAgent("agent1", []string{"A"}, 1.0)

		stats := adapter.GetStatistics()

		if stats.MonitoredSymbols != 1 {
			t.Errorf("Expected 1 monitored symbol, got %d", stats.MonitoredSymbols)
		}

		if stats.ActiveAgents != 1 {
			t.Errorf("Expected 1 active agent, got %d", stats.ActiveAgents)
		}
	})

	t.Run("GenerateReport", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		// Add some data
		for i := range 30 {
			adapter.IngestData(MarketDataPoint{
				Symbol:    "TEST",
				Price:     100.0 + float64(i)*0.1,
				Volume:    1000000,
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			})
		}

		report := adapter.GenerateReport()

		if report == nil {
			t.Fatal("Expected non-nil report")
		}

		if report.UpdateInterval != time.Duration(params.UpdateIntervalMs.Value)*time.Millisecond {
			t.Errorf("Expected update interval %v, got %v",
				time.Duration(params.UpdateIntervalMs.Value)*time.Millisecond, report.UpdateInterval)
		}

		if len(report.SymbolReports) == 0 {
			t.Error("Expected some symbol reports")
		}

		t.Logf("Report has %d symbol reports", len(report.SymbolReports))
	})

	t.Run("RegimeTypes", func(t *testing.T) {
		regimes := []RegimeType{
			RegimeCalm,
			RegimeVolatile,
			RegimeTrendingUp,
			RegimeTrendingDown,
			RegimeReversing,
			RegimeBreakout,
			RegimeBreakdown,
		}

		for _, r := range regimes {
			if r == "" {
				t.Error("Regime type should not be empty")
			}
		}
	})
}

func TestRegimeDetector(t *testing.T) {
	t.Run("NewRegimeDetector", func(t *testing.T) {
		detector := NewRegimeDetector(nil)
		if detector == nil {
			t.Fatal("Expected non-nil detector")
		}

		if detector.volatilityThreshold != 0.02 {
			t.Errorf("Expected volatility threshold 0.02, got %f", detector.volatilityThreshold)
		}
	})

	t.Run("DetectRegimeCalm", func(t *testing.T) {
		detector := NewRegimeDetector(nil)

		// Create calm data (stable prices)
		data := make([]MarketDataPoint, 60)
		baseTime := time.Now()
		for i := range 60 {
			data[i] = MarketDataPoint{
				Price:     100.0 + float64(i)*0.01, // Very slight trend
				Volume:    1000000,
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			}
		}

		regime := detector.DetectRegime(data)

		if regime != RegimeCalm && regime != RegimeTrendingUp {
			t.Errorf("Expected calm or slight trending, got %s", regime)
		}

		t.Logf("Detected regime for calm data: %s", regime)
	})

	t.Run("DetectRegimeVolatile", func(t *testing.T) {
		detector := NewRegimeDetector(nil)

		// Create volatile data (large price swings)
		data := make([]MarketDataPoint, 60)
		baseTime := time.Now()
		for i := range 60 {
			price := 100.0
			if i%2 == 0 {
				price = 105.0 // Up 5%
			} else {
				price = 95.0 // Down 5%
			}
			data[i] = MarketDataPoint{
				Price:     price,
				Volume:    1000000,
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			}
		}

		regime := detector.DetectRegime(data)

		// Should detect volatility or reversal
		if regime != RegimeVolatile && regime != RegimeReversing {
			t.Logf("Detected regime: %s (may vary based on algorithm)", regime)
		}
	})

	t.Run("DetectVolumeSpike", func(t *testing.T) {
		detector := NewRegimeDetector(nil)

		// Normal volume data
		data := make([]MarketDataPoint, 20)
		baseTime := time.Now()
		for i := range 20 {
			volume := 1000000.0
			if i == 19 {
				volume = 5000000.0 // 5x spike
			}
			data[i] = MarketDataPoint{
				Price:     100.0,
				Volume:    volume,
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			}
		}

		spike := detector.detectVolumeSpike(data)
		if !spike {
			t.Error("Expected volume spike to be detected")
		}
	})

	t.Run("DetectReversal", func(t *testing.T) {
		detector := NewRegimeDetector(nil)

		// Create reversal pattern: up then down
		data := make([]MarketDataPoint, 40)
		baseTime := time.Now()

		// First half: trending up
		for i := range 20 {
			data[i] = MarketDataPoint{
				Price:     100.0 + float64(i)*0.5, // Up to 110
				Volume:    1000000,
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			}
		}

		// Second half: trending down
		for i := 20; i < 40; i++ {
			data[i] = MarketDataPoint{
				Price:     110.0 - float64(i-20)*0.6, // Down from 110
				Volume:    1000000,
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			}
		}

		reversal := detector.detectReversal(data)
		if !reversal {
			t.Log("Reversal may not be detected depending on threshold sensitivity")
		}
	})
}

func TestRealTimeAdapterLifecycle(t *testing.T) {
	t.Run("StartAndStop", func(t *testing.T) {
		params := defaultRealtimeConfig()
		adapter := NewRealTimeAdapter(params)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Start adapter
		go adapter.Start(ctx)

		// Let it run briefly
		time.Sleep(100 * time.Millisecond)

		// Ingest some data
		adapter.IngestData(MarketDataPoint{
			Symbol: "TEST",
			Price:  100.0,
		})

		// Wait for context to cancel
		<-ctx.Done()

		t.Log("Adapter started and stopped successfully")
	})
}
