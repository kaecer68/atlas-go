package orchestrator

import (
	"hash/fnv"
	"math"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type DistributionParams struct {
	Mean      float64
	StdDev    float64
	MinReturn float64
	MaxReturn float64
}

type ForwardReturnFallback struct {
	RiskOnParams  DistributionParams
	RiskOffParams DistributionParams
}

func DefaultFallbackParams() ForwardReturnFallback {
	return ForwardReturnFallback{
		RiskOnParams: DistributionParams{
			Mean:      0.0008,
			StdDev:    0.015,
			MinReturn: -0.05,
			MaxReturn: 0.05,
		},
		RiskOffParams: DistributionParams{
			Mean:      0.0001,
			StdDev:    0.008,
			MinReturn: -0.03,
			MaxReturn: 0.03,
		},
	}
}

func GenerateForwardReturn(symbol string, quote domain.Quote, regime domain.Regime, fallback ForwardReturnFallback) float64 {
	if quote.Open > 0 && quote.Last > 0 {
		intraday := (quote.Last - quote.Open) / quote.Open

		fr := intraday
		if fr > 0.05 {
			fr = 0.05
		}
		if fr < -0.05 {
			fr = -0.05
		}

		if math.Abs(fr) < 0.001 {
			return generateFromDistribution(symbol, regime, fallback)
		}

		return fr * 0.9
	}

	return generateFromDistribution(symbol, regime, fallback)
}

func generateFromDistribution(symbol string, regime domain.Regime, fallback ForwardReturnFallback) float64 {
	params := fallback.RiskOnParams
	if regime == domain.RegimeRiskOff {
		params = fallback.RiskOffParams
	}

	hash := hashString(symbol)
	normalized := (float64(hash%10000) - 5000) / 5000.0
	fr := params.Mean + normalized*params.StdDev

	if fr < params.MinReturn {
		fr = params.MinReturn
	}
	if fr > params.MaxReturn {
		fr = params.MaxReturn
	}
	return fr
}

func hashString(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64())
}
