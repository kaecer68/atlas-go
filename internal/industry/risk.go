package industry

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// RiskLevel represents the severity of a risk.
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// CustomerConcentration tracks the dependency on major customers.
type CustomerConcentration struct {
	CustomerName          string     `json:"customer_name"`
	CustomerTicker        string     `json:"customer_ticker,omitempty"`
	RevenueSharePct       float64    `json:"revenue_share_pct"` // 0-100
	GeographicRegion      string     `json:"geographic_region"` // US, CN, EU, etc.
	RiskScore             float64    `json:"risk_score"`        // 0-1, higher = more risky
	LastOrderDate         *time.Time `json:"last_order_date,omitempty"`
	OrderVisibilityMonths float64    `json:"order_visibility_months"`
}

// NewsSource represents a news/information source.
type NewsSource struct {
	Name         string  `json:"name"`
	Region       string  `json:"region"`        // US, TW, JP, etc.
	Tier         int     `json:"tier"`          // 1 = primary, 2 = secondary, 3 = tertiary
	LatencyHours float64 `json:"latency_hours"` // Average delay vs primary source
	Reliability  float64 `json:"reliability"`   // 0-1
	URL          string  `json:"url,omitempty"`
}

// RiskEvent represents a detected risk event.
type RiskEvent struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"` // "customer_concentration", "news_latency", "asymmetric"
	Severity          RiskLevel `json:"severity"`
	IndustryID        string    `json:"industry_id"`
	Symbol            string    `json:"symbol,omitempty"`
	Description       string    `json:"description"`
	ImpactEstimate    float64   `json:"impact_estimate"` // Estimated price impact
	Confidence        float64   `json:"confidence"`      // 0-1
	DetectedAt        time.Time `json:"detected_at"`
	Source            string    `json:"source"`
	RecommendedAction string    `json:"recommended_action,omitempty"`
}

// RiskMonitor monitors industry-specific risks.
type RiskMonitor struct {
	mu               sync.RWMutex
	customerData     map[string][]CustomerConcentration // symbol -> customers
	newsSources      []NewsSource
	asymmetricConfig AsymmetricRiskConfig
}

// AsymmetricRiskConfig configures asymmetric risk detection.
type AsymmetricRiskConfig struct {
	BadNewsThreshold      float64 `json:"bad_news_threshold"`      // Price drop threshold
	GoodNewsThreshold     float64 `json:"good_news_threshold"`     // Price rise threshold
	ReactionTimeMinutes   int     `json:"reaction_time_minutes"`   // Time to react
	VolumeSpikeMultiplier float64 `json:"volume_spike_multiplier"` // Volume increase threshold
}

// NewRiskMonitor creates a new risk monitor with default customer concentration data loaded.
func NewRiskMonitor() *RiskMonitor {
	rm := &RiskMonitor{
		customerData: make(map[string][]CustomerConcentration),
		newsSources:  DefaultNewsSources(),
		asymmetricConfig: AsymmetricRiskConfig{
			BadNewsThreshold:      -0.03, // 3% drop
			GoodNewsThreshold:     0.05,  // 5% rise
			ReactionTimeMinutes:   30,    // 30 minutes
			VolumeSpikeMultiplier: 2.0,   // 2x average volume
		},
	}

	// Load default customer concentration data during initialization
	for symbol, customers := range DefaultCustomerConcentrations() {
		rm.customerData[symbol] = customers
	}

	return rm
}

// DefaultNewsSources returns the default news source configuration.
func DefaultNewsSources() []NewsSource {
	return []NewsSource{
		{Name: "Bloomberg", Region: "US", Tier: 1, LatencyHours: 0, Reliability: 0.95},
		{Name: "Reuters", Region: "US", Tier: 1, LatencyHours: 0, Reliability: 0.93},
		{Name: "CNBC", Region: "US", Tier: 1, LatencyHours: 0.5, Reliability: 0.88},
		{Name: "Wall Street Journal", Region: "US", Tier: 1, LatencyHours: 1, Reliability: 0.92},
		{Name: "Financial Times", Region: "EU", Tier: 1, LatencyHours: 1, Reliability: 0.90},
		{Name: "Morgan Stanley Research", Region: "US", Tier: 2, LatencyHours: 2, Reliability: 0.85},
		{Name: "Goldman Sachs Research", Region: "US", Tier: 2, LatencyHours: 2, Reliability: 0.85},
		{Name: "工商時報", Region: "TW", Tier: 3, LatencyHours: 8, Reliability: 0.75},
		{Name: "經濟日報", Region: "TW", Tier: 3, LatencyHours: 8, Reliability: 0.75},
		{Name: "財訊雙週刊", Region: "TW", Tier: 3, LatencyHours: 168, Reliability: 0.70},
	}
}

// AddCustomerConcentration adds customer concentration data for a symbol.
func (rm *RiskMonitor) AddCustomerConcentration(symbol string, customers []CustomerConcentration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.customerData[symbol] = customers
}

// GetCustomerConcentration returns customer concentration for a symbol.
func (rm *RiskMonitor) GetCustomerConcentration(symbol string) []CustomerConcentration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.customerData[symbol]
}

// CalculateCustomerConcentrationRisk calculates the customer concentration risk.
func (rm *RiskMonitor) CalculateCustomerConcentrationRisk(symbol string) *RiskEvent {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	customers, ok := rm.customerData[symbol]
	if !ok || len(customers) == 0 {
		return nil
	}

	// Calculate total revenue from top customers
	var totalRevenueShare float64
	var topCustomerShare float64
	var topCustomerName string
	var usExposure float64

	for _, customer := range customers {
		totalRevenueShare += customer.RevenueSharePct
		if customer.RevenueSharePct > topCustomerShare {
			topCustomerShare = customer.RevenueSharePct
			topCustomerName = customer.CustomerName
		}
		if customer.GeographicRegion == "US" {
			usExposure += customer.RevenueSharePct
		}
	}

	params := config.GetParametersConfig().Industry

	// Calculate risk score
	riskScore := 0.0
	if topCustomerShare > params.CustomerShareThreshold1.Value {
		riskScore += params.RiskScoreWeight1.Value
	}
	if topCustomerShare > params.CustomerShareThreshold2.Value {
		riskScore += params.RiskScoreWeight2.Value
	}
	if usExposure > params.USExposureThreshold1.Value {
		riskScore += params.RiskScoreWeight3.Value
	}
	if usExposure > params.USExposureThreshold2.Value {
		riskScore += params.RiskScoreWeight4.Value
	}

	if riskScore == 0 {
		return nil
	}

	severity := RiskLevelLow
	if riskScore > params.SeverityThresholdCritical.Value {
		severity = RiskLevelCritical
	} else if riskScore > params.SeverityThresholdHigh.Value {
		severity = RiskLevelHigh
	} else if riskScore > params.SeverityThresholdMedium.Value {
		severity = RiskLevelMedium
	}

	return &RiskEvent{
		ID:                fmt.Sprintf("risk-customer-%s-%d", symbol, time.Now().Unix()),
		Type:              "customer_concentration",
		Severity:          severity,
		IndustryID:        "semiconductor", // Default, should be parameterized
		Symbol:            symbol,
		Description:       fmt.Sprintf("Top customer %s accounts for %.0f%% of revenue; US exposure %.0f%%", topCustomerName, topCustomerShare, usExposure),
		ImpactEstimate:    -riskScore * params.ImpactMultiplier.Value,
		Confidence:        params.RiskConfidence.Value,
		DetectedAt:        time.Now(),
		Source:            "internal_analysis",
		RecommendedAction: rm.getCustomerConcentrationAction(riskScore),
	}
}

func (rm *RiskMonitor) getCustomerConcentrationAction(riskScore float64) string {
	params := config.GetParametersConfig().Industry
	switch {
	case riskScore > params.SeverityThresholdCritical.Value:
		return "立即減碼，建立避險部位"
	case riskScore > params.SeverityThresholdHigh.Value:
		return "降低權重，監控客戶動態"
	case riskScore > params.SeverityThresholdMedium.Value:
		return "維持觀察，設定停損點"
	default:
		return "正常監控"
	}
}

// CalculateNewsLatencyRisk calculates the news latency risk for Taiwan stocks.
func (rm *RiskMonitor) CalculateNewsLatencyRisk(symbol string, industryID string) *RiskEvent {
	// Check if this is an export-oriented industry
	exportIndustries := map[string]bool{
		"semiconductor":   true,
		"ai_supply_chain": true,
		"electronics":     true,
		"robotics":        true,
		"shipping":        true,
	}

	if !exportIndustries[industryID] {
		return nil
	}

	// Calculate average latency for Tier 1 US sources
	var avgTier1Latency float64
	var tier1Count int
	for _, source := range rm.newsSources {
		if source.Tier == 1 && source.Region == "US" {
			avgTier1Latency += source.LatencyHours
			tier1Count++
		}
	}

	if tier1Count > 0 {
		avgTier1Latency /= float64(tier1Count)
	}

	// Calculate average latency for Taiwan sources
	var avgTaiwanLatency float64
	var taiwanCount int
	for _, source := range rm.newsSources {
		if source.Region == "TW" {
			avgTaiwanLatency += source.LatencyHours
			taiwanCount++
		}
	}

	if taiwanCount > 0 {
		avgTaiwanLatency /= float64(taiwanCount)
	}

	latencyGap := avgTaiwanLatency - avgTier1Latency
	if latencyGap <= 0 {
		return nil
	}

	// Risk increases with latency gap
	riskScore := math.Min(1.0, latencyGap/24.0) // Max risk at 24h gap

	severity := RiskLevelLow
	if riskScore > 0.8 {
		severity = RiskLevelHigh
	} else if riskScore > 0.5 {
		severity = RiskLevelMedium
	}

	return &RiskEvent{
		ID:                fmt.Sprintf("risk-latency-%s-%d", symbol, time.Now().Unix()),
		Type:              "news_latency",
		Severity:          severity,
		IndustryID:        industryID,
		Symbol:            symbol,
		Description:       fmt.Sprintf("台灣新聞延遲 %.0f 小時，美國Tier 1消息先到", latencyGap),
		ImpactEstimate:    -riskScore * 0.05,
		Confidence:        0.80,
		DetectedAt:        time.Now(),
		Source:            "news_source_analysis",
		RecommendedAction: "優先監控美國Tier 1新聞源",
	}
}

// CalculateAsymmetricRisk calculates the asymmetric risk (bad news vs good news impact).
func (rm *RiskMonitor) CalculateAsymmetricRisk(symbol string, priceChangePct float64, volumeMultiplier float64) *RiskEvent {
	// Detect bad news event
	if priceChangePct >= rm.asymmetricConfig.BadNewsThreshold {
		return nil
	}

	// Check if volume spike confirms
	if volumeMultiplier < rm.asymmetricConfig.VolumeSpikeMultiplier {
		return nil
	}

	// Calculate severity based on price drop
	severity := RiskLevelLow
	dropPct := math.Abs(priceChangePct)
	if dropPct > 0.10 {
		severity = RiskLevelCritical
	} else if dropPct > 0.07 {
		severity = RiskLevelHigh
	} else if dropPct > 0.05 {
		severity = RiskLevelMedium
	}

	return &RiskEvent{
		ID:                fmt.Sprintf("risk-asymmetric-%s-%d", symbol, time.Now().Unix()),
		Type:              "asymmetric",
		Severity:          severity,
		IndustryID:        "", // Should be determined from symbol
		Symbol:            symbol,
		Description:       fmt.Sprintf("股價下跌 %.1f%%，成交量放大 %.1f 倍，壞消息衝擊", dropPct*100, volumeMultiplier),
		ImpactEstimate:    priceChangePct,
		Confidence:        math.Min(1.0, volumeMultiplier/3.0),
		DetectedAt:        time.Now(),
		Source:            "price_volume_analysis",
		RecommendedAction: rm.getAsymmetricAction(dropPct),
	}
}

func (rm *RiskMonitor) getAsymmetricAction(dropPct float64) string {
	switch {
	case dropPct > 0.10:
		return "立即停損，評估基本面是否惡化"
	case dropPct > 0.07:
		return "減碼避險，等待市場穩定"
	case dropPct > 0.05:
		return "觀察支撐，設定緊密停損"
	default:
		return "正常監控"
	}
}

// GetAllRisks returns all risks for a symbol.
func (rm *RiskMonitor) GetAllRisks(symbol string, industryID string, priceChangePct float64, volumeMultiplier float64) []RiskEvent {
	var risks []RiskEvent

	if risk := rm.CalculateCustomerConcentrationRisk(symbol); risk != nil {
		risks = append(risks, *risk)
	}

	if risk := rm.CalculateNewsLatencyRisk(symbol, industryID); risk != nil {
		risks = append(risks, *risk)
	}

	if risk := rm.CalculateAsymmetricRisk(symbol, priceChangePct, volumeMultiplier); risk != nil {
		risks = append(risks, *risk)
	}

	return risks
}

// GetHighestRisk returns the highest severity risk.
func (rm *RiskMonitor) GetHighestRisk(risks []RiskEvent) *RiskEvent {
	if len(risks) == 0 {
		return nil
	}

	severityOrder := map[RiskLevel]int{
		RiskLevelLow:      1,
		RiskLevelMedium:   2,
		RiskLevelHigh:     3,
		RiskLevelCritical: 4,
	}

	highest := &risks[0]
	for i := range risks {
		if severityOrder[risks[i].Severity] > severityOrder[highest.Severity] {
			highest = &risks[i]
		}
	}
	return highest
}

// DefaultCustomerConcentrations returns sample customer concentration data.
func DefaultCustomerConcentrations() map[string][]CustomerConcentration {
	return map[string][]CustomerConcentration{
		"2330.TW": {
			{CustomerName: "Apple", CustomerTicker: "AAPL", RevenueSharePct: 25.0, GeographicRegion: "US", RiskScore: 0.7, OrderVisibilityMonths: 6},
			{CustomerName: "NVIDIA", CustomerTicker: "NVDA", RevenueSharePct: 15.0, GeographicRegion: "US", RiskScore: 0.6, OrderVisibilityMonths: 4},
			{CustomerName: "AMD", CustomerTicker: "AMD", RevenueSharePct: 10.0, GeographicRegion: "US", RiskScore: 0.5, OrderVisibilityMonths: 3},
			{CustomerName: "MediaTek", CustomerTicker: "2454.TW", RevenueSharePct: 8.0, GeographicRegion: "TW", RiskScore: 0.3, OrderVisibilityMonths: 3},
		},
		"2382.TW": {
			{CustomerName: "Microsoft", CustomerTicker: "MSFT", RevenueSharePct: 30.0, GeographicRegion: "US", RiskScore: 0.7, OrderVisibilityMonths: 4},
			{CustomerName: "Google", CustomerTicker: "GOOGL", RevenueSharePct: 20.0, GeographicRegion: "US", RiskScore: 0.6, OrderVisibilityMonths: 3},
			{CustomerName: "Amazon", CustomerTicker: "AMZN", RevenueSharePct: 15.0, GeographicRegion: "US", RiskScore: 0.6, OrderVisibilityMonths: 3},
		},
		"2317.TW": {
			{CustomerName: "Apple", CustomerTicker: "AAPL", RevenueSharePct: 50.0, GeographicRegion: "US", RiskScore: 0.9, OrderVisibilityMonths: 6},
			{CustomerName: "Sony", CustomerTicker: "SONY", RevenueSharePct: 10.0, GeographicRegion: "JP", RiskScore: 0.4, OrderVisibilityMonths: 3},
		},
		"2454.TW": {
			{CustomerName: "Samsung", CustomerTicker: "005930.KS", RevenueSharePct: 20.0, GeographicRegion: "KR", RiskScore: 0.5, OrderVisibilityMonths: 3},
			{CustomerName: "OPPO", CustomerTicker: "", RevenueSharePct: 15.0, GeographicRegion: "CN", RiskScore: 0.6, OrderVisibilityMonths: 2},
			{CustomerName: "Xiaomi", CustomerTicker: "1810.HK", RevenueSharePct: 12.0, GeographicRegion: "CN", RiskScore: 0.5, OrderVisibilityMonths: 2},
		},
		"2603.TW": {
			{CustomerName: "Global Trade", CustomerTicker: "", RevenueSharePct: 40.0, GeographicRegion: "Global", RiskScore: 0.4, OrderVisibilityMonths: 1},
		},
	}
}

// String returns a human-readable summary of the risk event.
func (re *RiskEvent) String() string {
	return fmt.Sprintf("[%s] %s: %s (Impact: %.1f%%, Confidence: %.0f%%)",
		re.Severity,
		re.Type,
		re.Description,
		re.ImpactEstimate*100,
		re.Confidence*100,
	)
}
