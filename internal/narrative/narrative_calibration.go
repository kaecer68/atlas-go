package narrative

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// NarrativeCalibrationReport summarizes the result of a self-calibration run.
type NarrativeCalibrationReport struct {
	Timestamp        time.Time         `json:"timestamp"`
	ModelsUpdated    int               `json:"models_updated"`
	TemplatesUpdated int               `json:"templates_updated"`
	Models           []InvestmentModel `json:"models"`
	Verdict          string            `json:"verdict"`
	Summary          string            `json:"summary"`
}

// SelfCalibrate evaluates model performance against replay data and updates
// model weights and template hit rates. It orchestrates the existing
// EvaluateModels → UpdateModelWeights → updateTemplateHitRates pipeline
// and produces a calibration report.
func (ne *NarrativeEngine) SelfCalibrate(replayPath string) (*NarrativeCalibrationReport, error) {
	modelsBefore := make([]InvestmentModel, len(ne.models))
	copy(modelsBefore, ne.models)

	if err := ne.EvaluateModels(replayPath); err != nil {
		return nil, fmt.Errorf("narrative self-calibrate: %w", err)
	}

	updated := 0
	templatesUpdated := 0
	for i := range ne.models {
		if ne.models[i].Weight != modelsBefore[i].Weight {
			updated++
		}
		for _, theme := range ne.models[i].ActiveThemes {
			if tmpl, ok := ne.kb.GetTemplateByTheme(theme); ok {
				if tmpl.HistoricalHitRate != 0 {
					templatesUpdated++
				}
			}
		}
	}

	report := &NarrativeCalibrationReport{
		Timestamp:        time.Now(),
		ModelsUpdated:    updated,
		TemplatesUpdated: templatesUpdated,
		Models:           ne.ListModels(),
		Verdict:          "calibrated",
		Summary:          fmt.Sprintf("evaluated %d models, updated %d weights, %d template hit rates", len(ne.models), updated, templatesUpdated),
	}

	logging.Info(
		"narrative", "self_calibrate",
		logging.FInt("models_updated", updated),
		logging.FInt("templates_updated", templatesUpdated),
	)
	return report, nil
}
