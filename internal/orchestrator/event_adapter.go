package orchestrator

// eventFlowAdapter is a generic adapter that bridges any external event
// flow predictor into the orchestrator's EventFlowPredictor interface.
// It is used by cmd/atlas wiring to inject event-driven predictions (F04).
type eventFlowAdapter struct {
	predictToday func() (string, float64)
}

func (a *eventFlowAdapter) PredictToday() (string, float64) {
	return a.predictToday()
}

// NewEventFlowAdapter wraps a predictToday function as an EventFlowPredictor.
// Usage in cmd/atlas:
//
//	sys.WithEventPredictor(orchestrator.NewEventFlowAdapter(func() (string, float64) {
//	    report := edPredictor.Predict(time.Now())
//	    if len(report.Predictions) == 0 {
//	        return "neutral", 0
//	    }
//	    return report.Predictions[0].Direction, report.Predictions[0].Confidence
//	}))
func NewEventFlowAdapter(predictToday func() (string, float64)) EventFlowPredictor {
	return &eventFlowAdapter{predictToday: predictToday}
}
