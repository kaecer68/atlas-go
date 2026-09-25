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

// TestStage3AlertEvaluator_ModelConfidenceDegraded pins the fixed predicate for
// issue #1944 N-U5: the neutral sentinel is the ledger's neutral encoding 0
// (inflow -> +conf, outflow -> -conf, neutral -> 0), NOT 0.5. The assertion was
// flipped from the bug-pinned version, which fed five 0.5 values (the padding
// value) and expected an alert.
func TestStage3AlertEvaluator_ModelConfidenceDegraded(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		// Five ledger-backed records, all neutral (DirectionSign == 0).
		RecentEventFlowPredictions:            func(days int) []float64 { return []float64{0, 0, 0, 0, 0} },
		RecentEventFlowPredictionsActualCount: func(days int) int { return 5 },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert for a genuinely neutral model, got %d", len(alerts))
	}
	if alerts[0].Level != AlertLevelWarning {
		t.Fatalf("expected warning level, got %v", alerts[0].Level)
	}
	if !strings.Contains(alerts[0].Message, "neutral") {
		t.Fatalf("expected message to mention neutral, got %q", alerts[0].Message)
	}
	if got := alerts[0].Metadata["actual_count"]; got != 5 {
		t.Errorf("metadata actual_count=%v, want 5", got)
	}
}

func TestStage3AlertEvaluator_ModelConfidenceDegraded_SkipsIfActive(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		// The last record is a real inflow prediction (DirectionSign 0.6), so the
		// model is not neutral.
		RecentEventFlowPredictions:            func(days int) []float64 { return []float64{0, 0, 0, 0, 0.6} },
		RecentEventFlowPredictionsActualCount: func(days int) int { return 5 },
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
		RecentEventFlowPredictions:            func(days int) []float64 { return []float64{0, 0, 0} }, // only 3 days
		RecentEventFlowPredictionsActualCount: func(days int) int { return 3 },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if len(captureHistory(monitor)) != 0 {
		t.Fatalf("expected no alert with fewer than 5 predictions, got %d", len(captureHistory(monitor)))
	}
}

// TestStage3AlertEvaluator_ModelConfidenceDegraded_NoAlertOnPadding is the
// regression test for the false alert half of issue #1944 N-U5: when the
// prediction ledger has no records the production wiring pads the slice with
// 0.5 values. Padding is absence of evidence, so the rule must stay silent.
func TestStage3AlertEvaluator_ModelConfidenceDegraded_NoAlertOnPadding(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		// Exactly what cmd/atlas/stage3_tasks.go returns for an empty ledger.
		RecentEventFlowPredictions:            func(days int) []float64 { return []float64{0.5, 0.5, 0.5, 0.5, 0.5} },
		RecentEventFlowPredictionsActualCount: func(days int) int { return 0 },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if got := len(captureHistory(monitor)); got != 0 {
		t.Fatalf("0.5 padding with 0 real records must not fire a neutral alert, got %d", got)
	}
}

// TestStage3AlertEvaluator_ModelConfidenceDegraded_NilActualCountStaysSilent:
// without the evidence counter the rule cannot tell padding from data, so it
// must not claim a degraded model.
func TestStage3AlertEvaluator_ModelConfidenceDegraded_NilActualCountStaysSilent(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions:            func(days int) []float64 { return []float64{0, 0, 0, 0, 0} },
		RecentEventFlowPredictionsActualCount: nil,
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if got := len(captureHistory(monitor)); got != 0 {
		t.Fatalf("nil actual-count callback must not fire a neutral alert, got %d", got)
	}
}

// TestStage3AlertEvaluator_ModelConfidenceDegraded_HalfIsNotNeutral documents the
// other half of the bug: 0.5 is a real (positive) signed magnitude, not the
// neutral sentinel, so a ledger holding five 0.5 records is NOT a degraded model.
func TestStage3AlertEvaluator_ModelConfidenceDegraded_HalfIsNotNeutral(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 6, 30, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions:            func(days int) []float64 { return []float64{0.5, 0.5, 0.5, 0.5, 0.5} },
		RecentEventFlowPredictionsActualCount: func(days int) int { return 5 },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateDaily()
	if got := len(captureHistory(monitor)); got != 0 {
		t.Fatalf("0.5 is inflow-with-0.5-confidence, not neutral: expected no alert, got %d", got)
	}
}

func TestStage3AlertEvaluator_PredictionDrift(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			// Small variance so std is non-zero (metadata only now).
			return []float64{0.45, 0.55, 0.48, 0.52, 0.49, 0.51, 0.46, 0.54, 0.47, 0.53}
		},
		LatestCapitalFlowPrediction: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.5}, true
		},
		LatestCapitalFlowActual: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "bullish", Value: 10.0}, true
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 drift alert (direction mismatch), got %d", len(alerts))
	}
	if alerts[0].Level != AlertLevelInfo {
		t.Fatalf("expected info level, got %v", alerts[0].Level)
	}
	// Metadata must carry the normalized labels so on-call can debug
	// without re-deriving thresholds from raw values.
	if got := alerts[0].Metadata["predicted_direction"]; got != "neutral" {
		t.Errorf("metadata predicted_direction=%v", got)
	}
	if got := alerts[0].Metadata["actual_direction"]; got != "bullish" {
		t.Errorf("metadata actual_direction=%v", got)
	}
}

func TestStage3AlertEvaluator_PredictionDrift_NoAlertOnDirectionMatch(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.6, 0.4, 0.5, 0.6, 0.4, 0.5, 0.6, 0.4, 0.5}
		},
		LatestCapitalFlowPrediction: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.5}, true
		},
		LatestCapitalFlowActual: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.6}, true
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	if len(captureHistory(monitor)) != 0 {
		t.Fatalf("expected no drift alert when directions match, got %d", len(captureHistory(monitor)))
	}
}

func TestStage3AlertEvaluator_PredictionDrift_InsufficientHistoryAlert(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}
		},
		RecentEventFlowPredictionsActualCount: func(days int) int { return 2 },
		LatestCapitalFlowPrediction: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.5}, true
		},
		LatestCapitalFlowActual: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "bullish", Value: 10.0}, true
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 insufficient-history alert, got %d", len(alerts))
	}
	if got, ok := alerts[0].Metadata["warmup"].(bool); !ok || !got {
		t.Fatalf("expected warmup=true metadata, got %v", alerts[0].Metadata["warmup"])
	}
}

func TestStage3AlertEvaluator_PredictionDrift_NilActualCountCallbackUsesLegacy(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.45, 0.55, 0.48, 0.52, 0.49, 0.51, 0.46, 0.54, 0.47, 0.53}
		},
		RecentEventFlowPredictionsActualCount: nil,
		LatestCapitalFlowPrediction: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.5}, true
		},
		LatestCapitalFlowActual: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "bullish", Value: 10.0}, true
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	alerts := captureHistory(monitor)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 regular drift alert in legacy mode, got %d", len(alerts))
	}
	if _, ok := alerts[0].Metadata["warmup"]; ok {
		t.Fatalf("legacy mode should not emit warmup metadata")
	}
}

// TestStage3AlertEvaluator_DriftComparisonIsRecordedEvenWithoutAlert proves the
// observation hook added for issue #1941: every evaluated prediction/actual
// pair is handed to OnCapitalFlowDriftCompared, hit or miss, including when the
// alert itself is suppressed (warmup gate / cooldown). That is what gives the
// Stage-3 task a durable prediction-vs-actual record instead of only a
// mismatch alert.
func TestStage3AlertEvaluator_DriftComparisonIsRecordedEvenWithoutAlert(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)

	var pairs [][2]CapitalFlowSignal
	deps := Stage3AlertDeps{
		// Warmup gate active: the alert path returns early, the record must not.
		RecentEventFlowPredictionsActualCount: func(days int) int { return 0 },
		RecentEventFlowPredictions: func(days int) []float64 {
			return []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}
		},
		LatestCapitalFlowPrediction: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.5}, true
		},
		LatestCapitalFlowActual: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "bearish", Value: -1.75}, true
		},
		OnCapitalFlowDriftCompared: func(predicted, actual CapitalFlowSignal) {
			pairs = append(pairs, [2]CapitalFlowSignal{predicted, actual})
		},
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	if len(pairs) != 1 {
		t.Fatalf("expected 1 recorded comparison, got %d", len(pairs))
	}
	if pairs[0][0].Direction != "neutral" || pairs[0][1].Direction != "bearish" || pairs[0][1].Value != -1.75 {
		t.Errorf("recorded pair = %+v, want the predicted/actual signals preserved", pairs[0])
	}
}

// TestStage3AlertEvaluator_DriftComparisonSkippedWhenActualUnavailable: no
// fabricated comparison when the actual side is missing.
func TestStage3AlertEvaluator_DriftComparisonSkippedWhenActualUnavailable(t *testing.T) {
	monitor := newStage3TestMonitor(t)
	now := time.Date(2026, 7, 13, 13, 45, 0, 0, time.UTC)

	recorded := 0
	deps := Stage3AlertDeps{
		RecentEventFlowPredictions: func(days int) []float64 { return make([]float64, days) },
		LatestCapitalFlowPrediction: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{Direction: "neutral", Value: 0.5}, true
		},
		LatestCapitalFlowActual: func() (CapitalFlowSignal, bool) {
			return CapitalFlowSignal{}, false
		},
		OnCapitalFlowDriftCompared: func(predicted, actual CapitalFlowSignal) { recorded++ },
	}
	eval := NewStage3AlertEvaluator(monitor, deps)
	eval.now = func() time.Time { return now }

	eval.EvaluateMarketClose()
	if recorded != 0 {
		t.Fatalf("expected no comparison record when the actual is unavailable, got %d", recorded)
	}
}
