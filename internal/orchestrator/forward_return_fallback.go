package orchestrator

import (
	"hash/fnv"

	"github.com/kaecer68/atlas-go/internal/config"
)

// DistributionParams is one regime's placeholder return band: a mean, a
// spread, and hard clamps. It models NO forward information — see
// syntheticPlaceholderReturn in system.go for the only consumer.
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

// DefaultFallbackParams returns the two regime bands of the synthetic
// placeholder distribution, read from configs/parameters.json
// (forward_return.risk_on_* / risk_off_*).
//
// The name is historical: these params feed the placeholder written to
// RecommendationOutcome.ForwardReturn when no forward-looking data exists
// (issues #1944 I20/I18). They do not model a forward return.
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

// hashString is the deterministic, non-cryptographic seed used by
// drawNormalized (system.go) to build the placeholder draw.
//
// It exists because of the A4 L2/L3 seed bug: a symbol-only seed made every
// agent recommending the same symbol draw byte-identical values, collapsing a
// multi-agent window into a few repeated samples and exploding the rolling
// Sharpe. Callers must fold the agent (and, for the placeholder, the trading
// day) into the input.
func hashString(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	// int64(uint64) can be negative, which would make % 10000 negative in Go.
	// Normalize to a non-negative value so callers can use it as a seed.
	return int64(h.Sum64() & 0x7fffffffffffffff) //nolint:gosec // hash to int64 is deterministic for seeding, not crypto
}
