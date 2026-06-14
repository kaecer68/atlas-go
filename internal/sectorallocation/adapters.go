package sectorallocation

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// =====================================================================
// Industry module adapters
// =====================================================================

// CycleAdapter implements CycleInputProvider by wrapping industry.CycleTracker
// and extracting a multiplier from CyclePosition.ContinuousPhaseScore (-1..1).
type CycleAdapter struct {
	tracker *industry.CycleTracker
}

// NewCycleAdapter returns a CycleAdapter backed by the given tracker.
func NewCycleAdapter(tracker *industry.CycleTracker) *CycleAdapter {
	return &CycleAdapter{tracker: tracker}
}

// GetCycleMultiplier maps ContinuousPhaseScore ∈ [-1, 1] → [0.8, 1.2].
// Default 1.0 (no adjustment) when the position is missing or uncalibrated.
func (a *CycleAdapter) GetCycleMultiplier(ctx context.Context, industryID string) (float64, error) {
	if a.tracker == nil {
		return 1.0, nil
	}
	pos, ok := a.tracker.GetPosition(industryID)
	if !ok || pos == nil {
		return 1.0, nil
	}
	if pos.Confidence <= 0.0 {
		return 1.0, nil
	}
	return 1.0 + (pos.ContinuousPhaseScore * 0.2), nil
}

// SeasonalAdapter implements SeasonalInputProvider by wrapping industry.SeasonalEngine.
type SeasonalAdapter struct {
	engine *industry.SeasonalEngine
}

// NewSeasonalAdapter returns a SeasonalAdapter backed by the given engine.
func NewSeasonalAdapter(engine *industry.SeasonalEngine) *SeasonalAdapter {
	return &SeasonalAdapter{engine: engine}
}

// GetSeasonalMultiplier returns industry.SeasonalEngine.GetPatternAdjustment directly.
func (a *SeasonalAdapter) GetSeasonalMultiplier(ctx context.Context, industryID string, now time.Time) (float64, error) {
	if a.engine == nil {
		return 1.0, nil
	}
	return a.engine.GetPatternAdjustment(industryID, now), nil
}

// GetActivePatternNames returns industry.SeasonalEngine.DetectCurrentPatterns names.
func (a *SeasonalAdapter) GetActivePatternNames(ctx context.Context, industryID string, now time.Time) []string {
	if a.engine == nil {
		return nil
	}
	patterns := a.engine.DetectCurrentPatterns(now)
	names := make([]string, 0, len(patterns))
	for _, p := range patterns {
		names = append(names, p.Name)
	}
	return names
}

// LinkageAdapter implements LinkageInputProvider by wrapping industry.LinkageAnalyzer
// and ShockPropagation. Extracts a multiplier from IndustryLinkageScore.SystemicImportance.
type LinkageAdapter struct {
	analyzer    *industry.LinkageAnalyzer
	propagation *industry.ShockPropagation
}

// NewLinkageAdapter returns a LinkageAdapter. propagation may be nil; the
// adapter falls back to LinkageAnalyzer.CalculateLinkageScore otherwise.
func NewLinkageAdapter(analyzer *industry.LinkageAnalyzer, propagation *industry.ShockPropagation) *LinkageAdapter {
	return &LinkageAdapter{analyzer: analyzer, propagation: propagation}
}

// GetLinkageMultiplier maps SystemicImportance ∈ [0, 1] → [1.0, 1.15].
// Default 1.0 when linkage is unknown.
func (a *LinkageAdapter) GetLinkageMultiplier(ctx context.Context, industryID string) (float64, error) {
	if a.propagation != nil {
		score := a.propagation.CalculateLinkageScore(industryID)
		if score != nil {
			return 1.0 + (score.SystemicImportance * 0.15), nil
		}
	}
	if a.analyzer != nil {
		score := a.analyzer.CalculateLinkageScore(industryID)
		if score != nil {
			return 1.0 + (score.SystemicImportance * 0.15), nil
		}
	}
	return 1.0, nil
}

// =====================================================================
// Narrative / Macro / Factor adapters — depend on cross-module engines that
// are constructed at wire-up time. The adapters expose a thin shim so the
// WeightEngine never has to know about the concrete engine types.
// =====================================================================

// NarrativeProviderFunc lets the caller wrap a function or method into a
// NarrativeInputProvider without depending on the narrative module's type.
type NarrativeProviderFunc func(ctx context.Context, industryID string) (multiplier float64, confidence float64, reason string, err error)

// GetNarrativeMultiplier implements NarrativeInputProvider.
func (f NarrativeProviderFunc) GetNarrativeMultiplier(ctx context.Context, industryID string) (float64, error) {
	m, _, _, err := f(ctx, industryID)
	return m, err
}

// NewNarrativeAdapter builds a NarrativeInputProvider from a function. The
// three return values from the function map directly to (multiplier, confidence, reason).
func NewNarrativeAdapter(fn func(ctx context.Context, industryID string) (float64, float64, string, error)) NarrativeInputProvider {
	if fn == nil {
		fn = func(_ context.Context, _ string) (float64, float64, string, error) {
			return 1.0, 0.0, "no-op", nil
		}
	}
	return narrativeShim{fn: fn}
}

type narrativeShim struct {
	fn func(ctx context.Context, industryID string) (float64, float64, string, error)
}

func (s narrativeShim) GetNarrativeMultiplier(ctx context.Context, industryID string) (float64, error) {
	m, _, _, err := s.fn(ctx, industryID)
	return m, err
}

// MacroProviderFunc adapts an arbitrary macro-tilt function to MacroInputProvider.
type MacroProviderFunc func(ctx context.Context, industryID, macroLevel, primaryFlow string) (float64, error)

// GetMacroTilt implements MacroInputProvider.
func (f MacroProviderFunc) GetMacroTilt(ctx context.Context, industryID, macroLevel, primaryFlow string) (float64, error) {
	return f(ctx, industryID, macroLevel, primaryFlow)
}

// GetMacroFlowDescription returns an empty string by default; the macro
// module's own description lives behind MacroProviderFunc.
func (f MacroProviderFunc) GetMacroFlowDescription(macroLevel, primaryFlow string) string {
	return ""
}

// FactorProviderFunc adapts a factor-tilt function to FactorInputProvider.
type FactorProviderFunc func(ctx context.Context, industryID string) (float64, error)

// GetFactorTilt implements FactorInputProvider.
func (f FactorProviderFunc) GetFactorTilt(ctx context.Context, industryID string) (float64, error) {
	return f(ctx, industryID)
}
