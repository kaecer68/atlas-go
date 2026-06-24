package portfolio

import (
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// NewFactorEngine constructs a FactorEngine with default runtime parameters.
func NewFactorEngine() *FactorEngine {
	return &FactorEngine{
		params: DefaultRuntimeParameters(),
	}
}

// WithHistoricalPrices attaches a historical price repository for momentum calc.
func (fe *FactorEngine) WithHistoricalPrices(hp *HistoricalPrices) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.history = hp
	return fe
}

// WithFundamentalProvider attaches a fundamental data provider.
func (fe *FactorEngine) WithFundamentalProvider(fp *FundamentalProvider) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.fundamentals = fp
	return fe
}

// WithParameters attaches runtime parameters.
func (fe *FactorEngine) WithParameters(p *RuntimeParameters) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.params = p
	return fe
}

// WithNarrativeProvider attaches a narrative factor provider.
func (fe *FactorEngine) WithNarrativeProvider(fn NarrativeProviderFunc) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.narrativeProv = fn
	return fe
}

// WithIndustryCycleProvider attaches an industry cycle factor provider.
func (fe *FactorEngine) WithIndustryCycleProvider(fn IndustryCycleProviderFunc) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.cycleProv = fn
	return fe
}

// WithLinkageProvider attaches a linkage factor provider.
func (fe *FactorEngine) WithLinkageProvider(fn LinkageProviderFunc) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.linkageProv = fn
	return fe
}

// WithTSMCProvider attaches a TSMC factor provider.
func (fe *FactorEngine) WithTSMCProvider(fn TSMCProviderFunc) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.tsmcProv = fn
	return fe
}

// WithCorporateActionProvider attaches a corporate action provider and initializes
// the adjustment cache (adjustedSymbols map and adjustmentTTL = 24h) on first call.
// Subsequent calls retain existing cache state.
func (fe *FactorEngine) WithCorporateActionProvider(p CorporateActionProvider) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.corpActions = p
	if fe.adjustedSymbols == nil {
		fe.adjustedSymbols = make(map[string]time.Time)
	}
	if fe.adjustmentTTL == 0 {
		fe.adjustmentTTL = 24 * time.Hour
	}
	return fe
}

// SetCycleTracker replaces the industry cycle provider to use a shared CycleTracker.
func (fe *FactorEngine) SetCycleTracker(ct *industry.CycleTracker) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.cycleProv = func(symbol string) *domain.IndustryCycleFactorScore {
		id := classifyIndustry(symbol)
		if id == "" {
			return nil
		}
		pos, ok := ct.GetPosition(id)
		if !ok {
			return nil
		}
		return &domain.IndustryCycleFactorScore{
			Score:      pos.GetPhaseScore(),
			Phase:      string(pos.BusinessCycle),
			PhaseScore: pos.GetPhaseScore(),
			Confidence: pos.Confidence,
			IndustryID: id,
		}
	}
}

// SetCycleCardBuilder enhances the cycle provider to use composite cycle sentiment
// from CycleStatusCardBuilder, producing more accurate FactorIndustryCycle scores.
// Falls back to CycleTracker when the card builder returns no card.
func (fe *FactorEngine) SetCycleCardBuilder(cb *industry.CycleStatusCardBuilder, ct *industry.CycleTracker) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.cycleProv = func(symbol string) *domain.IndustryCycleFactorScore {
		id := classifyIndustry(symbol)
		if id == "" {
			return nil
		}
		var phase string
		var phaseScore, confidence float64
		if cb != nil {
			card, err := cb.BuildCard(time.Now(), id)
			if err == nil && card != nil {
				phase = card.BusinessCycle
				phaseScore = 1.0 + (card.CompositeCoefficient-1.0)*2.0
				confidence = card.CycleConfidence
				if phase == "" {
					phase = "unknown"
				}
				return &domain.IndustryCycleFactorScore{
					Score:      math.Max(0, math.Min(2.0, phaseScore)),
					Phase:      phase,
					PhaseScore: phaseScore,
					Confidence: confidence,
					IndustryID: id,
				}
			}
		}
		if ct != nil {
			pos, ok := ct.GetPosition(id)
			if ok {
				return &domain.IndustryCycleFactorScore{
					Score:      pos.GetPhaseScore(),
					Phase:      string(pos.BusinessCycle),
					PhaseScore: pos.GetPhaseScore(),
					Confidence: pos.Confidence,
					IndustryID: id,
				}
			}
		}
		return nil
	}
}

// WithPreciousMetalsProvider attaches a macro data provider for precious metals factor scoring.
func (fe *FactorEngine) WithPreciousMetalsProvider(fn PMContextProvider) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.pmCtxProv = fn
	return fe
}

// WithETFAnalyzer attaches an ETF analyzer for ETF-specific factor scoring.
func (fe *FactorEngine) WithETFAnalyzer(ea *ETFAnalyzer) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.etfAnalyzer = ea
	return fe
}

// GetETFAnalyzer returns the attached ETF analyzer, or nil.
func (fe *FactorEngine) GetETFAnalyzer() *ETFAnalyzer {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	return fe.etfAnalyzer
}
