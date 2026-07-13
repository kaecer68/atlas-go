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

	// LatestCapitalFlowPrediction returns the most recent capital-flow
	// prediction value. The bool is false if no prediction is available.
	LatestCapitalFlowPrediction func() (float64, bool)

	// LatestCapitalFlowActual returns the most recent capital-flow actual
	// value. The bool is false if no actual value is available.
	LatestCapitalFlowActual func() (float64, bool)
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
// every 10 minutes.
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

func (e *Stage3AlertEvaluator) evaluateDataStaleness() {
	if e.deps.ChannelLastDataAt == nil {
		return
	}
	now := e.now()
	for channel, lastData := range e.deps.ChannelLastDataAt() {
		age := now.Sub(lastData)
		if age > 6*time.Hour {
			if e.checkCooldown("data-staleness-critical", 1*time.Hour) {
				e.monitor.Alert(AlertLevelCritical, "stage3_data_staleness",
					fmt.Sprintf("channel %s data is %.0f hours stale", channel, age.Hours()),
					map[string]any{"channel": channel, "hours": age.Hours(), "severity": "critical"})
				e.recordFired("data-staleness-critical")
			}
		} else if age > 2*time.Hour {
			if e.checkCooldown("data-staleness-warning", 1*time.Hour) {
				e.monitor.Alert(AlertLevelWarning, "stage3_data_staleness",
					fmt.Sprintf("channel %s data is %.0f hours stale", channel, age.Hours()),
					map[string]any{"channel": channel, "hours": age.Hours(), "severity": "warning"})
				e.recordFired("data-staleness-warning")
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
		e.monitor.Alert(AlertLevelWarning, "stage3_event_calendar",
			fmt.Sprintf("event calendar has %d events on trading day (expected >= 3)", count),
			map[string]any{"event_count": count, "date": now.Format("2006-01-02")})
		e.recordFired("event-calendar-sparse")
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
		e.monitor.Alert(AlertLevelWarning, "stage3_model_confidence",
			"event flow prediction has been neutral for 5 consecutive days",
			map[string]any{"consecutive_neutral_days": len(predictions)})
		e.recordFired("model-confidence-degraded")
	}
}

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
	// Recent predictions are used to estimate a simple standard deviation.
	recent := e.deps.RecentEventFlowPredictions(10)
	_, std := meanStd(recent)
	if std <= 0 {
		return
	}
	diff := math.Abs(actual - pred)
	if diff <= 2*std {
		return
	}
	if e.checkCooldown("prediction-drift", 24*time.Hour) {
		e.monitor.Alert(AlertLevelInfo, "stage3_prediction_drift",
			fmt.Sprintf("capital flow actual %.2f vs prediction %.2f exceeds 2σ (σ=%.2f)", actual, pred, std),
			map[string]any{"actual": actual, "prediction": pred, "std": std, "diff": diff})
		e.recordFired("prediction-drift")
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
