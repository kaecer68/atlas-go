// Package globalmarket provides cross-market expansion capabilities
// Extends Atlas from Taiwan-only to global market coverage
package globalmarket

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// MarketRegion represents a geographic financial market
type MarketRegion string

const (
	RegionTaiwan    MarketRegion = "TW" // Taiwan (base)
	RegionUS        MarketRegion = "US" // United States
	RegionEurope    MarketRegion = "EU" // Europe
	RegionAsia      MarketRegion = "ASIA" // Asia ex-Taiwan
	RegionChina     MarketRegion = "CN" // China A-shares
	RegionJapan     MarketRegion = "JP" // Japan
	RegionEmerging  MarketRegion = "EM" // Emerging markets
)

// MarketConfig holds configuration for a specific market
type MarketConfig struct {
	Region         MarketRegion         `json:"region"`
	Name           string               `json:"name"`
	Currency       string               `json:"currency"`
	Timezone       string               `json:"timezone"`
	TradingHours   TradingSchedule      `json:"trading_hours"`
	Tickers        []string             `json:"tickers"`
	Indices        []string             `json:"indices"`
	MarketCapMin   float64              `json:"market_cap_min"`   // Minimum market cap in local currency
	LiquidityMin   float64              `json:"liquidity_min"`    // Minimum daily volume
	Enabled        bool                 `json:"enabled"`
	DataSource     string               `json:"data_source"`
	Specialization []AgentSpecialization `json:"specialization"`
	Correlation    map[string]float64   `json:"correlation"`      // Correlation with other markets
}

// TradingSchedule defines market hours
type TradingSchedule struct {
	PreMarket   string   `json:"pre_market"`   // e.g., "04:00-09:30"
	Regular     string   `json:"regular"`      // e.g., "09:30-16:00"
	AfterHours  string   `json:"after_hours"`  // e.g., "16:00-20:00"
	Holidays    []string `json:"holidays"`     // ISO dates
	WeekendDays []int    `json:"weekend_days"` // 0=Sunday, 6=Saturday
}

// AgentSpecialization maps agents to market expertise
type AgentSpecialization struct {
	AgentID      string   `json:"agent_id"`
	SkillType    string   `json:"skill_type"`   // e.g., "macro", "sector", "style"
	Expertise    []string `json:"expertise"`    // e.g., ["semiconductor", "AI"]
	Weight       float64  `json:"weight"`       // Weight in this market
}

// GlobalMarketManager manages multiple regional markets
type GlobalMarketManager struct {
	markets     map[MarketRegion]*MarketConfig
	agents      map[string]*GlobalAgent
	correlation *CorrelationMatrix
	config      *GlobalExpansionConfig
	mu          sync.RWMutex
}

// GlobalExpansionConfig configures the expansion
type GlobalExpansionConfig struct {
	PrimaryMarket      MarketRegion           `json:"primary_market"`
	EnabledMarkets     []MarketRegion         `json:"enabled_markets"`
	CrossMarketWeight  float64                `json:"cross_market_weight"`
	TimezoneOverlap    time.Duration          `json:"timezone_overlap"`
	DataSyncInterval   time.Duration          `json:"data_sync_interval"`
	MaxPositions       int                    `json:"max_positions"`
	RegionalLimits     map[MarketRegion]float64 `json:"regional_limits"`
}

// DefaultGlobalExpansionConfig returns sensible defaults
func DefaultGlobalExpansionConfig() *GlobalExpansionConfig {
	return &GlobalExpansionConfig{
		PrimaryMarket:     RegionTaiwan,
		EnabledMarkets:    []MarketRegion{RegionTaiwan, RegionUS, RegionAsia},
		CrossMarketWeight: 0.3,
		TimezoneOverlap:   2 * time.Hour,
		DataSyncInterval:  5 * time.Minute,
		MaxPositions:      50,
		RegionalLimits: map[MarketRegion]float64{
			RegionTaiwan: 0.5,  // 50% max Taiwan
			RegionUS:     0.4,  // 40% max US
			RegionAsia:   0.3,  // 30% max Asia ex-Taiwan
		},
	}
}

// NewGlobalMarketManager creates a manager
func NewGlobalMarketManager(config *GlobalExpansionConfig) *GlobalMarketManager {
	if config == nil {
		config = DefaultGlobalExpansionConfig()
	}
	
	gmm := &GlobalMarketManager{
		markets:     make(map[MarketRegion]*MarketConfig),
		agents:      make(map[string]*GlobalAgent),
		correlation: NewCorrelationMatrix(),
		config:      config,
	}
	
	gmm.initializeDefaultMarkets()
	
	return gmm
}

// initializeDefaultMarkets sets up base market configurations
func (gmm *GlobalMarketManager) initializeDefaultMarkets() {
	// Taiwan (base market)
	gmm.markets[RegionTaiwan] = &MarketConfig{
		Region:   RegionTaiwan,
		Name:     "Taiwan Stock Exchange",
		Currency: "TWD",
		Timezone: "Asia/Taipei",
		TradingHours: TradingSchedule{
			Regular:     "09:00-13:30",
			WeekendDays: []int{0, 6},
		},
		Tickers:      []string{"2330.TW", "2317.TW", "2454.TW", "2881.TW", "2308.TW"},
		Indices:      []string{"TWSE", "TAIEX"},
		MarketCapMin: 1000000000, // 1B TWD
		LiquidityMin: 50000000,   // 50M daily volume
		Enabled:      true,
		DataSource:   "twse",
		Specialization: []AgentSpecialization{
			{AgentID: "taiwan_macro", SkillType: "macro", Weight: 1.0},
			{AgentID: "sector_semiconductor", SkillType: "sector", Expertise: []string{"semiconductor"}, Weight: 1.0},
		},
		Correlation: map[string]float64{
			string(RegionUS):     0.65,
			string(RegionChina):  0.75,
			string(RegionAsia):   0.80,
			string(RegionJapan):  0.70,
		},
	}
	
	// US Markets
	gmm.markets[RegionUS] = &MarketConfig{
		Region:   RegionUS,
		Name:     "US Equity Markets",
		Currency: "USD",
		Timezone: "America/New_York",
		TradingHours: TradingSchedule{
			PreMarket:   "04:00-09:30",
			Regular:     "09:30-16:00",
			AfterHours:  "16:00-20:00",
			WeekendDays: []int{0, 6},
		},
		Tickers:      []string{"AAPL", "MSFT", "GOOGL", "NVDA", "TSLA", "AMZN", "META"},
		Indices:      []string{"SPX", "NDX", "DJI"},
		MarketCapMin: 1000000000, // 1B USD
		LiquidityMin: 10000000,    // 10M daily volume
		Enabled:      false, // Disabled by default, enable for expansion
		DataSource:   "polygon", // or "alpaca", "ibkr"
		Specialization: []AgentSpecialization{
			{AgentID: "us_macro", SkillType: "macro", Weight: 1.0},
			{AgentID: "us_tech_specialist", SkillType: "sector", Expertise: []string{"technology"}, Weight: 0.8},
			{AgentID: "super_druckenmiller", SkillType: "style", Weight: 1.0},
			{AgentID: "super_aschenbrenner", SkillType: "style", Weight: 1.0},
		},
		Correlation: map[string]float64{
			string(RegionTaiwan): 0.65,
			string(RegionEurope): 0.85,
			string(RegionAsia):  0.60,
		},
	}
	
	// Asia Ex-Taiwan
	gmm.markets[RegionAsia] = &MarketConfig{
		Region:   RegionAsia,
		Name:     "Asia Ex-Taiwan",
		Currency: "Mixed",
		Timezone: "Asia/Singapore",
		TradingHours: TradingSchedule{
			Regular:     "09:00-17:00",
			WeekendDays: []int{0, 6},
		},
		Tickers:      []string{"005930.KS", "9988.HK", "7203.T", "RELIANCE.NS"}, // Samsung, Alibaba, Toyota, Reliance
		Indices:      []string{"KOSPI", "HSI", "N225", "SENSEX"},
		MarketCapMin: 500000000, // 500M local
		LiquidityMin: 5000000,    // 5M daily
		Enabled:      false,
		DataSource:   "bloomberg",
		Specialization: []AgentSpecialization{
			{AgentID: "asia_macro", SkillType: "macro", Weight: 1.0},
			{AgentID: "korea_tech_specialist", SkillType: "sector", Expertise: []string{"technology", "electronics"}, Weight: 0.8},
		},
		Correlation: map[string]float64{
			string(RegionTaiwan): 0.80,
			string(RegionChina):  0.85,
			string(RegionUS):     0.60,
		},
	}
	
	// Japan
	gmm.markets[RegionJapan] = &MarketConfig{
		Region:   RegionJapan,
		Name:     "Tokyo Stock Exchange",
		Currency: "JPY",
		Timezone: "Asia/Tokyo",
		TradingHours: TradingSchedule{
			PreMarket:   "08:00-09:00",
			Regular:     "09:00-15:00",
			AfterHours:  "15:00-17:00",
			WeekendDays: []int{0, 6},
		},
		Tickers:      []string{"7203.T", "6758.T", "9984.T"}, // Toyota, Sony, SoftBank
		Indices:      []string{"N225", "TPX"},
		MarketCapMin: 100000000000, // 100B JPY
		LiquidityMin: 1000000000,   // 1B daily
		Enabled:      false,
		DataSource:   "jpx",
		Specialization: []AgentSpecialization{
			{AgentID: "japan_macro", SkillType: "macro", Weight: 1.0},
			{AgentID: "japan_tech_specialist", SkillType: "sector", Expertise: []string{"electronics", "automotive"}, Weight: 0.8},
		},
		Correlation: map[string]float64{
			string(RegionTaiwan): 0.70,
			string(RegionUS):     0.72,
			string(RegionAsia):   0.85,
		},
	}
	
	// Europe
	gmm.markets[RegionEurope] = &MarketConfig{
		Region:   RegionEurope,
		Name:     "European Markets",
		Currency: "EUR",
		Timezone: "Europe/London",
		TradingHours: TradingSchedule{
			Regular:     "08:00-16:30",
			WeekendDays: []int{0, 6},
		},
		Tickers:      []string{"ASML.AS", "SAP.DE", "NESN.SW", "OR.PA"},
		Indices:      []string{"SX5E", "UKX", "DAX"},
		MarketCapMin: 1000000000, // 1B EUR
		LiquidityMin: 10000000,   // 10M daily
		Enabled:      false,
		DataSource:   "euroclear",
		Specialization: []AgentSpecialization{
			{AgentID: "europe_macro", SkillType: "macro", Weight: 1.0},
			{AgentID: "europe_tech_specialist", SkillType: "sector", Expertise: []string{"semiconductor", "software"}, Weight: 0.8},
		},
		Correlation: map[string]float64{
			string(RegionUS):     0.85,
			string(RegionTaiwan): 0.55,
			string(RegionAsia):   0.60,
		},
	}
	
	// China A-shares
	gmm.markets[RegionChina] = &MarketConfig{
		Region:   RegionChina,
		Name:     "China A-Shares",
		Currency: "CNY",
		Timezone: "Asia/Shanghai",
		TradingHours: TradingSchedule{
			Regular:     "09:30-15:00",
			WeekendDays: []int{0, 6},
		},
		Tickers:      []string{"000001.SZ", "600519.SS"}, // Ping An, Kweichow Moutai
		Indices:      []string{"CSI300", "SSE50"},
		MarketCapMin: 10000000000, // 10B CNY
		LiquidityMin: 100000000,   // 100M daily
		Enabled:      false,
		DataSource:   "wind",
		Specialization: []AgentSpecialization{
			{AgentID: "china_macro", SkillType: "macro", Weight: 1.0},
			{AgentID: "china_consumer_specialist", SkillType: "sector", Expertise: []string{"consumer"}, Weight: 0.8},
		},
		Correlation: map[string]float64{
			string(RegionTaiwan): 0.75,
			string(RegionAsia):   0.85,
			string(RegionUS):     0.45,
		},
	}
}

// EnableMarket activates a market for trading
func (gmm *GlobalMarketManager) EnableMarket(region MarketRegion) error {
	gmm.mu.Lock()
	defer gmm.mu.Unlock()
	
	market, exists := gmm.markets[region]
	if !exists {
		return fmt.Errorf("market %s not configured", region)
	}
	
	market.Enabled = true
	
	// Spawn agents for this market if needed
	gmm.spawnMarketAgents(region)
	
	return nil
}

// DisableMarket deactivates a market
func (gmm *GlobalMarketManager) DisableMarket(region MarketRegion) {
	gmm.mu.Lock()
	defer gmm.mu.Unlock()
	
	if market, exists := gmm.markets[region]; exists {
		market.Enabled = false
	}
}

// spawnMarketAgents creates specialized agents for a market
func (gmm *GlobalMarketManager) spawnMarketAgents(region MarketRegion) {
	market := gmm.markets[region]
	if market == nil {
		return
	}
	
	for _, spec := range market.Specialization {
		agentID := fmt.Sprintf("%s_%s", region, spec.AgentID)
		
		agent := &GlobalAgent{
			ID:          agentID,
			BaseAgentID: spec.AgentID,
			Region:      region,
			SkillType:   spec.SkillType,
			Expertise:   spec.Expertise,
			Weight:      spec.Weight,
			Enabled:     true,
			CreatedAt:   time.Now(),
		}
		
		gmm.agents[agentID] = agent
	}
}

// GetActiveMarkets returns enabled markets
func (gmm *GlobalMarketManager) GetActiveMarkets() []*MarketConfig {
	gmm.mu.RLock()
	defer gmm.mu.RUnlock()
	
	var active []*MarketConfig
	for _, market := range gmm.markets {
		if market.Enabled {
			active = append(active, market)
		}
	}
	
	return active
}

// GetMarketAgent returns the appropriate agent for a market
func (gmm *GlobalMarketManager) GetMarketAgent(region MarketRegion, skillType string) *GlobalAgent {
	gmm.mu.RLock()
	defer gmm.mu.RUnlock()
	
	prefix := string(region)
	
	for id, agent := range gmm.agents {
		if agent.Region == region && agent.SkillType == skillType {
			if agent.Enabled {
				return agent
			}
		}
		// Check if this agent is for this region
		if len(id) > len(prefix) && id[:len(prefix)] == prefix && agent.SkillType == skillType {
			if agent.Enabled {
				return agent
			}
		}
	}
	
	// Fallback to primary market agent
	for _, agent := range gmm.agents {
		if agent.SkillType == skillType && agent.Enabled {
			return agent
		}
	}
	
	return nil
}

// GetCrossMarketCorrelation returns correlation between two markets
func (gmm *GlobalMarketManager) GetCrossMarketCorrelation(r1, r2 MarketRegion) float64 {
	gmm.mu.RLock()
	defer gmm.mu.RUnlock()
	
	market1, exists1 := gmm.markets[r1]
	market2, exists2 := gmm.markets[r2]
	
	if !exists1 || !exists2 {
		return 0.5 // Default moderate correlation
	}
	
	// Check market1's correlation to r2
	if corr, exists := market1.Correlation[string(r2)]; exists {
		return corr
	}
	
	// Check market2's correlation to r1
	if corr, exists := market2.Correlation[string(r1)]; exists {
		return corr
	}
	
	return 0.5 // Default
}

// CalculateGlobalExposure computes portfolio exposure across markets
func (gmm *GlobalMarketManager) CalculateGlobalExposure(positions map[string]float64) *ExposureReport {
	report := &ExposureReport{
		ByRegion:     make(map[MarketRegion]float64),
		ByCurrency:   make(map[string]float64),
		TotalValue:   0,
		GeneratedAt:  time.Now(),
	}
	
	for symbol, value := range positions {
		region := gmm.inferRegionFromSymbol(symbol)
		report.ByRegion[region] += value
		
		if market, exists := gmm.markets[region]; exists {
			report.ByCurrency[market.Currency] += value
		}
		
		report.TotalValue += value
	}
	
	// Calculate percentages
	if report.TotalValue > 0 {
		for region, value := range report.ByRegion {
			report.ByRegion[region] = value / report.TotalValue
		}
		for currency, value := range report.ByCurrency {
			report.ByCurrency[currency] = value / report.TotalValue
		}
	}
	
	// Check limits
	report.LimitBreaches = gmm.checkLimitBreaches(report.ByRegion)
	
	return report
}

// inferRegionFromSymbol determines market from ticker symbol
func (gmm *GlobalMarketManager) inferRegionFromSymbol(symbol string) MarketRegion {
	// Taiwan: .TW suffix
	if len(symbol) > 3 && symbol[len(symbol)-3:] == ".TW" {
		return RegionTaiwan
	}
	
	// US: No suffix or common US patterns
	if len(symbol) < 5 && (symbol[0] >= 'A' && symbol[0] <= 'Z') {
		return RegionUS
	}
	
	// Korea: .KS suffix
	if len(symbol) > 3 && symbol[len(symbol)-3:] == ".KS" {
		return RegionAsia // South Korea
	}
	
	// Japan: .T suffix
	if len(symbol) > 2 && symbol[len(symbol)-2:] == ".T" {
		return RegionJapan
	}
	
	// Hong Kong: .HK suffix
	if len(symbol) > 3 && symbol[len(symbol)-3:] == ".HK" {
		return RegionAsia
	}
	
	// Default to primary market
	return gmm.config.PrimaryMarket
}

// checkLimitBreaches identifies region exposure violations
func (gmm *GlobalMarketManager) checkLimitBreaches(exposures map[MarketRegion]float64) []LimitBreach {
	var breaches []LimitBreach
	
	for region, exposure := range exposures {
		if limit, exists := gmm.config.RegionalLimits[region]; exists {
			if exposure > limit {
				breaches = append(breaches, LimitBreach{
					Region:   region,
					Exposure: exposure,
					Limit:    limit,
					Excess:   exposure - limit,
				})
			}
		}
	}
	
	return breaches
}

// ExposureReport summarizes portfolio distribution
type ExposureReport struct {
	ByRegion      map[MarketRegion]float64 `json:"by_region"`
	ByCurrency    map[string]float64       `json:"by_currency"`
	TotalValue    float64                  `json:"total_value"`
	LimitBreaches []LimitBreach            `json:"limit_breaches"`
	GeneratedAt   time.Time                `json:"generated_at"`
}

// LimitBreach indicates exceeded regional limit
type LimitBreach struct {
	Region   MarketRegion `json:"region"`
	Exposure float64      `json:"exposure"`
	Limit    float64      `json:"limit"`
	Excess   float64      `json:"excess"`
}

// GlobalAgent represents a market-specialized agent
type GlobalAgent struct {
	ID          string    `json:"id"`
	BaseAgentID string    `json:"base_agent_id"`
	Region      MarketRegion `json:"region"`
	SkillType   string    `json:"skill_type"`
	Expertise   []string  `json:"expertise"`
	Weight      float64   `json:"weight"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	LastActive  time.Time `json:"last_active"`
}

// CorrelationMatrix tracks cross-market correlations
type CorrelationMatrix struct {
	data map[string]map[string]float64
	mu   sync.RWMutex
}

// NewCorrelationMatrix creates a matrix
func NewCorrelationMatrix() *CorrelationMatrix {
	return &CorrelationMatrix{
		data: make(map[string]map[string]float64),
	}
}

// SetCorrelation updates correlation between two markets
func (cm *CorrelationMatrix) SetCorrelation(m1, m2 string, corr float64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if cm.data[m1] == nil {
		cm.data[m1] = make(map[string]float64)
	}
	if cm.data[m2] == nil {
		cm.data[m2] = make(map[string]float64)
	}
	
	cm.data[m1][m2] = corr
	cm.data[m2][m1] = corr
}

// GetCorrelation retrieves correlation value
func (cm *CorrelationMatrix) GetCorrelation(m1, m2 string) float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	if cm.data[m1] != nil {
		if corr, exists := cm.data[m1][m2]; exists {
			return corr
		}
	}
	
	return 0.5 // Default
}

// GetDiversificationScore calculates portfolio diversification
func (cm *CorrelationMatrix) GetDiversificationScore(markets []MarketRegion) float64 {
	if len(markets) <= 1 {
		return 1.0
	}
	
	// Average inverse correlation
	totalCorr := 0.0
	count := 0
	
	for i, m1 := range markets {
		for j, m2 := range markets {
			if i < j {
				corr := cm.GetCorrelation(string(m1), string(m2))
				totalCorr += corr
				count++
			}
		}
	}
	
	if count == 0 {
		return 1.0
	}
	
	avgCorr := totalCorr / float64(count)
	// Lower correlation = higher diversification score
	return 1.0 - avgCorr
}

// Save persists global market configuration
func (gmm *GlobalMarketManager) Save(filepath string) error {
	gmm.mu.RLock()
	defer gmm.mu.RUnlock()
	
	state := struct {
		Markets     map[MarketRegion]*MarketConfig `json:"markets"`
		Agents      map[string]*GlobalAgent        `json:"agents"`
		Config      *GlobalExpansionConfig         `json:"config"`
		SavedAt     time.Time                      `json:"saved_at"`
	}{
		Markets: gmm.markets,
		Agents:  gmm.agents,
		Config:  gmm.config,
		SavedAt: time.Now(),
	}
	
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filepath, data, 0644)
}

// Load restores global market configuration
func (gmm *GlobalMarketManager) Load(filepath string) error {
	gmm.mu.Lock()
	defer gmm.mu.Unlock()
	
	data, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}
	
	var state struct {
		Markets map[MarketRegion]*MarketConfig `json:"markets"`
		Agents  map[string]*GlobalAgent        `json:"agents"`
		Config  *GlobalExpansionConfig         `json:"config"`
	}
	
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	
	gmm.markets = state.Markets
	gmm.agents = state.Agents
	gmm.config = state.Config
	
	return nil
}

// GenerateReport creates comprehensive global expansion analysis
func (gmm *GlobalMarketManager) GenerateReport() *GlobalExpansionReport {
	gmm.mu.RLock()
	defer gmm.mu.RUnlock()
	
	report := &GlobalExpansionReport{
		GeneratedAt:      time.Now(),
		EnabledMarkets:   make([]MarketRegion, 0),
		AvailableMarkets: make([]MarketRegion, 0),
		AgentCount:       len(gmm.agents),
	}
	
	for region, market := range gmm.markets {
		if market.Enabled {
			report.EnabledMarkets = append(report.EnabledMarkets, region)
		} else {
			report.AvailableMarkets = append(report.AvailableMarkets, region)
		}
	}
	
	// Calculate diversification score
	report.DiversificationScore = gmm.correlation.GetDiversificationScore(report.EnabledMarkets)
	
	// Count agents per region
	report.AgentsByRegion = make(map[MarketRegion]int)
	for _, agent := range gmm.agents {
		if agent.Enabled {
			report.AgentsByRegion[agent.Region]++
		}
	}
	
	return report
}

// GlobalExpansionReport summarizes expansion status
type GlobalExpansionReport struct {
	GeneratedAt        time.Time              `json:"generated_at"`
	EnabledMarkets     []MarketRegion         `json:"enabled_markets"`
	AvailableMarkets   []MarketRegion         `json:"available_markets"`
	AgentCount         int                    `json:"agent_count"`
	AgentsByRegion     map[MarketRegion]int   `json:"agents_by_region"`
	DiversificationScore float64              `json:"diversification_score"`
}
