package calibration

import (
	"fmt"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/config"
)

func Run(workDir, module, dataPath string, dryRun, verbose bool) (string, error) {
	replayPath := dataPath
	if replayPath == "" {
		if replayPath = config.GetReplayDataPath(workDir); replayPath == "" {
			replayPath = filepath.Join(workDir, "data", "replay", "tw_extended_90days.csv")
		}
	}
	paramsPath := filepath.Join(workDir, "configs", "parameters.json")
	paramsCfg, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return "", fmt.Errorf("load parameters config: %w", err)
	}
	returns, n, err := LoadReturns(replayPath)
	if err != nil {
		return "", fmt.Errorf("load returns: %w", err)
	}
	results, err := CalibrateModule(module, config.NewInferenceEngine(paramsCfg), returns, n, paramsCfg)
	if err != nil {
		return "", err
	}
	report := FormatReport(results, verbose)
	if dryRun {
		return report + "\n[DRY-RUN] No changes written.\n", nil
	}
	if err := SaveResults(paramsCfg, results, paramsPath); err != nil {
		return "", err
	}
	return report + fmt.Sprintf("\nSaved updated parameters to %s\n", paramsPath), nil
}
