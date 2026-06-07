package portfolio

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// QuoteProvider fetches quotes for a set of symbols.
type QuoteProvider interface {
	GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
}

func isFinite(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}

// preciousMetalsRegistry maps known precious-metal-related symbols to their subtype.
// Gold ETFs: 00635U, 00693U, 00708L (Taiwan); GLD, IAU, SGOL, BAR (international)
// Silver ETFs: SLV, SIVR, PSLV (international); no TW silver ETF as of 2026
var preciousMetalsRegistry = map[string]string{
	"00635U": "gold",
	"00693U": "gold",
	"00708L": "gold",
	"GLD":    "gold",
	"IAU":    "gold",
	"SGOL":   "gold",
	"BAR":    "gold",
	"SLV":    "silver",
	"SIVR":   "silver",
	"PSLV":   "silver",
}

// isPreciousMetal checks if a symbol is a known precious metals instrument.
func isPreciousMetal(symbol string) (bool, string) {
	subtype, ok := preciousMetalsRegistry[symbol]
	return ok, subtype
}

// IsPreciousMetal is the public version for external callers.
func (fe *FactorEngine) IsPreciousMetal(symbol string) bool {
	isPM, _ := isPreciousMetal(symbol)
	return isPM
}

type FactorEngine struct {
	history       *HistoricalPrices
	fundamentals  *FundamentalProvider
	params        *RuntimeParameters
	narrativeProv func(symbol string) *domain.NarrativeFactorScore
	cycleProv     func(symbol string) *domain.IndustryCycleFactorScore
	linkageProv   func(symbol string) *domain.LinkageFactorScore
	tsmcProv      func(symbol string) *domain.FactorScoreItem
	pmCtxProv     PMContextProvider
	etfAnalyzer   *ETFAnalyzer
	mu            sync.RWMutex
}

// PreciousMetalsContext provides macro inputs for precious metals factor scoring.
// All fields float64; NaN means "data unavailable" → corresponding sub-factor returns 0.
type PreciousMetalsContext struct {
	RealRate            float64 // real interest rate (nominal − inflation expectation)
	VIX                 float64
	DXY                 float64
	CPIYoY              float64
	CentralBankNetBuy   float64 // quarterly annualized tonnes (WGC)
	CBReserveTrend      float64 // [-1, 1] signal from CB buying trend direction
	IndiaGoldImportsYoY float64 // India gold imports YoY % change
	ChinaGoldImportsYoY float64 // China SGE withdrawal YoY % change
	COMEXNetLong        float64 // CFTC COT managed money net long contracts
	GoldSilverRatioZ    float64 // z-score vs 5y mean of gold/silver ratio
}

// PMContextProvider supplies PreciousMetalsContext for a given symbol.
type PMContextProvider func(symbol string) *PreciousMetalsContext

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

func (fe *FactorEngine) WithParameters(p *RuntimeParameters) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.params = p
	return fe
}

func (fe *FactorEngine) WithNarrativeProvider(fn func(symbol string) *domain.NarrativeFactorScore) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.narrativeProv = fn
	return fe
}

func (fe *FactorEngine) WithIndustryCycleProvider(fn func(symbol string) *domain.IndustryCycleFactorScore) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.cycleProv = fn
	return fe
}

func (fe *FactorEngine) WithLinkageProvider(fn func(symbol string) *domain.LinkageFactorScore) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.linkageProv = fn
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

// RefreshETFNAV refreshes ETF NAV values for all tracked ETFs using
// the given QuoteProvider to fetch market quotes. Returns the number of
// symbols whose NAV was updated. If the provider is nil or no
// ETFAnalyzer is attached, returns 0.
func (fe *FactorEngine) RefreshETFNAV(ctx context.Context, provider QuoteProvider) int {
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea == nil || provider == nil {
		return 0
	}

	symbols := ea.AllSymbols()
	if len(symbols) == 0 {
		return 0
	}

	quotes, err := provider.GetQuotes(ctx, time.Now(), symbols)
	if err != nil {
		logging.Warn("factor_engine", "refresh_etf_nav_failed", logging.Err(err))
		return 0
	}

	return ea.UpdateNAVFromQuotes(quotes)
}

// CalculateMomentumScore computes momentum based on price change over the configured lookback period.
// Falls back to intraday return when no historical data is available.
func (fe *FactorEngine) CalculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateMomentumDetail(symbol, quotes).Score
}

// calculateMomentumDetail returns the full breakdown for momentum calculation.
func (fe *FactorEngine) calculateMomentumDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	hp := fe.history
	fe.mu.RUnlock()

	if hp != nil {
		ret := hp.MomentumReturn(symbol, fe.params.Factor.MomentumLookbackDays)
		if ret != 0 {
			score := ret / fe.params.Factor.MomentumStdDevDivisor
			if score > 1.0 {
				score = 1.0
			}
			if score < -1.0 {
				score = -1.0
			}
			return domain.FactorScoreItem{
				Score:     score,
				Formula:   fmt.Sprintf("clamp(ret%d / %.2f, -1, 1)", fe.params.Factor.MomentumLookbackDays, fe.params.Factor.MomentumStdDevDivisor),
				RawInputs: map[string]float64{fmt.Sprintf("ret%d", fe.params.Factor.MomentumLookbackDays): ret},
			}
		}
	}

	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    fmt.Sprintf("clamp(intraday / %.2f * %.1f, -1, 1)", fe.params.Factor.MomentumIntradayThreshold, fe.params.Factor.MomentumIntradayDiscount),
			RawInputs:  map[string]float64{"open": 0, "last": 0},
			IsFallback: true,
		}
	}
	intradayReturn := (quote.Last - quote.Open) / quote.Open
	score := intradayReturn / fe.params.Factor.MomentumIntradayThreshold * fe.params.Factor.MomentumIntradayDiscount
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:      score,
		Formula:    fmt.Sprintf("clamp(intraday / %.2f * %.1f, -1, 1)", fe.params.Factor.MomentumIntradayThreshold, fe.params.Factor.MomentumIntradayDiscount),
		RawInputs:  map[string]float64{"open": quote.Open, "last": quote.Last, "intraday_return": intradayReturn},
		IsFallback: true,
	}
}

// CalculateValueScore computes value based on P/E and P/B from fundamentals.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateValueScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateValueDetail(symbol, quotes).Score
}

// calculateValueDetail returns the full breakdown for value calculation.
// Implements SCOR-02 (industry-relative P/E) and SCOR-03 (negative/undefined P/E handling).
// Precious metals (gold, silver) have no P/E or P/B — returns 0.
func (fe *FactorEngine) calculateValueDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	_ = quotes

	if isPM, _ := isPreciousMetal(symbol); isPM {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "precious_metal: P/E not applicable",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	fe.mu.RLock()
	fp := fe.fundamentals
	fe.mu.RUnlock()

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		score := 0.0
		count := 0
		raw := map[string]float64{}
		var formula string
		var isFallback bool

		// SCOR-03: Handle negative/undefined P/E
		// Check if P/E is valid (positive and not NaN/Inf)
		peValid := data.PE > 0 && isFinite(data.PE)

		if peValid {
			// SCOR-02: Industry-relative P/E comparison
			sector := data.Sector
			if sector == "" {
				sector = "other" // Default to "other" if no sector specified
			}
			sectorMedianPE := fp.SectorMedianPE(sector)

			var peScore float64
			if sectorMedianPE > 0 {
				// Relative P/E: PE / SectorMedianPE
				// PE = sector median → score 1.0
				// PE = 2x median → score 0.0
				// PE = 0.5x median → score 1.5 (capped)
				relativePE := data.PE / sectorMedianPE
				peScore = 1.0 - (relativePE - 1.0)
				raw["sector_median_pe"] = sectorMedianPE
				raw["relative_pe"] = relativePE
				formula = "clamp(1 - (PE/sectorMedianPE - 1), -1, 1)"
			} else {
				// Fallback to absolute P/E if no sector data available
				peScore = 1.0 - (data.PE-fe.params.Factor.ValuePERangeCenter)/fe.params.Factor.ValuePERangeWidth
				formula = fmt.Sprintf("clamp(1 - (PE-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePERangeCenter, fe.params.Factor.ValuePERangeWidth)
			}

			if peScore > 1.0 {
				peScore = 1.0
			}
			if peScore < -1.0 {
				peScore = -1.0
			}
			score += peScore
			count++
			raw["pe"] = data.PE
			raw["pe_score"] = peScore
		} else {
			// P/E is invalid (negative, zero, NaN, or Inf)
			// Try P/B first
			if data.PB > 0 && isFinite(data.PB) {
				pbScore := 1.0 - (data.PB-fe.params.Factor.ValuePBRangeCenter)/fe.params.Factor.ValuePBRangeWidth
				if pbScore > 1.0 {
					pbScore = 1.0
				}
				if pbScore < -1.0 {
					pbScore = -1.0
				}
				score += pbScore
				count++
				raw["pb"] = data.PB
				raw["pb_score"] = pbScore
				raw["pe_switched_to_pb"] = 1.0 // Mark that we switched from P/E to P/B
				formula = fmt.Sprintf("clamp(1 - (PB-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePBRangeCenter, fe.params.Factor.ValuePBRangeWidth)
			} else if data.PS > 0 && isFinite(data.PS) {
				// P/B also invalid, try P/S
				psScore := 1.0 - (data.PS-fe.params.Factor.ValuePSRangeCenter)/fe.params.Factor.ValuePSRangeWidth
				if psScore > 1.0 {
					psScore = 1.0
				}
				if psScore < -1.0 {
					psScore = -1.0
				}
				score += psScore
				count++
				raw["ps"] = data.PS
				raw["ps_score"] = psScore
				raw["pe_switched_to_ps"] = 1.0 // Mark that we switched from P/E to P/S
				formula = fmt.Sprintf("clamp(1 - (PS-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePSRangeCenter, fe.params.Factor.ValuePSRangeWidth)
			} else {
				// All value metrics invalid, use fallback
				isFallback = true
				formula = "fallback: no valid value metrics"
			}
		}

		// If P/E was valid, also include P/B as secondary metric
		if peValid && data.PB > 0 && isFinite(data.PB) {
			pbScore := 1.0 - (data.PB-fe.params.Factor.ValuePBRangeCenter)/fe.params.Factor.ValuePBRangeWidth
			if pbScore > 1.0 {
				pbScore = 1.0
			}
			if pbScore < -1.0 {
				pbScore = -1.0
			}
			score += pbScore
			count++
			raw["pb"] = data.PB
			raw["pb_score"] = pbScore
			formula = fmt.Sprintf("avg(clamp(1 - (PE/sectorMedianPE - 1), -1, 1), clamp(1 - (PB-%.2f)/%.2f, -1, 1))", fe.params.Factor.ValuePBRangeCenter, fe.params.Factor.ValuePBRangeWidth)
		}

		if count > 0 {
			return domain.FactorScoreItem{
				Score:      score / float64(count),
				Formula:    formula,
				RawInputs:  raw,
				IsFallback: isFallback,
			}
		}
	}
	return domain.FactorScoreItem{
		Score:      fe.params.Factor.ValueFallbackScore,
		Formula:    "fallback: no fundamentals available",
		RawInputs:  map[string]float64{},
		IsFallback: true,
	}
}

// CalculateQualityScore computes quality based on dividend yield and price stability.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateQualityScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateQualityDetail(symbol).Score
}

// calculateQualityDetail returns the full breakdown for quality calculation.
// Precious metals (gold, silver) have no ROE/profit-margin — returns 0.
func (fe *FactorEngine) calculateQualityDetail(symbol string) domain.FactorScoreItem {
	if isPM, _ := isPreciousMetal(symbol); isPM {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "precious_metal: ROE not applicable",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	fe.mu.RLock()
	fp := fe.fundamentals
	hp := fe.history
	fe.mu.RUnlock()

	score := 0.0
	count := 0
	raw := map[string]float64{}

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		if data.DividendYield > 0 {
			dyScore := data.DividendYield / fe.params.Factor.QualityDividendYieldCap
			if dyScore > 1.0 {
				dyScore = 1.0
			}
			score += dyScore
			count++
			raw["dividend_yield"] = data.DividendYield
			raw["dividend_yield_score"] = dyScore
		}
	}

	if hp != nil {
		vol := hp.Volatility(symbol, fe.params.Factor.MomentumLookbackDays)
		if vol > 0 {
			volScore := 1.0 - vol/fe.params.Factor.QualityVolatilityStd
			if volScore > 1.0 {
				volScore = 1.0
			}
			if volScore < -1.0 {
				volScore = -1.0
			}
			score += volScore
			count++
			raw[fmt.Sprintf("volatility_%dd", fe.params.Factor.MomentumLookbackDays)] = vol
			raw["volatility_score"] = volScore
		}
	}

	if count > 0 {
		return domain.FactorScoreItem{
			Score:     score / float64(count),
			Formula:   fmt.Sprintf("avg(DividendYield/%.2f, clamp(1 - Vol%dd/%.2f, -1, 1))", fe.params.Factor.QualityDividendYieldCap, fe.params.Factor.MomentumLookbackDays, fe.params.Factor.QualityVolatilityStd),
			RawInputs: raw,
		}
	}
	return domain.FactorScoreItem{
		Score:      fe.params.Factor.QualityFallbackScore,
		Formula:    fmt.Sprintf("avg(DividendYield/%.2f, clamp(1 - Vol%dd/%.2f, -1, 1))", fe.params.Factor.QualityDividendYieldCap, fe.params.Factor.MomentumLookbackDays, fe.params.Factor.QualityVolatilityStd),
		RawInputs:  map[string]float64{},
		IsFallback: true,
	}
}

func (fe *FactorEngine) CalculateInstitutionalSentimentScore(input FactorBridgeInput) domain.FactorScoreItem {
	weights := fe.params.Factor.InstitutionalSentimentWeights
	foreignWeight := weights["foreign"]
	domesticWeight := weights["domestic"]
	marginWeight := weights["margin"]
	retailWeight := weights["retail"]
	score := foreignWeight*input.ForeignFlowScore +
		domesticWeight*input.DomesticFlowScore +
		marginWeight*input.MarginBalanceScore +
		retailWeight*input.RetailSentimentScore
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: fmt.Sprintf("%.2f*ForeignFlowScore + %.2f*DomesticFlowScore + %.2f*MarginBalanceScore + %.2f*RetailSentimentScore", foreignWeight, domesticWeight, marginWeight, retailWeight),
		RawInputs: map[string]float64{
			"foreign_score":   input.ForeignFlowScore,
			"domestic_score":  input.DomesticFlowScore,
			"margin_score":    input.MarginBalanceScore,
			"retail_score":    input.RetailSentimentScore,
			"foreign_weight":  foreignWeight,
			"domestic_weight": domesticWeight,
			"margin_weight":   marginWeight,
			"retail_weight":   retailWeight,
		},
	}
}

func (fe *FactorEngine) CalculateLiquidityScore(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 || quote.Volume == 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "-log(abs(return) / volume)",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	ret := (quote.Last - quote.Open) / quote.Open
	if ret == 0 {
		return domain.FactorScoreItem{
			Score:     0.0,
			Formula:   "-log(abs(0) / volume) = 0",
			RawInputs: map[string]float64{"return": 0, "volume": float64(quote.Volume)},
		}
	}
	illiq := math.Abs(ret) / float64(quote.Volume)
	score := -math.Log(illiq + 1e-10)
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: "clamp(-log(abs(return)/volume), -1, 1)",
		RawInputs: map[string]float64{
			"return": ret,
			"volume": float64(quote.Volume),
			"illiq":  illiq,
		},
	}
}

// CalculateAllScores returns momentum, value, quality, agent, institutional sentiment,
// and liquidity scores. The agent score is computed from the provided recommendations
// for the symbol. An optional FactorBridgeInput can be provided for macro-aware factors.
func (fe *FactorEngine) CalculateAllScores(
	symbol string,
	quotes map[string]domain.Quote,
	agentRecs []domain.Recommendation,
	agentWeights map[string]float64,
	factorWeights map[FactorType]float64,
	bridgeInputs ...FactorBridgeInput,
) map[FactorType]float64 {
	momentumScore := fe.CalculateMomentumScore(symbol, quotes)
	valueScore := fe.CalculateValueScore(symbol, quotes)
	qualityScore := fe.CalculateQualityScore(symbol, quotes)

	var agentScore float64
	var totalWeight float64
	for _, rec := range agentRecs {
		if rec.Symbol != symbol {
			continue
		}
		weight := 1.0
		if w, ok := agentWeights[rec.Agent]; ok {
			weight = w
		}
		agentScore += float64(rec.Conviction) * weight / 100.0
		totalWeight += weight
	}
	if totalWeight > 0 {
		agentScore /= totalWeight
	}

	result := map[FactorType]float64{
		FactorMomentum: momentumScore,
		FactorValue:    valueScore,
		FactorQuality:  qualityScore,
		FactorAgent:    agentScore,
	}

	// Add macro-aware factors if bridge input available
	if len(bridgeInputs) > 0 {
		input := bridgeInputs[0]
		instSentScore := fe.CalculateInstitutionalSentimentScore(input)
		liqScore := fe.CalculateLiquidityScore(symbol, quotes)
		result[FactorInstSent] = instSentScore.Score
		result[FactorLiquidity] = liqScore.Score
	}

	// Precious Metals factor
	if isPM, _ := isPreciousMetal(symbol); isPM {
		pmScore := fe.CalculatePreciousMetalsScore(symbol, quotes)
		result[FactorPreciousMetals] = pmScore.Score
	}

	// ETF factor
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea != nil && ea.IsETF(symbol) {
		if quote, ok := quotes[symbol]; ok {
			etfScore := fe.CalculateETFScore(symbol, quote)
			result[FactorETF] = etfScore.Score
		}
	}

	// Compute weighted total if factorWeights provided
	if len(factorWeights) > 0 {
		total := 0.0
		for ft, score := range result {
			if w, ok := factorWeights[ft]; ok {
				total += score * w
			}
		}
		result["total"] = total
	}

	return result
}

// CalculateAllScoresWithBreakdown returns both the score map and a detailed
// breakdown showing formulas, raw inputs, and fallback flags per factor.
// An optional FactorBridgeInput can be provided for macro-aware institutional sentiment and liquidity.
func (fe *FactorEngine) CalculateAllScoresWithBreakdown(
	symbol string,
	quotes map[string]domain.Quote,
	agentRecs []domain.Recommendation,
	agentWeights map[string]float64,
	factorWeights map[FactorType]float64,
	bridgeInputs ...FactorBridgeInput,
) (*domain.FactorScoreBreakdown, map[FactorType]float64) {
	mom := fe.calculateMomentumDetail(symbol, quotes)
	val := fe.calculateValueDetail(symbol, quotes)
	qly := fe.calculateQualityDetail(symbol)

	var agentScore float64
	var totalWeight float64
	rawAgent := map[string]float64{}
	for _, rec := range agentRecs {
		if rec.Symbol != symbol {
			continue
		}
		weight := 1.0
		if w, ok := agentWeights[rec.Agent]; ok {
			weight = w
		}
		agentScore += float64(rec.Conviction) * weight / 100.0
		totalWeight += weight
		rawAgent[rec.Agent+"_conviction"] = float64(rec.Conviction)
		rawAgent[rec.Agent+"_weight"] = weight
	}
	if totalWeight > 0 {
		agentScore /= totalWeight
	}

	agent := domain.FactorScoreItem{
		Score:     agentScore,
		Formula:   "weighted_avg(Conviction / 100)",
		RawInputs: rawAgent,
	}

	var instSent, liq domain.FactorScoreItem
	if len(bridgeInputs) > 0 {
		input := bridgeInputs[0]
		instSent = fe.CalculateInstitutionalSentimentScore(input)
		liq = fe.CalculateLiquidityScore(symbol, quotes)
	}

	result := map[FactorType]float64{
		FactorMomentum: mom.Score,
		FactorValue:    val.Score,
		FactorQuality:  qly.Score,
		FactorAgent:    agent.Score,
	}

	var nar, icl, link domain.FactorScoreItem
	fe.mu.RLock()
	narProv := fe.narrativeProv
	iclProv := fe.cycleProv
	linkProv := fe.linkageProv
	fe.mu.RUnlock()

	if narProv != nil {
		if nfs := narProv(symbol); nfs != nil {
			nar = domain.FactorScoreItem{
				Score:     nfs.Score,
				Formula:   fmt.Sprintf("narrative(theme=%s, hit_rate=%.2f)", nfs.Theme, nfs.HitRate),
				RawInputs: map[string]float64{"theme_hit_rate": nfs.HitRate, "confidence": nfs.Confidence},
			}
			result[FactorNarrative] = nar.Score
		}
	}
	if iclProv != nil {
		if ics := iclProv(symbol); ics != nil {
			icl = domain.FactorScoreItem{
				Score:     ics.Score,
				Formula:   fmt.Sprintf("industry_cycle(phase=%s, phase_score=%.2f)", ics.Phase, ics.PhaseScore),
				RawInputs: map[string]float64{"phase_score": ics.PhaseScore, "confidence": ics.Confidence},
			}
			result[FactorIndustryCycle] = icl.Score
		}
	}
	if linkProv != nil {
		if lfs := linkProv(symbol); lfs != nil {
			link = domain.FactorScoreItem{
				Score:     lfs.Score,
				Formula:   fmt.Sprintf("linkage(systemic=%.2f, propagation=%.2f)", lfs.SystemicImportance, lfs.ShockPropagation),
				RawInputs: map[string]float64{"systemic_importance": lfs.SystemicImportance, "shock_propagation_speed": lfs.ShockPropagation, "avg_correlation": lfs.AvgCorrelation},
			}
			result[FactorLinkage] = link.Score
		}
	}

	breakdown := &domain.FactorScoreBreakdown{
		Momentum:               mom,
		Value:                  val,
		Quality:                qly,
		Agent:                  agent,
		InstitutionalSentiment: instSent,
		Liquidity:              liq,
		Narrative:              nar,
		IndustryCycle:          icl,
		Linkage:                link,
		TSMC: domain.FactorScoreItem{},
	}

	// Precious Metals: compute PM score when symbol is a known PM instrument.
	var pm domain.FactorScoreItem
	if isPM, _ := isPreciousMetal(symbol); isPM {
		pm = fe.CalculatePreciousMetalsScore(symbol, quotes)
		result[FactorPreciousMetals] = pm.Score
		breakdown.PreciousMetals = pm
	}

	// ETF: compute ETF factor score when ETF analyzer data is available.
	var etf domain.FactorScoreItem
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea != nil && ea.IsETF(symbol) {
		if quote, ok := quotes[symbol]; ok {
			etf = fe.CalculateETFScore(symbol, quote)
			result[FactorETF] = etf.Score
			breakdown.ETF = etf
		}
	}

	if len(bridgeInputs) > 0 {
		result[FactorInstSent] = instSent.Score
		result[FactorLiquidity] = liq.Score
	}

	// SCOR-04: Apply reduced weight for fallback factors in total calculation
	if len(factorWeights) > 0 {
		total := 0.0
		rawTotal := map[string]float64{}

		getEffectiveWeight := func(item domain.FactorScoreItem, defaultWeight float64) float64 {
			if item.IsFallback {
				return defaultWeight * fe.params.Factor.FallbackWeightReduction
			}
			return defaultWeight
		}

		momWeight := getEffectiveWeight(mom, factorWeights[FactorMomentum])
		total += mom.Score * momWeight
		rawTotal[string(FactorMomentum)] = mom.Score * momWeight

		valWeight := getEffectiveWeight(val, factorWeights[FactorValue])
		total += val.Score * valWeight
		rawTotal[string(FactorValue)] = val.Score * valWeight

		qlyWeight := getEffectiveWeight(qly, factorWeights[FactorQuality])
		total += qly.Score * qlyWeight
		rawTotal[string(FactorQuality)] = qly.Score * qlyWeight

		agentWeight := getEffectiveWeight(agent, factorWeights[FactorAgent])
		total += agent.Score * agentWeight
		rawTotal[string(FactorAgent)] = agent.Score * agentWeight

		if len(bridgeInputs) > 0 {
			instWeight := getEffectiveWeight(instSent, factorWeights[FactorInstSent])
			total += instSent.Score * instWeight
			rawTotal[string(FactorInstSent)] = instSent.Score * instWeight

			liqWeight := getEffectiveWeight(liq, factorWeights[FactorLiquidity])
			total += liq.Score * liqWeight
			rawTotal[string(FactorLiquidity)] = liq.Score * liqWeight
		}

		if nar.Score != 0 || nar.Formula != "" {
			narWeight := getEffectiveWeight(nar, factorWeights[FactorNarrative])
			total += nar.Score * narWeight
			rawTotal[string(FactorNarrative)] = nar.Score * narWeight
			breakdown.Narrative.Weight = factorWeights[FactorNarrative]
		}
		if icl.Score != 0 || icl.Formula != "" {
			iclWeight := getEffectiveWeight(icl, factorWeights[FactorIndustryCycle])
			total += icl.Score * iclWeight
			rawTotal[string(FactorIndustryCycle)] = icl.Score * iclWeight
			breakdown.IndustryCycle.Weight = factorWeights[FactorIndustryCycle]
		}
		if pm.Score != 0 || pm.Formula != "" {
			pmWeight := getEffectiveWeight(pm, factorWeights[FactorPreciousMetals])
			total += pm.Score * pmWeight
			rawTotal[string(FactorPreciousMetals)] = pm.Score * pmWeight
			breakdown.PreciousMetals.Weight = factorWeights[FactorPreciousMetals]
		}
		if etf.Score != 0 || etf.Formula != "" {
			etfWeight := getEffectiveWeight(etf, factorWeights[FactorETF])
			total += etf.Score * etfWeight
			rawTotal[string(FactorETF)] = etf.Score * etfWeight
			breakdown.ETF.Weight = factorWeights[FactorETF]
		}

		result["total"] = total
		breakdown.Total = domain.FactorScoreItem{
			Score:     total,
			Formula:   "sum(factor_score * effective_weight)",
			RawInputs: rawTotal,
		}
		breakdown.Momentum.Weight = factorWeights[FactorMomentum]
		breakdown.Value.Weight = factorWeights[FactorValue]
		breakdown.Quality.Weight = factorWeights[FactorQuality]
		breakdown.Agent.Weight = factorWeights[FactorAgent]
		if len(bridgeInputs) > 0 {
			breakdown.InstitutionalSentiment.Weight = factorWeights[FactorInstSent]
			breakdown.Liquidity.Weight = factorWeights[FactorLiquidity]
		}
	}

	return breakdown, result
}

// CalculateETFScore returns an ETF-specific composite factor score.
// Delegates to the attached ETFAnalyzer. Returns a zero-score fallback if no analyzer is attached.
func (fe *FactorEngine) CalculateETFScore(symbol string, quote domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.etfAnalyzer == nil {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "etf_score: no analyzer attached",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	return fe.etfAnalyzer.CalculateETFScore(symbol, quote)
}

// ── Precious Metals Factor (P0-2) ──
// SOURCE: Erb & Harvey (2013), World Gold Council, P0-2 task brief
//
// Composite precious metals score for gold:
//   PM_gold = 0.16·RealRate + 0.10·DXY + 0.10·InflExp + 0.12·CB + 0.08·Flow + 0.06·PhyDem + 0.10·COMEX + 0.28·RiskOff
// For silver:
//   PM_silver = 0.60·PM_gold + 0.15·Industrial + 0.10·GoldSilver + 0.15·COMEX

// CalculatePreciousMetalsScore returns the composite precious metals factor score.
// Returns 0 if symbol is not a known precious metal instrument.
func (fe *FactorEngine) CalculatePreciousMetalsScore(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	isPM, subtype := isPreciousMetal(symbol)
	if !isPM {
		return domain.FactorScoreItem{Score: 0.0, Formula: "not_precious_metal"}
	}

	ctx := fe.getPMContext(symbol)

	realRate := fe.pmRealRateScore(ctx)
	dxy := fe.pmDXYScore(ctx)
	inflExp := fe.pmInflationExpectScore(ctx)
	cbBuy := fe.pmCentralBankScore(ctx)
	etfFlow := fe.pmETFFlowScore()
	riskOff := fe.pmRiskOffScore(ctx)
	physDemand := fe.pmPhysicalDemandScore(ctx)
	comex := fe.pmCOMEXScore(ctx)

	goldScore := 0.16*realRate + 0.10*dxy + 0.10*inflExp + 0.12*cbBuy + 0.08*etfFlow + 0.06*physDemand + 0.10*comex + 0.28*riskOff

	if riskOff >= 1.0 && goldScore < 0.5 {
		goldScore = 0.5
	}

	score := goldScore
	formula := "gold: 0.16*RR + 0.10*DXY + 0.10*Inf + 0.12*CB + 0.08*Flow + 0.06*PhyDem + 0.10*COMEX + 0.28*RiskOff"

	if subtype == "silver" {
		indDemand := fe.pmIndustrialDemandScore(ctx)
		gsRatio := fe.pmGoldSilverRatioScore(ctx)
		score = 0.60*goldScore + 0.15*indDemand + 0.10*gsRatio + 0.15*comex
		formula = "silver: 0.60*PM_gold + 0.15*Ind + 0.10*GS + 0.15*COMEX"
	}

	return domain.FactorScoreItem{
		Score:   score,
		Formula: formula,
		RawInputs: map[string]float64{
			"real_rate":       realRate,
			"dxy":             dxy,
			"inflation":       inflExp,
			"cb_buy":          cbBuy,
			"etf_flow":        etfFlow,
			"risk_off":        riskOff,
			"physical_demand": physDemand,
			"comex":           comex,
			"gold_composite":  goldScore,
		},
	}
}

func (fe *FactorEngine) getPMContext(symbol string) *PreciousMetalsContext {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.pmCtxProv != nil {
		return fe.pmCtxProv(symbol)
	}
	return nil
}

// pmRealRateScore: −normalize(r_real). Higher real rates → lower gold demand.
// Uses linear scaling with center at 1.5%: r=0.5%→+0.67, r=2%→−0.33.
// SOURCE: Erb & Harvey (2013)
func (fe *FactorEngine) pmRealRateScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.RealRate) {
		return 0
	}
	r := ctx.RealRate
	score := -(r*100 - 1.5) / 1.5
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// pmDXYScore: −normalize(DXY change). Stronger dollar → lower gold.
// Center at 100; ±10 point move = ±1.0.
func (fe *FactorEngine) pmDXYScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.DXY) {
		return 0
	}
	score := -(ctx.DXY - 100) / 10.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// pmInflationExpectScore: normalize(CPI). Higher inflation → higher gold.
// Bonus +0.2 if CPI > 3%; capped at 1.0.
func (fe *FactorEngine) pmInflationExpectScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.CPIYoY) {
		return 0
	}
	score := (ctx.CPIYoY - 0.02) * 50.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	if ctx.CPIYoY > 3.0 {
		score += 0.2
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// pmCentralBankScore: heuristic based on CBReserveTrend from context.
// ReserveTrend > 0.3 → +0.7, > 0 → +0.3, > −0.3 → 0, > −1 → −0.2.
// SOURCE: World Gold Council quarterly report, central bank gold reserve trend.
func (fe *FactorEngine) pmCentralBankScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.CBReserveTrend) {
		return 0
	}
	switch {
	case ctx.CBReserveTrend > 0.3:
		return 0.7
	case ctx.CBReserveTrend > 0:
		return 0.3
	case ctx.CBReserveTrend > -0.3:
		return 0
	case ctx.CBReserveTrend > -1:
		return -0.2
	default:
		return -0.2
	}
}

// pmETFFlowScore: uses GLD 20d momentum as ETF flow proxy.
// GLD > 5% → +0.5 (strong inflows), > 0 → +0.2, < −5% → −0.3, else 0.
// Falls back to 0 if GLD price history unavailable.
func (fe *FactorEngine) pmETFFlowScore() float64 {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.history == nil {
		return 0
	}
	ret20d := fe.history.MomentumReturn("GLD", 20)
	switch {
	case ret20d > 0.05:
		return 0.5
	case ret20d > 0:
		return 0.2
	case ret20d < -0.05:
		return -0.3
	default:
		return 0
	}
}

// pmRiskOffScore: normalize(VIX). Higher VIX → higher gold.
// Bonus +0.25 if VIX > 25; floor at 1.0 if VIX > 35 (extreme panic).
func (fe *FactorEngine) pmRiskOffScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.VIX) {
		return 0
	}
	score := (ctx.VIX - 20) / 20.0
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	if ctx.VIX > 35 {
		return 1.0
	}
	if ctx.VIX > 25 {
		score += 0.25
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// pmPhysicalDemandScore: composite of India and China gold import trends.
// Normalizes imports to [-1, 1]: 10% YoY → +1.0, −10% → −1.0, linear between.
// 0.5 * India score + 0.5 * China score.
func (fe *FactorEngine) pmPhysicalDemandScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil {
		return 0
	}
	indiaScore := fe.normalizeImportYoY(ctx.IndiaGoldImportsYoY)
	chinaScore := fe.normalizeImportYoY(ctx.ChinaGoldImportsYoY)
	return 0.5*indiaScore + 0.5*chinaScore
}

func (fe *FactorEngine) normalizeImportYoY(yoy float64) float64 {
	if math.IsNaN(yoy) {
		return 0
	}
	score := yoy / 10.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// pmCOMEXScore: contrarian COT signal. COMEX net long > 200k → −0.5 (too bullish),
// < 50k → +0.5 (too bearish), between → 0.
func (fe *FactorEngine) pmCOMEXScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.COMEXNetLong) {
		return 0
	}
	switch {
	case ctx.COMEXNetLong > 200000:
		return -0.5
	case ctx.COMEXNetLong < 50000:
		return 0.5
	default:
		return 0
	}
}

// pmIndustrialDemandScore: VIX as PMI proxy. VIX < 15 → +0.5 (strong PMI),
// VIX < 20 → +0.2, VIX > 25 → −0.3, VIX > 30 → −0.5.
func (fe *FactorEngine) pmIndustrialDemandScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.VIX) {
		return 0
	}
	switch {
	case ctx.VIX < 15:
		return 0.5
	case ctx.VIX < 20:
		return 0.2
	case ctx.VIX > 30:
		return -0.5
	case ctx.VIX > 25:
		return -0.3
	default:
		return 0
	}
}

// pmGoldSilverRatioScore: mean-reversion signal. GoldSilverRatioZ > 1.5 → +0.5
// (gold overvalued, bullish silver mean reversion). Z < −1.5 → −0.3.
// Linear interpolation between.
func (fe *FactorEngine) pmGoldSilverRatioScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.GoldSilverRatioZ) {
		return 0
	}
	z := ctx.GoldSilverRatioZ
	if z > 1.5 {
		return 0.5
	}
	if z < -1.5 {
		return -0.3
	}
	if z > 0 {
		return z / 1.5 * 0.5
	}
	return z / 1.5 * 0.3
}

func classifyIndustry(symbol string) string {
	switch symbol {
	case "2330.TW", "2454.TW", "2303.TW":
		return "semiconductor"
	case "2317.TW", "2382.TW":
		return "electronics"
	case "2881.TW", "2882.TW", "2891.TW":
		return "financials"
	case "2603.TW", "2609.TW", "2615.TW":
		return "shipping"
	default:
		return ""
	}
}
