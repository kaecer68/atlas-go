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
type ConvictionCalibrationProvider struct {
	workDir string
}

func NewConvictionCalibrationProvider(workDir string) *ConvictionCalibrationProvider {
	return &ConvictionCalibrationProvider{workDir: workDir}
}

// Recommendations loads recommendation outcomes from session directories and
// filters by executor skill. When executorSkill is empty or "all", returns
// all recommendations without filtering.
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
			// Filter by executor skill if specified
			if executorSkill != "" && executorSkill != "all" && o.Skill != executorSkill {
				continue
			}
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

// RunConvictionCalibration loads historical data, runs calibration on the
// specified executors' parameters, logs the results, and auto-applies when
// improvement exceeds 5%. When no metas are provided, defaults to aggregate
// calibration across all four factors.
func RunConvictionCalibration(workDir string, metas ...StrategyMeta) error {
	provider := NewConvictionCalibrationProvider(workDir)
	engine := &CalibrationEngine{}

	if len(metas) == 0 {
		fc := loadFactorConfig()
		metas = []StrategyMeta{{
			ID: "all", Skill: "all",
			Description: "Aggregate factor-driven conviction across all executors",
			Factors:     []string{"momentum", "value", "quality", "liquidity"},
			Parameters: append(append(momentumParams(fc), valueParams(fc)...),
				append(qualityParams(fc), liquidityParams(fc)...)...),
		}}
	}
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
