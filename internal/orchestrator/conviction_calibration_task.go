package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// ConvictionCalibrationProvider reads CalibratedOrder JSONL from session
// directories and converts them to CalibRecommendation for the engine.
// Executor attribution is not in the raw JSONL yet, so calibration runs
// on aggregate data across all executors using a global meta.
type ConvictionCalibrationProvider struct {
	workDir string
}

func NewConvictionCalibrationProvider(workDir string) *ConvictionCalibrationProvider {
	return &ConvictionCalibrationProvider{workDir: workDir}
}

// Recommendations loads all recommendation outcomes from session directories.
// Since executor_id/skill is not yet in the JSONL format, all recommendations
// are returned under the single key "all" for aggregate calibration.
func (p *ConvictionCalibrationProvider) Recommendations(executorSkill string) ([]CalibRecommendation, error) {
	sessionsDir := filepath.Join(p.workDir, "data", "state", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir %s: %w", sessionsDir, err)
	}
	var all []CalibRecommendation
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		orders, err := portfolio.LoadOrdersFromJSONL(filepath.Join(sessionsDir, e.Name(), "recommendation_outcomes.jsonl"))
		if err != nil {
			continue
		}
		for _, o := range orders {
			fs := make(map[string]float64, len(o.FactorScores))
			for k, v := range o.FactorScores {
				fs[string(k)] = v
			}
			all = append(all, CalibRecommendation{
				Symbol:       o.Symbol,
				ForwardRet:   o.ForwardReturn,
				FactorScores: fs,
			})
		}
	}
	return all, nil
}

// GlobalConvictionMeta returns a synthetic StrategyMeta covering all four
// factors. Used for aggregate calibration when executor-level attribution
// is not available in the data.
func GlobalConvictionMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "all", Skill: "all",
		Description: "Aggregate factor-driven conviction across all executors",
		Factors:     []string{"momentum", "value", "quality", "liquidity"},
		Parameters: append(append(momentumParams(fc), valueParams(fc)...),
			append(qualityParams(fc), liquidityParams(fc)...)...),
	}
}

// RunConvictionCalibration loads historical data, runs calibration on aggregate
// parameters, and logs the results. Designed to be called from
// BackgroundTaskManager task functions.
func RunConvictionCalibration(workDir string) error {
	provider := NewConvictionCalibrationProvider(workDir)
	engine := &CalibrationEngine{}

	metas := []StrategyMeta{GlobalConvictionMeta()}
	reports, err := engine.CalibrateAll(metas, provider, 10)
	if err != nil {
		return fmt.Errorf("conviction calibration: %w", err)
	}
	if len(reports) == 0 {
		logging.Info("conviction_calibrate", "insufficient_data",
			"msg", "Need >= 10 samples with factor scores for calibration")
		return nil
	}
	for _, r := range reports {
		logging.Info("conviction_calibrate", "completed",
			"executor", r.ExecutorID,
			"verdict", r.Verdict,
			"baseline", r.BaselineScore,
			"optimized", r.OptimizedScore,
			"improvement", r.ImprovementPct,
			"samples", r.SamplesEvaluated)
		for name := range r.ParametersAfter {
			if r.ParametersBefore[name] != r.ParametersAfter[name] {
				logging.Info("conviction_calibrate", "param_change",
					"name", name,
					"before", r.ParametersBefore[name],
					"after", r.ParametersAfter[name])
			}
		}
		if r.Verdict == "applied" {
			if err := engine.ApplyToConfig(r); err != nil {
				logging.Error("conviction_calibrate", "apply_failed",
					"executor", r.ExecutorID,
					"err", err.Error())
			} else {
				logging.Info("conviction_calibrate", "parameters_applied",
					"executor", r.ExecutorID,
					"improvement_pct", r.ImprovementPct)
			}
		}
	}
	return nil
}
