package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

func applyMomentumCrashProtection(recs []domain.Recommendation, quotes map[string]domain.Quote) []domain.Recommendation {
	vix := 0.0
	vixFound := false
	for _, q := range quotes {
		if q.Symbol == "VIX" || q.Symbol == "^VIX" {
			vix = q.Last
			vixFound = true
			break
		}
	}
	if !vixFound {
		logging.Warn("executors", "vix_not_found", "event", "momentum_crash_protection_disabled")
		return recs
	}
	cfg := config.GetParametersConfig().Engine.Executors
	if vix <= cfg.VIXMomentumCrashThreshold.Value {
		return recs
	}

	params := config.GetParametersConfig().Orchestrator
	for i := range recs {
		if recs[i].FactorScores.Momentum == 0 {
			continue
		}
		recs[i].FactorScores.Momentum = 0
		if recs[i].FactorScores.Breakdown != nil {
			recs[i].FactorScores.Breakdown.Momentum.Score = 0
		}
		remainingWeight := params.FactorWeightValue.Value + params.FactorWeightQuality.Value + params.FactorWeightAgent.Value
		recs[i].FactorScores.Total = recs[i].FactorScores.Value*(params.FactorWeightValue.Value/remainingWeight) +
			recs[i].FactorScores.Quality*(params.FactorWeightQuality.Value/remainingWeight) +
			recs[i].FactorScores.Agent*(params.FactorWeightAgent.Value/remainingWeight)
		if recs[i].FactorScores.Breakdown != nil {
			recs[i].FactorScores.Breakdown.Total.Score = recs[i].FactorScores.Total
		}
	}
	return recs
}
