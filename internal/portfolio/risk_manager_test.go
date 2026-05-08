package portfolio

import (
	"testing"
)

func TestNewRiskManager(t *testing.T) {
	rm := NewRiskManager()
	if rm == nil {
		t.Fatal("expected non-nil risk manager")
	}

	metrics := rm.GetRiskMetrics()
	if metrics.MaxDrawdown != 0.08 {
		t.Errorf("expected default max drawdown 0.08, got %f", metrics.MaxDrawdown)
	}
	if metrics.ActivePositions != 0 {
		t.Errorf("expected 0 active positions, got %d", metrics.ActivePositions)
	}
}

func TestRiskManagerUpdatePortfolioValuePeakTracking(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Initial value
	rm.UpdatePortfolioValue(100000)
	metrics := rm.GetRiskMetrics()
	if metrics.CurrentDrawdown != 0 {
		t.Errorf("expected 0 drawdown at peak, got %f", metrics.CurrentDrawdown)
	}

	// Increase value
	rm.UpdatePortfolioValue(110000)
	metrics = rm.GetRiskMetrics()
	if metrics.CurrentDrawdown != 0 {
		t.Errorf("expected 0 drawdown at new peak, got %f", metrics.CurrentDrawdown)
	}

	// Decrease value
	rm.UpdatePortfolioValue(105000)
	metrics = rm.GetRiskMetrics()
	expectedDrawdown := (110000.0 - 105000.0) / 110000.0
	if metrics.CurrentDrawdown != expectedDrawdown {
		t.Errorf("expected drawdown %f, got %f", expectedDrawdown, metrics.CurrentDrawdown)
	}
}

func TestRiskManagerUpdatePortfolioValueDrawdownAlert(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Start at 100000
	rm.UpdatePortfolioValue(100000)
	// Drop to 91000 (9% drawdown, exceeding 8% limit)
	alerts := rm.UpdatePortfolioValue(91000)

	foundDrawdownAlert := false
	for _, alert := range alerts {
		if alert.Type == AlertDrawdown {
			foundDrawdownAlert = true
			if alert.Level != LevelCritical {
				t.Errorf("expected critical level for drawdown, got %d", alert.Level)
			}
		}
	}
	if !foundDrawdownAlert {
		t.Error("expected drawdown alert when exceeding limit")
	}
}

func TestRiskManagerUpdatePortfolioValueDailyLossAlert(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Start day at 100000
	rm.dailyStartValue = 100000
	// Lose 5% (exceeds 3% daily limit)
	alerts := rm.UpdatePortfolioValue(95000)

	foundDailyLossAlert := false
	for _, alert := range alerts {
		if alert.Type == AlertDailyLoss {
			foundDailyLossAlert = true
			if alert.Level != LevelWarning {
				t.Errorf("expected warning level for daily loss, got %d", alert.Level)
			}
		}
	}
	if !foundDailyLossAlert {
		t.Error("expected daily loss alert when exceeding limit")
	}
}

func TestRiskManagerUpdatePortfolioValueConcentrationAlert(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Add positions that exceed concentration limit
	// totalExposure = 20000, currentValue = 100000, maxPositionSize = 0.15
	// 20000/100000 = 0.20 > 0.15
	rm.totalExposure = 20000
	alerts := rm.UpdatePortfolioValue(100000)

	foundConcentrationAlert := false
	for _, alert := range alerts {
		if alert.Type == AlertConcentration {
			foundConcentrationAlert = true
			if alert.Level != LevelWarning {
				t.Errorf("expected warning level for concentration, got %d", alert.Level)
			}
		}
	}
	if !foundConcentrationAlert {
		t.Error("expected concentration alert when exceeding limit")
	}
}

func TestRiskManagerUpdatePortfolioValueNoAlerts(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Normal values, no limits exceeded
	rm.dailyStartValue = 100000
	rm.totalExposure = 10000 // 10% of portfolio, below 15% limit
	alerts := rm.UpdatePortfolioValue(99000)

	if len(alerts) != 0 {
		t.Errorf("expected no alerts, got %d: %+v", len(alerts), alerts)
	}
}

func TestRiskManagerAddPositionSuccess(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)
	rm.currentValue = 100000

	err := rm.AddPosition("2330.TW", 10, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metrics := rm.GetRiskMetrics()
	if metrics.ActivePositions != 1 {
		t.Errorf("expected 1 active position, got %d", metrics.ActivePositions)
	}
	if metrics.TotalExposure != 5000 {
		t.Errorf("expected 5000 exposure, got %f", metrics.TotalExposure)
	}
}

func TestRiskManagerAddPositionExceedsSizeLimit(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)
	rm.currentValue = 100000

	// Position value = 20000, limit = 15000
	err := rm.AddPosition("2330.TW", 100, 200)
	if err == nil {
		t.Fatal("expected error for position exceeding size limit")
	}
}

func TestRiskManagerAddPositionExceedsTotalExposure(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)
	rm.currentValue = 100000

	// Add first position
	err := rm.AddPosition("2330.TW", 10, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Add second position that would exceed total exposure (80% max)
	// Current exposure = 5000, adding 80000 would exceed 80000 limit
	err = rm.AddPosition("2317.TW", 1000, 100)
	if err == nil {
		t.Fatal("expected error for total exposure exceeding limit")
	}
}

func TestRiskManagerUpdatePosition(t *testing.T) {
	rm := NewRiskManager()
	rm.currentValue = 100000

	rm.AddPosition("2330.TW", 10, 500)
	rm.UpdatePosition("2330.TW", 550)

	position := rm.positions["2330.TW"]
	if position == nil {
		t.Fatal("expected position to exist")
	}

	expectedUnrealized := (550.0 - 500.0) * 10.0
	if position.Unrealized != expectedUnrealized {
		t.Errorf("expected unrealized %f, got %f", expectedUnrealized, position.Unrealized)
	}
}

func TestRiskManagerUpdatePositionStopLossAlert(t *testing.T) {
	rm := NewRiskManager()
	rm.currentValue = 100000

	// Add position at 500
	rm.AddPosition("2330.TW", 10, 500)
	// Update to 470 (6% loss, exceeds 5% stop loss)
	rm.UpdatePosition("2330.TW", 470)

	alerts := rm.GetActiveAlerts()
	foundStopLoss := false
	for _, alert := range alerts {
		if alert.Type == AlertPositionSize {
			foundStopLoss = true
		}
	}
	if !foundStopLoss {
		t.Error("expected stop loss alert when position drops below threshold")
	}
}

func TestRiskManagerUpdatePositionTakeProfitAlert(t *testing.T) {
	rm := NewRiskManager()
	rm.currentValue = 100000

	// Add position at 500
	rm.AddPosition("2330.TW", 10, 500)
	// Update to 650 (30% gain, exceeds 20% take profit)
	rm.UpdatePosition("2330.TW", 650)

	alerts := rm.GetActiveAlerts()
	foundTakeProfit := false
	for _, alert := range alerts {
		if alert.Type == AlertPositionSize && alert.Level == LevelInfo {
			foundTakeProfit = true
		}
	}
	if !foundTakeProfit {
		t.Error("expected take profit alert when position exceeds threshold")
	}
}

func TestRiskManagerUpdatePositionNotFound(t *testing.T) {
	rm := NewRiskManager()
	// Should not panic
	rm.UpdatePosition("UNKNOWN.TW", 100)
}

func TestRiskManagerRemovePosition(t *testing.T) {
	rm := NewRiskManager()
	rm.currentValue = 100000

	rm.AddPosition("2330.TW", 10, 500)
	rm.RemovePosition("2330.TW", 500)

	metrics := rm.GetRiskMetrics()
	if metrics.ActivePositions != 0 {
		t.Errorf("expected 0 active positions, got %d", metrics.ActivePositions)
	}
	if metrics.TotalExposure != 0 {
		t.Errorf("expected 0 exposure, got %f", metrics.TotalExposure)
	}
}

func TestRiskManagerRemovePositionNotFound(t *testing.T) {
	rm := NewRiskManager()
	// Should not panic
	rm.RemovePosition("UNKNOWN.TW", 0)
}

func TestRiskManagerShouldStopTradingDrawdown(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Drawdown below threshold
	rm.UpdatePortfolioValue(100000)
	if rm.ShouldStopTrading() {
		t.Error("expected not to stop trading at peak")
	}

	rm.peakValue = 100000
	rm.currentValue = 87900
	rm.currentDrawdown = 0.121
	if !rm.ShouldStopTrading() {
		t.Error("expected to stop trading when drawdown exceeds 150% of max")
	}
}

func TestRiskManagerShouldStopTradingCriticalAlerts(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Add 3 critical alerts
	for range 3 {
		alert := rm.createAlert(AlertDrawdown, LevelCritical, "test", "portfolio", 0.1, 0.08)
		rm.riskAlerts = append(rm.riskAlerts, alert)
	}

	if !rm.ShouldStopTrading() {
		t.Error("expected to stop trading with 3+ critical alerts")
	}
}

func TestRiskManagerShouldStopTradingNormal(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.08, 0.15, 0.03)

	// Add 2 critical alerts (below threshold)
	for range 2 {
		alert := rm.createAlert(AlertDrawdown, LevelCritical, "test", "portfolio", 0.1, 0.08)
		rm.riskAlerts = append(rm.riskAlerts, alert)
	}

	// Small drawdown
	rm.currentDrawdown = 0.05

	if rm.ShouldStopTrading() {
		t.Error("expected not to stop trading with 2 critical alerts and moderate drawdown")
	}
}

func TestRiskManagerResetDaily(t *testing.T) {
	rm := NewRiskManager()
	rm.dailyStartValue = 100000
	rm.currentValue = 110000

	rm.ResetDaily()
	if rm.dailyStartValue != 110000 {
		t.Errorf("expected daily start value to reset to current value, got %f", rm.dailyStartValue)
	}
}

func TestRiskManagerGetActiveAlerts(t *testing.T) {
	rm := NewRiskManager()

	// Add resolved alert
	resolvedAlert := rm.createAlert(AlertDrawdown, LevelWarning, "resolved", "portfolio", 0.05, 0.08)
	resolvedAlert.Resolved = true
	rm.riskAlerts = append(rm.riskAlerts, resolvedAlert)

	// Add unresolved alert
	unresolvedAlert := rm.createAlert(AlertDailyLoss, LevelWarning, "unresolved", "portfolio", 0.05, 0.03)
	rm.riskAlerts = append(rm.riskAlerts, unresolvedAlert)

	activeAlerts := rm.GetActiveAlerts()
	if len(activeAlerts) != 1 {
		t.Errorf("expected 1 active alert, got %d", len(activeAlerts))
	}
	if activeAlerts[0].Message != "unresolved" {
		t.Errorf("expected unresolved alert, got %s", activeAlerts[0].Message)
	}
}

func TestRiskManagerCalculatePositionSize(t *testing.T) {
	rm := NewRiskManager()
	rm.currentValue = 100000

	// Normal case
	size := rm.CalculatePositionSize(500, 0.20)
	if size <= 0 {
		t.Errorf("expected positive position size, got %f", size)
	}

	// High volatility should reduce size
	sizeHighVol := rm.CalculatePositionSize(500, 1.0)
	sizeLowVol := rm.CalculatePositionSize(500, 0.10)
	if sizeHighVol >= sizeLowVol {
		t.Errorf("expected high vol to produce smaller size, got high=%f low=%f", sizeHighVol, sizeLowVol)
	}
}

func TestRiskManagerCalculatePositionSizeRespectsMaxPosition(t *testing.T) {
	rm := NewRiskManager()
	rm.currentValue = 100000

	// With very low volatility, size could be large but should be capped
	size := rm.CalculatePositionSize(100, 0.01)
	maxValue := rm.currentValue * rm.maxPositionSize
	if size*100 > maxValue {
		t.Errorf("position value %f exceeds max %f", size*100, maxValue)
	}
}

func TestRiskManagerSetRiskParameters(t *testing.T) {
	rm := NewRiskManager()
	rm.SetRiskParameters(0.10, 0.20, 0.05)

	if rm.maxDrawdownPct != 0.10 {
		t.Errorf("expected max drawdown 0.10, got %f", rm.maxDrawdownPct)
	}
	if rm.maxPositionSize != 0.20 {
		t.Errorf("expected max position size 0.20, got %f", rm.maxPositionSize)
	}
	if rm.maxDailyLossPct != 0.05 {
		t.Errorf("expected max daily loss 0.05, got %f", rm.maxDailyLossPct)
	}
}

func TestRiskManagerCreateAlert(t *testing.T) {
	rm := NewRiskManager()
	alert := rm.createAlert(AlertDrawdown, LevelCritical, "test message", "portfolio", 0.1, 0.08)

	if alert.Type != AlertDrawdown {
		t.Errorf("expected alert type AlertDrawdown, got %d", alert.Type)
	}
	if alert.Level != LevelCritical {
		t.Errorf("expected level Critical, got %d", alert.Level)
	}
	if alert.Message != "test message" {
		t.Errorf("expected message 'test message', got %s", alert.Message)
	}
	if alert.Target != "portfolio" {
		t.Errorf("expected target 'portfolio', got %s", alert.Target)
	}
	if alert.Current != 0.1 {
		t.Errorf("expected current 0.1, got %f", alert.Current)
	}
	if alert.Threshold != 0.08 {
		t.Errorf("expected threshold 0.08, got %f", alert.Threshold)
	}
	if alert.Resolved {
		t.Error("expected new alert to be unresolved")
	}
	if alert.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}
