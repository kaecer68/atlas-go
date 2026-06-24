package portfolio

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteProvider fetches quotes for a set of symbols.
type QuoteProvider interface {
	GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
}

// CorporateActionProvider fetches corporate action data for a symbol within a date range.
// It abstracts the data source for events such as cash dividends, stock dividends,
// and capital reductions that require historical price back-adjustment.
type CorporateActionProvider interface {
	GetCorporateActions(ctx context.Context, symbol string, start, end time.Time) ([]domain.CorporateAction, error)
}

// FactorEngine is the multi-factor scoring engine for portfolio optimization.
// It aggregates historical price, fundamental, and provider-injected data into
// normalized [-1, 1] factor scores (Momentum, Value, Quality, InstitutionalSentiment,
// Liquidity, Narrative, IndustryCycle, Linkage, TSMC, PreciousMetals, ETF).
//
// All providers are optional; missing providers yield fallback scores. The struct
// uses two mutexes: mu (RWMutex) guards read/write access to attached providers
// (history, fundamentals, params, providers, etfAnalyzer); adjustedMu (Mutex) guards
// the corporate-action adjustment cache (adjustedSymbols).
type FactorEngine struct {
	history         *HistoricalPrices
	fundamentals    *FundamentalProvider
	params          *RuntimeParameters
	narrativeProv   NarrativeProviderFunc
	cycleProv       IndustryCycleProviderFunc
	linkageProv     LinkageProviderFunc
	tsmcProv        TSMCProviderFunc
	pmCtxProv       PMContextProvider
	corpActions     CorporateActionProvider
	etfAnalyzer     *ETFAnalyzer
	mu              sync.RWMutex
	adjustedMu      sync.Mutex
	adjustedSymbols map[string]time.Time
	adjustmentTTL   time.Duration
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

// NarrativeProviderFunc returns the narrative factor score for a symbol.
// Nil return means the symbol has no narrative context.
type NarrativeProviderFunc func(symbol string) *domain.NarrativeFactorScore

// IndustryCycleProviderFunc returns the industry cycle factor score for a symbol.
// Nil return means no cycle position is available for the symbol's industry.
type IndustryCycleProviderFunc func(symbol string) *domain.IndustryCycleFactorScore

// LinkageProviderFunc returns the linkage factor score for a symbol.
// Nil return means no linkage information is available.
type LinkageProviderFunc func(symbol string) *domain.LinkageFactorScore

// TSMCProviderFunc returns the TSMC factor score for a symbol.
// Nil return means no TSMC relevance for the symbol.
type TSMCProviderFunc func(symbol string) *domain.FactorScoreItem
