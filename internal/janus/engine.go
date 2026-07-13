package janus

import (
	"fmt"
	"maps"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/prism"
)

// Engine is the central JANUS meta-layer that tracks PRISM cohort performance,
// computes dynamic weights, and emits emergent regime classifications.
type Engine struct {
	tracker     *CohortPerformanceTracker
	calculator  *CohortWeightCalculator
	detector    *RegimeDetector
	config      JANUSConfig
	mu          sync.RWMutex
	lastWeights map[prism.RegimeType]CohortWeight
	lastClass   RegimeClassification
	lastUpdated time.Time

	// compositeScore: synthesized from macro when no PRISM Sharpe data.
	compositeScore float64
	// hasSynthetic tracks whether UpdateFromMacro was called, so
	// GetCurrentRegimeScore can distinguish "no synthesis attempted"
	// from "synthesis returned zero".
	hasSynthetic bool
}

// NewEngine creates a JANUS engine with default configuration.
func NewEngine() *Engine {
	cfg := DefaultJANUSConfig()
	return NewEngineWithConfig(cfg)
}

// NewEngineWithConfig creates a JANUS engine with custom configuration.
func NewEngineWithConfig(config JANUSConfig) *Engine {
	return &Engine{
		tracker:     NewCohortPerformanceTracker(90),
		calculator:  NewCohortWeightCalculator(config),
		detector:    NewRegimeDetector(config),
		config:      config,
		lastWeights: make(map[prism.RegimeType]CohortWeight),
		lastClass:   MixedRegime,
	}
}

// RecordTrainingResult records a PRISM training result for the specified cohort.
func (e *Engine) RecordTrainingResult(regime prism.RegimeType, result prism.TrainingResult) {
	snapshot := CohortSnapshot{
		Regime:      regime,
		SharpeRatio: result.SharpeRatio,
		HitRate:     result.HitRate,
		TotalReturn: result.TotalReturn,
		Signals:     result.SignalsCount,
		RecordedAt:  time.Now(),
	}
	e.tracker.RecordSnapshot(snapshot)
}

// RecordSnapshot records a raw cohort snapshot directly.
func (e *Engine) RecordSnapshot(snapshot CohortSnapshot) {
	e.tracker.RecordSnapshot(snapshot)
}

// Update recomputes weights and regime classification based on the latest tracked data.
func (e *Engine) Update() {
	e.mu.Lock()
	defer e.mu.Unlock()

	perf := e.tracker.GetPerformance()
	if len(perf) == 0 {
		return
	}

	// Compute blended weights (default JANUS output).
	weights := e.calculator.CalculateWeights(perf)

	// Compute short-only and long-only weights for regime detection.
	shortWeights := e.calculator.CalculateWindowWeights(perf, WindowShort)
	longWeights := e.calculator.CalculateWindowWeights(perf, WindowLong)

	classification := e.detector.Detect(shortWeights, longWeights)

	e.lastWeights = weights
	e.lastClass = classification
	e.lastUpdated = time.Now()
}

// UpdateFromMacro stores a composite score synthesized from macro signals.
// Used as fallback when PRISM training results (RecordTrainingResult) are
// not available in production, so regime history sessions can report a
// meaningful score reflecting current market state.
func (e *Engine) UpdateFromMacro(snap marketdata.MacroDataSnapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.compositeScore = synthesizeCompositeScore(snap)
	e.hasSynthetic = true
	e.lastUpdated = time.Now()
}

// GetCompositeScore returns the macro-synthesized score, or 0 if
// UpdateFromMacro was never called.
func (e *Engine) GetCompositeScore() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.compositeScore
}

// GetCurrentRegimeScore returns the best available regime score:
// real Sharpe (averaged across cohorts from PRISM tracker) when at least
// one cohort has non-zero Sharpe, otherwise macro-synthesized fallback
// (only if UpdateFromMacro was ever called). Returns (score, isSynthetic).
// isSynthetic=true means the score is macro-derived, not from PRISM training.
func (e *Engine) GetCurrentRegimeScore() (float64, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if score, ok := e.realRegimeScoreLocked(); ok {
		return score, false
	}
	if e.hasSynthetic {
		return e.compositeScore, true
	}
	return 0, false
}

// realRegimeScoreLocked returns average Sharpe across cohorts when at least one
// cohort has non-zero Sharpe. EnsureAllRegimes seeds zero-Sharpe snapshots; we
// must skip those to avoid returning a synthetic-feeling 0. Caller must hold
// e.mu (read or write).
func (e *Engine) realRegimeScoreLocked() (float64, bool) {
	perf := e.tracker.GetPerformance()
	var sum float64
	count := 0
	for _, p := range perf {
		if p != nil && p.ShortWindow != nil && p.ShortWindow.SharpeRatio != 0 {
			sum += p.ShortWindow.SharpeRatio
			count++
		}
	}
	if count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

// synthesizeCompositeScore maps macro signals to a ~[-95, +30] score:
//
//	tanh(foreignFlow / 5) * 30            — continuous Taiwan regime signal
//	-max(0, VIX - 20) * 1.5                — panic penalty above baseline 20
//
// foreignFlow is in NTD billions (TWSE daily reports convention):
// ±5B NTD saturates at ±30, so a typical ±1B day yields ~7.
//
// Oracle review notes:
//   - foreign flow as tanh preserves magnitude info that step function loses
//     (±1B vs ±100B NTD now distinguishable). Scale 5B saturates at ±30.
//   - VIX coefficient 1.5 not 0.5: VIX 40 (= -30) matches foreign ±5B magnitude,
//     giving 1:1 balance per Oracle recommendation.
//   - Range asymmetry (-95 to +30) is intentional: downside risk dominates
//     Taiwan equity regime in historical drawdowns (Chiao et al. 2006).
func synthesizeCompositeScore(snap marketdata.MacroDataSnapshot) float64 {
	vixBaseline := 20.0
	if snap.VIXBaseline > 0 {
		vixBaseline = snap.VIXBaseline
	}
	score := math.Tanh(snap.ForeignInvestorNet.Value/5) * 30
	if snap.VIX.Value > vixBaseline {
		score -= (snap.VIX.Value - vixBaseline) * 1.5
	}
	return score
}

// GetCohortWeights returns the most recently computed JANUS weights.
func (e *Engine) GetCohortWeights() map[prism.RegimeType]CohortWeight {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Defensive copy.
	out := make(map[prism.RegimeType]CohortWeight, len(e.lastWeights))
	maps.Copy(out, e.lastWeights)
	return out
}

// GetRegimeClassification returns the latest emergent regime signal.
func (e *Engine) GetRegimeClassification() RegimeClassification {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastClass
}

// GetStatus returns a serializable snapshot of the current JANUS state.
func (e *Engine) GetStatus() Status {
	e.mu.RLock()
	defer e.mu.RUnlock()

	weights := make(map[string]float64, len(e.lastWeights))
	for regime, cw := range e.lastWeights {
		weights[regime.String()] = cw.Weight
	}

	perf := e.tracker.GetPerformance()
	perfMap := make(map[string]WindowPerformanceSnapshot, len(perf))
	for regime, p := range perf {
		perfMap[regime.String()] = WindowPerformanceSnapshot{
			ShortSharpe: cull(p.ShortWindow),
			MedSharpe:   cull(p.MedWindow),
			LongSharpe:  cull(p.LongWindow),
		}
	}

	return Status{
		Weights:            weights,
		Classification:     string(e.lastClass),
		LastUpdated:        e.lastUpdated,
		WindowPerformances: perfMap,
	}
}

// ApplyAdjustment scales recommendation conviction by the active JANUS weight
// of the cohort that most closely matches the current market regime.
//
// If no weights have been computed yet, recommendations are returned unchanged.
func (e *Engine) ApplyAdjustment(
	recommendations []domain.Recommendation,
	currentRegime domain.Regime,
) []domain.Recommendation {
	e.mu.RLock()
	weights := e.lastWeights
	e.mu.RUnlock()

	if len(weights) == 0 {
		return recommendations
	}

	// Map domain.Regime to the closest prism.RegimeType to resolve a cohort weight.
	targetRegime := mapDomainRegimeToPRISM(currentRegime)
	cw, ok := weights[targetRegime]
	if !ok {
		// Fallback to equal scaling if the specific regime cohort is missing.
		return recommendations
	}

	adjusted := make([]domain.Recommendation, len(recommendations))
	for i, rec := range recommendations {
		adj := rec
		// Scale conviction by cohort weight relative to neutral (1.0 / cohortCount).
		// With 5 cohorts, neutral is 0.20. Weight > 0.20 => boost, < 0.20 => reduce.
		// We use a gentler scaling: 1.0 + (weight - neutral) so that weight 0.30 => 1.10x.
		neutral := 1.0 / float64(len(weights))
		scale := 1.0 + (cw.Weight - neutral)
		adj.Conviction = max(min(int(float64(adj.Conviction)*scale), 100), 0)
		adjusted[i] = adj
	}
	return adjusted
}

// Status is a serializable view of the JANUS engine state.
type Status struct {
	Weights            map[string]float64                   `json:"weights"`
	Classification     string                               `json:"classification"`
	LastUpdated        time.Time                            `json:"last_updated"`
	WindowPerformances map[string]WindowPerformanceSnapshot `json:"window_performances"`
}

// WindowPerformanceSnapshot exposes Sharpe values per window for reporting.
type WindowPerformanceSnapshot struct {
	ShortSharpe float64 `json:"short_sharpe,omitempty"`
	MedSharpe   float64 `json:"med_sharpe,omitempty"`
	LongSharpe  float64 `json:"long_sharpe,omitempty"`
}

func cull(wp *WindowPerformance) float64 {
	if wp == nil {
		return 0
	}
	return wp.SharpeRatio
}

func mapDomainRegimeToPRISM(r domain.Regime) prism.RegimeType {
	switch r {
	case domain.RegimeRiskOn:
		return prism.RegimeRiskOn
	case domain.RegimeRiskOff:
		return prism.RegimeRiskOff
	case domain.RegimeNeutral:
		// Neutral maps to Low-Volatility as the closest stable regime.
		return prism.RegimeLowVolatility
	default:
		return prism.RegimeTransition
	}
}

// EnsureAllRegimes initializes tracker slots for every PRISM regime so that
// weight calculations produce entries even before data arrives.
func (e *Engine) EnsureAllRegimes() {
	for i := range int(prism.RegimeCount) {
		regime := prism.RegimeType(i)
		// Inject a neutral zero snapshot so the regime appears in performance maps.
		e.tracker.RecordSnapshot(CohortSnapshot{
			Regime:      regime,
			SharpeRatio: 0,
			HitRate:     0.5,
			TotalReturn: 0,
			Signals:     0,
			RecordedAt:  time.Now(),
		})
	}
}

// String returns a human-readable summary of the current JANUS state.
func (e *Engine) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return fmt.Sprintf("JANUS[class=%s weights=%+v updated=%s]",
		e.lastClass, e.lastWeights, e.lastUpdated.Format(time.RFC3339))
}

// JANUSHealthStatus represents the health assessment of the JANUS engine.
type JANUSHealthStatus struct {
	Initialized     bool    `json:"initialized"`
	RegimeClass     string  `json:"regime_class"`
	Confidence      float64 `json:"confidence"`
	CohortCount     int     `json:"cohort_count"`
	LastUpdatedAgoH float64 `json:"last_updated_ago_hours"`
}

// HealthStatus returns a health assessment of the JANUS engine suitable for monitoring.
func (e *Engine) HealthStatus() JANUSHealthStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hs := JANUSHealthStatus{
		Initialized: !e.lastUpdated.IsZero(),
		RegimeClass: string(e.lastClass),
		CohortCount: len(e.lastWeights),
	}
	if !e.lastUpdated.IsZero() {
		hs.LastUpdatedAgoH = time.Since(e.lastUpdated).Hours()
	}
	if len(e.lastWeights) > 0 {
		totalConf := 0.0
		for _, cw := range e.lastWeights {
			totalConf += cw.Weight
		}
		hs.Confidence = totalConf / float64(len(e.lastWeights))
	}
	return hs
}

// RecordHealthTo writes the current JANUS health status to a channel health store.
func (e *Engine) RecordHealthTo(store ChannelHealthRecorder) {
	hs := e.HealthStatus()
	if !hs.Initialized {
		store.Record("janus_regime", "error", "JANUS engine not yet updated")
		return
	}
	if hs.LastUpdatedAgoH > 168 {
		store.Record("janus_regime", "error", fmt.Sprintf("JANUS data stale: %.1f hours old", hs.LastUpdatedAgoH))
		return
	}
	if hs.LastUpdatedAgoH > 48 {
		store.Record("janus_regime", "warn", fmt.Sprintf("JANUS data aging: %.1f hours old", hs.LastUpdatedAgoH))
		return
	}
	store.Record("janus_regime", "ok", "")
}

// ChannelHealthRecorder matches the subset of the monitoring health store interface JANUS needs.
type ChannelHealthRecorder interface {
	Record(channelID, status, message string)
}
