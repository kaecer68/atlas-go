package marketdata

import (
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// MicrostructureSnapshot holds proxy microstructure metrics derived from OHLCV.
type MicrostructureSnapshot struct {
	Symbol             string    `json:"symbol"`
	AsOf               time.Time `json:"as_of"`
	LiquidityScore     float64   `json:"liquidity_score"`     // 0-100
	SpreadEstimate     float64   `json:"spread_estimate"`     // estimated spread (%)
	VolumeSurgeRatio   float64   `json:"volume_surge_ratio"`  // volume / 20d avg
	RealizedVolatility float64   `json:"realized_volatility"` // 5-day realized vol
	AbnormalVolume     bool      `json:"abnormal_volume"`     // volume > 3x avg
	PriceGapRisk       bool      `json:"price_gap_risk"`      // high vol + low liquidity
	TradeabilityScore  float64   `json:"tradeability_score"`  // 0-100 composite
}

// MicrostructureProvider calculates microstructure proxies from quote data.
type MicrostructureProvider struct {
	avgVolumeLookup func(symbol string) int64
}

// NewMicrostructureProvider creates a provider.
func NewMicrostructureProvider(avgVolumeLookup func(symbol string) int64) *MicrostructureProvider {
	return &MicrostructureProvider{avgVolumeLookup: avgVolumeLookup}
}

// Calculate computes microstructure metrics for a single symbol.
func (p *MicrostructureProvider) Calculate(symbol string, quote domain.Quote) MicrostructureSnapshot {
	avgVol := p.avgVolumeLookup(symbol)
	if avgVol == 0 {
		avgVol = quote.Volume
	}

	spread := p.calculateSpreadEstimate(quote, avgVol)
	liquidity := p.calculateLiquidityScore(quote.Volume, avgVol, spread)
	volSurge := float64(quote.Volume) / float64(avgVol)
	abnormal := volSurge > 3.0

	realizedVol := (quote.High - quote.Low) / quote.Last
	if quote.Last == 0 {
		realizedVol = 0
	}

	tradeability := liquidity * (1.0 - math.Min(realizedVol*2.0, 0.5))

	return MicrostructureSnapshot{
		Symbol:             symbol,
		AsOf:               time.Now().UTC(),
		LiquidityScore:     liquidity,
		SpreadEstimate:     spread,
		VolumeSurgeRatio:   volSurge,
		RealizedVolatility: realizedVol,
		AbnormalVolume:     abnormal,
		PriceGapRisk:       spread > 0.02 && realizedVol > 0.05,
		TradeabilityScore:  math.Max(0, math.Min(100, tradeability)),
	}
}

func (p *MicrostructureProvider) calculateSpreadEstimate(quote domain.Quote, avgVolume int64) float64 {
	if quote.Last == 0 || avgVolume == 0 {
		return 0.01
	}

	priceRange := (quote.High - quote.Low) / quote.Last
	volumeFactor := 1.0 / math.Sqrt(float64(quote.Volume)/float64(avgVolume))

	return priceRange * volumeFactor
}

func (p *MicrostructureProvider) calculateLiquidityScore(volume, avgVolume int64, spread float64) float64 {
	volumeScore := math.Min(float64(volume)/float64(avgVolume)*25.0, 50.0)
	spreadScore := math.Max(0, 50.0-spread*1000.0)

	return volumeScore + spreadScore
}
