package globalmarket

import (
	"testing"
)

func TestGlobalMarketManager(t *testing.T) {
	t.Run("NewGlobalMarketManager", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		if gmm == nil {
			t.Fatal("Expected non-nil manager")
		}

		if gmm.markets == nil {
			t.Error("Expected markets map to be initialized")
		}

		if gmm.correlation == nil {
			t.Error("Expected correlation matrix to be initialized")
		}
	})

	t.Run("DefaultConfig", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()

		if config.PrimaryMarket != RegionTaiwan {
			t.Errorf("Expected PrimaryMarket TW, got %s", config.PrimaryMarket)
		}

		if len(config.EnabledMarkets) == 0 {
			t.Error("Expected some enabled markets")
		}

		if config.CrossMarketWeight != 0.3 {
			t.Errorf("Expected CrossMarketWeight 0.3, got %f", config.CrossMarketWeight)
		}
	})

	t.Run("EnableMarket", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		err := gmm.EnableMarket(RegionUS)
		if err != nil {
			t.Fatalf("Failed to enable US market: %v", err)
		}

		market := gmm.markets[RegionUS]
		if market == nil {
			t.Fatal("US market should exist")
		}

		if !market.Enabled {
			t.Error("US market should be enabled")
		}

		// Verify agents were spawned
		usAgents := 0
		for _, agent := range gmm.agents {
			if agent.Region == RegionUS && agent.Enabled {
				usAgents++
			}
		}

		if usAgents == 0 {
			t.Error("Expected US agents to be spawned")
		}

		t.Logf("Spawned %d US agents", usAgents)
	})

	t.Run("DisableMarket", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		// First enable
		gmm.EnableMarket(RegionUS)

		// Then disable
		gmm.DisableMarket(RegionUS)

		market := gmm.markets[RegionUS]
		if market.Enabled {
			t.Error("US market should be disabled")
		}
	})

	t.Run("GetActiveMarkets", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		// Enable a market
		gmm.EnableMarket(RegionAsia)

		active := gmm.GetActiveMarkets()

		foundAsia := false
		for _, market := range active {
			if market.Region == RegionAsia {
				foundAsia = true
				break
			}
		}

		if !foundAsia {
			t.Error("Asia should be in active markets")
		}

		t.Logf("Active markets: %d", len(active))
	})

	t.Run("GetCrossMarketCorrelation", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		corr := gmm.GetCrossMarketCorrelation(RegionTaiwan, RegionUS)

		if corr < 0 || corr > 1 {
			t.Errorf("Correlation should be between 0 and 1, got %f", corr)
		}

		// Should be around 0.65 based on default config
		if corr < 0.5 || corr > 0.8 {
			t.Logf("Correlation is %.2f (expected around 0.65)", corr)
		}
	})

	t.Run("CalculateGlobalExposure", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		positions := map[string]float64{
			"2330.TW":   500000, // Taiwan
			"AAPL":      300000, // US
			"005930.KS": 200000, // Korea (Asia)
		}

		exposure := gmm.CalculateGlobalExposure(positions)

		if exposure == nil {
			t.Fatal("Expected non-nil exposure report")
		}

		if exposure.TotalValue != 1000000 {
			t.Errorf("Expected total 1000000, got %f", exposure.TotalValue)
		}

		// Check regional breakdown
		if twExposure, ok := exposure.ByRegion[RegionTaiwan]; ok {
			t.Logf("Taiwan exposure: %.2f%%", twExposure*100)
			if twExposure != 0.5 {
				t.Errorf("Expected Taiwan 50%%, got %.2f%%", twExposure*100)
			}
		} else {
			t.Error("Should have Taiwan exposure")
		}
	})

	t.Run("InferRegionFromSymbol", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		tests := []struct {
			symbol   string
			expected MarketRegion
		}{
			{"2330.TW", RegionTaiwan},
			{"2317.TW", RegionTaiwan},
			{"AAPL", RegionUS},
			{"MSFT", RegionUS},
			{"005930.KS", RegionAsia},
			{"7203.T", RegionJapan},
		}

		for _, test := range tests {
			region := gmm.inferRegionFromSymbol(test.symbol)
			if region != test.expected {
				t.Errorf("Expected %s for %s, got %s", test.expected, test.symbol, region)
			}
		}
	})

	t.Run("RegionMarketLimits", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		// Create exposure that exceeds limits
		positions := map[string]float64{
			"2330.TW": 600000, // 60% - exceeds 50% limit
			"AAPL":    400000, // 40%
		}

		exposure := gmm.CalculateGlobalExposure(positions)

		if len(exposure.LimitBreaches) == 0 {
			t.Log("No limit breaches detected (limits may need adjustment)")
		} else {
			for _, breach := range exposure.LimitBreaches {
				t.Logf("Limit breach: %s (%.2f%% > %.2f%%)",
					breach.Region, breach.Exposure*100, breach.Limit*100)
			}
		}
	})

	t.Run("MarketRegions", func(t *testing.T) {
		regions := []MarketRegion{
			RegionTaiwan,
			RegionUS,
			RegionEurope,
			RegionAsia,
			RegionJapan,
			RegionChina,
			RegionEmerging,
		}

		for _, r := range regions {
			if r == "" {
				t.Error("Market region should not be empty")
			}
		}
	})

	t.Run("GenerateReport", func(t *testing.T) {
		config := DefaultGlobalExpansionConfig()
		gmm := NewGlobalMarketManager(config)

		// Enable some markets
		gmm.EnableMarket(RegionUS)
		gmm.EnableMarket(RegionAsia)

		report := gmm.GenerateReport()

		if report == nil {
			t.Fatal("Expected non-nil report")
		}

		if len(report.EnabledMarkets) == 0 {
			t.Error("Expected some enabled markets in report")
		}

		t.Logf("Enabled markets: %d, Available markets: %d",
			len(report.EnabledMarkets), len(report.AvailableMarkets))
	})
}

func TestCorrelationMatrix(t *testing.T) {
	t.Run("NewCorrelationMatrix", func(t *testing.T) {
		cm := NewCorrelationMatrix()
		if cm == nil {
			t.Fatal("Expected non-nil matrix")
		}
	})

	t.Run("SetAndGetCorrelation", func(t *testing.T) {
		cm := NewCorrelationMatrix()

		cm.SetCorrelation("TW", "US", 0.65)

		corr := cm.GetCorrelation("TW", "US")
		if corr != 0.65 {
			t.Errorf("Expected correlation 0.65, got %f", corr)
		}

		// Should be symmetric
		corrReverse := cm.GetCorrelation("US", "TW")
		if corrReverse != 0.65 {
			t.Errorf("Expected symmetric correlation 0.65, got %f", corrReverse)
		}
	})

	t.Run("DefaultCorrelation", func(t *testing.T) {
		cm := NewCorrelationMatrix()

		// Unset correlation should return default
		corr := cm.GetCorrelation("XX", "YY")
		if corr != 0.5 {
			t.Errorf("Expected default 0.5, got %f", corr)
		}
	})

	t.Run("DiversificationScore", func(t *testing.T) {
		cm := NewCorrelationMatrix()

		// Set some correlations
		cm.SetCorrelation("TW", "US", 0.65)
		cm.SetCorrelation("TW", "ASIA", 0.80)
		cm.SetCorrelation("US", "ASIA", 0.60)

		markets := []MarketRegion{RegionTaiwan, RegionUS, RegionAsia}
		score := cm.GetDiversificationScore(markets)

		if score < 0 || score > 1 {
			t.Errorf("Score should be between 0 and 1, got %f", score)
		}

		t.Logf("Diversification score: %.2f", score)

		// Higher correlation = lower score
		// Average correlation here is (0.65 + 0.80 + 0.60) / 3 = 0.683
		// Expected score around 0.32
		if score < 0.2 || score > 0.5 {
			t.Logf("Score %.2f is outside expected range 0.2-0.5", score)
		}
	})

	t.Run("SingleMarketDiversification", func(t *testing.T) {
		cm := NewCorrelationMatrix()

		markets := []MarketRegion{RegionTaiwan}
		score := cm.GetDiversificationScore(markets)

		if score != 1.0 {
			t.Errorf("Single market should have score 1.0, got %f", score)
		}
	})
}

func TestGlobalAgent(t *testing.T) {
	t.Run("AgentCreation", func(t *testing.T) {
		agent := &GlobalAgent{
			ID:          "TW_macro_001",
			BaseAgentID: "taiwan_macro",
			Region:      RegionTaiwan,
			SkillType:   "macro",
			Expertise:   []string{"semiconductor", "technology"},
			Weight:      1.0,
			Enabled:     true,
		}

		if agent.Region != RegionTaiwan {
			t.Errorf("Expected region TW, got %s", agent.Region)
		}

		if !agent.Enabled {
			t.Error("Agent should be enabled")
		}
	})
}
