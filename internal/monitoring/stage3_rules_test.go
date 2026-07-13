package monitoring

import (
	"strings"
	"testing"
	"time"
)

func newStage3TestMonitor(t *testing.T) *Monitor {
	t.Helper()
	return NewMonitor()
}

func captureHistory(m *Monitor) []Alert {
	return m.GetHistory(100)
}

func TestStage3AlertEvaluator_DataStaleness(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		ChannelLastDataAt: func() map[string]time.Time {
			return map[string]time.Time{
				"twse_capital_flow": now.Add(-3 * time.Hour),
				"twse_margin":       now.Add(-7 * time.Hour),
				"recent_ok":         now.Add(-30 * time.Minute),
			}
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateStaleness()
	alerts := captureHistory(monitor)
	if len(alerts) != 2 {
		t.Fatalf("expected 2 staleness alerts, got %d", len(alerts))
	}

	var warning, critical bool
	for _, a := range alerts {
		if a.Level == AlertLevelWarning && strings.Contains(a.Message, "twse_capital_flow") {
			warning = true
		}
		if a.Level == AlertLevelCritical && strings.Contains(a.Message, "twse_margin") {
			critical = true
		}
	}
	if !warning {
		t.Fatalf("expected warning for twse_capital_flow, got %+v", alerts)
	}
	if !critical {
		t.Fatalf("expected critical for twse_margin, got %+v", alerts)
	}
}

func TestStage3AlertEvaluator_DataStaleness_Cooldown(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		ChannelLastDataAt: func() map[string]time.Time {
			return map[string]time.Time{
				"twse_capital_flow": now.Add(-3 * time.Hour),
			}
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateStaleness()
	if len(captureHistory(monitor)) != 1 {
		t.Fatalf("expected first evaluation to emit 1 alert")
	}

	// Second evaluation at the same time should be suppressed by cooldown.
	eval.EvaluateStaleness()
	if len(captureHistory(monitor)) != 1 {
		t.Fatalf("expected cooldown to suppress second alert, got %d", len(captureHistory(monitor)))
	}

	// After the cooldown window, it should fire again.
	eval.now = func() time.Time { return now.Add(2 * time.Hour) }
	eval.EvaluateStaleness()
	if len(captureHistory(monitor)) != 2 {
		t.Fatalf("expected alert after cooldown, got %d", len(captureHistory(monitor)))
	}
}

func TestStage3AlertEvaluator_EventCalendarSparse(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		IsTradingDay:            func(date time.Time) bool { return true },
		EventCalendarEventCount: func(date time.Time) int { return 2 },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Level != AlertLevelWarning {
		t.Fatalf("expected warning level, got %v", alerts[0].Level)
	}
	if !strings.Contains(alerts[0].Message, "2 events") {
		t.Fatalf("expected message to mention event count, got %q", alerts[0].Message)
	}
}

func TestStage3AlertEvaluator_EventCalendarSparse_SkipsNonTradingDay(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		IsTradingDay:            func(date time.Time) bool { return false },
		EventCalendarEventCount: func(date time.Time) int { return 0 },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if len(captureHistory(monitor)) != 0 {
		t.Fatalf("expected no alerts on non-trading day, got %d", len(captureHistory(monitor)))
	}
}

func TestStage3AlertEvaluator_ModelConfidenceDegraded(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.5, 0.5, 0.5, 0.5}
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Level != AlertLevelWarning {
		t.Fatalf("expected warning level, got %v", alerts[0].Level)
	}
	if !strings.Contains(alerts[0].Message, "neutral") {
		t.Fatalf("expected message to mention neutral, got %q", alerts[0].Message)
	}
}

func TestStage3AlertEvaluator_ModelConfidenceDegraded_SkipsIfActive(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.5, 0.5, 0.5, 0.6}
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if len(captureHistory(monitor)) != 0 {
		t.Fatalf("expected no alert when predictions are not all neutral, got %d", len(captureHistory(monitor)))
	}
}

func TestStage3AlertEvaluator_ModelConfidenceDegraded_NeedsFiveDays(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.5, 0.5} // only 3 days
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if len(captureHistory(monitor)) != 0 {
		t.Fatalf("expected no alert with fewer than 5 predictions, got %d", len(captureHistory(monitor)))
	}
}

func TestStage3AlertEvaluator_PredictionDrift(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			// Small variance around 0.5 so std is non-zero.
			return []float64{0.45, 0.55, 0.48, 0.52, 0.49, 0.51, 0.46, 0.54, 0.47, 0.53}
		},
		LatestCapitalFlowPrediction: func() (float64, bool) { return 0.5, true },
		LatestCapitalFlowActual:     func() (float64, bool) { return 10.0, true },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 drift alert, got %d", len(alerts))
	}
	if alerts[0].Level != AlertLevelInfo {
		t.Fatalf("expected info level, got %v", alerts[0].Level)
	}
}

func TestStage3AlertEvaluator_PredictionDrift_SkipsIfWithinSigma(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.6, 0.4, 0.5, 0.6, 0.4, 0.5, 0.6, 0.4, 0.5}
		},
		LatestCapitalFlowPrediction: func() (float64, bool) { return 0.5, true },
		LatestCapitalFlowActual:     func() (float64, bool) { return 0.6, true },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	if len(captureHistory(monitor)) != 0 {
		t.Fatalf("expected no drift alert within 2σ, got %d", len(captureHistory(monitor)))
	}
}
