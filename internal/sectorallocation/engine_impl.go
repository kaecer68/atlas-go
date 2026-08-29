package sectorallocation

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
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
	prior     *StrategicSectorPrior
	projector *Projector
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

// NewEngineTestConfig 是測試 helper：回傳空白 SectorAllocationConfig（base weights 為空），
// 因為真實 production 改用 NewDefaultEngineWithProjector + StrategicPrior 路徑，
// ComputeProjectedTarget 不再依賴 cfg.BaseWeights。
func NewEngineTestConfig() config.SectorAllocationConfig {
	return config.SectorAllocationConfig{}
}

// LoadStrategicPriorFromConfigForTest 是測試 helper：直接從 default config 讀 strategic prior。
// spec §4.1：SA02 期間 source 鎖 heuristic、calibration_status 鎖 calibrating、model_version 鎖 semver。
func LoadStrategicPriorFromConfigForTest() *StrategicSectorPrior {
	cfg := config.GetParametersConfig()
	if cfg == nil {
		return &StrategicSectorPrior{}
	}
	prior, err := LoadStrategicPrior(cfg)
	if err != nil {
		return &StrategicSectorPrior{}
	}
	return prior
}

// NewDefaultEngineWithProjector 是 SA04 新介面：把 StrategicSectorPrior 與 Projector
// 注入 defaultEngine，啟用 ComputeProjectedTarget 唯一 projection 入口。
// 既有 ComputeWeights/ComputeWeight 仍可用（觀察向後相容）；SA11 promotion 之前，
// ComputeProjectedTarget 是唯一寫入 simulation state 的入口。
func NewDefaultEngineWithProjector(
	cfg config.SectorAllocationConfig,
	prior *StrategicSectorPrior,
	projector *Projector,
	cycle CycleInputProvider,
	seasonal SeasonalInputProvider,
	linkage LinkageInputProvider,
	narrative NarrativeInputProvider,
	macro MacroInputProvider,
	factor FactorInputProvider,
	weightMin, weightMax float64,
) WeightEngine {
	if projector == nil {
		projector = NewDefaultProjector()
	}
	return &defaultEngine{
		cfg:       cfg,
		prior:     prior,
		projector: projector,
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

// ComputeProjectedTarget（SA04 唯一 projection 入口）：
// 透過 Projector 與 StrategicSectorPrior 計算 final L1 target。
// 既有 ComputeWeights/ComputeWeight 維持不動；此方法是 SA-INV-01/04/05/07 守門。
func (e *defaultEngine) ComputeProjectedTarget(ctx context.Context, drivers DriverInputs) (ProjectedTarget, error) {
	if e.projector == nil {
		return ProjectedTarget{}, fmt.Errorf("WeightEngine: Projector not injected (SA04 必須用 NewDefaultEngineWithProjector)")
	}
	// Strategic prior 作為 base；nil prior 會被 Projector 拒絕（non L1 行為）。
	base := map[industry.SectorID]float64{}
	if e.prior != nil {
		maps.Copy(base, e.prior.Weights)
	}

	// 從 6 個 provider 收集 driver deltas（SA-INV-08 每個 driver 最多一次）。
	drivers.Cycle = collectCycleDeltas(ctx, e.cycle, drivers.Cycle)
	drivers.Seasonal = collectSeasonalDeltas(ctx, e.seasonal, drivers.Seasonal, drivers.AsOfTradingDate)
	drivers.Linkage = collectLinkageDeltas(ctx, e.linkage, drivers.Linkage)
	drivers.Narrative = collectNarrativeDeltas(ctx, e.narrative, drivers.Narrative)
	drivers.Macro = collectMacroDeltas(ctx, e.macro, drivers.Macro, drivers.MacroAction)
	drivers.CapitalFlow = collectFactorDeltas(ctx, e.factor, drivers.CapitalFlow)

	return e.projector.Project(base, drivers)
}

// collectCycleDeltas applies the cycle multiplier from p to each sector in in.
// If p is nil or the call fails, in is returned unchanged.
func collectCycleDeltas(ctx context.Context, p CycleInputProvider, in map[industry.SectorID]float64) map[industry.SectorID]float64 {
	if p == nil {
		return in
	}
	out := make(map[industry.SectorID]float64, len(in))
	for id, v := range in {
		m, err := p.GetCycleMultiplier(ctx, string(id))
		if err != nil || m <= 0 {
			out[id] = v
			continue
		}
		out[id] = v * m
	}
	return out
}

// collectSeasonalDeltas applies the seasonal multiplier from p to each sector in in.
// If p is nil or the call fails, in is returned unchanged.
func collectSeasonalDeltas(ctx context.Context, p SeasonalInputProvider, in map[industry.SectorID]float64, asOf string) map[industry.SectorID]float64 {
	if p == nil {
		return in
	}
	// Parse asOf into time.Time for GetSeasonalMultiplier.
	asOfDate, err := time.Parse("2006-01-02", asOf)
	if err != nil {
		return in
	}
	out := make(map[industry.SectorID]float64, len(in))
	for id, v := range in {
		m, err := p.GetSeasonalMultiplier(ctx, string(id), asOfDate)
		if err != nil || m <= 0 {
			out[id] = v
			continue
		}
		out[id] = v * m
	}
	return out
}

// collectLinkageDeltas applies the linkage multiplier from p to each sector in in.
// If p is nil or the call fails, in is returned unchanged.
func collectLinkageDeltas(ctx context.Context, p LinkageInputProvider, in map[industry.SectorID]float64) map[industry.SectorID]float64 {
	if p == nil {
		return in
	}
	out := make(map[industry.SectorID]float64, len(in))
	for id, v := range in {
		m, err := p.GetLinkageMultiplier(ctx, string(id))
		if err != nil || m <= 0 {
			out[id] = v
			continue
		}
		out[id] = v * m
	}
	return out
}

// collectNarrativeDeltas applies the narrative multiplier from p to each sector in in.
// If p is nil or the call fails, in is returned unchanged.
func collectNarrativeDeltas(ctx context.Context, p NarrativeInputProvider, in map[industry.SectorID]float64) map[industry.SectorID]float64 {
	if p == nil {
		return in
	}
	out := make(map[industry.SectorID]float64, len(in))
	for id, v := range in {
		m, err := p.GetNarrativeMultiplier(ctx, string(id))
		if err != nil || m <= 0 {
			out[id] = v
			continue
		}
		out[id] = v * m
	}
	return out
}

// collectMacroDeltas applies the macro tilt from p to each sector in in.
// MacroAction is passed to GetMacroTilt. If p is nil or the call fails, in is returned unchanged.
func collectMacroDeltas(ctx context.Context, p MacroInputProvider, in map[industry.SectorID]float64, action MacroAction) map[industry.SectorID]float64 {
	if p == nil {
		return in
	}
	out := make(map[industry.SectorID]float64, len(in))
	for id, v := range in {
		m, err := p.GetMacroTilt(ctx, string(id), string(action), "")
		if err != nil {
			out[id] = v
			continue
		}
		// macro tilt is additive: new = v * (1 + tilt)
		out[id] = v * (1 + m)
	}
	return out
}

// collectFactorDeltas applies the factor tilt from p to each sector in in.
// If p is nil or the call fails, in is returned unchanged.
func collectFactorDeltas(ctx context.Context, p FactorInputProvider, in map[industry.SectorID]float64) map[industry.SectorID]float64 {
	if p == nil {
		return in
	}
	out := make(map[industry.SectorID]float64, len(in))
	for id, v := range in {
		m, err := p.GetFactorTilt(ctx, string(id))
		if err != nil {
			out[id] = v
			continue
		}
		// factor tilt is additive: new = v * (1 + tilt)
		out[id] = v * (1 + m)
	}
	return out
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
