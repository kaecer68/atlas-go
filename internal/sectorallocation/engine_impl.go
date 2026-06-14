package sectorallocation

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// defaultEngine is the canonical WeightEngine implementation. It composes
// six input providers and applies the multi-factor formula:
//
//	adjusted = clamp(
//	    baseWeight
//	      × cycleMultiplier
//	      × seasonalMultiplier
//	      × linkageMultiplier
//	      × narrativeMultiplier
//	      × (1 + macroTilt)
//	      × (1 + factorTilt),
//	    weightFloor, darwinian.WeightMax * baseWeight,
//	)
//
// Each provider may return an error; the engine logs via the returned
// adjustment log but does not fail the whole computation. Missing
// providers default to neutral (1.0 / 0.0).
type defaultEngine struct {
	cfg       config.SectorAllocationConfig
	cycle     CycleInputProvider
	seasonal  SeasonalInputProvider
	linkage   LinkageInputProvider
	narrative NarrativeInputProvider
	macro     MacroInputProvider
	factor    FactorInputProvider
	weightMin float64
	weightMax float64
}

// NewDefaultEngine returns a WeightEngine wired to the given config and
// input providers. weightMin/weightMax are the Darwinian clipping bounds
// (typically 0.3 and 2.5). Missing providers are tolerated.
func NewDefaultEngine(
	cfg config.SectorAllocationConfig,
	cycle CycleInputProvider,
	seasonal SeasonalInputProvider,
	linkage LinkageInputProvider,
	narrative NarrativeInputProvider,
	macro MacroInputProvider,
	factor FactorInputProvider,
	weightMin, weightMax float64,
) WeightEngine {
	return &defaultEngine{
		cfg:       cfg,
		cycle:     cycle,
		seasonal:  seasonal,
		linkage:   linkage,
		narrative: narrative,
		macro:     macro,
		factor:    factor,
		weightMin: weightMin,
		weightMax: weightMax,
	}
}

// ComputeWeights iterates over cfg.BaseWeights and applies the formula
// for each industry. The returned slice is sorted by AdjustedWeight desc.
func (e *defaultEngine) ComputeWeights(ctx context.Context, now time.Time) ([]SectorWeight, error) {
	if len(e.cfg.BaseWeights) == 0 {
		return nil, fmt.Errorf("sector_allocation.base_weights is empty")
	}
	out := make([]SectorWeight, 0, len(e.cfg.BaseWeights))
	for id, base := range e.cfg.BaseWeights {
		w, err := e.ComputeWeight(ctx, id, now)
		if err != nil {
			return nil, fmt.Errorf("compute %s: %w", id, err)
		}
		w.BaseWeight = base
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AdjustedWeight > out[j].AdjustedWeight
	})
	return out, nil
}

// ComputeWeight applies the full multi-factor formula for a single industry.
func (e *defaultEngine) ComputeWeight(ctx context.Context, industryID string, now time.Time) (*SectorWeight, error) {
	base, ok := e.cfg.BaseWeights[industryID]
	if !ok {
		return nil, fmt.Errorf("industry %q has no base_weight", industryID)
	}

	log := make([]string, 0, 12)
	appendLog := func(name string, v float64) {
		log = append(log, fmt.Sprintf("%s=%.4f", name, v))
	}

	appendLog("base", base)

	cycle, err := safeGetCycle(ctx, e.cycle, industryID)
	if err != nil {
		return nil, fmt.Errorf("cycle: %w", err)
	}
	appendLog("cycle", cycle)

	seasonal, err := safeGetSeasonal(ctx, e.seasonal, industryID, now)
	if err != nil {
		return nil, fmt.Errorf("seasonal: %w", err)
	}
	appendLog("seasonal", seasonal)

	linkage, err := safeGetLinkage(ctx, e.linkage, industryID)
	if err != nil {
		return nil, fmt.Errorf("linkage: %w", err)
	}
	appendLog("linkage", linkage)

	narrative, err := safeGetNarrative(ctx, e.narrative, industryID)
	if err != nil {
		return nil, fmt.Errorf("narrative: %w", err)
	}
	appendLog("narrative", narrative)

	macroTilt, err := safeGetMacroTilt(ctx, e.macro, industryID, "", "")
	if err != nil {
		return nil, fmt.Errorf("macro: %w", err)
	}
	appendLog("macro_tilt", macroTilt)

	factorTilt, err := safeGetFactorTilt(ctx, e.factor, industryID)
	if err != nil {
		return nil, fmt.Errorf("factor: %w", err)
	}
	appendLog("factor_tilt", factorTilt)

	multiplier := 1.0
	multiplier *= cycle * e.cfg.CycleWeight
	multiplier *= seasonal * e.cfg.SeasonalWeight
	multiplier *= linkage * e.cfg.LinkageWeight
	multiplier *= narrative * e.cfg.NarrativeWeight
	multiplier *= (1.0 + macroTilt) * e.cfg.MacroWeight
	multiplier *= (1.0 + factorTilt) * e.cfg.FactorWeight
	appendLog("raw_multiplier", multiplier)

	darwinianMultiplier := clamp(multiplier, e.weightMin, e.weightMax)
	appendLog("darwinian_multiplier", darwinianMultiplier)

	adjusted := base * darwinianMultiplier
	appendLog("raw_adjusted", adjusted)

	finalFloor := base * e.cfg.WeightFloor
	if finalFloor < 0.005 {
		finalFloor = 0.005
	}
	clamped := adjusted
	if clamped < finalFloor {
		clamped = finalFloor
	}
	appendLog("clamped", clamped)

	derivationFactors := e.cfg.DerivationFactors[industryID]
	derivation := make([]WeightFactor, 0, len(derivationFactors))
	for _, d := range derivationFactors {
		derivation = append(derivation, WeightFactor{
			Factor:       d.Factor,
			Contribution: d.Weight,
			Source:       d.Source,
			Evidence:     d.Evidence,
		})
	}

	return &SectorWeight{
		ID:                industryID,
		BaseWeight:        base,
		AdjustedWeight:    clamped,
		DerivationFactors: derivation,
		AdjustmentLog:     log,
	}, nil
}

func safeGetCycle(ctx context.Context, p CycleInputProvider, id string) (float64, error) {
	if p == nil {
		return 1.0, nil
	}
	v, err := p.GetCycleMultiplier(ctx, id)
	if err != nil || v <= 0 {
		return 1.0, err
	}
	return v, nil
}

func safeGetSeasonal(ctx context.Context, p SeasonalInputProvider, id string, now time.Time) (float64, error) {
	if p == nil {
		return 1.0, nil
	}
	v, err := p.GetSeasonalMultiplier(ctx, id, now)
	if err != nil || v <= 0 {
		return 1.0, err
	}
	return v, nil
}

func safeGetLinkage(ctx context.Context, p LinkageInputProvider, id string) (float64, error) {
	if p == nil {
		return 1.0, nil
	}
	v, err := p.GetLinkageMultiplier(ctx, id)
	if err != nil || v <= 0 {
		return 1.0, err
	}
	return v, nil
}

func safeGetNarrative(ctx context.Context, p NarrativeInputProvider, id string) (float64, error) {
	if p == nil {
		return 1.0, nil
	}
	v, err := p.GetNarrativeMultiplier(ctx, id)
	if err != nil || v <= 0 {
		return 1.0, err
	}
	return v, nil
}

func safeGetMacroTilt(ctx context.Context, p MacroInputProvider, id, macroLevel, primaryFlow string) (float64, error) {
	if p == nil {
		return 0.0, nil
	}
	v, err := p.GetMacroTilt(ctx, id, macroLevel, primaryFlow)
	return v, err
}

func safeGetFactorTilt(ctx context.Context, p FactorInputProvider, id string) (float64, error) {
	if p == nil {
		return 0.0, nil
	}
	v, err := p.GetFactorTilt(ctx, id)
	return v, err
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
