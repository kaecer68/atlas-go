package recommender

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// =====================================================================
// Adapters — wrap real producer types to satisfy the consumer-defined
// interfaces in deps.go. Each adapter is NIL-safe: wrapping nil returns
// zero values with no nil deref, so production wiring can use
// NewHandlerWithServices(svc, ...) and any unset service degrades
// gracefully to the fallback hardcoded values in handler.go.
// =====================================================================

// =====================================================================
// NarrativeAdapter wraps *monitoring/service.NarrativeService.
// =====================================================================

// NewNarrativeAdapterFunc wraps getter functions to satisfy the
// NarrativeProvider interface — useful for tests and decoupled wiring.
func NewNarrativeAdapterFunc(
	getStress func() narrative.TaiwanStressIndex,
	getNarrative func(context.Context) (narrative.MarketNarrativeData, error),
) NarrativeProvider {
	if getStress == nil {
		getStress = func() narrative.TaiwanStressIndex { return narrative.TaiwanStressIndex{} }
	}
	if getNarrative == nil {
		getNarrative = func(context.Context) (narrative.MarketNarrativeData, error) {
			return narrative.MarketNarrativeData{}, nil
		}
	}
	return &narrativeAdapter{getStress: getStress, getNarrative: getNarrative}
}

type narrativeAdapter struct {
	getStress    func() narrative.TaiwanStressIndex
	getNarrative func(context.Context) (narrative.MarketNarrativeData, error)
}

func (a *narrativeAdapter) GetCurrentStressIndex() narrative.TaiwanStressIndex {
	return a.getStress()
}

func (a *narrativeAdapter) BuildMarketNarrativeData(ctx context.Context) (narrative.MarketNarrativeData, error) {
	return a.getNarrative(ctx)
}

// =====================================================================
// CapitalFlowAdapter wraps *capitalflow.Service (added in 661f2dc7).
// =====================================================================

type capitalFlowServiceProvider interface {
	LatestDaily(ctx context.Context) (capitalflow.DailyReport, error)
	Summary(ctx context.Context) (capitalflow.SummaryReport, error)
	LatestAssessment(ctx context.Context) (capitalflow.CapitalFlowAssessment, error)
}

// NewCapitalFlowAdapter wires a *capitalflow.Service into
// CapitalFlowProvider. The provider may be nil; in that case the
// adapter degrades to the "資金流向均衡" fallback for all reports.
func NewCapitalFlowAdapter(provider capitalFlowServiceProvider) CapitalFlowProvider {
	if provider == nil {
		return NewCapitalFlowFunc(nil, nil, nil)
	}
	return NewCapitalFlowFunc(provider.LatestDaily, provider.Summary, provider.LatestAssessment)
}

// NewCapitalFlowFunc wraps LatestDaily, Summary, and LatestAssessment
// functions. Any function may be nil — the adapter substitutes zero-value
// fallbacks so callers can opt into one report without wiring the others.
func NewCapitalFlowFunc(
	latestDaily func(context.Context) (capitalflow.DailyReport, error),
	summary func(context.Context) (capitalflow.SummaryReport, error),
	latestAssessment func(context.Context) (capitalflow.CapitalFlowAssessment, error),
) CapitalFlowProvider {
	if latestDaily == nil {
		latestDaily = func(context.Context) (capitalflow.DailyReport, error) {
			return capitalflow.DailyReport{}, nil
		}
	}
	if summary == nil {
		summary = func(context.Context) (capitalflow.SummaryReport, error) {
			return capitalflow.SummaryReport{}, nil
		}
	}
	if latestAssessment == nil {
		latestAssessment = func(context.Context) (capitalflow.CapitalFlowAssessment, error) {
			return capitalflow.CapitalFlowAssessment{}, nil
		}
	}
	return &capitalFlowAdapter{
		latestDaily:      latestDaily,
		summary:          summary,
		latestAssessment: latestAssessment,
	}
}

type capitalFlowAdapter struct {
	latestDaily      func(context.Context) (capitalflow.DailyReport, error)
	summary          func(context.Context) (capitalflow.SummaryReport, error)
	latestAssessment func(context.Context) (capitalflow.CapitalFlowAssessment, error)
}

func (a *capitalFlowAdapter) LatestDaily(ctx context.Context) (capitalflow.DailyReport, error) {
	return a.latestDaily(ctx)
}

func (a *capitalFlowAdapter) Summary(ctx context.Context) (capitalflow.SummaryReport, error) {
	return a.summary(ctx)
}

func (a *capitalFlowAdapter) LatestAssessment(ctx context.Context) (capitalflow.CapitalFlowAssessment, error) {
	return a.latestAssessment(ctx)
}

// =====================================================================
// EventPredictorAdapter wraps *eventdriven.Predictor.
// =====================================================================

type eventDrivenPredictorProvider interface {
	Predict(now time.Time) eventdriven.PredictionReport
}

// NewEventPredictorAdapter wires a *eventdriven.Predictor into
// EventPredictor. PredictToday() takes the first daily prediction
// from the 5-day forward report and labels it "today" for display.
func NewEventPredictorAdapter(predictor eventDrivenPredictorProvider) EventPredictor {
	return &eventPredictorAdapter{predictor: predictor}
}

type eventPredictorAdapter struct {
	predictor eventDrivenPredictorProvider
	cacheMu   sync.RWMutex
	cacheKey  string // "YYYY-MM-DD"
	cacheRep  *eventdriven.PredictionReport
	cacheAt   time.Time
}

const predictTodayCacheTTL = 60 * time.Second

func (a *eventPredictorAdapter) PredictToday() (eventdriven.FlowPrediction, error) {
	if a.predictor == nil {
		return eventdriven.FlowPrediction{}, nil
	}
	report := a.cachedReport()
	if len(report.Predictions) == 0 {
		return eventdriven.FlowPrediction{}, nil
	}
	return report.Predictions[0], nil
}

func (a *eventPredictorAdapter) NextNDays(n int) ([]eventdriven.FlowPrediction, error) {
	if a.predictor == nil {
		return nil, nil
	}
	report := a.cachedReport()
	if n > len(report.Predictions) {
		n = len(report.Predictions)
	}
	return report.Predictions[:n], nil
}

// cachedReport returns a PredictionReport, caching by date within predictTodayCacheTTL.
func (a *eventPredictorAdapter) cachedReport() eventdriven.PredictionReport {
	now := time.Now()
	key := now.Format("2006-01-02")

	a.cacheMu.RLock()
	if a.cacheRep != nil && a.cacheKey == key && time.Since(a.cacheAt) < predictTodayCacheTTL {
		rep := *a.cacheRep
		a.cacheMu.RUnlock()
		return rep
	}
	a.cacheMu.RUnlock()

	report := a.predictor.Predict(now)

	a.cacheMu.Lock()
	a.cacheRep = &report
	a.cacheKey = key
	a.cacheAt = time.Now()
	a.cacheMu.Unlock()
	return report
}

// =====================================================================
// ComparisonEngineAdapter wraps *strategy.ComparisonEngine.
// =====================================================================

// NewComparisonEngineAdapter wires a strategy.ComparisonEngine into
// the ComparisonEngine interface. Internally holds a getter function
// to decouple from the real producer's package import.
type comparisonEngineProvider interface {
	GetScore(strategyID string, days int) (float64, error)
	RankedIDs() ([]string, error)
}

func NewComparisonEngineAdapter(provider comparisonEngineProvider) ComparisonEngine {
	if provider == nil {
		return NewComparisonEngineFunc(nil, nil)
	}
	return NewComparisonEngineFunc(
		func(strategyID string) (float64, error) {
			return provider.GetScore(strategyID, 30)
		},
		func() ([]string, error) {
			return provider.RankedIDs()
		},
	)
}

// NewComparisonEngineFunc wraps GetScore and RankedIDs functions.
func NewComparisonEngineFunc(getScore func(string) (float64, error), rankedIDs func() ([]string, error)) ComparisonEngine {
	if getScore == nil {
		getScore = func(string) (float64, error) { return 0, nil }
	}
	if rankedIDs == nil {
		rankedIDs = func() ([]string, error) { return nil, nil }
	}
	return &comparisonEngineAdapter{getScore: getScore, rankedIDs: rankedIDs}
}

type comparisonEngineAdapter struct {
	getScore  func(string) (float64, error)
	rankedIDs func() ([]string, error)
}

func (a *comparisonEngineAdapter) GetScore(strategyID string) (float64, error) {
	return a.getScore(strategyID)
}

func (a *comparisonEngineAdapter) RankedStrategies() ([]string, error) {
	return a.rankedIDs()
}

// =====================================================================
// Compile-time assertions (caught at build time if a wrapper drifts
// from its declared interface).
// =====================================================================

var (
	_ NarrativeProvider   = (*narrativeAdapter)(nil)
	_ CapitalFlowProvider = (*capitalFlowAdapter)(nil)
	_ EventPredictor      = (*eventPredictorAdapter)(nil)
	_ ComparisonEngine    = (*comparisonEngineAdapter)(nil)
)
