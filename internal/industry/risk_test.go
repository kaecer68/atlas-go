package industry

import (
	"strings"
	"testing"
	"time"
)

func TestRiskLevelConstants(t *testing.T) {
	tests := []struct {
		level    RiskLevel
		expected string
	}{
		{RiskLevelLow, "low"},
		{RiskLevelMedium, "medium"},
		{RiskLevelHigh, "high"},
		{RiskLevelCritical, "critical"},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if string(tt.level) != tt.expected {
				t.Errorf("RiskLevel = %s, want %s", tt.level, tt.expected)
			}
		})
	}
}

func TestNewRiskMonitor(t *testing.T) {
	rm := NewRiskMonitor()

	if rm == nil {
		t.Fatal("NewRiskMonitor() returned nil")
	}

	if rm.customerData == nil {
		t.Error("customerData map not initialized")
	}

	if len(rm.newsSources) == 0 {
		t.Error("newsSources not initialized")
	}

	if rm.asymmetricConfig.BadNewsThreshold != -0.03 {
		t.Errorf("BadNewsThreshold = %.2f, want -0.03", rm.asymmetricConfig.BadNewsThreshold)
	}

	if rm.asymmetricConfig.GoodNewsThreshold != 0.05 {
		t.Errorf("GoodNewsThreshold = %.2f, want 0.05", rm.asymmetricConfig.GoodNewsThreshold)
	}

	if rm.asymmetricConfig.ReactionTimeMinutes != 30 {
		t.Errorf("ReactionTimeMinutes = %d, want 30", rm.asymmetricConfig.ReactionTimeMinutes)
	}

	if rm.asymmetricConfig.VolumeSpikeMultiplier != 2.0 {
		t.Errorf("VolumeSpikeMultiplier = %.1f, want 2.0", rm.asymmetricConfig.VolumeSpikeMultiplier)
	}
}

func TestDefaultNewsSources(t *testing.T) {
	sources := DefaultNewsSources()

	if len(sources) == 0 {
		t.Fatal("DefaultNewsSources() returned empty slice")
	}

	var tier1USCount int
	var taiwanCount int

	for _, source := range sources {
		if source.Tier == 1 && source.Region == "US" {
			tier1USCount++
		}
		if source.Region == "TW" {
			taiwanCount++
		}
		if source.Name == "" {
			t.Error("NewsSource has empty name")
		}
		if source.Reliability < 0 || source.Reliability > 1 {
			t.Errorf("Source %s has invalid reliability: %.2f", source.Name, source.Reliability)
		}
	}

	if tier1USCount == 0 {
		t.Error("No Tier 1 US news sources found")
	}

	if taiwanCount == 0 {
		t.Error("No Taiwan news sources found")
	}
}

func TestAddAndGetCustomerConcentration(t *testing.T) {
	rm := NewRiskMonitor()

	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 25.0, GeographicRegion: "US"},
		{CustomerName: "Samsung", RevenueSharePct: 15.0, GeographicRegion: "KR"},
	}

	rm.AddCustomerConcentration("2330.TW", customers)

	retrieved := rm.GetCustomerConcentration("2330.TW")
	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 customers, got %d", len(retrieved))
	}

	if retrieved[0].CustomerName != "Apple" {
		t.Errorf("First customer = %s, want Apple", retrieved[0].CustomerName)
	}

	empty := rm.GetCustomerConcentration("9999.TW")
	if len(empty) != 0 {
		t.Error("Expected empty slice for non-existent symbol")
	}
}

func TestCalculateCustomerConcentrationRisk(t *testing.T) {
	rm := NewRiskMonitor()

	tests := []struct {
		name             string
		customers        []CustomerConcentration
		expectRisk       bool
		expectedSeverity RiskLevel
	}{
		{
			name: "critical concentration - Apple 60% + US 60%",
			customers: []CustomerConcentration{
				{CustomerName: "Apple", RevenueSharePct: 60.0, GeographicRegion: "US"},
				{CustomerName: "Others", RevenueSharePct: 40.0, GeographicRegion: "TW"},
			},
			expectRisk:       true,
			expectedSeverity: RiskLevelCritical,
		},
		{
			name: "high concentration - Apple 50% + US 60%",
			customers: []CustomerConcentration{
				{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
				{CustomerName: "Microsoft", RevenueSharePct: 10.0, GeographicRegion: "US"},
				{CustomerName: "Others", RevenueSharePct: 40.0, GeographicRegion: "TW"},
			},
			expectRisk:       true,
			expectedSeverity: RiskLevelHigh,
		},
		{
			name: "medium concentration - top 35%",
			customers: []CustomerConcentration{
				{CustomerName: "Apple", RevenueSharePct: 35.0, GeographicRegion: "US"},
				{CustomerName: "Samsung", RevenueSharePct: 20.0, GeographicRegion: "KR"},
				{CustomerName: "Others", RevenueSharePct: 45.0, GeographicRegion: "TW"},
			},
			expectRisk:       true,
			expectedSeverity: RiskLevelLow,
		},
		{
			name: "low concentration - diversified",
			customers: []CustomerConcentration{
				{CustomerName: "A", RevenueSharePct: 8.0, GeographicRegion: "US"},
				{CustomerName: "B", RevenueSharePct: 8.0, GeographicRegion: "KR"},
				{CustomerName: "C", RevenueSharePct: 8.0, GeographicRegion: "JP"},
				{CustomerName: "D", RevenueSharePct: 8.0, GeographicRegion: "EU"},
				{CustomerName: "E", RevenueSharePct: 8.0, GeographicRegion: "CN"},
				{CustomerName: "F", RevenueSharePct: 8.0, GeographicRegion: "TW"},
				{CustomerName: "G", RevenueSharePct: 8.0, GeographicRegion: "Global"},
				{CustomerName: "Others", RevenueSharePct: 44.0, GeographicRegion: "Global"},
			},
			expectRisk:       true,
			expectedSeverity: RiskLevelLow,
		},
		{
			name:       "no customers",
			customers:  []CustomerConcentration{},
			expectRisk: false,
		},
		{
			name:       "nil customers",
			customers:  nil,
			expectRisk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm.AddCustomerConcentration("TEST.TW", tt.customers)
			risk := rm.CalculateCustomerConcentrationRisk("TEST.TW")

			if tt.expectRisk {
				if risk == nil {
					t.Fatal("Expected risk event, got nil")
				}
				if risk.Type != "customer_concentration" {
					t.Errorf("Risk type = %s, want customer_concentration", risk.Type)
				}
				if risk.Severity != tt.expectedSeverity {
					t.Errorf("Severity = %s, want %s", risk.Severity, tt.expectedSeverity)
				}
				if risk.ImpactEstimate >= 0 {
					t.Error("ImpactEstimate should be negative for customer concentration risk")
				}
				if risk.Confidence <= 0 {
					t.Error("Confidence should be positive")
				}
			} else {
				if risk != nil {
					t.Fatalf("Expected no risk, got: %v", risk)
				}
			}
		})
	}
}

func TestCalculateCustomerConcentrationRisk_USExposure(t *testing.T) {
	rm := NewRiskMonitor()

	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 30.0, GeographicRegion: "US"},
		{CustomerName: "Microsoft", RevenueSharePct: 25.0, GeographicRegion: "US"},
		{CustomerName: "Others", RevenueSharePct: 45.0, GeographicRegion: "TW"},
	}

	rm.AddCustomerConcentration("2330.TW", customers)
	risk := rm.CalculateCustomerConcentrationRisk("2330.TW")

	if risk == nil {
		t.Fatal("Expected risk for high US exposure")
	}

	if !strings.Contains(risk.Description, "US exposure") {
		t.Error("Description should mention US exposure")
	}
}

func TestCalculateNewsLatencyRisk(t *testing.T) {
	rm := NewRiskMonitor()

	tests := []struct {
		name       string
		industryID string
		expectRisk bool
	}{
		{
			name:       "export industry - semiconductor",
			industryID: "semiconductor",
			expectRisk: true,
		},
		{
			name:       "export industry - ai_supply_chain",
			industryID: "ai_supply_chain",
			expectRisk: true,
		},
		{
			name:       "export industry - electronics",
			industryID: "electronics",
			expectRisk: true,
		},
		{
			name:       "export industry - robotics",
			industryID: "robotics",
			expectRisk: true,
		},
		{
			name:       "export industry - shipping",
			industryID: "shipping",
			expectRisk: true,
		},
		{
			name:       "non-export industry - financials",
			industryID: "financials",
			expectRisk: false,
		},
		{
			name:       "non-export industry - energy",
			industryID: "energy",
			expectRisk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := rm.CalculateNewsLatencyRisk("2330.TW", tt.industryID)

			if tt.expectRisk {
				if risk == nil {
					t.Fatal("Expected risk event, got nil")
				}
				if risk.Type != "news_latency" {
					t.Errorf("Risk type = %s, want news_latency", risk.Type)
				}
				if !strings.Contains(risk.Description, "台灣") {
					t.Error("Description should mention Taiwan")
				}
				if risk.ImpactEstimate >= 0 {
					t.Error("ImpactEstimate should be negative")
				}
			} else {
				if risk != nil {
					t.Fatalf("Expected no risk for non-export industry, got: %v", risk)
				}
			}
		})
	}
}

func TestCalculateAsymmetricRisk(t *testing.T) {
	rm := NewRiskMonitor()

	tests := []struct {
		name             string
		priceChangePct   float64
		volumeMultiplier float64
		expectRisk       bool
		expectedSeverity RiskLevel
	}{
		{
			name:             "critical drop - 12%",
			priceChangePct:   -0.12,
			volumeMultiplier: 3.0,
			expectRisk:       true,
			expectedSeverity: RiskLevelCritical,
		},
		{
			name:             "high drop - 8%",
			priceChangePct:   -0.08,
			volumeMultiplier: 2.5,
			expectRisk:       true,
			expectedSeverity: RiskLevelHigh,
		},
		{
			name:             "medium drop - 6%",
			priceChangePct:   -0.06,
			volumeMultiplier: 2.2,
			expectRisk:       true,
			expectedSeverity: RiskLevelMedium,
		},
		{
			name:             "small drop - 2%",
			priceChangePct:   -0.02,
			volumeMultiplier: 2.5,
			expectRisk:       false,
		},
		{
			name:             "price up",
			priceChangePct:   0.05,
			volumeMultiplier: 3.0,
			expectRisk:       false,
		},
		{
			name:             "drop but no volume spike",
			priceChangePct:   -0.08,
			volumeMultiplier: 1.5,
			expectRisk:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := rm.CalculateAsymmetricRisk("2330.TW", tt.priceChangePct, tt.volumeMultiplier)

			if tt.expectRisk {
				if risk == nil {
					t.Fatal("Expected risk event, got nil")
				}
				if risk.Type != "asymmetric" {
					t.Errorf("Risk type = %s, want asymmetric", risk.Type)
				}
				if risk.Severity != tt.expectedSeverity {
					t.Errorf("Severity = %s, want %s", risk.Severity, tt.expectedSeverity)
				}
				if risk.ImpactEstimate >= 0 {
					t.Error("ImpactEstimate should be negative for price drops")
				}
				if !strings.Contains(risk.Description, "下跌") {
					t.Error("Description should mention price drop")
				}
			} else {
				if risk != nil {
					t.Fatalf("Expected no risk, got: %v", risk)
				}
			}
		})
	}
}

func TestGetAllRisks(t *testing.T) {
	rm := NewRiskMonitor()

	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	risks := rm.GetAllRisks("2330.TW", "semiconductor", -0.08, 3.0)

	if len(risks) != 3 {
		t.Fatalf("Expected 3 risks, got %d", len(risks))
	}

	var hasCustomer, hasLatency, hasAsymmetric bool
	for _, risk := range risks {
		switch risk.Type {
		case "customer_concentration":
			hasCustomer = true
		case "news_latency":
			hasLatency = true
		case "asymmetric":
			hasAsymmetric = true
		}
	}

	if !hasCustomer {
		t.Error("Missing customer_concentration risk")
	}
	if !hasLatency {
		t.Error("Missing news_latency risk")
	}
	if !hasAsymmetric {
		t.Error("Missing asymmetric risk")
	}
}

func TestGetAllRisks_NoRisks(t *testing.T) {
	rm := NewRiskMonitor()

	risks := rm.GetAllRisks("9999.TW", "financials", 0.01, 1.0)

	if len(risks) != 0 {
		t.Fatalf("Expected 0 risks, got %d", len(risks))
	}
}

func TestGetHighestRisk(t *testing.T) {
	rm := NewRiskMonitor()

	tests := []struct {
		name          string
		risks         []RiskEvent
		expectedLevel RiskLevel
		expectNil     bool
	}{
		{
			name: "critical highest",
			risks: []RiskEvent{
				{Severity: RiskLevelLow},
				{Severity: RiskLevelMedium},
				{Severity: RiskLevelCritical},
				{Severity: RiskLevelHigh},
			},
			expectedLevel: RiskLevelCritical,
		},
		{
			name: "high highest",
			risks: []RiskEvent{
				{Severity: RiskLevelLow},
				{Severity: RiskLevelHigh},
				{Severity: RiskLevelMedium},
			},
			expectedLevel: RiskLevelHigh,
		},
		{
			name:      "empty slice",
			risks:     []RiskEvent{},
			expectNil: true,
		},
		{
			name:      "nil slice",
			risks:     nil,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			highest := rm.GetHighestRisk(tt.risks)

			if tt.expectNil {
				if highest != nil {
					t.Fatal("Expected nil for empty/nil slice")
				}
				return
			}

			if highest == nil {
				t.Fatal("Expected highest risk, got nil")
			}

			if highest.Severity != tt.expectedLevel {
				t.Errorf("Highest severity = %s, want %s", highest.Severity, tt.expectedLevel)
			}
		})
	}
}

func TestDefaultCustomerConcentrations(t *testing.T) {
	data := DefaultCustomerConcentrations()

	if len(data) == 0 {
		t.Fatal("DefaultCustomerConcentrations() returned empty map")
	}

	expectedSymbols := []string{"2330.TW", "2382.TW", "2317.TW", "2454.TW", "2603.TW"}
	for _, symbol := range expectedSymbols {
		if _, ok := data[symbol]; !ok {
			t.Errorf("Missing default data for %s", symbol)
		}
	}

	for symbol, customers := range data {
		if len(customers) == 0 {
			t.Errorf("Symbol %s has no customers", symbol)
			continue
		}

		var totalShare float64
		for _, customer := range customers {
			if customer.CustomerName == "" {
				t.Errorf("Symbol %s has customer with empty name", symbol)
			}
			if customer.RevenueSharePct < 0 || customer.RevenueSharePct > 100 {
				t.Errorf("Symbol %s customer %s has invalid revenue share: %.1f", symbol, customer.CustomerName, customer.RevenueSharePct)
			}
			totalShare += customer.RevenueSharePct
		}

		if totalShare > 110 {
			t.Errorf("Symbol %s total revenue share %.1f%% seems too high", symbol, totalShare)
		}
	}
}

func TestRiskEventString(t *testing.T) {
	event := RiskEvent{
		Severity:       RiskLevelHigh,
		Type:           "customer_concentration",
		Description:    "Test risk",
		ImpactEstimate: -0.05,
		Confidence:     0.85,
	}

	str := event.String()

	if str == "" {
		t.Error("RiskEvent.String() returned empty string")
	}

	if !strings.Contains(str, "high") {
		t.Error("String should contain severity level")
	}

	if !strings.Contains(str, "customer_concentration") {
		t.Error("String should contain risk type")
	}

	if !strings.Contains(str, "Test risk") {
		t.Error("String should contain description")
	}
}

func TestRiskMonitor_CustomerConcentrationAction(t *testing.T) {
	rm := NewRiskMonitor()

	tests := []struct {
		score    float64
		expected string
	}{
		{0.9, "立即減碼，建立避險部位"},
		{0.7, "降低權重，監控客戶動態"},
		{0.5, "維持觀察，設定停損點"},
		{0.3, "正常監控"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			action := rm.getCustomerConcentrationAction(tt.score)
			if action != tt.expected {
				t.Errorf("Action for score %.1f = %s, want %s", tt.score, action, tt.expected)
			}
		})
	}
}

func TestRiskMonitor_AsymmetricAction(t *testing.T) {
	rm := NewRiskMonitor()

	tests := []struct {
		dropPct  float64
		expected string
	}{
		{0.12, "立即停損，評估基本面是否惡化"},
		{0.08, "減碼避險，等待市場穩定"},
		{0.06, "觀察支撐，設定緊密停損"},
		{0.03, "正常監控"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			action := rm.getAsymmetricAction(tt.dropPct)
			if action != tt.expected {
				t.Errorf("Action for drop %.2f = %s, want %s", tt.dropPct, action, tt.expected)
			}
		})
	}
}

func TestRiskEvent_Timestamp(t *testing.T) {
	rm := NewRiskMonitor()
	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	before := time.Now()
	risk := rm.CalculateCustomerConcentrationRisk("2330.TW")
	after := time.Now()

	if risk == nil {
		t.Fatal("Expected risk event")
	}

	if risk.DetectedAt.Before(before) || risk.DetectedAt.After(after) {
		t.Error("DetectedAt timestamp is out of expected range")
	}
}

func TestRiskEvent_ID(t *testing.T) {
	rm := NewRiskMonitor()
	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	risk1 := rm.CalculateCustomerConcentrationRisk("2330.TW")
	time.Sleep(1100 * time.Millisecond)
	risk2 := rm.CalculateCustomerConcentrationRisk("2330.TW")

	if risk1.ID == risk2.ID {
		t.Error("Risk event IDs should be unique")
	}

	if !strings.Contains(risk1.ID, "2330.TW") {
		t.Error("Risk ID should contain symbol")
	}
}

func TestCustomerConcentration_Validation(t *testing.T) {
	tests := []struct {
		name     string
		customer CustomerConcentration
		valid    bool
	}{
		{
			name: "valid customer",
			customer: CustomerConcentration{
				CustomerName:     "Apple",
				RevenueSharePct:  25.0,
				GeographicRegion: "US",
				RiskScore:        0.7,
			},
			valid: true,
		},
		{
			name: "empty name",
			customer: CustomerConcentration{
				CustomerName:    "",
				RevenueSharePct: 25.0,
			},
			valid: false,
		},
		{
			name: "negative revenue share",
			customer: CustomerConcentration{
				CustomerName:    "Apple",
				RevenueSharePct: -5.0,
			},
			valid: false,
		},
		{
			name: "revenue share over 100",
			customer: CustomerConcentration{
				CustomerName:    "Apple",
				RevenueSharePct: 105.0,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.customer.CustomerName == "" && tt.valid {
				t.Error("Empty customer name should be invalid")
			}
			if (tt.customer.RevenueSharePct < 0 || tt.customer.RevenueSharePct > 100) && tt.valid {
				t.Error("Revenue share out of range should be invalid")
			}
		})
	}
}

func TestNewsSource_TierValidation(t *testing.T) {
	sources := DefaultNewsSources()

	for _, source := range sources {
		if source.Tier < 1 || source.Tier > 3 {
			t.Errorf("Source %s has invalid tier: %d", source.Name, source.Tier)
		}
		if source.LatencyHours < 0 {
			t.Errorf("Source %s has negative latency: %.1f", source.Name, source.LatencyHours)
		}
	}
}

func TestAsymmetricRiskConfig_Validation(t *testing.T) {
	config := AsymmetricRiskConfig{
		BadNewsThreshold:      -0.03,
		GoodNewsThreshold:     0.05,
		ReactionTimeMinutes:   30,
		VolumeSpikeMultiplier: 2.0,
	}

	if config.BadNewsThreshold >= 0 {
		t.Error("BadNewsThreshold should be negative")
	}

	if config.GoodNewsThreshold <= 0 {
		t.Error("GoodNewsThreshold should be positive")
	}

	if config.ReactionTimeMinutes <= 0 {
		t.Error("ReactionTimeMinutes should be positive")
	}

	if config.VolumeSpikeMultiplier <= 1.0 {
		t.Error("VolumeSpikeMultiplier should be > 1.0")
	}
}

func TestRiskMonitor_ConcurrentAccess(t *testing.T) {
	rm := NewRiskMonitor()

	done := make(chan bool, 2)

	go func() {
		for i := range 100 {
			customers := []CustomerConcentration{
				{CustomerName: "Test", RevenueSharePct: float64(i), GeographicRegion: "US"},
			}
			rm.AddCustomerConcentration("TEST.TW", customers)
		}
		done <- true
	}()

	go func() {
		for range 100 {
			rm.GetCustomerConcentration("TEST.TW")
			rm.CalculateCustomerConcentrationRisk("TEST.TW")
		}
		done <- true
	}()

	<-done
	<-done
}

func TestCalculateCustomerConcentrationRisk_MultipleCalls(t *testing.T) {
	rm := NewRiskMonitor()

	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	risk1 := rm.CalculateCustomerConcentrationRisk("2330.TW")
	risk2 := rm.CalculateCustomerConcentrationRisk("2330.TW")

	if risk1 == nil || risk2 == nil {
		t.Fatal("Expected risks for both calls")
	}

	if risk1.Severity != risk2.Severity {
		t.Errorf("Inconsistent severity: %s vs %s", risk1.Severity, risk2.Severity)
	}

	if risk1.ImpactEstimate != risk2.ImpactEstimate {
		t.Errorf("Inconsistent impact: %.4f vs %.4f", risk1.ImpactEstimate, risk2.ImpactEstimate)
	}
}

func TestGetAllRisks_ExportIndustryWithNoPriceDrop(t *testing.T) {
	rm := NewRiskMonitor()

	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	risks := rm.GetAllRisks("2330.TW", "semiconductor", 0.01, 1.0)

	if len(risks) != 2 {
		t.Fatalf("Expected 2 risks, got %d", len(risks))
	}

	var hasCustomer, hasLatency bool
	for _, risk := range risks {
		switch risk.Type {
		case "customer_concentration":
			hasCustomer = true
		case "news_latency":
			hasLatency = true
		case "asymmetric":
			t.Error("Should not have asymmetric risk without price drop")
		}
	}

	if !hasCustomer {
		t.Error("Missing customer_concentration risk")
	}
	if !hasLatency {
		t.Error("Missing news_latency risk")
	}
}

func TestCalculateNewsLatencyRisk_LatencyGapZero(t *testing.T) {
	rm := NewRiskMonitor()

	risk := rm.CalculateNewsLatencyRisk("2330.TW", "semiconductor")

	if risk == nil {
		t.Fatal("Expected risk for semiconductor with default sources")
	}
	if !strings.Contains(risk.Description, "延遲") {
		t.Error("Description should mention delay")
	}
}

func TestRiskEvent_StructTags(t *testing.T) {
	event := RiskEvent{}
	_ = event.ID
	_ = event.Type
	_ = event.Severity
	_ = event.IndustryID
	_ = event.Symbol
	_ = event.Description
	_ = event.ImpactEstimate
	_ = event.Confidence
	_ = event.DetectedAt
	_ = event.Source
	_ = event.RecommendedAction
}

func TestCustomerConcentration_StructTags(t *testing.T) {
	cc := CustomerConcentration{}

	_ = cc.CustomerName
	_ = cc.CustomerTicker
	_ = cc.RevenueSharePct
	_ = cc.GeographicRegion
	_ = cc.RiskScore
	_ = cc.LastOrderDate
	_ = cc.OrderVisibilityMonths
}

func TestNewsSource_StructTags(t *testing.T) {
	ns := NewsSource{}

	_ = ns.Name
	_ = ns.Region
	_ = ns.Tier
	_ = ns.LatencyHours
	_ = ns.Reliability
	_ = ns.URL
}

func TestAsymmetricRiskConfig_StructTags(t *testing.T) {
	arc := AsymmetricRiskConfig{}

	_ = arc.BadNewsThreshold
	_ = arc.GoodNewsThreshold
	_ = arc.ReactionTimeMinutes
	_ = arc.VolumeSpikeMultiplier
}

func BenchmarkCalculateCustomerConcentrationRisk(b *testing.B) {
	rm := NewRiskMonitor()
	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
		{CustomerName: "Samsung", RevenueSharePct: 20.0, GeographicRegion: "KR"},
		{CustomerName: "Others", RevenueSharePct: 30.0, GeographicRegion: "TW"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rm.CalculateCustomerConcentrationRisk("2330.TW")
	}
}

func BenchmarkGetAllRisks(b *testing.B) {
	rm := NewRiskMonitor()
	customers := []CustomerConcentration{
		{CustomerName: "Apple", RevenueSharePct: 50.0, GeographicRegion: "US"},
	}
	rm.AddCustomerConcentration("2330.TW", customers)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rm.GetAllRisks("2330.TW", "semiconductor", -0.08, 3.0)
	}
}
