package monitoring

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Stage3AlertDeps provides the data sources required to evaluate Stage 3
// data-quality alert rules. Callers wire the actual data accessors in cmd/atlas
// so that the evaluator stays decoupled from concrete services.
type Stage3AlertDeps struct {
	// TimeZone is used for schedule checks. If nil, UTC is assumed.
	TimeZone *time.Location

	// ChannelLastDataAt returns the most recent data timestamp for each channel.
	// Channels that have never produced data should be omitted from the map.
	ChannelLastDataAt func() map[string]time.Time

	// IsTradingDay returns true if the given date is a Taiwan trading day.
	IsTradingDay func(date time.Time) bool

	// EventCalendarEventCount returns the number of calendar events for the
	// given date.
	EventCalendarEventCount func(date time.Time) int

	// RecentEventFlowPredictions returns the last N event-flow prediction
	// confidence values. A value of 0.5 means neutral / no signal.
	RecentEventFlowPredictions func(days int) []float64

	// RecentEventFlowPredictionsActualCount counts ledger-backed (not
	// 0.5-padded) records returned by RecentEventFlowPredictions. Used by
	// the prediction-drift rule to gate alerts until enough history exists.
	RecentEventFlowPredictionsActualCount func(days int) int

	// OnAlertFired is invoked after each alert fires past its cooldown.
	// Optional; production wiring emits atlas_stage3_alerts_fired_total.
	OnAlertFired func(ruleID string, level AlertLevel, metadata map[string]any)

	// LatestCapitalFlowPrediction returns the most recent capital-flow
	// prediction as a normalized direction label (manifest #F01: both sides
	// of the drift comparison are now unit-agnostic). The Value carries the
	// raw confidence in [0,1] for diagnostics.
	LatestCapitalFlowPrediction func() (CapitalFlowSignal, bool)

	// LatestCapitalFlowActual returns the most recent capital-flow actual
	// outcome as a normalized direction label. The Value carries the raw
	// QualityScore in [-3,3] for diagnostics.
	LatestCapitalFlowActual func() (CapitalFlowSignal, bool)
}

// CapitalFlowSignal normalizes a prediction or actual outcome into a
// direction label so the drift comparison (#F01) no longer mixes units.
type CapitalFlowSignal struct {
	Direction string  // "bullish" | "bearish" | "neutral"
	Value     float64 // raw value preserved for diagnostics
}

// ClassifyDirection maps a scalar to a direction label using symmetric
// thresholds. Conventions: positive = bullish, negative = bearish, near
// zero = neutral. Callers pass thresholds appropriate to the input range
// (e.g. [0.4, 0.6] for [0,1] confidence, [-0.5, 0.5] for [-3,3] QualityScore).
func ClassifyDirection(value, bullishThreshold, bearishThreshold float64) string {
	switch {
	case value > bullishThreshold:
		return "bullish"
	case value < bearishThreshold:
		return "bearish"
	default:
		return "neutral"
	}
}

// Stage3AlertEvaluator evaluates the Stage 3 data-quality alert rules and
// emits alerts through the provided Monitor. It is safe for use by a single
// BackgroundTaskManager goroutine.
type Stage3AlertEvaluator struct {
	monitor   *Monitor
	deps      Stage3AlertDeps
	mu        sync.Mutex
	lastFired map[string]time.Time
	now       func() time.Time
}

// NewStage3AlertEvaluator creates an evaluator wired to the given monitor and
// dependencies.
func NewStage3AlertEvaluator(monitor *Monitor, deps Stage3AlertDeps) *Stage3AlertEvaluator {
	if monitor == nil {
		panic("Stage3AlertEvaluator: monitor is required")
	}
	return &Stage3AlertEvaluator{
		monitor:   monitor,
		deps:      deps,
		lastFired: make(map[string]time.Time),
		now:       time.Now,
	}
}

// EvaluateDaily runs the daily scheduled rules (event-calendar-sparse,
// model-confidence-degraded). It is intended to be called at 06:30 local time.
func (e *Stage3AlertEvaluator) EvaluateDaily() {
	e.evaluateEventCalendarSparse()
	e.evaluateModelConfidenceDegraded()
}

// EvaluateMarketClose runs the prediction-drift rule after the market close
// data is available. Intended to be called at 13:45 local time.
func (e *Stage3AlertEvaluator) EvaluateMarketClose() {
	e.evaluatePredictionDrift()
}

// EvaluateStaleness runs the data-staleness rules. Intended to be called
// every 5 minutes (aligned with .omo/plans/Atlas 錢潮方向預測實作規劃.md § Stage 3.2:
// "任一 channel 失效時報警會在 5 分鐘內觸發"; changed from 10m → 5m in Stage 8.1
// per user decision Q2 option C, 2026-07-15).
func (e *Stage3AlertEvaluator) EvaluateStaleness() {
	e.evaluateDataStaleness()
}

func (e *Stage3AlertEvaluator) tz() *time.Location {
	if e.deps.TimeZone != nil {
		return e.deps.TimeZone
	}
	return time.UTC
}

func (e *Stage3AlertEvaluator) checkCooldown(ruleID string, cooldown time.Duration) bool {
	e.mu.Lock()
	last := e.lastFired[ruleID]
	now := e.now()
	e.mu.Unlock()
	return now.Sub(last) >= cooldown
}

func (e *Stage3AlertEvaluator) recordFired(ruleID string) {
	e.mu.Lock()
	e.lastFired[ruleID] = e.now()
	e.mu.Unlock()
}

func (e *Stage3AlertEvaluator) emitAndTrack(ruleID string, level AlertLevel, category, message string, metadata map[string]any) {
	e.monitor.Alert(level, category, message, metadata)
	e.recordFired(ruleID)
	if e.deps.OnAlertFired != nil {
		e.deps.OnAlertFired(ruleID, level, metadata)
	}
}

func (e *Stage3AlertEvaluator) evaluateDataStaleness() {
	if e.deps.ChannelLastDataAt == nil {
		return
	}
	now := e.now()
	for channel, lastData := range e.deps.ChannelLastDataAt() {
		age := now.Sub(lastData)
		if age > 6*time.Hour {
			if e.checkCooldown("data-staleness-critical", 1*time.Hour) {
				e.emitAndTrack("data-staleness-critical", AlertLevelCritical, "stage3_data_staleness",
					fmt.Sprintf("channel %s data is %.0f hours stale", channel, age.Hours()),
					map[string]any{"channel": channel, "hours": age.Hours(), "severity": "critical"})
			}
		} else if age > 2*time.Hour {
			if e.checkCooldown("data-staleness-warning", 1*time.Hour) {
				e.emitAndTrack("data-staleness-warning", AlertLevelWarning, "stage3_data_staleness",
					fmt.Sprintf("channel %s data is %.0f hours stale", channel, age.Hours()),
					map[string]any{"channel": channel, "hours": age.Hours(), "severity": "warning"})
			}
		}
	}
}

func (e *Stage3AlertEvaluator) evaluateEventCalendarSparse() {
	if e.deps.IsTradingDay == nil || e.deps.EventCalendarEventCount == nil {
		return
	}
	now := e.now().In(e.tz())
	if !e.deps.IsTradingDay(now) {
		return
	}
	count := e.deps.EventCalendarEventCount(now)
	if count >= 3 {
		return
	}
	if e.checkCooldown("event-calendar-sparse", 24*time.Hour) {
		e.emitAndTrack("event-calendar-sparse", AlertLevelWarning, "stage3_event_calendar",
			fmt.Sprintf("event calendar has %d events on trading day (expected >= 3)", count),
			map[string]any{"event_count": count, "date": now.Format("2006-01-02")})
	}
}

func (e *Stage3AlertEvaluator) evaluateModelConfidenceDegraded() {
	if e.deps.RecentEventFlowPredictions == nil {
		return
	}
	predictions := e.deps.RecentEventFlowPredictions(5)
	if len(predictions) < 5 {
		return
	}
	allNeutral := true
	for _, p := range predictions {
		if math.Abs(p-0.5) > 1e-6 {
			allNeutral = false
			break
		}
	}
	if !allNeutral {
		return
	}
	if e.checkCooldown("model-confidence-degraded", 24*time.Hour) {
		e.emitAndTrack("model-confidence-degraded", AlertLevelWarning, "stage3_model_confidence",
			"event flow prediction has been neutral for 5 consecutive days",
			map[string]any{"consecutive_neutral_days": len(predictions)})
	}
}

// predictionDriftWarmupThreshold is the minimum number of real (non-padded)
// historical predictions required before the prediction-drift alert can
// reason about a non-trivial standard deviation. At fewer than this many
// records the rule emits a dedicated "insufficient_history" alert instead
// of either silently skipping or issuing a spurious drift alert.
const predictionDriftWarmupThreshold = 5

func (e *Stage3AlertEvaluator) evaluatePredictionDrift() {
	if e.deps.LatestCapitalFlowPrediction == nil || e.deps.LatestCapitalFlowActual == nil {
		return
	}
	pred, ok := e.deps.LatestCapitalFlowPrediction()
	if !ok {
		return
	}
	actual, ok := e.deps.LatestCapitalFlowActual()
	if !ok {
		return
	}
	recent := e.deps.RecentEventFlowPredictions(10)

	// Production wiring provides this callback; tests that pre-date the
	// warmup addition leave it nil and pass through to the legacy path.
	if e.deps.RecentEventFlowPredictionsActualCount != nil {
		actualCount := e.deps.RecentEventFlowPredictionsActualCount(10)
		if actualCount < predictionDriftWarmupThreshold {
			if e.checkCooldown("prediction-drift-insufficient-history", 24*time.Hour) {
				e.emitAndTrack("prediction-drift-insufficient-history", AlertLevelInfo, "stage3_prediction_drift",
					fmt.Sprintf("prediction-drift suppressed: only %d real predictions in last 10 (need %d)", actualCount, predictionDriftWarmupThreshold),
					map[string]any{"actual_count": actualCount, "threshold": predictionDriftWarmupThreshold, "warmup": true})
			}
			return
		}
	}

	// Manifest #F01: drift is now a *direction* comparison (unit-agnostic).
	// The 2σ-on-raw-delta check is replaced by a single-day direction hit.
	// Windowed hit-rate analysis belongs to F03 (auto-calibrate) territory.
	_, std := meanStd(recent)
	_ = std // retained for diagnostic metadata; no longer gates emission
	if pred.Direction == actual.Direction {
		return
	}
	if e.checkCooldown("prediction-drift", 24*time.Hour) {
		e.emitAndTrack("prediction-drift", AlertLevelInfo, "stage3_prediction_drift",
			fmt.Sprintf("capital flow direction mismatch: predicted=%s (%.2f) actual=%s (%.2f)",
				pred.Direction, pred.Value, actual.Direction, actual.Value),
			map[string]any{
				"predicted_direction": pred.Direction, "predicted_value": pred.Value,
				"actual_direction": actual.Direction, "actual_value": actual.Value,
				"recent_std": std,
			})
	}
}

// meanStd returns the mean and population standard deviation of xs.
func meanStd(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var sq float64
	for _, x := range xs {
		d := x - mean
		sq += d * d
	}
	std := math.Sqrt(sq / float64(len(xs)))
	return mean, std
}
