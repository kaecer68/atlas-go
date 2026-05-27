package metalearning

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/swarm"
)

// scenarioToLearningData converts a swarm TrainingScenario into a SwarmLearningData
// record suitable for the MetaLearner's genetic algorithm.
func scenarioToLearningData(s swarm.TrainingScenario) SwarmLearningData {
	acc := s.Performance.Accuracy
	convSpeed := 1.0
	if acc > 0 {
		convSpeed = acc * 100 // normalize to 0-100 range as convergence proxy
	}

	return SwarmLearningData{
		FishID:           s.ID,
		Scenario:         s.Scenario,
		LearningRate:     lrForAccuracy(acc),
		BatchSize:        batchSizeForSteps(len(s.States)),
		Epochs:           10,
		FinalAccuracy:    acc,
		ConvergenceSpeed: convSpeed,
		Stability:        stabilityFromDrawdown(s.Performance.MaxDrawdown),
		Timestamp:        time.Now(),
		StrategyParams: map[string]float64{
			"accuracy":          acc,
			"sharpe_ratio":      s.Performance.SharpeRatio,
			"max_drawdown":      s.Performance.MaxDrawdown,
			"total_predictions": float64(s.Performance.TotalPredictions),
		},
	}
}

// SubmitTrainingScenarios converts export swarm training data into the MetaLearner's
// internal format and submits each scenario for strategy evolution.
func (ml *MetaLearner) SubmitTrainingScenarios(scenarios []swarm.TrainingScenario) {
	if len(scenarios) == 0 {
		return
	}

	for _, s := range scenarios {
		data := scenarioToLearningData(s)
		ml.SubmitSwarmData(data)
	}

	// Run one evolution cycle after ingesting new data
	ml.evolvePopulation()
}

// lrForAccuracy maps fish accuracy to a learning-rate parameter.
func lrForAccuracy(acc float64) float64 {
	switch {
	case acc > 0.8:
		return 0.001 // fine-tuning
	case acc > 0.6:
		return 0.01 // moderate
	default:
		return 0.1 // exploratory
	}
}

func batchSizeForSteps(steps int) int {
	switch {
	case steps > 500:
		return 64
	case steps > 100:
		return 32
	default:
		return 16
	}
}

func stabilityFromDrawdown(drawdown float64) float64 {
	if drawdown <= 0 {
		return 1.0
	}
	stab := 1.0 - drawdown
	if stab < 0 {
		return 0
	}
	return stab
}
