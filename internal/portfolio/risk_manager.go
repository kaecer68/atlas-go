package portfolio

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// RiskManager manages portfolio risk and drawdown control
type RiskManager struct {
	mu sync.RWMutex

	// Risk parameters
	maxDrawdownPct    float64 // Maximum allowed drawdown (e.g., 0.08 for 8%)
	maxPositionSize   float64 // Maximum position size as percentage of portfolio
	maxDailyLossPct   float64 // Maximum daily loss percentage
	stopLossEnabled   bool
	takeProfitEnabled bool

	// Current state
	currentDrawdown float64
	peakValue       float64
	currentValue    float64
	dailyStartValue float64
	lastUpdate      time.Time

	// Risk alerts
	riskAlerts   []RiskAlert
	alertHistory []RiskAlert

	// Position tracking
	positions     map[string]*Position
	totalExposure float64
}

// RiskAlert represents a risk management alert
type RiskAlert struct {
	ID         string
	Type       AlertType
	Level      AlertLevel
	Message    string
	Target     string
	Current    float64
	Threshold  float64
	Timestamp  time.Time
	Resolved   bool
	ResolvedAt *time.Time
}

// AlertType represents different types of risk alerts
type AlertType int

const (
	AlertDrawdown AlertType = iota
	AlertPositionSize
	AlertDailyLoss
	AlertVolatility
	AlertConcentration
)

// AlertLevel represents the severity level of an alert
type AlertLevel int

const (
	LevelInfo AlertLevel = iota
	LevelWarning
	LevelCritical
	LevelEmergency
)

// Position represents a trading position
type Position struct {
	Symbol       string
	Size         float64
	EntryPrice   float64
	CurrentPrice float64
	Unrealized   float64
	Realized     float64
	OpenTime     time.Time
	LastUpdate   time.Time
}

// NewRiskManager creates a new risk manager with default parameters
func NewRiskManager() *RiskManager {
	return &RiskManager{
		maxDrawdownPct:    0.08, // 8% max drawdown
		maxPositionSize:   0.15, // 15% max position size
		maxDailyLossPct:   0.03, // 3% max daily loss
		stopLossEnabled:   true,
		takeProfitEnabled: true,
		positions:         make(map[string]*Position),
		lastUpdate:        time.Now(),
		dailyStartValue:   100000.0, // Default starting value
		currentValue:      100000.0,
		peakValue:         100000.0,
	}
}

// SetRiskParameters configures risk management parameters
func (rm *RiskManager) SetRiskParameters(maxDrawdown, maxPosSize, maxDailyLoss float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.maxDrawdownPct = maxDrawdown
	rm.maxPositionSize = maxPosSize
	rm.maxDailyLossPct = maxDailyLoss
}

// UpdatePortfolioValue updates the current portfolio value and checks risk limits
func (rm *RiskManager) UpdatePortfolioValue(value float64) []RiskAlert {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.currentValue = value
	rm.lastUpdate = time.Now()

	// Update peak value
	if value > rm.peakValue {
		rm.peakValue = value
	}

	// Calculate current drawdown
	rm.currentDrawdown = (rm.peakValue - value) / rm.peakValue

	var newAlerts []RiskAlert

	// Check drawdown limit
	if rm.currentDrawdown > rm.maxDrawdownPct {
		alert := rm.createAlert(AlertDrawdown, LevelCritical,
			fmt.Sprintf("Portfolio drawdown %.2f%% exceeds limit %.2f%%",
				rm.currentDrawdown*100, rm.maxDrawdownPct*100),
			"portfolio", rm.currentDrawdown, rm.maxDrawdownPct)
		newAlerts = append(newAlerts, alert)
	}

	// Check daily loss
	dailyLoss := (rm.dailyStartValue - value) / rm.dailyStartValue
	if dailyLoss > rm.maxDailyLossPct {
		alert := rm.createAlert(AlertDailyLoss, LevelWarning,
			fmt.Sprintf("Daily loss %.2f%% exceeds limit %.2f%%",
				dailyLoss*100, rm.maxDailyLossPct*100),
			"portfolio", dailyLoss, rm.maxDailyLossPct)
		newAlerts = append(newAlerts, alert)
	}

	// Check position concentration
	if rm.totalExposure > rm.currentValue*rm.maxPositionSize {
		alert := rm.createAlert(AlertConcentration, LevelWarning,
			fmt.Sprintf("Position concentration %.2f%% exceeds limit %.2f%%",
				(rm.totalExposure/rm.currentValue)*100, rm.maxPositionSize*100),
			"positions", rm.totalExposure/rm.currentValue, rm.maxPositionSize)
		newAlerts = append(newAlerts, alert)
	}

	rm.riskAlerts = append(rm.riskAlerts, newAlerts...)
	rm.alertHistory = append(rm.alertHistory, newAlerts...)

	return newAlerts
}

// AddPosition adds a new position and checks risk limits
func (rm *RiskManager) AddPosition(symbol string, size, price float64) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check position size limit
	positionValue := size * price
	if positionValue > rm.currentValue*rm.maxPositionSize {
		return fmt.Errorf("position size %.2f exceeds maximum %.2f",
			positionValue, rm.currentValue*rm.maxPositionSize)
	}

	// Check total exposure
	newTotalExposure := rm.totalExposure + positionValue
	if newTotalExposure > rm.currentValue*0.8 { // 80% max total exposure
		return fmt.Errorf("total exposure would exceed 80%% of portfolio")
	}

	// Add position
	position := &Position{
		Symbol:       symbol,
		Size:         size,
		EntryPrice:   price,
		CurrentPrice: price,
		Unrealized:   0,
		Realized:     0,
		OpenTime:     time.Now(),
		LastUpdate:   time.Now(),
	}

	rm.positions[symbol] = position
	rm.totalExposure = newTotalExposure

	return nil
}

// UpdatePosition updates an existing position's current price
func (rm *RiskManager) UpdatePosition(symbol string, currentPrice float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	position, exists := rm.positions[symbol]
	if !exists {
		return
	}

	position.CurrentPrice = currentPrice
	position.LastUpdate = time.Now()
	position.Unrealized = (currentPrice - position.EntryPrice) * position.Size

	// Check for stop loss or take profit
	if rm.stopLossEnabled && position.Unrealized < -position.EntryPrice*position.Size*0.05 {
		alert := rm.createAlert(AlertPositionSize, LevelWarning,
			fmt.Sprintf("Stop loss triggered for %s at %.2f", symbol, currentPrice),
			symbol, currentPrice, position.EntryPrice*0.95)
		rm.riskAlerts = append(rm.riskAlerts, alert)
	}

	if rm.takeProfitEnabled && position.Unrealized > position.EntryPrice*position.Size*0.20 {
		alert := rm.createAlert(AlertPositionSize, LevelInfo,
			fmt.Sprintf("Take profit triggered for %s at %.2f", symbol, currentPrice),
			symbol, currentPrice, position.EntryPrice*1.20)
		rm.riskAlerts = append(rm.riskAlerts, alert)
	}
}

// RemovePosition removes a position from tracking
func (rm *RiskManager) RemovePosition(symbol string, realizedPnL float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	position, exists := rm.positions[symbol]
	if !exists {
		return
	}

	position.Realized = realizedPnL
	rm.totalExposure -= position.Size * position.CurrentPrice
	delete(rm.positions, symbol)
}

// GetRiskMetrics returns current risk metrics
func (rm *RiskManager) GetRiskMetrics() RiskMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return RiskMetrics{
		CurrentDrawdown: rm.currentDrawdown,
		MaxDrawdown:     rm.maxDrawdownPct,
		TotalExposure:   rm.totalExposure,
		ExposureRatio:   rm.totalExposure / rm.currentValue,
		ActivePositions: len(rm.positions),
		ActiveAlerts:    len(rm.getActiveAlerts()),
		LastUpdate:      rm.lastUpdate,
	}
}

// RiskMetrics represents current risk metrics
type RiskMetrics struct {
	CurrentDrawdown float64
	MaxDrawdown     float64
	TotalExposure   float64
	ExposureRatio   float64
	ActivePositions int
	ActiveAlerts    int
	LastUpdate      time.Time
}

// GetActiveAlerts returns currently active alerts
func (rm *RiskManager) GetActiveAlerts() []RiskAlert {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.getActiveAlerts()
}

// getActiveAlerts returns unresolved alerts
func (rm *RiskManager) getActiveAlerts() []RiskAlert {
	active := make([]RiskAlert, 0)
	for _, alert := range rm.riskAlerts {
		if !alert.Resolved {
			active = append(active, alert)
		}
	}
	return active
}

// createAlert creates a new risk alert
func (rm *RiskManager) createAlert(alertType AlertType, level AlertLevel, message, target string, current, threshold float64) RiskAlert {
	return RiskAlert{
		ID:        fmt.Sprintf("alert_%d_%s", time.Now().Unix(), target),
		Type:      alertType,
		Level:     level,
		Message:   message,
		Target:    target,
		Current:   current,
		Threshold: threshold,
		Timestamp: time.Now(),
		Resolved:  false,
	}
}

// ResetDaily resets daily tracking
func (rm *RiskManager) ResetDaily() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dailyStartValue = rm.currentValue
}

// ShouldStopTrading determines if trading should be stopped based on risk criteria
func (rm *RiskManager) ShouldStopTrading() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Stop trading if drawdown exceeds critical threshold
	if rm.currentDrawdown > rm.maxDrawdownPct*1.5 { // 150% of max drawdown
		return true
	}

	// Stop trading if too many critical alerts
	criticalCount := 0
	for _, alert := range rm.getActiveAlerts() {
		if alert.Level == LevelCritical || alert.Level == LevelEmergency {
			criticalCount++
		}
	}

	return criticalCount >= 3
}

// CalculatePositionSize calculates optimal position size based on risk parameters
func (rm *RiskManager) CalculatePositionSize(price, volatility float64) float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Kelly criterion with risk adjustment
	maxLoss := 0.02 // 2% max loss per trade
	if volatility > 0 {
		// Adjust for volatility
		maxLoss = math.Min(maxLoss, 0.01/volatility)
	}

	// Position size as percentage of portfolio
	positionSize := (rm.currentValue * maxLoss) / price

	// Apply maximum position size limit
	maxPosValue := rm.currentValue * rm.maxPositionSize
	if positionSize*price > maxPosValue {
		positionSize = maxPosValue / price
	}

	return positionSize
}
