package orchestrator

import (
	"hash/fnv"
	"math"

	"github.com/kaecer68/atlas-go/internal/config"
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

func DefaultFallbackParams(cfg *config.ParametersConfig) ForwardReturnFallback {
	return ForwardReturnFallback{
		RiskOnParams: DistributionParams{
			Mean:      cfg.ForwardReturn.RiskOnMean.Value,
			StdDev:    cfg.ForwardReturn.RiskOnStdDev.Value,
			MinReturn: -0.05,
			MaxReturn: 0.05,
		},
		RiskOffParams: DistributionParams{
			Mean:      cfg.ForwardReturn.RiskOffMean.Value,
			StdDev:    cfg.ForwardReturn.RiskOffStdDev.Value,
			MinReturn: -0.03,
			MaxReturn: 0.03,
		},
	}
}

// GenerateForwardReturn produces a forward-return proxy for a recommendation.
// agentID scopes the synthetic distribution seed so different agents drawing
// a fallback for the same symbol get different values (A4 L2: the seed used
// to be symbol-only, which made every agent recommending the same symbol
// receive byte-identical forward returns).
func GenerateForwardReturn(symbol, agentID string, quote domain.Quote, regime domain.Regime, fallback ForwardReturnFallback) float64 {
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
			return generateFromDistribution(symbol, agentID, regime, fallback)
		}

		return fr * 0.9
	}

	return generateFromDistribution(symbol, agentID, regime, fallback)
}

func generateFromDistribution(symbol, agentID string, regime domain.Regime, fallback ForwardReturnFallback) float64 {
	params := fallback.RiskOnParams
	if regime == domain.RegimeRiskOff {
		params = fallback.RiskOffParams
	}

	// Agent-scoped seed: the same symbol drawn by different agents must yield
	// different samples, otherwise multi-agent windows collapse into a few
	// repeated values and the rolling Sharpe explodes (A4 L2/L3).
	hash := hashString(agentID + "|" + symbol)
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
	// int64(uint64) can be negative, which would make % 10000 negative in Go.
	// Normalize to a non-negative value so callers can use it as a seed.
	return int64(h.Sum64() & 0x7fffffffffffffff) //nolint:gosec // hash to int64 is deterministic for seeding, not crypto
}
